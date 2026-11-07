package ui_test

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A caret that the text's alignment puts against the right of the box
// shows inside the box, as it does where the text scrolls: drawn on the
// edge, the input's clip would cut it away.
func TestRightAlignedCaretShows(t *testing.T) {
	for _, test := range []struct {
		name, value string
		align       ui.Align
		dir         ui.Direction
	}{
		{"end empty", "", ui.End, ui.LeftToRight},
		{"end after text", "abc", ui.End, ui.LeftToRight},
		{"start right to left", "", ui.Start, ui.RightToLeft},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := test.value
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Padding(20).Children(func() {
					ui.TextInputBase(c, &v).Width(200).TextAlign(test.align).Direction(test.dir).AutoFocus().Label("field")
				})
			}, 260, 60)
			tt.Key(0, ui.KeyEnd)
			box, ok := tt.Find("field")
			if !ok {
				t.Fatal("no field")
			}
			on := tt.Image()
			pix := append([]uint8(nil), on.Pix...)
			tt.Advance(530 * time.Millisecond)
			off := tt.Image()
			blinked := -1
			for i := range pix {
				if pix[i] != off.Pix[i] {
					blinked = max(blinked, (i%on.Stride)/4)
				}
			}
			if blinked < 0 {
				t.Fatalf("no caret blinks in %v", box)
			}
			if right := int(box.X + box.W); blinked >= right || blinked < right-3 {
				t.Errorf("the caret blinks at column %d, not at the right of %v", blinked, box)
			}
		})
	}
}
