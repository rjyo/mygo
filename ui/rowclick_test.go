package ui

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// A cell that reads its row's double click with RowDoubleClicked leaves
// the click to the row, which chooses as a click, Shift-click and
// Cmd-click do.
func TestCellSeesRowDoubleClick(t *testing.T) {
	var s ListState
	var sel Selection[int]
	s.Selection = &sel
	cols := []TableColumn{{Title: "A"}, {Title: "B"}}
	doubleCol, doubles := -1, 0
	tt := NewTester(func(c *Context) {
		Table(c, &s, cols, 6, func(row, col int) {
			cell := Box(c).FillWidth().Children(func() { Textf(c, "r%dc%d", row, col) })
			if cell.RowDoubleClicked() {
				doubleCol, doubles = col, doubles+1
			}
		}).Grow(1)
	}, 400, 300)
	tt.Frame()
	tt.Click("r1c1")
	if got := chosenRows(&sel); !slices.Equal(got, []int{1}) {
		t.Fatalf("click r1c1: chose %v", got)
	}
	tt.ClickWith(Shift, "r3c0")
	if got := chosenRows(&sel); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("Shift-click r3c0: chose %v", got)
	}
	tt.ClickWith(Cmd, "r5c1")
	if got := chosenRows(&sel); !slices.Equal(got, []int{1, 2, 3, 5}) {
		t.Fatalf("Cmd-click r5c1: chose %v", got)
	}
	// The keys extend from the row clicked last.
	tt.Click("r0c0")
	tt.Key(Shift, KeyDown)
	if got := chosenRows(&sel); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("click r0c0, Shift+Down: chose %v", got)
	}
	r, _ := tt.Find("r4c1")
	tt.ClickAt(r.X+2, r.Y+2)
	tt.ClickAt(r.X+2, r.Y+2)
	tt.Frame()
	if doubles != 1 || doubleCol != 1 || !slices.Equal(chosenRows(&sel), []int{4}) {
		t.Fatalf("double click r4c1: the cell saw %d, in column %d; chose %v", doubles, doubleCol, chosenRows(&sel))
	}
}

// A row whose double click opens a text input keeps the input focused: the
// row's click does not take the focus back to the list.
func TestRowDoubleClickEditorKeepsFocus(t *testing.T) {
	var s ListState
	sel := -1
	s.Selected = &sel
	editing, text := -1, "x"
	var focused bool
	tt := NewTester(func(c *Context) {
		List(c, &s, 4, func(i int) {
			if i == editing {
				in := TextInput(c, &text).AutoFocus()
				focused = in.Focused()
				return
			}
			if Box(c).FillWidth().Children(func() { Textf(c, "Row %d", i) }).RowDoubleClicked() {
				editing = i
			}
		}).Grow(1)
	}, 300, 300)
	tt.Frame()
	r, _ := tt.Find("Row 2")
	tt.ClickAt(r.X+2, r.Y+2)
	tt.ClickAt(r.X+2, r.Y+2)
	tt.Frame()
	tt.Frame()
	if editing != 2 || !focused {
		t.Fatalf("double-clicked Row 2: editing %d, input focused %v", editing, focused)
	}
}

