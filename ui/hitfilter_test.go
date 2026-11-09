package ui

import (
	"fmt"
	"slices"
	"testing"
)

// TestHitFilterMatches hit-tests a grid of points over elements that are
// transformed in 2D and 3D, clipped square, round and inset, scrolled,
// stuck and positioned, and over text: what contains tells by what showed
// of an element first is what it tells by its box alone.
func TestHitFilterMatches(t *testing.T) {
	var scroll ScrollState
	scroll.Y = 70
	view := func(c *context) {
		coreRow(c).Gap(10).Padding(10).Children(func() {
			coreColumn(c).Gap(12).Width(220).Children(func() {
				coreButton(c, "Turned").Rotate(20)
				coreButton(c, "Scaled").Scale(1.4, 0.7)
				coreBox(c).Size(120, 50).Background(RGB(200, 200, 0)).Perspective(300).RotateY(50)
				coreBox(c).Size(120, 60).Radius(30).Clip().Children(func() {
					coreButton(c, "Round clip").Translate(-10, -6)
				})
				coreBox(c).Size(140, 50).ClipInset(6, 12, 6, 12, 10).Children(func() {
					coreButton(c, "Inset").Width(140)
				})
				coreBox(c).Size(160, 60).Children(func() {
					coreButton(c, "Absolute").Absolute().Top(20).Left(30).ZIndex(2)
					coreButton(c, "Under").Absolute().Top(10).Left(10)
				})
			})
			coreScroll(c).TrackScroll(&scroll).Size(240, 300).Children(func() {
				coreText(c, "Header").Sticky().Top(0).Background(RGB(240, 240, 240))
				for i := range 30 {
					coreRow(c).Gap(6).Padding(3).Children(func() {
						coreText(c, fmt.Sprintf("Item %d with words that wrap", i)).Grow(1)
						coreButton(c, "Go").Rotate(float32(i) * 3)
					})
				}
			})
		})
	}
	tt := coreNewTester(view, 600, 420)
	tt.Frame()
	tt.Frame()
	tested, hits := 0, 0
	for y := float32(-3); y < 425; y += 1.5 {
		for x := float32(-3); x < 605; x += 1.75 {
			tt.rt.noHitFilter = false
			filtered := tt.rt.hitChain(x, y)
			tt.rt.noHitFilter = true
			all := tt.rt.hitChain(x, y)
			if !slices.Equal(filtered, all) {
				t.Fatalf("at %v,%v the filtered hit test finds %x, the whole one %x", x, y, filtered, all)
			}
			tested++
			if len(all) > 0 {
				hits++
			}
		}
	}
	if hits < tested/2 {
		t.Fatalf("%d of %d points hit an element", hits, tested)
	}
}
