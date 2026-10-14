//go:build darwin

package metal

import (
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// TestSpinnersHideUnderWhatPaintsOver checks that a spinner's layer shows
// only where nothing painted after it covers its box, as the stop button
// over a loader, and that what painted before, or a shadow, hides none.
func TestSpinnersHideUnderWhatPaintsOver(t *testing.T) {
	box := scene.Rect{X: 10, Y: 10, W: 20, H: 20}
	spin := scene.Op{Kind: scene.OpSpinner, Rect: box, Gradient: [4]float32{0, 90, 1}}
	spin.Border[0] = 2
	fill := func(rc scene.Rect) scene.Op {
		return scene.Op{Kind: scene.OpFill, Rect: rc, Color: scene.Color{A: 255}}
	}
	area := func(rects []scene.Rect) (a float32) {
		for _, rc := range rects {
			a += rc.W * rc.H
		}
		return a
	}
	for _, c := range []struct {
		name  string
		ops   []scene.Op
		shown float32
	}{
		{"alone", []scene.Op{spin}, 400},
		{"over a fill", []scene.Op{fill(box), spin}, 400},
		{"under a square", []scene.Op{spin, fill(scene.Rect{X: 16, Y: 16, W: 8, H: 8})}, 400 - 64},
		{"under a shadow", []scene.Op{spin, {Kind: scene.OpShadow, Rect: box, Color: scene.Color{A: 255}, Blur: 8}}, 400},
		{"under a pane", []scene.Op{spin, fill(scene.Rect{W: 100, H: 100})}, 0},
		{"a pulse under a square", []scene.Op{{Kind: scene.OpPulse, Rect: box, Gradient: [4]float32{0, 0.4, 1.6}}, fill(scene.Rect{X: 16, Y: 16, W: 8, H: 8})}, 400 - 64},
		{"clipped", []scene.Op{{Kind: scene.OpPushClip, Rect: scene.Rect{W: 20, H: 100}}, spin, {Kind: scene.OpPopClip}}, 200},
	} {
		var r Renderer
		s := &scene.Scene{Width: 100, Height: 100, Ops: c.ops}
		got := r.findSpinners(s)
		if len(got) != 1 {
			t.Fatalf("%s: %d spinners", c.name, len(got))
		}
		if a := area(got[0].shown); a != c.shown {
			t.Errorf("%s: %v pixels show, not %v: %v", c.name, a, c.shown, got[0].shown)
		}
		for i, a := range got[0].shown {
			for _, b := range got[0].shown[i+1:] {
				if !a.Intersect(b).Empty() {
					t.Errorf("%s: %v and %v overlap", c.name, a, b)
				}
			}
		}
	}
}
