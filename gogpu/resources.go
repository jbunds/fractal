package main

import (
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// colorScheme defines five RGB colors used to generate cosine-smoothed color gradients.
type colorScheme struct {
	c1r, c1g, c1b float64
	c2r, c2g, c2b float64
	c3r, c3g, c3b float64
	c4r, c4g, c4b float64
	c5r, c5g, c5b float64
}

func colorSchemes() map[string]colorScheme {
	return map[string]colorScheme{
		"green": colorScheme{
			c1r:  25.0, c1g:  30.0, c1b:  28.0,
			c2r: 105.0, c2g: 125.0, c2b: 100.0,
			c3r: 215.0, c3g: 135.0, c3b:  30.0,
			c4r: 245.0, c4g: 200.0, c4b:  35.0,
			c5r: 245.0, c5g: 240.0, c5b: 225.0,
		},
		"red": colorScheme{
			c1r:  40.0, c1g:   5.0, c1b:   5.0,
			c2r: 180.0, c2g:  20.0, c2b:  10.0,
			c3r: 255.0, c3g:  80.0, c3b:  10.0,
			c4r: 255.0, c4g: 200.0, c4b:  40.0,
			c5r: 255.0, c5g: 255.0, c5b: 245.0,
		},
	}
}

// initResources initializes all resources consumed by the GPU shader.
func initResources(
	device     *wgpu.Device,
	theme,
	shaderCode string,
) (
	[]uint32,
	*wgpu.Texture,
	*wgpu.Buffer,
	*wgpu.BindGroupLayout,
	*wgpu.BindGroupLayout,
	*wgpu.ComputePipeline,
) {
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: shaderCode})
	if err != nil {
		panic(err)
	}

	paletteColors        := initPalette(theme)
	paletteTex           := initPaletteTex(device)
	uniformBuf           := initUniformBuf(device)
	bgLayout0, bgLayout1 := initBindGroupLayouts(device)
	layout               := initPipelineLayout(device, bgLayout0, bgLayout1)
	pipeline             := initPipeline(device, layout, shader)

	return paletteColors,
	       paletteTex,
	       uniformBuf,
	       bgLayout0,
	       bgLayout1,
	       pipeline
}

// initPalette initializes the pre-computed color palette used by the GPU shader to render colored pixels on the canvas.
func initPalette(theme string) []uint32 {
	colors := make([]uint32, paletteSize)
	cs     := colorSchemes()[theme]

	for i := range colors {
		t    := float64(i) / float64(paletteSize)
		tAdj := 0.5 - 0.5 * math.Cos(t * 2.0 * math.Pi)

		var r, g, b float64

		switch {
		case tAdj < 0.25:
			p := tAdj / 0.25
			r  = cs.c1r + (cs.c2r - cs.c1r) * p
			g  = cs.c1g + (cs.c2g - cs.c1g) * p
			b  = cs.c1b + (cs.c2b - cs.c1b) * p
		case tAdj < 0.50:
			p := (tAdj - 0.25) / 0.25
			r  = cs.c2r + (cs.c3r - cs.c2r) * p
			g  = cs.c2g + (cs.c3g - cs.c2g) * p
			b  = cs.c2b + (cs.c3b - cs.c2b) * p
		case tAdj < 0.75:
			p := (tAdj - 0.50) / 0.25
			r  = cs.c3r + (cs.c4r - cs.c3r) * p
			g  = cs.c3g + (cs.c4g - cs.c3g) * p
			b  = cs.c3b + (cs.c4b - cs.c3b) * p
		default:
			p := (tAdj - 0.75) / 0.25
			r  = cs.c4r + (cs.c5r - cs.c4r) * p
			g  = cs.c4g + (cs.c5g - cs.c4g) * p
			b  = cs.c4b + (cs.c5b - cs.c4b) * p
		}
		colors[i] = uint32(r) | (uint32(g) << 8) | (uint32(b) << 16) | (255 << 24)
	}
	return colors
}

// initPaletteTex initializes the pre-computed color palette and corresponding buffer passed to the GPU shader.
func initPaletteTex(device *wgpu.Device) (*wgpu.Texture) {
	paletteTex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size:          wgpu.Extent3D{Width: paletteSize, Height: 1, DepthOrArrayLayers: 1},
		SampleCount:   1,
		MipLevelCount: 1,
		Dimension:     wgpu.TextureDimension1D,
		Format:        gputypes.TextureFormatR32Uint,
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		panic(err)
	}
	return paletteTex
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
				Texture:    &gputypes.TextureBindingLayout{
					SampleType:    gputypes.TextureSampleTypeUint,
					ViewDimension: gputypes.TextureViewDimension1D,
				},
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
