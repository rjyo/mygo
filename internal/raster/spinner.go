package raster

import (
	"math"

	"github.com/egoist/mygo/internal/scene"
)

// spinner paints a spinner (scene.OpSpinner) as it shows at the frame's
// time: its arc, with round caps, still. Only its box's place and size
// follow the transform, as the renderers turning spinners place them.
func (r *renderer) spinner(op *scene.Op) {
	box := op.Transform.Bounds(op.Rect)
	width := op.Border[0]
	if box.Empty() || width <= 0 || op.Gradient[1] <= 0 {
		return
	}
	if sx := box.W / op.Rect.W; op.Transform.Set && sx > 0 {
		width *= sx
	}
	c := op.Color.Premul(opacityOf(op.Opacity))
	hw := width / 2
	cx, cy := box.X+box.W/2, box.Y+box.H/2
	radius := (min(box.W, box.H) - width) / 2
	from := float64(op.Gradient[0]) * math.Pi / 180
	sweep := float64(min(op.Gradient[1], 360)) * math.Pi / 180
	// The ends of the arc, which its caps round.
	ex0, ey0 := cx+radius*float32(math.Cos(from)), cy+radius*float32(math.Sin(from))
	ex1, ey1 := cx+radius*float32(math.Cos(from+sweep)), cy+radius*float32(math.Sin(from+sweep))
	x0, y0, x1, y1 := r.pixelBounds(box)
	for y := y0; y < y1; y++ {
		row := r.dst.Pix[y*r.dst.Stride:]
		py := float32(y) + 0.5
		for x := x0; x < x1; x++ {
			px := float32(x) + 0.5
			dx, dy := px-cx, py-cy
			// The distance to the arc: to the circle where the pixel's
			// angle is within the sweep, else to the nearer end.
			a := math.Atan2(float64(dy), float64(dx)) - from
			a -= 2 * math.Pi * math.Floor(a/(2*math.Pi))
			var d float32
			if a <= sweep {
				d = float32(math.Abs(math.Hypot(float64(dx), float64(dy)) - float64(radius)))
			} else {
				d = min(hypot(px-ex0, py-ey0), hypot(px-ex1, py-ey1))
			}
			cov := clamp01(hw - d + 0.5)
			if cov == 0 {
				continue
			}
			if cov *= r.clipCoverage(x, y); cov > 0 {
				blend(row[4*x:4*x+4], c, cov)
			}
		}
	}
}

func hypot(x, y float32) float32 {
	return float32(math.Hypot(float64(x), float64(y)))
}
