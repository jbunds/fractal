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
			ui{},    renderer{}, fractal{}, parameters{},
			state{}, gpu{},      assets{},  uniforms{},
		),
		cmpopts.IgnoreUnexported(
			gpucontext.TextureView{},
		),
		cmpopts.IgnoreFields(parameters{}, "maxIter"),
		cmpopts.IgnoreFields(ui{},
			"app",               "renderer",            "animToken",
			"prog",              "progClose",           "initTokenOnce",
			"aboutWindowIsOpen", "aboutWindowHasFocus", "resumeAnimWhenShown",
			"hidePrimaryWindow", "hideAboutWindow",
		),
	}
}

func TestNewUI(t *testing.T) {
	t.Parallel()
	want := new(ui)
	got  := newUI(t.Context())
	if diff := cmp.Diff(want, got, cmpOpts()); diff != "" {
		t.Errorf("newUI mismatch (-want +got):\n%s", diff)
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
	got := updateUniforms(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12)
	if diff := cmp.Diff(want, got, cmpOpts()); diff != "" {
		t.Errorf("updateUniforms() mismatch (-want +got):\n%s", diff)
	}
}

func TestRemoveComments(t *testing.T) {
	t.Parallel()
	code := `// a comment

1 + 1 == 2

// another comment

2 + 2 == 4

  // yet another comment
`
	want := "1 + 1 == 2\n\n2 + 2 == 4"
	got  := removeComments(code)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("removeComments() mismatch (-want +got):\n%s", diff)
	}
}
