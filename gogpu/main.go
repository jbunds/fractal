// Package fractal/gogpu iteratively renders and magnifies fractals of the Mandelbrot or filled Julia set.
//
// Tested on a MacBook Air M1.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

//go:embed common.wgsl
var commonShaderCode string

//go:embed mandelbrot.wgsl
var mandelbrotShaderCode string

//go:embed julia.wgsl
var juliaShaderCode string

const (
	mainWidth             = 800   // primary window logical width
	mainHeight            = 800   // primary window logical height
	aboutWidth            = 240   // about window logical width
	aboutHeight           = 164   // about window logical height
	baseIterations        = 500   // initial number of iterations used to compute interior boundaries
	paletteSize           = 2000  // number of colors to pre-compute and pass to the GPU shader for fast lookup
	viewportWidth         = 3.0   // viewport width of the initial frame, i.e., the span of the complex plane rendered to the viewport
	scaleFactor           = 0.993 // multiplicative factor by which each successive rendering is iteratively magnified
	linearGrowthFactor    = 0.3   // multiplicative factor by which boundary calculation iterations increase per each successive frame when rendering Mandelbrot fractals
	quadraticGrowthFactor = 0.003 // multiplicative factor by which boundary calculation iterations increase per the square of the frame count when rendering Julia fractals
	maxPrecisionFrames    = 2745  // empirically-determined limit for the number of frames to render before reaching precision limit
)

// state stores the application state (uniforms, color palette, viewport width, rendered frame count, and FPS).
type state struct {
	frameCount        int      // tracks the number of frames rendered
	paletteColors     []uint32 // pre-computed color palette
	viewportWidth,             // tracks the width of the current frame's view of the complex plane
	fps               float64  // fps imprecisely tracks FPS rendered
	xRealHi, xRealLo,
	yImagHi, yImagLo  float32  // target coordinates of the Mandelbrot set defined by 𝑓(𝑧) = 𝑧² + 𝑐
	cRealHi, cRealLo,
	cImagHi, cImagLo  float32  // complex constant term of the filled Julia set defined by 𝑓(𝑧) = 𝑧² + 𝑐
}

// gpu stores all GPU resources required to render a frame (device, buffers, compute pipeline).
type gpu struct {
	device          *wgpu.Device          // logical GPU device
	paletteTex      *wgpu.Texture         // pre-computed color palette 1D texture
	uniformBuf      *wgpu.Buffer          // uniforms buffer
	staticBindGroup *wgpu.BindGroup       // uniforms and color palette buffer
	bgLayout0       *wgpu.BindGroupLayout // uniforms and color palette layout
	bgLayout1       *wgpu.BindGroupLayout // storage texture layout
	pipeline        *wgpu.ComputePipeline // GPU compute pipeline configuration
}

// assets stores assets used to render frames (canvas, font, texture view).
type assets struct {
	canvas             *ggcanvas.Canvas       // wraps gg.Context
	fontSource         *text.FontSource       // font used to render per-frame stats
	paletteView        *wgpu.TextureView      // handle to the color palette 1D texture view
	fractalView        gpucontext.TextureView // handle to the fractal texture view
	fractalViewRelease func()                 // fractal TextureView release function
}

// renderer stores most runtime state.
type renderer struct {
	maxIter  func(int) float64
	powScale uint32
	state    *state
	gpu      *gpu
	assets   *assets
}

// uniforms stores per-frame uniforms.
type uniforms struct { // total: (2 uint32 + 14 float32) * 4 bytes == 64 bytes
	paletteSize, powScale                   uint32
	frameCount,  maxIter                   float32 // block 1
	width,       height,  scaleHi, scaleLo float32 // block 2
	xRealHi,     yImagHi, xRealLo, yImagLo float32 // block 3
	cRealHi,     cImagHi, cRealLo, cImagLo float32 // block 4
}

