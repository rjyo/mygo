package editor

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// undoes checks that Undo makes want of the text, and Redo makes the
// text it had again, and undoes again.
func undoes(t *testing.T, b *Buffer, want string) {
	t.Helper()
	text := b.String()
	if !b.Undo() {
		t.Fatalf("Undo of %q: no step", text)
	}
	if got := b.String(); got != want {
		t.Fatalf("Undo of %q = %q, want %q", text, got, want)
	}
	if !b.Redo() {
		t.Fatal("Redo: no step")
	}
	if got := b.String(); got != text {
		t.Fatalf("Redo = %q, want %q", got, text)
	}
	b.Undo()
}

// A remote edit stays when the steps around it are taken back.
func TestBufferEditRemote(t *testing.T) {
	t.Run("before a step", func(t *testing.T) {
		b := NewBuffer("hello world")
		b.Edit(Edit{Start: 11, End: 11, Text: "!"})
		b.EditRemote(Edit{Start: 0, End: 0, Text: "Say: "})
		if got := b.String(); got != "Say: hello world!" {
			t.Fatalf("text %q", got)
		}
		undoes(t, b, "Say: hello world")
		if b.CanUndo() {
			t.Fatal("the remote edit is a step of undo")
		}
	})
	t.Run("after a step", func(t *testing.T) {
		b := NewBuffer("hello world")
		b.Edit(Edit{Start: 0, End: 0, Text: "Oh, "})
		b.EditRemote(Edit{Start: 15, End: 15, Text: "!"})
		undoes(t, b, "hello world!")
	})
	t.Run("on both sides of a step", func(t *testing.T) {
		b := NewBuffer("hello world")
		b.Edit(Edit{Start: 5, End: 5, Text: "XY"})
		// Insertions at its edges are outside it.
		b.EditRemote(Edit{Start: 5, End: 5, Text: "<"}, Edit{Start: 7, End: 7, Text: ">"})
		if got := b.String(); got != "hello<XY> world" {
			t.Fatalf("text %q", got)
		}
		undoes(t, b, "hello<> world")
	})
	t.Run("deleted text comes back where it was", func(t *testing.T) {
		b := NewBuffer("one two three")
		b.Edit(Edit{Start: 3, End: 7})
		b.EditRemote(Edit{Start: 0, End: 0, Text: ">> "}, Edit{Start: 9, End: 9, Text: " <<"})
		undoes(t, b, ">> one two three <<")
	})
	t.Run("a remote edit at the place of a deletion", func(t *testing.T) {
		b := NewBuffer("one two three")
		b.Edit(Edit{Start: 3, End: 7}) // "one three"
		b.EditRemote(Edit{Start: 3, End: 3, Text: ","})
		undoes(t, b, "one, two three")
		// Replacing the text after it.
		b = NewBuffer("one two three")
		b.Edit(Edit{Start: 3, End: 7})
		b.EditRemote(Edit{Start: 4, End: 9, Text: "3"})
		undoes(t, b, "one two 3")
	})
	t.Run("overlapping a step", func(t *testing.T) {
		b := NewBuffer("hello world")
		b.Edit(Edit{Start: 5, End: 5, Text: " there"})
		b.EditRemote(Edit{Start: 8, End: 9, Text: "E"})
		if got := b.String(); got != "hello thEre world" {
			t.Fatalf("text %q", got)
		}
		if b.CanUndo() || b.Undo() {
			t.Fatal("a step the remote edit overlaps is still in the history")
		}
		// Taking a deletion's place away.
		b = NewBuffer("abcdef")
		b.Edit(Edit{Start: 2, End: 4})
		b.EditRemote(Edit{Start: 1, End: 3})
		if got := b.String(); got != "af" || b.CanUndo() {
			t.Fatalf("text %q, can undo %v", got, b.CanUndo())
		}
	})
	t.Run("only the step overlapping leaves", func(t *testing.T) {
		b := NewBuffer("hello world")
		b.Edit(Edit{Start: 0, End: 0, Text: "1 "})
		b.Edit(Edit{Start: 13, End: 13, Text: " 2"})
		b.Edit(Edit{Start: 0, End: 0, Text: "3 "})
		// Over the second step's text and the end of the text before it.
		b.EditRemote(Edit{Start: 10, End: 17, Text: "earth 4"})
		if got := b.String(); got != "3 1 hello earth 4" {
			t.Fatalf("text %q", got)
		}
		undoes(t, b, "1 hello earth 4")
		undoes(t, b, "hello earth 4")
		if b.CanUndo() {
			t.Fatal("the second step is still in the history")
		}
	})
	t.Run("a step touching one that left", func(t *testing.T) {
		b := NewBuffer("hello world")
		b.Edit(Edit{Start: 11, End: 11, Text: " and"})
		b.Edit(Edit{Start: 15, End: 15, Text: " more"})
		b.EditRemote(Edit{Start: 17, End: 18, Text: "O"})
		// The second step's text stays, the first's goes.
		undoes(t, b, "hello world mOre")
	})
	t.Run("several in a row", func(t *testing.T) {
		b := NewBuffer("a b c")
		b.Edit(Edit{Start: 1, End: 1, Text: "1"}) // a1 b c
		b.Edit(Edit{Start: 4, End: 4, Text: "2"}) // a1 b2 c
		b.Edit(Edit{Start: 7, End: 7, Text: "3"}) // a1 b2 c3
		b.EditRemote(Edit{Start: 0, End: 0, Text: "["})
		b.EditRemote(Edit{Start: 9, End: 9, Text: "]"})
		b.EditRemote(Edit{Start: 4, End: 5, Text: "B"}, Edit{Start: 7, End: 8, Text: "C"})
		if got := b.String(); got != "[a1 B2 C3]" {
			t.Fatalf("text %q", got)
		}
		for _, want := range []string{"[a1 B2 C]", "[a1 B C]", "[a B C]"} {
			if !b.Undo() || b.String() != want {
				t.Fatalf("undo = %q, want %q", b.String(), want)
			}
		}
		for _, want := range []string{"[a1 B C]", "[a1 B2 C]", "[a1 B2 C3]"} {
			if !b.Redo() || b.String() != want {
				t.Fatalf("redo = %q, want %q", b.String(), want)
			}
		}
	})
	t.Run("whole text", func(t *testing.T) {
		b := NewBuffer("hello")
		b.Edit(Edit{Start: 5, End: 5, Text: "!"})
		b.EditRemote(Edit{Start: 0, End: 6, Text: "bye"})
		if b.CanUndo() {
			t.Fatal("a remote edit of the whole text left a step")
		}
	})
}

