package editor

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

const testPatch = `diff --git a/greet.go b/greet.go
index 3b18e51..a2e8f0c 100644
--- a/greet.go
+++ b/greet.go
@@ -1,5 +1,6 @@ package main
 package main

-func greet() string {
-	return "hello"
+// greet greets.
+func greet(name string) string {
+	return "hello " + name
 }
diff --git a/old.txt b/new.txt
similarity 90%
rename from old.txt
rename to new.txt
--- a/old.txt
+++ b/new.txt
@@ -1 +1 @@
-a
\ No newline at end of file
+b
diff --git a/added.md b/added.md
new file mode 100644
--- /dev/null
+++ b/added.md
@@ -0,0 +1,2 @@
+# Title
+text
`

func TestParsePatch(t *testing.T) {
	files, err := ParsePatch(testPatch)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("%d files", len(files))
	}
	f := files[0]
	if f.OldName != "greet.go" || f.NewName != "greet.go" || len(f.Hunks) != 1 {
		t.Fatalf("first file %+v", f)
	}
	h := f.Hunks[0]
	if h.OldStart != 1 || h.OldLines != 5 || h.NewStart != 1 || h.NewLines != 6 || h.Section != "package main" || len(h.Lines) != 8 {
		t.Fatalf("hunk %+v", h)
	}
	old := "package main\n\nfunc greet() string {\n\treturn \"hello\"\n}\n"
	got, err := f.Apply(old)
	if err != nil {
		t.Fatal(err)
	}
	want := "package main\n\n// greet greets.\nfunc greet(name string) string {\n\treturn \"hello \" + name\n}\n"
	if got != want {
		t.Fatalf("applied %q", got)
	}
	r := files[1]
	if r.OldName != "old.txt" || r.NewName != "new.txt" {
		t.Fatalf("rename %+v", r)
	}
	if got, err := r.Apply("a"); err != nil || got != "b\n" {
		t.Fatalf("applied rename %q, %v", got, err)
	}
	n := files[2]
	if n.OldName != "" || n.NewName != "added.md" || n.NewMode != "100644" {
		t.Fatalf("new file %+v", n)
	}
	if got, err := n.Apply(""); err != nil || got != "# Title\ntext\n" {
		t.Fatalf("applied new file %q, %v", got, err)
	}
	if _, err := f.Apply("something else\n"); err == nil {
		t.Fatal("applied to the wrong text")
	}
}

// A patch shows without the files, numbered as they are.
func TestDiffViewerPatch(t *testing.T) {
	files, _ := ParsePatch(`--- a/x.go
+++ b/x.go
@@ -10,3 +10,3 @@ func f() {
 	a()
-	b()
+	c()
 	d()
@@ -40,2 +40,3 @@
 	e()
+	f()
 }
`)
	d := &DiffState{}
	d.SetPatch(files[0])
	tt := ui.NewTester(func(c *ui.Context) {
		DiffView(c, d).Fill()
	}, 800, 400)
	tt.Frame()
	if hs := d.Hunks(); len(hs) != 2 {
		t.Fatalf("hunks %+v", hs)
	}
	if n := d.sides[1].number(3); n != 40 {
		t.Fatalf("line 3 of the new text is line %d of the file", n)
	}
	if n := d.sides[0].number(1); n != 11 {
		t.Fatalf("line 1 of the old text is line %d of the file", n)
	}
	if d.New.Language() == nil || !strings.HasPrefix(d.sides[1].blks[0].id.(string), "@@ -10,3 +10,3 @@") {
		t.Fatalf("language %v, blocks %+v", d.New.Language(), d.sides[1].blks)
	}
}

// The patch of issue #1047: its first hunk ends in a doc comment.
const commentPatch = `diff --git a/compare.cpp b/compare.cpp
--- a/compare.cpp
+++ b/compare.cpp
@@ -1,7 +1,7 @@
 #include <cstdlib>
 #include <vector>
 
-int threshold = 24;
+int threshold = 32;
 
 /**
  * Compares two RGBA frames pixel by pixel.
@@ -27,3 +27,3 @@ int compare(...)
     // padding 9
-    for (int i = 0; i < cd.size() && i < fd.size(); i += 4) {
+    for (size_t i = 0; i < cd.size() && i < fd.size(); i += 4) {
         const int dr = std::abs(cd[i] - fd[i]);
`