func main() {
	var (
		initTokenOnce   sync.Once
		lastFrameTime   time.Time
		animToken       atomic.Pointer[gogpu.AnimationToken]
		currentRenderer atomic.Value
	)

	fractalType, params, err := flags(flag.CommandLine, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot parse flags: %v\n", err)
		os.Exit(1)
	}

	displayName := params.name
	if strings.ContainsAny(string(params.name[0]), "+-0") { // hack to avoid using regexp
		displayName = "unnamed"
	}

	var title string

	shaderCode := commonShaderCode + "\n"

	switch fractalType {
	case "mandelbrot":
		title       = fmt.Sprintf("%s - %s (%v, %vi)",      fractalType, displayName, params.xReal, params.yImag)
		shaderCode += mandelbrotShaderCode
	case "julia":
		title       = fmt.Sprintf("%s - %s (c = %v + %vi)", fractalType, displayName, params.cReal, params.cImag)
		shaderCode += juliaShaderCode
	}

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithAppName("Fractal").
		WithTitle(title).
		WithSize(mainWidth, mainHeight).
		WithResizable(false))

	app.SetQuitOnLastWindowClosed(false)

	currentRenderer.Store(newRenderer(fractalType, params))


	// GoGPU callback registrations and definitions

	app.OnSurfaceAvailable(func() {

		// TODO(jbunds): handle case where use clicks the "close window" icon in the far-left of the primary window's title bar

		// will crash the program
		//primWin := app.PrimaryWindow()
		//primWin.SetOnClose(func() bool {
		//	primWin.Hide()
		//	return false
		//})

		// nasty, heavy-handed workaround: refuse to allow users to close the primary window
		// the primary window can still be hidden via ⌘+w
		app.PrimaryWindow().SetOnClose(func() bool { return false })

		aboutWin, err := app.NewWindow(gogpu.DefaultConfig().
			WithTitle(""). // looks more slick and modern when combined with the transparent title bar triggered via WithHeaderAlignment(gogpu.HeaderAlignLeft) below
			WithSize(aboutWidth, aboutHeight).
			WithTransparent(true).
			WithResizable(false).
			WithHeaderAlignment(gogpu.HeaderAlignLeft)) // renders transparent title bar
		if err != nil {
			panic(err)
		}
		aboutWin.Hide()

		cr := currentRenderer.Load().(*renderer)
		cr.init(app, shaderCode)

		// when appendAboutMenuItem is called before addFractalsMenu, duplicate items are added to the main application menu

		//  TODO(jbunds): fix whatever is removing the native "Window" menu,
		//                which may be triggered by customizing the menus per
		//                the call to app.SetCustomMenu() in addFratcalsMenu() ?
		addFractalsMenu(app, &currentRenderer)

		// TODO(jbunds): replace the native "About ..." application menu item action instead of appending to the menu
		appendAboutMenuItem(app, &currentRenderer, aboutWin)

//		if winMenu := app.GetSystemMenu(gogpu.SystemMenuWindow); winMenu != nil {
//			winMenu.AddItem(gogpu.MenuItem{Separator: true})
//			winMenu.AddItem(gogpu.MenuItem{
//				Title: fmt.Sprintf("mandelbrot - %s", params.name),
//				Role:  gogpu.RoleShowAll,         // no RoleMaximize
//				Role:  gogpu.RoleBringAllToFront, // no RoleMaximize
//			})
//		}
	})

	app.EventSource().OnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		switch {
		case mods.HasSuper():
			switch key {
			case gpucontext.KeyQ: // ⌘+q
				currentRenderer.Load().(*renderer).release()
				gg.CloseAccelerator()
				app.Quit()
			case gpucontext.KeyW: // ⌘+w
				if animToken.Load() != nil {
					animToken.Swap(nil) // reduce GPU load by suspending animation while the primary window is hidden
				}
				app.PrimaryWindow().Hide()
			}
		case key == gpucontext.KeySpace:
			toggleAnimation(app, &animToken)
		}
	})

	app.OnDraw(func(dc *gogpu.Context) {
		r := currentRenderer.Load().(*renderer)
		r.draw(dc, &animToken)

		elapsed := time.Since(lastFrameTime).Milliseconds()
		if elapsed > 0 {
			r.state.fps = float64(1000.0 / elapsed)
		}
		lastFrameTime = time.Now()

		initTokenOnce.Do(func() {
			animToken.Store(app.StartAnimation())
		})

		if animToken.Load() != nil {
			app.RequestRedraw() // renders at VSync frequency (~60 FPS)
		}

	})

	app.OnClose(func() {
		currentRenderer.Load().(*renderer).release()
	})

	lastFrameTime = time.Now()

	// main event loop

	if err := app.Run(); err != nil {
		panic(err)
	}
}