// Undo and redo take back and make again only the buffer's own steps,
// with remote edits before, between and after them.
func TestBufferEditRemoteRedo(t *testing.T) {
	b := NewBuffer("hello")
	b.Edit(Edit{Start: 5, End: 5, Text: " world"})
	b.Undo()
	b.EditRemote(Edit{Start: 0, End: 0, Text: "> "})
	if !b.CanRedo() {
		t.Fatal("a remote edit dropped the redo steps")
	}
	b.Redo()
	if got := b.String(); got != "> hello world" {
		t.Fatalf("redo %q", got)
	}
	undoes(t, b, "> hello")
	// A remote edit at the place a redo step inserts at goes before it,
	// as does one ending there.
	b.EditRemote(Edit{Start: 7, End: 7, Text: "!"})
	b.Redo()
	if got := b.String(); got != "> hello! world" {
		t.Fatalf("redo %q", got)
	}
	b.Undo()
	b.EditRemote(Edit{Start: 5, End: 8, Text: "p"})
	b.Redo()
	if got := b.String(); got != "> help world" {
		t.Fatalf("redo %q", got)
	}
	undoes(t, b, "> help")
	// One overlapping it drops it.
	b = NewBuffer("hello world")
	b.Edit(Edit{Start: 5, End: 6})
	b.Undo()
	b.EditRemote(Edit{Start: 4, End: 7, Text: "O W"})
	if got := b.String(); got != "hellO World" || b.CanRedo() {
		t.Fatalf("text %q, can redo %v", got, b.CanRedo())
	}
	// The redo step under one that left still applies, to the text
	// without it.
	b = NewBuffer("abcd")
	b.Edit(Edit{Start: 3, End: 3, Text: "X"})
	b.Edit(Edit{Start: 0, End: 0, Text: "Y"})
	b.Undo()
	b.Undo()
	b.EditRemote(Edit{Start: 2, End: 4, Text: "__"})
	if !b.Redo() || b.String() != "Yab__" || b.CanRedo() {
		t.Fatalf("redo %q, can redo %v", b.String(), b.CanRedo())
	}
	// A local edit still drops them.
	b = NewBuffer("abc")
	b.Edit(Edit{Start: 3, End: 3, Text: "d"})
	b.Undo()
	b.EditRemote(Edit{Start: 0, End: 0, Text: "_"})
	b.Edit(Edit{Start: 0, End: 0, Text: "x"})
	if b.CanRedo() {
		t.Fatal("redo after a local edit")
	}
}

