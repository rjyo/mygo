package ui

import "testing"

// TestMenuButtonHasPopup checks that the buttons opening a menu as they
// are pressed tell assistive technology so (aria-haspopup="menu"), with
// no HasPopup of the app's (#1434), that HasPopup before or after wins
// over it, and that an element with a context menu tells nothing.
func TestMenuButtonHasPopup(t *testing.T) {
	var menuOpen bool
	build := func(m *Menu) { m.Item("Duplicate") }
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Padding(20).Gap(10).AlignItems(Start).Children(func() {
			coreMenuButton(c, "Sort by", build)
			corePopoverMenuButton(c, "View", build)
			coreButtonBase(c).Label("More").Padding(4).Children(func() { coreText(c, "…") }).Menu(build)
			coreButtonBase(c).Label("Options").Padding(4).Children(func() { coreText(c, "⋯") }).PopoverMenuButton(build)
			m := coreMenuBase(c, &menuOpen)
			m.Trigger.Label("Actions").Padding(4).Children(func() { coreText(c, "Actions") })
			m.Popup(func(panel *node) { m.Item().Children(func() { coreText(c, "Open") }) })
			coreMenuButton(c, "Share", build).HasPopup(PopupDialog)
			coreButtonBase(c).Label("Plain").Padding(4).Children(func() { coreText(c, "Plain") }).HasPopup(PopupNone).Menu(build)
			coreRow(c).Padding(8).Focusable().Label("Report").Children(func() { coreText(c, "Report.pdf") }).ContextMenu(build)
		})
	}, 400, 600)
	for name, want := range map[string]Popup{
		"Sort by": PopupMenu, "View": PopupMenu, "More": PopupMenu, "Options": PopupMenu, "Actions": PopupMenu,
		"Share": PopupDialog, "Plain": PopupNone,
	} {
		info, ok := tt.AccessByRole(RoleMenuButton, name)
		if !ok {
			t.Errorf("no menu button %q", name)
			continue
		}
		if info.HasPopup != want {
			t.Errorf("%q: HasPopup = %d, want %d", name, info.HasPopup, want)
		}
	}
	if info, ok := tt.Access("Report"); !ok || info.HasPopup != PopupNone {
		t.Errorf("the row with a context menu: %+v", info)
	}
	// A drawn menu's button is expanded while its menu shows, and still
	// opens a menu.
	if info, _ := tt.AccessByRole(RoleMenuButton, "View"); info.Expanded {
		t.Error("View is expanded before its menu opens")
	}
	if err := tt.Click("View"); err != nil {
		t.Fatal(err)
	}
	if info, _ := tt.AccessByRole(RoleMenuButton, "View"); !info.Expanded || info.HasPopup != PopupMenu {
		t.Errorf("View with its menu open: %+v", info)
	}
}

// TestMenuButtonHasPopupDrawnMenus checks Element.Menu's button in a
// window drawing its menus (PopoverMenus), which builds it as
// PopoverMenuButton's.
func TestMenuButtonHasPopupDrawnMenus(t *testing.T) {
	tt := coreNewTester(func(c *context) {
		c.SetMenuPresentation(PopoverMenus)
		coreMenuButton(c, "Sort by", func(m *Menu) { m.Item("Name") })
	}, 400, 400)
	if info, ok := tt.AccessByRole(RoleMenuButton, "Sort by"); !ok || info.HasPopup != PopupMenu {
		t.Errorf("Sort by: %+v", info)
	}
}