// newRenderer constructs and returns the *renderer used to store runtime state.
func newRenderer(fractalType string, params params) *renderer {
	xRealHi, xRealLo := splitFloat64(params.xReal)
	yImagHi, yImagLo := splitFloat64(params.yImag)
	cRealHi, cRealLo := splitFloat64(params.cReal)
	cImagHi, cImagLo := splitFloat64(params.cImag)
	var powScale uint32
	// TODO(jbunds): fix leaky abstraction
	if params.name == "basilica"      ||
	   params.name == "cantor dust"   ||
	   params.name == "dendrite"      ||
	   params.name == "rabbit"        ||
	   params.name == "siegel disc"   ||
	   params.name == "+0.37 + 0.16i" ||
	   params.name == "+0.40 + 0.10i" ||
	   params.name == "-0.50 - 0.56i" ||
	   params.name == "-1.50 + 0.00i" {
		powScale = 1
	}

	var maxIter func(int) float64

	switch fractalType {
	case "mandelbrot":
		maxIter = func(frameCount int) float64 {
			return float64(baseIterations) + float64(frameCount) * linearGrowthFactor
		}
	case "julia":
		maxIter = func(frameCount int) float64 {
			return float64(baseIterations) + (float64(frameCount * frameCount) * quadraticGrowthFactor)
		}
	}

	return &renderer{
		powScale: powScale,
		maxIter:  maxIter,
		gpu:      new(gpu),
		assets:   new(assets),
		state:    &state{
			frameCount:    0,
			viewportWidth: viewportWidth,
			xRealHi:       xRealHi,
			xRealLo:       xRealLo,
			yImagHi:       yImagHi,
			yImagLo:       yImagLo,
			cRealHi:       cRealHi,
			cRealLo:       cRealLo,
			cImagHi:       cImagHi,
			cImagLo:       cImagLo,
		},
	}
}

