//go:build windows && (amd64 || arm64)

package text

import (
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Latin text wrapped from one line is what DirectWrite wraps, for any text
// made of what wrapping takes, at any width, in a font's first checks and
// beyond.
func TestDWriteWrapFuzz(t *testing.T) {
	iterations := 400
	if n, err := strconv.Atoi(os.Getenv("MYGO_WRAP_FUZZ")); err == nil {
		iterations = n
	}
	words := []string{"a", "I", "to", "the", "window", "layout", "AVATAR", "Type", "fi", "ffl", "office", "don't", "well-known", "e.g.", "U.S.A.", "1,234.56", "3.14", "$5", "50%", "a/b", "http://example.com/path?q=1&r=2", "(paren)", "[box]", "{curly}", "\"quoted\"", "'single'", "end.", "yes,", "stop;", "wait:", "what?", "no!", "x+y", "a=b", "#tag", "@me", "under_score", "<tag>", "~", "*star*", "pipe|", `back\slash`, "café", "Über", "señor", "naïve", "Ærø", "garçon", "verylongwordthatdoesnotfitalineandhastobrokenbydirectwrite", "--", "-", "...", "A-1", "1-2", "T.", "V,", "Y.", "L'", "f)", "ß"}
	seps := []string{" ", " ", " ", " ", "  ", "   ", "", "-", "/"}
	styles := []Style{{Size: 13}, {Size: 14, Weight: 700}, {Size: 13, Italic: true}, {Size: 15, Family: "Georgia"}, {Size: 12, Family: "Arial"}, {Size: 13, Family: "Consolas"}, {Size: 16, Family: "Times New Roman"}, {Size: 13, Family: "Calibri"}, {Size: 13, Lang: "en-US"}}
	rng := rand.New(rand.NewPCG(1, 2))
	s := newSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	wrapped, leftOut := 0, 0
	defer func() { t.Logf("%d wrappings compared, %d left to DirectWrite", wrapped, leftOut) }()
	for it := range iterations {
		var b strings.Builder
		for range 1 + rng.IntN(80) {
			b.WriteString(words[rng.IntN(len(words))])
			b.WriteString(seps[rng.IntN(len(seps))])
		}
		text := []rune(b.String())
		style := styles[it%len(styles)]
		if !wrapsFromWhole(text, style, nil, 1, false, false) {
			continue
		}
		family, custom := e.family(style.Family)
		format := formatKey{style.Family, family, custom, style.weight(), style.Italic, style.FontSize(), style.Lang}
		for _, width := range []float32{float32(20 + rng.IntN(100)), float32(100 + rng.IntN(400)), float32(300+rng.IntN(500)) + rng.Float32()} {
			e.resetScratch()
			// Every wrapping checked by the test, not only some.
			if f := e.wrapFonts[format]; f != nil {
				f.checked = wrapCheckedFirst
				f.lines = 1
			}
			got := e.wrapWhole(text, style, width)
			if got == nil {
				// What does not shape as a glyph for each rune, or may
				// break otherwise, stays with DirectWrite.
				leftOut++
				continue
			}
			wrapped++
			want := e.shapeText(text, style, nil, width, false, false)
			if !sameLines(got, want) {
				report(t, text, style, width, got, want)
				return
			}
		}
	}
	for k, f := range e.wrapFonts {
		if f.disagreed {
			t.Errorf("%+v disagreed with DirectWrite", k)
		}
	}
	if wrapped == 0 {
		t.Error("nothing was wrapped from one line")
	}
}

// A text wrapped across widths is kept while frames wrap it, and let go of
// after.
func TestDWriteWrapKept(t *testing.T) {
	s := newSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	text := []rune("the window layout wraps again at every width of the window")
	style := Style{Size: 13}
	for w := 100; w < 140; w++ {
		e.resetScratch()
		if e.wrapWhole(text, style, float32(w)) == nil {
			t.Fatalf("not wrapped from one line at %d", w)
		}
		e.endFrame()
	}
	if len(e.wholes) != 1 {
		t.Fatalf("%d texts kept, want 1", len(e.wholes))
	}
	for range keptWholeFrames + 1 {
		e.endFrame()
	}
	if len(e.wholes) != 0 {
		t.Errorf("%d texts kept after they stopped wrapping", len(e.wholes))
	}
}

// report tells the first line where wrapped text differs from DirectWrite's.
func report(t *testing.T, text []rune, style Style, width float32, got, want []shapedLine) {
	t.Helper()
	t.Logf("text %q", string(text))
	for i := range min(len(got), len(want)) {
		if got[i].start != want[i].start || got[i].end != want[i].end {
			t.Fatalf("in %+v at %v, line %d is %q, DirectWrite's %q", style, width, i, string(text[got[i].start:got[i].end]), string(text[want[i].start:want[i].end]))
		}
		if !sameLines(got[i:i+1], want[i:i+1]) {
			x, y := got[i].runs, want[i].runs
			t.Errorf("in %+v at %v, line %d %q has %d runs, DirectWrite's %d", style, width, i, string(text[got[i].start:got[i].end]), len(x), len(y))
			if len(x) == 1 && len(y) == 1 {
				for k := range min(len(x[0].glyphs), len(y[0].glyphs)) {
					g, h := x[0].glyphs[k], y[0].glyphs[k]
					if g.ID != h.ID || g.X != h.X || g.Advance != h.Advance {
						t.Errorf("  glyph %d (%q): id %d/%d x %v/%v advance %v/%v", k, string(text[g.Cluster]), g.ID, h.ID, g.X, h.X, g.Advance, h.Advance)
					}
				}
			}
			t.FailNow()
		}
	}
	t.Fatalf("in %+v at %v, %d lines, DirectWrite's %d", style, width, len(got), len(want))
}

// Prose of plain words, as most text is, wraps from one line in every font,
// as DirectWrite wraps it.
func TestDWriteWrapProse(t *testing.T) {
	words := []string{"a", "I", "to", "the", "window", "layout", "AVATAR", "Type", "don't", "end.", "yes,", "stop;", "wait:", "what?", "no!", "(paren)", "\"quoted\"", "café", "Über", "señor", "naïve", "Ærø", "garçon", "T.", "V,", "Y.", "L'", "ß", "WAVE", "Toyota", "AVAST", "1984", "x2"}
	styles := []Style{{Size: 13}, {Size: 14, Weight: 700}, {Size: 13, Italic: true}, {Size: 15, Family: "Georgia"}, {Size: 12, Family: "Arial"}, {Size: 13, Family: "Consolas"}, {Size: 16, Family: "Times New Roman"}, {Size: 11, Family: "Tahoma"}, {Size: 13, Lang: "en-US"}}
	rng := rand.New(rand.NewPCG(3, 4))
	s := newSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	wrapped, leftOut := 0, 0
	for it := range 600 {
		var b strings.Builder
		for k := range 1 + rng.IntN(70) {
			if k > 0 {
				b.WriteString(" ")
			}
			b.WriteString(words[rng.IntN(len(words))])
		}
		text := []rune(b.String())
		style := styles[it%len(styles)]
		family, custom := e.family(style.Family)
		format := formatKey{style.Family, family, custom, style.weight(), style.Italic, style.FontSize(), style.Lang}
		for _, width := range []float32{float32(60 + rng.IntN(100)), float32(150+rng.IntN(400)) + rng.Float32()} {
			e.resetScratch()
			if f := e.wrapFonts[format]; f != nil {
				f.checked = wrapCheckedFirst
				f.lines = 1
			}
			got := e.wrapWhole(text, style, width)
			if got == nil {
				leftOut++
				continue
			}
			wrapped++
			if want := e.shapeText(text, style, nil, width, false, false); !sameLines(got, want) {
				report(t, text, style, width, got, want)
			}
		}
	}
	t.Logf("%d wrappings compared, %d left to DirectWrite", wrapped, leftOut)
	if leftOut > wrapped/10 {
		t.Errorf("%d of %d wrappings left to DirectWrite", leftOut, wrapped+leftOut)
	}
}
