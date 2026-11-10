package ui

import (
	"fmt"
	"image/color"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// testScrollbarStyle is the default scroll bar style, a 6 DIP thumb in a
// 12 DIP bar, for the tests that lay out bars without a tester's theme.
func testScrollbarStyle() ScrollbarStyle { return ScrollbarStyle{ThumbWidth: 6}.Resolve(nil) }

// A theme's ScrollbarStyle paints a scroll container's track and thumb,
// with the thumb's hover and drag colors, and a container's own style
// wins over the theme's.
func TestScrollbarStylePaints(t *testing.T) {
	track, thumb, hover, active := RGB(10, 20, 30), RGB(200, 10, 20), RGB(30, 200, 40), RGB(40, 50, 220)
	var st *state
	tt := coreNewTester(func(c *context) {
		th := *c.Theme()
		th.ScrollbarStyle = ScrollbarStyle{
			TrackWidth: 20, ThumbWidth: 8, MinLength: 40,
			Track: track, Thumb: thumb, Hover: hover, Active: active,
		}
		c.SetTheme(&th)
		sc := coreScroll(c).Size(100, 100)
		st = sc.st
		sc.Children(func() { coreBox(c).Size(100, 1000) })
	}, 140, 140)
	defer tt.rt.close()
	tt.Move(50, 50)
	tt.Frame()
	if style := st.scrollbarStyle(); style.TrackWidth != 20 || style.ThumbWidth != 8 || style.MinLength != 40 {
		t.Fatalf("the bar is %+v, want a 20 DIP track with an 8 DIP thumb at least 40 long", style)
	}
	pixel := func(x, y float32) Color {
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 140
		c := img.RGBAAt(int(x*s), int(y*s))
		return Color{R: c.R, G: c.G, B: c.B, A: c.A}
	}
	// The 20 DIP track is at x 80 to 100; the 8 DIP thumb is centered in
	// it, x 86 to 94, from y 2 to 42.
	if got := pixel(82, 70); got != track {
		t.Errorf("the track at (82, 70) is %v, want %v", got, track)
	}
	if got := pixel(90, 20); got != thumb {
		t.Errorf("the thumb at (90, 20) is %v, want %v", got, thumb)
	}
	// Over the track, the thumb takes its hover color.
	tt.Move(90, 70)
	tt.Frame()
	if got := pixel(90, 20); got != hover {
		t.Errorf("the hovered thumb is %v, want %v", got, hover)
	}
	// Dragged, its active color.
	tt.Press(90, 20)
	tt.Frame()
	if got := pixel(90, 20); got != active {
		t.Errorf("the dragged thumb is %v, want %v", got, active)
	}
	tt.Release(90, 20)
	// A container's own style wins over the theme's.
	blue := RGB(0, 0, 255)
	tt2 := coreNewTester(func(c *context) {
		th := *c.Theme()
		th.ScrollbarStyle = ScrollbarStyle{Thumb: thumb}
		c.SetTheme(&th)
		sc := coreScroll(c).Size(100, 100).ScrollbarStyle(ScrollbarStyle{Thumb: blue})
		st = sc.st
		sc.Children(func() { coreBox(c).Size(100, 1000) })
	}, 140, 140)
	defer tt2.rt.close()
	tt2.Move(50, 50)
	tt2.Frame()
	img := tt2.Image()
	s := float32(img.Bounds().Dx()) / 140
	c := img.RGBAAt(int(94*s), int(20*s))
	if got := (Color{R: c.R, G: c.G, B: c.B, A: c.A}); got != blue {
		t.Errorf("a container's thumb is %v, want its own %v", got, blue)
	}
}

// A square thumb fills its bar's corners, and a thumb as wide as its bar
// changes color rather than growing when dragged, as VS Code's.
func TestScrollbarStyleSquare(t *testing.T) {
	thumb, active := RGB(200, 10, 20), RGB(40, 50, 220)
	tt := coreNewTester(func(c *context) {
		th := *c.Theme()
		th.ScrollbarStyle = ScrollbarStyle{
			TrackWidth: 14, ThumbWidth: 14, Square: true,
			Thumb: thumb, Active: active,
		}
		c.SetTheme(&th)
		sc := coreScroll(c).Size(100, 100)
		sc.Children(func() { coreBox(c).Size(100, 1000).Background(RGB(255, 255, 255)) })
	}, 140, 140)
	defer tt.rt.close()
	tt.Move(50, 50)
	tt.Frame()
	pixel := func(x, y float32) Color {
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 140
		c := img.RGBAAt(int(x*s), int(y*s))
		return Color{R: c.R, G: c.G, B: c.B, A: c.A}
	}
	// The 14 DIP bar is at x 86 to 100, the thumb from y 2 to 42: its
	// top-left corner is square.
	if got := pixel(86, 2); got != thumb {
		t.Errorf("the square thumb's corner is %v, want %v", got, thumb)
	}
	tt.Press(93, 10)
	tt.Frame()
	// The thumb has no room to grow: x 85, just left of the bar, is the
	// content, not the dragged thumb.
	if got := pixel(85, 20); got != (Color{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("the dragged full-width thumb grew to x 85: %v", got)
	}
	tt.Release(93, 10)
}

func TestScrollbarsHiddenPreservesScrollingAndContentInput(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, horizontal := range []bool{false, true} {
			for _, visible := range []bool{false, true} {
				t.Run(fmt.Sprintf("scale%g/horizontal%t/visible%t", scale, horizontal, visible), func(t *testing.T) {
					var st *state
					var track ScrollState
					clicks := 0
					tt := coreNewTester(func(c *context) {
						scroll := coreScrollBoth(c).Size(100, 100).Scrollbars(visible).TrackScroll(&track)
						st = scroll.st
						scroll.Children(func() {
							content := coreBox(c).Size(400, 400).Focusable().Cursor(CursorPointer)
							content.flags |= flagClickable
							if content.Clicked() {
								clicks++
							}
						})
					}, 140, 140)
					defer tt.rt.close()
					tt.SetScale(scale)
					tt.Scroll(50, 50, 30, 40)
					if track.X != 30 || track.Y != 40 {
						t.Fatalf("wheel no longer updates TrackScroll: %+v", track)
					}
					track.X, track.Y = 50, 60
					tt.Frame()
					g := scrollBars(Rect{st.x, st.y, st.w, st.h}, st.barInset, float32(st.contentW), float32(st.contentH), float32(st.scrollX), float32(st.scrollY), st.flags, ScrollbarStyle{}.Resolve(tt.rt.c.theme), st.rtl)
					trackRect := g.vTrack
					if horizontal {
						trackRect = g.hTrack
					}
					x, y := trackRect.X+trackRect.W/2, trackRect.Y+trackRect.H/2
					tt.Move(x, y)
					if st.barsShown != visible || tt.rt.overScrollbar(st.id) != visible {
						t.Fatalf("indicator/hover policy diverged: shown=%v over=%v", st.barsShown, tt.rt.overScrollbar(st.id))
					}
					wantCursor := CursorPointer
					if visible {
						wantCursor = CursorDefault
					}
					if tt.Cursor() != wantCursor {
						t.Fatalf("content cursor intercepted: got %v want %v", tt.Cursor(), wantCursor)
					}
					tt.ClickAt(x, y)
					if visible {
						if clicks != 0 {
							t.Fatal("default indicator stopped intercepting its track")
						}
					} else if clicks != 1 || st.scrollX != 50 || st.scrollY != 60 || tt.rt.focused == 0 {
						t.Fatalf("hidden track intercepted click/focus or changed offsets: clicks=%d offsets=%v,%v focus=%d", clicks, st.scrollX, st.scrollY, tt.rt.focused)
					}
				})
			}
		}
	}
}

func TestScrollbarsHiddenCancelsActiveDragWithoutContentClick(t *testing.T) {
	visible, clicks := true, 0
	var st *state
	tt := coreNewTester(func(c *context) {
		scroll := coreScrollBoth(c).Size(100, 100).Scrollbars(visible).Key("target")
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
	g := scrollBars(Rect{0, 0, 100, 100}, st.barInset, 400, 400, float32(st.scrollX), float32(st.scrollY), st.flags, ScrollbarStyle{}.Resolve(tt.rt.c.theme), false)
	x, y := g.h.X+g.h.W/2, g.h.Y+g.h.H/2
	tt.Press(x, y)
	if tt.rt.scrollDrag.st != st {
		t.Fatal("fixture did not start a native scrollbar drag")
	}
	tt.Move(x+10, y)
	beforeX, beforeY := st.scrollX, st.scrollY
	if beforeX <= 30 || beforeY != 40 {
		t.Fatal("fixture did not move a live thumb from nonzero offsets")
	}
	visible = false
	tt.Frame()
	if tt.rt.scrollDrag.st != nil || st.barsShown {
		t.Fatal("hiding left an active thumb drag or painted bar")
	}
	if tt.Cursor() != CursorText {
		t.Fatal("hiding did not restore the stationary pointer's content cursor")
	}
	tt.Move(99, y)
	tt.Release(99, y)
	if st.scrollX != beforeX || st.scrollY != beforeY || clicks != 0 {
		t.Fatal("cancelled bar drag scrolled or synthesized a content click")
	}
	tt.ClickAt(50, y)
	if clicks != 1 {
		t.Fatal("content did not become clickable after cancelling drag")
	}
}

func TestScrollbarsPolicyResetsByOmissionWithoutRelayout(t *testing.T) {
	hidden := true
	var st *state
	tt := coreNewTester(func(c *context) {
		scroll := coreScroll(c).Size(100, 100)
		if hidden {
			scroll.Scrollbars(false)
		}
		st = scroll.st
		scroll.Children(func() { coreBox(c).Size(100, 400) })
	}, 140, 140)
	defer tt.rt.close()
	tt.Move(50, 50)
	before := tt.rt.computed
	hidden = false
	tt.Frame()
	if st.barFlags == 0 || !st.barsShown {
		t.Fatal("omitting the override did not restore default hover indicators")
	}
	if tt.rt.computed != before {
		t.Fatal("indicator-only policy invalidated layout")
	}
}

func TestScrollbarsPolicyDoesNotInheritAndDoesNotHideNeighbor(t *testing.T) {
	var hidden, normal *state
	tt := coreNewTester(func(c *context) {
		coreRow(c).Scrollbars(false).Children(func() {
			scroll := coreScroll(c).Size(100, 100).Scrollbars(false)
			hidden = scroll.st
			scroll.Children(func() { coreBox(c).Size(100, 400) })
			scroll = coreScroll(c).Size(100, 100)
			normal = scroll.st
			scroll.Children(func() { coreBox(c).Size(100, 400) })
		})
	}, 240, 140)
	defer tt.rt.close()
	tt.Move(50, 50)
	if hidden.barsShown {
		t.Fatal("explicitly hidden indicator was painted")
	}
	tt.Move(150, 50)
	if !normal.barsShown || normal.barFlags == 0 {
		t.Fatal("neighbor inherited another container's indicator policy")
	}
}

func TestScrollbarsHiddenPreservesKeyboardAndAccessibilityReveal(t *testing.T) {
	var track ScrollState
	tt := coreNewTester(func(c *context) {
		coreScroll(c).Size(140, 100).Scrollbars(false).TrackScroll(&track).Children(func() {
			for i := range 12 {
				coreButton(c, fmt.Sprintf("Item %d", i)).Height(30).Shrink(0)
			}
		})
	}, 180, 140)
	defer tt.rt.close()
	if err := tt.Click("Item 0"); err != nil {
		t.Fatal(err)
	}
	for range 11 {
		tt.Key(0, KeyTab)
	}
	if track.Y <= 0 {
		t.Fatal("keyboard focus stopped revealing offscreen controls")
	}
	track.Y = 0
	tt.Frame()
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	last := accessNode(t, tt.h.access, platform.RoleButton, "Item 11")
	if last.Actions&platform.ActionScrollIntoView == 0 {
		t.Fatal("hidden indicators removed accessibility scrolling")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.AccessAction, ID: last.ID, Action: platform.AccessScrollIntoView})
	if track.Y <= 0 {
		t.Fatal("accessibility scroll-into-view stopped working")
	}
}

func TestScrollbarsPolicyChangesOnlyIndicatorPixels(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("scale%g", scale), func(t *testing.T) {
			visible := true
			tt := coreNewTester(func(c *context) {
				theme := *c.theme
				theme.Scrollbar = RGB(240, 20, 30)
				c.SetTheme(&theme)
				coreScrollBoth(c).Size(100, 100).Scrollbars(visible).Children(func() {
					coreBox(c).Size(400, 400).Background(RGB(255, 255, 255))
				})
			}, 140, 140)
			defer tt.rt.close()
			tt.SetScale(scale)
			tt.Move(50, 50)
			before := tt.Image()
			visible = false
			tt.Frame()
			after := tt.Image()
			changed := 0
			for y := range before.Rect.Dy() {
				for x := range before.Rect.Dx() {
					if before.RGBAAt(x, y) == after.RGBAAt(x, y) {
						continue
					}
					changed++
					if float32(x) < 88*scale && float32(y) < 88*scale || float32(x) >= 100*scale || float32(y) >= 100*scale {
						t.Fatalf("indicator policy changed content outside the edge tracks at %d,%d", x, y)
					}
					pixel := after.RGBAAt(x, y)
					if pixel.R != 255 || pixel.G != 255 || pixel.B != 255 {
						t.Fatalf("hidden indicator left paint at %d,%d: %v", x, y, pixel)
					}
				}
			}
			if changed == 0 {
				t.Fatal("visible fixture painted no indicator pixels")
			}
		})
	}
}

