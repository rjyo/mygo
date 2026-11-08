package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// A text box that MyGo does not edit, drawn and handled by an element of
// its own, reports aria-readonly and aria-multiline as one with an editor
// does.
func TestAccessTextboxStatesWithoutEditor(t *testing.T) {
	tt := NewTester(func(c *Context) {
		Column(c).Children(func() {
			Box(c).Role(RoleTextField).Label("Plain")
			Box(c).Role(RoleTextField).Label("Locked").ReadOnly(true)
			Box(c).Role(RoleTextField).Label("Notes").Multiline(true)
			Box(c).Role(RoleButton).Label("Button").ReadOnly(true).Multiline(true)
			TextInput(c, new(string)).Label("Input").Multiline(true)
		})
	}, 400, 300)
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	get := func(name string, r Role) AccessInfo {
		t.Helper()
		a, ok := tt.AccessByRole(r, name)
		if !ok {
			t.Fatalf("no %q", name)
		}
		return a
	}
	if a := get("Plain", RoleTextField); a.ReadOnly || a.Multiline {
		t.Errorf("Plain: %+v", a)
	}
	if a := get("Locked", RoleTextField); !a.ReadOnly || a.Multiline {
		t.Errorf("Locked: %+v", a)
	}
	if a := get("Notes", RoleTextField); a.ReadOnly || !a.Multiline {
		t.Errorf("Notes: %+v", a)
	}
	if a := get("Button", RoleButton); a.ReadOnly || a.Multiline {
		t.Errorf("a button has neither: %+v", a)
	}
	if a := get("Input", RoleTextField); a.Multiline {
		t.Errorf("a built-in editor decides for itself: %+v", a)
	}
}
