package terminal

import (
	"fmt"
	"image"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// rowGlyphs returns where each row's first glyph is, nil for a row without
// text: a row made again has its glyphs elsewhere.
func rowGlyphs(term *Terminal) []*ui.Glyph {
	term.mu.Lock()
	defer term.mu.Unlock()
	var out []*ui.Glyph
	for _, l := range term.v.lines {
		var g *ui.Glyph
		if len(l.runs) > 0 && len(l.runs[0].glyphs) > 0 {
			g = &l.runs[0].glyphs[0]
		}
		out = append(out, g)
	}
	return out
}

// Rows a program scrolls are taken from the last frame, not made again:
// a log printing a line at a time makes one row a frame.
func TestScrolledRowsKept(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 200)
	for i := range 30 {
		term.Feed(fmt.Appendf(nil, "\x1b[3%dmline %d\x1b[0m \x1b[1mbold\x1b[0m\r\n", i%7+1, i))
	}
	tt.Frame()
	for step := range 3 {
		before := rowGlyphs(term)
		term.Feed(fmt.Appendf(nil, "\x1b[32mline %d\x1b[0m\r\n", 30+step))
		tt.Frame()
		after := rowGlyphs(term)
		rows := len(after)
		if rows < 4 || len(before) != rows {
			t.Fatalf("%d rows, then %d", len(before), rows)
		}
		// Each row shows what the row below it did, but the last two: the
		// new line and the cursor's empty row.
		for y := range rows - 2 {
			if after[y] == nil || after[y] != before[y+1] {
				t.Errorf("step %d: row %d was made again rather than taken from row %d", step, y, y+1)
			}
		}
		if g := after[rows-2]; g == nil || slices.Contains(before, g) {
			t.Errorf("step %d: the new line's row was not made", step)
		}
	}
}

// Whatever rows a frame takes from the last, it draws what making every
// row again draws: rows rewritten in place with the same text but other
// colors, styles, backgrounds or combining marks, rows moved by
// scrolling, inserting and scroll regions, under a selection, a search
// and a palette that changes.
func TestTakenRowsDrawAsMade(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 500, 300)
	check := func(step string) {
		t.Helper()
		tt.Frame()
		got := clone(tt.Image())
		// Without the last frame's rows, every row is made again.
		term.mu.Lock()
		term.v.lines = nil
		term.mu.Unlock()
		tt.Frame()
		if want := tt.Image(); !slices.Equal(got.Pix, want.Pix) {
			t.Errorf("%s: the frame differs from one whose rows were all made again", step)
		}
	}
	// A steady cursor: a blinking one shows by the clock once a click
	// focuses the terminal.
	term.Feed([]byte("\x1b[2 q"))
	for i := range 20 {
		term.Feed(fmt.Appendf(nil, "\x1b[3%d;4%dm%02d\x1b[0m same \x1b[1mbold\x1b[0m \x1b[4munder\x1b[0m 漢字 👍🏽 café │─\r\n", i%7+1, (i+3)%8, i%3))
	}
	check("filled")
	term.Feed([]byte("\x1b[32mone more\x1b[0m\r\n"))
	check("scrolled a line")
	// The same text in place, but otherwise.
	for _, s := range []string{
		"\x1b[3;1H\x1b[35m00\x1b[0m same",
		"\x1b[3;1H\x1b[35;1m00\x1b[0m same",
		"\x1b[3;1H\x1b[35;3m00\x1b[0m same",
		"\x1b[3;1H\x1b[35;4:3m00\x1b[0m same",
		"\x1b[3;1H\x1b[35;9m00\x1b[0m same",
		"\x1b[3;1H\x1b[35;7m00\x1b[0m same",
		"\x1b[3;1H\x1b[38;5;200m00\x1b[0m same",
		"\x1b[3;1H\x1b[38;2;10;200;30m00\x1b[0m same",
		"\x1b[4;1H\x1b[44m\x1b[K\x1b[0m",
		"\x1b[4;1H\x1b[45m\x1b[K\x1b[0m",
		"\x1b[4;1H\x1b[48;2;1;2;3m\x1b[K\x1b[0m",
		"\x1b[5;1Hcafé\x1b[K",
		"\x1b[5;1Hcafè\x1b[K",
		"\x1b[5;1Hcafe\x1b[K",
		"\x1b[5;1H漢字\x1b[K",
		"\x1b[5;1Hab字\x1b[K",
		"\x1b[5;1Hab\x1b[8mcd\x1b[0m\x1b[K",
		"\x1b[5;1Habcd\x1b[K",
	} {
		term.Feed([]byte(s))
		check(fmt.Sprintf("rewrote %q", s))
	}
	// A row rewritten to show what the row below it, which stays as it
	// is, shows: it takes that row's last frame, and the row below keeps
	// its own.
	term.Feed([]byte("\x1b[3;1Hfirst\x1b[K\x1b[4;1Hsecond\x1b[K\x1b[10;1H"))
	check("wrote two rows")
	term.Feed([]byte("\x1b[10;1Hx\x1b[10;1H"))
	tt.Frame()
	term.Feed([]byte("\x1b[3;1Hthird\x1b[K\x1b[10;1H"))
	tt.Frame() // the frame before, whose rows the next one's slots hold
	term.Feed([]byte("\x1b[3;1Hsecond\x1b[K\x1b[10;1H"))
	check("copied the row below")
	// Rows that show the same, many times over, then one of them not.
	term.Feed([]byte("\x1b[30;1H\r\nsame\r\nsame\r\nsame\r\n"))
	check("repeated a row")
	term.Feed([]byte("\x1b[2A\x1b[33msame\x1b[0m\x1b[2B\r\nsame\r\n"))
	check("recolored a repeated row")
	// Rows moved by inserting, deleting and scrolling a region.
	term.Feed([]byte("\x1b[3;1H\x1b[2L"))
	check("inserted rows")
	term.Feed([]byte("\x1b[2;1H\x1b[M"))
	check("deleted a row")
	term.Feed([]byte("\x1b[4;9r\x1b[9;1H\nregion\n\x1b[r"))
	check("scrolled a region")
	// A selection, which moves with its text.
	x0, y0 := cellCenter(term, 1, 2)
	x1, y1 := cellCenter(term, 12, 5)
	tt.Press(x0, y0)
	tt.Move(x1, y1)
	tt.Release(x1, y1)
	check("selected")
	term.Feed([]byte("\x1b[30;1H\r\nmore\r\n"))
	check("scrolled the selection")
	tt.ClickAt(x0, y0)
	check("unselected")
	term.Search("same")
	check("searched")
	term.Feed([]byte("\x1b]4;2;rgb:ff/00/00\x07\x1b]4;44;rgb:00/ff/00\x07"))
	check("changed the palette")
	term.Feed([]byte("\x1b[2J\x1b[H\x1b[31mafter\x1b[0m clearing\r\n"))
	check("cleared")
}

func clone(m *image.RGBA) *image.RGBA {
	c := *m
	c.Pix = slices.Clone(m.Pix)
	return &c
}