func TestScrollbarsHiddenListKeepsAccessibleRowScrolling(t *testing.T) {
	selected := -1
	list := ListState{Selected: &selected}
	tt := coreNewTester(func(c *context) {
		coreList(c, &list, 1000, func(i int) { coreTextf(c, "Row %d", i).Height(30) }).Scrollbars(false).Fill()
	}, 160, 120)
	defer tt.rt.close()
	tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
	row := accessNode(t, tt.h.access, platform.RoleListItem, "Row 0")
	tt.send(platform.SurfaceEvent{Kind: platform.AccessAction, ID: row.ID, Action: platform.AccessFocus})
	for range 10 {
		tt.Key(0, KeyDown)
	}
	if selected != 10 || list.frame.e.st.scrollY <= 0 || list.frame.e.st.barsShown {
		t.Fatalf("hidden indicators changed list keyboard/reveal: selection=%d offset=%v shown=%v", selected, list.frame.e.st.scrollY, list.frame.e.st.barsShown)
	}
}

func TestScrollbarsFinalFluentValueKeepsVisibleThumbDrag(t *testing.T) {
	var st *state
	tt := coreNewTester(func(c *context) {
		scroll := coreScrollBoth(c).Size(100, 100).Scrollbars(false).Scrollbars(true).Key("target")
		st = scroll.st
		scroll.Children(func() { coreBox(c).Size(400, 400) })
	}, 140, 140)
	defer tt.rt.close()
	tt.Move(50, 50)
	g := scrollBars(Rect{0, 0, 100, 100}, st.barInset, 400, 400, 0, 0, st.flags, ScrollbarStyle{}.Resolve(tt.rt.c.theme), false)
	x, y := g.h.X+g.h.W/2, g.h.Y+g.h.H/2
	tt.Press(x, y)
	tt.Frame()
	if st.barFlags == 0 || tt.rt.scrollDrag.st != st {
		t.Fatal("an intermediate fluent value cancelled the finally visible thumb")
	}
	tt.Move(x+10, y)
	tt.Release(x+10, y)
	if st.scrollX <= 0 {
		t.Fatal("finally visible thumb stopped dragging after a rebuild")
	}
}

