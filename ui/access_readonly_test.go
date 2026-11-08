package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// aria-readonly is reported by the roles ARIA gives it to: a grid, its
// cells and headers (not those of a table), a slider, a stepper, a check
// box, a switch, a radio group, a combo box and a list box of your own,
// which then take no set-value, increment or decrement from assistive
// technology. Others, and MyGo's own controls, ignore it.
func TestAccessReadOnlyRoles(t *testing.T) {
	value := 3.0
	on := false
	tt := NewTester(func(c *Context) {
		Column(c).Children(func() {
			Column(c).Role(RoleGrid).Label("Sheet").ReadOnly(true).Children(func() {
				Row(c).Role(RoleRow).Children(func() {
					Box(c).Role(RoleColumnHeader).Label("Head").ReadOnly(true)
					Box(c).Role(RoleCell).Label("Locked cell").ReadOnly(true)
					Box(c).Role(RoleCell).Label("Open cell")
				})
			})
			Column(c).Role(RoleTable).Label("Plain table").Children(func() {
				Row(c).Role(RoleRow).Children(func() {
					Box(c).Role(RoleCell).Label("Table cell").ReadOnly(true)
				})
			})
			Box(c).Role(RoleSlider).Label("Locked slider").Focusable().Range(0, 10, 3).ReadOnly(true)
			Box(c).Role(RoleSlider).Label("Open slider").Focusable().Range(0, 10, 3)
			Box(c).Role(RoleStepper).Label("Locked stepper").Focusable().Range(0, 10, 3).ReadOnly(true)
			Box(c).Role(RoleCheckBox).Label("Locked check").Focusable().OnClick(func() {}).ReadOnly(true)
			Box(c).Role(RoleSwitch).Label("Locked switch").Focusable().OnClick(func() {}).ReadOnly(true)
			Box(c).Role(RoleRadioGroup).Label("Locked group").ReadOnly(true)
			Box(c).Role(RoleComboBox).Label("Locked combo").ReadOnly(true)
			Box(c).Role(RoleListBox).Label("Locked list box").ReadOnly(true)
			Box(c).Role(RoleList).Label("Plain list").ReadOnly(true)
			Box(c).Role(RoleButton).Label("Button").Focusable().OnClick(func() {}).ReadOnly(true)
			Box(c).Role(RoleRadio).Label("Radio").Focusable().ReadOnly(true)
			Slider(c, &value, 0, 10).Label("Built-in slider").ReadOnly(true)
			Checkbox(c, &on, "Built-in check").ReadOnly(true)
		})
	}, 400, 600)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	node := func(r platform.AccessRole, name string) platform.AccessNode {
		t.Helper()
		return accessNode(t, tt.h.access, r, name)
	}
	ro := func(n platform.AccessNode) bool { return n.States&platform.AccessReadOnly != 0 }

	for _, c := range []struct {
		role platform.AccessRole
		name string
		want bool
	}{
		{platform.RoleGrid, "Sheet", true},
		{platform.RoleColumnHeader, "Head", true},
		{platform.RoleCell, "Locked cell", true},
		{platform.RoleCell, "Open cell", false},
		{platform.RoleCell, "Table cell", false},
		{platform.RoleSlider, "Locked slider", true},
		{platform.RoleSlider, "Open slider", false},
		{platform.RoleStepper, "Locked stepper", true},
		{platform.RoleCheckBox, "Locked check", true},
		{platform.RoleSwitch, "Locked switch", true},
		{platform.RoleRadioGroup, "Locked group", true},
		{platform.RoleComboBox, "Locked combo", true},
		{platform.RoleList, "Locked list box", true},
		{platform.RoleList, "Plain list", false},
		{platform.RoleButton, "Button", false},
		{platform.RoleRadio, "Radio", false},
		{platform.RoleSlider, "Built-in slider", false},
		{platform.RoleCheckBox, "Built-in check", false},
	} {
		if n := node(c.role, c.name); ro(n) != c.want {
			t.Errorf("%q: read-only %v, want %v", c.name, ro(n), c.want)
		}
	}

	const step = platform.ActionIncrement | platform.ActionDecrement
	if n := node(platform.RoleSlider, "Open slider"); n.Actions&step != step {
		t.Errorf("an open slider steps: %v", n.Actions)
	}
	for _, c := range []struct {
		role platform.AccessRole
		name string
	}{{platform.RoleSlider, "Locked slider"}, {platform.RoleStepper, "Locked stepper"}} {
		if n := node(c.role, c.name); n.Actions&(step|platform.ActionSetValue) != 0 || n.Actions&platform.ActionFocus == 0 {
			t.Errorf("%q: a read-only one is focused, not changed: %v", c.name, n.Actions)
		}
	}
	// Core-AAM takes a value's setting away, not a press: the app decides
	// what pressing a read-only check box does.
	if n := node(platform.RoleCheckBox, "Locked check"); n.Actions&platform.ActionPress == 0 {
		t.Errorf("a read-only check box is still pressed: %v", n.Actions)
	}
	if n := node(platform.RoleButton, "Button"); n.Actions&platform.ActionPress == 0 {
		t.Errorf("a button is still pressed: %v", n.Actions)
	}
	if n := node(platform.RoleCheckBox, "Built-in check"); n.Actions&platform.ActionPress == 0 {
		t.Errorf("a built-in check box is still pressed: %v", n.Actions)
	}
}

// What the Tester shows of it.
func TestAccessReadOnlyInfo(t *testing.T) {
	tt := NewTester(func(c *Context) {
		Column(c).Role(RoleGrid).Label("Sheet").Children(func() {
			Row(c).Role(RoleRow).Children(func() {
				Box(c).Role(RoleCell).Label("Locked").ReadOnly(true)
			})
		})
	}, 300, 200)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	if a, ok := tt.AccessByRole(RoleCell, "Locked"); !ok || !a.ReadOnly {
		t.Errorf("a locked grid cell: %+v, %v", a, ok)
	}
}
