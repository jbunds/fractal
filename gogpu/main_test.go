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
			renderer{},
			fractal{},
			parameters{},
			state{},
			gpu{},
			assets{},
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
		fractal: &fractal{params: &parameters{}},
		theme:  "foo",
		state:  &state{viewportWidth: 3},
		gpu:    &gpu{},
		assets: &assets{}}
	got := newRenderer(&fractal{params: &parameters{}}, "foo")
	if diff := cmp.Diff(want, got, cmpOpts()); diff != "" {
		t.Errorf("newRenderer() mismatch (-want +got):\n%s", diff)
	}
}