// firstKind returns the kind of the first token of the line of b holding
// s, past its indentation, as b's highlighter makes it once done.
func firstKind(t *testing.T, b *Buffer, s string) TokenKind {
	t.Helper()
	line := -1
	for i := range b.LineCount() {
		if strings.Contains(b.Line(i), s) {
			line = i
			break
		}
	}
	if line < 0 {
		t.Fatalf("no line holds %q", s)
	}
	h := b.highlighter()
	if !h.Highlight(b.LineCount()-1, time.Hour) {
		t.Fatal("highlighter not done")
	}
	text := b.Line(line)
	toks, final := h.Tokens(line, text, nil)
	if !final {
		t.Fatalf("tokens of %q not final", text)
	}
	indent := len(text) - len(strings.TrimLeft(text, " \t"))
	for _, tk := range toks {
		if tk.End > indent {
			return tk.Kind
		}
	}
	t.Fatalf("no tokens past the indentation of %q: %+v", text, toks)
	return 0
}

// Each hunk of a patch is highlighted from state 0, not from the state
// the hunk before ends in (#1047).
func TestPatchHunksHighlightApart(t *testing.T) {
	files, err := ParsePatch(commentPatch)
	if err != nil {
		t.Fatal(err)
	}
	d := &DiffState{}
	d.SetPatch(files[0])
	if k := firstKind(t, d.New, "for (size_t"); k != TokenKeyword {
		t.Fatalf("new side's for is %v, want a keyword", k)
	}
	if k := firstKind(t, d.Old, "for (int"); k != TokenKeyword {
		t.Fatalf("old side's for is %v, want a keyword", k)
	}
	// The doc comment the first hunk ends in is one still.
	if k := firstKind(t, d.New, "Compares two"); k != TokenComment {
		t.Fatalf("doc comment is %v", k)
	}
	// So the views draw it.
	tt := ui.NewTester(func(c *ui.Context) {
		DiffView(c, d).Fill()
	}, 800, 400)
	tt.Frame()
	tt.Frame()
	n := d.New.LineCount() - 2
	if l := d.eds[1].view.line(n); !strings.Contains(l.text, "for (size_t") || len(l.toks) == 0 || l.toks[0].Kind == TokenComment {
		t.Fatalf("line %d %q drawn with tokens %+v", n, l.text, l.toks)
	}

	// Another language, as an app sets, starts the hunks afresh too.
	cpp := *d.New.Language()
	d.New.SetLanguage(&cpp)
	if k := firstKind(t, d.New, "for (size_t"); k != TokenKeyword {
		t.Fatalf("for is %v in another language", k)
	}

	// The hunks' first lines move with edits: lines added to the first
	// hunk, and its last line deleted.
	d.New.Edit(Edit{Start: 0, Text: "/* a\n b */\n"})
	if k := firstKind(t, d.New, "for (size_t"); k != TokenKeyword {
		t.Fatalf("for is %v after lines added above", k)
	}
	at := d.New.Offset(Pos{Line: d.New.LineCount() - 4})
	d.New.Edit(Edit{Start: at, End: d.New.Offset(Pos{Line: d.New.LineCount() - 3}), Text: ""})
	if k := firstKind(t, d.New, "for (size_t"); k != TokenKeyword || strings.Contains(d.New.String(), "Compares") {
		t.Fatalf("for is %v after the comment's line went: %q", k, d.New.String())
	}
	// A new highlighter, after edits, takes the lines where they are.
	d.New.SetLanguage(LanguageFor("compare.cpp"))
	if k := firstKind(t, d.New, "for (size_t"); k != TokenKeyword {
		t.Fatalf("for is %v in a highlighter made after the edits", k)
	}
	// An edit opening a comment in the first hunk stops at the second,
	// tokenizing no line past its first.
	calls := 0
	counted := *LanguageFor("compare.cpp")
	inner := counted.Tokenizer
	counted.Tokenizer = TokenizerFunc(func(line string, state uint64, toks []Token) ([]Token, uint64) {
		calls++
		return inner.Tokenize(line, state, toks)
	})
	d.New.SetLanguage(&counted)
	firstKind(t, d.New, "for (size_t")
	second := 0
	for i := range d.New.LineCount() {
		if strings.Contains(d.New.Line(i), "padding 9") {
			second = i
		}
	}
	at = d.New.Offset(Pos{Line: 2})
	d.New.Edit(Edit{Start: at, End: at, Text: "/* "})
	calls = 0
	h := d.New.highlighter()
	h.Highlight(d.New.LineCount()-1, time.Hour)
	if calls != second-2 {
		t.Fatalf("tokenized %d lines after the edit, want the %d from it to the second hunk", calls, second-2)
	}
	if k := firstKind(t, d.New, "padding 9"); k != TokenComment {
		t.Fatalf("second hunk's first line is %v", k)
	}
	if k := firstKind(t, d.New, "for (size_t"); k != TokenKeyword {
		t.Fatalf("for is %v after a comment opened in the first hunk", k)
	}
	if k := firstKind(t, d.New, "include <vector>"); k != TokenComment {
		t.Fatalf("the first hunk is %v under the comment opened", k)
	}
}

