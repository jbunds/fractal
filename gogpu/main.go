// Package mandelbrot/gogpu renders the Mandelbrot set and zooms in on preset coordinates.
//
// Tested on a MacBook Air M1.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // enable GPU-bound rendering and rasterized tiles
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/wgpu"
)

//go:embed mandelbrot.wgsl
var shaderCode string

const (
	mainWidth          = 800   // primary window logical width
	mainHeight         = 800   // primary window logical height
	aboutWidth         = 240   // about window logical width
	aboutHeight        = 164   // about window logical height
	baseIterations     = 500   // initial number of iterations used to compute interior boundaries
	paletteSize        = 2000  // number of colors to pre-compute and pass to the GPU shader for fast lookup
	initialZoom        = 3.0   // initial magnification factor of the rendered image
	zoomFactor         = 0.993 // multiplicative factor by which the rendering is iteratively magnified
	growthRate         = 0.2   // multiplicative factor by which boundary calculation iterations increases per each successive magnification
	maxPrecisionFrames = 2745  // empirically-determined limit for the number of frames to render before reaching precision limit
)

// state stores the application state (uniforms, color palette, and FPS stats).
type state struct {
	frameCount            int     // tracks the number of frames rendered
	zoom, fps             float64 // zoom tracks the magnification factor of the current frame; fps imprecisely tracks FPS rendered
	targetXHi, targetYHi,
	targetXLo, targetYLo  float32 // double-precision target coordinates in the complex plane
	paletteColors        []uint32 // pre-computed color palette

}

// gpu stores all GPU resources required to render a frame (device, buffers, compute pipeline).
type gpu struct {
	device          *wgpu.Device          // logical GPU device
	paletteBuf      *wgpu.Buffer          // pre-computed color palette buffer
	uniformBuf      *wgpu.Buffer          // uniforms buffer
	staticBindGroup *wgpu.BindGroup       // uniforms and color palette buffer
	bgLayout0       *wgpu.BindGroupLayout // uniforms and color palette layout
	bgLayout1       *wgpu.BindGroupLayout // storage texture layout
	pipeline        *wgpu.ComputePipeline // GPU compute pipeline configuration
}

// assets stores assets used to render frames (canvas, font, texture).
type assets struct {
	canvas             *ggcanvas.Canvas       // wraps gg.Context
	fontSource         *text.FontSource       // font used to render per-frame stats
	fractalView        gpucontext.TextureView // handle to the fractal texture view
	fractalViewRelease func()                 // fractal TextureView release function
}

// renderer stores most runtime state.
type renderer struct {
	state  *state
	gpu    *gpu
	assets *assets
}

// uniforms stores per-frame uniforms.
type uniforms struct { // total: (1 uint32 field + 11 float32 fields) * 4 bytes == 48 bytes
	paletteSize                                   uint32
	frameCount, iterations, pad                  float32 // block 1
	width,      height,     zoomHi,    zoomLo    float32 // block 2
	targetXHi,  targetYHi,  targetXLo, targetYLo float32 // block 3
}

