// Package fractal iteratively renders and magnifies fractals of the Mandelbrot or filled Julia set.
//
// Tested on a MacBook Air M1.
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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
	"github.com/jbunds/progress"
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
	baseIterations        = 500   // initial number of maximum iterations used to compute interior boundaries
	paletteSize           = 2000  // number of colors to pre-compute and pass to the GPU shader for fast lookup
	viewportWidth         = 3.0   // viewport width of the initial frame, i.e., the span of the complex plane rendered to the viewport
	scaleFactor           = 0.993 // multiplicative factor by which each successive rendering is iteratively magnified
	linearGrowthFactor    = 0.3   // multiplicative factor by which interior boundary calculation maximum iterations increase per each successive Mandelbrot fractal frame
	quadraticGrowthFactor = 0.003 // multiplicative factor by which interior boundary calculation maximum iterations increase per the square of the Julia fractal frame count
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
	shaderCode      string                // WGSL shader code (specific to fractal kind: Mandelbrot or Julia)
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
	theme   string
	fractal *fractal
	state   *state
	gpu     *gpu
	assets  *assets
}

// uniforms stores per-frame uniforms.
type uniforms struct { // total: (2 uint32 + 14 float32) * 4 bytes == 64 bytes
	paletteSize, powScale                   uint32
	frameCount,  maxIter                   float32 // block 1
	width,       height,  scaleHi, scaleLo float32 // block 2
	xRealHi,     yImagHi, xRealLo, yImagLo float32 // block 3
	cRealHi,     cImagHi, cRealLo, cImagLo float32 // block 4
}

// ui encapsulates GUI state and orchestrates window and animation lifecycle.
type ui struct {
	app                  *gogpu.App
	primaryWindow,
	aboutWindow          *gogpu.Window
	renderer             atomic.Value
	animToken            atomic.Pointer[gogpu.AnimationToken]
	lastFrameTime        time.Time
	initTokenOnce        sync.Once
	pendingMenuRebuild   bool
	hidePrimaryWindow,
	hideAboutWindow,
	aboutWindowIsOpen,
	aboutWindowHasFocus,
	resumeAnimWhenShown  atomic.Bool
}

