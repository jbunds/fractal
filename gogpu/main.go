// Package mandelbrot/gogpu renders the Mandelbrot set and zooms in on preset coordinates.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"maps"
	"math"
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // enable GPU-bound rendering and rasterized tiles
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

//go:embed mandelbrot.wgsl
var shaderCode string

const (
	width          = 800   // logical application window width  (points)
	height         = 800   // logical application window height (points)
	baseIterations = 500   // initial number of iterations used to compute interior boundaries
	paletteSize    = 2000  // number of colors to pre-compute and pass to the GPU shader for fast lookup
	initialZoom    = 3.0   // initial magnification factor of the rendered image
	zoomFactor     = 0.993 // multiplicative factor by which the rendering is iteratively magnified
	growthRate     = 0.2   // multiplicative factor by which boundary calculation iterations increases per each successive magnification
)

type context struct {
	frameCounter    int                    // tracks the number of frames rendered
	zoom            float64                // tracks the magnification factor of the current frame
	fps             float64                // imprecisely tracks FPS rendered
	targetXHi,
	targetYHi,
	targetXLo,
	targetYLo       float32                // double-precision target coordinates in the complex plane
	paletteColors   []uint32               // pre-computed color palette

	canvas          *ggcanvas.Canvas       // wraps gg.Context with GoGPU integration to manage the CPU-to-GPU pipeline
	cc              *gg.Context            // drawing context
	device          *wgpu.Device           // logical GPU device

	paletteBuf      *wgpu.Buffer           // pre-computed color palette buffer
	uniformBuf      *wgpu.Buffer
	staticBindGroup *wgpu.BindGroup        // uniforms and color palette buffers
	bgLayout0       *wgpu.BindGroupLayout  // uniforms and color palette layout
	bgLayout1       *wgpu.BindGroupLayout  // storage texture layout
	pipeline        *wgpu.ComputePipeline  // configures the GPU compute pipeline

	fractalView     gpucontext.TextureView // handle to the fractal texture view
	relFractalView  func()
}

type uniforms struct { // total: (1 uint32 field + 11 float32 fields) * 4 bytes == 48 bytes
	paletteSize                                    uint32
	frameCounter, iterations, pad                  float32 // block 1
	width,        height,     zoomHi,    zoomLo    float32 // block 2
	targetXHi,    targetYHi,  targetXLo, targetYLo float32 // block 3
}

func main() {
	var (
		initTokenOnce  sync.Once
		animToken      atomic.Pointer[gogpu.AnimationToken]
		lastFrameTime  time.Time
		currentContext atomic.Value
	)

	coords, err := flags(flag.CommandLine, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot parse flags: %v\n", err)
		os.Exit(1)
	}

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithAppName("Mandelbrot").
		WithTitle(fmt.Sprintf("mandelbrot - %s", coords.name)).
		WithSize(width, height))

	currentContext.Store(newContext(coords))

	customizeMenus(&currentContext, app, coords.name)

	// GoGPU callbacks

	app.OnSurfaceAvailable(func() {
		currentContext.Load().(*context).setup(app)
	})

	events := app.EventSource()

	events.OnKeyPress(func(key gpucontext.Key, _ gpucontext.Modifiers) {
		if key == gpucontext.KeySpace {
			if oldToken := animToken.Swap(nil); oldToken != nil {
				oldToken.Stop()
			} else {
				animToken.Store(app.StartAnimation())
			}
		}
	})

	app.OnDraw(func(dc *gogpu.Context) {
		cc := currentContext.Load().(*context)
		cc.draw(dc, &animToken)

		elapsed := time.Since(lastFrameTime).Milliseconds()
		if elapsed > 0 {
			cc.fps = float64(1000 / elapsed)
		}
		lastFrameTime = time.Now()

		initTokenOnce.Do(func() {
			animToken.Store(app.StartAnimation())
		})

		go func() {
			runtime.Gosched() // yield CPU to allow other goroutines to run
			if animToken.Load() != nil {
				app.RequestRedraw()
			}
		}()

	})

	app.OnClose(func() {
		currentContext.Load().(*context).release()
	})

	lastFrameTime = time.Now()

	// main event loop

	if err := app.Run(); err != nil {
		panic(err)
	}
}

