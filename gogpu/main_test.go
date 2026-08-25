package main

import (
	"testing"

	"github.com/gogpu/gpucontext"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func cmpOpts() cmp.Options {
	return cmp.Options{
		cmp.AllowUnexported(
			assets{},     fractal{},  gpu{},
			parameters{}, renderer{}, state{}, uniforms{},
		),
		cmpopts.IgnoreUnexported(
			gpucontext.TextureView{},
		),
		cmp.FilterPath(func(p cmp.Path) bool { // TODO(jbunds): clean this up
			f, ok := p[len(p) - 1].(cmp.StructField)
			return ok && f.Name() == "maxIter"
		}, cmp.Ignore()),
	}
}

func TestNewRenderer(t *testing.T) {
	t.Parallel()
	want := &renderer{
		theme:  "bar",
		fractal: &fractal{params: &parameters{}},
		gpu:     &gpu{shaderCode: "foo"},
		state:   &state{viewportWidth: 3},
		assets:  &assets{},
	}
	got := newRenderer(&fractal{params: &parameters{}}, "foo", "bar")
	if diff := cmp.Diff(want, got, cmpOpts()); diff != "" {
		t.Errorf("newRenderer() mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateUniforms(t *testing.T) {
	t.Parallel()
	want := &uniforms{
		paletteSize: 2000,
		width:        800, height:   800,
		frameCount:     1, powScale:   2,
		xRealHi:        3, xRealLo:    4,
		yImagHi:        5, yImagLo:    6,
		cRealHi:        7, cRealLo:    8,
		cImagHi:        9, cImagLo:   10,
		scaleHi:       11, maxIter:   12,
	}
	got  := updateUniforms(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12)
	if diff := cmp.Diff(want, got, cmpOpts()); diff != "" {
		t.Errorf("updateUniforms() mismatch (-want +got):\n%s", diff)
	}
}
