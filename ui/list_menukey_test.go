package ui

import (
	"slices"
	"testing"
)

// The menu key opens the context menu of the row chosen in the focused
// list, below the row, as a right-click on it opens it at the pointer.
func TestMenuKeyOpensChosenRowsMenu(t *testing.T) {
	for _, multi := range []bool{false, true} {
		var s ListState
		sel := -1
		var set Selection[int]
		if multi {
			s.Selection = &set
		} else {
			s.Selected = &sel
		}
		menuRow := -1
		tt := NewTester(func(c *Context) {
			List(c, &s, 6, func(i int) {
				Textf(c, "Row %d", i).ContextMenu(func(m *Menu) {
					menuRow = i
					m.Item("Delete")
				})
			}).Grow(1).ContextMenu(func(m *Menu) {
				menuRow = -2
				m.Item("New")
			})
		}, 300, 300)
		tt.Frame()
		tt.Click("Row 2")
		tt.Key(0, KeyDown)
		tt.Key(0, KeyContextMenu)
		if got := tt.Menu(); !slices.Equal(got, []string{"Delete"}) || menuRow != 3 {
			t.Fatalf("multi %v: the menu key opened %q for row %d, want Row 3's", multi, got, menuRow)
		}
		r, _ := tt.Find("Row 3")
		if at := tt.h.menuAt; at[0] < r.X-1 || at[0] > r.X+1 || at[1] < r.Y+r.H-1 || at[1] > r.Y+r.H+8 {
			t.Errorf("multi %v: the menu at %v, not below Row 3 at %+v", multi, at, r)
		}
		tt.CloseMenu()
		tt.Key(Shift, KeyF10)
		if err := tt.ChooseMenuItem("Delete"); err != nil || menuRow != 3 {
			t.Fatalf("multi %v: Shift+F10: %v, row %d", multi, err, menuRow)
		}
	}
}

// Without a row chosen, the menu key opens the list's own menu.
func TestMenuKeyWithoutChosenRow(t *testing.T) {
	sel := -1
	s := ListState{Selected: &sel}
	tt := NewTester(func(c *Context) {
		List(c, &s, 6, func(i int) {
			Textf(c, "Row %d", i).ContextMenu(func(m *Menu) { m.Item("Delete") })
		}).Grow(1).ContextMenu(func(m *Menu) { m.Item("New") })
	}, 300, 300)
	tt.Frame()
	tt.Key(0, KeyTab)
	tt.Key(0, KeyContextMenu)
	if got := tt.Menu(); !slices.Equal(got, []string{"New"}) {
		t.Fatalf("the menu key opened %q, want the list's", got)
	}
}

// In a Table, the menu key opens the menu of the chosen row's cells.
func TestMenuKeyOpensChosenTableRowsMenu(t *testing.T) {
	sel := -1
	s := ListState{Selected: &sel}
	cols := []TableColumn{{ID: "name", Title: "Name"}, {ID: "size", Title: "Size"}}
	menuRow := -1
	tt := NewTester(func(c *Context) {
		Table(c, &s, cols, 4, func(row, col int) {
			Textf(c, "r%dc%d", row, col).ContextMenu(func(m *Menu) {
				menuRow = row
				m.Item("Open")
			})
		}).Grow(1)
	}, 400, 300)
	tt.Frame()
	tt.Click("r1c0")
	tt.Key(0, KeyContextMenu)
	if got := tt.Menu(); !slices.Equal(got, []string{"Open"}) || menuRow != 1 {
		t.Fatalf("the menu key opened %q for row %d, want r1's", got, menuRow)
	}
}