// A Transact's edits stay one step around a remote edit, and a burst of
// typing one group.
func TestBufferEditRemoteGroups(t *testing.T) {
	b := NewBuffer("abc")
	b.Transact(func() {
		b.Edit(Edit{Start: 0, End: 0, Text: "1"})
		b.EditRemote(Edit{Start: 4, End: 4, Text: "R"})
		b.Edit(Edit{Start: 1, End: 1, Text: "2"})
	})
	if got := b.String(); got != "12abcR" {
		t.Fatalf("text %q", got)
	}
	undoes(t, b, "abcR")
	b.Undo()
	if b.CanUndo() {
		t.Fatal("more than one step")
	}
	// Overlapping the step a Transact makes: what it edits next is a step
	// of its own.
	b = NewBuffer("abc")
	b.Transact(func() {
		b.Edit(Edit{Start: 3, End: 3, Text: "xyz"})
		b.EditRemote(Edit{Start: 4, End: 5, Text: "Y"})
		b.Edit(Edit{Start: 0, End: 0, Text: ">"})
		b.Edit(Edit{Start: 1, End: 1, Text: ">"})
	})
	undoes(t, b, "abcxYz")
	b.Undo()
	if b.CanUndo() {
		t.Fatal("the overlapped step is still in the history")
	}

	// Typing an input method's composition is one group however long it
	// takes, as long as it goes on.
	compose := func(b *Buffer, e Edit) {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.edit([]Edit{e}, editCompose, "owner", nil)
	}
	b = NewBuffer("abc")
	compose(b, Edit{Start: 3, End: 3, Text: "x"})
	b.EditRemote(Edit{Start: 0, End: 0, Text: "_"})
	compose(b, Edit{Start: 5, End: 5, Text: "y"})
	undoes(t, b, "_abc")
	b.Undo()
	if b.CanUndo() {
		t.Fatal("typing around a remote edit made two steps")
	}
	// A remote edit overlapping the group ends it.
	b = NewBuffer("abc")
	compose(b, Edit{Start: 3, End: 3, Text: "xy"})
	b.EditRemote(Edit{Start: 4, End: 4, Text: "_"})
	compose(b, Edit{Start: 6, End: 6, Text: "z"})
	undoes(t, b, "abcx_y")
	b.Undo()
	if b.CanUndo() {
		t.Fatal("the overlapped group is still in the history")
	}
}

// Each version undo and redo log after a remote edit has its changes'
// text, as an incremental parser needs.
func TestBufferEditRemoteLog(t *testing.T) {
	b := NewBuffer("one\ntwo\nthree\n")
	b.Transact(func() {
		b.Edit(Edit{Start: 0, End: 3, Text: "1\n1"})
		b.Edit(Edit{Start: 8, End: 11, Text: "2"})
		b.Edit(Edit{Start: 10, End: 10, Text: "x\n"})
	})
	b.EditRemote(Edit{Start: 4, End: 4, Text: "remote\n"}, Edit{Start: 20, End: 20, Text: "!"})
	text, v := b.snapshot()
	b.Undo()
	b.Redo()
	b.Undo()
	entries, _, ok := b.entriesSince(v)
	if !ok || len(entries) != 9 {
		t.Fatalf("%d entries, %v", len(entries), ok)
	}
	for _, e := range entries {
		for _, c := range e.changes {
			if c.oldEnd > text.Len() || c.newEnd > e.text.Len() {
				t.Fatalf("change %+v out of the texts", c)
			}
			if text.LineOf(c.oldStart) != c.oldLine || text.LineOf(c.oldEnd) != c.oldEndLine ||
				e.text.LineOf(c.newStart) != c.newLine || e.text.LineOf(c.newEnd) != c.newEndLine {
				t.Fatalf("change %+v: lines of %q to %q", c, text, e.text)
			}
		}
		// The text outside the changes is the text before them.
		old := 0
		var want strings.Builder
		for _, c := range e.changes {
			want.WriteString(text.Slice(old, c.oldStart))
			want.WriteString(e.text.Slice(c.newStart, c.newEnd))
			old = c.oldEnd
		}
		want.WriteString(text.Slice(old, text.Len()))
		if want.String() != e.text.String() {
			t.Fatalf("entry %d's text %q is not its changes' %q", e.version, e.text, want.String())
		}
		text = e.text
	}
	if got := b.String(); got != "one\nremote\ntwo\nthree\n!" {
		t.Fatalf("undo = %q", got)
	}
}