func main() {
	fractal, theme, err := flags(flag.CommandLine, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot parse flags: %v\n", err)
		os.Exit(1)
	}

	// workaround for none of the following working as expected:
	//
	//   gogpu.SetLogger(nil)
	//   gogpu.SetLogger(slog.New(slog.DiscardHandler))
	//   gogpu.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	slog.SetDefault(slog.New(slog.DiscardHandler))

	ui := new(ui)

	ui.app = gogpu.NewApp(gogpu.DefaultConfig().
		WithAppName("Fractal").
		WithTitle(fractal.titleText).
		WithSize(mainWidth, mainHeight).
		WithResizable(false))

	ui.app.SetQuitOnLastWindowClosed(false)

	removeComments := func(text string) string {
		re := regexp.MustCompile(`(?m)^\s*//.*\n`)
		return strings.TrimSpace(re.ReplaceAllString(text, ""))
	}

	// closures (singletons)

	shaderCode := map[string]string{
		"mandelbrot": removeComments(commonShaderCode + "\n" + mandelbrotShaderCode),
		"julia":      removeComments(commonShaderCode + "\n" + juliaShaderCode),
	}

	ui.renderer.Store(newRenderer(fractal, shaderCode[fractal.kind], theme))

	rebuildThemesMenu := func() { // indirectly called via OnUpdate() by addFractalsMenu when a new fractal is selected from the menu
		themesMenu := gogpu.NewMenuWithTitle("Themes")
		for _, cs := range slices.Sorted(maps.Keys(colorSchemes())) {
			themesMenu.AddItem(gogpu.MenuItem{Title: cs, Action: func() { // TODO(jbunds): prepend a checkmark to the current theme menu item and disable it
				cr := ui.renderer.Load().(*renderer)
				if cr.theme == cs { return }
				// apply the new theme to the current renderer
				newRenderer := newRenderer(cr.fractal, shaderCode[cr.fractal.kind], cs)
				newRenderer.init(ui.app, cs)
				oldRenderer := ui.renderer.Swap(newRenderer).(*renderer)
				oldRenderer.release()
				ui.app.RequestRedraw()
			}})
		}
		ui.app.SetCustomMenu("themes", themesMenu)
	}

	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGINT,  // ctrl+c
		syscall.SIGTERM, // standard kill signal
		syscall.SIGHUP)  // terminal closed, SSH disconnection, etc
	go func() {
		<-sigChan
		ui.app.Quit() // triggers OnClose() to ensure clean progress bar shutdown
	}()

	// TODO(jbunds): instantiate a new progress bar whenever a new fractal is selected via the "Fractals" menu
	prog := progress.New(ctx, maxPrecisionFrames, os.Stderr,
		progress.WithTracker(progress.Fraction),
		progress.WithTheme("magma"),
		progress.WithPersistBar(true),
	)
	defer prog.Close() // called via panic() paths (idempotent)

	// GoGPU callback registrations and definitions

	ui.app.OnSurfaceAvailable(func() {
		ui.aboutWindow, err = ui.app.NewWindow(gogpu.DefaultConfig().
			WithTitle("").
			WithSize(aboutWidth, aboutHeight).
			WithTransparent(true).
			WithResizable(false).
			WithHeaderAlignment(gogpu.HeaderAlignLeft)) // renders transparent title bar
		if err != nil {
			panic(err)
		}
		ui.aboutWindow.Hide()
		ui.aboutWindowIsOpen.Store(false)
		ui.aboutWindowHasFocus.Store(false)

		ui.primaryWindow = ui.app.PrimaryWindow()

		ui.primaryWindow.SetOnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
			if mods.HasSuper() && key == gpucontext.KeyW { // ⌘+W
				if ui.aboutWindowHasFocus.Load() {
					ui.hideAboutWindow.Store(true)
					ui.app.RequestRedraw()
				} else if ui.primaryWindow.Visible() {
					ui.hidePrimaryWin()
				}
			}
		})

		ui.primaryWindow.SetOnClose(func() bool {
			if ui.primaryWindow.Visible() {
				ui.hidePrimaryWin()
			}
			return false // reject the close window request and hide the window instead
		})

		// TODO(jbunds): fix the following bug:
		//
		//   1. launch the app
		//   2. open the About window
		//   3. use macOS "Hot Corners" to bring the primary application window into the foreground and give it focus
		//   4. press ⌘+W
		//   5. bug: the About window is closed instead of the primary application window
		//
		// i suspect this may be impossible to fix unless and until the GoGPU core API supports per-window focus tracking
		ui.primaryWindow.SetOnPointer(func(e gpucontext.PointerEvent) {
			if e.Type == gpucontext.PointerDown {
				ui.aboutWindowHasFocus.Store(false)
			}
		})

		cr := ui.renderer.Load().(*renderer)
		cr.init(ui.app, cr.theme)

		setCustomAppMenu(ui)

		addFractalsMenu(ui, shaderCode)

		rebuildThemesMenu()

		//  TODO(jbunds): fix whatever is removing the native "Window" menu,
		//                which may be triggered by customizing the menus per
		//                the call to app.SetMenu() or app.SetCustomMenu() (?)
		//
		//                the "Window" menu only gets added without the calls to
		//                setCustomAppMenu, addFractalsMenu, and rebuildThemesMenu
		addWindowMenu(ui.app)
	})

	ui.app.EventSource().OnKeyPress(func(key gpucontext.Key, _ gpucontext.Modifiers) {
		if key == gpucontext.KeySpace { // space bar
			toggleAnimation(ui.app, &ui.animToken)
		}
	})

	ui.app.OnUpdate(func(_ float64) {
		if ui.pendingMenuRebuild {
			rebuildThemesMenu()
			ui.pendingMenuRebuild = false
		}
		if ui.hidePrimaryWindow.Swap(false) {
			ui.primaryWindow.Hide()
		}
		if ui.hideAboutWindow.Swap(false) {
			ui.aboutWindow.Hide()
			ui.aboutWindowIsOpen.Store(false)
			ui.aboutWindowHasFocus.Store(false)
			if ui.primaryWindow.Visible() {
				ui.primaryWindow.Show()
			}
		}
	})

	ui.app.OnDraw(func(dc *gogpu.Context) {
		cr := ui.renderer.Load().(*renderer)
		cr.draw(dc, &ui.animToken)

		elapsed := time.Since(ui.lastFrameTime).Milliseconds()
		if elapsed > 0 {
			cr.state.fps = float64(1000.0 / elapsed)
		}
		ui.lastFrameTime = time.Now()

		ui.initTokenOnce.Do(func() {
			ui.animToken.Store(ui.app.StartAnimation())
		})

		if cr.state.frameCount <= maxPrecisionFrames {
			prog.Report(1, strconv.Itoa(cr.state.frameCount))
		} else {
			prog.Close() // normal progress bar shutdown sequence
		}

		if ui.animToken.Load() != nil {
			ui.app.RequestRedraw() // renders at VSync frequency (~60 FPS)
		}
	})

	ui.app.OnClose(func() {
		ui.renderer.Load().(*renderer).release()
		gg.CloseAccelerator()
		cancel()
		prog.Close()
	})

	ui.lastFrameTime = time.Now()

	// main event loop

	if err := ui.app.Run(); err != nil {
		ui.renderer.Load().(*renderer).release()
		gg.CloseAccelerator()
		cancel()
		panic(err) // deferred call to prog.Close() triggered here
	}

	cancel()
}

