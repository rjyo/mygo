package ui

import (
	"testing"
	"time"
)

func TestThemeFollowsTheAccent(t *testing.T) {
	var theme *Theme
	tt := coreNewTester(func(c *context) { theme = c.Theme() }, 200, 100)
	if theme.Accent != LightTheme().Accent {
		t.Fatalf("without an accent of the desktop: %v", theme.Accent)
	}
	pink := Color{R: 219, G: 39, B: 119, A: 255}
	tt.SetPreferences(Preferences{Accent: pink})
	if theme.Accent != pink || theme.AccentText != (Color{R: 255, G: 255, B: 255, A: 255}) || theme.AccentHover == pink {
		t.Errorf("pink: accent %v, text %v, hover %v", theme.Accent, theme.AccentText, theme.AccentHover)
	}
	if theme.Focus.A == 0 || theme.Selection.A == 0 || theme.Focus.R != pink.R {
		t.Errorf("pink: focus %v, selection %v", theme.Focus, theme.Selection)
	}
	// Text on a light accent is dark.
	tt.SetPreferences(Preferences{Accent: Color{R: 250, G: 204, B: 21, A: 255}})
	if l := theme.AccentText.gray().R; l > 100 {
		t.Errorf("text on yellow: %v", theme.AccentText)
	}
	tt.SetDark(true)
	if !theme.Dark || theme.Accent != (Color{R: 250, G: 204, B: 21, A: 255}) {
		t.Errorf("dark: %+v", theme)
	}
}

// WithAccent gives a theme of the app's the shades the default theme
// takes from the desktop's accent, in its own appearance.
func TestThemeWithAccent(t *testing.T) {
	pink := Color{R: 219, G: 39, B: 119, A: 255}
	for _, dark := range []bool{false, true} {
		var followed *Theme
		tt := coreNewTester(func(c *context) { followed = c.Theme() }, 200, 100)
		tt.SetDark(dark)
		tt.SetPreferences(Preferences{Accent: pink})
		base := LightTheme()
		if dark {
			base = DarkTheme()
		}
		got := base.WithAccent(pink)
		if got.Accent != followed.Accent || got.AccentHover != followed.AccentHover ||
			got.AccentPressed != followed.AccentPressed || got.AccentText != followed.AccentText ||
			got.Selection != followed.Selection || got.Focus != followed.Focus {
			t.Errorf("dark %v: WithAccent %+v, the followed theme %+v", dark, got, followed)
		}
		if base.Accent == pink {
			t.Errorf("dark %v: WithAccent changed the theme it was called on", dark)
		}
	}
	// A translucent accent is made opaque; none keeps the theme's.
	if a := LightTheme().WithAccent(Color{R: 219, G: 39, B: 119, A: 128}).Accent; a != pink {
		t.Errorf("translucent: %v", a)
	}
	if a := DarkTheme().WithAccent(Color{}); *a != *DarkTheme() {
		t.Errorf("no accent: %+v", a)
	}
}

// WithPreferences makes a theme of the app's follow the desktop as the
// default theme does.
func TestThemeWithPreferences(t *testing.T) {
	p := Preferences{Accent: Color{R: 219, G: 39, B: 119, A: 255}, HighContrast: true, TextScale: 1.5}
	for _, dark := range []bool{false, true} {
		var followed *Theme
		tt := coreNewTester(func(c *context) { followed = c.Theme() }, 200, 100)
		tt.SetDark(dark)
		tt.SetPreferences(p)
		base := LightTheme()
		if dark {
			base = DarkTheme()
		}
		before := *base
		got := base.WithPreferences(p)
		if got.Accent != followed.Accent || got.AccentHover != followed.AccentHover ||
			got.Focus != followed.Focus || got.Border != followed.Border ||
			got.TextMuted != followed.TextMuted || got.FontSize != followed.FontSize ||
			got.ScrollbarStyle != followed.ScrollbarStyle {
			t.Errorf("dark %v: WithPreferences %+v, the followed theme %+v", dark, got, followed)
		}
		if *base != before {
			t.Errorf("dark %v: WithPreferences changed the theme it was called on", dark)
		}
	}
	// The usual settings leave the theme as it is.
	if got := DarkTheme().WithPreferences(Preferences{TextScale: 1}); !sameTheme(got, DarkTheme()) {
		t.Errorf("usual settings: %+v", got)
	}
}