// Modified compares with the saved text, whatever made it.
func TestBufferEditRemoteModified(t *testing.T) {
	b := NewBuffer("a")
	b.EditRemote(Edit{Start: 1, End: 1, Text: "b"})
	if !b.Modified() {
		t.Fatal("not modified after a remote edit")
	}
	b.MarkSaved()
	b.Edit(Edit{Start: 0, End: 0, Text: "x"})
	if !b.Modified() {
		t.Fatal("not modified")
	}
	b.Undo()
	if b.Modified() {
		t.Fatal("modified, undone back to the saved text")
	}
	// Texts of one length, in chunks that break apart differently.
	long := largeText(2000)
	b = NewBuffer(long)
	b.Edit(Edit{Start: 0, End: 1, Text: "F"})
	b.Edit(Edit{Start: len(long) / 2, End: len(long)/2 + 1, Text: "x"})
	if !b.Modified() {
		t.Fatal("not modified")
	}
	b.Undo()
	b.Edit(Edit{Start: 0, End: 1, Text: "f"})
	if b.Modified() {
		t.Fatalf("modified with the saved text")
	}
}

// Remote edits anywhere, between local steps that each insert or delete
// a run of a character of its own, then undo and redo: each takes back
// or makes again one step, the run of its character, and touches
// nothing else, whatever the remote edits did.
func TestBufferEditRemoteRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	const ascii = "abcdefghij \n"
	word := func(n int) string {
		var s strings.Builder
		for range n {
			s.WriteByte(ascii[r.IntN(len(ascii))])
		}
		return s.String()
	}
	// at returns a random boundary of a character of text.
	at := func(text string) int {
		i := r.IntN(len(text) + 1)
		for i < len(text) && !utf8.RuneStart(text[i]) {
			i++
		}
		return i
	}
	// asciiRun returns a range of text holding only ASCII.
	asciiRun := func(text string) (int, int) {
		start := at(text)
		end := start
		for n := r.IntN(6); n > 0 && end < len(text) && text[end] < utf8.RuneSelf; n-- {
			end++
		}
		return start, end
	}
	// oneRun reports whether long is short with a run of one character
	// put in, found nowhere else.
	oneRun := func(short, long string) bool {
		s, l := []rune(short), []rune(long)
		if len(l) <= len(s) {
			return false
		}
		i := 0
		for i < len(s) && s[i] == l[i] {
			i++
		}
		j := 0
		for j < len(s)-i && s[len(s)-1-j] == l[len(l)-1-j] {
			j++
		}
		mid := l[i : len(l)-j]
		c := mid[0]
		return c >= utf8.RuneSelf && strings.Count(string(mid), string(c)) == len(mid) &&
			strings.Count(long, string(c)) == len(mid) && !strings.ContainsRune(short, c)
	}
	steps := 0
	for round := range 300 {
		b := NewBuffer(word(30))
		next := rune(0x4e00)
		var did []string
		check := func(op string, before string, ok bool) {
			after := b.String()
			did = append(did, fmt.Sprintf("%s: %q", op, after))
			if !ok {
				if after != before {
					t.Fatalf("round %d: %s changed the text: %s", round, op, strings.Join(did, "\n"))
				}
				return
			}
			steps++
			if !oneRun(before, after) && !oneRun(after, before) {
				t.Fatalf("round %d: %s of %q made %q:\n%s", round, op, before, after, strings.Join(did, "\n"))
			}
		}
		for range 80 {
			text := b.String()
			switch r.IntN(8) {
			case 0, 1: // a step putting in a run of its character
				i := at(text)
				b.Edit(Edit{Start: i, End: i, Text: strings.Repeat(string(next), 1+r.IntN(3))})
				next++
				did = append(did, fmt.Sprintf("insert: %q", b.String()))
			case 2: // a step deleting the run of a character
				for _, c := range text {
					i, j := strings.IndexRune(text, c), strings.LastIndex(text, string(c))+utf8.RuneLen(c)
					if c >= utf8.RuneSelf && strings.Count(text[i:j], string(c))*utf8.RuneLen(c) == j-i {
						b.Edit(Edit{Start: i, End: j})
						did = append(did, fmt.Sprintf("delete: %q", b.String()))
						break
					}
				}
			case 3, 4: // remote edits, of ASCII only
				var edits []Edit
				for range 1 + r.IntN(2) {
					i, j := asciiRun(text)
					edits = append(edits, Edit{Start: i, End: j, Text: word(r.IntN(4))})
				}
				if edits[0].Start > edits[len(edits)-1].Start {
					slices.Reverse(edits)
				}
				if len(edits) == 2 && edits[0].End > edits[1].Start {
					edits = edits[:1]
				}
				b.EditRemote(edits...)
				did = append(did, fmt.Sprintf("remote %v: %q", edits, b.String()))
			case 5, 6:
				check("undo", text, b.Undo())
			case 7:
				check("redo", text, b.Redo())
			}
		}
		// Back to the start: the steps left take back their runs.
		for b.CanUndo() {
			text := b.String()
			check("undo", text, b.Undo())
		}
	}
	if steps < 1000 {
		t.Fatalf("only %d steps taken back or made again", steps)
	}
}

