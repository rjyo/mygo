package ui

import (
	"fmt"
	"testing"
)

func TestScrollbarAxes(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		horizontal, vertical, keep, rtl bool
		scale                           float32
	}{
		{"both", true, true, false, false, 1},
		{"horizontal", true, false, false, false, 1},
		{"vertical RTL", false, true, true, true, 2},
		{"horizontal RTL", true, false, true, true, 2},
		{"neither", false, false, true, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var track ScrollState
			clicks := 0
			tt := coreNewTester(func(c *context) {
				coreScrollBoth(c).Absolute().Left(0).Top(0).Size(100, 100).ScrollbarInsets(4).
					ScrollbarAxes(tc.horizontal, tc.vertical).KeepScrollbars(tc.keep).TrackScroll(&track).
					ScrollbarStyle(ScrollbarStyle{
						TrackWidth: 12, ThumbWidth: 6, Track: RGB(240, 20, 30), Thumb: RGB(240, 20, 30),
					}).Children(func() {
					content := coreBox(c).Size(400, 400).Background(RGB(255, 255, 255)).Cursor(CursorText).Focusable()
					content.flags |= flagClickable
					if content.Clicked() {
						clicks++
					}
				})
			}, 140, 140)
			defer tt.rt.close()
			tt.SetScale(tc.scale)
			vx, cornerX, wantX := float32(90), float32(94), float32(130)
			if tc.rtl {
				tt.SetDirection(RightToLeft)
				vx, cornerX, wantX = 10, 6, 70
			}
			tt.Move(120, 120)
			if !tc.keep {
				tt.Move(50, 50)
			}
			img := tt.Image()
			type sample struct {
				x, y    float32
				visible bool
			}
			points := []sample{{vx, 50, tc.vertical}, {50, 90, tc.horizontal}}
			// The other bar must not extend into the shared corner.
			if tc.horizontal != tc.vertical {
				points = append(points, sample{cornerX, 94, false})
			}
			for _, point := range points {
				if painted := img.RGBAAt(int(point.x*tc.scale), int(point.y*tc.scale)).G == 20; painted != point.visible {
					t.Fatalf("track at %g,%g painted=%v, want %v", point.x, point.y, painted, point.visible)
				}
				tt.Move(point.x, point.y)
				wantCursor := CursorText
				if point.visible {
					wantCursor = CursorDefault
				}
				if tt.Cursor() != wantCursor {
					t.Fatalf("track at %g,%g cursor=%v, want %v", point.x, point.y, tt.Cursor(), wantCursor)
				}
				before := clicks
				tt.ClickAt(point.x, point.y)
				if (clicks == before) != point.visible {
					t.Fatalf("track at %g,%g hit target disagrees with visibility", point.x, point.y)
				}
			}
			track.X, track.Y = 100, 100
			tt.Frame()
			tt.Scroll(50, 50, 30, 40)
			if track.X != wantX || track.Y != 140 {
				t.Fatalf("indicator policy changed scrolling: %+v", track)
			}
		})
	}
}

func TestScrollbarAxesDuringDrag(t *testing.T) {
	for _, horizontal := range []bool{false, true} {
		for _, hideDragged := range []bool{false, true} {
			t.Run(fmt.Sprintf("horizontal%t/hideDragged%t", horizontal, hideDragged), func(t *testing.T) {
				xVisible, yVisible := true, true
				clicks := 0
				var st *state
				tt := coreNewTester(func(c *context) {
					scroll := coreScrollBoth(c).Size(100, 100).Key("target").
						ScrollbarAxes(false, false).ScrollbarAxes(xVisible, yVisible)
					st = scroll.st
					scroll.Children(func() {
						content := coreBox(c).Size(400, 400).Cursor(CursorText)
						content.flags |= flagClickable
						if content.Clicked() {
							clicks++
						}
					})
				}, 140, 140)
				defer tt.rt.close()
				tt.Scroll(50, 50, 30, 40)
				tt.Move(50, 50)
				g := scrollBars(Rect{0, 0, 100, 100}, st.barInset, 400, 400, float32(st.scrollX), float32(st.scrollY), st.flags, st.scrollbarStyle(), false)
				thumb := g.v
				if horizontal {
					thumb = g.h
				}
				x, y := thumb.X+thumb.W/2, thumb.Y+thumb.H/2
				tt.Press(x, y)
				tt.Frame()
				if tt.rt.scrollDrag.st != st {
					t.Fatal("final fluent value did not preserve the drag")
				}
				if horizontal {
					x += 10
				} else {
					y += 10
				}
				tt.Move(x, y)
				beforeX, beforeY := st.scrollX, st.scrollY
				if horizontal == hideDragged {
					xVisible = false
				} else {
					yVisible = false
				}
				tt.Frame()
				if (tt.rt.scrollDrag.st == nil) != hideDragged {
					t.Fatal("hid the wrong drag axis")
				}
				if st.scrollX != beforeX || st.scrollY != beforeY {
					t.Fatal("changing visibility moved the content before the pointer moved")
				}
				if hideDragged && tt.Cursor() != CursorText {
					t.Fatal("stationary pointer kept the scrollbar cursor")
				}
				if horizontal {
					x++
				} else {
					y++
				}
				tt.Move(x, y)
				if !hideDragged && (horizontal && st.scrollX <= beforeX || !horizontal && st.scrollY <= beforeY) {
					t.Fatalf("dragging forward after hiding the other bar did not scroll forward: (%g, %g) → (%g, %g)", beforeX, beforeY, st.scrollX, st.scrollY)
				}
				tt.Release(x, y)
				moved := st.scrollX != beforeX || st.scrollY != beforeY
				if moved == hideDragged || clicks != 0 {
					t.Fatal("visibility change lost the live drag or synthesized input")
				}
				tt.ClickAt(50, 50)
				if clicks != 1 {
					t.Fatal("content did not become clickable")
				}
			})
		}
	}
}

