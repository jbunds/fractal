// package mandelbrot/gogpu renders the Mandelbrot set on my MacBook Air M1
package main

// GOGPU_GRAPHICS_API=metal go run .

import (
	_ "embed"
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
	width       =  800   // application window width
	height      =  800   // application window height
	maxIter     =  500   // increase to reveal finer filament detail to the detriment of performance / frame-rendering latency
	initialZoom =  3.5   // initial magnification factor of the rendered Mandelbrot set
	zoomFactor  =  0.993 // multiplicative factor by which the rendering is iteratively magnified

	// seahorse valley
	targetX     = -0.743643887037158704752191506114774
	targetY     =  0.131825904205311970493132056385139

	// elephant valley
	//targetX     = 0.275
	//targetY     = 0.0

	// triple spiral valley
	//targetX     = -0.088
	//targetY     =  0.654

	// scepter valley
	//targetX     = -1.45
	//targetY     =  0.0
)

type uniforms struct { // total: 12 float32 fields * 4 bytes == 48 bytes
	width,     height,    maxIter,   subStep   float32 // block 1
	zoomHi,    zoomLo,    targetXHi, targetYHi float32 // block 2
	targetXLo, targetYLo, pad0,      pad1      float32 // block 3
}

func main() {
	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("mandelbrot").
		WithSize(width, height).
		WithContinuousRender(true))

	var (
		setupOnce  sync.Once
		device     *wgpu.Device
		uniformBuf *wgpu.Buffer
		bgLayout   *wgpu.BindGroupLayout
		pipeline   *wgpu.ComputePipeline
	)

	frameCounter := 0
	newZoom      := initialZoom

	app.OnDraw(func(dc *gogpu.Context) {

		setupOnce.Do(func() {
			device                         = app.DeviceProvider().Device()
			uniformBuf, bgLayout, pipeline = setup(device)
		})

		canvas, err := ggcanvas.New(app.GPUContextProvider(), width, height) // instantiate gg canvas for UI overlay
		if err != nil { panic(err) }

		//if err := canvas.Context().LoadFontFace("/System/Library/Fonts/Supplemental/Verdana.ttf", 16); err != nil { panic(err) }

		// per-frame state updates

		frameCounter  += 1
		newZoom       *= zoomFactor
    subStep       := newZoom / float64(width) * 0.25

		surfaceView   := dc.SurfaceView()
		surfaceWidth,
		surfaceHeight := dc.SurfaceSize()

		// update uniforms (zoom logic)

		unis := updateFrameState(surfaceWidth, surfaceHeight, newZoom, subStep)

		bindGroup, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{ // recreated for every frame since surfaceView changes every frame
			Layout:  bgLayout,
			Entries: []wgpu.BindGroupEntry{
				{Binding: 0, Size: 48, Buffer: uniformBuf},
				{Binding: 1, TextureView: surfaceView},
			},
		})
		if err != nil { panic(err) }

		// encode & dispatch
		encoder, err := device.CreateCommandEncoder(nil); if err != nil { panic(err) }
		pass,    err := encoder.BeginComputePass(nil);    if err != nil { panic(err) }
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, bindGroup, nil)
		pass.Dispatch(uint32((surfaceWidth + 15) / 16), uint32((surfaceHeight + 15) / 16), 1)
		pass.End()

		cmds, err := encoder.Finish(); if err != nil { panic(err) }

		device.Queue().WriteBuffer(uniformBuf, 0, unsafe.Slice((*byte)(unsafe.Pointer(&unis)), 48))
		device.Queue().Submit(cmds)

		canvas.RenderDirect(dc.RenderTarget().SurfaceView(), width, height)

		bindGroup.Release()
	})

	app.OnClose(func() {})

	if err := app.Run(); err != nil { panic(err) }
}   

func setup(device *wgpu.Device) (*wgpu.Buffer, *wgpu.BindGroupLayout, *wgpu.ComputePipeline) {
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: shaderCode})
	if err != nil { panic(err) }

	uniformBuf := initUniformBuf(device)
	bgLayout   := initBindGroupLayout(device)
	layout     := initPipelineLayout(device, bgLayout)
	pipeline   := initPipeline(device, layout, shader)

	return uniformBuf, bgLayout, pipeline
}

func initUniformBuf(device *wgpu.Device) *wgpu.Buffer {
	uniformBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  48,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil { panic(err) }
	return uniformBuf
}

func initBindGroupLayout(device *wgpu.Device) *wgpu.BindGroupLayout {
	bgLayout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Entries: []wgpu.BindGroupLayoutEntry{
			{    // uniform
				Binding:    0,
				Visibility: wgpu.ShaderStageCompute,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			}, { // storage texture (physical screen)
				Binding:        1,
				Visibility:     wgpu.ShaderStageCompute,
				StorageTexture: &gputypes.StorageTextureBindingLayout{
					Format: gputypes.TextureFormatBGRA8Unorm, // must match surface format
					Access: gputypes.StorageTextureAccessWriteOnly,
				},
			},
		},
	})
	if err != nil { panic(err) }
	return bgLayout
}

func initPipelineLayout(device *wgpu.Device, bgLayout *wgpu.BindGroupLayout) *wgpu.PipelineLayout {
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgLayout},
	})
	if err != nil { panic(err) }
	return layout
}

func initPipeline(device *wgpu.Device, layout *wgpu.PipelineLayout, shader *wgpu.ShaderModule) *wgpu.ComputePipeline {
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Layout:     layout,
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil { panic(err) }
	return pipeline
}

func updateFrameState(width, height uint32, zoom, subStep float64) uniforms {
	zHi,  zLo  := splitFloat64(zoom)
	txHi, txLo := splitFloat64(targetX) // TODO(jeff): calculate txHi and txLo just once since targetX is a constant
	tyHi, tyLo := splitFloat64(targetY) // TODO(jeff): calculate tyHi and tyLo just once since targetY is a constant

	return uniforms{
		width:     float32(width),
		height:    float32(height),
		maxIter:   float32(maxIter),
		subStep:   float32(subStep),
		zoomHi:    zHi,
		zoomLo:    zLo,
		targetXHi: txHi,
		targetXLo: txLo,
		targetYHi: tyHi,
		targetYLo: tyLo,
	}
}

func splitFloat64(v float64) (float32, float32) {
	high := float32(v)
	low  := float32(v - float64(high))
	return high, low
}