func customizeMenus(cc *atomic.Value, app *gogpu.App, item string) {
	// TODO(jbunds): implement a custom "About Mandelbrot" window

	// custom "Points" menu
	points     := pointsOfInterest()
	pointsMenu := gogpu.NewMenuWithTitle("Points")

	for _, v := range slices.Sorted(maps.Keys(points)) {
		pointsMenu.AddItem(gogpu.MenuItem{Title: v, Disabled: v == item, Action: func() {
			newContext := newContext(points[v])
			oldContext := cc.Swap(newContext).(*context)
			oldContext.release()
			newContext.setup(app)
			app.SetTitle(fmt.Sprintf("mandelbrot - %s", v))
			app.RequestRedraw()
		}})
	}

	app.SetCustomMenu("points", pointsMenu)
}

func newContext(coords coords) *context {
	targetXHi, targetXLo := splitFloat64(coords.x)
	targetYHi, targetYLo := splitFloat64(coords.y)
	return &context{
		frameCounter: 0,
		zoom:         initialZoom,
		targetXHi:    targetXHi,
		targetXLo:    targetXLo,
		targetYHi:    targetYHi,
		targetYLo:    targetYLo,
	}
}

// setup initializes all resources required to render frames in the main application window.
func (c *context) setup(app *gogpu.App) {
	c.device = app.DeviceProvider().Device()

	paletteColors,
		paletteBuf,
		uniformBuf,
		bgLayout0,
		bgLayout1,
		pipeline := initResources(c.device, baseIterations)

	c.paletteColors = paletteColors
	c.paletteBuf    = paletteBuf
	c.uniformBuf    = uniformBuf
	c.bgLayout0     = bgLayout0
	c.bgLayout1     = bgLayout1
	c.pipeline      = pipeline

	var err error
	c.canvas, err = ggcanvas.New(app.GPUContextProvider(), width, height)
	if err != nil { panic(err) }

	c.cc = c.canvas.Context()
	if err := c.cc.LoadFontFace("/System/Library/Fonts/Supplemental/Verdana.ttf", 12) // TODO(jbunds): make portable
	err != nil { panic(err) }

	err = c.device.Queue().WriteBuffer(c.paletteBuf, 0, unsafe.Slice((*byte)(unsafe.Pointer(&c.paletteColors[0])), len(c.paletteColors) * 4))
	if err != nil { panic(err) }

	c.staticBindGroup, err = c.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout:  c.bgLayout0,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Size: 48,                      Buffer: c.uniformBuf},
			{Binding: 1, Size: uint64(paletteSize * 4), Buffer: c.paletteBuf},
		},
	})
	if err != nil { panic(err) }

	c.fractalView, c.relFractalView = c.cc.CreateOffscreenTexture(width, height)
	if c.relFractalView == nil {
		panic("CreateOffscreenTexture failed (GPU unavailable)")
	}
}