// RowClicked reports each click on a row, on the row chosen too, and
// leaves the click to the row, which chooses: Tester.Click on the row's
// Label reaches it. The keys moving the choice report none, nor does a
// Cmd-click extending it, and a double click reports its first click,
// its second going to RowDoubleClicked.
func TestRowClicked(t *testing.T) {
	var s ListState
	var sel Selection[int]
	s.Selection = &sel
	s.Label = func(i int) string { return fmt.Sprintf("File %d", i) }
	var clicked, doubled []int
	tt := NewTester(func(c *Context) {
		List(c, &s, 5, func(i int) {
			name := Box(c).FillWidth().Padding(6, 12).Children(func() { Textf(c, "Row %d", i) })
			if name.RowClicked() {
				clicked = append(clicked, i)
			}
			if name.RowDoubleClicked() {
				doubled = append(doubled, i)
			}
		}).Grow(1)
	}, 300, 300)
	tt.Frame()
	// Clicks far enough apart in time not to make a double click.
	click := func(label string, mods Modifiers) {
		t.Helper()
		tt.Advance(time.Second)
		if err := tt.ClickWith(mods, label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	click("File 1", 0)
	if !slices.Equal(clicked, []int{1}) || !slices.Equal(chosenRows(&sel), []int{1}) {
		t.Fatalf("clicked File 1: RowClicked %v, chose %v", clicked, chosenRows(&sel))
	}
	click("File 1", 0)
	if !slices.Equal(clicked, []int{1, 1}) || len(doubled) != 0 {
		t.Fatalf("clicked File 1 again, chosen: RowClicked %v, RowDoubleClicked %v", clicked, doubled)
	}
	tt.Key(0, KeyDown)
	tt.Key(0, KeyDown)
	tt.Frame()
	if !slices.Equal(clicked, []int{1, 1}) || !slices.Equal(chosenRows(&sel), []int{3}) {
		t.Fatalf("Down, Down: RowClicked %v, chose %v", clicked, chosenRows(&sel))
	}
	click("File 0", Cmd)
	if !slices.Equal(clicked, []int{1, 1}) || !slices.Equal(chosenRows(&sel), []int{0, 3}) {
		t.Fatalf("Cmd-clicked File 0: RowClicked %v, chose %v", clicked, chosenRows(&sel))
	}
	clicked = nil
	tt.Advance(time.Second)
	r, _ := tt.Find("Row 4")
	tt.ClickAt(r.X+2, r.Y+2)
	tt.Frame()
	tt.ClickAt(r.X+2, r.Y+2)
	tt.Frame()
	if !slices.Equal(clicked, []int{4}) || !slices.Equal(doubled, []int{4}) {
		t.Fatalf("double-clicked Row 4: RowClicked %v, RowDoubleClicked %v", clicked, doubled)
	}
}

// The second click of a double click coming before a frame is no
// RowClicked either.
func TestRowClickedDoubleInOneFrame(t *testing.T) {
	sel := -1
	s := ListState{Selected: &sel}
	clicked, doubled := 0, 0
	tt := NewTester(func(c *Context) {
		List(c, &s, 3, func(i int) {
			row := Textf(c, "Row %d", i).FillWidth()
			if row.RowClicked() {
				clicked++
			}
			if row.RowDoubleClicked() {
				doubled++
			}
		}).Grow(1)
	}, 300, 300)
	tt.Frame()
	r, _ := tt.Find("Row 1")
	tt.ClickAt(r.X+2, r.Y+2)
	tt.ClickAt(r.X+2, r.Y+2)
	tt.Frame()
	if clicked != 1 || doubled != 1 || sel != 1 {
		t.Fatalf("double-clicked Row 1 within a frame: %d RowClicked, %d RowDoubleClicked, chose %d", clicked, doubled, sel)
	}
}

// In a list that does not choose its rows, RowClicked makes the row take
// the click, which Tester.Click on its Label reaches.
func TestRowClickedWithoutChoosing(t *testing.T) {
	var s ListState
	s.Label = func(i int) string { return fmt.Sprintf("Item %d", i) }
	opened := -1
	tt := NewTester(func(c *Context) {
		List(c, &s, 4, func(i int) {
			if Textf(c, "Row %d", i).Padding(6, 12).FillWidth().RowClicked() {
				opened = i
			}
		}).Grow(1)
	}, 300, 300)
	tt.Frame()
	if err := tt.Click("Item 2"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if opened != 2 {
		t.Fatalf("clicked Item 2: opened %d", opened)
	}
}

// In a Table, the cell under the pointer reports the click; assistive
// technology pressing the row reports it once, to the first cell asking,
// and focusing the row chooses it without a click.
func TestRowClickedTableAndAccess(t *testing.T) {
	sel := -1
	s := ListState{Selected: &sel}
	s.Label = func(i int) string { return fmt.Sprintf("Line %d", i) }
	cols := []TableColumn{{Title: "A"}, {Title: "B"}}
	type hit struct{ row, col int }
	var hits []hit
	tt := NewTester(func(c *Context) {
		Table(c, &s, cols, 4, func(row, col int) {
			if Box(c).FillWidth().Children(func() { Textf(c, "r%dc%d", row, col) }).RowClicked() {
				hits = append(hits, hit{row, col})
			}
		}).Grow(1)
	}, 400, 300)
	tt.Frame()
	if err := tt.Click("r2c1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !slices.Equal(hits, []hit{{2, 1}}) || sel != 2 {
		t.Fatalf("clicked r2c1: RowClicked %v, chose %d", hits, sel)
	}
	hits = nil
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	row := accessNode(t, tt.h.access, platform.RoleRow, "Line 3")
	tt.send(platform.SurfaceEvent{Kind: platform.AccessAction, ID: row.ID, Action: platform.AccessFocus})
	tt.Frame()
	if len(hits) != 0 || sel != 3 {
		t.Fatalf("focused Line 3 from assistive technology: RowClicked %v, chose %d", hits, sel)
	}
	row = accessNode(t, tt.h.access, platform.RoleRow, "Line 1")
	tt.send(platform.SurfaceEvent{Kind: platform.AccessAction, ID: row.ID, Action: platform.AccessPress})
	tt.Frame()
	if !slices.Equal(hits, []hit{{1, 0}}) || sel != 1 {
		t.Fatalf("pressed Line 1 from assistive technology: RowClicked %v, chose %d", hits, sel)
	}
}
