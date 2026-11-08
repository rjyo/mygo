package ui

import "testing"

// A DragWindow handle's double click goes to the window, which maximizes,
// unless the view takes it with WindowDoubleClicked; a single press still
// moves the window either way.
func TestWindowDoubleClicked(t *testing.T) {
	for _, takes := range []bool{false, true} {
		doubles := 0
		tt := NewTester(func(c *Context) {
			h := Box(c.Key("handle")).Size(200, 100).DragWindow()
			if takes && h.WindowDoubleClicked() {
				doubles++
			}
		}, 200, 100)
		tt.Frame()
		tt.ClickAt(50, 50)
		tt.Frame()
		tt.ClickAt(50, 50)
		tt.Frame()
		want, wantWindow := 0, 1
		if takes {
			want, wantWindow = 1, 0
		}
		if doubles != want || tt.h.titleDoubles != wantWindow {
			t.Errorf("takes=%v: the view saw %d double clicks, the window %d", takes, doubles, tt.h.titleDoubles)
		}
		tt.rt.close()
	}
}
