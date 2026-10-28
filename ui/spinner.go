package ui

import (
	"math"
	"time"

	"github.com/egoist/mygo/internal/scene"
)

// Spinner paints an arc of the circle in r, width DIPs wide, in c: sweep
// degrees long, from degrees clockwise from 3 o'clock at Now, turning
// clockwise once a period. Where the window's renderer animates layers
// (Metal's, on macOS), the system turns it between frames, which draw
// nothing for it: a window whose only motion is spinners draws no frames.
// Elsewhere it paints the arc and asks for the next frame
// (AnimationFrame).
func (p *Painter) Spinner(r Rect, width float32, c Color, from, sweep float32, period time.Duration) {
	if r.W <= 0 || r.H <= 0 || !p.visible(r, width) {
		return
	}
	if p.rt.animatesSpinners() {
		op := scene.Op{Kind: scene.OpSpinner, Rect: scene.Rect{X: r.X * p.scale, Y: r.Y * p.scale, W: r.W * p.scale, H: r.H * p.scale},
			Color: c.scene(), Opacity: p.opacity, Gradient: [4]float32{from, sweep, float32(period.Seconds())}}
		op.Border[0] = width * p.scale
		p.add(op)
		return
	}
	p.AnimationFrame()
	p.StrokePath(arcPath(r.X+r.W/2, r.Y+r.H/2, (min(r.W, r.H)-width)/2, from, sweep), width, c)
}

// Pulse paints the rectangle r with corners radius DIPs round in c,
// fading to low of its opacity and back, easing in and out each way, once
// a period, at Now at into that. Where the window's renderer animates
// layers, as for Spinner, the system fades it between frames, which draw
// nothing for it. Elsewhere it paints it as at Now and asks for a frame in
// a 48th of the period.
func (p *Painter) Pulse(r Rect, radius float32, c Color, low float32, period, at time.Duration) {
	if r.W <= 0 || r.H <= 0 || period <= 0 || !p.visible(r, 0) {
		return
	}
	if p.rt.animatesSpinners() {
		op := scene.Op{Kind: scene.OpPulse, Rect: scene.Rect{X: r.X * p.scale, Y: r.Y * p.scale, W: r.W * p.scale, H: r.H * p.scale},
			Radii: scene.Uniform(radius * p.scale), Continuous: continuousCorners, Color: c.scene(), Opacity: p.opacity,
			Gradient: [4]float32{float32((at % period).Seconds()), low, float32(period.Seconds())}}
		p.add(op)
		return
	}
	p.After(period / 48)
	t := float32(at%period) / float32(period)
	p.Fill(r, c.Alpha(scene.PulseOpacity(t, low)), radius)
}

// animatesSpinners reports whether the window's renderer turns spinners
// itself (scene.OpSpinner).
func (rt *engine) animatesSpinners() bool {
	a, ok := rt.host.(interface{ animatesSpinners() bool })
	return ok && a.animatesSpinners()
}

// arcPath is the arc of a circle around (cx, cy), from degrees clockwise
// from 3 o'clock, sweeping as far.
func arcPath(cx, cy, radius, from, sweep float32) *Path {
	path := new(Path)
	steps := max(4, int(sweep/6))
	for i := 0; i <= steps; i++ {
		rad := float64(from+sweep*float32(i)/float32(steps)) * math.Pi / 180
		x, y := cx+radius*float32(math.Cos(rad)), cy+radius*float32(math.Sin(rad))
		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	return path
}