// rebaseEdits gives edits and spans that make one text either way, and
// composeSpans spans that keep what both keep.
func TestRebaseSpans(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	// apply makes text of x with edits.
	apply := func(x string, es []Edit) string {
		var s strings.Builder
		last := 0
		for _, e := range es {
			s.WriteString(x[last:e.Start])
			s.WriteString(e.Text)
			last = e.End
		}
		s.WriteString(x[last:])
		return s.String()
	}
	// edits returns sorted edits of a text of n bytes that do not
	// overlap, inserting letters from letter.
	edits := func(n int, letter byte) []Edit {
		var out []Edit
		at := 0
		for at <= n && r.IntN(4) > 0 {
			start := at + r.IntN(4)
			if start > n {
				break
			}
			end := min(n, start+r.IntN(3))
			out = append(out, Edit{Start: start, End: end, Text: strings.Repeat(string(letter), r.IntN(3))})
			at = end
			if start == end && out[len(out)-1].Text == "" {
				out = out[:len(out)-1]
			}
		}
		return out
	}
	spansOf := func(es []Edit) []span {
		out := make([]span, len(es))
		for i, e := range es {
			out[i] = span{e.Start, e.End, len(e.Text)}
		}
		return out
	}
	both := 0
	for range 20000 {
		x := "0123456789abcdef"[:4+r.IntN(12)]
		es, cs := edits(len(x), 'E'), edits(len(x), 'C')
		es2, c2, ok := rebaseEdits(es, spansOf(cs), nil, nil)
		if !ok {
			overlap := false
			for _, e := range es {
				for _, c := range cs {
					overlap = overlap || c.Start < e.End && e.Start < c.End
				}
			}
			if !overlap {
				t.Fatalf("%v and %v overlap", es, cs)
			}
			continue
		}
		both++
		// c's texts are the same either way.
		cs2 := make([]Edit, len(cs))
		for i, s := range c2 {
			cs2[i] = Edit{Start: s.start, End: s.end, Text: cs[i].Text}
		}
		if a, b := apply(apply(x, cs), es2), apply(apply(x, es), cs2); a != b {
			t.Fatalf("%q: %v then %v = %q, %v then %v = %q", x, cs, es2, a, es, cs2, b)
		}
		// Composed, as an edit dropped from the history and then a remote
		// one: what neither replaces stays.
		y := apply(x, es)
		bs := edits(len(y), 'B')
		z := apply(y, bs)
		comp := composeSpans(spansOf(es), spansOf(bs))
		last, d := 0, 0
		for i, s := range comp {
			if s.start < last || s.end < s.start || i > 0 && s.start == last && comp[i-1].end == last {
				t.Fatalf("spans %v out of order", comp)
			}
			if x[last:s.start] != z[last+d:s.start+d] {
				t.Fatalf("%q, %v, %v: %v lose %q", x, es, bs, comp, x[last:s.start])
			}
			d += s.n - (s.end - s.start)
			last = s.end
		}
		if x[last:] != z[last+d:] {
			t.Fatalf("%q, %v, %v: %v lose %q", x, es, bs, comp, x[last:])
		}
	}
	if both < 2000 {
		t.Fatalf("only %d edits moved", both)
	}
}