// A string over lines stops at the end of its hunk too.
func TestPatchHunksHighlightApartStrings(t *testing.T) {
	files, err := ParsePatch(`--- a/app.py
+++ b/app.py
@@ -1,4 +1,4 @@
-import os
+import sys
 
 def f():
     """Does f.
@@ -20,3 +20,3 @@ def f():
 def g():
-    return 1
+    return 2
`)
	if err != nil {
		t.Fatal(err)
	}
	d := &DiffState{}
	d.SetPatch(files[0])
	for _, b := range []*Buffer{d.Old, d.New} {
		if k := firstKind(t, b, "Does f"); k != TokenString {
			t.Fatalf("docstring is %v", k)
		}
		if k := firstKind(t, b, "def g"); k != TokenKeyword {
			t.Fatalf("def after the docstring's hunk is %v", k)
		}
		if k := firstKind(t, b, "return"); k != TokenKeyword {
			t.Fatalf("return is %v", k)
		}
	}
	// Texts of the app's are one text, highlighted as such.
	d.Old = NewBuffer(d.Old.String())
	d.Old.SetLanguage(LanguageFor("app.py"))
	if k := firstKind(t, d.Old, "def g"); k != TokenString {
		t.Fatalf("def in the docstring of a whole text is %v", k)
	}
	// A Language.Highlighter of the app's sees the patch's text whole,
	// with its hunks' starts, which NewTokenHighlighter restarts at.
	var seen Snapshot
	d.New.SetLanguage(&Language{Name: "Own", Highlighter: func(text Snapshot, _ func()) Highlighter {
		seen = text
		return NewTokenHighlighter(LanguageFor("app.py").Tokenizer, text)
	}})
	if k := firstKind(t, d.New, "def g"); k != TokenKeyword || seen.String() != d.New.String() {
		t.Fatalf("def is %v with the app's highlighter of %q", k, seen.String())
	}
}

// Lines restarting the highlighter move with their first byte, and go
// when an edit replaces it.
func TestMoveRestarts(t *testing.T) {
	text := "a\nb\nc\nd\ne"
	for _, c := range []struct {
		edit Edit
		want []int
	}{
		{Edit{Start: 0, End: 0, Text: "x\n"}, []int{0, 2, 5}},  // at the first restart: it stays
		{Edit{Start: 2, End: 2, Text: "x\n"}, []int{0, 1, 5}},  // at a restart's start: it stays
		{Edit{Start: 2, End: 4, Text: ""}, []int{0, 1, 3}},     // b's line, with its newline
		{Edit{Start: 1, End: 3, Text: ""}, []int{0, 3}},        // b's start, joined to a
		{Edit{Start: 0, End: 9, Text: "z"}, []int{0}},          // everything
		{Edit{Start: 6, End: 6, Text: "\n\n"}, []int{0, 1, 6}}, // after the last
	} {
		b := NewBuffer(text)
		b.setRestarts([]int{0, 1, 4})
		b.Edit(c.edit)
		if got := b.restarts; !slices.Equal(got, c.want) {
			t.Errorf("%+v: restarts %v, want %v", c.edit, got, c.want)
		}
	}
	// Undo moves them back.
	b := NewBuffer(text)
	b.setRestarts([]int{1, 4})
	b.Edit(Edit{Start: 0, Text: "x\ny\n"})
	b.Undo()
	if !slices.Equal(b.restarts, []int{1, 4}) {
		t.Errorf("restarts %v after undo", b.restarts)
	}
	// Two meeting are one.
	b = NewBuffer(text)
	b.setRestarts([]int{1, 2})
	b.Edit(Edit{Start: 2, End: 4})
	if !slices.Equal(b.restarts, []int{1}) {
		t.Errorf("restarts %v after the line between went", b.restarts)
	}
}

