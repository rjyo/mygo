package main

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

// The marks hide, but on the lines with a caret.
func TestLivePreview(t *testing.T) {
	st := &editor.State{Buffer: editor.NewBuffer("a **b** c\n# Title\n- [ ] task\n- item\n")}
	st.Select(0, 0)
	shown := func(rs []editor.Replacement) []string {
		var out []string
		for _, r := range rs {
			out = append(out, st.Buffer.Slice(r.Start, r.End))
		}
		return out
	}
	got := shown(livePreview(st))
	want := []string{"# ", "- [ ]", "-"}
	if len(got) != len(want) {
		t.Fatalf("replaced %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("replaced %q, want %q", got, want)
		}
	}
	// The caret off the first line, its marks hide too.
	st.Select(st.Buffer.Len(), st.Buffer.Len())
	if got := shown(livePreview(st)); len(got) != 5 || got[0] != "**" || got[1] != "**" {
		t.Fatalf("replaced %q", got)
	}
}
