//go:build !js

package e2e

import (
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TestWindowHiddenBuildsAsItHides checks the promise of
// Context.WindowHidden: a view that asks builds once as its window hides,
// out of sight, and again as it shows. A frame waiting for the display
// would never come: a minimized or hidden window's display link stops on
// macOS, and Windows sends a minimized window no WM_PAINT. Linux and
// Windows tell only that a window is minimized, and Xvfb has no window
// manager to minimize one.
func TestWindowHiddenBuildsAsItHides(t *testing.T) {
	type state struct {
		name string
		// do hides the window, and returns what shows it again.
		do func(w *mygo.Window) (undo func())
	}
	var states []state
	if runtime.GOOS == "darwin" {
		states = append(states, state{"hidden", func(w *mygo.Window) func() {
			w.Hide()
			return w.Show
		}})
	}
	if runtime.GOOS != "linux" {
		states = append(states, state{"minimized", func(w *mygo.Window) func() {
			w.Minimize()
			return w.Restore
		}})
	}
	if len(states) == 0 {
		t.Skip("no window manager to minimize windows")
	}
	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			var builtHidden, builtShown, lastHidden atomic.Bool
			var builds atomic.Int64
			w := newWindow(t, mygo.WindowOptions{Title: "WindowHidden", Width: 320, Height: 240, Content: ui.View(func(c *ui.Context) {
				hidden := c.WindowHidden()
				lastHidden.Store(hidden)
				builds.Add(1)
				if hidden {
					builtHidden.Store(true)
				} else {
					builtShown.Store(true)
				}
				ui.Text(c, "WindowHidden")
			})})
			// In front, the window shows: one behind a full screen app's
			// would be out of sight from the start. A window may also be
			// out of sight a moment before it first shows, so the view
			// builds anew until it sees it shown.
			w.Focus()
			eventually(t, "the window focused", w.IsFocused)
			asked := int64(-1)
			eventually(t, "a build that sees the window shown", func() bool {
				n := builds.Load()
				if asked >= 0 && n > asked && !lastHidden.Load() {
					return true
				}
				asked = n
				w.Update(func() {})
				return false
			})
			builtHidden.Store(false)
			undo := s.do(w)
			eventually(t, "a build as the window hides", builtHidden.Load)
			builtShown.Store(false)
			undo()
			eventually(t, "a build as the window shows again", builtShown.Load)
		})
	}
}
