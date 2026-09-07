package main

import (
	"os"
	"sync/atomic"
	"testing"

	"github.com/gogpu/gpucontext"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func cmpOpts() cmp.Options {
	return cmp.Options{
		cmp.AllowUnexported(
			ui{},     state{},      fractal{},
			gpu{},    renderer{},   uniforms{},
			assets{}, parameters{}, fakeToken{},
		),
		cmpopts.EquateComparable(
			titleAndRole{},
			atomic.Bool{},
		),
		cmpopts.IgnoreUnexported(
			gpucontext.TextureView{},
		),
		cmpopts.IgnoreFields(parameters{}, "maxIter"),
		cmpopts.IgnoreFields(ui{},
			"app",       "renderer",        "animToken",
			"prog",      "progClose",       "initTokenOnce",
			"animating", "hideAboutWindow", "hidePrimaryWindow",
		),
	}
}

func TestNewProgressBar(t *testing.T) {
	t.Parallel()
	ui := new(ui)

	null, _  := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	stdErr   := os.Stderr
	os.Stderr = null
	t.Cleanup(func() {os.Stderr = stdErr})

	ui.newProgressBar(t.Context())

	if ui.prog == nil || ui.progClose == nil {
		t.Errorf("newProgressBar() did not instantiate a new progress.Progress bar")
	}

	ui.newProgressBar(t.Context()) // eek
	ui.progClose()
}

func TestSyncAnimation(t *testing.T) {
	t.Parallel()
	tests := []struct{
		name          string
		animating     bool
		animToken     animToken
		visible       bool
		wantAnimToken animToken
	}{{
		name:          "animation: playing, token: set, window: visible",
		animating:     true,
		animToken:     &fakeToken{},
		visible:       true,
		wantAnimToken: &fakeToken{},
	}, {
		name:          "animation: playing, token: set, window: hidden",
		animating:     true,
		animToken:     &fakeToken{},
		visible:       false,
	}, {
		name:          "animation: playing, token: nil, window: visible",
		animating:     true,
		visible:       true,
	}, {
		name:          "animation: playing, token: nil, window: hidden",
		animating:     true,
		visible:       false,
	}, {
		name:          "animation: paused, token: set, window: visible",
		animating:     false,
		animToken:     &fakeToken{},
		visible:       true,
	}, {
		name:          "animation: paused, token: set, window: hidden",
		animating:     false,
		animToken:     &fakeToken{},
		visible:       false,
	}, {
		name:          "animation: paused, token: nil, window: visible",
		animating:     false,
		visible:       true,
	}, {
		name:          "animation: paused, token: nil, window: hidden",
		animating:     false,
		visible:       false,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ui    := new(ui)
			ui.app = &fakeApp{
				animating: tt.animating,
				animToken: tt.animToken,
			}
			ui.animating.Store(tt.animating)
			ui.animToken.Store(tokenRef{token: tt.animToken})
			ui.primaryWindow = &fakeWin{visible: tt.visible}

			ui.syncAnimation()

			gotAnimToken := ui.loadToken()
			if diff := cmp.Diff(tt.wantAnimToken, gotAnimToken, cmpOpts()); diff != "" {
				t.Errorf("syncAnimation() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHidePrimaryWin(t *testing.T) {
	t.Parallel()
	ui      := new(ui)
	fakeTok := &fakeToken{}
	ui.app   = &fakeApp{animToken: fakeTok}
	ui.animToken.Store(tokenRef{token: fakeTok})

	ui.hidePrimaryWin()

	if !fakeTok.stopCalled {
		t.Errorf("Stop() was not called")
	}
	if !ui.hidePrimaryWindow.Load() {
		t.Errorf("expected ui.hidePrimaryWindow.Load() to be true, got %t\n", ui.hidePrimaryWindow.Load())
	}
}

func TestHideAboutWin(t *testing.T) {
	t.Parallel()
	ui              := new(ui)
	fakeAboutWin    := &fakeWin{visible: true}
	fakePrimaryWin  := &fakeWin{visible: true}
	ui.primaryWindow = fakePrimaryWin
	ui.aboutWindow   = fakeAboutWin

	ui.hideAboutWin()

	if !fakeAboutWin.hideCalled {
		t.Errorf("aboutWindow.Hide() not called")
	}
	if ui.aboutWindow.Visible() {
		t.Errorf("expected ui.aboutWindow.Visible() to be false, got %t\n", ui.aboutWindow.Visible())
	}
	if ui.aboutWindowHasFocus.Load() {
		t.Errorf("expected ui.aboutWindowHasFocus.Load() to be false, got %t\n", ui.aboutWindowHasFocus.Load())
	}
	if !fakePrimaryWin.showCalled {
		t.Errorf("primaryWindow.Show() not called")
	}
}

func TestScheduleMenuRebuild(t *testing.T) {
	t.Parallel()
	ui              := new(ui)
	fakeApp         := &fakeApp{}
	ui.app           = fakeApp
	ui.primaryWindow = &fakeWin{visible: true}

	ui.scheduleMenuRebuild()

	if !ui.pendingMenuRebuild.Load() {
		t.Errorf("expected ui.pendingMenuRebuild.Load() to be true, got %t\n", ui.pendingMenuRebuild.Load())
	}
	if !fakeApp.reqRedrawCalled {
		t.Errorf("RequestRedraw() not called")
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
