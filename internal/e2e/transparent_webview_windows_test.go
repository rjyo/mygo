//go:build windows && (amd64 || arm64)

package e2e

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TestTransparentWebWindow reads the screen through a transparent web page.
// The red half must paint; the clear half must show the blue window behind it,
// rather than the opaque redirection bitmap of the page's host window.
func TestTransparentWebWindow(t *testing.T) {
	for _, tc := range []struct {
		name          string
		disableShadow bool
		disableResize bool
	}{
		{"shadowless", true, false},
		{"with shadow", false, false},
		{"fixed size", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			back := newWindow(t, mygo.WindowOptions{
				X: 40, Y: 40, Width: 400, Height: 300, Frameless: true,
				Content: ui.View(func(c *ui.Context) { c.Root().Background(ui.RGB(0, 0, 255)) }),
			})
			pixel := func(x float64, red bool) bool {
				r, g, b, ok := screenColor(back, x, 150)
				if red {
					return ok && r > 200 && g < 60 && b < 60
				}
				return ok && b > 200 && r < 60 && g < 60
			}
			eventually(t, "the blue window on the screen", func() bool { return pixel(100, false) && pixel(300, false) })

			front := newWindow(t, mygo.WindowOptions{
				X: 40, Y: 40, Width: 400, Height: 300,
				Frameless: true, Transparent: true, AlwaysOnTop: true,
				DisableShadow: tc.disableShadow, DisableResize: tc.disableResize,
			})
			front.Page().LoadHTML(`<html style="background:transparent"><body style="margin:0;background:transparent"><div style="position:absolute;left:50%;top:0;right:0;bottom:0;background:#ff0000"></div></body></html>`, "")
			waitFor(t, front, "document.readyState === 'complete' && document.querySelector('div') !== null")
			if noRedirect, _, _ := composition(front); !noRedirect {
				t.Fatal("the transparent WebView host still has an opaque redirection bitmap")
			}
			eventually(t, "the page's red half on the screen", func() bool { return pixel(300, true) })
			eventually(t, "the blue window through the page's clear half", func() bool { return pixel(100, false) })
		})
	}
}
