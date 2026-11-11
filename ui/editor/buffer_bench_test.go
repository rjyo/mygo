package editor

import (
	"fmt"
	"strings"
	"testing"
)

// largeText is a document of n lines of code, about 30 bytes each.
func largeText(n int) string {
	var s strings.Builder
	for i := range n {
		fmt.Fprintf(&s, "func f%d() int { return %d }\n", i, i)
	}
	return s.String()
}

// Typing into a large document, each character a step of undo.
func BenchmarkBufferEditLarge(b *testing.B) {
	buf := NewBuffer(largeText(100_000))
	at := buf.Len() / 2
	for b.Loop() {
		buf.Edit(Edit{Start: at, End: at, Text: "x"})
		at++
	}
}

// Undoing and redoing a step of a large document with a long history.
func BenchmarkBufferUndoLarge(b *testing.B) {
	buf := NewBuffer(largeText(100_000))
	for i := range 1000 {
		at := buf.Offset(Pos{Line: i * 100, Col: 5})
		buf.Edit(Edit{Start: at, End: at + 2, Text: "abc"})
	}
	for b.Loop() {
		buf.Undo()
		buf.Redo()
	}
}

// Modified of a large document, undone back to its saved text and with
// a text of another length.
func BenchmarkBufferModifiedLarge(b *testing.B) {
	for _, undone := range []bool{true, false} {
		b.Run(fmt.Sprintf("undone=%v", undone), func(b *testing.B) {
			buf := NewBuffer(largeText(100_000))
			buf.Edit(Edit{Start: 10, End: 10, Text: "x"})
			if undone {
				buf.Undo()
			}
			for b.Loop() {
				buf.Modified()
			}
		})
	}
}
