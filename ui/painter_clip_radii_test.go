package ui

import (
	"image/color"
	"testing"
)

// ClipRadii rounds each corner by its own radius, clockwise from the top
// left: a square top left and bottom left, round top right and bottom right.
func TestPainterClipRadii(t *testing.T) {
	red := RGB(255, 0, 0)
	tt := coreNewTester(func(c *context) {
		coreBox(c).Size(100, 100).Draw(func(p *Painter, r Rect) {
			p.ClipRadii(r, [4]float32{0, 40, 40, 0}, func() { p.Fill(r, red, 0) })
		})
	}, 100, 100)
	img := tt.Image()
	shown := func(x, y int) bool { return img.RGBAAt(x, y) == (color.RGBA{255, 0, 0, 255}) }
	for _, pt := range [][2]int{{1, 1}, {1, 98}, {50, 50}} {
		if !shown(pt[0], pt[1]) {
			t.Errorf("(%d, %d) is clipped: %v", pt[0], pt[1], img.RGBAAt(pt[0], pt[1]))
		}
	}
	for _, pt := range [][2]int{{98, 1}, {98, 98}} {
		if shown(pt[0], pt[1]) {
			t.Errorf("(%d, %d) shows past the round corner", pt[0], pt[1])
		}
	}
}
