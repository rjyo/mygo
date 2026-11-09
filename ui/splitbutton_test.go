package ui_test

import (
	"image/color"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// splitApp is a view of a split button between two buttons, counting what
// its segments did.
type splitApp struct {
	saved, savedAs, copied int
	drawn                  bool
	rtl                    bool
	disabled, actionOff    bool
	open                   bool
}

func (a *splitApp) menu(m *ui.Menu) {
	if m.Item("Save As…").Chosen() {
		a.savedAs++
	}
	if m.Item("Save a Copy").Chosen() {
		a.copied++
	}
}

func (a *splitApp) view(c *ui.Context) {
	col := ui.Column(c).Padding(20).Gap(10).AlignItems(ui.Start)
	if a.rtl {
		col.Direction(ui.RightToLeft)
	}
	col.Children(func() {
		ui.Button(c, "Before")
		save := ui.SplitButton(c, "Save")
		save.Group.Label("Saving").Disabled(a.disabled)
		save.Button.Disabled(a.actionOff)
		if a.drawn {
			save.Arrow.PopoverMenuButton(a.menu, ui.PopoverMenuButtonOptions{Open: &a.open})
		} else {
			save.Arrow.Menu(a.menu)
		}
		if save.Button.Clicked() {
			a.saved++
		}
		ui.Button(c, "After")
	})
}

// TestSplitButtonClicksAndMenu checks that the button of a split button
// does its action and its arrow opens the system's menu, whose choice its
// function reports.
func TestSplitButtonClicksAndMenu(t *testing.T) {
	a := &splitApp{}
	tt := ui.NewTester(a.view, 400, 300)
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if a.saved != 1 || len(tt.Menu()) != 0 {
		t.Fatalf("a click on Save: saved %d, menu %q", a.saved, tt.Menu())
	}
	if err := tt.Click("More options"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); !slices.Equal(got, []string{"Save As…", "Save a Copy"}) {
		t.Fatalf("the arrow opened %q", got)
	}
	if a.saved != 1 {
		t.Fatalf("the arrow clicked Save")
	}
	if err := tt.ChooseMenuItem("Save a Copy"); err != nil {
		t.Fatal(err)
	}
	if a.copied != 1 || a.savedAs != 0 || a.saved != 1 {
		t.Fatalf("choosing Save a Copy: copied %d, saved as %d, saved %d", a.copied, a.savedAs, a.saved)
	}
}

// TestSplitButtonKeys checks that a split button is one stop of Tab, that
// the arrows move between its segments, the other way right to left, and
// that the keys click its button and open its menu.
func TestSplitButtonKeys(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		a := &splitApp{rtl: rtl}
		labels := []string{"Before", "Save", "More options", "After"}
		tt := ui.NewTester(a.view, 400, 300)
		if got := splitTabs(tt, 0, 3, labels...); !slices.Equal(got, []string{"Before", "Save", "After"}) {
			t.Fatalf("rtl %v: Tab went %q", rtl, got)
		}
		tt.Key(ui.Shift, ui.KeyTab)
		forth, back := ui.KeyRight, ui.KeyLeft
		if rtl {
			forth, back = back, forth
		}
		var went []string
		for _, k := range []ui.Key{forth, forth, back, ui.KeyEnd, ui.KeyHome} {
			tt.Key(0, k)
			went = append(went, splitFocus(tt, labels...))
		}
		if want := []string{"More options", "Save", "More options", "More options", "Save"}; !slices.Equal(went, want) {
			t.Fatalf("rtl %v: the arrows went %q, not %q", rtl, went, want)
		}
		tt.Key(0, ui.KeyEnter)
		tt.Key(0, ui.KeySpace)
		if a.saved != 2 || len(tt.Menu()) != 0 {
			t.Fatalf("rtl %v: Enter and Space on Save saved %d times, menu %q", rtl, a.saved, tt.Menu())
		}
		tt.Key(ui.Alt, ui.KeyDown)
		if len(tt.Menu()) != 2 || a.saved != 2 {
			t.Fatalf("rtl %v: Alt+Down on Save: menu %q, saved %d", rtl, tt.Menu(), a.saved)
		}
		tt.CloseMenu()
		tt.Key(0, forth)
		tt.Key(0, ui.KeyDown)
		if len(tt.Menu()) != 2 {
			t.Fatalf("rtl %v: Down on the arrow opened %q", rtl, tt.Menu())
		}
		if err := tt.ChooseMenuItem("Save As…"); err != nil {
			t.Fatal(err)
		}
		if a.savedAs != 1 || a.saved != 2 {
			t.Fatalf("rtl %v: saved as %d, saved %d", rtl, a.savedAs, a.saved)
		}
		// Tab leaves it, and comes back to the segment it left.
		if got := splitTabs(tt, 0, 1, labels...); got[0] != "After" {
			t.Fatalf("rtl %v: Tab from the arrow went to %q", rtl, got[0])
		}
		if got := splitTabs(tt, ui.Shift, 1, labels...); got[0] != "More options" {
			t.Fatalf("rtl %v: Shift+Tab came back to %q", rtl, got[0])
		}
	}
}