func main() {
	var (
		initTokenOnce   sync.Once
		lastFrameTime   time.Time
		animToken       atomic.Pointer[gogpu.AnimationToken]
		currentRenderer atomic.Value
	)

	coords, err := flags(flag.CommandLine, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot parse flags: %v\n", err)
		os.Exit(1)
	}

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithAppName("Mandelbrot").
		WithTitle(fmt.Sprintf("mandelbrot - %s", coords.name)).
		WithSize(mainWidth, mainHeight))

	currentRenderer.Store(newRenderer(coords))

	// GoGPU callback registrations and definitions

	app.OnSurfaceAvailable(func() {
		cr := currentRenderer.Load().(*renderer)
		cr.init(app)
		//  TODO(jbunds): fix whatever is removing the native "Window" menu,
		//                which appears to be triggered by customizing the menus per the call to app.SetCustomMenu() in addPointsMenu()
		cr.addPointsMenu(app, &currentRenderer, &animToken, coords.name)

		// TODO(jbunds): replace the native "About ..." application menu item action instead of appending to the menu
		cr.appendAboutMenuItem(app, &currentRenderer)
	})

	app.EventSource().OnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		if key == gpucontext.KeySpace {
			toggleAnimation(app, &animToken)
		}
		if key == gpucontext.KeyQ && mods.HasSuper() { // ⌘+q
			currentRenderer.Load().(*renderer).release()
			app.Quit()
		}
		if key == gpucontext.KeyW && mods.HasSuper() { // ⌘+w
			if animToken.Load() != nil {
				animToken.Swap(nil) // reduce GPU load by suspending animation while the primary window is hidden
			}
			app.PrimaryWindow().Hide()
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
func newRenderer(coords coords) *renderer {
	targetXHi, targetXLo := splitFloat64(coords.x)
	targetYHi, targetYLo := splitFloat64(coords.y)
	return &renderer{
		state: &state{
			frameCount: 0,
			zoom:       initialZoom,
			targetXHi:  targetXHi,
			targetXLo:  targetXLo,
			targetYHi:  targetYHi,
			targetYLo:  targetYLo,
		},
		gpu:    &gpu{},
		assets: &assets{},
	}
}

// init initializes all resources required to render frames in the main application window.
func (r *renderer) init(app *gogpu.App) {
	var err error
	r.assets.fontSource, err = loadFontSource()
	if err != nil {
		panic(err)
	}
	r.gpu.device = app.DeviceProvider().Device()

	paletteColors,
		paletteBuf,
		uniformBuf,
		bgLayout0,
		bgLayout1,
		pipeline := initResources(r.gpu.device, baseIterations)

	r.state.paletteColors = paletteColors
	r.gpu.paletteBuf      = paletteBuf
	r.gpu.uniformBuf      = uniformBuf
	r.gpu.bgLayout0       = bgLayout0
	r.gpu.bgLayout1       = bgLayout1
	r.gpu.pipeline        = pipeline

	r.assets.canvas, err = ggcanvas.New(app.GPUContextProvider(), mainWidth, mainHeight)
	if err != nil {
		panic(err)
	}

	r.assets.canvas.Context().SetFont(r.assets.fontSource.Face(12))

	err = r.gpu.device.Queue().WriteBuffer(r.gpu.paletteBuf, 0, unsafe.Slice((*byte)(unsafe.Pointer(&r.state.paletteColors[0])), len(r.state.paletteColors) * 4))
	if err != nil {
		panic(err)
	}

	r.gpu.staticBindGroup, err = r.gpu.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout:  r.gpu.bgLayout0,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Size: 48,                      Buffer: r.gpu.uniformBuf},
			{Binding: 1, Size: uint64(paletteSize * 4), Buffer: r.gpu.paletteBuf},
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

	r.state.zoom *= zoomFactor
	r.state.frameCount++

	unis := updateUniforms( // magnification logic
		r.state.frameCount,
		mainWidth, mainHeight,
		r.state.targetXHi, r.state.targetXLo,
		r.state.targetYHi, r.state.targetYLo,
		r.state.zoom,
		float64(baseIterations) + float64(r.state.frameCount) * growthRate, // GPU fractal region-detection iterations
	)

	err := r.gpu.device.Queue().WriteBuffer(r.gpu.uniformBuf, 0, unsafe.Slice((*byte)(unsafe.Pointer(&unis)), 48))
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

	r.gpu.device.Queue().Submit(cmds)

	r.drawStats()

	err = r.assets.canvas.RenderDirect(dc.RenderTarget().SurfaceView(), width, height)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// fractalViewBindGroup creates bind group holding the per-frame texture view of the rendered fractal.
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

// drawStatus draws a rectangular box in the bottom-left corner of the main window showing some basic runtime stats.
func (r *renderer) drawStats() {
	err := r.assets.canvas.Draw(func(cc *gg.Context) {
		cc.DrawGPUTextureBase(r.assets.fractalView, 0, 0, mainWidth, mainHeight)
		cc.SetRGBA(0, 0, 0, 0.15)
		cc.DrawRoundedRectangle(10, mainHeight - 40, 336, 30, 4)
		cc.Fill()
		cc.SetColor(gg.Cyan)
		cc.DrawString(fmt.Sprintf("FPS: %.0f",             r.state.fps       ),  18, mainHeight - 20)
		cc.SetColor(gg.Green)
		cc.DrawString(fmt.Sprintf("magnification: %e", 1 / r.state.zoom      ),  72, mainHeight - 20)
		cc.SetColor(gg.Cyan)
		cc.DrawString(fmt.Sprintf("frames: %d",            r.state.frameCount), 258, mainHeight - 20)
	})
	if err != nil {
		panic(err)
	}
}

// addPointsMenu creates a "Points" menu to allow users to select a new points of interest from a preset list of named target coordinates.
func (r *renderer) addPointsMenu(app *gogpu.App, cr *atomic.Value, token *atomic.Pointer[gogpu.AnimationToken], item string) {
	points     := pointsOfInterest()
	pointsMenu := gogpu.NewMenuWithTitle("Points")

	for _, v := range slices.Sorted(maps.Keys(points)) {
		pointsMenu.AddItem(gogpu.MenuItem{Title: v, Disabled: v == item, Action: func() {
			newRenderer := newRenderer(points[v])
			oldRenderer := cr.Swap(newRenderer).(*renderer) // replaces currentRenderer in main() scope to reset the render cycle with new target coordinates
			oldRenderer.release()
			newRenderer.init(app)
			app.SetTitle(fmt.Sprintf("mandelbrot - %s", v))
			app.PrimaryWindow().Show()
			if token.Load() == nil {
				token.Swap(app.StartAnimation()) // resume animation if paused, e.g., when the primary window was hidden by the user pressing ⌘+w
			}
			app.RequestRedraw()
		}})
	}

	// TODO(jbunds): determine what causes the animation to pause when the user clicks on any menu header
	// TODO(jbunds): determine what causes the native "Window" menu to be removed from the menu bar
	app.SetCustomMenu("points", pointsMenu)
}

// appendAboutMenuItem appends an "About Mandelbrot" menu item to the application
// menu which renders a small window with some text when selected.
func (r *renderer) appendAboutMenuItem(app *gogpu.App, cr *atomic.Value) {
	aboutWin, err := app.NewWindow(gogpu.DefaultConfig().
		WithTitle("About Mandelbrot").
		WithSize(aboutWidth, aboutHeight).
		WithTransparent(true).
		WithResizable(false))
	if err != nil {
		panic(err)
	}

	aboutWin.Hide()

	appMenu := app.GetSystemMenu(gogpu.SystemMenuApplication)
	appMenu.AddItem(gogpu.MenuItem{Separator: true})
	appMenu.AddItem(gogpu.MenuItem{
		Title:  " \u24D8  About Mandelbrot", // or " ⓘ   About Mandelbrot", but not "\u2139  About Mandelbrot"
		//Role:   gogpu.RoleAbout, // causes my custom window to be effectively discarded
		Action: func() { aboutWin.Show() },
	})

	canvas, err := ggcanvas.New(app.GPUContextProvider(), aboutWidth, aboutHeight)
	if err != nil {
		panic(err)
	}

	var (
		view     *gpucontext.TextureView
		release  func()
		rendered bool
	)

	// assumes the bimedians (perpendicular bisectors) of both windows (primary and About) are aligned
	offsetX := mainWidth  / 2.0 - aboutWidth  / 2.0
	offsetY := mainHeight / 2.0 - aboutHeight / 2.0

	aboutWin.SetOnDraw(func(dc *gogpu.Context) {
		ar  := cr.Load().(*renderer) // in case the user selects one of the menu items from the "Points" menu before selecting the About menu item
		err := canvas.Draw(func(cc *gg.Context) {
			if !rendered {
				view, release = ar.renderAboutWindow(cc)
			}
			cc.MarkFrameRendered()
			rendered = true

			// composite the About window TextureView atop the background

			cc.Push() // save pre-translation state
			cc.Translate(-offsetX, -offsetY) // align the background of the About window with the fractal image beneath
			cc.DrawGPUTextureBase(ar.assets.fractalView, 0, 0, aboutWidth, aboutHeight)
			cc.Pop() // restore pre-translation state so the overlay and text remain unaffected by the translation
			cc.SetRGBA(0, 0, 0, 0.6)
			cc.DrawRectangle(0, 0, aboutWidth, aboutHeight)
			cc.Fill()
			cc.DrawGPUTexture(*view, 0, 0, aboutWidth, aboutHeight)
		})
		if err != nil {
			panic(err)
		}
		if err := canvas.Render(dc.RenderTarget()); err != nil { // or canvas.RenderTo(dc.AsTextureDrawer())
			panic(err)
		}
		app.RequestRedraw()
	})

	// https://pkg.go.dev/github.com/gogpu/gogpu#readme-multi-window-input
	// despite what the godoc claims, when the "About Mandelbrot" window is focused and ⌘+w is pressed, the primary window closes
	aboutWin.SetOnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		// checking aboutWin.Visible() doesn't help here, ⌘+w still closes the primary window even when the About window has focus
		if key == gpucontext.KeyW && mods.HasSuper() { // ⌘+w
			aboutWin.Close() // aboutWin.Hide() ?
		}
	})

	aboutWin.SetOnClose(func() bool {
		app.PrimaryWindow().Show() // i don't know why the primary window loses focus, but it does...
		if release != nil {
			release()
		}
		return true
	})
}

