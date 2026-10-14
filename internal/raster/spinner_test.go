package raster

import (
	"math"
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

func spinnerScene(from float32) *scene.Scene {
	s := &scene.Scene{Width: 40, Height: 40}
	op := scene.Op{Kind: scene.OpSpinner, Rect: scene.Rect{W: 40, H: 40}, Color: scene.Color{R: 255, A: 255},
		Gradient: [4]float32{from, 90, 1}}
	op.Border[0] = 4
	s.Ops = append(s.Ops, op)
	return s
}

func TestSpinnerDrawsItsArc(t *testing.T) {
	var img Image
	img.Resize(40, 40)
	Render(&img, spinnerScene(0))
	// Red, premultiplied BGRA, at deg degrees around, 18 pixels out.
	at := func(deg float64, radius float64) [4]byte {
		rad := deg * math.Pi / 180
		x, y := int(20+radius*math.Cos(rad)), int(20+radius*math.Sin(rad))
		i := y*img.Stride + 4*x
		return [4]byte(img.Pix[i : i+4])
	}
	if p := at(45, 18); p[2] < 250 || p[3] < 250 {
		t.Errorf("the middle of the arc is %v, not red", p)
	}
	for _, deg := range []float64{180, 270, 135} {
		if p := at(deg, 18); p[3] != 0 {
			t.Errorf("the circle at %v° is %v, outside the arc", deg, p)
		}
	}
	if p := at(45, 8); p[3] != 0 {
		t.Errorf("inside the ring is %v", p)
	}
}

func TestSpinnerTurningIsNoDamage(t *testing.T) {
	var r Renderer
	r.Render(spinnerScene(0))
	if rects := r.Render(spinnerScene(120)); len(rects) != 0 {
		t.Errorf("a spinner turning damages %v", rects)
	}
	recolored := spinnerScene(0)
	recolored.Ops[0].Color.G = 255
	if rects := r.Render(recolored); len(rects) == 0 {
		t.Error("a spinner changing color damages nothing")
	}
}

func TestPulseFadesAsAtItsTime(t *testing.T) {
	for _, c := range []struct {
		at   float32
		want int
	}{{0, 255}, {0.8, 102}, {1.6, 255}} {
		s := &scene.Scene{Width: 10, Height: 10}
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPulse, Rect: scene.Rect{W: 10, H: 10}, Color: scene.Color{B: 255, A: 255},
			Gradient: [4]float32{c.at, 0.4, 1.6}})
		var img Image
		img.Resize(10, 10)
		Render(&img, s)
		if a := int(img.Pix[5*img.Stride+4*5+3]); a < c.want-1 || a > c.want+1 {
			t.Errorf("%vs into the pulse, its alpha is %d, not %d", c.at, a, c.want)
		}
	}
}

func TestPulseCurve(t *testing.T) {
	for _, c := range []struct{ t, want float32 }{{0, 1}, {0.25, 0.7}, {0.5, 0.4}, {0.75, 0.7}, {1, 1}} {
		if got := scene.PulseOpacity(c.t, 0.4); math.Abs(float64(got-c.want)) > 1e-4 {
			t.Errorf("PulseOpacity(%v) = %v, not %v", c.t, got, c.want)
		}
	}
}
