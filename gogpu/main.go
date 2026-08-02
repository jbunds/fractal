// Package mandelbrot/gogpu renders the Mandelbrot set and zooms in on preset coordinates.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"math"
	"os"
	"sync"
	"unsafe"

	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

//go:embed mandelbrot.wgsl
var shaderCode string

const (
	width       = 800   // application window width
	height      = 800   // application window height
	baseIter    = 500   // initial number of iterations used to compute interior boundaries
	paletteSize = 2000  // number of colors to pre-compute and pass to the GPU shader for fast lookup
	initialZoom = 3.5   // initial magnification factor of the rendered image
	zoomFactor  = 0.993 // multiplicative factor by which the rendering is iteratively magnified
	growthRate  = 0.2   // multiplicative factor by which boundary calculation iterations increases per each successive magnification
)

// the order in which the fields are declared must match the order of the fields
type uniforms struct { // total: 4 bytes * (1 uint32 field + 11 float32 fields) == 48 bytes
	paletteSize                                    uint32
	frameCounter, iterations, pad                  float32 // block 1
	width,        height,     zoomHi,    zoomLo    float32 // block 2
	targetXHi,    targetYHi,  targetXLo, targetYLo float32 // block 3
}

func main() {

	coords, err  := flags(flag.CommandLine, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot parse flags: %v\n", err)
		os.Exit(1)
	}
	targetX := coords.x
	targetY := coords.y

	app     := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle(fmt.Sprintf("mandelbrot - %s", coords.name)).
		WithSize(width, height).
		WithContinuousRender(true))

	var (
		setupOnce sync.Once

		paletteData []uint32

		// singletons required by OnDraw()
		paletteBuf      *wgpu.Buffer
		device          *wgpu.Device
		uniformBuf      *wgpu.Buffer
		bgLayout0       *wgpu.BindGroupLayout
		bgLayout1       *wgpu.BindGroupLayout
		pipeline        *wgpu.ComputePipeline
		staticBindGroup *wgpu.BindGroup
		canvas          *ggcanvas.Canvas
	)

	frameCounter  := 1
	stopRendering := false // TODO(jbunds): clean this up: investigate using, e.g., the token-based StartAnimation approach instead of calling WithContinuousRender
	newZoom       := initialZoom
	targetXHi,
	targetXLo     := splitFloat64(targetX)
	targetYHi,
	targetYLo     := splitFloat64(targetY)

	app.OnDraw(func(dc *gogpu.Context) {

		if stopRendering { return } // reduce GPU load without exiting the application

		iterations := float64(baseIter) + float64(frameCounter) * growthRate

		setupOnce.Do(func() {
			device = app.DeviceProvider().Device()

			paletteData,
			paletteBuf,
			uniformBuf,
			bgLayout0,
			bgLayout1,
			pipeline = setup(device, iterations)

			var err error
			canvas, err = ggcanvas.New(app.GPUContextProvider(), width, height)
			if err != nil { panic(err) }

			// TODO(jbunds): render some metadata (e.g., magnification factor, FPS) on the canvas as the program zooms in on the target coordinates.
			//if err := canvas.Context().LoadFontFace("/System/Library/Fonts/Supplemental/Verdana.ttf", 16); err != nil { panic(err) }

			device.Queue().WriteBuffer(paletteBuf, 0, unsafe.Slice((*byte)(unsafe.Pointer(&paletteData[0])), len(paletteData) * 4))

			staticBindGroup, err = device.CreateBindGroup(&wgpu.BindGroupDescriptor{
				Layout:  bgLayout0,
				Entries: []wgpu.BindGroupEntry{
					{Binding: 0, Size: 48,                      Buffer: uniformBuf},
					{Binding: 1, Size: uint64(paletteSize * 4), Buffer: paletteBuf},
				},
			})
			if err != nil { panic(err) }
		})

		if frameCounter++; frameCounter > 2745 { // TODO(jbunds): programmatically determine the value of this magic number via other parameters
			stopRendering = true
			if staticBindGroup != nil { staticBindGroup.Release() }
			fmt.Println("stopped rendering")
			return
		}

		// per-frame state updates

		newZoom *= zoomFactor

		// update uniforms (zoom logic)

		surfaceWidth,
		surfaceHeight := dc.SurfaceSize()
		unis          := updateUniforms(
			frameCounter,
			surfaceWidth,
			surfaceHeight,
			targetXHi,
			targetXLo,
			targetYHi,
			targetYLo,
			newZoom,
			iterations,
		)

		device.Queue().WriteBuffer(uniformBuf, 0, unsafe.Slice((*byte)(unsafe.Pointer(&unis)), 48))

		transientBindGroup, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Layout:  bgLayout1,
			Entries: []wgpu.BindGroupEntry{
				{Binding: 0, TextureView: dc.SurfaceView()},
			},
		})
		if err != nil { panic(err) }

		// encode & dispatch
		encoder, err := device.CreateCommandEncoder(nil); if err != nil { panic(err) }
		pass,    err := encoder.BeginComputePass(nil);    if err != nil { panic(err) }
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, staticBindGroup,    nil)
		pass.SetBindGroup(1, transientBindGroup, nil)
		pass.Dispatch(uint32((surfaceWidth + 15) / 16), uint32((surfaceHeight + 7) / 8), 1)
		pass.End()

		cmds, err := encoder.Finish(); if err != nil { panic(err) }

		device.Queue().Submit(cmds)

		canvas.RenderDirect(dc.RenderTarget().SurfaceView(), surfaceWidth, surfaceHeight)

		transientBindGroup.Release()
	})

	app.OnClose(func() {
		if staticBindGroup != nil { staticBindGroup.Release() }
	})

	if err := app.Run(); err != nil { panic(err) }
}

