//go:build !mygo_noinspector

package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

// TestAutomationTreeKeys reads the keys of elements as plugins/automation
// does: Element.Key's, and Context.Key's for widgets that take theirs
// before they are made.
func TestAutomationTreeKeys(t *testing.T) {
	name, agree := "Ada", false
	tt := NewTester(func(c *Context) {
		Column(c).Fill().Children(func() {
			Button(c, "Add").Key("add")
			c.Key("name")
			TextInput(c, &name)
			c.Key(7)
			Checkbox(c, &agree, "Agree")
			Button(c, "Plain")
		})
	}, 400, 300)
	tt.Frame()
	tree, keys := automated{&windowHost{rt: tt.rt}}.AutomationTree()
	want := map[platform.AccessRole]any{platform.RoleButton: "add", platform.RoleTextField: "name", platform.RoleCheckBox: 7}
	seen := 0
	for _, n := range tree.Nodes {
		k, ok := keys[n.ID]
		switch {
		case n.Role == platform.RoleButton && n.Label == "Plain":
			if ok {
				t.Errorf("a button without a key has %v", k)
			}
		case want[n.Role] != nil:
			seen++
			if k != want[n.Role] {
				t.Errorf("%v %q: key %v, want %v", n.Role, n.Label, k, want[n.Role])
			}
		}
	}
	if seen != len(want) {
		t.Errorf("found %d of the keyed elements in %+v", seen, tree.Nodes)
	}
}

// TestAutomationSettlesOutOfSight presses a button of a window out of
// sight whose surface draws no frame, as macOS's of a hidden window or
// behind the locked screen: the view takes the press once Settle builds
// the frame waiting, which then reads as pending no more, and the tree
// shows the outcome. In a window that shows, Settle leaves the frame to
// the surface.
func TestAutomationSettlesOutOfSight(t *testing.T) {
	t.Setenv("MYGO_GPU", "0")
	s := &occludedSurface{}
	h := &windowHost{conn: &surface.Conn{Surface: s, Post: func(f func()) { f() }}}
	count, builds := 0, 0
	h.rt = newRuntime(func(c *context) {
		builds++
		coreColumn(c).Children(func() {
			coreTextf(c, "Count: %d", count)
			if coreButton(c, "Add").Clicked() {
				count++
			}
		})
	}, h)
	a := automated{h}
	frame := func() { h.event(platform.SurfaceEvent{Kind: platform.SurfaceFrame}) }
	find := func(label string) uint64 {
		tree, _ := a.AutomationTree()
		for _, n := range tree.Nodes {
			if n.Label == label {
				return n.ID
			}
		}
		return 0
	}
	frame()
	add := find("Add")
	if add == 0 {
		t.Fatal("no button")
	}

	s.occluded = true
	h.event(platform.SurfaceEvent{Kind: platform.AccessAction, ID: add, Action: platform.AccessPress})
	if !a.FramePending() || count != 0 {
		t.Fatalf("after the press: pending %v, count %d", a.FramePending(), count)
	}
	a.Settle()
	if count != 1 || a.FramePending() || find("Count: 1") == 0 {
		t.Fatalf("settled: count %d, pending %v", count, a.FramePending())
	}
	built := builds
	a.Settle()
	if builds != built {
		t.Errorf("settling again built %d more", builds-built)
	}

	// A window that shows draws its frames as the surface asks for them.
	s.occluded = false
	h.event(platform.SurfaceEvent{Kind: platform.AccessAction, ID: add, Action: platform.AccessPress})
	a.Settle()
	if count != 1 || !a.FramePending() {
		t.Fatalf("a press in a window that shows, settled: count %d, pending %v", count, a.FramePending())
	}
	frame()
	if count != 2 || a.FramePending() {
		t.Errorf("after its frame: count %d, pending %v", count, a.FramePending())
	}
}