// draw renders a new frame to the canvas.
func (c *context) draw(dc *gogpu.Context, token *atomic.Pointer[gogpu.AnimationToken]) {
	if c.canvas.Context() == nil { return } // the call to c.release() below calls c.canvas.Close()

	if c.frameCounter > 2745 { // TODO(jbunds): programmatically determine the value of this magic number
		token.Load().Stop()
		c.release()
		fmt.Println("stopped rendering (precision exhausted)")
		return
	}

	// per-frame state updates

	c.zoom *= zoomFactor
	c.frameCounter++

	unis := updateUniforms( // magnification logic
		c.frameCounter,
		width,
		height,
		c.targetXHi,
		c.targetXLo,
		c.targetYHi,
		c.targetYLo,
		c.zoom,
		float64(baseIterations) + float64(c.frameCounter) * growthRate, // GPU region-detection iterations
	)

	err := c.device.Queue().WriteBuffer(c.uniformBuf, 0, unsafe.Slice((*byte)(unsafe.Pointer(&unis)), 48))
	if err != nil { panic(err) }

	transientBindGroup, err := c.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout:  c.bgLayout1,
		Entries: []wgpu.BindGroupEntry{{
			Binding:     0,
			TextureView: (*wgpu.TextureView)(c.fractalView.Pointer()),
		}},
	})
	if err != nil { panic(err) }
	defer transientBindGroup.Release()

	// physical pixels, e.g., 1600x1600 for a 800x800 logical application window on a HiDPI display
	surfaceWidth, surfaceHeight := dc.SurfaceSize()

	// encode & dispatch
	encoder, err := c.device.CreateCommandEncoder(nil)
	if err != nil { panic(err) }

	pass,    err := encoder.BeginComputePass(nil)
	if err != nil { panic(err) }

	pass.SetPipeline(c.pipeline)
	pass.SetBindGroup(0, c.staticBindGroup,  nil)
	pass.SetBindGroup(1, transientBindGroup, nil)
	pass.Dispatch(((surfaceWidth + 15) / 16), ((surfaceHeight + 7) / 8), 1)

	err = pass.End()
	if err != nil { panic(err) }

	cmds, err := encoder.Finish()
	if err != nil { panic(err) }

	c.device.Queue().Submit(cmds)

	err = c.canvas.Draw(func(cc *gg.Context) {
		cc.DrawGPUTextureBase(c.fractalView, 0, 0, width, height)
		cc.SetRGBA(0, 0, 0, 0.15)
		cc.DrawRoundedRectangle(10, height - 40, 336, 30, 4)
		cc.Fill()
		cc.SetColor(gg.Red)
		cc.DrawString(fmt.Sprintf("FPS: %.0f",         c.fps         ),  18, height - 20)
		cc.SetColor(gg.Green)
		cc.DrawString(fmt.Sprintf("magnification: %e", 1 / c.zoom    ),  72, height - 20)
		cc.SetColor(gg.Yellow)
		cc.DrawString(fmt.Sprintf("frames: %d",        c.frameCounter), 258, height - 20)
	})
	if err != nil { panic(err) }

	err = c.canvas.RenderDirect(dc.RenderTarget().SurfaceView(), surfaceWidth, surfaceHeight)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// release deallocates resources (decrements internal reference count or immediately destroys the object's native handle).
func (c *context) release() {
	c.canvas.Close()
	c.relFractalView()
	c.bgLayout0.Release()
	c.bgLayout1.Release()
	c.paletteBuf.Release()
	c.uniformBuf.Release()
	c.staticBindGroup.Release()
}

// updateUniforms updates the per-frame uniforms passed to the GPU shader.
func updateUniforms(
	frameCounter                               int,
	width,     height                          uint32,
	targetXHi, targetXLo, targetYHi, targetYLo float32,
	zoom,      iterations                      float64) uniforms {

	zoomHi, zoomLo := splitFloat64(zoom)

	return uniforms{
		paletteSize:  paletteSize,
		width:        float32(width),
		height:       float32(height),
		iterations:   float32(iterations),

		zoomHi:       zoomHi,
		zoomLo:       zoomLo,
		targetXHi:    targetXHi,
		targetYHi:    targetYHi,

		targetXLo:    targetXLo,
		targetYLo:    targetYLo,
		frameCounter: float32(frameCounter),
	}
}

