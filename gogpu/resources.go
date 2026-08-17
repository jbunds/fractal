package main

import (
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// initResources initializes all resources consumed by the GPU shader.
func initResources(
	device     *wgpu.Device,
	shaderCode string,
) (
	[]uint32,
	*wgpu.Buffer,
	*wgpu.Buffer,
	*wgpu.BindGroupLayout,
	*wgpu.BindGroupLayout,
	*wgpu.ComputePipeline,
) {
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: shaderCode})
	if err != nil {
		panic(err)
	}

	paletteColors, paletteBuf := initPaletteBuf(device)
	uniformBuf                := initUniformBuf(device)
	bgLayout0, bgLayout1      := initBindGroupLayouts(device)
	layout                    := initPipelineLayout(device, bgLayout0, bgLayout1)
	pipeline                  := initPipeline(device, layout, shader)

	return paletteColors, paletteBuf, uniformBuf, bgLayout0, bgLayout1, pipeline
}

// initPalette initializes the pre-computed color palette used by the GPU shader to render colored pixels on the canvas.
func initPalette() []uint32 {
	colors        := make([]uint32, paletteSize)
	c1r, c1g, c1b :=  25.0,  30.0,  28.0
	c2r, c2g, c2b := 105.0, 125.0, 100.0
	c3r, c3g, c3b := 215.0, 135.0,  30.0
	c4r, c4g, c4b := 245.0, 200.0,  35.0
	c5r, c5g, c5b := 245.0, 240.0, 225.0
	for i := range colors {
		t    := float64(i) / float64(paletteSize)
		tAdj := 0.5 - 0.5 * math.Cos(t * 2.0 * math.Pi)
		var r, g, b float64
		if tAdj < 0.25 {
			p := tAdj / 0.25
			r  = c1r + (c2r - c1r) * p
			g  = c1g + (c2g - c1g) * p
			b  = c1b + (c2b - c1b) * p
		} else if tAdj < 0.50 {
			p := (tAdj - 0.25) / 0.25
			r  = c2r + (c3r - c2r) * p
			g  = c2g + (c3g - c2g) * p
			b  = c2b + (c3b - c2b) * p
		} else if tAdj < 0.75 {
			p := (tAdj - 0.50) / 0.25
			r  = c3r + (c4r - c3r) * p
			g  = c3g + (c4g - c3g) * p
			b  = c3b + (c4b - c3b) * p
		} else {
			p := (tAdj - 0.75) / 0.25
			r  = c4r + (c5r - c4r) * p
			g  = c4g + (c5g - c4g) * p
			b  = c4b + (c5b - c4b) * p
		}
		colors[i] = uint32(r) | (uint32(g) << 8) | (uint32(b) << 16) | (255 << 24)
	}
	return colors
}

// initPaletteBuf initializes the pre-computed color palette and corresponding buffer passed to the GPU shader.
func initPaletteBuf(device *wgpu.Device) ([]uint32, *wgpu.Buffer) {
	paletteColors   := initPalette()
	paletteBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  uint64(len(paletteColors) * 4),
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		panic(err)
	}
	return paletteColors, paletteBuf
}

// initUniformBuf initializes the uniform buffer used to pass uniforms to the GPU shader.
func initUniformBuf(device *wgpu.Device) *wgpu.Buffer {
	uniformBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  64,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		panic(err)
	}
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
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage},
			},
		},
	})
	if err != nil {
		panic(err)
	}

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
	if err != nil {
		panic(err)
	}
	return bgLayout0, bgLayout1
}

// initPipelineLayout initializes the GPU shader compute pipeline layout.
func initPipelineLayout(device *wgpu.Device, bgLayout0, bgLayout1 *wgpu.BindGroupLayout) *wgpu.PipelineLayout {
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgLayout0, bgLayout1},
	})
	if err != nil {
		panic(err)
	}
	return layout
}

// initPipeline initializes the GPU shader compute pipeline.
func initPipeline(device *wgpu.Device, layout *wgpu.PipelineLayout, shader *wgpu.ShaderModule) *wgpu.ComputePipeline {
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Layout:     layout,
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil {
		panic(err)
	}
	return pipeline
}