// Editors showing a buffer follow a remote edit, and their undo takes
// back their own steps with the selections they had, moved through it.
func TestCodeEditorEditRemote(t *testing.T) {
	src := "package main\n\nfunc f() {\n\treturn\n}\n"
	buf := NewBuffer(src)
	buf.SetLanguage(LanguageNamed("go"))
	a, b := &State{Buffer: buf}, &State{Buffer: buf}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() {
			View(c, a).Grow(1).AutoFocus()
			View(c, b).Grow(1)
		})
	}, 800, 300)
	ret, fn := strings.Index(src, "return"), strings.Index(src, "f()")
	a.Select(ret, ret+len("return"))
	b.Select(0, len("package"))
	tt.Frame()
	b.Fold(2)
	a.SetDiagnostics(buf.Version(), []Diagnostic{{Start: fn, End: fn + 3, Message: "unused"}})
	a.SetInlays([]Inlay{{Offset: fn + 2, Text: "x int"}})
	tt.Type("panic")
	tt.Frame()

	const remote = "// Remote.\n"
	buf.EditRemote(Edit{Start: 0, End: 0, Text: remote})
	tt.Frame()
	n := len(remote)
	if s := a.Selection(); s.Head != ret+n+len("panic") || !s.Empty() {
		t.Fatalf("caret %+v", s)
	}
	if s := b.Selection(); s.Start() != n || s.End() != n+len("package") {
		t.Fatalf("other editor's selection %+v", s)
	}
	if f := b.Folded(); !slices.Equal(f, []int{3}) {
		t.Fatalf("folded %v", f)
	}
	if d := a.Diagnostics(); len(d) != 1 || d[0].Start != fn+n || d[0].End != fn+n+3 {
		t.Fatalf("diagnostics %+v", d)
	}
	if in := a.ed.view.inlays; len(in) != 1 || in[0].at != fn+n+2 {
		t.Fatalf("inlays %+v", in)
	}

	// Undo takes back the typing alone, and selects what it replaced.
	tt.Key(ui.Cmd, ui.KeyZ)
	if got := buf.String(); got != remote+src {
		t.Fatalf("undo %q", got)
	}
	if s := a.Selection(); s.Start() != ret+n || s.End() != ret+n+len("return") {
		t.Fatalf("selection after undo %+v", s)
	}
	buf.EditRemote(Edit{Start: 0, End: n})
	tt.Frame()
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	if got, want := buf.String(), strings.Replace(src, "return", "panic", 1); got != want {
		t.Fatalf("redo %q, want %q", got, want)
	}
	if s := a.Selection(); s.Head != ret+len("panic") || !s.Empty() {
		t.Fatalf("caret after redo %+v", s)
	}
	buf.Undo()
	if buf.CanUndo() {
		t.Fatal("the remote edits are steps of undo")
	}
}

// A remote edit in the middle of a large document with a full history
// keeps every step, which then takes back its own edit.
func TestBufferEditRemoteLarge(t *testing.T) {
	src := largeText(100_000)
	b := NewBuffer(src)
	for i := range undoMax {
		at := b.Offset(Pos{Line: i * 10, Col: 5})
		b.Edit(Edit{Start: at, End: at + 2, Text: "abc"})
	}
	mid := b.Offset(Pos{Line: 50_000})
	b.EditRemote(Edit{Start: mid, End: mid, Text: "// remote\n"})
	steps := 0
	for b.Undo() {
		steps++
	}
	at := strings.Index(src, "func f50000(")
	if want := src[:at] + "// remote\n" + src[at:]; steps != undoMax || b.String() != want {
		t.Fatalf("%d steps taken back, text right %v", steps, b.String() == want)
	}
}

