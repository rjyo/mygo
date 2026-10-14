package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/scene"
)

// spinGPU is a GPU renderer that turns spinners itself while animates is
// set, and keeps the kinds of the last frame's ops.
type spinGPU struct {
	testGPU
	animates bool
	kinds    []scene.Kind
	spinner  scene.Op
	onOp     func(scene.Op)
}

func (g *spinGPU) AnimatesSpinners() bool { return g.animates }

func (g *spinGPU) Render(s *scene.Scene) error {
	g.kinds = g.kinds[:0]
	for _, op := range s.Ops {
		g.kinds = append(g.kinds, op.Kind)
		if op.Kind == scene.OpSpinner {
			g.spinner = op
		}
		if g.onOp != nil {
			g.onOp(op)
		}
	}
	return g.testGPU.Render(s)
}

func (g *spinGPU) has(k scene.Kind) bool {
	for _, x := range g.kinds {
		if x == k {
			return true
		}
	}
	return false
}

// spinHost returns a window host on a test surface drawing view with g,
// and a function drawing a frame.
func spinHost(t *testing.T, g *spinGPU, view func(c *context)) (*windowHost, func()) {
	h, _, frame := gpuHost(t, func() (gpuRenderer, error) { return g, nil })
	h.rt = newRuntime(view, h)
	return h, frame
}

func TestSpinnerLeftToTheRenderer(t *testing.T) {
	g := &spinGPU{animates: true}
	h, frame := spinHost(t, g, func(c *context) {
		coreBox(c).Size(20, 20).Draw(func(p *Painter, r Rect) {
			p.Spinner(r, 2, RGB(255, 0, 0), 30, 90, time.Second)
		})
	})
	// The first frame paints before the window has its renderer, and asks
	// for the next.
	frame()
	frame()
	if !g.has(scene.OpSpinner) {
		t.Fatalf("the frame's ops are %v, with no spinner", g.kinds)
	}
	if s := g.spinner; s.Rect != (scene.Rect{W: 20, H: 20}) || s.Border[0] != 2 || s.Gradient != [4]float32{30, 90, 1} {
		t.Errorf("the spinner is %+v", s)
	}
	if rt := h.rt; rt.animating || rt.repainting || !rt.repaintAt.IsZero() {
		t.Error("a window turning only spinners asks for frames")
	}

	// Without the renderer turning it, it paints its arc and asks for the
	// next frame.
	g.animates = false
	h.rt.requestFrame()
	frame()
	if g.has(scene.OpSpinner) || !h.rt.repainting {
		t.Errorf("drawing its own frames, the window painted %v, asking for the next: %v", g.kinds, h.rt.repainting)
	}
}

// TestSpinnerPaintedWholeWhenTheRendererStops checks that a frame painting
// only what moves paints the window whole once the renderer stops turning
// spinners, so that those it turned turn again as arcs.
func TestSpinnerPaintedWholeWhenTheRendererStops(t *testing.T) {
	g := &spinGPU{animates: true}
	_, frame := spinHost(t, g, func(c *context) {
		coreBox(c).Size(20, 20).Draw(func(p *Painter, r Rect) {
			p.Spinner(r, 2, RGB(255, 0, 0), 0, 90, time.Second)
		})
		coreBox(c).Size(20, 20).Draw(func(p *Painter, r Rect) {
			p.AnimationFrame()
			p.Fill(r, RGB(0, 0, 255), 0)
		})
	})
	frame()
	frame()
	if !g.has(scene.OpSpinner) {
		t.Fatalf("repainting, the frame's ops are %v, with no spinner", g.kinds)
	}
	g.animates = false
	frame()
	if g.has(scene.OpSpinner) || !g.has(scene.OpGlyphs) {
		t.Errorf("after the renderer stopped, the frame's ops are %v", g.kinds)
	}
}

func TestPulseLeftToTheRenderer(t *testing.T) {
	g := &spinGPU{animates: true}
	var pulse scene.Op
	h, frame := spinHost(t, g, func(c *context) {
		coreBox(c).Size(8, 8).Draw(func(p *Painter, r Rect) {
			p.Pulse(r, 4, RGB(0, 0, 255), 0.35, 1600*time.Millisecond, 2000*time.Millisecond)
		})
	})
	g.onOp = func(op scene.Op) {
		if op.Kind == scene.OpPulse {
			pulse = op
		}
	}
	frame()
	frame()
	if pulse.Kind != scene.OpPulse || pulse.Radii[0] != 4 || pulse.Gradient != [4]float32{0.4, 0.35, 1.6} {
		t.Fatalf("the pulse is %+v, in %v", pulse, g.kinds)
	}
	if rt := h.rt; rt.animating || rt.repainting || !rt.repaintAt.IsZero() {
		t.Error("a window fading only pulses asks for frames")
	}
	g.animates = false
	h.rt.requestFrame()
	frame()
	// The test surface takes no posts: the frame it asks for stays asked.
	h.repaintTimer(time.Time{})
	if g.has(scene.OpPulse) || h.rt.repaintAt.IsZero() {
		t.Errorf("drawing its own frames, the window painted %v, asking for the next at %v", g.kinds, h.rt.repaintAt)
	}
}