// newRenderer constructs and returns the *renderer used to store runtime state.
func newRenderer(fractal *fractal, shaderCode, theme string) *renderer {
	xRealHi, xRealLo := splitFloat64(fractal.params.xReal)
	yImagHi, yImagLo := splitFloat64(fractal.params.yImag)
	cRealHi, cRealLo := splitFloat64(fractal.params.cReal)
	cImagHi, cImagLo := splitFloat64(fractal.params.cImag)

	return &renderer{
		fractal: fractal,
		theme:   theme,
		gpu:     &gpu{shaderCode: shaderCode},
		assets:  &assets{},
		state:   &state{
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

// pauseAnimation stops the animation if running and records whether to resume on Show().
func (u *ui) pauseAnimation() {
	if oldToken := u.animToken.Swap(nil); oldToken != nil {
		oldToken.Stop()
		u.resumeAnimWhenShown.Store(true)
	} else {
		u.resumeAnimWhenShown.Store(false)
	}
}

// hidePrimaryWin pauses the animation and defers hiding the primary window to OnUpdate().
func (u *ui) hidePrimaryWin() {
	u.pauseAnimation()
	u.hidePrimaryWindow.Store(true) // defer call to primaryWindow.Hide() via OnUpdate() to avoid GoGPU internal mutex deadlock
	u.app.RequestRedraw()           // ensure OnUpdate() fires even if the animation is paused
}

// scheduleMenuRebuild marks the "Themes" menu for rebuild per the next OnUpdate() cycle.
func (u *ui) scheduleMenuRebuild() {
	u.pendingMenuRebuild = true
	u.app.RequestRedraw()
}

// init initializes all resources required to render frames in the main application window.
func (r *renderer) init(app *gogpu.App, theme string) {
	// TODO(jbunds): consider using a sync.Map-based main-global (singleton) cache to cache all static resources
	//               (arguably overkill as the overhead of reinstantiating static resources is not a bottleneck)
	//
	//	func getOrInit[T any](r *renderer, key string, load func() (T, error)) T {
	//		if val, ok := r.cache.Load(key); ok {
	//			return val.(T)
	//		}
	//		onceKey := key + ":_once"
	//		val, _  := r.cache.LoadOrStore(onceKey, new(sync.Once))
	//		once    := val.(*sync.Once)
	//		var result T
	//		var onceErr error
	//		once.Do(func() {
	//			result, onceErr = load()
	//			if onceErr != nil { panic(onceErr) }
	//			r.cache.Store(key, result)
	//			r.cache.Delete(onceKey)
	//		})
	//		val, ok := r.cache.Load(key)
	//		if !ok { panic("cache invariant broken: value not stored") }
	//		return val.(T)
	//	}

	var err error
	r.assets.fontSource, err = loadFont(os.DirFS("/").(fs.StatFS))
	if err != nil {
		panic(err)
	}

	r.gpu.device = app.DeviceProvider().Device()

	r.state.paletteColors,
	r.gpu.paletteTex,
	r.gpu.uniformBuf,
	r.gpu.bgLayout0,
	r.gpu.bgLayout1,
	r.gpu.pipeline = initResources(r.gpu.device, r.gpu.shaderCode, theme)

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
		&wgpu.ImageCopyTexture{Texture: r.gpu.paletteTex},
		unsafe.Slice(
			(*byte)(unsafe.Pointer(&r.state.paletteColors[0])), // #nosec G103 - audited
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
		// fmt.Println("stopped rendering (precision exhausted)")
		return
	}

	// per-frame state updates

	r.state.viewportWidth *= scaleFactor
	r.state.frameCount++

	unis := updateUniforms( // magnification logic
		r.state.frameCount,    r.fractal.params.powScale,
		r.state.xRealHi,       r.state.xRealLo,
		r.state.yImagHi,       r.state.yImagLo,
		r.state.cRealHi,       r.state.cRealLo,
		r.state.cImagHi,       r.state.cImagLo,
		r.state.viewportWidth, r.fractal.params.maxIter(r.state.frameCount),
	)

	err := r.gpu.device.Queue().WriteBuffer(
		r.gpu.uniformBuf, 0,
		unsafe.Slice((*byte)(unsafe.Pointer(unis)), 64)) // #nosec G103 - audited
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
	r.assets.fractalViewRelease()
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
