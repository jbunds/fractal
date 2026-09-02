package main

import (
	"fmt"
	"os"
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

// setCustomAppMenu sets the application menu with a custom "About Fractal" manu
// item which renders a small translucent window with some text when selected.
func setCustomAppMenu(app *gogpu.App, cr *atomic.Value, aboutWindow *gogpu.Window, aboutWindowIsOpen *atomic.Bool, hideAboutWindow *atomic.Bool) {
	app.SetMenu(gogpu.NewMenu().
		// TODO(jbunds): fix bug whereby selecting the custom "About Fractal" item from the
		//               application menu incorrectly renders a new frame to the primary window
		AddItem(gogpu.MenuItem{Title: "About Fractal", Role: gogpu.RoleAbout, Action: func() { aboutWindow.Show(); aboutWindowIsOpen.Store(true) }}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Settings…",     Role: gogpu.RolePreferences}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Services",      Role: gogpu.RoleServices}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Hide Fractal",  Role: gogpu.RoleHide}).
		AddItem(gogpu.MenuItem{Title: "Hide Others",   Role: gogpu.RoleHideOthers}).
		AddItem(gogpu.MenuItem{Title: "Show All",      Role: gogpu.RoleShowAll}).
		AddItem(gogpu.MenuItem{Separator: true}).
		AddItem(gogpu.MenuItem{Title: "Quit Fractal",  Role: gogpu.RoleQuit, Action: func() { app.Quit() }}))

	var (
		release  func()
		drawOnce sync.Once
	)

	aboutWindow.SetOnDraw(func(dc *gogpu.Context) {
		drawOnce.Do(func() {
			release = drawAboutWindow(app, dc, cr)
		})
	})

	aboutWindow.SetOnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		if mods.HasSuper() && key == gpucontext.KeyW { // ⌘+W
			hideAboutWindow.Store(true) // defer call to aboutWindow.Hide() to avoid GoGPU internal mutex deadlock
			app.RequestRedraw()         // ensure OnUpdate fires even if animation is paused
		}
	})

	aboutWindow.SetOnClose(func() bool {
		if release != nil {
			release()
		}
		hideAboutWindow.Store(true) // defer call to aboutWindow.Hide() to avoid GoGPU internal mutex deadlock
		app.RequestRedraw()         // ensure OnUpdate fires even if animation is paused
		return false                // reject native close / destroy request and hide instead to preserve window handle and callbacks
	})
}

// TODO(jbunds): refactor addFractalsMenu & labels (at least they're not in the hot path...)

// addFractalsMenu creates a "Fractals" menu to allow users to select a new combination of
// fractal kind ("Mandelbrot" or "Julia") and parameter of interest (target x, y coordinates
// to zoom in on for the Mandelbrot set, or complex constant for the filled Julia set)
// from a preset list of named (or unnamed) parameters.
//
// Selecting a new fractal from the menu also schedules a rebuild of the "Themes" menu
// to use the new renderer so the two menus remain in sync.
func addFractalsMenu(app *gogpu.App, cr *atomic.Value, shaderCode map[string]string, scheduleMenuRebuild func()) {
	fractals                := fractals()
	labels, sortedMenuItems := labels(fractals)
	fractalsMenu            := gogpu.NewMenuWithTitle("Fractals")

	for _, kind := range kinds(fractals) {
		fractalMenu := gogpu.NewMenu()
		for _, name := range sortedMenuItems[kind] {
			newFractal := fractals[name]
			label      := labels[kind][name]
			fractalMenu.AddItem(gogpu.MenuItem{Title: label, Action: func() { // TODO(jbunds): disable the menu item for the fractal currently being rendered
				curRenderer := cr.Load().(*renderer)
				if newFractal.kind == curRenderer.fractal.kind &&
				   newFractal.name == curRenderer.fractal.name {
					app.PrimaryWindow().Show()
					return
				}
				// instantiate and initialize a new renderer with the new fractal and the current theme
				newRenderer := newRenderer(newFractal, shaderCode[newFractal.kind], curRenderer.theme)
				newRenderer.init(app, curRenderer.theme)
				oldRenderer := cr.Swap(newRenderer).(*renderer)
				oldRenderer.release()
				scheduleMenuRebuild() // rebuild the "Themes" menu using the new renderer
				app.SetTitle(newFractal.titleText)
				app.RequestRedraw()
				app.PrimaryWindow().Show()
			}})
		}
		fractalsMenu.AddItem(gogpu.MenuItem{
			Title:   cases.Title(language.English).String(kind),
			Submenu: fractalMenu,
		})
	}

	// TODO(jbunds): determine what causes the animation to pause when the user clicks on any menu header
	// TODO(jbunds): determine what causes the native "Window" menu to be removed from the menu bar
	app.SetCustomMenu("fractals", fractalsMenu)
}

// addWindowMenu adds a standard "Window" menu.
func addWindowMenu(app *gogpu.App) {
	// TODO(jbunds): fix this so a "Window" menu is actually added to the menu bar.
	if winMenu := app.GetSystemMenu(gogpu.SystemMenuWindow); winMenu != nil {
		winMenu.AddItem(gogpu.MenuItem{Title: "Minimize",           Role: gogpu.RoleMinimize})
		winMenu.AddItem(gogpu.MenuItem{Title: "Zoom",               Role: gogpu.RoleZoom})
		winMenu.AddItem(gogpu.MenuItem{Separator: true})
		winMenu.AddItem(gogpu.MenuItem{Title: "Enter Full Screen",  Role: gogpu.RoleFullScreen})
		winMenu.AddItem(gogpu.MenuItem{Title: "Show / Hide All",    Role: gogpu.RoleShowAll})
		winMenu.AddItem(gogpu.MenuItem{Separator: true})
		winMenu.AddItem(gogpu.MenuItem{Title: "Bring All to Front", Role: gogpu.RoleBringAllToFront})
		winMenu.AddItem(gogpu.MenuItem{Title: "Close",              Role: gogpu.RoleClose})
	}
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

	cc.SetColor(gg.Transparent)
	cc.DrawRectangle(0, 0, aboutWidth, aboutHeight)
	cc.Fill()

	cc.SetColor(gg.RGB(1, 0.9725, 0.8627)) // approximates "cornsilk"
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