// renderAboutWindow renders the About window content to an off-screen texture.
func (r *renderer) renderAboutWindow(cc *gg.Context) (*gpucontext.TextureView, func()) {
	view, release := cc.CreateOffscreenTexture(aboutWidth, aboutHeight)
	if release == nil {
		fmt.Fprint(os.Stderr, "GPU unavailable") // TODO(jbunds): implement CPU fallback ?
	}

	cc.BeginGPUFrame()
	cc.ClearWithColor(gg.Transparent)

	// background
	cc.SetColor(gg.Transparent)
	cc.DrawRectangle(0, 0, aboutWidth, aboutHeight)
	cc.Fill()

	// foreground
	cc.SetColor(gg.White)
	cc.SetFont(r.assets.fontSource.Face(10))
	cc.DrawString("Mandelbrot 1.0",   15, 24)
	cc.DrawString("Jeff Bunds",       15, 44)
	cc.DrawString("Copyright © 2026", 15, 64)
	cc.Fill()

	if err := cc.FlushGPUWithViewPreserveContent(view, aboutWidth, aboutHeight); err != nil {
		fmt.Fprint(os.Stderr, "GPU unavailable") // TODO(jbunds): implement CPU fallback ?
	}
	return &view, release
}

// release marks resources for deallocation.
func (r *renderer) release() {
	if r.assets.fractalViewRelease != nil {
		r.assets.fractalViewRelease()
	}
	r.assets.canvas.Close()
	r.assets.fontSource.Close()
	r.gpu.pipeline.Release()
	r.gpu.bgLayout0.Release()
	r.gpu.bgLayout1.Release()
	r.gpu.paletteBuf.Release()
	r.gpu.uniformBuf.Release()
	r.gpu.staticBindGroup.Release()
}

// updateUniforms updates the per-frame uniforms passed to the GPU shader.
func updateUniforms(
	frameCount                                 int,
	width,     height                          uint32,
	targetXHi, targetXLo, targetYHi, targetYLo float32,
	zoom,      iterations                      float64) uniforms {

	zoomHi, zoomLo := splitFloat64(zoom)

	return uniforms{
		paletteSize: paletteSize,
		width:       float32(width),
		height:      float32(height),
		iterations:  float32(iterations),

		zoomHi:      zoomHi,
		zoomLo:      zoomLo,
		targetXHi:   targetXHi,
		targetYHi:   targetYHi,

		targetXLo:   targetXLo,
		targetYLo:   targetYLo,
		frameCount:  float32(frameCount),
	}
}

// toggleAnimation toggles between pausing and resuming the animation loop,
// e.g., when the spacebar is pressed, or when the primary window is hidden.
func toggleAnimation(app *gogpu.App, token *atomic.Pointer[gogpu.AnimationToken]) {
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
