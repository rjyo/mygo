package ui

import (
	"bytes"
	"testing"
	"time"
)

// A focused input's caret blinks in frames painting the window again, not
// in frames building the view, and those show what building it does.
func TestCaretBlinkRepaints(t *testing.T) {
	for _, multiline := range []bool{false, true} {
		value := "some text"
		tt := newRepaintTester(func(c *context) { textInput(c, &value, multiline).Width(200).AutoFocus() })
		tt.SetSize(300, 120)
		tt.frame(0)
		tt.frame(0)
		for round := range 3 {
			tt.rt.blinkStart = time.Now()
			if built, _ := tt.frame(530 * time.Millisecond); built {
				t.Fatalf("multiline %v, round %d: the blink built the view", multiline, round)
			}
			got := bytes.Clone(tt.Image().Pix)
			tt.rt.noParts = true
			tt.rt.changed()
			tt.frame(0)
			tt.rt.noParts = false
			if !bytes.Equal(got, tt.Image().Pix) {
				t.Fatalf("multiline %v, round %d: the blink frame differs from the frame painted whole", multiline, round)
			}
		}
	}
}
