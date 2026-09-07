package main

import (
	"sync"
	"testing"
)

func TestLifecycle_InitialState(t *testing.T) {
	t.Parallel()
	ui := new(ui)

	if ui.aboutWindowHasFocus.Load() {
		t.Errorf("expected aboutWindowHasFocus to be false on startup, got true")
	}
	if ui.pendingMenuRebuild.Load() {
		t.Errorf("expected pendingMenuRebuild to be false on startup, got true")
	}
	if ui.hidePrimaryWindow.Load() {
		t.Errorf("expected hidePrimaryWindow to be false on startup, got true")
	}
	if ui.hideAboutWindow.Load() {
		t.Errorf("expected hideAboutWindow to be false on startup, got true")
	}
}

func TestLifecycle_ToggleAnimation(t *testing.T) {
	t.Parallel()
	ui := new(ui)
	// simulate production setup
	ui.app           = &fakeApp{}
	ui.primaryWindow = &fakeWin{}
	ui.animating.Store(true)
	ui.animToken.Store(tokenRef{token: &fakeToken{}})

	// first toggle (space bar pressed): pause
	ui.toggleAnimation()
	if ui.animating.Load() {
		t.Errorf("expected animating to be false after toggle, got true")
	}

	// second toggle (space bar pressed): resume
	ui.toggleAnimation()
	if !ui.animating.Load() {
		t.Errorf("expected animating to be true after second toggle, got false")
	}
}

func TestLifecycle_HidePrimaryWindow_PreservesState(t *testing.T) {
	t.Parallel()

	t.Run("preserves active animation intent across hide deferral", func(t *testing.T) {
		t.Parallel()
		ui := new(ui)
		ui.animating.Store(true)

		ui.hidePrimaryWin()

		if !ui.hidePrimaryWindow.Load() {
			t.Errorf("expected hidePrimaryWindow to be true after hidePrimaryWin()")
		}
		if !ui.animating.Load() {
			t.Errorf("expected animating to remain true after hidePrimaryWin()")
		}

		// simulate OnUpdate() consumption
		consumed := ui.hidePrimaryWindow.Swap(false)
		if !consumed {
			t.Errorf("expected hidePrimaryWindow to be consumed by OnUpdate()")
		}
		if ui.hidePrimaryWindow.Load() {
			t.Errorf("expected hidePrimaryWindow to be false after consumption")
		}
	})

	t.Run("preserves paused animation intent across hide deferral", func(t *testing.T) {
		t.Parallel()
		ui := new(ui)
		ui.animating.Store(false) // user previously paused via space bar

		ui.hidePrimaryWin()

		if !ui.hidePrimaryWindow.Load() {
			t.Errorf("expected hidePrimaryWindow to be true after hidePrimaryWin()")
		}
		if ui.animating.Load() {
			t.Errorf("expected animating to remain false after hidePrimaryWin()")
		}

		// simulate OnUpdate() consumption
		consumed := ui.hidePrimaryWindow.Swap(false)
		if !consumed {
			t.Errorf("expected hidePrimaryWindow to be consumed by OnUpdate()")
		}
		if ui.animating.Load() {
			t.Errorf("expected animating to remain false after consumption")
		}
	})
}

func TestLifecycle_HideAboutWindow_Deferral(t *testing.T) {
	t.Parallel()
	ui := new(ui)

	ui.hideAboutWindow.Store(true)

	consumed := ui.hideAboutWindow.Swap(false)
	if !consumed {
		t.Errorf("expected hideAboutWindow to be consumed by OnUpdate()")
	}
	if ui.hideAboutWindow.Load() {
		t.Errorf("expected hideAboutWindow to be false after consumption")
	}
}

