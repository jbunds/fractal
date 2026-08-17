// Package mandelbrot/gogpu iteratively renders and magnifies the Mandelbrot or filled Julia set.
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
	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/wgpu"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

//go:embed common.wgsl
var commonShaderCode string

//go:embed mandelbrot.wgsl
var mandelbrotShaderCode string

//go:embed julia.wgsl
var juliaShaderCode string

const (
	mainWidth          = 800   // primary window logical width
	mainHeight         = 800   // primary window logical height
	aboutWidth         = 240   // about window logical width
	aboutHeight        = 164   // about window logical height
	baseIterations     = 500   // initial number of iterations used to compute interior boundaries
	paletteSize        = 2000  // number of colors to pre-compute and pass to the GPU shader for fast lookup
	viewportWidth      = 3.0   // viewport width of the initial frame, i.e., the span of the complex plane rendered to the viewport
	scaleFactor        = 0.993 // multiplicative factor by which each successive rendering is iteratively magnified
	growthRate         = 0.3   // multiplicative factor by which boundary calculation iterations increase per each successive frame
	maxPrecisionFrames = 2745  // empirically-determined limit for the number of frames to render before reaching precision limit
)

// state stores the application state (uniforms, color palette, and FPS stats).
type state struct {
	frameCount            int      // tracks the number of frames rendered
	paletteColors         []uint32 // pre-computed color palette
	viewportWidth,                 // tracks the width of the current frame's view of the complex plane
	fps                   float64  // fps imprecisely tracks FPS rendered
	targetXHi, targetYHi,
	targetXLo, targetYLo  float32  // target coordinates of the Mandelbrot set
	cRealHi,   cRealLo,
	cImagHi,   cImagLo    float32  // complex constant term of the Julia set
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

// assets stores assets used to render frames (canvas, font, texture view).
type assets struct {
	canvas             *ggcanvas.Canvas       // wraps gg.Context
	fontSource         *text.FontSource       // font used to render per-frame stats
	fractalView        gpucontext.TextureView // handle to the fractal texture view
	fractalViewRelease func()                 // fractal TextureView release function
}

// renderer stores most runtime state.
type renderer struct {
	isJulia     bool
	usePowScale uint32
	state       *state
	gpu         *gpu
	assets      *assets
}

// uniforms stores per-frame uniforms.
type uniforms struct { // total: (2 uint32 + 14 float32) * 4 bytes == 64 bytes
	paletteSize, usePowScale                      uint32
	frameCount,  iterations                      float32 // block 1
	width,       height,    scaleHi,   scaleLo   float32 // block 2
	targetXHi,   targetYHi, targetXLo, targetYLo float32 // block 3
	cRealHi,     cImagHi,   cRealLo,   cImagLo   float32 // block 4
}

func main() {
	var (
		initTokenOnce   sync.Once
		lastFrameTime   time.Time
		animToken       atomic.Pointer[gogpu.AnimationToken]
		currentRenderer atomic.Value
	)

	fractal, coords, err := flags(flag.CommandLine, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot parse flags: %v\n", err)
		os.Exit(1)
	}

	var shaderCode string
	var p1, p2     float64 // TODO(jbunds): use better names for these overloaded variables
	switch fractal {
	case "julia":
		p1, p2     = coords.cReal, coords.cImag
		shaderCode = commonShaderCode + "\n" + juliaShaderCode
	default:
		p1, p2     = coords.x, coords.y
		shaderCode = commonShaderCode + "\n" + mandelbrotShaderCode
	}

	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithAppName("Mandelbrot").
		WithTitle(fmt.Sprintf("%s - %s (c = %v, %vi)", fractal, coords.name, p1, p2)).
		WithSize(mainWidth, mainHeight).
		WithResizable(false))

	currentRenderer.Store(newRenderer(fractal, coords))

	// GoGPU callback registrations and definitions

	app.OnSurfaceAvailable(func() {
		aboutWin, err := app.NewWindow(gogpu.DefaultConfig().
			WithTitle(""). // looks more slick and modern when combined with the transparent title bar triggered via WithHeaderAlignment(gogpu.HeaderAlignLeft) below
			WithSize(aboutWidth, aboutHeight).
			WithTransparent(true).
			WithResizable(false).
			WithHeaderAlignment(gogpu.HeaderAlignLeft)) // renders transparent title bar
		if err != nil {
			panic(err)
		}
		aboutWin.Hide()

		cr := currentRenderer.Load().(*renderer)
		cr.init(app, shaderCode)

		// when appendAboutMenuItem is called before addPointsMenu, duplicate items are added to the main application menu

		//  TODO(jbunds): fix whatever is removing the native "Window" menu,
		//                which may be triggered by customizing the menus per
		//                the call to app.SetCustomMenu() in addPointsMenu() ?
		cr.addPointsMenu(app, &currentRenderer)

		// TODO(jbunds): replace the native "About ..." application menu item action instead of appending to the menu
		cr.appendAboutMenuItem(app, &currentRenderer, aboutWin)

//		if winMenu := app.GetSystemMenu(gogpu.SystemMenuWindow); winMenu != nil {
//			winMenu.AddItem(gogpu.MenuItem{Separator: true})
//			winMenu.AddItem(gogpu.MenuItem{
//				Title: fmt.Sprintf("mandelbrot - %s", coords.name),
//				Role:  gogpu.RoleShowAll,         // no RoleMaximize
//				Role:  gogpu.RoleBringAllToFront, // no RoleMaximize
//			})
//		}
	})

	app.EventSource().OnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		switch {
		case mods.HasSuper():
			switch key {
			case gpucontext.KeyQ: // ⌘+q
				currentRenderer.Load().(*renderer).release()
				app.Quit()
			case gpucontext.KeyW: // ⌘+w
				if animToken.Load() != nil {
					animToken.Swap(nil) // reduce GPU load by suspending animation while the primary window is hidden
				}
				app.PrimaryWindow().Hide()
			}
		case key == gpucontext.KeySpace:
			toggleAnimation(app, &animToken)
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
func newRenderer(fractal string, coords coords) *renderer {
	targetXHi, targetXLo := splitFloat64(coords.x)
	targetYHi, targetYLo := splitFloat64(coords.y)
	cRealHi,   cRealLo   := splitFloat64(coords.cReal)
	cImagHi,   cImagLo   := splitFloat64(coords.cImag)
	var usePowScale uint32
	if coords.name == "airplane"    ||
	   coords.name == "basilica"    ||
	   coords.name == "cantor dust" ||
	   coords.name == "rabbit"      ||
	   coords.name == "siegel"      {
		usePowScale = 1
	}
	return &renderer{
		isJulia:     fractal == "julia",
		usePowScale: usePowScale,
		gpu:         new(gpu),
		assets:      new(assets),
		state:       &state{
			frameCount:    0,
			viewportWidth: viewportWidth,
			targetXHi:     targetXHi,
			targetXLo:     targetXLo,
			targetYHi:     targetYHi,
			targetYLo:     targetYLo,
			cRealHi:       cRealHi,
			cRealLo:       cRealLo,
			cImagHi:       cImagHi,
			cImagLo:       cImagLo,
		},
	}
}

// init initializes all resources required to render frames in the main application window.
func (r *renderer) init(app *gogpu.App, shaderCode string) {
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
		pipeline := initResources(r.gpu.device, shaderCode)


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
			{Binding: 0, Size: 64,                      Buffer: r.gpu.uniformBuf},
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

	r.state.viewportWidth *= scaleFactor
	r.state.frameCount++

	iterations := float64(baseIterations) + float64(r.state.frameCount) * growthRate // GPU fractal region-detection iterations

	if r.isJulia {
		// decent (except for siegel):
		iterations = float64(baseIterations) + (float64(r.state.frameCount * r.state.frameCount) * 0.003)
	}

	unis := updateUniforms( // magnification logic
		r.state.frameCount,    r.usePowScale,
		mainWidth,             mainHeight,
		r.state.targetXHi,     r.state.targetXLo,
		r.state.targetYHi,     r.state.targetYLo,
		r.state.cRealHi,       r.state.cRealLo,
		r.state.cImagHi,       r.state.cImagLo,
		r.state.viewportWidth, iterations,
	)

	err := r.gpu.device.Queue().WriteBuffer(r.gpu.uniformBuf, 0, unsafe.Slice((*byte)(unsafe.Pointer(unis)), 64))
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

// appendAboutMenuItem appends an "About Mandelbrot" menu item to the application
// menu which renders a small window with some text when selected.
func (r *renderer) appendAboutMenuItem(app *gogpu.App, cr *atomic.Value, aboutWin *gogpu.Window) {
	app.SetMenu(gogpu.NewMenu().
		// TODO(jbunds): fix bug whereby selecting the custom "About Mandelbrot" item from the
		//               application menu incorrectly renders a new frame to the primary window
		AddItem(gogpu.MenuItem{Title: "About Mandelbrot", Role: gogpu.RoleAbout, Action: func() { aboutWin.Show() }}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Settings…",        Role: gogpu.RolePreferences}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Services",         Role: gogpu.RoleServices}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Hide Mandelbrot",  Role: gogpu.RoleHide}).
		AddItem(gogpu.MenuItem{Title: "Hide Others",      Role: gogpu.RoleHideOthers}).
		AddItem(gogpu.MenuItem{Title: "Show All",         Role: gogpu.RoleShowAll}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Quit Mandelbrot",  Role: gogpu.RoleQuit}))

//	appMenu := app.GetSystemMenu(gogpu.SystemMenuApplication)
//	appMenu.AddItem(gogpu.MenuItem{Separator: true})
//	appMenu.AddItem(gogpu.MenuItem{
//		Title:  " \u24D8  About Mandelbrot", // or " ⓘ   About Mandelbrot"
//		Role:   gogpu.RoleAbout, // doesn't prepend the "circled i" system icon to the title like RolePreferences (gear icon) or RoleQuit ("boxed x" icon) roles do
//		Action: func() { aboutWin.Show() },
//	})

	canvas, err := ggcanvas.New(app.GPUContextProvider(), aboutWidth, aboutHeight)
	if err != nil {
		panic(err)
	}

	var (
		renderOnce sync.Once
		view       *gpucontext.TextureView
		release    func()
	)

	// assumes the bimedians (perpendicular bisectors) of both windows (primary and About) are aligned
	offsetX := mainWidth  / 2.0 - aboutWidth  / 2.0
	offsetY := mainHeight / 2.0 - aboutHeight / 2.0

	aboutWin.SetOnDraw(func(dc *gogpu.Context) {
		ar  := cr.Load().(*renderer) // in case the user selects one of the menu items from the "Points" menu before selecting the About menu item
		err := canvas.Draw(func(cc *gg.Context) {
			renderOnce.Do(func() {
				view, release = ar.renderAboutWindow(cc)
			})
			cc.MarkFrameRendered() // ?

			// composite the About window TextureView atop the background

			cc.Push()                        // save pre-translation state
			cc.Translate(-offsetX, -offsetY) // align the background of the About window with the fractal image beneath
			cc.DrawGPUTextureBase(ar.assets.fractalView, 0, 0, aboutWidth, aboutHeight)
			cc.Pop()                         // restore pre-translation state so the overlay and text remain unaffected by the translation
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
	})

//		aboutWin.SetOnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
//fmt.Println("HERE")
//			if mods.HasSuper() {
//				switch key {
//				case gpucontext.KeyW: // ⌘+w
//					aboutWin.Close()
//				case gpucontext.KeyQ: // ⌘+q
//					cr.Load().(*renderer).release()
//					app.Quit()
//				}
//			}
//		})
//
	aboutWin.SetOnClose(func() bool {
		app.PrimaryWindow().Show() // i don't know why the primary window loses focus, but it does...
		if release != nil {
			release()
		}
		return true
	})

	// https://pkg.go.dev/github.com/gogpu/gogpu#readme-multi-window-input
	//
	// despite what the godoc ^ claims ("fires only when w2 is focused"), when the
	// "About Mandelbrot" window is focused and ⌘+w is pressed, the primary window closes
	//
	// TODO(jbunds): intercept ⌘+w and call aboutWin.Close() ONLY if aboutWin has focus when ⌘+w is pressed
	//               fixing this may require github.com/gogpu/ui/app and github.com/gogpu/ui/desktop
	//               https://pkg.go.dev/github.com/gogpu/gogpu#readme-multi-window-input

//	aboutWin.SetOnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
//		// checking aboutWin.Visible() doesn't help here, ⌘+w still closes the primary window even when the About window has focus
//		// why is https://pkg.go.dev/github.com/gogpu/gogpu#App.HasFocus a method on App instead of Window ?
//		// https://pkg.go.dev/github.com/gogpu/gpucontext#FocusEvent
//		//aboutWin.HasFocus()
//		if key == gpucontext.KeyW && mods.HasSuper() { // ⌘+w
//			aboutWin.Close()
//		}
//	})
//
//	aboutWin.SetOnClose(func() bool {
//		app.PrimaryWindow().Show() // i don't know why the primary window loses focus, but it does...
//		if release != nil {
//			release()
//		}
//		return true
//	})
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
	cc.DrawString("Mandelbrot 1.0",   15, 30)
	cc.DrawString("Jeff Bunds",       15, 50)
	cc.DrawString("Copyright © 2026", 15, 70)
	cc.Fill()

	if err := cc.FlushGPUWithViewPreserveContent(view, aboutWidth, aboutHeight); err != nil {
		fmt.Fprint(os.Stderr, "GPU unavailable") // TODO(jbunds): implement CPU fallback ?
	}
	return &view, release
}

// addPointsMenu creates a "Points" menu to allow users to select a new points of interest from a preset list of named target coordinates.
func (r *renderer) addPointsMenu(app *gogpu.App, cr *atomic.Value) {
	points     := pointsOfInterest()
	pointsMenu := gogpu.NewMenuWithTitle("Points")

	for _, fractal := range slices.Sorted(maps.Keys(points)) {
		fractalMenu := gogpu.NewMenu()
		for _, coordsName := range slices.Sorted(maps.Keys(points[fractal])) {
			// TODO(jbunds): clean this up
			p1, p2 := points[fractal][coordsName].x, points[fractal][coordsName].y
			if fractal == "julia" {
				p1, p2 = points[fractal][coordsName].cReal, points[fractal][coordsName].cImag
			}
			fractalMenu.AddItem(gogpu.MenuItem{Title: fmt.Sprintf("%s:  %v, %vi", coordsName, p1, p2), Action: func() {
				newRenderer := newRenderer(fractal, points[fractal][coordsName])
				oldRenderer := cr.Swap(newRenderer).(*renderer) // replaces currentRenderer in main() scope to reset the render cycle with new target coordinates
				oldRenderer.release()
				// TODO(jbunds): clean this up
				shaderCode := commonShaderCode + "\n" + mandelbrotShaderCode
				if fractal == "julia" {
					shaderCode = commonShaderCode + "\n" + juliaShaderCode
				}
				newRenderer.init(app, shaderCode)
				app.SetTitle(fmt.Sprintf("%s - %s (c = %v, %vi)", fractal, coordsName, p1, p2))
				app.PrimaryWindow().Show()
				app.RequestRedraw()
			}})
		}
		pointsMenu.AddItem(gogpu.MenuItem{
			Title:   cases.Title(language.English).String(fractal),
			Submenu: fractalMenu})
	}

	// TODO(jbunds): determine what causes the animation to pause when the user clicks on any menu header
	// TODO(jbunds): determine what causes the native "Window" menu to be removed from the menu bar
	app.SetCustomMenu("points", pointsMenu)
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
	frameCount                int,
	usePowScale,
	width,         height     uint32,
	targetXHi,     targetXLo,
	targetYHi,     targetYLo,
	cRealHi,       cRealLo,
	cImagHi,       cImagLo    float32,
	viewportWidth, iterations float64) *uniforms {

	scaleHi, scaleLo := splitFloat64(viewportWidth)

	return &uniforms{
		paletteSize: paletteSize,
		scaleHi:     scaleHi,
		scaleLo:     scaleLo,
		usePowScale: usePowScale,

		width:       float32(width),
		height:      float32(height),
		iterations:  float32(iterations),
		frameCount:  float32(frameCount),

		targetXHi:   targetXHi,
		targetYHi:   targetYHi,
		targetXLo:   targetXLo,
		targetYLo:   targetYLo,

		cRealHi:     cRealHi,
		cRealLo:     cRealLo,
		cImagHi:     cImagHi,
		cImagLo:     cImagLo,
	}
}

// toggleAnimation toggles between pausing and resuming the animation loop,
// e.g., when the spacebar is pressed, or when the primary window is hidden.
func toggleAnimation(app *gogpu.App, token *atomic.Pointer[gogpu.AnimationToken]) {
	// TODO(jbunds): fix the bug where animation is not actually toggled when the "About Mandelbrot" window has focus
	if oldToken := token.Swap(nil); oldToken != nil {
//fmt.Println("suspending animation")
		oldToken.Stop()
	} else {
//fmt.Println("resuming animation")
		token.Store(app.StartAnimation())
	}
}

// splitFloat64 splits a float64 into two float32s.
func splitFloat64(v float64) (float32, float32) {
	high := float32(v)
	low  := float32(v - float64(high))
	return high, low
}
