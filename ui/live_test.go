package ui

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// liveView is a window with text and a drawing that changes on its own
// (Painter.Live), as a terminal's grid as output comes: a bar where *at
// puts it.
func liveView(at *int, draws *int) func(c *context) {
	return func(c *context) {
		coreColumn(c).Padding(4).Gap(2).Background(RGB(250, 250, 250)).Children(func() {
			for i := range 3 {
				coreText(c, fmt.Sprintf("line %d of the text kept painted", i))
			}
			coreBox(c).Size(100, 12).Clip().Draw(func(p *Painter, r Rect) {
				*draws++
				p.Live()
				p.Fill(Rect{r.X + float32(*at*10), r.Y, 8, r.H}, RGB(200, 60, 40), 0)
			})
		})
	}
}

// Services.Repaint paints the drawings that noted Live again without
// building the view, and shows what painting the frame whole does.
func TestRepaintLive(t *testing.T) {
	at, draws := 0, 0
	tt := newRepaintTester(liveView(&at, &draws))
	tt.SetSize(200, 100)
	tt.SetScale(2)
	if _, more := tt.frame(0); more {
		t.Fatal("the frame asked for another")
	}
	for round := range 3 {
		at = round + 1
		n := draws
		tt.rt.repaint()
		if !tt.h.requested.Load() {
			t.Fatalf("round %d: repaint asked for no frame", round)
		}
		if built, more := tt.frame(0); built || more {
			t.Fatalf("round %d: the frame built %v, asked for another %v", round, built, more)
		}
		if draws != n+1 || tt.rt.repaints == 0 {
			t.Fatalf("round %d: %d draws, %d frames painted only what moves", round, draws-n, tt.rt.repaints)
		}
		got := bytes.Clone(tt.Image().Pix)
		tt.rt.retained = false
		tt.frame(0)
		if !bytes.Equal(got, tt.Image().Pix) {
			t.Fatalf("round %d: the frame painting again differs from the frame painted whole", round)
		}
	}
}

// A repaint asked for after something asked for a frame built anew, or
// while the view moves, builds it; one while nothing of the window shows
// waits for it to show.
func TestRepaintLiveBuilds(t *testing.T) {
	at, draws := 0, 0
	tt := newRepaintTester(liveView(&at, &draws))
	tt.SetSize(200, 100)
	for _, c := range []struct {
		name  string
		asked func()
	}{
		{"an event", func() { tt.rt.event(platform.SurfaceEvent{Kind: platform.PointerMove, X: 150, Y: 50}) }},
		{"a change", tt.rt.changed},
		{"the theme", tt.rt.themeChanged},
	} {
		c.asked()
		tt.rt.repaint()
		if tt.rt.redraw {
			t.Fatalf("after %s: the next frame paints again", c.name)
		}
		if built, _ := tt.frame(0); !built {
			t.Fatalf("after %s: the frame did not build the view", c.name)
		}
	}
	tt.h.hidden = true
	tt.h.requested.Store(false)
	tt.rt.repaint()
	if tt.h.requested.Load() || !tt.rt.held {
		t.Fatal("a repaint while nothing shows asked for a frame")
	}
	tt.h.hidden = false
	tt.rt.event(platform.SurfaceEvent{Kind: platform.SurfaceShown})
	if !tt.h.requested.Load() {
		t.Fatal("the window showing asked for no frame")
	}
}

// Services.Repaint posts one repaint until a frame takes it.
func TestRepaintPostsOnce(t *testing.T) {
	at, draws := 0, 0
	tt := newRepaintTester(liveView(&at, &draws))
	s := tt.rt.public.Services()
	s.Repaint()
	if !tt.rt.repaintPosted.Load() {
		t.Fatal("Repaint posted nothing")
	}
	tt.h.requested.Store(false)
	s.Repaint()
	if tt.h.requested.Load() {
		t.Fatal("Repaint posted again before a frame")
	}
	tt.frame(16 * time.Millisecond)
	if tt.rt.repaintPosted.Load() {
		t.Fatal("the frame left the repaint posted")
	}
}

// Frames painting a spinner again leave a drawing that noted Live as it
// is, until Services.Repaint asks for it, and show what painting them
// whole does.
func TestLiveWaitsForRepaint(t *testing.T) {
	at, draws, spins := 0, 0, 0
	live := liveView(&at, &draws)
	tt := newRepaintTester(func(c *context) {
		live(c)
		coreBox(c).Size(20, 20).Draw(func(p *Painter, r Rect) {
			spins++
			p.AnimationFrame()
			p.Fill(Rect{r.X + float32(p.Now().UnixMilli()/16%10), r.Y, 4, 4}, RGB(40, 90, 200), 0)
		})
	})
	tt.SetSize(200, 100)
	tt.frame(16 * time.Millisecond)
	same := func(when string) {
		t.Helper()
		got := bytes.Clone(tt.Image().Pix)
		tt.rt.retained = false
		tt.frame(0)
		if !bytes.Equal(got, tt.Image().Pix) {
			t.Fatalf("%s: the frame painting again differs from the frame painted whole", when)
		}
	}
	for round := range 3 {
		n, s := draws, spins
		for range 3 {
			if built, _ := tt.frame(16 * time.Millisecond); built {
				t.Fatalf("round %d: a spinner frame built the view", round)
			}
		}
		if draws != n || spins != s+3 {
			t.Fatalf("round %d: spinner frames drew the live drawing %d times, the spinner %d", round, draws-n, spins-s)
		}
		same("spinner frames")
		at = round + 1
		n = draws
		tt.rt.repaint()
		tt.frame(16 * time.Millisecond)
		if draws != n+1 {
			t.Fatalf("round %d: a repaint drew the live drawing %d times", round, draws-n)
		}
		same("a repaint")
	}
}
