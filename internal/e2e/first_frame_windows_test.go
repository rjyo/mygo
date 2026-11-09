//go:build windows && (amd64 || arm64)

package e2e

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	win "github.com/egoist/mygo/internal/windows"
	"github.com/egoist/mygo/ui"
)

// cloaked reports whether DWM keeps w out of sight, and whether its first
// frame reached its surface.
func cloaked(w *mygo.Window) (cloaked, drawn bool) {
	mygo.RunOnMain(func() { cloaked, drawn = win.TestCloaked(w.NativeHandle()) })
	return
}

// TestContentWindowCloakedUntilFirstFrame checks that a window of native
// UI stays out of sight, though shown, until its first frame shows in it:
// it showed white until then, as the GPU renderer started (#905).
func TestContentWindowCloakedUntilFirstFrame(t *testing.T) {
	var w atomic.Pointer[mygo.Window]
	// atBuild is how the first build found the window: 1 cloaked, 2 not,
	// 3 not found. The build may run while NewWindow creates the window,
	// before w is set (at 200%, say), so it looks for windows not drawn yet.
	var atBuild atomic.Int32
	var ready atomic.Bool
	mygo.RunOnMain(func() {
		nw := newWindow(t, mygo.WindowOptions{Width: 320, Height: 240, Content: ui.View(func(c *ui.Context) {
			c.Root().Background(ui.RGB(30, 30, 30))
			if atBuild.Load() != 0 {
				return
			}
			switch n, cloaked := win.TestUndrawnCloaked(); {
			case n == 0:
				atBuild.Store(3)
			case cloaked:
				atBuild.Store(1)
			default:
				atBuild.Store(2)
			}
		})})
		w.Store(nw)
		nw.OnReadyToShow(func() { ready.Store(true) })
	})
	eventually(t, "ready to show", ready.Load)
	switch atBuild.Load() {
	case 2:
		t.Fatal("shown before its first frame: not cloaked as it drew it")
	case 3:
		t.Fatal("no window of native UI waiting for its first frame as it built")
	}
	eventually(t, "the window uncloaked after its first frame", func() bool {
		c, drawn := cloaked(w.Load())
		return !c && drawn
	})
	if !w.Load().IsVisible() {
		t.Error("not visible")
	}
}

// firstFrameRevealed reports whether w, whose first frame was drawn, is
// out of DWM's cloak: shown, it showed its frame.
func firstFrameRevealed(w *mygo.Window) bool {
	c, drawn := cloaked(w)
	return !c && drawn
}

// TestShownFlyoutCloakedUntilComposed checks that a flyout of native UI
// made hidden and shown before it drew a frame, whose frames show through
// DirectComposition, stays out of sight after the WM_PAINT that drew its
// first frame until DWM composed it: uncloaked at once, it showed black for
// a refresh or more, as the frame reached DWM after the window (#1591).
func TestShownFlyoutCloakedUntilComposed(t *testing.T) {
	work := mygo.Screen.PrimaryDisplay().WorkArea
	parent := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, X: work.X + 100, Y: work.Y + 100})
	var fl *mygo.Window
	var release func()
	var composed bool
	mygo.RunOnMain(func() {
		release = win.TestHoldReveal()
		fl = mygo.NewFlyout(mygo.FlyoutOptions{Parent: parent, Hidden: true, Shadow: true, Width: 220, Height: 240, Content: ui.View(func(c *ui.Context) {
			c.Root().Background(ui.RGB(30, 30, 30))
			for i := range 10 {
				ui.Text(c, "Row "+strconv.Itoa(i))
			}
		})})
		fl.Show()
		composed = win.TestSurfaceComposed(fl.NativeHandle())
	})
	released := false
	unhold := func() {
		if !released {
			released = true
			mygo.RunOnMain(release)
		}
	}
	defer unhold()
	defer fl.Destroy()
	eventually(t, "the flyout's first frame drawn", func() bool {
		_, drawn := cloaked(fl)
		return drawn
	})
	if !fl.IsVisible() {
		t.Fatal("the flyout is not visible")
	}
	if composed {
		// Some refreshes later, DWM has still not composed it, as held.
		time.Sleep(150 * time.Millisecond)
		if c, _ := cloaked(fl); !c {
			t.Fatal("shown after its first frame's WM_PAINT, before DWM composed the frame")
		}
	}
	unhold()
	eventually(t, "the flyout uncloaked once its first frame was composed", func() bool { return firstFrameRevealed(fl) })
}
