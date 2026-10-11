package ui

import (
	"bytes"
	"fmt"
	"math"
	"testing"
	"time"
)

// retainedView is a window with much it keeps painted and what moves:
// a spinner, whose arc a path draws in a new mask each frame, inside a
// clip and an opacity, a bar that a later step moves (Painter.After),
// text over and under them, and an icon drawn for the first time in the
// frame painted whole, whose mask lasts only that frame.
func retainedView(icon *SVG) func(c *context) {
	return func(c *context) {
		coreColumn(c).Padding(4).Gap(2).Background(RGB(250, 250, 250)).Children(func() {
			for i := range 8 {
				coreText(c, fmt.Sprintf("line %d of the text kept painted", i))
			}
			coreRow(c).Gap(4).Children(func() {
				coreBox(c).Size(40, 40).Shrink(0).Clip().Radius(8).Opacity(0.6).Background(RGB(220, 230, 255)).Children(func() {
					coreBox(c).Size(40, 40).Draw(func(p *Painter, r Rect) {
						p.AnimationFrame()
						turn := float64(p.Now().UnixMilli()%900) / 900 * 2 * math.Pi
						path := new(Path)
						for k := 0; k <= 12; k++ {
							a := turn + float64(k)*0.12
							x, y := r.X+r.W/2+14*float32(math.Cos(a)), r.Y+r.H/2+14*float32(math.Sin(a))
							if k == 0 {
								path.MoveTo(x, y)
							} else {
								path.LineTo(x, y)
							}
						}
						p.StrokePath(path, 3, RGB(40, 90, 200))
					})
				})
				coreBox(c).Size(60, 12).Shrink(0).Draw(func(p *Painter, r Rect) {
					p.After(30 * time.Millisecond)
					x := float32(p.Now().UnixMilli()/30%10) * 5
					p.Fill(Rect{r.X + x, r.Y, 8, r.H}, RGB(200, 60, 40), 2)
				})
				coreIcon(c, icon).Size(16, 16)
			})
			for i := range 4 {
				coreText(c, fmt.Sprintf("and more text below, %d", i))
			}
		})
	}
}

// Frames that paint only what moves show what painting them whole does,
// frame after frame.
func TestRepaintKeepsTheRest(t *testing.T) {
	tt := newRepaintTester(retainedView(MustParseSVG([]byte(checkSVG))))
	tt.SetSize(320, 320)
	tt.SetScale(2)
	tt.frame(16 * time.Millisecond)
	for round := range 4 {
		// Several frames in a row paint only what moves...
		for range 5 {
			if built, _ := tt.frame(16 * time.Millisecond); built {
				t.Fatalf("round %d: a frame built the view", round)
			}
		}
		if tt.rt.repaints == 0 || len(tt.rt.segs) != 2 {
			t.Fatalf("round %d: %d frames painted only what moves, %d elements noted", round, tt.rt.repaints, len(tt.rt.segs))
		}
		got := bytes.Clone(tt.Image().Pix)
		// ...and show what the same frame painted whole does.
		tt.rt.retained = false
		tt.frame(0)
		if tt.rt.repaints != 0 {
			t.Fatal("the frame did not paint whole")
		}
		want := tt.Image()
		for i := range want.Pix {
			if got[i] != want.Pix[i] {
				p := i / 4
				t.Fatalf("round %d: pixel (%d, %d) is %v, painted whole %v", round, p%want.Rect.Dx(), p/want.Rect.Dx(), got[p*4:p*4+4], want.Pix[p*4:p*4+4])
			}
		}
	}
}

// A window painting a frame whole frees the masks the frames of another
// keep, which then paints its next frame whole.
func TestRepaintAfterAnotherWindow(t *testing.T) {
	icon := MustParseSVG([]byte(checkSVG))
	a := newRepaintTester(retainedView(icon))
	a.SetSize(320, 320)
	a.frame(16 * time.Millisecond)
	if a.rt.repaints != 1 {
		t.Fatalf("a: %d frames painted only what moves", a.rt.repaints)
	}
	newRepaintTester(retainedView(icon)).SetSize(320, 320)
	a.frame(16 * time.Millisecond)
	if a.rt.repaints != 0 {
		t.Fatal("a kept the masks of its last frame painted whole after another window painted one")
	}
	a.frame(16 * time.Millisecond)
	if a.rt.repaints != 1 {
		t.Fatal("a painted whole again")
	}
}

// A pose moving moves what it holds, which frames painting only what
// asked would keep where it was: those frames paint whole.
func TestRepaintWithAPoseMoving(t *testing.T) {
	lifted := false
	tt := newRepaintTester(func(c *context) {
		coreRow(c).Children(func() {
			coreBox(c).Size(20, 20).Draw(func(p *Painter, r Rect) {
				p.AnimationFrame()
				p.Fill(Rect{r.X + float32(p.Now().UnixMilli()%10), r.Y, 2, r.H}, RGB(0, 0, 255), 0)
			})
			p := Pose{}
			if lifted {
				p = Pose{X: 100}
			}
			coreBox(c).Size(40, 40).Background(RGB(255, 0, 0)).AnimatePose(p, Animation{Duration: 100 * time.Millisecond, Ease: Linear})
		})
	})
	tt.frame(16 * time.Millisecond)
	if tt.rt.repaints != 1 {
		t.Fatalf("the spinner alone: %d frames painted only what moves", tt.rt.repaints)
	}
	lifted = true
	tt.rt.requestFrame()
	tt.frame(0)
	if built, _ := tt.frame(50 * time.Millisecond); built {
		t.Fatal("moving, a frame built the view")
	}
	if tt.rt.repaints != 0 {
		t.Fatal("a frame moving a pose painted only the spinner")
	}
	// Halfway: the card's left edge at 20 + 50.
	if p := tt.Image().RGBAAt(75, 20); p.R != 255 || p.G != 0 {
		t.Errorf("halfway, the card is not at 70: %v", p)
	}
}

// A content transition starts after frames that painted only what moves,
// which kept the text painted.
func TestContentTransitionAfterRepaints(t *testing.T) {
	value := "1"
	tt := newRepaintTester(func(c *context) {
		coreRow(c).Children(func() {
			coreBox(c).Size(20, 20).Draw(func(p *Painter, r Rect) {
				p.AnimationFrame()
				p.Fill(Rect{r.X + float32(p.Now().UnixMilli()%10), r.Y, 2, r.H}, RGB(0, 0, 255), 0)
			})
			coreText(c, value).ContentTransition(ContentTransition{Numeric: true, Animation: &Animation{Duration: 100 * time.Millisecond, Ease: Linear}})
		})
	})
	for range 3 {
		tt.frame(16 * time.Millisecond)
	}
	if tt.rt.repaints != 3 {
		t.Fatalf("%d frames painted only what moves", tt.rt.repaints)
	}
	value = "2"
	tt.rt.requestFrame()
	tt.frame(0)
	tt.frame(50 * time.Millisecond)
	if _, faded := paintedGlyphs(tt.Tester); faded == 0 {
		t.Error("halfway, the number does not roll")
	}
}
