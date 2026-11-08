//go:build windows

package d3d11

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/raster"
	"github.com/egoist/mygo/internal/scene"
)

// A stationary or translated rounded card clips its content and hard children without uploading a whole-card mask.
func TestRoundedCardClipsTransformedContentWithoutMask(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hwnd, err := newHiddenWindow(192, 160)
	if err != nil {
		t.Fatal(err)
	}
	defer destroyWindow(hwnd)
	r, err := New(hwnd)
	if err != nil {
		t.Skip("Direct3D 11 unavailable:", err)
	}
	defer r.Release()
	for _, moved := range []bool{false, true} {
		move := scene.Affine{}
		if moved {
			move = scene.Translation(5, 7)
		}
		for _, inner := range []bool{false, true} {
			for _, transform := range []scene.Affine{{}, scene.Scaling(1.01, 1.01), scene.Rotation(11), scene.Projective(1, 0, 0, 1, 20, 15, 0.001, 0.0004)} {
				s := &scene.Scene{Width: 192, Height: 160, Clear: scene.Color{A: 255}}
				s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 13, Y: 11, W: 133, H: 117}, Radii: [4]float32{19, 19, 19, 19}, Transform: move})
				if inner {
					// #1418: a hard child reaching into its rounded parent's
					// bottom corners must retain those corners; #1420 also
					// folds a shared translation into the clips' frame positions.
					s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 13, Y: 70, W: 133, H: 58}, Transform: move})
				}
				s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{W: 190, H: 150}, Color: scene.Color{R: 255, G: 255, B: 255, A: 255}, Transform: transform})
				if inner {
					s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPopClip})
				}
				s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPopClip})
				if err := r.draw(s); err != nil {
					t.Fatal(err)
				}
				if len(r.images) != 0 {
					t.Fatalf("moved=%t inner=%t transform=%v: rounded card uploaded %d mask textures", moved, inner, transform, len(r.images))
				}
				pix, stride, err := r.read()
				if err != nil {
					t.Fatal(err)
				}
				ref := raster.NewImage(s.Width, s.Height)
				raster.Render(ref, s)
				maxDiff, totalDiff := 0, 0
				for y := range s.Height {
					for x := range s.Width {
						// This grayscale fixture has equal RGB channels in BGRA and RGBA.
						d := int(pix[y*stride+4*x]) - int(ref.Pix[y*ref.Stride+4*x])
						if d < 0 {
							d = -d
						}
						maxDiff = max(maxDiff, d)
						totalDiff += d
					}
				}
				if maxDiff > 20 || float64(totalDiff)/float64(s.Width*s.Height) > 0.2 {
					t.Fatalf("moved=%t inner=%t transform=%v: rounded coverage differs from CPU: max=%d mean=%f", moved, inner, transform, maxDiff, float64(totalDiff)/float64(s.Width*s.Height))
				}
				if err := r.present(0); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
