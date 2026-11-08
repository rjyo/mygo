package ui

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// yuvFrames returns a w×h picture of gputest's bars in NV12, and the same
// picture in I420, its Cb and Cr in planes of their own, every plane in
// rows padded past their samples, as decoders pad them.
func yuvFrames(w, h int, m YUVMatrix) (nv12, i420 *YUVImage) {
	s := gputest.NV12Frame(w, h, scene.YUVMatrix(m)).Surface
	luma, chroma := s.Planes[0], s.Planes[1]
	nv12 = &YUVImage{Format: NV12, Width: w, Height: h, Matrix: m,
		Planes: [3][]byte{luma.Pix, chroma.Pix}, Strides: [3]int{luma.Stride, chroma.Stride}}
	cw, ch := (w+1)/2, (h+1)/2
	cs := cw + 5
	u, v := make([]byte, cs*ch), make([]byte, cs*ch)
	for y := range ch {
		for x := range cw {
			u[y*cs+x], v[y*cs+x] = chroma.Pix[y*chroma.Stride+2*x], chroma.Pix[y*chroma.Stride+2*x+1]
		}
	}
	i420 = &YUVImage{Format: I420, Width: w, Height: h, Matrix: m,
		Planes: [3][]byte{luma.Pix, u, v}, Strides: [3]int{luma.Stride, cs, cs}}
	return nv12, i420
}

// TestBitmapUpdateYUV shows frames of video in NV12 and in I420 in a
// bitmap: both draw the pixels their planes convert to, the planes stay
// the app's (but I420's chroma, which the bitmap interleaves into a plane
// it keeps), and a frame of the same size keeps the bitmap's image, so
// that the GPU keeps its textures and uploads the planes again.
func TestBitmapUpdateYUV(t *testing.T) {
	const w, h = 37, 21
	nv12, i420 := yuvFrames(w, h, BT709)
	want := gputest.NV12Frame(w, h, scene.BT709).Surface.RGBA(nil, w, h)
	bm := NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	tt := coreNewTester(func(c *context) {
		coreImage(c, bm).Size(w, h)
	}, w, h)
	shows := func(what string) {
		t.Helper()
		tt.Frame()
		if got := tt.Image(); !bytes.Equal(got.Pix, want) {
			t.Errorf("%s draws other pixels than its planes convert to", what)
		}
		if bw, bh := bm.Size(); bw != w || bh != h {
			t.Errorf("%s: the bitmap is %d×%d", what, bw, bh)
		}
	}

	bm.UpdateYUV(nv12)
	s := bm.img.Surface
	if s == nil || s.Format != scene.SurfaceNV12 || s.Matrix != scene.BT709 {
		t.Fatalf("NV12 shows as %+v", s)
	}
	if &s.Planes[0].Pix[0] != &nv12.Planes[0][0] || &s.Planes[1].Pix[0] != &nv12.Planes[1][0] {
		t.Error("NV12's planes were copied")
	}
	shows("NV12")

	img, ver := bm.img, bm.img.Version()
	bm.UpdateYUV(i420)
	if bm.img != img || bm.img.Version() == ver {
		t.Error("a frame of the same size made another image, or left its version")
	}
	s = bm.img.Surface
	if &s.Planes[0].Pix[0] != &i420.Planes[0][0] || s.Planes[1].Stride != 2*((w+1)/2) {
		t.Errorf("I420 shows as luma copied, or chroma in rows of %d", s.Planes[1].Stride)
	}
	chroma := &s.Planes[1].Pix[0]
	shows("I420")
	bm.UpdateYUV(i420)
	if &bm.img.Surface.Planes[1].Pix[0] != chroma {
		t.Error("the next I420 frame interleaved its chroma into another plane")
	}

	// Another size, and an RGBA picture again.
	small, _ := yuvFrames(8, 6, BT601)
	bm.UpdateYUV(small)
	if bw, bh := bm.Size(); bw != 8 || bh != 6 || bm.img == img {
		t.Errorf("a frame of 8×6: a bitmap of %d×%d, the same image %v", bw, bh, bm.img == img)
	}
	red := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range len(red.Pix) / 4 {
		copy(red.Pix[4*i:], []byte{255, 0, 0, 255})
	}
	bm.Update(red)
	tt.Frame()
	if bm.img.Surface != nil || tt.Image().RGBAAt(w/2, h/2) != (color.RGBA{255, 0, 0, 255}) {
		t.Error("an RGBA picture after frames of video does not draw")
	}
}

// TestYUVImageMatrix checks that a frame's matrix and range reach the
// renderers as the scene's.
func TestYUVImageMatrix(t *testing.T) {
	var chroma []byte
	for _, c := range []struct {
		m    YUVMatrix
		full bool
		want scene.YUVMatrix
	}{
		{BT601, false, scene.BT601},
		{BT709, false, scene.BT709},
		{BT2020, false, scene.BT2020},
		{BT601, true, scene.BT601 + scene.FullRange},
		{BT709, true, scene.BT709 + scene.FullRange},
		{BT2020, true, scene.BT2020 + scene.FullRange},
	} {
		img, _ := yuvFrames(4, 4, c.m)
		img.FullRange = c.full
		if got := img.surface(&chroma).Matrix; got != c.want {
			t.Errorf("matrix %d, full range %v: %d, want %d", c.m, c.full, got, c.want)
		}
	}
}

// TestYUVImageTooSmall checks that a frame whose planes do not hold its
// picture is refused, rather than read past its planes by a renderer.
func TestYUVImageTooSmall(t *testing.T) {
	for name, change := range map[string]func(*YUVImage){
		"no size":          func(m *YUVImage) { m.Width = 0 },
		"short luma":       func(m *YUVImage) { m.Planes[0] = m.Planes[0][:m.Strides[0]*(m.Height-1)+m.Width-1] },
		"narrow luma rows": func(m *YUVImage) { m.Strides[0] = m.Width - 1 },
		"short chroma":     func(m *YUVImage) { m.Planes[1] = m.Planes[1][:m.Strides[1]*((m.Height+1)/2-1)+2*((m.Width+1)/2)-1] },
		"no Cr":            func(m *YUVImage) { m.Format, m.Planes[2] = I420, nil },
		"odd chroma rows":  func(m *YUVImage) { m.Strides[1]++; m.Planes[1] = append(m.Planes[1], make([]byte, 16)...) },
		"another matrix":   func(m *YUVImage) { m.Matrix = BT2020 + 1 },
		"another format":   func(m *YUVImage) { m.Format = I420 + 1 },
	} {
		img, _ := yuvFrames(9, 7, BT709)
		change(img)
		func() {
			defer func() {
				if r, ok := recover().(string); !ok || !strings.HasPrefix(r, "ui: ") {
					t.Errorf("%s: recovered %v", name, r)
				}
			}()
			NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1))).UpdateYUV(img)
		}()
	}
}

// BenchmarkUpdateYUV shows a new 1080×2400 frame each time, as a phone's
// screen streams: NV12's planes are taken as they are, and I420's chroma
// is interleaved into NV12's, a quarter of the frame's bytes.
func BenchmarkUpdateYUV(b *testing.B) {
	nv12, i420 := yuvFrames(1080, 2400, BT709)
	for _, img := range []*YUVImage{nv12, i420} {
		name := map[YUVFormat]string{NV12: "NV12", I420: "I420"}[img.Format]
		b.Run(name, func(b *testing.B) {
			bm := NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
			b.SetBytes(int64(img.Width * img.Height * 3 / 2))
			for b.Loop() {
				bm.UpdateYUV(img)
			}
		})
	}
}
