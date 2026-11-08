//go:build windows

package d3d11

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/raster"
	"github.com/egoist/mygo/internal/scene"
)

// Fractional viewports can remove whole card corners without a mask, while overlapping antialiasing must keep its original pixel coverage.
func TestReducedClipCoverageMatchesCPU(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hwnd, err := newHiddenWindow(256, 224)
	if err != nil {
		t.Fatal(err)
	}
	defer destroyWindow(hwnd)
	r, err := New(hwnd)
	if err != nil {
		t.Skip("Direct3D 11 unavailable:", err)
	}
	defer r.Release()
	for _, name := range []string{"lyrics", "left", "top", "right", "bottom", "asymmetric", "opaque-rounded-parent", "duplicate-fractional", "near-fractional", "outside-fractional", "cuts-corner", "duplicate-rounded", "continuous-card", "continuous-cut", "asymmetric-midpoint"} {
		t.Run(name, func(t *testing.T) {
			card := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 20, Y: 20, W: 200, H: 160}, Radii: [4]float32{10, 10, 10, 10}}
			hard := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{W: 256, H: 224}}
			wantMask := false
			switch name {
			case "continuous-card":
				card.Continuous, wantMask = true, true
			case "lyrics":
				// The measured opening card and lyrics viewport, scaled by 1/8.
				card.Rect, card.Radii = scene.Rect{X: 26.25, Y: 73.25, W: 175.875, H: 118.125}, [4]float32{0.8283625, 0.8283625, 0.8283625, 0.8283625}
				hard.Rect = scene.Rect{X: 120.25, Y: 27.189285, W: 153.5, H: 159.25}
			case "left":
				hard.Rect.X, hard.Rect.W = 50.25, 200
			case "top":
				hard.Rect.Y, hard.Rect.H = 50.25, 170
			case "right":
				hard.Rect.W = 180.25
			case "bottom":
				hard.Rect.H = 140.25
			case "asymmetric":
				card.Radii, hard.Rect.X = [4]float32{8, 12, 16, 20}, 70.25
			case "opaque-rounded-parent":
				card.Rect, card.Radii = scene.Rect{W: 256, H: 224}, [4]float32{10, 10, 10, 10}
				hard.Rect, hard.Radii = scene.Rect{X: 50.25, Y: 50.25, W: 150, H: 100}, [4]float32{8, 8, 8, 8}
			case "duplicate-fractional", "near-fractional", "outside-fractional":
				card.Rect.X, hard.Rect.X, wantMask = 20.25, 20.25, true
				if name == "near-fractional" {
					hard.Rect.X = 20.5
				} else if name == "outside-fractional" {
					hard.Rect.X = 20.125
				}
			case "cuts-corner":
				hard.Rect.X, wantMask = 25.25, true
			case "duplicate-rounded":
				hard, wantMask = card, true
			case "continuous-cut":
				card.Continuous, hard.Rect.X, wantMask = true, 70.25, true
			case "asymmetric-midpoint":
				card.Radii, hard.Rect.W, wantMask = [4]float32{70, 0, 0, 0}, 140.25, true
			}
			for _, transform := range []scene.Affine{{}, scene.Scaling(1.01, 1.01), scene.Rotation(11), scene.Projective(1, 0, 0, 1, 0, 0, 0.001, 0.0004)} {
				if name == "continuous-card" && !transform.Set {
					continue // the existing untransformed path uses the shader
				}
				s := &scene.Scene{Width: 256, Height: 224, Clear: scene.Color{A: 255}, Ops: []scene.Op{card, hard,
					{Kind: scene.OpFill, Rect: scene.Rect{X: -200, Y: -200, W: 2000, H: 2000}, Color: scene.Color{R: 255, G: 255, B: 255, A: 255}, Transform: transform},
					{Kind: scene.OpPopClip}, {Kind: scene.OpPopClip},
				}}
				if err := r.draw(s); err != nil {
					t.Fatal(err)
				}
				if (len(r.images) > 0) != wantMask {
					t.Fatalf("transform=%v: mask textures=%d, want mask=%t", transform, len(r.images), wantMask)
				}
				pix, stride, err := r.read()
				if err != nil {
					t.Fatal(err)
				}
				ref := raster.NewImage(s.Width, s.Height)
				raster.Render(ref, s)
				maxDiff, total := 0, 0
				for y := range s.Height {
					for x := range s.Width {
						diff := int(pix[y*stride+4*x]) - int(ref.Pix[y*ref.Stride+4*x])
						if diff < 0 {
							diff = -diff
						}
						maxDiff, total = max(maxDiff, diff), total+diff
					}
				}
				if maxDiff > 2 || float64(total)/float64(s.Width*s.Height) > 0.03 {
					t.Fatalf("transform=%v: clip coverage differs from CPU: max=%d mean=%f", transform, maxDiff, float64(total)/float64(s.Width*s.Height))
				}
				if err := r.present(0); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
