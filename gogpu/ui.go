package main

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// TODO(jbunds): refactor addFractalsMenu & uiLabels (at least they're not in the hot path...)

// addFractalsMenu creates a "Fractals" menu to allow users to select a new combination of
// fractal kind ("Mandelbrot" or "Julia") and parameter of interest (target x, y coordinates
// to zoom in on for the Mandelbrot set, or complex constant for the filled Julia set)
// from a preset list of named (or unnamed) parameters.
func addFractalsMenu(app *gogpu.App, cr *atomic.Value, scheduleMenuRebuild func()) {
	fractals                := fractals()
	labels, sortedMenuItems := uiLabels(fractals)
	fractalsMenu            := gogpu.NewMenuWithTitle("Fractals")

	for _, kind := range kinds(fractals) {
		fractalMenu := gogpu.NewMenu()
		for _, name := range sortedMenuItems[kind] {
			newFractal := fractals[name]
			label      := labels[kind][name]
			fractalMenu.AddItem(gogpu.MenuItem{Title: label, Action: func() {
				curRenderer := cr.Load().(*renderer)
				curRenderer.release()
				newRenderer := newRenderer(newFractal, curRenderer.theme)
				newRenderer.init(app, kind, curRenderer.theme)
				cr.Store(newRenderer) // alternatively: cr.Swap(newRenderer).(*renderer).release() here and delete the line calling release() above
				scheduleMenuRebuild()
				// TODO(jbunds): handle case where user closed the primary window by clicking on the "close window"
				//               icon in the window's title bar, preferably by somehow hiding the primary window
				//               instead of actually closing it, easily facilitating its reuse here
				//
				//               note that attempting to instantiate a new "primary" window here
				//               via app.NewWindow() will crash the program
				app.SetTitle(newFractal.titleText)
				app.RequestRedraw()
				app.PrimaryWindow().Show() // program will crash here if the user closed the primary window by clicking
				                           // on the "close window" icon in the primary window's title bar
			}})
		}
		fractalsMenu.AddItem(gogpu.MenuItem{
			Title:   cases.Title(language.English).String(kind),
			Submenu: fractalMenu,
		})
	}

	// TODO(jbunds): determine what causes the animation to pause when the user clicks on any menu header
	// TODO(jbunds): determine what causes the native "Window" menu to be removed from the menu bar
	app.SetCustomMenu("fractals", fractalsMenu) // either this call or the call to app.SetCustomMenu() in rebuildMenus() erroneously extends the native application menu
}

// uiLabels creates formatted and sorted lists of labels for UI elements as maps keyed off the given "params" struct.
func uiLabels(fractals map[string]*fractal) (map[string]map[string]string, map[string][]string) {
	maxLen := make(map[string]int)
	for _, fractal := range fractals {
		maxLen[fractal.kind] = max(maxLen[fractal.kind], len(normalizeName(fractal.name)))
	}

	format := map[string]map[string]string{
		"menu": { // TODO(jbunds): encapsulate format strings in params.go
			"mandelbrot": fmt.Sprintf("%%-%ds  %%v, %%vi", maxLen["mandelbrot"] + 1),
			"julia":      fmt.Sprintf("%%-%ds  c = %%v + %%vi", maxLen["julia"] + 1),
		},
		"window": {
			"mandelbrot": "%s - %s (%v, %vi)",
			"julia":      "%s - %s (c = %v + %vi)",
		},
	}

	labelEntries    := make(map[string]map[string]string, len(fractals)) // fractal-specific formatted menu item text and window title
	sortedMenuItems := make(map[string][]string)                         // fractal-kind-specific sorted list of menu items

	for _, kind := range kinds(fractals) {
		labelEntries[kind]  = make(map[string]string)

		sortedFractalNames := slices.SortedFunc(fractalNamesByKind(fractals, kind), func(a, b string) int {
			// TODO(jbunds): clean this up
			prioA := 0; if isUnnamed(a) { prioA = 1 }
			prioB := 0; if isUnnamed(b) { prioB = 1 }
			if prioA != prioB { return prioA - prioB }
			return cmp.Compare(a, b)
		})

		sortedMenuItems[kind] = sortedFractalNames

		for _, name := range sortedFractalNames {
			fractal     := fractals[name]
			displayName := normalizeName(fractal.name)
			
			switch kind { // TODO(jbunds): encapsulate fractal-kind-specific UI display logic and special case handling in params.go
			case "mandelbrot":
				displayXReal := fmt.Sprintf("%v", fractal.params.xReal) // hack to avoid using strconv
				if !strings.HasPrefix(displayXReal, "-") {
					displayXReal = " " + fmt.Sprintf("%v", fractal.params.xReal)
				}
				labelEntries[kind][name] = fmt.Sprintf(format["menu"][kind], displayName + ":", displayXReal, fractal.params.yImag)
			case "julia":
				displayCReal := fmt.Sprintf("%v", fractal.params.cReal) // hack to avoid using strconv
				displayCImag := fmt.Sprintf("%v", fractal.params.cImag)
				if !strings.HasPrefix(displayCReal, "-") {
					displayCReal = " " + fmt.Sprintf("%v", fractal.params.cReal)
				}
				if fractal.name == "golden" { // special handling for φ since it's irrational
					displayCReal = "(φ - 2)"
					displayCImag = "(φ - 1)"
				}
				labelEntries[kind][name] = fmt.Sprintf(format["menu"  ][kind], displayName + ":", displayCReal, displayCImag)
			}
		}
	}

	return labelEntries, sortedMenuItems
}