// KeepScrollbars shows a container's bars with the pointer away, looking
// as they do on hover, only while its content overflows it; Scrollbars(false)
// hides them still, and leaving it out shows them on hover again.
func TestKeepScrollbars(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("scale%g", scale), func(t *testing.T) {
			keep, visible, tall := true, true, true
			var st *state
			tt := coreNewTester(func(c *context) {
				theme := *c.theme
				theme.Scrollbar = RGB(240, 20, 30)
				c.SetTheme(&theme)
				scroll := coreScrollBoth(c).Size(100, 100).Scrollbars(visible).KeepScrollbars(keep)
				st = scroll.st
				scroll.Children(func() {
					size := float32(400)
					if !tall {
						size = 50
					}
					coreBox(c).Size(size, size).Background(RGB(255, 255, 255))
				})
			}, 140, 140)
			defer tt.rt.close()
			tt.SetScale(scale)
			tt.Move(120, 120)
			tt.Frame()
			if !st.barsShown {
				t.Fatal("kept bars do not show with the pointer away")
			}
			away := tt.Image()
			tt.Move(50, 50)
			tt.Frame()
			hovered := tt.Image()
			if !st.barsShown {
				t.Fatal("kept bars do not show with the pointer over the container")
			}
			if !away.Rect.Eq(hovered.Rect) {
				t.Fatal("images differ in size")
			}
			for y := range away.Rect.Dy() {
				for x := range away.Rect.Dx() {
					if away.RGBAAt(x, y) != hovered.RGBAAt(x, y) {
						t.Fatalf("a kept bar looks otherwise with the pointer away, at %d,%d: %v, on hover %v", x, y, away.RGBAAt(x, y), hovered.RGBAAt(x, y))
					}
				}
			}
			// A kept thumb is dragged as a hovered one is.
			g := scrollBars(Rect{0, 0, 100, 100}, st.barInset, 400, 400, 0, 0, st.flags, ScrollbarStyle{}.Resolve(tt.rt.c.theme), false)
			tt.Move(120, 120)
			tt.Frame()
			x, y := g.v.X+g.v.W/2, g.v.Y+g.v.H/2
			tt.Press(x, y)
			tt.Move(x, y+20)
			tt.Release(x, y+20)
			if st.scrollY <= 0 {
				t.Fatal("dragging a kept thumb did not scroll")
			}
			// Content that fits shows no bar, kept or not.
			tall = false
			tt.Move(120, 120)
			tt.Frame()
			fits := tt.Image()
			keep = false
			tt.Frame()
			plain := tt.Image()
			for y := range fits.Rect.Dy() {
				for x := range fits.Rect.Dx() {
					if fits.RGBAAt(x, y) != plain.RGBAAt(x, y) {
						t.Fatalf("a kept bar shows over content that fits, at %d,%d", x, y)
					}
				}
			}
			// Hidden wins, and the default shows on hover only again.
			tall, keep, visible = true, true, false
			tt.Frame()
			if st.barsShown || st.barFlags != 0 {
				t.Fatal("Scrollbars(false) did not hide kept bars")
			}
			keep, visible = false, true
			tt.Frame()
			if st.barsShown {
				t.Fatal("omitting KeepScrollbars kept the bars with the pointer away")
			}
		})
	}
}