// initResources initializes all resources consumed by the GPU shader.
func initResources(
	device     *wgpu.Device,
	iterations float64,
) (
	[]uint32,
	*wgpu.Buffer,
	*wgpu.Buffer,
	*wgpu.BindGroupLayout,
	*wgpu.BindGroupLayout,
	*wgpu.ComputePipeline,
) {
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: shaderCode})
	if err != nil { panic(err) }

	paletteColors, paletteBuf := initPaletteBuf(device, iterations)
	uniformBuf                := initUniformBuf(device)
	bgLayout0, bgLayout1      := initBindGroupLayouts(device)
	layout                    := initPipelineLayout(device, bgLayout0, bgLayout1)
	pipeline                  := initPipeline(device, layout, shader)

	return paletteColors, paletteBuf, uniformBuf, bgLayout0, bgLayout1, pipeline
}

// initPalette initializes the pre-computed color palette used by the GPU shader to render colored pixels on the canvas.
func initPalette(iterations float64) []uint32 {
	colors := make([]uint32, paletteSize)
	for i := range paletteSize {
		iterations := float64(i) * (iterations / float64(paletteSize))
		a := uint32(255)
		r := uint32(math.Sin(0.015 * iterations + 1.0) * 127 + 128)
		g := uint32(math.Sin(0.012 * iterations + 2.0) * 127 + 128)
		b := uint32(math.Sin(0.010 * iterations + 4.0) * 127 + 128)
		colors[i] = r | (g << 8) | (b << 16) | (a << 24)
	}
	return colors
}

// initPaletteBuf initializes the pre-computed color palette and corresponding buffer passed to the GPU shader.
func initPaletteBuf(device *wgpu.Device, iterations float64) ([]uint32, *wgpu.Buffer) {
	paletteColors   := initPalette(iterations)
	paletteBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  uint64(len(paletteColors) * 4),
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil { panic(err) }
	return paletteColors, paletteBuf
}

// initUniformBuf initializes the uniform buffer used to pass uniforms to the GPU shader.
func initUniformBuf(device *wgpu.Device) *wgpu.Buffer {
	uniformBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  48,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil { panic(err) }
	return uniformBuf
}

// initBindGroupLayouts initializes the resource bindings for the GPU shader.
func initBindGroupLayouts(device *wgpu.Device) (*wgpu.BindGroupLayout, *wgpu.BindGroupLayout) {
	bgLayout0, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Entries: []wgpu.BindGroupLayoutEntry{
			{    // uniforms
				Binding:    0,
				Visibility: wgpu.ShaderStageCompute,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			}, { // pre-computed 2000-color palette
				Binding:    1,
				Visibility: wgpu.ShaderStageCompute,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
			},
		},
	})
	if err != nil { panic(err) }

	bgLayout1, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Entries: []wgpu.BindGroupLayoutEntry{{ // storage texture (physical screen)
			Binding:        0,
			Visibility:     wgpu.ShaderStageCompute,
			StorageTexture: &gputypes.StorageTextureBindingLayout{
				Format: gputypes.TextureFormatBGRA8Unorm,
				Access: gputypes.StorageTextureAccessWriteOnly,
			},
		}},
	})
	if err != nil { panic(err) }
	return bgLayout0, bgLayout1
}

// initPipelineLayout initializes the GPU shader compute pipeline layout.
func initPipelineLayout(device *wgpu.Device, bgLayout0, bgLayout1 *wgpu.BindGroupLayout) *wgpu.PipelineLayout {
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgLayout0, bgLayout1},
	})
	if err != nil { panic(err) }
	return layout
}

// initPipeline initializes the GPU shader compute pipeline.
func initPipeline(device *wgpu.Device, layout *wgpu.PipelineLayout, shader *wgpu.ShaderModule) *wgpu.ComputePipeline {
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Layout:     layout,
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil { panic(err) }
	return pipeline
}

// splitFloat64 splits a float64 into two float32s.
func splitFloat64(v float64) (float32, float32) {
	high := float32(v)
	low  := float32(v - float64(high))
	return high, low
}