// recordingHighlighter is an app's highlighter keeping the snapshots it
// is given.
type recordingHighlighter struct{ texts []Snapshot }

func (h *recordingHighlighter) Edit(_, new Snapshot, _ []Change)                    { h.texts = append(h.texts, new) }
func (h *recordingHighlighter) Highlight(int, time.Duration) bool                   { return true }
func (h *recordingHighlighter) Tokens(_ int, _ string, dst []Token) ([]Token, bool) { return dst, true }

// A Language.Highlighter of the app's knows where a patch's hunks start,
// from its snapshots, as the edits move them.
func TestPatchRestartsOnSnapshot(t *testing.T) {
	files, err := ParsePatch(`--- a/app.py
+++ b/app.py
@@ -1,3 +1,3 @@
-import os
+import sys
 
 x = 1
@@ -20,2 +20,2 @@ def f():
-    return 1
+    return 2
`)
	if err != nil {
		t.Fatal(err)
	}
	d := &DiffState{}
	d.SetPatch(files[0])
	if got := d.New.Snapshot().Restarts(); !slices.Equal(got, []int{0, 3}) {
		t.Fatalf("snapshot's restarts %v, want [0 3]", got)
	}
	h := &recordingHighlighter{}
	d.New.SetLanguage(&Language{Name: "Own", Highlighter: func(text Snapshot, _ func()) Highlighter {
		h.texts = append(h.texts, text)
		return h
	}})
	d.New.highlighter()
	d.New.Edit(Edit{Start: 0, Text: "# top\n"})
	d.New.highlighter()
	if len(h.texts) != 2 {
		t.Fatalf("%d snapshots, want 2", len(h.texts))
	}
	for i, want := range [][]int{{0, 3}, {0, 4}} {
		if got := h.texts[i].Restarts(); !slices.Equal(got, want) {
			t.Errorf("snapshot %d: restarts %v, want %v", i, got, want)
		}
	}
	// A buffer of the app's own has none.
	if got := NewBuffer("a\nb").Snapshot().Restarts(); got != nil {
		t.Errorf("a plain buffer's restarts are %v", got)
	}
}

// With the files, a patch's changes are its runs of - and + lines, in the
// files' lines, though comparing the texts would align them otherwise: git
// deleted the first "x" here, not the second.
func TestSetPatchFilesHunks(t *testing.T) {
	files, _ := ParsePatch(`--- a/x.txt
+++ b/x.txt
@@ -1,4 +1,2 @@
-x
-y
 x
 z
`)
	d := &DiffState{}
	if err := d.SetPatchFiles(files[0], "x\ny\nx\nz\n", "x\nz\n"); err != nil {
		t.Fatal(err)
	}
	if hs := d.Hunks(); !slices.Equal(hs, []DiffHunk{{0, 2, 0, 0}}) {
		t.Fatalf("hunks %+v, want the patch's", hs)
	}
	if d.Old.String() != "x\ny\nx\nz\n" || d.New.String() != "x\nz\n" {
		t.Fatalf("texts %q, %q", d.Old.String(), d.New.String())
	}
}

// linesPatch returns lines, "line 0" to "line n-1", as a file's text, and
// the patch changing each of at to "changed", with three lines of context.
func linesPatch(n int, at ...int) (old, new string, p FilePatch) {
	a := make([]string, n)
	for i := range a {
		a[i] = "line " + strconv.Itoa(i)
	}
	b := slices.Clone(a)
	var s strings.Builder
	s.WriteString("--- a/x.txt\n+++ b/x.txt\n")
	for _, i := range at {
		b[i] = "changed"
		lo, hi := max(0, i-3), min(n, i+4)
		fmt.Fprintf(&s, "@@ -%d,%d +%d,%d @@\n", lo+1, hi-lo, lo+1, hi-lo)
		for k := lo; k < hi; k++ {
			if k == i {
				s.WriteString("-" + a[k] + "\n+changed\n")
			} else {
				s.WriteString(" " + a[k] + "\n")
			}
		}
	}
	files, _ := ParsePatch(s.String())
	return strings.Join(a, "\n") + "\n", strings.Join(b, "\n") + "\n", files[0]
}

