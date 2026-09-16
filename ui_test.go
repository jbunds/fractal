package main

import (
	"slices"
	"testing"

	"github.com/gogpu/gogpu"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func TestSetAppMenu(t *testing.T) {
	t.Parallel()
	ui            := new(ui)
	fakeApp       := &fakeApp{}
	fakeWin       := &fakeWin{}
	ui.app         = fakeApp
	ui.aboutWindow = fakeWin

	setAppMenu(ui)

	got                := fakeApp.appMenu
	gotTitlesAndRoles  := menuTitlesAndRoles(t, got.Items)
	wantTitlesAndRoles := []*titleAndRole{
		{title: "About Fractal", role: gogpu.RoleAbout},
		{title: "",              role: gogpu.RoleNone},
		{title:"Settings…",      role: gogpu.RolePreferences},
		{title: "",              role: gogpu.RoleNone},
		{title:"Services",       role: gogpu.RoleServices},
		{title: "",              role: gogpu.RoleNone},
		{title:"Hide Fractal",   role: gogpu.RoleHide},
		{title:"Hide Others",    role: gogpu.RoleHideOthers},
		{title:"Show All",       role: gogpu.RoleShowAll},
		{title: "",              role: gogpu.RoleNone},
		{title:"Quit Fractal",   role: gogpu.RoleQuit},
	}

	if diff := cmp.Diff(wantTitlesAndRoles, gotTitlesAndRoles, cmpOpts()); diff != "" {
		t.Errorf("setAppMenu() mismatch (-want +got):\n%s", diff)
	}

	firstItem := got.Items[0]
	firstItem.Action() // calls ui.aboutWindow.Show() and ui.aboutWindowHasFocus.Store(true)

	if !fakeWin.showCalled {
		t.Errorf("Show() not called")
	}
	if !ui.aboutWindow.Visible() {
		t.Errorf("expected ui.aboutWindow.Visible() to be true, got %t\n", ui.aboutWindow.Visible())
	}
	if !ui.aboutWindowHasFocus.Load() {
		t.Errorf("expected ui.aboutWindowHasFocus.Load() to be true, got %t\n", ui.aboutWindowHasFocus.Load())
	}
	if ui.hideAboutWindow.Load() {
		t.Errorf("expected ui.hideAboutWindow.Load() to be false, got %t\n", ui.hideAboutWindow.Load())
	}

	ui.aboutWindow.Close() // calls ui.hideAboutWindow.Store(true) via SetOnClose()

	if !ui.hideAboutWindow.Load() {
		t.Errorf("expected ui.hideAboutWindow.Load() to be tru, got %t\n", ui.hideAboutWindow.Load())
	}

	quitItem := got.Items[len(got.Items) - 1]
	quitItem.Action()

	if !fakeApp.quitCalled {
		t.Errorf("Quit() not called")
	}
}

func TestAddFractalsMenu(t *testing.T) {
	t.Parallel()
	ui      := new(ui)
	fakeApp := &fakeApp{}
	ui.app   = fakeApp

	addFractalsMenu(t.Context(), ui, map[string]string{})

	got                := fakeApp.customMenus["fractals"].Items
	gotTitlesAndRoles  := menuTitlesAndRoles(t, got)
	wantTitlesAndRoles := []*titleAndRole{
		{title: "Julia",      role: gogpu.RoleNone},
		{title: "Mandelbrot", role: gogpu.RoleNone},
	}

	if diff := cmp.Diff(wantTitlesAndRoles, gotTitlesAndRoles, cmpOpts()); diff != "" {
		t.Errorf("addFractalsMenu() mismatch (-want +got):\n%s", diff)
	}

	gotSubmenuTitles := make(map[string][]string)
	for _, mi := range got {
		for _, sm := range mi.Submenu.Items {
			gotSubmenuTitles[mi.Title] = append(gotSubmenuTitles[mi.Title], sm.Title)
		}
	}

	wantSubmenuTitles       := make(map[string][]string)
	fractals                := fractals()
	labels, sortedMenuItems := labels(fractals)
	for _, kind := range kinds(fractals) {
		for _, name := range sortedMenuItems[kind] {
			titleCasedTitle := cases.Title(language.English).String(kind)
			wantSubmenuTitles[titleCasedTitle] = append(wantSubmenuTitles[titleCasedTitle], labels[kind][name])
		}
	}

	if diff := cmp.Diff(wantSubmenuTitles, gotSubmenuTitles); diff != "" {
		t.Errorf("addFractalsMenu() mismatch (-want +got):\n%s", diff)
	}
}

func TestRebuildThemesMenu(t *testing.T) {
	t.Parallel()
	ui      := new(ui)
	fakeApp := &fakeApp{}
	ui.app   = fakeApp

	rebuildThemesMenu(t.Context(), ui, map[string]string{})

	gotTitlesAndRoles  := menuTitlesAndRoles(t, fakeApp.customMenus["themes"].Items)
	wantTitlesAndRoles := []*titleAndRole{
		{title: "green", role: gogpu.RoleNone},
		{title: "red",   role: gogpu.RoleNone},
	}

	if diff := cmp.Diff(wantTitlesAndRoles, gotTitlesAndRoles, cmpOpts()); diff != "" {
		t.Errorf("rebuildThemesMenu() mismatch (-want +got):\n%s", diff)
	}
}

func TestAddWindowMenu(t *testing.T) {
	t.Parallel()
	fakeApp := &fakeApp{
		appMenuHandle: &fakeSystemMenuHandle{},
	}

	addWindowMenu(fakeApp)

	got := fakeApp.appMenuHandle.(*fakeSystemMenuHandle)

	if len(got.items) != 8 {
		t.Errorf("expected 8 submenus, got %d\n", len(got.items))
	}

	gotTitlesAndRoles  := menuTitlesAndRoles(t, got.items)
	wantTitlesAndRoles := []*titleAndRole{
		{title: "Minimize",           role: gogpu.RoleMinimize},
		{title: "Zoom",               role: gogpu.RoleZoom},
		{title: "",                   role: gogpu.RoleNone},
		{title: "Enter Full Screen",  role: gogpu.RoleFullScreen},
		{title: "Show / Hide All",    role: gogpu.RoleShowAll},
		{title: "",                   role: gogpu.RoleNone},
		{title: "Bring All to Front", role: gogpu.RoleBringAllToFront},
		{title: "Close",              role: gogpu.RoleClose},
	}

	if diff := cmp.Diff(wantTitlesAndRoles, gotTitlesAndRoles, cmpOpts()); diff != "" {
		t.Errorf("addWindowMenu() mismatch (-want +got):\n%s", diff)
	}
}

type titleAndRole struct {
	title string
	role  gogpu.MenuRole
}

func menuTitlesAndRoles(t *testing.T, items []gogpu.MenuItem) []*titleAndRole {
	t.Helper()
	return slices.Collect(func(yield func(*titleAndRole) bool) {
		for _, mi := range items {
			if !yield(&titleAndRole{title: mi.Title, role: mi.Role}) {
				return
			}
		}
	})
}