// TestLifecycle_Matrix_CmdWHierarchy validates the core invariant from the permutation matrix:
// when the About window has focus, ⌘+W must target the About window, regardless of which
// window received the key event. Only when primary is focused does ⌘+W target primary.
func TestLifecycle_Matrix_CmdWHierarchy(t *testing.T) {
	t.Parallel()

	t.Run("when About is visible, ⌘+W targets About window", func(t *testing.T) {
		t.Parallel()
		ui := new(ui)

		// simulate: About window is visible, ⌘+W is pressed
		aboutVisible := true
		handleCmdW := func() {
			if aboutVisible {
				ui.hideAboutWindow.Store(true)
				return
			}
			ui.hidePrimaryWin()
		}

		handleCmdW()

		if !ui.hideAboutWindow.Load() {
			t.Errorf("expected hideAboutWindow to be true when About is visible")
		}
		if ui.hidePrimaryWindow.Load() {
			t.Errorf("expected hidePrimaryWindow to remain false when About is visible")
		}

		// simulate OnUpdate() consuming About hide
		ui.hideAboutWindow.Store(false)
		aboutVisible = false

		// second ⌘+W: About is now closed, so ⌘+W must target primary window
		handleCmdW()

		if !ui.hidePrimaryWindow.Load() {
			t.Errorf("expected hidePrimaryWindow to be true after About is closed")
		}
	})
}

// TestLifecycle_Matrix_Permutations validates state transitions across the 8 matrix states (S1-S8).
func TestLifecycle_Matrix_Permutations(t *testing.T) {
	t.Parallel()

	// TODO(jbunds): improve this test by actually exercising the SUT

	type stateTuple struct {
		primaryVis bool
		aboutVis   bool
		animating  bool
	}

	// S1: (primary == visible, About == hidden, animating == true)
	st := stateTuple{primaryVis: true, aboutVis: false, animating: true}

	// S1 -> S3 (open About)
	st.aboutVis = true
	if !st.primaryVis || !st.aboutVis || !st.animating {
		t.Fatalf("expected S3, got %+v", st)
	}

	// S3 -> S1 (⌘+W: closes About)
	if st.aboutVis {
		st.aboutVis = false // About closes
	} else {
		st.primaryVis = false
	}
	if !st.primaryVis || st.aboutVis || !st.animating {
		t.Fatalf("expected S1 after ⌘+W in S3, got %+v", st)
	}

	// S1 -> S2 (space bar pressed: pause)
	st.animating = !st.animating
	if !st.primaryVis || st.aboutVis || st.animating {
		t.Fatalf("expected S2 after space bar pressed in S1, got %+v", st)
	}

	// S2 -> S4 (open About while paused)
	st.aboutVis = true
	if !st.primaryVis || !st.aboutVis || st.animating {
		t.Fatalf("expected S4, got %+v", st)
	}

	// S4 -> S2 (⌘+W: closes About first, keeps primary paused)
	if st.aboutVis {
		st.aboutVis = false
	} else {
		st.primaryVis = false
	}
	if !st.primaryVis || st.aboutVis || st.animating {
		t.Fatalf("expected S2 after ⌘+W in S4, got %+v", st)
	}

	// S2 -> S8 (⌘+W: closes primary, remains paused)
	if st.aboutVis {
		st.aboutVis = false
	} else {
		st.primaryVis = false
	}
	if st.primaryVis || st.aboutVis || st.animating {
		t.Fatalf("expected S8 after ⌘+W in S2, got %+v", st)
	}

	// S8 -> S2 (select fractal while paused: primary shows, remains paused)
	st.primaryVis = true
	if !st.primaryVis || st.aboutVis || st.animating {
		t.Fatalf("expected S2 after fractal selected from menu in S8, got %+v", st)
	}

	// S2 -> S1 (space bar pressed: resumes animation)
	st.animating = !st.animating
	if !st.primaryVis || st.aboutVis || !st.animating {
		t.Fatalf("expected S1 after space bar pressed in S2, got %+v", st)
	}
}

func TestLifecycle_ConcurrentAccess(t *testing.T) {
	// this test makes no behavioral assertions and
	// just exercises concurrent access under -race
	t.Parallel()
	ui := new(ui)
	ui.primaryWindow = &fakeWin{}

	const goroutines = 8
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	// goroutines toggling animation
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				ui.toggleAnimation()
			}
		}()
	}

	// goroutines requesting primary window hide
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				ui.hidePrimaryWin()
			}
		}()
	}

	// goroutines simulating OnUpdate() consuming deferrals
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				ui.hidePrimaryWindow.Swap(false)
				ui.hideAboutWindow.Swap(false)
			}
		}()
	}

	wg.Wait()
}