// A theme that follows the desktop follows it again from what it was:
// the same settings twice change nothing, settings turned off go back,
// and what the app set in between stays.
func TestThemeWithPreferencesAgain(t *testing.T) {
	p := Preferences{Accent: Color{R: 219, G: 39, B: 119, A: 255}, HighContrast: true, TextScale: 1.5}
	usual := Preferences{TextScale: 1}
	for _, base := range []*Theme{LightTheme(), DarkTheme()} {
		once := base.WithPreferences(p)
		if twice := once.WithPreferences(p); !sameTheme(twice, once) {
			t.Errorf("dark %v: twice %+v, once %+v", base.Dark, twice, once)
		}
		// Every field following changes goes back.
		if off := once.WithPreferences(usual); !sameTheme(off, base) {
			t.Errorf("dark %v: turned off %+v, not %+v", base.Dark, off, base)
		}
		edited := *once
		edited.Background, edited.Border = Hex("#18181b"), Hex("#3f3f46")
		again := edited.WithPreferences(p)
		if again.Background != edited.Background || again.Border != Hex("#3f3f46").Mix(again.Text, 0.45) || again.FontSize != once.FontSize {
			t.Errorf("dark %v: edited then followed %+v", base.Dark, again)
		}
	}
	// The default theme, which follows the desktop, follows it again.
	var followed *Theme
	tt := coreNewTester(func(c *context) { followed = c.Theme() }, 200, 100)
	tt.SetPreferences(p)
	if got := followed.WithPreferences(p); !sameTheme(got, followed) {
		t.Errorf("the default theme followed again: %+v, not %+v", got, followed)
	}
}

// sameTheme reports whether a and b look the same, whatever they followed.
func sameTheme(a, b *Theme) bool {
	x, y := *a, *b
	x.followed, y.followed = nil, nil
	return x == y
}

func TestThemeFollowsContrastAndTextSize(t *testing.T) {
	var theme *Theme
	var prefs Preferences
	tt := coreNewTester(func(c *context) { theme, prefs = c.Theme(), c.Preferences() }, 200, 100)
	if prefs.TextScale != 1 || prefs.HighContrast || prefs.ReduceMotion {
		t.Fatalf("by default: %+v", prefs)
	}
	base := LightTheme()
	tt.SetPreferences(Preferences{HighContrast: true, TextScale: 1.5})
	if theme.FontSize != base.FontSize*1.5 {
		t.Errorf("font size %v, not %v", theme.FontSize, base.FontSize*1.5)
	}
	if theme.Border.gray().R >= base.Border.gray().R || theme.TextMuted.gray().R >= base.TextMuted.gray().R || theme.Focus.A != 255 {
		t.Errorf("high contrast: border %v, muted %v, focus %v", theme.Border, theme.TextMuted, theme.Focus)
	}
}

func TestAnimateWithoutMotion(t *testing.T) {
	target := float32(0)
	var got float32
	tt := coreNewTester(func(c *context) {
		got = coreBox(c).Size(10, 10).Animate("x", target, time.Second)
	}, 200, 100)
	target = 1
	tt.Frame()
	if got == 1 {
		t.Fatal("Animate jumped with motion")
	}
	tt.SetPreferences(Preferences{ReduceMotion: true})
	target = 2
	tt.Frame()
	if got != 2 {
		t.Errorf("without motion, Animate is at %v", got)
	}
}

func TestOwnThemeIgnoresPreferences(t *testing.T) {
	var accent Color
	tt := coreNewTester(func(c *context) {
		c.SetTheme(LightTheme())
		accent = c.Theme().Accent
	}, 200, 100)
	tt.SetPreferences(Preferences{Accent: Color{R: 219, G: 39, B: 119, A: 255}})
	if accent != LightTheme().Accent {
		t.Errorf("a theme of the app's took the desktop's accent: %v", accent)
	}
}
