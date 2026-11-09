package raster

import (
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// A sidebar-sized gradient, mixed in Oklab and in sRGB.
func BenchmarkLinearStopsOklab(b *testing.B) {
	s := &scene.Scene{Width: 460, Height: 900}
	s.Stops = []scene.Stop{
		{Color: scene.Color{R: 30, G: 60, B: 120, A: 255}, At: 0},
		{Color: scene.Color{R: 180, G: 60, B: 200, A: 255}, At: 0.5},
		{Color: scene.Color{R: 250, G: 200, B: 80, A: 255}, At: 1},
	}
	s.Ops = []scene.Op{{Kind: scene.OpFill, Rect: scene.Rect{W: 460, H: 900}, Paint: scene.PaintLinearStopsOklab,
		Gradient: [4]float32{0, 0, 0, 900}, Stops: [2]int32{0, 3}}}
	img := NewImage(460, 900)
	b.ReportAllocs()
	for b.Loop() {
		Render(img, s)
	}
}

func BenchmarkLinearStopsSRGB(b *testing.B) {
	s := &scene.Scene{Width: 460, Height: 900}
	s.Stops = []scene.Stop{
		{Color: scene.Color{R: 30, G: 60, B: 120, A: 255}, At: 0},
		{Color: scene.Color{R: 180, G: 60, B: 200, A: 255}, At: 0.5},
		{Color: scene.Color{R: 250, G: 200, B: 80, A: 255}, At: 1},
	}
	s.Ops = []scene.Op{{Kind: scene.OpFill, Rect: scene.Rect{W: 460, H: 900}, Paint: scene.PaintLinearStops,
		Gradient: [4]float32{0, 0, 0, 900}, Stops: [2]int32{0, 3}}}
	img := NewImage(460, 900)
	b.ReportAllocs()
	for b.Loop() {
		Render(img, s)
	}
}