// TestSplitButtonAccess checks what assistive technology sees: a group
// holding a button and a menu button that opens a menu, expanded while
// the menu drawn in the window shows.
func TestSplitButtonAccess(t *testing.T) {
	a := &splitApp{drawn: true}
	tt := ui.NewTester(a.view, 400, 300)
	group, ok := tt.AccessByRole(ui.RoleGroup, "Saving")
	if !ok {
		t.Fatal("no group")
	}
	button, ok := tt.AccessByRole(ui.RoleButton, "Save")
	if !ok || !button.Focusable {
		t.Fatalf("the button: %+v", button)
	}
	arrow, ok := tt.AccessByRole(ui.RoleMenuButton, "More options")
	if !ok || arrow.HasPopup != ui.PopupMenu || arrow.Expanded || !arrow.Focusable {
		t.Fatalf("the arrow: %+v", arrow)
	}
	for _, r := range []ui.Rect{button.Bounds, arrow.Bounds} {
		if r.X < group.Bounds.X || r.Y < group.Bounds.Y || r.X+r.W > group.Bounds.X+group.Bounds.W || r.Y+r.H > group.Bounds.Y+group.Bounds.H {
			t.Errorf("segment %v outside the group %v", r, group.Bounds)
		}
	}
	if button.Bounds.X+button.Bounds.W > arrow.Bounds.X {
		t.Errorf("the arrow %v is not after the button %v", arrow.Bounds, button.Bounds)
	}
	if err := tt.Click("More options"); err != nil {
		t.Fatal(err)
	}
	if arrow, _ := tt.AccessByRole(ui.RoleMenuButton, "More options"); !arrow.Expanded || !a.open {
		t.Fatalf("the arrow with its menu open: %+v, open %v", arrow, a.open)
	}
	if _, ok := tt.AccessByRole(ui.RoleMenuItem, "Save As…"); !ok {
		t.Fatal("no menu item Save As…")
	}
	if err := tt.Click("Save As…"); err != nil {
		t.Fatal(err)
	}
	if a.savedAs != 1 || a.saved != 0 || a.open {
		t.Fatalf("choosing Save As…: saved as %d, saved %d, open %v", a.savedAs, a.saved, a.open)
	}
}

