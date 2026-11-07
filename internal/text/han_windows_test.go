//go:build windows && (amd64 || arm64)

package text

import (
	"maps"
	"slices"
	"syscall"
	"testing"
	"unsafe"
)

var iidIDWriteFontFace5 = guid{0x98eff3a5, 0xb667, 0x479a, [8]byte{0xb1, 0x45, 0xe2, 0xfa, 0x5b, 0x9f, 0xdc, 0x29}}

// IDWriteFontFace3's and IDWriteFontFace5's methods the tests call.
const (
	face3GetFamilyNames        = 40
	face5GetFontAxisValueCount = 53
	face5GetFontAxisValues     = 54
)

// faceFamily returns the English name of a font face's family.
func faceFamily(face uintptr) string {
	f3 := queryInterface(face, &iidIDWriteFontFace3)
	if f3 == 0 {
		return ""
	}
	defer release(f3)
	var strs uintptr
	if failed(call(f3, face3GetFamilyNames, uintptr(unsafe.Pointer(&strs)))) {
		return ""
	}
	defer release(strs)
	return englishName(strs)
}

// faceWeightAxis returns the value of a font face's weight axis
// (IDWriteFontFace5, Windows 10 1803 and later).
func faceWeightAxis(face uintptr) (float32, bool) {
	f5 := queryInterface(face, &iidIDWriteFontFace5)
	if f5 == 0 {
		return 0, false
	}
	defer release(f5)
	n := uint32(call(f5, face5GetFontAxisValueCount))
	if n == 0 {
		return 0, false
	}
	// DWRITE_FONT_AXIS_VALUE
	values := make([]struct {
		tag   uint32
		value float32
	}, n)
	if failed(call(f5, face5GetFontAxisValues, uintptr(unsafe.Pointer(&values[0])), uintptr(n))) {
		return 0, false
	}
	for _, v := range values {
		if v.tag == 'w'|'g'<<8|'h'<<16|'t'<<24 {
			return v.value, true
		}
	}
	return 0, false
}

// Chinese text in no language that writes Han characters, as an English
// app's, takes the first of the user's preferred languages that does: one
// font, that of Chinese text, rather than the English fallback's mix of a
// Japanese font where it has the characters and a Chinese one where it
// lacks them, which draws one label in two weights (#1454).
func TestDWriteUserHanLanguage(t *testing.T) {
	s := newSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	saved := e.han
	defer func() { e.han = saved }()
	texts := []string{"我们的音乐播放器正在播放周杰伦的歌曲", "这是一个中文标签，看看字重是否一致。", "Hello 世界 说话 东西"}
	// hanFamilies returns the families of the runs of Han characters of
	// text in a style and spans.
	hanFamilies := func(text string, style Style, spans []Span) map[string]bool {
		r := []rune(text)
		e.resetScratch()
		fams := map[string]bool{}
		for _, line := range e.shapeText(r, style, spans, 0, false, false) {
			for _, run := range line.runs {
				if slices.ContainsFunc(r[run.start:run.end], isCJK) {
					fams[faceFamily(run.font.native)] = true
				}
			}
		}
		return fams
	}

	// The fallback of the user's locale alone, the runner's en-US.
	e.han = nil
	for _, text := range texts {
		t.Logf("%q in the locale %s: %v", text, syscall.UTF16ToString(e.locale), slices.Sorted(maps.Keys(hanFamilies(text, Style{Size: 13}, nil))))
	}

	// A user of English Windows who reads Chinese (userHanLanguage).
	e.han = utf16z("zh-Hans-CN")
	for _, style := range []Style{{Size: 13}, {Size: 13, Lang: "en"}, {Size: 13, Weight: 700}, {Size: 13, Weight: 300}, {Size: 13, Lang: "en-US", Weight: 600}} {
		chinese := style
		chinese.Lang = "zh-Hans-CN"
		for _, text := range texts {
			got := hanFamilies(text, style, nil)
			if len(got) != 1 {
				t.Errorf("%q in %+v is drawn in %v, not one font", text, style, slices.Sorted(maps.Keys(got)))
			}
			if want := hanFamilies(text, chinese, nil); !maps.Equal(got, want) {
				t.Errorf("%q in %+v is drawn in %v, Chinese text in %v", text, style, slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
			}
		}
	}
	// Text in a language that writes Han characters keeps its own.
	for _, text := range texts {
		e.han = nil
		want := hanFamilies(text, Style{Size: 13, Lang: "ja"}, nil)
		wantSpan := hanFamilies(text, Style{Size: 13, Lang: "en"}, []Span{{End: 100, Lang: "ja"}})
		e.han = utf16z("zh-Hans-CN")
		if got := hanFamilies(text, Style{Size: 13, Lang: "ja"}, nil); !maps.Equal(got, want) {
			t.Errorf("Japanese %q is drawn in %v, %v without the user's Chinese", text, slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
		}
		if got := hanFamilies(text, Style{Size: 13, Lang: "en"}, []Span{{End: 100, Lang: "ja"}}); !maps.Equal(got, wantSpan) {
			t.Errorf("a Japanese span %q is drawn in %v, %v without the user's Chinese", text, slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(wantSpan)))
		}
	}
}

// A variable font's glyphs are rasterized at the instance DirectWrite
// picked for the weight, as fallback picks them too: the face of the glyph
// run, which carries the instance's axis values, is the one rasterized,
// and a heavier weight covers more pixels. Windows has variable fonts of
// its own (Bahnschrift from Windows 10, Segoe UI Variable from Windows 11).
func TestDWriteVariableWeight(t *testing.T) {
	s := newSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	tested := false
	for _, family := range []string{"Bahnschrift", "Segoe UI Variable Text"} {
		if !e.hasSystem(family) {
			continue
		}
		last := 0
		for _, weight := range []int{300, 400, 700} {
			e.resetScratch()
			lines := e.shapeText([]rune("H"), Style{Size: 40, Family: family, Weight: weight}, nil, 0, false, false)
			if len(lines) != 1 || len(lines[0].runs) != 1 {
				t.Fatalf("H in %s %d shaped into %+v", family, weight, lines)
			}
			run := lines[0].runs[0]
			wght, ok := faceWeightAxis(run.font.native)
			if !ok {
				t.Logf("%s's face has no axis values here", family)
				break
			}
			tested = true
			if int(wght) != weight {
				t.Errorf("%s %d's run is of the instance of weight %v", family, weight, wght)
			}
			coverage := 0
			for _, p := range e.glyph(run.font, run.glyphs[0].ID, 1, 0, 0, false).pix {
				coverage += int(p)
			}
			if coverage <= last {
				t.Errorf("H in %s %d covers %d, no more than a lighter weight's %d", family, weight, coverage, last)
			}
			last = coverage
		}
	}
	if !tested {
		t.Skip("no variable font with axis values")
	}
}