// init initializes all resources required to render frames in the main application window.
func (r *renderer) init(app *gogpu.App, shaderCode string) {
	// TODO(jbunds): cache / memoize all runtime-invariant resources,
	//               e.g., color palette and associated downstream gogpu objects
	var err error
	r.assets.fontSource, err = loadFontSource()
	if err != nil {
		panic(err)
	}
	r.gpu.device = app.DeviceProvider().Device()

	paletteColors,
		paletteTex,
		uniformBuf,
		bgLayout0,
		bgLayout1,
		pipeline := initResources(r.gpu.device, shaderCode)

	r.state.paletteColors = paletteColors
	r.gpu.paletteTex      = paletteTex
	r.gpu.uniformBuf      = uniformBuf
	r.gpu.bgLayout0       = bgLayout0
	r.gpu.bgLayout1       = bgLayout1
	r.gpu.pipeline        = pipeline

	r.assets.canvas, err = ggcanvas.New(app.GPUContextProvider(), mainWidth, mainHeight)
	if err != nil {
		panic(err)
	}

	r.assets.canvas.Context().SetFont(r.assets.fontSource.Face(12))

	r.assets.paletteView, err = r.gpu.device.CreateTextureView(r.gpu.paletteTex, &wgpu.TextureViewDescriptor{
		Format:    gputypes.TextureFormatR32Uint,
		Dimension: gputypes.TextureViewDimension1D,
	})
	if err != nil {
		panic(err)
	}

	err = r.gpu.device.Queue().WriteTexture(
		&wgpu.ImageCopyTexture{Texture: paletteTex},
		unsafe.Slice(
			(*byte)(unsafe.Pointer(&r.state.paletteColors[0])),
			len(r.state.paletteColors) * 4),
		&wgpu.ImageDataLayout{BytesPerRow: paletteSize * 4},
		&wgpu.Extent3D{Width: paletteSize, Height: 1, DepthOrArrayLayers: 1})
	if err != nil {
		panic(err)
	}

	r.gpu.staticBindGroup, err = r.gpu.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout:  r.gpu.bgLayout0,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Size: 64, Buffer: r.gpu.uniformBuf},
			{Binding: 1, TextureView: r.assets.paletteView},
		},
	})
	if err != nil {
		panic(err)
	}

	r.assets.fractalView,
	r.assets.fractalViewRelease = r.assets.canvas.Context().CreateOffscreenTexture(mainWidth, mainHeight)

	if r.assets.fractalViewRelease == nil {
		fmt.Fprint(os.Stderr, "GPU unavailable") // TODO(jbunds): implement CPU fallback ?
	}
}

// draw renders a new frame to the canvas.
func (r *renderer) draw(dc *gogpu.Context, token *atomic.Pointer[gogpu.AnimationToken]) {
	if r.assets.canvas.Context() == nil {
		return // the call to r.release() below calls r.assets.canvas.Close()
	}

	if r.state.frameCount > maxPrecisionFrames {
		if t := token.Load(); t != nil {
			t.Stop()
		}
		r.release()
		fmt.Println("stopped rendering (precision exhausted)")
		return
	}

	// per-frame state updates

	r.state.viewportWidth *= scaleFactor
	r.state.frameCount++

	unis := updateUniforms( // magnification logic
		r.state.frameCount,    r.powScale,
		r.state.xRealHi,       r.state.xRealLo,
		r.state.yImagHi,       r.state.yImagLo,
		r.state.cRealHi,       r.state.cRealLo,
		r.state.cImagHi,       r.state.cImagLo,
		r.state.viewportWidth, r.maxIter(r.state.frameCount),
	)

	err := r.gpu.device.Queue().WriteBuffer(
		r.gpu.uniformBuf, 0,
		unsafe.Slice((*byte)(unsafe.Pointer(unis)), 64))
	if err != nil {
		panic(err)
	}

	fractalViewBindGroup := r.fractalViewBindGroup()
	defer fractalViewBindGroup.Release()

	encoder, err := r.gpu.device.CreateCommandEncoder(nil)
	if err != nil {
		panic(err)
	}

	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		panic(err)
	}

	pass.SetPipeline(r.gpu.pipeline)
	pass.SetBindGroup(0, r.gpu.staticBindGroup,  nil)
	pass.SetBindGroup(1, fractalViewBindGroup, nil)

	width, height := dc.SurfaceSize() // https://pkg.go.dev/github.com/gogpu/gogpu#App.ScaleFactor

	pass.Dispatch(((width + 15) / 16), ((height + 7) / 8), 1)

	err = pass.End()
	if err != nil {
		panic(err)
	}

	cmds, err := encoder.Finish()
	if err != nil {
		panic(err)
	}

	_, err = r.gpu.device.Queue().Submit(cmds)
	if err != nil {
		panic(err)
	}

	r.drawStats()

	err = r.assets.canvas.RenderDirect(dc.RenderTarget().SurfaceView(), width, height)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// drawStatus draws a rectangular box in the bottom-left corner of the main window showing some basic runtime stats.
