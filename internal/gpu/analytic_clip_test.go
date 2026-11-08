package gpu

import (
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// Transformed content inside a stationary rounded card used to allocate a whole-card mask on every animation frame.
func TestStationaryRoundedClipUsesFrameCoordinates(t *testing.T) {
	transforms := []struct {
		name string
		m    scene.Affine
	}{
		{"scale", scene.Scaling(0.85, 0.9)},
		{"rotate", scene.Translation(30, 25).Mul(scene.Rotation(12))},
		{"perspective", scene.Projective(1, 0, 0, 1, 20, 15, 0.001, 0.0004)},
	}
	for _, transform := range transforms {
		for _, inner := range []bool{false, true} {
			name := transform.name
			if inner {
				name += "/integer-child"
			}
			t.Run(name, func(t *testing.T) {
				outer := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{W: 256, H: 256}}
				card := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 10, Y: 10, W: 200, H: 150}, Radii: [4]float32{17, 17, 17, 17}}
				ops := []scene.Op{outer, card}
				wantScissor := Scissor{10, 10, 210, 160}
				if inner {
					ops = append(ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 20, Y: 20, W: 120, H: 100}})
					wantScissor = Scissor{20, 20, 140, 120}
				}
				ops = append(ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 20, Y: 20, W: 100, H: 80}, Color: scene.Color{R: 255, A: 255}, Transform: transform.m})
				if inner {
					ops = append(ops, scene.Op{Kind: scene.OpPopClip})
				}
				ops = append(ops, scene.Op{Kind: scene.OpPopClip}, scene.Op{Kind: scene.OpPopClip})
				b := Builder{NoCull: true, NoFold: true}
				defer b.Release()
				uploads := 0
				b.Build(&scene.Scene{Width: 256, Height: 256, Ops: ops}, func(img *scene.Image) uintptr { uploads++; return uintptr(img.ID()) })
				if uploads != 0 || len(b.maskSlots) != 0 || len(b.Instances) == 0 {
					t.Fatalf("stationary card uploaded %d masks, retained %d, drew %d instances", uploads, len(b.maskSlots), len(b.Instances))
				}
				wantClip := [4]float32{10, 10, 200, 150}
				wantRadii := scene.Corners(card.Rect, card.Radii, card.Continuous)
				for _, in := range b.Instances {
					if in.Transform1[3] != 0 || in.Clip != wantClip || in.ClipRadii != wantRadii {
						t.Fatalf("analytic clip mode %v rect %v radii %v, want 0 %v %v", in.Transform1[3], in.Clip, in.ClipRadii, wantClip, wantRadii)
					}
				}
				for _, batch := range b.Batches {
					if batch.ClipMask != 0 || batch.Scissor != wantScissor {
						t.Fatalf("analytic card lost exact hard scissor: mask %v scissor %v, want %v", batch.ClipMask, batch.Scissor, wantScissor)
					}
				}
			})
		}
	}
}

// Analytic single-card clipping must retain pixel masks when clips need combined coverage or separate coordinate systems.
func TestComplexClipRetainsCoverageMask(t *testing.T) {
	for _, name := range []string{"fractional-child", "fractional-ancestor", "multiple-rounded", "continuous-card", "transformed-clip", "gradient-mask", "image-mask"} {
		t.Run(name, func(t *testing.T) {
			outer := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{W: 256, H: 256}}
			card := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 10, Y: 10, W: 200, H: 150}, Radii: [4]float32{17, 17, 17, 17}}
			child := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 20, Y: 20, W: 120, H: 100}}
			switch name {
			case "continuous-card":
				card.Continuous = true
			case "fractional-child":
				child.Rect.X += 0.25
			case "fractional-ancestor":
				outer.Rect.X = 20.25 // this edge actually reaches into the rounded corner
			case "multiple-rounded":
				child.Radii = [4]float32{8, 8, 8, 8}
			case "transformed-clip":
				child.Transform = scene.Rotation(5)
			case "gradient-mask":
				card.Paint, card.Color, card.Color2, card.Gradient = scene.PaintLinear, scene.Color{A: 0}, scene.Color{A: 255}, [4]float32{10, 10, 210, 10}
			case "image-mask":
				card.Image = scene.NewImageRGBA(2, 1, []byte{0, 0, 0, 128, 0, 0, 0, 255})
				card.Src = scene.Rect{W: 2, H: 1}
			}
			fill := scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 20, Y: 20, W: 100, H: 80}, Color: scene.Color{R: 255, A: 255}, Transform: scene.Scaling(0.85, 0.9)}
			ops := []scene.Op{outer, card, child, fill, {Kind: scene.OpPopClip}, {Kind: scene.OpPopClip}, {Kind: scene.OpPopClip}}
			b := Builder{NoCull: true, NoFold: true}
			defer b.Release()
			uploads := 0
			b.Build(&scene.Scene{Width: 256, Height: 256, Ops: ops}, func(img *scene.Image) uintptr { uploads++; return uintptr(img.ID()) })
			if uploads == 0 || len(b.maskSlots) == 0 || len(b.Instances) == 0 {
				t.Fatalf("complex clip lost its mask: uploads %d retained %d instances %d", uploads, len(b.maskSlots), len(b.Instances))
			}
			for _, in := range b.Instances {
				if in.Transform1[3] != 1 && in.Transform1[3] != 2 {
					t.Fatalf("complex clip uses analytic mode %v", in.Transform1[3])
				}
			}
		})
	}
}