// With the files, the unchanged lines around a patch's changes collapse
// into separators, as in any diff, and expand to the files' lines.
func TestSetPatchFilesCollapses(t *testing.T) {
	old, new, p := linesPatch(100, 20, 60)
	d := &DiffState{}
	if err := d.SetPatchFiles(p, old, new); err != nil {
		t.Fatal(err)
	}
	seps := map[int]*DiffSeparator{} // by the old line the gap starts at
	d.RenderSeparator = func(c *ui.Context, sep ui.Element, s *DiffSeparator) {
		seps[s.a] = s
		DefaultSeparator(c, sep, s)
	}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 2000)
	tt.Frame()
	tt.Frame()
	for _, start := range []int{0, 21, 61} {
		if s, ok := seps[start]; !ok || s.Header != "" {
			t.Fatalf("no separator of the gap at %d: %v", start, seps)
		}
	}
	if top, mid := d.LineTop(DiffNew, 5), d.LineTop(DiffNew, 10); top != mid {
		t.Fatalf("lines 5 and 10 at %v and %v, collapsed into one separator", top, mid)
	}
	seps[21].ExpandAll()
	tt.Frame()
	if top, next := d.LineTop(DiffNew, 40), d.LineTop(DiffNew, 41); next <= top {
		t.Fatalf("expanded lines 40 and 41 at %v and %v", top, next)
	}
	if n := d.sides[1].number(40); n != 41 {
		t.Fatalf("line 40 of the new file numbered %d", n)
	}
	if hs := d.Hunks(); !slices.Equal(hs, []DiffHunk{{20, 21, 20, 21}, {60, 61, 60, 61}}) {
		t.Fatalf("hunks %+v", hs)
	}
}

// A file's lines end at its final newline, as git diff shows them: the
// empty line the editor starts after it neither counts among the lines
// that collapse nor shows once they expand.
func TestSetPatchFilesEndsAtTheFinalNewline(t *testing.T) {
	old, new, p := linesPatch(100, 20, 60)
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		d := &DiffState{Layout: layout}
		if err := d.SetPatchFiles(p, old, new); err != nil {
			t.Fatal(err)
		}
		seps := map[int]*DiffSeparator{}
		d.RenderSeparator = func(c *ui.Context, sep ui.Element, s *DiffSeparator) {
			seps[s.a] = s
			DefaultSeparator(c, sep, s)
		}
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 2000)
		tt.Frame()
		tt.Frame()
		// Lines 64 to 99 collapse after the change at 60.
		if s := seps[61]; s == nil || s.Lines != 36 {
			t.Fatalf("%v: last separator %+v, want 36 lines", layout, s)
		}
		seps[61].ExpandAll()
		tt.Frame()
		if !hides(d.sides[1].hidden(), 100) {
			t.Fatalf("%v: the new file's empty line after its final newline shows", layout)
		}
		if layout == DiffSplit && !hides(d.sides[0].hidden(), 100) {
			t.Fatalf("the old file's empty line after its final newline shows")
		}
		if hides(d.sides[1].hidden(), 99) {
			t.Fatalf("%v: the file's last line is hidden", layout)
		}
	}
}

// A patch that does not make the new file from the old, or a binary one,
// returns an error and leaves the diff as it was.
func TestSetPatchFilesMismatch(t *testing.T) {
	old, new, p := linesPatch(10, 4)
	d := &DiffState{Old: NewBuffer("a\n"), New: NewBuffer("b\n")}
	cases := []struct {
		name     string
		p        FilePatch
		old, new string
	}{
		{"another old file", p, "something else\n", new},
		{"another new file", p, old, new + "more\n"},
		{"binary", FilePatch{NewName: "x.png", Binary: true}, old, new},
	}
	for _, c := range cases {
		if err := d.SetPatchFiles(c.p, c.old, c.new); err == nil {
			t.Errorf("%s: no error", c.name)
		}
		if d.Old.String() != "a\n" || d.New.String() != "b\n" || len(d.Hunks()) != 1 {
			t.Fatalf("%s: the diff changed to %q, %q", c.name, d.Old.String(), d.New.String())
		}
	}
}

