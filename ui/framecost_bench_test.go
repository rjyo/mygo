package ui

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// liveRows is a view of n rows built whole, none of them virtualized as
// a List's are, each a label and a button, under a text input: the
// fixture of the frame-cost benchmarks, which change one thing of a
// large tree and measure what the frame costs.
type liveRows struct {
	labels []string
	query  string
	list   bool
	builds int
	// at is where a step points, found as it first does.
	at Rect
}

func newLiveRows(n int) *liveRows {
	v := &liveRows{}
	for i := range n {
		v.labels = append(v.labels, fmt.Sprintf("Row %d", i))
	}
	return v
}

func (v *liveRows) row(c *context, i int) {
	coreRow(c).Gap(8).Padding(4, 8).Children(func() {
		coreText(c, v.labels[i]).Grow(1)
		coreButton(c, "Open")
	})
}

func (v *liveRows) view(c *context) {
	v.builds++
	coreColumn(c).Grow(1).Children(func() {
		coreTextInput(c, &v.query)
		if v.list {
			coreList(c, nil, len(v.labels), func(i int) { v.row(c, i) }).Grow(1)
			return
		}
		coreScroll(c).Grow(1).Children(func() {
			for i := range v.labels {
				v.row(c, i)
			}
		})
	})
}

// hostEvent sends ev as a window's surface does: the frame it asks for,
// if any, comes as the surface's next, which paints the last frame's
// elements again when only drawings changed, and builds anew otherwise.
func (t *Tester) hostEvent(ev platform.SurfaceEvent) {
	t.h.requested.Store(false)
	t.rt.event(ev)
	for i := 0; i < 20 && t.asked(); i++ {
		t.h.requested.Store(false)
		t.runLater()
		t.rt.surfaceFrame()
	}
}

func benchLiveRows(b *testing.B, n int, list bool, step func(tt *Tester, v *liveRows, i int)) {
	v := newLiveRows(n)
	v.list = list
	tt := coreNewTester(v.view, 980, 720)
	for range 3 {
		tt.Frame()
	}
	v.builds = 0
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		step(tt, v, i)
		i++
	}
	b.ReportMetric(float64(v.builds)/float64(i), "builds/op")
}

// changeLabel changes the text of one row and asks for a frame, as an app
// does after changing its state (Window.Update).
func changeLabel(tt *Tester, v *liveRows, i int) {
	v.labels[5] = fmt.Sprintf("Row 5 (%d)", i)
	tt.Frame()
}

// sameFrame asks for a frame with nothing changed, as an app does that
// calls Window.Update after writing a value equal to the last.
func sameFrame(tt *Tester, v *liveRows, i int) { tt.Frame() }

// hoverButton moves the pointer onto a row's button and off it again,
// one or the other each time.
func hoverButton(tt *Tester, v *liveRows, i int) {
	if i == 0 {
		v.at, _ = tt.Find("Open")
	}
	r := v.at
	x := r.X + 5
	if i%2 == 1 {
		x = r.X - 300
	}
	tt.hostEvent(platform.SurfaceEvent{Kind: platform.PointerMove, X: float64(x), Y: float64(r.Y + 5)})
}

// crossRows moves the pointer from one row's label to the next one's and
// back, inside the scroll container that holds them.
func crossRows(tt *Tester, v *liveRows, i int) {
	if i == 0 {
		v.at, _ = tt.Find("Row 5")
	}
	r := v.at
	y := r.Y + 5
	if i%2 == 1 {
		y += r.H + 8
	}
	tt.hostEvent(platform.SurfaceEvent{Kind: platform.PointerMove, X: float64(r.X + 5), Y: float64(y)})
}

// typeKey types a letter into the text input above the rows, once
// focused.
func typeKey(tt *Tester, v *liveRows, i int) {
	if i == 0 {
		tt.ClickAt(20, 15)
		v.builds = 0
	}
	if len(v.query) > 64 {
		v.query = ""
	}
	tt.Type("a")
}

// BenchmarkLiveRows1000Change changes one label of 1,000 rows built whole.
func BenchmarkLiveRows1000Change(b *testing.B) { benchLiveRows(b, 1000, false, changeLabel) }

// BenchmarkLiveRows3000Change changes one label of 3,000 rows built whole:
// with BenchmarkLiveRows1000Change, how a frame's cost grows with what the
// view builds while the change stays the same.
func BenchmarkLiveRows3000Change(b *testing.B) { benchLiveRows(b, 3000, false, changeLabel) }

// BenchmarkLiveRows3000Same asks for a frame of 3,000 rows built whole
// with nothing changed.
func BenchmarkLiveRows3000Same(b *testing.B) { benchLiveRows(b, 3000, false, sameFrame) }

// BenchmarkLiveRows3000Hover moves the pointer onto a button of 3,000 rows
// built whole, and off it, as a window's surface sends the moves.
func BenchmarkLiveRows3000Hover(b *testing.B) { benchLiveRows(b, 3000, false, hoverButton) }

// BenchmarkLiveRows3000Cross moves the pointer across the labels of 3,000
// rows built whole, inside the scroll container holding them, which shows
// its scroll bars on hover.
func BenchmarkLiveRows3000Cross(b *testing.B) { benchLiveRows(b, 3000, false, crossRows) }

// BenchmarkLiveRows3000Type types into a text input above 3,000 rows built
// whole.
func BenchmarkLiveRows3000Type(b *testing.B) { benchLiveRows(b, 3000, false, typeKey) }

// BenchmarkListRows3000Change changes one label of 3,000 rows of a List,
// which builds those in view.
func BenchmarkListRows3000Change(b *testing.B) { benchLiveRows(b, 3000, true, changeLabel) }
