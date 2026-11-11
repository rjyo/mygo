package editor

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestTrailingInlayHoverHitTest(t *testing.T) {
	st := &State{Buffer: NewBuffer("abc\n"), HoverDelay: time.Second}
	st.SetInlays([]Inlay{{Offset: 3, Text: "  You, 2 months ago • subject", Trailing: true}})
	var requests []HoverRequest
	st.OnHover = func(r HoverRequest) { requests = append(requests, r) }
	tt := ui.NewTester(func(c *ui.Context) { View(c, st).Fill() }, 900, 300)
	tt.Frame()
	ed := &st.ed
	caret, ok := ed.caretRect(3)
	if !ok {
		t.Fatal("the source line end is not visible")
	}
	y := float32(-ed.scrollY) + caret.Y + ed.view.lineHeight/2
	tt.Move(ed.textX+caret.X+5*ed.view.charWidth, y)
	tt.Advance(1500 * time.Millisecond)
	tt.Frame()
	if len(requests) != 1 || requests[0].Offset != 3 {
		t.Fatalf("hover over trailing inlay requests = %#v", requests)
	}

	// The blank space after the decoration is still not source text.
	tt.Move(ed.textX+caret.X+50*ed.view.charWidth, y)
	tt.Advance(1500 * time.Millisecond)
	tt.Frame()
	if len(requests) != 1 {
		t.Fatalf("blank space after trailing inlay requested hover: %#v", requests)
	}
}
