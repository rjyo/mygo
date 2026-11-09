package ui_test

import (
	"fmt"
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// hoverControls is a view of controls that look at their hover as they
// paint, and some that read it as they build.
type hoverControls struct {
	builds              int
	on, checked         bool
	text, query, choice string
	volume, amount      float64
	tab, segment        int
	rating              int
	open, node          bool
	list, table         ui.ListState
	page                string
}

func (v *hoverControls) view(c *ui.Context) {
	v.builds++
	ui.Row(c).Grow(1).Gap(12).Padding(12).Children(func() {
		ui.Sidebar(c, &v.page, func() {
			ui.SidebarItem(c, "inbox", nil, "Inbox")
			ui.SidebarItem(c, "sent", nil, "Sent")
		}).Width(140)
		ui.Column(c).Gap(8).Width(300).Children(func() {
			ui.Row(c).Gap(6).Children(func() {
				ui.Button(c, "Plain")
				ui.PrimaryButton(c, "Primary")
				ui.Button(c, "Tipped").Tooltip("A tip")
			})
			ui.Row(c).Gap(6).Children(func() {
				ui.SplitButton(c, "Split")
				ui.PrimarySplitButton(c, "Run")
			})
			ui.Toolbar(c, func() {
				ui.Button(c, "Cut")
				ui.Button(c, "Copy")
				ui.SplitButton(c, "Paste")
			})
			ui.TextInput(c, &v.text)
			ui.SearchField(c, &v.query)
			ui.Checkbox(c, &v.checked, "Checked")
			ui.Switch(c, &v.on)
			ui.Slider(c, &v.volume, 0, 100)
			ui.Stepper(c, &v.amount, 0, 10, 1)
			ui.Tabs(c, &v.tab, "One", "Two", "Three")
			ui.Segmented(c, &v.segment, "A", "B")
			ui.Rating(c, &v.rating, 5)
			ui.Select(c, &v.choice, []string{"Red", "Green"})
			ui.Link(c, "A link", "https://example.com")
			ui.Collapsible(c, "More", &v.open, func() { ui.Text(c, "Inside") })
			ui.Tree(c, func() {
				ui.TreeItem(c, "Node", &v.node, func() { ui.Text(c, "Leaf") })
			})
			// Read as it builds: the hover changing builds it anew.
			b := ui.Box(c).Size(120, 24)
			if b.Hovered() {
				b.Background(ui.RGB(200, 60, 60))
			}
			// Read as it paints, by a drawing.
			d := ui.Box(c).Size(120, 24)
			d.Draw(func(p *ui.Painter, r ui.Rect) {
				if d.Hovered() {
					p.Fill(r, ui.RGB(60, 60, 200), 4)
				}
			})
		})
		ui.Column(c).Gap(8).Grow(1).Children(func() {
			ui.List(c.Key("list"), &v.list, 30).Height(160).Rows(func(row ui.ListRow) {
				t := ui.Textf(row.Context, "Row %d", row.Index).Padding(4, 8)
				if row.Index == 3 && row.Hovered() {
					t.Background(ui.RGB(230, 230, 120))
				}
			})
			ui.Table(c.Key("table"), &v.table, []ui.TableColumn{{Title: "Name"}, {Title: "Size", Width: 60}}, 20, func(row, col int) {
				ui.Textf(c, "%d:%d", row, col)
			}).Height(160)
			ui.Scroll(c).Height(120).Children(func() {
				for i := range 20 {
					ui.Row(c).Gap(6).Children(func() {
						ui.Text(c, fmt.Sprintf("Item %d", i)).Grow(1)
						ui.Button(c, "Go")
					})
				}
			})
		})
	})
}

// TestHoverLookPaintsAsBuilt moves the pointer over controls whose look
// follows their hover as they paint, as a window's surface sends the
// moves: a frame that only paints the last frame's elements again, where
// that is all the hover needs, shows what a frame built anew shows.
func TestHoverLookPaintsAsBuilt(t *testing.T) {
	v := &hoverControls{}
	tt := ui.NewTester(v.view, 980, 560)
	tt.Frame()
	repaints := 0
	for y := float32(5); y < 560; y += 11 {
		for x := float32(5); x < 980; x += 29 {
			builds, paints := v.builds, tt.Paints()
			tt.HostMove(x, y)
			if v.builds != builds || tt.Paints() == paints {
				continue // built anew, or nothing to paint
			}
			repaints++
			painted := snapshot(tt.Image())
			// The same frame, built anew.
			tt.Frame()
			if !sameImage(painted, tt.Image()) {
				t.Fatalf("at %v,%v: the frame painted for the hover differs from the frame built anew", x, y)
			}
		}
	}
	if repaints < 50 {
		t.Fatalf("%d hovers painted without building the view anew, want 50 at least", repaints)
	}
}

// TestHoverReadAsBuiltBuilds tells that an element whose hover the view
// reads as it builds is built anew as the pointer comes over it and
// leaves it, and one read as it paints only paints.
func TestHoverReadAsBuiltBuilds(t *testing.T) {
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		ui.Column(c).Gap(10).Padding(10).Children(func() {
			b := ui.Box(c).Size(100, 30)
			if b.Hovered() {
				b.Background(ui.RGB(255, 0, 0))
			}
			ui.Button(c, "Paint")
		})
	}, 300, 200)
	tt.Frame()
	r, _ := tt.Find("Paint")
	for i, step := range []struct {
		x, y  float32
		build bool
	}{
		{20, 20, true},            // onto the box read as built
		{150, 150, true},          // off it
		{r.X + 5, r.Y + 5, false}, // onto the button, read as painted
		{150, 150, false},         // off it
		{20, 20, true},            // onto the box again
	} {
		before := builds
		tt.HostMove(step.x, step.y)
		if got := builds != before; got != step.build {
			t.Errorf("move %d: built anew %v, want %v", i, got, step.build)
		}
	}
	if c := tt.Image().RGBAAt(20, 20); c.R != 255 || c.G != 0 || c.B != 0 {
		t.Errorf("the box read as built shows %v under the pointer, not red", c)
	}
}

func snapshot(m *image.RGBA) *image.RGBA {
	c := image.NewRGBA(m.Rect)
	copy(c.Pix, m.Pix)
	return c
}

func sameImage(a, b *image.RGBA) bool {
	if a.Rect != b.Rect {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}
