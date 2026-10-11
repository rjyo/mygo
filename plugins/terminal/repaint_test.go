package terminal

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Output asks for a frame painting the view again, not one building the
// window's, and a view out of sight asks for none once the frame it asked
// for did not paint it.
func TestOutputRepaints(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	shown := true
	tt := ui.NewTester(func(c *ui.Context) {
		if shown {
			View(c, term).Fill().AutoFocus()
		} else {
			ui.Text(c, "elsewhere")
		}
	}, 400, 200)
	var builds, repaints int
	draw, repaint := func() { builds++ }, func() { repaints++ }
	term.draw.Store(&draw)
	term.repaint.Store(&repaint)
	want := func(when string, b, r int) {
		t.Helper()
		if builds != b || repaints != r {
			t.Fatalf("%s: %d frames built, %d painted again; want %d, %d", when, builds, repaints, b, r)
		}
	}

	term.Feed([]byte("a"))
	want("output", 0, 1)
	term.Feed([]byte("b"))
	want("output before a frame", 0, 1)
	tt.Frame()
	term.Feed([]byte("c"))
	want("output after a frame", 0, 2)

	// An input method composing at the cursor follows it.
	tt.Compose("ni", 2)
	tt.Frame()
	term.Feed([]byte("d"))
	want("output while composing", 1, 2)
	tt.Compose("", 0)
	tt.Frame()

	shown = false
	tt.Frame()
	term.Feed([]byte("e"))
	term.Feed([]byte("f"))
	tt.Frame()
	term.Feed([]byte("g"))
	// Only the first output, as the frame it asks for may still show the
	// view.
	want("output out of sight", 1, 3)
	shown = true
	tt.Frame()
	term.Feed([]byte("h"))
	want("output shown again", 1, 4)
}

// The cursor blinks by painting the view again, not by building the
// window's.
func TestBlinkRepaints(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 200)
	built, repainted := make(chan struct{}, 8), make(chan struct{}, 8)
	draw := func() { built <- struct{}{} }
	repaint := func() { repainted <- struct{}{} }
	term.Feed([]byte("\x1b[1 q")) // a blinking block
	tt.Frame()
	term.draw.Store(&draw)
	term.repaint.Store(&repaint)
	tt.Frame()
	select {
	case <-repainted:
	case <-built:
		t.Fatal("the blink built the window")
	case <-time.After(2 * blinkPeriod):
		t.Fatal("the cursor did not blink")
	}
}