// TestSplitButtonDrawnMenuKeys checks a menu drawn in the window, opened
// from the button by Alt+Down: it takes the focus, and gives it back to
// the button as Escape closes it.
func TestSplitButtonDrawnMenuKeys(t *testing.T) {
	a := &splitApp{drawn: true}
	labels := []string{"Before", "Save", "More options", "After"}
	tt := ui.NewTester(a.view, 400, 300)
	splitTabs(tt, 0, 2, labels...)
	tt.Key(ui.Alt, ui.KeyDown)
	if !a.open || !tt.HasText("Save a Copy") {
		t.Fatal("Alt+Down on Save opened no drawn menu")
	}
	tt.Key(0, ui.KeyEscape)
	if a.open || !tt.Focused("Save") {
		t.Fatalf("Escape: open %v, focus on %q", a.open, splitFocus(tt, labels...))
	}
	// Down, then Enter, chooses its second item.
	tt.Key(ui.Alt, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if a.copied != 1 || a.saved != 0 || a.open {
		t.Fatalf("copied %d, saved %d, open %v", a.copied, a.saved, a.open)
	}
}

// TestSplitButtonDisabled checks that disabling the group disables both
// segments, and disabling the button leaves the menu.
func TestSplitButtonDisabled(t *testing.T) {
	a := &splitApp{disabled: true}
	tt := ui.NewTester(a.view, 400, 300)
	tt.Click("Save")
	tt.Click("More options")
	if a.saved != 0 || len(tt.Menu()) != 0 {
		t.Fatalf("disabled: saved %d, menu %q", a.saved, tt.Menu())
	}
	for _, name := range []string{"Save", "More options"} {
		if info, _ := tt.Access(name); !info.Disabled {
			t.Errorf("%s is not disabled to assistive technology", name)
		}
	}
	if got := splitTabs(tt, 0, 2, "Before", "Save", "More options", "After"); !slices.Equal(got, []string{"Before", "After"}) {
		t.Errorf("Tab went %q", got)
	}
	a.disabled, a.actionOff = false, true
	tt.Frame()
	tt.Click("Save")
	tt.Click("More options")
	if a.saved != 0 || len(tt.Menu()) != 2 {
		t.Fatalf("the button disabled: saved %d, menu %q", a.saved, tt.Menu())
	}
}

// splitFocus returns which of labels has the focus, "" for none.
func splitFocus(tt *ui.Tester, labels ...string) string {
	for _, l := range labels {
		if tt.Focused(l) {
			return l
		}
	}
	return ""
}

// splitTabs presses Tab, with mods, n times and returns where the focus
// went each time.
func splitTabs(tt *ui.Tester, mods ui.Modifiers, n int, labels ...string) []string {
	var got []string
	for range n {
		tt.Key(mods, ui.KeyTab)
		got = append(got, splitFocus(tt, labels...))
	}
	return got
}

// TestSplitButtonMetrics checks that a split button is as high as a
// button, in the accent color as high as a primary button whether the
// window is active or not and as wide either way, and that its segments
// fill it inside its border, the arrow at its end, on the left right to
// left.
func TestSplitButtonMetrics(t *testing.T) {
	rtl := false
	tt := ui.NewTester(func(c *ui.Context) {
		row := ui.Row(c).Padding(20).Gap(8)
		if rtl {
			row.Direction(ui.RightToLeft)
		}
		row.Children(func() {
			ui.Button(c, "Plain").Label("plain")
			s := ui.SplitButton(c, "Split")
			s.Group.Label("split")
			s.Button.Label("split button")
			s.Arrow.Label("split arrow")
			ui.PrimaryButton(c, "Prime").Label("prime")
			p := ui.PrimarySplitButton(c, "Prime")
			p.Group.Label("primary split")
		})
	}, 600, 100)
	plain, split := at(t, tt, "plain"), at(t, tt, "split")
	if split.H != plain.H {
		t.Errorf("a split button is %v high, a button %v", split.H, plain.H)
	}
	button, arrow := at(t, tt, "split button"), at(t, tt, "split arrow")
	if button.X != split.X+1 || button.Y != split.Y+1 || button.H != split.H-2 || arrow.X+arrow.W != split.X+split.W-1 || arrow.X != button.X+button.W+1 {
		t.Errorf("segments %v and %v in a split button %v", button, arrow, split)
	}
	active := at(t, tt, "primary split")
	if prime := at(t, tt, "prime"); active.H != prime.H {
		t.Errorf("a primary split button is %v high, a primary button %v", active.H, prime.H)
	}
	tt.SetWindowActive(false)
	inactive := at(t, tt, "primary split")
	if prime := at(t, tt, "prime"); inactive.H != prime.H || inactive.W != active.W || inactive.H != active.H {
		t.Errorf("in an inactive window, a primary split button is %v, %v active, a primary button %v", inactive, active, prime)
	}
	rtl = true
	tt.SetWindowActive(true)
	tt.Frame()
	split, button, arrow = at(t, tt, "split"), at(t, tt, "split button"), at(t, tt, "split arrow")
	if arrow.X != split.X+1 || button.X != arrow.X+arrow.W+1 || button.X+button.W != split.X+split.W-1 {
		t.Errorf("right to left, segments %v and %v in a split button %v", button, arrow, split)
	}
}

// pixel returns the color of the window at x, y.
func pixel(tt *ui.Tester, x, y float32) color.RGBA { return tt.Image().RGBAAt(int(x), int(y)) }

// TestSplitButtonLook checks that a split button shows one face, which its
// segments share, with a line of its border's color between them, and
// that the pointer lights up the segment it is over alone; in a toolbar,
// that it has no face until the pointer is over it.
func TestSplitButtonLook(t *testing.T) {
	inToolbar := false
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).AlignItems(ui.Start).Children(func() {
			build := func() {
				s := ui.SplitButton(c, "Split")
				s.Group.Label("split")
				s.Button.Label("split button")
				s.Arrow.Label("split arrow").Menu(func(m *ui.Menu) { m.Item("Other") })
			}
			if inToolbar {
				ui.Toolbar(c, build)
			} else {
				build()
			}
		})
	}, 300, 120)
	type points struct{ button, arrow, line, border, outside color.RGBA }
	look := func() points {
		g, b, a := at(t, tt, "split"), at(t, tt, "split button"), at(t, tt, "split arrow")
		mid := g.Y + g.H/2
		return points{pixel(tt, b.X+3, mid), pixel(tt, a.X+2, mid), pixel(tt, b.X+b.W, mid), pixel(tt, b.X+b.W/2, g.Y), pixel(tt, g.X-6, mid)}
	}
	rest := look()
	if rest.button != rest.arrow || rest.button == rest.outside {
		t.Errorf("at rest, the button shows %v, the arrow %v, outside %v", rest.button, rest.arrow, rest.outside)
	}
	if rest.line != rest.border || rest.line == rest.button {
		t.Errorf("the line between the segments is %v, the border %v, the face %v", rest.line, rest.border, rest.button)
	}
	a := at(t, tt, "split arrow")
	tt.Move(a.X+a.W/2, a.Y+a.H/2)
	over := look()
	if over.arrow == rest.arrow || over.button != rest.button {
		t.Errorf("the pointer over the arrow: the arrow shows %v (%v at rest), the button %v (%v)", over.arrow, rest.arrow, over.button, rest.button)
	}

	// The ring of the button with the focus goes over the arrow's face.
	tt.Move(5, 5)
	tt.Key(0, ui.KeyTab)
	g, b := at(t, tt, "split"), at(t, tt, "split button")
	ring := pixel(tt, b.X+b.W/2, g.Y-2)
	if ring == rest.outside || pixel(tt, a.X+a.W/2, g.Y-2) != rest.outside {
		t.Errorf("the button's ring shows %v above it, %v above the arrow", ring, pixel(tt, a.X+a.W/2, g.Y-2))
	}
	tt.Move(a.X+a.W/2, a.Y+a.H/2)
	if got := pixel(tt, a.X+1, g.Y+g.H/2); got == over.arrow || got.B <= got.R {
		t.Errorf("the arrow hovered shows %v, its face %v, where the button's ring goes", got, over.arrow)
	}

	inToolbar = true
	tt.Move(5, 5)
	tt.Frame()
	rest = look()
	if rest.button != rest.outside || rest.arrow != rest.outside || rest.line != rest.outside {
		t.Errorf("in a toolbar at rest, the button shows %v, the arrow %v, the line %v, outside %v", rest.button, rest.arrow, rest.line, rest.outside)
	}
	b = at(t, tt, "split button")
	tt.Move(b.X+b.W/2, b.Y+b.H/2)
	over = look()
	if over.arrow == rest.outside || over.button == over.arrow || over.line == over.arrow {
		t.Errorf("in a toolbar, the pointer over the button: it shows %v, the arrow %v, the line %v, outside %v", over.button, over.arrow, over.line, rest.outside)
	}
}