// setup initializes all of the resources consumed by the GPU shader.
func setup(
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

	paletteData, paletteBuf := initPaletteBuf(device, iterations)
	uniformBuf              := initUniformBuf(device)
	bgLayout0, bgLayout1    := initBindGroupLayouts(device)
	layout                  := initPipelineLayout(device, bgLayout0, bgLayout1)
	pipeline                := initPipeline(device, layout, shader)

	return paletteData, paletteBuf, uniformBuf, bgLayout0, bgLayout1, pipeline
}

// initPalette initializes the pre-computed color palette used by the GPU shader to render colored pixels on the canvas.
func initPalette(iterations float64) []uint32 {
	data := make([]uint32, paletteSize)
	for i := range paletteSize {
		iterations := float64(i) * (iterations / float64(paletteSize))
		a := uint32(255)
		r := uint32(math.Sin(0.015 * iterations + 1.0) * 127 + 128)
		g := uint32(math.Sin(0.012 * iterations + 2.0) * 127 + 128)
		b := uint32(math.Sin(0.010 * iterations + 4.0) * 127 + 128)
		data[i] = r | (g << 8) | (b << 16) | (a << 24)
	}
	return data
}

// initPaletteBuf initializes the pre-computed color palette and corresponding buffer passed to the GPU shader.
func initPaletteBuf(device *wgpu.Device, iterations float64) ([]uint32, *wgpu.Buffer) {
	paletteData     := initPalette(iterations)
	paletteBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  uint64(len(paletteData) * 4),
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil { panic(err) }
	return paletteData, paletteBuf
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
		},
	}})
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

// updateUniforms updates the uniforms passed to the GPU shader.
func updateUniforms(
	frameCounter                               int,
	width, height                              uint32,
	targetXHi, targetXLo, targetYHi, targetYLo float32,
	zoom, iterations                           float64) uniforms {

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

// splitFloat64 splits a float64 into two float32s.
func splitFloat64(v float64) (float32, float32) {
	high := float32(v)
	low  := float32(v - float64(high))
	return high, low
}
