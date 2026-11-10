//go:build windows && (amd64 || arm64)

package windows

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestWindowRedirectionBitmap(t *testing.T) {
	build := windowsBuild
	t.Cleanup(func() { windowsBuild = build })
	for _, test := range []struct {
		name        string
		build       uint32
		opts        platform.WindowOptions
		hiddenTitle bool
		composes    bool
		want        bool
		wantCompose bool
	}{
		{name: "opaque web page", build: 19045},
		{name: "opaque native UI", build: 19045, opts: platform.WindowOptions{Surface: true}},
		{name: "transparent web page on Windows 10", build: 19045, opts: platform.WindowOptions{Transparent: true}, want: true},
		{name: "transparent web page on Windows 11", build: 22621, opts: platform.WindowOptions{Transparent: true}, want: true},
		{name: "transparent hidden title bar", build: 19045, opts: platform.WindowOptions{Transparent: true}, hiddenTitle: true, want: true},
		{name: "native UI without composition", build: 19045, opts: platform.WindowOptions{Transparent: true, Surface: true}, wantCompose: true},
		{name: "native UI with composition", build: 19045, opts: platform.WindowOptions{Transparent: true, Surface: true}, composes: true, want: true, wantCompose: true},
		{name: "flyout without composition", build: 19045, opts: platform.WindowOptions{Flyout: &platform.Flyout{}}, wantCompose: true},
		{name: "flyout with composition", build: 19045, opts: platform.WindowOptions{Flyout: &platform.Flyout{}}, composes: true, want: true, wantCompose: true},
		{name: "material on Windows 10", build: 19045, opts: platform.WindowOptions{Vibrancy: "mica"}},
		{name: "material on Windows 11", build: 22621, opts: platform.WindowOptions{Vibrancy: "mica"}, want: true},
		{name: "material controls without composition", build: 22621, opts: platform.WindowOptions{Vibrancy: "mica"}, hiddenTitle: true, wantCompose: true},
		{name: "material controls with composition", build: 22621, opts: platform.WindowOptions{Vibrancy: "mica"}, hiddenTitle: true, composes: true, want: true, wantCompose: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			windowsBuild = func() uint32 { return test.build }
			w := &window{opts: &test.opts, hiddenTitleBar: test.hiddenTitle}
			calls := 0
			compose := func() *composition {
				calls++
				if test.composes {
					return &composition{}
				}
				return nil
			}
			if got := w.needsNoRedirect(compose); got != test.want {
				t.Errorf("needsNoRedirect = %v, want %v", got, test.want)
			}
			wantCalls := 0
			if test.wantCompose {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Errorf("composition initialized %d times, want %d", calls, wantCalls)
			}
		})
	}
}

func TestTransparentWebWindowSuppressesNativeFramePainting(t *testing.T) {
	for _, test := range []struct {
		name        string
		transparent bool
		frameless   bool
		shadow      bool
		surface     bool
		vibrancy    string
		handled     bool
	}{
		{"transparent frameless web page", true, true, false, false, "", true},
		{"opaque frameless window", false, true, false, false, "", false},
		{"transparent framed window", true, false, false, false, "", false},
		{"transparent window with shadow", true, true, true, false, "", false},
		{"transparent native UI", true, true, false, true, "", false},
		{"transparent material window", true, true, false, false, "acrylic", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := &window{
				b:         New(),
				opts:      &platform.WindowOptions{Transparent: test.transparent, Surface: test.surface},
				frameless: test.frameless,
				shadow:    test.shadow,
				vibrancy:  test.vibrancy,
				minW:      640,
				minH:      140,
				maxW:      1400,
				maxH:      360,
			}
			for _, message := range []struct {
				id   uint32
				want uintptr
			}{{wmNCPaint, 0}, {wmNCActivate, 1}} {
				result, handled := w.message(message.id, 0, 0)
				if handled != test.handled || handled && result != message.want {
					t.Fatalf("message %#x: result=%d handled=%v", message.id, result, handled)
				}
			}
			if w.minW != 640 || w.minH != 140 || w.maxW != 1400 || w.maxH != 360 {
				t.Fatal("frame painting changed the sizing constraints")
			}
		})
	}
}
