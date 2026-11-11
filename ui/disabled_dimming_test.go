package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestDisabledColorsKeepInputAndAccessibilitySemantics(t *testing.T) {
	for _, explicitColors := range []bool{false, true} {
		clicks := 0
		var paragraph *node
		tt := coreNewTester(func(c *context) {
			theme := LightTheme()
			theme.DisableAutoDimming = explicitColors
			c.SetTheme(theme)
			coreColumn(c).Padding(10).Gap(4).Children(func() {
				if coreButtonBase(c).Label("Disabled").Size(30, 20).Background(RGB(0, 0, 0)).Disabled(true).Clicked() {
					clicks++
				}
				paragraph = coreRichText(c).Children(func() {
					coreText(c, "Inline").TextColor(RGB(0, 0, 0)).Disabled(true)
				})
				coreButton(c, "Enabled")
			})
		}, 140, 140)
		pixel := tt.Image().RGBAAt(15, 15)
		if explicitColors && pixel.R != 0 || !explicitColors && pixel.R == 0 {
			t.Fatalf("disabled block opacity: explicit=%v, pixel=%v", explicitColors, pixel)
		}
		if len(paragraph.spans) != 1 || (paragraph.spans[0].Color.A == 255) != explicitColors {
			t.Fatalf("disabled inline opacity: explicit=%v, spans=%+v", explicitColors, paragraph.spans)
		}
		tt.Click("Disabled")
		if clicks != 0 || tt.Focused("Disabled") {
			t.Fatal("disabled button accepted input")
		}
		tt.Key(0, KeyTab)
		if !tt.Focused("Enabled") {
			t.Fatal("Tab did not skip disabled button")
		}
		tt.send(platform.SurfaceEvent{Kind: platform.AccessibilityOn})
		n := accessNode(t, tt.h.access, platform.RoleButton, "Disabled")
		if n.States&platform.AccessDisabled == 0 {
			t.Fatal("disabled button lost accessibility state")
		}
		tt.send(platform.SurfaceEvent{Kind: platform.AccessAction, ID: n.ID, Action: platform.AccessPress})
		if clicks != 0 {
			t.Fatal("accessibility activated disabled button")
		}
	}
}

func TestDisableAutoDimmingThemed(t *testing.T) {
	for _, windowOptOut := range []bool{false, true} {
		var block, paragraph, outside *node
		tt := coreNewTester(func(c *context) {
			window, local := LightTheme(), LightTheme()
			window.DisableAutoDimming = windowOptOut
			local.DisableAutoDimming = !windowOptOut
			c.SetTheme(window)
			coreColumn(c).Children(func() {
				coreThemed(c, local).Children(func() {
					block = coreBox(c).Size(20, 20).Background(RGB(0, 0, 0)).Disabled(true)
					paragraph = coreRichText(c).Children(func() {
						coreText(c, "Inline").TextColor(RGB(0, 0, 0)).Disabled(true)
					})
				})
				outside = coreBox(c).Size(20, 20).Background(RGB(0, 0, 0)).Disabled(true)
			})
		}, 140, 140)
		img := tt.Image()
		for _, tc := range []struct {
			e        *node
			undimmed bool
		}{{block, !windowOptOut}, {outside, windowOptOut}} {
			pixel := img.RGBAAt(int(tc.e.x+5), int(tc.e.y+5))
			if (pixel.R == 0) != tc.undimmed {
				t.Errorf("window opt-out=%v: disabled block at y=%v has pixel %v, want undimmed=%v", windowOptOut, tc.e.y, pixel, tc.undimmed)
			}
		}
		if len(paragraph.spans) != 1 || (paragraph.spans[0].Color.A == 255) != !windowOptOut {
			t.Errorf("window opt-out=%v: scoped inline spans=%+v", windowOptOut, paragraph.spans)
		}
	}
}
