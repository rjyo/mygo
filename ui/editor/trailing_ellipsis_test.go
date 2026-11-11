package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTrailingInlayEllipsisStaysOnOneRow(t *testing.T) {
	st := &State{Buffer: NewBuffer("abc\n"), SoftWrap: true}
	st.SetInlays([]Inlay{{
		Offset: 3, Text: "  You, 2 months ago • a commit subject too long for this editor",
		Trailing: true, Ellipsis: "…",
	}})
	tt := ui.NewTester(func(c *ui.Context) { View(c, st).Fill() }, 240, 120)
	tt.Frame()
	line := st.ed.view.line(0)
	if len(line.rows) != 1 {
		t.Fatalf("trailing inlay used %d rows: %q", len(line.rows), line.disp)
	}
	if !strings.HasPrefix(line.disp, "abc  You,") || !strings.HasSuffix(line.disp, "…") {
		t.Fatalf("ellipsized trailing inlay = %q", line.disp)
	}
	if got := st.ed.view.inlays[0].text; !strings.HasSuffix(got, "editor") {
		t.Fatalf("layout changed the original inlay text to %q", got)
	}
}

func TestTrailingInlayEllipsisMeasuresReplacementWidths(t *testing.T) {
	st := &State{Buffer: NewBuffer("abc\n"), SoftWrap: true}
	st.SetReplacements([]Replacement{box(0, 1, 140, "Replacement")})
	st.SetInlays([]Inlay{{
		Offset: 3, Text: "  You, 2 months ago • a commit subject too long for this editor",
		Trailing: true, Ellipsis: "…",
	}})
	tt := ui.NewTester(func(c *ui.Context) { View(c, st).Fill() }, 240, 120)
	tt.Frame()
	line := st.ed.view.line(0)
	if len(line.rows) != 1 {
		t.Fatalf("trailing inlay after a replacement used %d rows: %q", len(line.rows), line.disp)
	}
	if got, want := line.disp, objectChar+"bc…"; got != want {
		t.Fatalf("ellipsized trailing inlay after a replacement = %q, want %q", got, want)
	}
	if strings.Contains(line.disp, "months") {
		t.Fatalf("the replacement's 140-DIP width was not included in elision: %q", line.disp)
	}
}