// TestPrimarySplitButtonLook checks that a primary split button shows the
// face of a primary button, a line apart from it between its segments,
// and the face of a button in a window not drawn as active, as a primary
// button does.
func TestPrimarySplitButtonLook(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Padding(20).Gap(8).Children(func() {
			ui.PrimaryButton(c, "Prime").Label("prime")
			s := ui.PrimarySplitButton(c, "Run")
			s.Button.Label("run")
			s.Arrow.Label("run arrow")
		})
	}, 300, 80)
	look := func() (prime, button, arrow, line color.RGBA) {
		p, b, a := at(t, tt, "prime"), at(t, tt, "run"), at(t, tt, "run arrow")
		mid := b.Y + b.H/2
		prime = pixel(tt, p.X+4, mid)
		// The line is a pixel wide between the segments, at a whole pixel
		// or between two, as the label's width rounds.
		line = prime
		for x := b.X + b.W - 1; x <= a.X+1; x++ {
			if px := pixel(tt, x, mid); px != prime {
				line = px
				break
			}
		}
		return prime, pixel(tt, b.X+4, mid), pixel(tt, a.X+3, mid), line
	}
	prime, button, arrow, line := look()
	if button != prime || arrow != prime || line == prime {
		t.Errorf("a primary button shows %v; a primary split button %v and %v, its line %v", prime, button, arrow, line)
	}
	tt.SetWindowActive(false)
	inactive, button, arrow, _ := look()
	if inactive == prime || button != inactive || arrow != inactive {
		t.Errorf("inactive, a primary button shows %v; a primary split button %v and %v", inactive, button, arrow)
	}
}

// TestSplitButtonToolbarOverflow checks that a split button a toolbar has
// no room for goes into its overflow menu as its two items, the button's
// choice doing its action.
func TestSplitButtonToolbarOverflow(t *testing.T) {
	saved := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Toolbar(c, func() {
			ui.Button(c, "A long button")
			s := ui.SplitButton(c, "Save")
			s.Arrow.Menu(func(m *ui.Menu) { m.Item("Save As…") })
			if s.Button.Clicked() {
				saved++
			}
		}).Width(170).Label("Tools")
	}, 400, 100)
	if tt.HasText("Save") {
		t.Fatal("the split button fits")
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); !slices.Equal(got, []string{"Save", "More options"}) {
		t.Fatalf("the overflow menu shows %q", got)
	}
	if err := tt.ChooseMenuItem("Save"); err != nil {
		t.Fatal(err)
	}
	if saved != 1 {
		t.Fatalf("choosing Save saved %d times", saved)
	}
	// The arrow's item opens its menu, below the overflow button.
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("More options"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); !slices.Equal(got, []string{"Save As…"}) {
		t.Fatalf("choosing More options opened %q", got)
	}
}