// Editing the new file compares again only the lines around the edit: the
// patch's changes elsewhere stay as git found them.
func TestSetPatchFilesEdit(t *testing.T) {
	tail := strings.Repeat("same\n", 40)
	files, _ := ParsePatch(`--- a/x.txt
+++ b/x.txt
@@ -1,5 +1,3 @@
-x
-y
 x
 z
 same
`)
	d := &DiffState{}
	if err := d.SetPatchFiles(files[0], "x\ny\nx\nz\n"+tail, "x\nz\n"+tail); err != nil {
		t.Fatal(err)
	}
	end := d.New.Len()
	d.New.Edit(Edit{Start: end, End: end, Text: "added\n"})
	if hs := d.Hunks(); !slices.Equal(hs, []DiffHunk{{0, 2, 0, 0}, {44, 44, 42, 43}}) {
		t.Fatalf("hunks %+v", hs)
	}
}

// A side of no lines starts after the line its header names: an added or
// deleted file, and the hunks of git diff -U0.
func TestSetPatchFilesEmptySides(t *testing.T) {
	cases := []struct {
		name, patch, old, new string
		want                  []DiffHunk
	}{
		{"added", "--- /dev/null\n+++ b/x.txt\n@@ -0,0 +1,2 @@\n+a\n+b\n", "", "a\nb\n", []DiffHunk{{0, 0, 0, 2}}},
		{"deleted", "--- a/x.txt\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-a\n-b\n", "a\nb\n", "", []DiffHunk{{0, 2, 0, 0}}},
		{"-U0", "--- a/x.txt\n+++ b/x.txt\n@@ -2 +1,0 @@\n-b\n@@ -3,0 +3 @@\n+d\n", "a\nb\nc\n", "a\nc\nd\n", []DiffHunk{{1, 2, 1, 1}, {3, 3, 2, 3}}},
	}
	for _, c := range cases {
		files, err := ParsePatch(c.patch)
		if err != nil {
			t.Fatal(err)
		}
		d := &DiffState{}
		if err := d.SetPatchFiles(files[0], c.old, c.new); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 400).Frame()
		if hs := d.Hunks(); !slices.Equal(hs, c.want) {
			t.Errorf("%s: hunks %+v, want %+v", c.name, hs, c.want)
		}
	}
}

// hides reports whether spans hide line.
func hides(spans []lineSpan, line int) bool {
	for _, s := range spans {
		if s.from <= line && line <= s.to {
			return true
		}
	}
	return false
}

// An editable diff shows the empty line after the final newline, where
// typing adds a line, from the frame Editable is set on.
func TestEditableDiffShowsTheLineAfterTheFinalNewline(t *testing.T) {
	d := &DiffState{Old: NewBuffer("a\nb\n"), New: NewBuffer("a\nc\n")}
	tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 400)
	tt.Frame()
	if !hides(d.sides[1].hidden(), 2) {
		t.Fatal("a read-only diff shows the line after the final newline")
	}
	d.Editable = true
	tt.Frame()
	if hides(d.sides[1].hidden(), 2) {
		t.Fatal("an editable diff hides the line after the final newline")
	}
}

// Lines deleted at the end of a file show, though the line after the new
// file's final newline, which they go before, does not.
func TestDiffShowsLinesDeletedAtTheEnd(t *testing.T) {
	for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
		d := &DiffState{Old: NewBuffer("a\nlast\n"), New: NewBuffer("a\n"), Layout: layout}
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 400)
		tt.Frame()
		tt.Frame()
		if !hides(d.sides[1].hidden(), 1) {
			t.Fatalf("%v: the line after the new file's final newline shows", layout)
		}
		// Two lines high: "a", and "last" deleted, or its filler.
		eds := d.eds[:]
		if layout == DiffUnified {
			eds = eds[1:]
		}
		for _, ed := range eds {
			if got, want := ed.view.heights.total(), 2*float64(ed.view.lineHeight); got != want {
				t.Fatalf("%v: a side %v high, want %v", layout, got, want)
			}
		}
	}
}