func TestScrollbarAxesOverrides(t *testing.T) {
	for _, axesFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(axesFirst), func(t *testing.T) {
			visible, override := false, true
			var st *state
			tt := coreNewTester(func(c *context) {
				scroll := coreScrollBoth(c).Size(100, 100).KeepScrollbars(true)
				st = scroll.st
				if axesFirst && override {
					scroll.ScrollbarAxes(true, false)
				}
				scroll.Scrollbars(visible)
				if !axesFirst && override {
					scroll.ScrollbarAxes(true, false)
				}
				scroll.Children(func() { coreBox(c).Size(400, 400) })
			}, 140, 140)
			defer tt.rt.close()
			tt.Move(94, 40)
			if st.barsShown || tt.rt.overScrollbar(st.id) {
				t.Fatal("Scrollbars(false) did not hide both axes")
			}
			visible = true
			tt.Frame()
			if tt.rt.overScrollbar(st.id) {
				t.Fatal("Scrollbars(true) reset the axis settings")
			}
			tt.Move(40, 94)
			if !tt.rt.overScrollbar(st.id) {
				t.Fatal("KeepScrollbars did not keep the enabled axis")
			}
			computed := tt.rt.computed
			override = false
			tt.Frame()
			tt.Move(94, 40)
			if !tt.rt.overScrollbar(st.id) {
				t.Fatal("omitting ScrollbarAxes did not restore the vertical bar")
			}
			if tt.rt.computed != computed {
				t.Fatal("indicator policy invalidated layout")
			}
		})
	}
}

func TestTableScrollbarAxes(t *testing.T) {
	list := ListState{}
	vertical := false
	tt := coreNewTester(func(c *context) {
		coreTable(c, &list, []TableColumn{{Title: "Name"}}, 200, func(row, col int) { coreTextf(c, "Cell %d", row) }).
			ScrollbarAxes(true, vertical).KeepScrollbars(true).Size(200, 200)
	}, 240, 240)
	defer tt.rt.close()
	tt.Move(230, 230)
	tt.Frame()
	st := list.frame.e.st
	g := scrollBars(Rect{st.x, st.y, st.w, st.h}, st.barInset, float32(st.contentW), float32(st.contentH), 0, 0, st.flags, st.scrollbarStyle(), false)
	tt.Move(g.vTrack.X+g.vTrack.W/2, g.vTrack.Y+g.vTrack.H/2)
	if tt.rt.overScrollbar(st.id) {
		t.Fatal("table did not hide its rows' vertical bar")
	}
	vertical = true
	tt.Frame()
	if !tt.rt.overScrollbar(st.id) {
		t.Fatal("table did not restore its rows' vertical bar")
	}
}

func TestScrollbarAxesCancelBeforePainting(t *testing.T) {
	vertical := true
	var st *state
	tt := coreNewTester(func(c *context) {
		scroll := coreScrollBoth(c).Size(100, 100).ScrollbarAxes(true, vertical)
		st = scroll.st
		scroll.Children(func() { coreBox(c).Size(400, 400) })
	}, 140, 140)
	defer tt.rt.close()
	tt.Move(50, 50)
	tt.Press(94, 12)
	if tt.rt.scrollDrag.st != st {
		t.Fatal("fixture did not start a vertical drag")
	}
	tt.Move(120, 120)
	vertical = false
	tt.rt.runFrame()
	if tt.rt.scrollDrag.st != nil || st.barsShown {
		t.Fatal("cancelled drag kept the other bar showing with the pointer away")
	}
	tt.Release(120, 120)
}
