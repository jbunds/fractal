package main

import (
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
)

// the following types and methods provide thin wrappers around the GoGPU
// API to facilitate effective testing of the SUT via test injection.

// app wraps gogpu.App to facilitate testing.
type app interface {
	Quit()
	RequestRedraw()
	SetTitle(string)
	SetMenu(*gogpu.Menu)
	SetCustomMenu(string, *gogpu.Menu)
	Run()                               error
	PrimaryWindow()                     window
	StartAnimation()                    animToken
	OnSurfaceAvailable(func())         *gogpu.App
	OnDraw(func(*gogpu.Context))       *gogpu.App
	OnClose(func())                    *gogpu.App
	OnUpdate(func(float64))            *gogpu.App
	SetQuitOnLastWindowClosed(bool)    *gogpu.App
	NewWindow(gogpu.Config)           (*gogpu.Window, error)
	GetSystemMenu(gogpu.SystemMenu)     systemMenuHandle
	DeviceProvider()                    gogpu.DeviceProvider
	EventSource()                       gpucontext.EventSource
	GPUContextProvider()                gpucontext.DeviceProvider
}

// wraps gogpu.App.
type gogpuApp struct {
	*gogpu.App
}

func (a *gogpuApp) StartAnimation()                  animToken        { return a.App.StartAnimation() }
func (a *gogpuApp) PrimaryWindow()                   window           { return a.App.PrimaryWindow()  }
func (a *gogpuApp) SetMenu(m *gogpu.Menu)                             {        a.App.SetMenu(m)       }
func (a *gogpuApp) GetSystemMenu(m gogpu.SystemMenu) systemMenuHandle { return a.App.GetSystemMenu(m) }

// wraps gogpu.Window.
type window interface {
	Visible() bool
	Show()
	Hide()
	Close()
	SetOnDraw(func(_ *gogpu.Context))
	SetOnKeyPress(func(gpucontext.Key, gpucontext.Modifiers))
	SetOnPointer(func(gpucontext.PointerEvent))
	SetOnClose(func() bool)
}

// animToken wraps gogpu.AnimationToken.
type animToken interface {
	Stop()
}

// thin shim to satisfy atomic.Value requiring a non-nil interface.
type tokenRef struct {
	token animToken
}

// fakes for testing

type fakeApp struct {
	*gogpuApp

	animating        bool
	animToken        animToken
	appMenu          *gogpu.Menu
	customMenus      map[string]*gogpu.Menu
	appMenuHandle    systemMenuHandle
	reqRedrawCalled,
	quitCalled       bool
}

func (f *fakeApp) Quit()                                             { f.quitCalled      = true }
func (f *fakeApp) RequestRedraw()                                    { f.reqRedrawCalled = true }
func (f *fakeApp) StartAnimation()                  animToken        { return f.animToken       }
func (f *fakeApp) GetSystemMenu(_ gogpu.SystemMenu) systemMenuHandle { return f.appMenuHandle   }

func (f *fakeApp) SetMenu(m *gogpu.Menu) {
	f.appMenu       = m
	f.appMenuHandle = &fakeSystemMenuHandle{}
}

func (f *fakeApp) SetCustomMenu(s string, m *gogpu.Menu) {
	if f.customMenus == nil {
		f.customMenus = make(map[string]*gogpu.Menu)
	}
	f.customMenus[s] = m
}

type fakeWin struct {
	visible,
	hasFocus,
	showCalled,
	hideCalled  bool
	onDraw      func(*gogpu.Context)
	onClose     func() bool
	onKeyPress  func(gpucontext.Key, gpucontext.Modifiers)
}

func (f *fakeWin) Show()                                                           { f.visible, f.showCalled, f.hasFocus = true, true, true }
func (f *fakeWin) Hide()                                                           { f.visible, f.hideCalled             = false, true      }
func (f *fakeWin) SetOnDraw(fn func(fn *gogpu.Context))                            { f.onDraw                            = fn               }
func (f *fakeWin) SetOnKeyPress(fn func(_ gpucontext.Key, _ gpucontext.Modifiers)) { f.onKeyPress                        = fn               }
func (f *fakeWin) SetOnPointer(_ func(_ gpucontext.PointerEvent))                  {                                                        }
func (f *fakeWin) SetOnClose(fn func() bool)                                       { f.onClose                           = fn               }
func (f *fakeWin) Close()                                                          { f.onClose()                                            }
func (f *fakeWin) Visible() bool                                                   { return f.visible                                       }

type fakeToken struct {
	stopCalled bool
}

func (f *fakeToken) Stop() {
	f.stopCalled = true
}

type systemMenuHandle interface {
	AddItem(gogpu.MenuItem) bool
}

type fakeSystemMenuHandle struct {
	items []gogpu.MenuItem
}

func (f *fakeSystemMenuHandle) AddItem(mi gogpu.MenuItem) bool {
	f.items = append(f.items, mi)
	return true
}