// A diff that is not editable compares lines with their line feeds, as
// git diff does: a last line gaining or losing its newline is changed,
// and the empty line an editor starts after a final newline is no line
// of the text. An editable diff shows that line, as typing there adds a
// line, so a newline added at the end adds it.
func TestReadOnlyDiffComparesTheFinalNewline(t *testing.T) {
	cases := []struct {
		old, new string
		want     []DiffHunk
	}{
		{"a\nb", "a\nb\n", []DiffHunk{{1, 2, 1, 2}}},
		{"a\nb\n", "a\nb", []DiffHunk{{1, 2, 1, 2}}},
		{"a\nb", "a\nc\n", []DiffHunk{{1, 2, 1, 2}}},
		{"a\nb\n", "a\nc", []DiffHunk{{1, 2, 1, 2}}},
		{"a\nb", "a\nc", []DiffHunk{{1, 2, 1, 2}}},
		{"a\nb\n", "a\nb\n\n", []DiffHunk{{2, 2, 2, 3}}},
		{"a\nb\nc\n", "a\n", []DiffHunk{{1, 3, 1, 1}}},
		{"", "\n", []DiffHunk{{0, 0, 0, 1}}},
		{"", "a", []DiffHunk{{0, 0, 0, 1}}},
		{"a", "a", nil},
		{"a\n", "a\n", nil},
	}
	for _, c := range cases {
		for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
			d := &DiffState{Old: NewBuffer(c.old), New: NewBuffer(c.new), Layout: layout}
			tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 400)
			tt.Frame()
			if hs := d.Hunks(); !slices.Equal(hs, c.want) {
				t.Errorf("%q to %q: hunks %+v, want %+v", c.old, c.new, hs, c.want)
			}
			// The empty line after a final newline never shows.
			if n := d.New.LineCount(); n > 1 && strings.HasSuffix(c.new, "\n") && !hides(d.sides[1].hidden(), n-1) {
				t.Errorf("%v: %q to %q shows the line after the new text's final newline", layout, c.old, c.new)
			}
		}
	}
	d := &DiffState{Old: NewBuffer("a\nb"), New: NewBuffer("a\nb\n"), Editable: true}
	if hs := d.Hunks(); !slices.Equal(hs, []DiffHunk{{2, 2, 2, 3}}) {
		t.Errorf("editable: hunks %+v, want the line after the newline added", hs)
	}
}

// endMark returns where a view marks its last line as having no final
// newline, and the mark, or -1.
func endMark(v *view) (int, string) {
	for _, in := range v.inlays {
		if strings.Contains(in.text, "No newline at end of file") {
			return in.at, in.text
		}
	}
	return -1, ""
}

// A diff that is not editable marks the last line of a side without a
// final newline, as git diff does, in either layout, whether or not the
// line changed; an editable one, or an empty text, has no mark.
func TestReadOnlyDiffMarksNoNewlineAtEnd(t *testing.T) {
	cases := []struct {
		old, new         string
		editable         bool
		oldMark, newMark int // the offsets marked, or -1
	}{
		{"a\nb", "a\nb\n", false, 3, -1},
		{"a\nb\n", "a\nc", false, -1, 3},
		{"a\nb", "a\nc", false, 3, 3},
		{"a\nb\n", "a\nc\n", false, -1, -1},
		{"", "a", false, -1, 1},
		{"a\nb", "a\nb\n", true, -1, -1},
	}
	for _, c := range cases {
		for _, layout := range []DiffLayout{DiffSplit, DiffUnified} {
			d := &DiffState{Old: NewBuffer(c.old), New: NewBuffer(c.new), Layout: layout, Editable: c.editable}
			tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 400)
			tt.Frame()
			tt.Frame()
			old := &d.eds[0].view
			if layout == DiffUnified {
				old = &d.oldView
			}
			if at, _ := endMark(old); at != c.oldMark {
				t.Errorf("%v: %q to %q (editable %v): old text marked at %d, want %d", layout, c.old, c.new, c.editable, at, c.oldMark)
			}
			if at, _ := endMark(&d.eds[1].view); at != c.newMark {
				t.Errorf("%v: %q to %q (editable %v): new text marked at %d, want %d", layout, c.old, c.new, c.editable, at, c.newMark)
			}
		}
	}
}

