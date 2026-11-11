package ui

import (
	"slices"
	"testing"
)

// The children out of their parent's flow are placed by the parent's
// layout: an absolute one with insets, an attached one, and one in an
// overlay. The walk over the children that finds them must place them as
// it did, and leave the children in the flow where they were.
func TestLayoutAbsoluteChild(t *testing.T) {
	var flow, abs *node
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Size(200, 200).Children(func() {
			flow = coreBox(c).Size(40, 20).Shrink(0)
			abs = coreBox(c).Absolute().Left(30).Top(50).Size(60, 40).Shrink(0)
		})
	}, 200, 200)
	tt.Frame()
	// The absolute child is placed by its insets, and takes no room in the
	// flow: the one in the flow stays where the layout put it.
	if abs.x != 30 || abs.y != 50 || abs.w != 60 || abs.h != 40 {
		t.Errorf("the absolute child at %v, %v, %vx%v, want 30, 50, 60x40", abs.x, abs.y, abs.w, abs.h)
	}
	if flow.x != 0 || flow.y != 0 || flow.w != 40 || flow.h != 20 {
		t.Errorf("the child in the flow at %v, %v, %vx%v, want 0, 0, 40x20", flow.x, flow.y, flow.w, flow.h)
	}
	if abs.y <= flow.y+flow.h {
		t.Errorf("the absolute child at %v, over the one in the flow at %v", abs.y, flow.y)
	}
}

// An element in its parent's layout moves from where the layout put it
// with its insets, as CSS's relative positioning does, and its siblings
// stay where they are.
func TestLayoutInsetChild(t *testing.T) {
	var moved, sibling *node
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Size(200, 100).Children(func() {
			moved = coreBox(c).Size(40, 20).Shrink(0).Left(10).Top(5)
			sibling = coreBox(c).Size(40, 20).Shrink(0)
		})
	}, 200, 100)
	tt.Frame()
	if moved.x != 10 || moved.y != 5 {
		t.Errorf("the child with insets at %v, %v, want 10, 5", moved.x, moved.y)
	}
	// Its sibling keeps the place the layout gave it: the insets move the
	// element alone.
	if sibling.x != 0 || sibling.y != 20 {
		t.Errorf("its sibling at %v, %v, want 0, 20", sibling.x, sibling.y)
	}
}

// An attached element is placed at the anchor of its target, wherever the
// target is: a button in the corner of its parent, a panel below another
// button.
func TestLayoutAttachedChild(t *testing.T) {
	var target, corner, panel *node
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Size(200, 200).Children(func() {
			coreRow(c).Width(200).Children(func() {
				coreBox(c).Size(40, 20).Shrink(0)
				target = coreBox(c).Size(40, 20).Shrink(0)
			})
			coreBox(c).Size(10, 10).Shrink(0)
			corner = coreBox(c).Size(30, 10).Shrink(0).Attach(AnchorBottomRight, AnchorBottomRight)
		})
		coreOverlay(c, func() {
			panel = coreBox(c).Size(30, 10).Shrink(0).AttachTo(target, AnchorBottomLeft, AnchorTopLeft)
		})
	}, 200, 200)
	tt.Frame()
	// The corner of its parent: the parent's bottom right, without its
	// border.
	if corner.x != 170 || corner.y != 190 {
		t.Errorf("the attached corner at %v, %v, want 170, 190", corner.x, corner.y)
	}
	// The panel below the button, wherever the button is.
	if panel.x != target.x || panel.y != target.y+target.h {
		t.Errorf("the panel at %v, %v, want %v, %v", panel.x, panel.y, target.x, target.y+target.h)
	}
}

// A child of an overlay is placed by the overlay, which the frame adds to
// the root as it lays out: the root walks its children for the ones out
// of the flow, and the overlay holds them.
func TestLayoutOverlayChild(t *testing.T) {
	var ov *node
	tt := coreNewTester(func(c *context) {
		coreBox(c).Size(100, 100).Shrink(0)
		coreOverlay(c, func() {
			ov = coreBox(c).Absolute().Left(10).Top(20).Size(30, 30).Shrink(0)
		})
	}, 200, 200)
	tt.Frame()
	if ov.x != 10 || ov.y != 20 || ov.w != 30 || ov.h != 30 {
		t.Errorf("the overlay's child at %v, %v, %vx%v, want 10, 20, 30x30", ov.x, ov.y, ov.w, ov.h)
	}
}

// A control that does not fit in its toolbar leaves the flow after the
// build, into the toolbar's overflow menu: the toolbar then walks its
// children for the ones out of the flow, and finds it.
func TestLayoutAbsoluteAfterBuild(t *testing.T) {
	tt := coreNewTester(func(c *context) {
		coreToolbar(c, func() {
			for _, n := range []string{"Alpha", "Bravo", "Charlie", "Delta"} {
				coreButton(c, n)
			}
		}).Fill()
	}, 120, 100)
	if tt.HasText("Delta") {
		t.Fatal("the controls fit in the toolbar")
	}
	tt.Click("More")
	if got, want := tt.Menu(), []string{"Bravo", "Charlie", "Delta"}; !slices.Equal(got, want) {
		t.Errorf("the overflow menu shows %q, want %q", got, want)
	}
}