func (r *renderer) drawStats() {
	err := r.assets.canvas.Draw(func(cc *gg.Context) {
		cc.DrawGPUTextureBase(r.assets.fractalView, 0, 0, mainWidth, mainHeight)
		cc.SetRGBA(0, 0, 0, 0.15)
		cc.DrawRoundedRectangle(10, mainHeight - 40, 336, 30, 4)
		cc.Fill()
		cc.SetColor(gg.Cyan)
		cc.DrawString(fmt.Sprintf("FPS: %.0f",             r.state.fps          ),  18, mainHeight - 20)
		cc.SetColor(gg.Green)
		cc.DrawString(fmt.Sprintf("magnification: %e", 1 / r.state.viewportWidth),  72, mainHeight - 20)
		cc.SetColor(gg.Cyan)
		cc.DrawString(fmt.Sprintf("frames: %d",            r.state.frameCount   ), 258, mainHeight - 20)
	})
	if err != nil {
		panic(err)
	}
}

// fractalViewBindGroup creates the BindGroup holding the per-frame TextureView of the rendered fractal.
func (r *renderer) fractalViewBindGroup() *wgpu.BindGroup {
	fractalViewBindGroup, err := r.gpu.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: r.gpu.bgLayout1,
		Entries: []wgpu.BindGroupEntry{{
			Binding:     0,
			TextureView: (*wgpu.TextureView)(r.assets.fractalView.Pointer()),
		}},
	})
	if err != nil {
		panic(err)
	}
	return fractalViewBindGroup
}

// release marks resources for deallocation.
func (r *renderer) release() {

	// TODO(jbunds): experiment with TrackResource:
	//
	//   https://pkg.go.dev/github.com/gogpu/gogpu#readme-resource-management
	//   https://pkg.go.dev/github.com/gogpu/gogpu#App.TrackResource

	if r.assets.fractalViewRelease != nil {
		r.assets.fractalViewRelease()
	}
	r.assets.canvas.Close()
	r.assets.fontSource.Close()
	r.assets.paletteView.Release()
	r.gpu.pipeline.Release()
	r.gpu.bgLayout0.Release()
	r.gpu.bgLayout1.Release()
	r.gpu.paletteTex.Release()
	r.gpu.uniformBuf.Release()
	r.gpu.staticBindGroup.Release()
}

// updateUniforms updates the per-frame uniforms passed to the GPU shader.
func updateUniforms(
	frameCount              int,
	powScale                uint32,
	xRealHi,       xRealLo,
	yImagHi,       yImagLo,
	cRealHi,       cRealLo,
	cImagHi,       cImagLo  float32,
	viewportWidth, maxIter  float64) *uniforms {

	scaleHi, scaleLo := splitFloat64(viewportWidth)

	return &uniforms{
		powScale:    powScale,
		paletteSize: paletteSize,
		scaleHi:     scaleHi,
		scaleLo:     scaleLo,

		width:       float32(mainWidth),
		height:      float32(mainHeight),
		maxIter:     float32(maxIter),
		frameCount:  float32(frameCount),

		xRealHi:     xRealHi,
		xRealLo:     xRealLo,
		yImagHi:     yImagHi,
		yImagLo:     yImagLo,

		cRealHi:     cRealHi,
		cRealLo:     cRealLo,
		cImagHi:     cImagHi,
		cImagLo:     cImagLo,
	}
}

// toggleAnimation toggles between pausing and resuming the animation loop,
// e.g., when the spacebar is pressed, or when the primary window is hidden.
func toggleAnimation(app *gogpu.App, token *atomic.Pointer[gogpu.AnimationToken]) {
	// TODO(jbunds): fix the bug where animation is not actually toggled when the "About Fractal" window has focus
	if oldToken := token.Swap(nil); oldToken != nil {
		oldToken.Stop()
	} else {
		token.Store(app.StartAnimation())
	}
}

// splitFloat64 splits a float64 into two float32s.
func splitFloat64(v float64) (float32, float32) {
	high := float32(v)
	low  := float32(v - float64(high))
	return high, low
}
