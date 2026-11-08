package markdown

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// sharedView builds the issue's document, whose quote a view sharing the
// selection shows through Options.Block, and returns its tester and the
// states of the views, the outer's and the inner's.
func sharedView(t *testing.T) (*ui.Tester, func() (outer, inner *viewState)) {
	const src = "Intro\n\n> Read [the guide](https://example.test/guide)\n\nOutro"
	var outerO, innerO Options
	innerO.ShareSelection = true
	var outer, inner *viewState
	outerO.Block = func(c *ui.Context, d *parser.Document, i int) bool {
		if d.Block(i).Kind != parser.Quote {
			return false
		}
		el := View(c, "Read [the guide](https://example.test/guide)", &innerO)
		inner = *ui.Local(el, stateKey{}, newState)
		return true
	}
	tt := ui.NewTester(func(c *ui.Context) {
		el := View(c, src, &outerO)
		outer = *ui.Local(el, stateKey{}, newState)
	}, 400, 300)
	tt.Frame()
	return tt, func() (*viewState, *viewState) { return outer, inner }
}

// A selection from a view sharing the selection into the view around it
// copies the inner view's part as its own Markdown.
func TestSharedViewCopyMarkdown(t *testing.T) {
	tt, _ := sharedView(t)
	from, ok := tt.Find("Read the guide")
	if !ok {
		t.Fatal("no inner text")
	}
	to, ok := tt.Find("Outro")
	if !ok {
		t.Fatal("no outer text")
	}
	tt.Press(from.X+1, from.Y+from.H/2)
	tt.Move(to.X+to.W-1, to.Y+to.H/2)
	tt.Release(to.X+to.W-1, to.Y+to.H/2)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	if got, want := tt.Clipboard(), "Read [the guide](https://example.test/guide)\n\nOutro"; got != want {
		t.Errorf("copied as Markdown %q, want %q", got, want)
	}
	tt.Command("copy")
	if got, want := tt.Clipboard(), "Read the guide\n\nOutro"; got != want {
		t.Errorf("copied %q, want %q", got, want)
	}
}

// A selection containing the inner view whole, and one from the outer
// view's text into the middle of the inner's, whose link it completes.
func TestSharedViewCopyMarkdownParts(t *testing.T) {
	_, get := sharedView(t)
	outer, inner := get()
	if outer == nil || inner == nil || len(outer.nested) != 1 || outer.nested[0].st != inner {
		t.Fatalf("inner view not noted in the outer one")
	}
	part := func(st *viewState, block int, text string, from, to int) ui.SelectedText {
		return rvPart(st, rvElemOf(t, st, block), text, from, to)
	}
	intro := part(outer, 1, "Intro", 0, 5)
	outro := part(outer, outer.doc.Block(outer.nested[0].block).Next, "Outro", 0, 5)
	const read = "Read the guide"
	for _, c := range []struct {
		name  string
		parts []ui.SelectedText
		want  string
	}{
		{"whole", []ui.SelectedText{intro, part(inner, 1, read, 0, len(read)), outro},
			"Intro\n\nRead [the guide](https://example.test/guide)\n\nOutro"},
		{"into the inner view", []ui.SelectedText{intro, part(inner, 1, read, 0, 11)},
			"Intro\n\nRead [the gu](https://example.test/guide)"},
		{"from the inner view", []ui.SelectedText{part(inner, 1, read, 9, len(read)), outro},
			"[guide](https://example.test/guide)\n\nOutro"},
		{"within the inner view", []ui.SelectedText{part(inner, 1, read, 5, 8)},
			"[the](https://example.test/guide)"},
		{"the outer view's alone", []ui.SelectedText{intro, outro},
			"Intro\n\n> Read [the guide](https://example.test/guide)\n\nOutro"},
	} {
		if got := outer.copyMarkdown(c.parts); got != c.want {
			t.Errorf("%s: copied %q, want %q", c.name, got, c.want)
		}
	}
}
