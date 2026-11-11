//go:build linux && (amd64 || arm64)

package linux

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// TestWhiten draws a template tray image white while the desktop prefers
// dark, as macOS tints it, keeping its shape.
func TestWhiten(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 255})
	src.SetNRGBA(1, 0, color.NRGBA{0, 0, 0, 56})
	var b bytes.Buffer
	if err := png.Encode(&b, src); err != nil {
		t.Fatal(err)
	}
	out, err := whiten(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	for x, a := range []uint8{255, 56} {
		if got := color.NRGBAModel.Convert(img.At(x, 0)).(color.NRGBA); got != (color.NRGBA{255, 255, 255, a}) {
			t.Errorf("pixel %d is %v, want white with alpha %d", x, got, a)
		}
	}
}

// TestTrayTemplateAppearance checks the image given to AppIndicator: a
// template turns white before the callback, then returns to the original
// black; an image in color is unchanged in either scheme.
func TestTrayTemplateAppearance(t *testing.T) {
	oldSet := appIndicatorSetIconFull
	t.Cleanup(func() { appIndicatorSetIconFull = oldSet })
	var shown []byte
	appIndicatorSetIconFull = func(_ ptr, file, _ *byte) {
		var err error
		shown, err = os.ReadFile(goStr(ptr(unsafe.Pointer(file))))
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, template := range []bool{true, false} {
		src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		ink := color.NRGBA{40, 90, 140, 102}
		if template {
			ink = color.NRGBA{A: 102}
		}
		src.SetNRGBA(0, 0, ink)
		var data bytes.Buffer
		if err := png.Encode(&data, src); err != nil {
			t.Fatal(err)
		}
		var observed []color.NRGBA
		h := &trayAppearanceHandler{changed: func() {
			img, err := png.Decode(bytes.NewReader(shown))
			if err != nil {
				t.Fatal(err)
			}
			observed = append(observed, color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA))
		}}
		tr := &tray{iconDir: t.TempDir(), h: h}
		if err := tr.SetImage(data.Bytes(), template); err != nil {
			t.Fatal(err)
		}
		tr.appearanceChanged(true)
		tr.appearanceChanged(true) // no duplicate callback
		tr.appearanceChanged(false)
		light := ink
		if template {
			light = color.NRGBA{255, 255, 255, ink.A}
		}
		if len(observed) != 2 || observed[0] != light || observed[1] != ink {
			t.Errorf("template=%v: callbacks saw %v, want [%v %v]", template, observed, light, ink)
		}
		if !bytes.Equal(shown, data.Bytes()) || !bytes.Equal(tr.image, data.Bytes()) {
			t.Errorf("template=%v: original image was not preserved", template)
		}
	}
}

type trayAppearanceHandler struct{ changed func() }

func (*trayAppearanceHandler) Clicked()             {}
func (*trayAppearanceHandler) RightClicked()        {}
func (h *trayAppearanceHandler) AppearanceChanged() { h.changed() }

// TestMenuHides checks that a window with a hidden title bar keeps its menu
// bar out of sight, so that the window buttons stay at the top of the
// window, in the page's title bar (#730).
func TestMenuHides(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts platform.WindowOptions
		full bool
		want bool
	}{
		{"default", platform.WindowOptions{}, false, false},
		{"frameless", platform.WindowOptions{Frameless: true}, false, false},
		{"auto hide", platform.WindowOptions{AutoHideMenu: true}, false, true},
		{"full screen", platform.WindowOptions{}, true, true},
		{"hidden title bar", platform.WindowOptions{TitleBarStyle: "hidden"}, false, true},
		{"hidden inset title bar", platform.WindowOptions{TitleBarStyle: "hiddenInset"}, false, true},
	} {
		o := tc.opts
		w := &window{opts: &o, autoHideMenu: o.AutoHideMenu}
		if tc.full {
			w.state |= stateFullscreen
		}
		if got := w.menuHides(); got != tc.want {
			t.Errorf("%s: menuHides() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
