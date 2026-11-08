package gputest

import "github.com/egoist/mygo/internal/scene"

// NV12Frame returns a w×h frame of video in NV12 with the matrix m, its
// planes in memory with rows padded past their width, as decoders pad
// them: bars of saturated colors over a gradient of luma, which the
// renderers sample as planes and the CPU renderer converts (Restore).
func NV12Frame(w, h int, m scene.YUVMatrix) *scene.Image {
	ls, cs := w+13, 2*((w+1)/2)+6
	cw, ch := (w+1)/2, (h+1)/2
	luma, chroma := make([]byte, ls*h), make([]byte, cs*ch)
	for y := range h {
		for x := range w {
			luma[y*ls+x] = byte(16 + (219*x)/max(w-1, 1))
		}
	}
	// Six bars of chroma, Cb and Cr from 16 to 240.
	bars := [][2]byte{{240, 128}, {16, 240}, {128, 16}, {240, 240}, {16, 16}, {90, 200}}
	for y := range ch {
		for x := range cw {
			c := bars[x*len(bars)/cw]
			if y < ch/4 {
				c = [2]byte{128, 128} // gray
			}
			chroma[y*cs+2*x], chroma[y*cs+2*x+1] = c[0], c[1]
		}
	}
	return SurfaceImage(&scene.Surface{
		Format: scene.SurfaceNV12,
		Matrix: m,
		Planes: [2]scene.Plane{{Pix: luma, Stride: ls}, {Pix: chroma, Stride: cs}},
	}, w, h)
}

// SurfaceImage returns an image showing s, whose pixels convert it.
func SurfaceImage(s *scene.Surface, w, h int) *scene.Image {
	img := scene.NewImageRGBA(w, h, nil)
	img.Surface = s
	img.Restore = func() []byte { return s.RGBA(nil, w, h) }
	return img
}

// VideoScene draws frames of video in NV12 of each matrix, scaled,
// rounded, faded and in grayscale, as video elements show them.
func VideoScene() *scene.Scene {
	s := &scene.Scene{Width: 360, Height: 260, Clear: scene.Color{R: 255, G: 255, B: 255, A: 255}}
	add := func(op scene.Op) { s.Ops = append(s.Ops, op) }
	for i, m := range []scene.YUVMatrix{scene.BT601, scene.BT709, scene.BT2020, scene.BT601 + scene.FullRange, scene.BT709 + scene.FullRange, scene.BT2020 + scene.FullRange} {
		img := NV12Frame(96, 54, m)
		x, y := float32(8+(i%3)*116), float32(8+(i/3)*72)
		add(scene.Op{Kind: scene.OpImage, Rect: scene.Rect{X: x, Y: y, W: 96, H: 54}, Image: img, Src: scene.Rect{W: 96, H: 54}})
	}
	big := NV12Frame(64, 36, scene.BT709)
	// Scaled up, with rounded corners, faded, and in grayscale.
	add(scene.Op{Kind: scene.OpImage, Rect: scene.Rect{X: 8, Y: 160, W: 160, H: 90}, Radii: [4]float32{12, 12, 12, 12}, Image: big, Src: scene.Rect{W: 64, H: 36}})
	add(scene.Op{Kind: scene.OpImage, Rect: scene.Rect{X: 184, Y: 160, W: 80, H: 45}, Image: big, Src: scene.Rect{W: 64, H: 36}, Opacity: 0.5})
	add(scene.Op{Kind: scene.OpImage, Rect: scene.Rect{X: 272, Y: 160, W: 80, H: 45}, Image: big, Src: scene.Rect{W: 64, H: 36}, Grayscale: true})
	// An odd size, whose chroma has a half sample at its edges.
	add(scene.Op{Kind: scene.OpImage, Rect: scene.Rect{X: 184, Y: 216, W: 37, H: 21}, Image: NV12Frame(37, 21, scene.BT601), Src: scene.Rect{W: 37, H: 21}})
	return s
}
