package ui_test

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// focusRows builds 50 rows in a scroll view 200 DIPs high, at the top of
// the window, calling row for each.
func focusRows(row func(c *ui.Context, i int, b ui.Element)) func(c *ui.Context) {
	return func(c *ui.Context) {
		ui.Scroll(c.Key("list")).Height(200).Children(func() {
			for i := range 50 {
				row(c, i, ui.Button(c.Key(i), fmt.Sprintf("Row %d", i)))
			}
		})
	}
}

// inView reports whether the element showing s lies inside the 200 DIP
// view, and its box.
func inView(t *testing.T, tt *ui.Tester, s string) (ui.Rect, bool) {
	t.Helper()
	r, ok := tt.Find(s)
	if !ok {
		t.Fatalf("%s is not built", s)
	}
	return r, r.Y >= 0 && r.Y+r.H <= 200
}

// The app moving the focus to an element scrolls it into view, as little
// as shows it, as Tab does and the web's focus() does (#1628): by Focus,
// by a field bound with FocusBind, and by a Handle.
func TestFocusScrollsTheElementIntoView(t *testing.T) {
	target := -1
	var bound int // the row's number plus one; zero for none
	var handle ui.Handle
	ways := map[string]struct {
		row   func(c *ui.Context, i int, b ui.Element)
		focus func()
	}{
		"Focus": {
			func(c *ui.Context, i int, b ui.Element) {
				if i == target {
					b.Focus()
				}
			},
			func() { target = 40 },
		},
		"FocusBind": {
			func(c *ui.Context, i int, b ui.Element) { b.FocusBind(&bound, i+1) },
			func() { bound = 41 },
		},
		"Handle": {
			func(c *ui.Context, i int, b ui.Element) {
				if i == 40 {
					b.Bind(&handle)
				}
			},
			func() { handle.Focus() },
		},
	}
	// Where Tab brings Row 40: as little as shows the button.
	tab := ui.NewTester(focusRows(func(*ui.Context, int, ui.Element) {}), 400, 300)
	for range 41 {
		tab.Key(0, ui.KeyTab)
	}
	tabbed, in := inView(t, tab, "Row 40")
	if !tab.Focused("Row 40") || !in {
		t.Fatalf("Tab to Row 40 left it at %v", tabbed.Y)
	}
	for name, w := range ways {
		t.Run(name, func(t *testing.T) {
			target, bound, handle = -1, 0, ui.Handle{}
			tt := ui.NewTester(focusRows(w.row), 400, 300)
			tt.Frame()
			if r, in := inView(t, tt, "Row 40"); in {
				t.Fatalf("Row 40 shows at the start, at %v", r.Y)
			}
			w.focus()
			tt.Frame()
			tt.Frame()
			r, in := inView(t, tt, "Row 40")
			if !tt.Focused("Row 40") || !in {
				t.Fatalf("focused Row 40 (%v) is at y = %v..%v, out of the 200 DIP view", tt.Focused("Row 40"), r.Y, r.Y+r.H)
			}
			if r.Y != tabbed.Y {
				t.Errorf("Row 40 is at y = %v, where Tab brings it to %v", r.Y, tabbed.Y)
			}
		})
	}
}

// Focus called in every frame scrolls the element into view once, as the
// focus moves to it, and then leaves the user's scrolling alone.
func TestFocusEveryFrameKeepsTheUserScroll(t *testing.T) {
	target := -1
	var s ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c.Key("list")).Height(200).TrackScroll(&s).Children(func() {
			for i := range 50 {
				b := ui.Button(c.Key(i), fmt.Sprintf("Row %d", i))
				if i == target {
					b.Focus()
				}
			}
		})
	}, 400, 300)
	tt.Frame()
	target = 40
	tt.Frame()
	tt.Frame()
	if _, in := inView(t, tt, "Row 40"); !in {
		t.Fatalf("Focus didn't scroll Row 40 into view")
	}
	// The user scrolls back to the top, the focus staying on Row 40.
	tt.Move(100, 100)
	tt.Scroll(100, 100, 0, -s.Y)
	tt.Frame()
	tt.Frame()
	if r, in := inView(t, tt, "Row 40"); in || s.Y != 0 || !tt.Focused("Row 40") {
		t.Errorf("Focus again on the focused Row 40 scrolled the view back to %v (Row 40 at %v)", s.Y, r.Y)
	}
}