// A fractional lyrics viewport used to turn its rounded player card into two large pixel masks, even after clipping off whole corners.
func TestStationaryClipReducesCoverageChain(t *testing.T) {
	for _, name := range []string{"lyrics", "left", "top", "right", "bottom", "asymmetric", "distant-fractional", "opaque-rounded-parent"} {
		t.Run(name, func(t *testing.T) {
			card := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 20, Y: 20, W: 200, H: 160}, Radii: [4]float32{10, 10, 10, 10}}
			hard := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{W: 256, H: 224}}
			want, radii := card.Rect, card.Radii
			switch name {
			case "lyrics":
				hard.Rect = scene.Rect{X: 70, Y: 0.25, W: 180, H: 140}
				want, radii = scene.Rect{X: 70, Y: 20, W: 150, H: 120.25}, [4]float32{0, 10, 0, 0}
			case "left":
				hard.Rect.X, hard.Rect.W = 50.25, 200
				want, radii = scene.Rect{X: 50.25, Y: 20, W: 169.75, H: 160}, [4]float32{0, 10, 10, 0}
			case "top":
				hard.Rect.Y, hard.Rect.H = 50.25, 170
				want, radii = scene.Rect{X: 20, Y: 50.25, W: 200, H: 129.75}, [4]float32{0, 0, 10, 10}
			case "right":
				hard.Rect.W = 180.25
				want, radii = scene.Rect{X: 20, Y: 20, W: 160.25, H: 160}, [4]float32{10, 0, 0, 10}
			case "bottom":
				hard.Rect.H = 140.25
				want, radii = scene.Rect{X: 20, Y: 20, W: 200, H: 120.25}, [4]float32{10, 10, 0, 0}
			case "asymmetric":
				card.Radii, hard.Rect.X, hard.Rect.W = [4]float32{8, 12, 16, 20}, 70.25, 180
				want, radii = scene.Rect{X: 70.25, Y: 20, W: 149.75, H: 160}, [4]float32{0, 12, 16, 0}
			case "distant-fractional":
				hard.Rect.X = 0.25
			case "opaque-rounded-parent":
				card.Rect, card.Radii = scene.Rect{W: 256, H: 224}, [4]float32{10, 10, 10, 10}
				hard.Rect, hard.Radii = scene.Rect{X: 50.25, Y: 50.25, W: 150, H: 100}, [4]float32{8, 8, 8, 8}
				want, radii = hard.Rect, hard.Radii
			}
			s := &scene.Scene{Width: 256, Height: 224, Ops: []scene.Op{card, hard,
				{Kind: scene.OpFill, Rect: scene.Rect{W: 256, H: 224}, Color: scene.Color{A: 255}, Transform: scene.Scaling(1.01, 1.01)},
				{Kind: scene.OpPopClip}, {Kind: scene.OpPopClip},
			}}
			b := Builder{NoCull: true, NoFold: true}
			defer b.Release()
			uploads := 0
			b.Build(s, func(img *scene.Image) uintptr { uploads++; return uintptr(img.ID()) })
			if uploads != 0 || len(b.Instances) == 0 {
				t.Fatalf("reducible clip uploaded %d masks and drew %d instances", uploads, len(b.Instances))
			}
			for _, in := range b.Instances {
				if in.Transform1[3] != 0 || in.Clip != rect(want) || in.ClipRadii != radii {
					t.Fatalf("wrong reduced clip: rect=%v radii=%v mode=%v, want %v %v", in.Clip, in.ClipRadii, in.Transform1[3], rect(want), radii)
				}
			}
		})
	}
}

// Edges whose partial coverages multiply must keep a mask; intersecting their rectangles would brighten their antialiasing.
func TestOverlappingAntialiasingKeepsClipMask(t *testing.T) {
	for _, name := range []string{"duplicate-fractional", "near-fractional", "cuts-corner", "duplicate-rounded", "thin-strip", "continuous-cut"} {
		t.Run(name, func(t *testing.T) {
			card := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 20, Y: 20, W: 200, H: 160}, Radii: [4]float32{20, 20, 20, 20}}
			hard := scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 70.25, Y: 0, W: 180, H: 220}}
			switch name {
			case "duplicate-fractional", "near-fractional":
				card.Rect.X, hard.Rect = 20.25, scene.Rect{X: 20.25, Y: 0, W: 230, H: 220}
				if name == "near-fractional" {
					hard.Rect.X = 20.5
				}
			case "cuts-corner":
				hard.Rect.X = 25.25
			case "duplicate-rounded":
				hard = card
			case "thin-strip":
				card.Rect.W, hard.Rect = 0.75, scene.Rect{X: 20.25, Y: 0, W: 0.5, H: 220}
			case "continuous-cut":
				card.Continuous = true
			}
			s := &scene.Scene{Width: 256, Height: 224, Ops: []scene.Op{card, hard,
				{Kind: scene.OpFill, Rect: scene.Rect{W: 256, H: 224}, Color: scene.Color{A: 255}, Transform: scene.Scaling(1.01, 1.01)},
				{Kind: scene.OpPopClip}, {Kind: scene.OpPopClip},
			}}
			b := Builder{NoCull: true, NoFold: true}
			defer b.Release()
			uploads := 0
			b.Build(s, func(img *scene.Image) uintptr { uploads++; return uintptr(img.ID()) })
			if uploads == 0 || len(b.Instances) == 0 {
				t.Fatalf("overlapping antialiasing lost its mask: uploads=%d instances=%d", uploads, len(b.Instances))
			}
		})
	}
}
