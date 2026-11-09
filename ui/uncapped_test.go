package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/pacing"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

// uncapped has frames uncapped (MYGO_UNCAPPED) for the rest of the test.
func uncapped(t *testing.T) {
	before := pacing.Uncapped
	pacing.Uncapped = true
	t.Cleanup(func() { pacing.Uncapped = before })
}

// askingSurface is a paced test surface that counts the frames asked for.
type askingSurface struct {
	refreshSurface
	asked int
}

func (s *askingSurface) RequestFrame() { s.asked++ }

// TestUncappedFramesInMemoryWaitForNoRefresh checks that, uncapped, a paced
// frame drawn in memory on a surface that shows such frames at once, as
// Windows' does, draws at once, rather than wait for the display's next
// refresh as TestFramesInMemoryWaitForTheRefresh's do: nothing waits for
// the display, off the main thread or on it.
func TestUncappedFramesInMemoryWaitForNoRefresh(t *testing.T) {
	uncapped(t)
	t.Setenv("MYGO_GPU", "0")
	s := &refreshSurface{refresh: make(chan struct{})}
	h := &windowHost{conn: &surface.Conn{Surface: s}}
	h.rt = newRuntime(func(c *context) { coreText(c, "Hello") }, h)
	s.h = h
	posted := 0
	h.conn.Post = func(func()) { posted++ }
	h.event(platform.SurfaceEvent{Kind: platform.SurfaceFrame})
	for i := range 3 {
		h.event(platform.SurfaceEvent{Kind: platform.SurfaceFrame, Paced: true})
		if s.pixels != i+2 || h.waiting || h.refresh != nil || posted != 0 {
			t.Fatalf("paced frame %d: %d frames, waiting %v, refresh awaited %v, %d posted", i, s.pixels, h.waiting, h.refresh != nil, posted)
		}
	}
}

// TestUncappedFramesComeAtOnce checks that, uncapped, every surface's
// frames count as coming as soon as asked for (framesAtOnce), so that a
// drawing asking for fewer frames (Painter.AnimationFrameRate) keeps its
// pace with the repaint timer rather than a frame each time one is asked
// for, as TestAnimationFrameRateAtOnce checks.
func TestUncappedFramesComeAtOnce(t *testing.T) {
	h := &windowHost{conn: &surface.Conn{Surface: &testSurface{}}}
	if h.framesAtOnce() {
		t.Fatal("a surface paced by the display has frames at once")
	}
	uncapped(t)
	if !h.framesAtOnce() {
		t.Fatal("uncapped, frames do not come at once")
	}
}

// TestUncappedIdleAsksNoFrame checks that uncapped frames never come while
// nothing changes: a frame asks for the next only while something moves,
// once each frame, and none once it stops, so that an idle window draws
// nothing, uncapped or not.
func TestUncappedIdleAsksNoFrame(t *testing.T) {
	uncapped(t)
	t.Setenv("MYGO_GPU", "0")
	moving := true
	s := &askingSurface{}
	h := &windowHost{conn: &surface.Conn{Surface: s}}
	h.rt = newRuntime(func(c *context) {
		coreBox(c).Size(100, 20).Draw(func(p *Painter, r Rect) {
			if moving {
				p.AnimationFrame()
			}
			p.Fill(Rect{r.X, r.Y, 2, r.H}, RGB(0, 0, 255), 0)
		})
	}, h)
	s.h = h
	frame := func() { h.event(platform.SurfaceEvent{Kind: platform.SurfaceFrame, Paced: true}) }
	frame()
	for i := range 5 {
		if s.asked != i+1 {
			t.Fatalf("frame %d of an animation asked for %d frames, want %d", i, s.asked, i+1)
		}
		frame()
	}
	moving = false
	asked := s.asked
	frame() // the frame the last asked for, which moves nothing
	frame() // as a resize's or a late tick's
	if s.asked != asked {
		t.Fatalf("frames once nothing moves asked for %d more", s.asked-asked)
	}
}

// TestUncappedFrameRate checks the line MYGO_FRAME_STATS logs uncapped:
// how many frames came a second, once they followed each other for a
// second, counted anew after a pause or a frame as long.
func TestUncappedFrameRate(t *testing.T) {
	var logged []string
	defer func(f func(...any)) { frameLog = f }(frameLog)
	frameLog = func(v ...any) { logged = append(logged, v[0].(string)) }
	f := newFrameStats(time.Hour)
	at := time.Unix(1000, 0)
	// 2 ms frames, back to back: 500 a second.
	for range 499 {
		f.rate(at, at.Add(2*time.Millisecond))
		at = at.Add(2 * time.Millisecond)
	}
	if len(logged) != 0 {
		t.Fatalf("logged before a second: %q", logged)
	}
	f.rate(at, at.Add(2*time.Millisecond))
	at = at.Add(2 * time.Millisecond)
	if len(logged) != 1 || !strings.Contains(logged[0], "500 frames a second, uncapped: 2.00 ms a frame") {
		t.Fatalf("after a second: %q", logged)
	}
	// An idle second counts for nothing: the count starts anew.
	at = at.Add(time.Second)
	for range 99 {
		f.rate(at, at.Add(10*time.Millisecond))
		at = at.Add(10 * time.Millisecond)
	}
	if len(logged) != 1 {
		t.Fatalf("the pause counted: %q", logged)
	}
	f.rate(at, at.Add(10*time.Millisecond))
	at = at.Add(10 * time.Millisecond)
	if len(logged) != 2 || !strings.Contains(logged[1], "100 frames a second") {
		t.Fatalf("after the pause: %q", logged)
	}
	// A frame that took 2 s, as a present that waited on the system, is a
	// pause too: the second it ends in counts the frames after it only,
	// not a few frames a second.
	for range 50 {
		f.rate(at, at.Add(10*time.Millisecond))
		at = at.Add(10 * time.Millisecond)
	}
	f.rate(at, at.Add(2*time.Second))
	at = at.Add(2 * time.Second)
	if len(logged) != 2 {
		t.Fatalf("the long frame counted: %q", logged)
	}
	for range 1000 {
		f.rate(at, at.Add(time.Millisecond))
		at = at.Add(time.Millisecond)
	}
	if len(logged) != 3 || !strings.Contains(logged[2], "1000 frames a second") {
		t.Fatalf("after the long frame: %q", logged)
	}
}
