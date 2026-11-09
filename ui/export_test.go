package ui

import "github.com/egoist/mygo/internal/platform"

// HostMove moves the pointer to (x, y) as a window's surface does: the
// frame the move asks for, if any, comes as the surface's next, which
// paints the last frame's elements again when that is all it needs, where
// Move builds a frame anew whatever the move asked for.
func (t *Tester) HostMove(x, y float32) {
	t.hostEvent(platform.SurfaceEvent{Kind: platform.PointerMove, X: float64(x), Y: float64(y)})
}

// Paints returns how many times the Tester's window painted.
func (t *Tester) Paints() int { return int(t.rt.paints) }