// A patch's own texts end without a newline: its "\ No newline at end of
// file" lines say which side's last line is marked.
func TestSetPatchMarksNoNewlineAtEnd(t *testing.T) {
	cases := []struct {
		patch            string
		oldMark, newMark bool
	}{
		{"--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+b\n", true, false},
		{"--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n a\n-b\n+c\n\\ No newline at end of file\n", false, true},
		{"--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n-a\n+c\n b\n\\ No newline at end of file\n", true, true},
		{"--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n a\n-b\n+c\n", false, false},
	}
	for _, c := range cases {
		files, err := ParsePatch(c.patch)
		if err != nil {
			t.Fatal(err)
		}
		d := &DiffState{}
		d.SetPatch(files[0])
		tt := ui.NewTester(func(c *ui.Context) { DiffView(c, d).Fill() }, 800, 400)
		tt.Frame()
		tt.Frame()
		if at, _ := endMark(&d.eds[0].view); (at == d.Old.Len()) != c.oldMark || at >= 0 && at != d.Old.Len() {
			t.Errorf("%q: old text marked at %d of %d, want marked %v", c.patch, at, d.Old.Len(), c.oldMark)
		}
		if at, _ := endMark(&d.eds[1].view); (at == d.New.Len()) != c.newMark || at >= 0 && at != d.New.Len() {
			t.Errorf("%q: new text marked at %d of %d, want marked %v", c.patch, at, d.New.Len(), c.newMark)
		}
	}
}

// Revert and Keep of a change of the final newline put it back or keep
// it, in a diff that is not editable.
func TestReadOnlyDiffRevertsTheFinalNewline(t *testing.T) {
	cases := []struct {
		old, new string
	}{
		{"a\nb", "a\nb\n"},
		{"a\nb\n", "a\nb"},
		{"a\nb", "a\nc\n"},
		{"a\nb\n", "a\nc"},
		{"a\nb\nc\n", "a\n"},
		{"a\n", "a\nb"},
	}
	for _, c := range cases {
		d := &DiffState{Old: NewBuffer(c.old), New: NewBuffer(c.new)}
		d.Revert(0)
		if got := d.New.String(); got != c.old {
			t.Errorf("%q to %q: reverted to %q", c.old, c.new, got)
		}
		d = &DiffState{Old: NewBuffer(c.old), New: NewBuffer(c.new)}
		d.Accept(0)
		if got := d.Old.String(); got != c.new {
			t.Errorf("%q to %q: kept as %q", c.old, c.new, got)
		}
	}
}

// Edits of a diff that is not editable, made by the app, compare again
// only the lines around them, with their line feeds: what a whole
// comparison of the texts finds.
func TestReadOnlyDiffEditsCompareTheFinalNewline(t *testing.T) {
	body := strings.Repeat("same\n", 10)
	cases := []struct {
		name, old, new string
		edit           func(n int) Edit
	}{
		{"newline added", body + "end", body + "end", func(n int) Edit { return Edit{Start: n, End: n, Text: "\n"} }},
		{"newline deleted", body + "end\n", body + "end\n", func(n int) Edit { return Edit{Start: n - 1, End: n} }},
		{"line added after the last", body + "end", body + "end", func(n int) Edit { return Edit{Start: n, End: n, Text: "\nend"} }},
		{"last line typed after the newline", body + "end\n", body + "end\n", func(n int) Edit { return Edit{Start: n, End: n, Text: "more"} }},
		{"blank line opened before the last", body + "\nend", body + "\nend", func(n int) Edit { return Edit{Start: n - 4, End: n - 4, Text: "\n"} }},
		{"last line repeated after the newline", body + "end", body + "end\n", func(n int) Edit { return Edit{Start: n, End: n, Text: "end"} }},
		{"first line edited, the newline changed at the end", "first\n" + body + "end\n", "first\n" + body + "end", func(int) Edit { return Edit{Start: 0, End: 5, Text: "First"} }},
		{"first line edited, the newline added at the end", "first\n" + body + "end", "first\n" + body + "end\n", func(int) Edit { return Edit{Start: 0, End: 5, Text: "First"} }},
	}
	for _, c := range cases {
		d := &DiffState{Old: NewBuffer(c.old), New: NewBuffer(c.new)}
		d.Hunks()
		d.New.Edit(c.edit(d.New.Len()))
		got := d.Hunks()
		want := (&DiffState{Old: NewBuffer(c.old), New: NewBuffer(d.New.String())}).Hunks()
		if !slices.Equal(got, want) {
			t.Errorf("%s: hunks %+v, a whole comparison's %+v", c.name, got, want)
		}
	}
}
