//go:build windows && (amd64 || arm64)

package text

import (
	"strings"
	"testing"
)

// Numbers put together are what DirectWrite shapes, in the system's fonts.
func TestDWriteNumbers(t *testing.T) {
	numbers := []string{"0", "7", "11", "1,234,567.89", "-42.5%", "+3.14", "$1,000", "1.1.1.1", "$-0.50", "%%", "--", "..", "1+1-1", "00000000000000000000000000000000", "12.345", "98765.4321"}
	styles := []Style{{Size: 11}, {Size: 13}, {Size: 13, Weight: 700}, {Size: 14, Italic: true}, {Size: 14, Family: "Consolas"}, {Size: 16, Family: "Arial"}, {Size: 15, Family: "Georgia"}, {Size: 13, Family: "Times New Roman"}, {Size: 13, Lang: "zh-Hans"}, {Size: 13, Lang: "en"}}
	s := newSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	for _, style := range styles {
		for _, number := range numbers {
			text := []rune(number)
			e.resetScratch()
			lines := e.shapeNumber(text, style, nil, 0, false)
			if lines == nil {
				t.Fatalf("%q in %+v was not put together", number, style)
			}
			if !sameNumber(lines, e.shapeText(text, style, nil, 0, false, false)) {
				t.Errorf("%q in %+v put together is not what DirectWrite shapes", number, style)
			}
		}
	}
	for _, f := range e.numbers {
		if f.disagreed {
			t.Error("a font disagreed with DirectWrite")
		}
	}
}

// What is not a number, wraps, or takes more than its characters' glyphs is
// left to DirectWrite, and a font it disagrees with stays with it.
func TestDWriteNumbersLeftOut(t *testing.T) {
	s := newSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	style := Style{Size: 13}
	for _, c := range []struct {
		text  string
		style Style
		spans []Span
		width float32
		rtl   bool
	}{
		{text: "12a", style: style},
		{text: "12:30", style: style},
		{text: "١٢٣", style: style},
		{text: "123", style: style, rtl: true},
		{text: "123", style: style, spans: []Span{{End: 1, Weight: 700}}},
		{text: "123", style: Style{Size: 13, LetterSpacing: 1}},
		{text: "123", style: Style{Size: 13, Features: "tnum"}},
		{text: "123", style: Style{Size: 13, Lang: "tr"}},
		{text: "123456", style: style, width: 5},
		{text: strings.Repeat("1", maxNumber+1), style: style},
	} {
		e.resetScratch()
		if e.shapeNumber([]rune(c.text), c.style, c.spans, c.width, c.rtl) != nil {
			t.Errorf("%q in %+v (width %v, RTL %v) was put together", c.text, c.style, c.width, c.rtl)
		}
	}

	text := []rune("42")
	e.resetScratch()
	want := e.shapeText(text, style, nil, 0, false, false)
	wantX := want[0].runs[0].glyphs[1].X
	if e.shapeNumber(text, style, nil, 0, false) == nil {
		t.Fatal("42 was not put together")
	}
	family, custom := e.family(style.Family)
	f := e.numbers[formatKey{style.Family, family, custom, style.weight(), style.Italic, style.FontSize(), style.Lang}]
	f.pairs[numberSlots['4']-1][numberSlots['2']-1].advance += 5
	e.resetScratch()
	got := e.shapeNumber(text, style, nil, 0, false)
	if !f.disagreed {
		t.Error("a number wrong by 5 did not count as a disagreement")
	}
	if x := got[0].runs[0].glyphs[1].X; x != wantX {
		t.Errorf("the second digit of a number that disagrees is at %v, DirectWrite puts it at %v", x, wantX)
	}
	e.resetScratch()
	if e.shapeNumber(text, style, nil, 0, false) != nil {
		t.Error("a font that disagreed still has numbers put together")
	}
}