// appendAboutMenuItem appends an "About Fractal" menu item to the application
// menu which renders a small window with some text when selected.
func appendAboutMenuItem(app *gogpu.App, cr *atomic.Value, aboutWin *gogpu.Window) {
	app.SetMenu(gogpu.NewMenu().
		// TODO(jbunds): fix bug whereby selecting the custom "About Fractal" item from the
		//               application menu incorrectly renders a new frame to the primary window
		AddItem(gogpu.MenuItem{Title: "About Fractal", Role: gogpu.RoleAbout, Action: func() { aboutWin.Show() }}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Settings…",     Role: gogpu.RolePreferences}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Services",      Role: gogpu.RoleServices}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Hide Fractal",  Role: gogpu.RoleHide}).
		AddItem(gogpu.MenuItem{Title: "Hide Others",   Role: gogpu.RoleHideOthers}).
		AddItem(gogpu.MenuItem{Title: "Show All",      Role: gogpu.RoleShowAll}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Quit Fractal",  Role: gogpu.RoleQuit}))

//	appMenu := app.GetSystemMenu(gogpu.SystemMenuApplication)
//	appMenu.AddItem(gogpu.MenuItem{Separator: true})
//	appMenu.AddItem(gogpu.MenuItem{
//		Title:  " \u24D8  About Fractal", // or " ⓘ   About Fractal"
//		Role:   gogpu.RoleAbout, // doesn't prepend the "circled i" system icon to the title like RolePreferences (gear icon) or RoleQuit ("boxed x" icon) roles do
//		Action: func() { aboutWin.Show() },
//	})

	var (
		release  func()
		drawOnce sync.Once
	)

	aboutWin.SetOnDraw(func(dc *gogpu.Context) {
		drawOnce.Do(func() {
			release = drawAboutWindow(app, dc, cr)
		})
	})

//		aboutWin.SetOnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
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

	aboutWin.SetOnClose(func() bool {
// properly handle the primary window losing focus:
//   https://pkg.go.dev/github.com/gogpu/ui/widget#ContextImpl.RequestFocus
//		app.PrimaryWindow().Show()
		if release != nil {
			release()
		}
		return true
	})

	// https://pkg.go.dev/github.com/gogpu/gogpu#readme-multi-window-input
	//
	// despite what the godoc ^ claims ("fires only when w2 is focused"), when the
	// "About Fractal" window is focused and ⌘+w is pressed, the primary window closes
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

// drawAboutWindow draws the About window once on demand.
func drawAboutWindow(app *gogpu.App, dc *gogpu.Context, cr *atomic.Value) func() {
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

	ar := cr.Load().(*renderer) // in case the user selects one of the menu items from the "Fractals" menu before selecting the About menu item
	err = canvas.Draw(func(cc *gg.Context) {
		renderOnce.Do(func() {
			view, release = renderAboutWindow(cc, ar.assets.fontSource.Face(10))
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
	return release
}

// renderAboutWindow renders the About window content to an off-screen texture.
func renderAboutWindow(cc *gg.Context, fontFace text.Face) (*gpucontext.TextureView, func()) {
	view, release := cc.CreateOffscreenTexture(aboutWidth, aboutHeight)
	if release == nil {
		fmt.Fprint(os.Stderr, "GPU unavailable") // TODO(jbunds): implement CPU fallback ?
	}

	cc.BeginGPUFrame()
	cc.ClearWithColor(gg.Transparent)

	cc.SetColor(gg.Transparent) // background
	cc.DrawRectangle(0, 0, aboutWidth, aboutHeight)
	cc.Fill()

	cc.SetColor(gg.White)       // foreground
	cc.SetFont(fontFace)
	cc.DrawString("Fractal 1.0",      15, 30)
	cc.DrawString("Jeff Bunds",       15, 50)
	cc.DrawString("Copyright © 2026", 15, 70)
	cc.Fill()

	if err := cc.FlushGPUWithViewPreserveContent(view, aboutWidth, aboutHeight); err != nil {
		fmt.Fprint(os.Stderr, "GPU unavailable") // TODO(jbunds): implement CPU fallback ?
	}
	return &view, release
}

// normalizeName returns "unnamed" if the name starts with + or -, original name otherwise.
func normalizeName(name string) string {
	if isUnnamed(name) {
		return "unnamed"
	}
	return name
}

// isUnnamed returns true if the parameter has no associated canonical name.
func isUnnamed(name string) bool {
	return strings.HasPrefix(name, "+") ||
	       strings.HasPrefix(name, "-")
}
