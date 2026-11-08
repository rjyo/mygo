package ui

import "testing"

// A paragraph's duplicate text label must not hide its inline link from named clicks, while an overlay still blocks the click.
func TestTesterClicksInlineLinkWithParagraphLabel(t *testing.T) {
	linkClicks, rowClicks, overlayClicks := 0, 0, 0
	covered := false
	tt := NewTester(func(c *Context) {
		Box(c).Size(300, 100).Children(func() {
			Row(c).Size(300, 80).OnClick(func() { rowClicks++ }).Children(func() {
				RichText(c).Children(func() {
					Link(c, "Artist", "").OnClick(func() { linkClicks++ })
				})
			})
			if covered {
				Box(c).Absolute().Top(0).Left(0).Size(300, 100).OnClick(func() { overlayClicks++ })
			}
		})
	}, 300, 100)
	if err := tt.Click("Artist"); err != nil {
		t.Fatal(err)
	}
	if linkClicks != 1 || rowClicks != 0 {
		t.Fatalf("clicked link %d times and row %d times", linkClicks, rowClicks)
	}
	covered = true
	tt.Frame()
	if err := tt.Click("Artist"); err == nil {
		t.Fatal("clicked a covered inline link")
	}
	if linkClicks != 1 || rowClicks != 0 || overlayClicks != 0 {
		t.Fatalf("blocked click sent input: link %d, row %d, overlay %d", linkClicks, rowClicks, overlayClicks)
	}
}