// A table's Scrollbars and KeepScrollbars are those of the list of its
// rows, which scrolls.
func TestTableScrollbars(t *testing.T) {
	for _, tc := range []struct {
		name          string
		visible, keep bool
		shown         bool
	}{
		{"default", true, false, false},
		{"kept", true, true, true},
		{"hidden", false, true, false},
	} {
		s := ListState{}
		tt := coreNewTester(func(c *context) {
			coreTable(c, &s, []TableColumn{{Title: "Name"}}, 200, func(row, col int) { coreTextf(c, "Cell %d", row) }).
				Scrollbars(tc.visible).KeepScrollbars(tc.keep).Size(200, 200)
		}, 240, 240)
		tt.Move(230, 230)
		tt.Frame()
		st := s.frame.e.st
		if st.flags&flagScrollY == 0 {
			tt.rt.close()
			t.Fatalf("%s: the list state is not the table's rows'", tc.name)
		}
		if st.barsShown != tc.shown || (st.barFlags != 0) != tc.visible {
			t.Errorf("%s: rows' bars shown %v hidden %v, want shown %v hidden %v", tc.name, st.barsShown, st.barFlags == 0, tc.shown, !tc.visible)
		}
		tt.rt.close()
	}
}

func TestScrollbarInCachedLayer(t *testing.T) {
	for _, c := range []struct {
		name  string
		cache bool
	}{{"plain", false}, {"cached", true}} {
		t.Run(c.name, func(t *testing.T) {
			tt := coreNewTester(func(ctx *context) {
				sc := coreScroll(ctx).Absolute().Left(160).Top(40).Size(100, 100).
					KeepScrollbars(true).ScrollbarStyle(ScrollbarStyle{
					TrackWidth: 12, ThumbWidth: 6,
					Track: RGB(240, 20, 30), Thumb: RGB(240, 20, 30),
				})
				if c.cache {
					sc.Cache()
				}
				sc.Children(func() {
					coreBox(ctx).Size(100, 400).Background(RGB(255, 255, 255))
				})
			}, 300, 180)
			defer tt.rt.close()
			img := tt.Image()
			if got, want := img.RGBAAt(254, 90), (color.RGBA{240, 20, 30, 255}); got != want {
				t.Fatalf("scrollbar track at (254,90) = %v, want %v", got, want)
			}
		})
	}
}