// A step's selections are of the texts before and after it, which a
// remote edit reaches at different offsets.
func TestBufferEditRemoteSelections(t *testing.T) {
	caret := func(at int) []sel { return []sel{{anchor: at, head: at, goal: -1}} }
	// A step putting XX at the start of the text, with a caret at 6 before
	// and at 6 after it.
	step := func() *Buffer {
		b := NewBuffer("aaaa bbbb cccc")
		b.mu.Lock()
		b.edit([]Edit{{Start: 0, End: 0, Text: "XX"}}, editOther, "owner", caret(6))
		b.mu.Unlock()
		b.setSelectionsAfter("owner", caret(6))
		return b
	}
	check := func(t *testing.T, tx transaction, before, after int) {
		t.Helper()
		if got := tx.selBefore.([]sel)[0].head; got != before {
			t.Errorf("selection before the step at %d, want %d", got, before)
		}
		if got := tx.selAfter.([]sel)[0].head; got != after {
			t.Errorf("selection after the step at %d, want %d", got, after)
		}
	}
	t.Run("undo", func(t *testing.T) {
		b := step()
		// At 7 of the text after the step, 5 of the text before it: the
		// caret before it follows, the caret after it does not.
		b.EditRemote(Edit{Start: 7, End: 7, Text: "R"})
		check(t, b.undo[0], 7, 6)
	})
	t.Run("redo", func(t *testing.T) {
		b := step()
		b.Undo()
		// At 5 of the text before the step, which the redo applies to.
		b.EditRemote(Edit{Start: 5, End: 5, Text: "R"})
		check(t, b.redo[0], 7, 6)
	})
}

// The step a Transact is making stays open when a step under it leaves.
func TestBufferEditRemoteOpenStep(t *testing.T) {
	b := NewBuffer("abc")
	b.Edit(Edit{Start: 0, End: 0, Text: "xyz"})
	b.Transact(func() {
		b.Edit(Edit{Start: 6, End: 6, Text: "1"})
		b.EditRemote(Edit{Start: 1, End: 2, Text: "Y"})
		b.Edit(Edit{Start: 7, End: 7, Text: "2"})
	})
	if got := b.String(); got != "xYzabc12" {
		t.Fatalf("text %q", got)
	}
	undoes(t, b, "xYzabc")
	if b.CanUndo() {
		t.Fatal("the overlapped step is still in the history")
	}
}

// When a remote edit takes the group typing goes on in, the next edit
// makes a step, rather than joining the group under it.
func TestBufferEditRemoteEndsGroup(t *testing.T) {
	compose := func(b *Buffer, e Edit) {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.edit([]Edit{e}, editCompose, "owner", nil)
	}
	b := NewBuffer("abc")
	compose(b, Edit{Start: 0, End: 0, Text: "1"}) // 1abc
	b.endGroup()
	compose(b, Edit{Start: 4, End: 4, Text: "xy"}) // 1abcxy
	b.EditRemote(Edit{Start: 5, End: 5, Text: "_"})
	compose(b, Edit{Start: 7, End: 7, Text: "z"})
	undoes(t, b, "1abcx_y")
	undoes(t, b, "abcx_y")
	if b.CanUndo() {
		t.Fatal("the overlapped group is still in the history")
	}
}

// A remote edit in the middle of a large document, by the steps of
// history it moves.
func BenchmarkBufferEditRemote(b *testing.B) {
	for _, steps := range []int{0, 100, 1000, undoMax} {
		b.Run(fmt.Sprintf("steps=%d", steps), func(b *testing.B) {
			buf := NewBuffer(largeText(100_000))
			for i := range steps {
				at := buf.Offset(Pos{Line: i * (100_000 / max(steps, 1)), Col: 5})
				// With selections, as an editor's steps have.
				buf.mu.Lock()
				buf.edit([]Edit{{Start: at, End: at + 2, Text: "abc"}}, editOther, "owner", []sel{{anchor: at, head: at + 2, goal: -1}})
				buf.mu.Unlock()
				buf.setSelectionsAfter("owner", []sel{{anchor: at + 3, head: at + 3, goal: -1}})
			}
			// At the start of a line, which no step's edit is in.
			mid := buf.Offset(Pos{Line: 50_001})
			for b.Loop() {
				buf.EditRemote(Edit{Start: mid, End: mid, Text: "x"})
			}
			if buf.CanUndo() != (steps > 0) {
				b.Fatal("the steps left the history")
			}
		})
	}
}
