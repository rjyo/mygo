//go:build darwin

package metal

import (
	"errors"
	"math"
	"slices"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/gpu"
	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

func TestDrawsAsTheCPURenderer(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Skip("no Metal:", err)
	}
	defer r.Release()
	s := gputest.Scene()
	// Twice: the second frame updates what the first uploaded.
	for range 2 {
		pix, err := r.renderOffscreen(s)
		if err != nil {
			t.Fatal(err)
		}
		gputest.Compare(t, "metal", pix, s.Width*4, s)
	}
	s = gputest.ContinuousScene()
	pix, err := r.renderOffscreen(s)
	if err != nil {
		t.Fatal(err)
	}
	gputest.Compare(t, "metal-continuous", pix, s.Width*4, s)
}

// The CPU presenter starts without drawing resources; the first GPU
// frame creates them, and a frame after they are freed draws correctly.
func TestDrawingResourcesOnDemand(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Skip("no Metal:", err)
	}
	defer r.Release()
	if r.queue != 0 || r.sampler != 0 || r.empty != 0 || r.cur.pipeline != 0 {
		t.Fatal("created GPU drawing resources before a GPU frame")
	}
	s := gputest.Scene()
	for range 2 {
		pix, err := r.renderOffscreen(s)
		if err != nil {
			t.Fatal(err)
		}
		gputest.Compare(t, "metal-on-demand", pix, s.Width*4, s)
		pool(r.releaseTextures)
	}
}

// TestShaderLibrary checks that the library compiled ahead of time comes
// from shader.metal as it is, and that Metal loads it.
func TestShaderLibrary(t *testing.T) {
	if gpu.SourceSum(shaderSource) != shaderLibrarySum {
		t.Fatal("shader.metal changed since shaderlib.go was generated: run go generate ./internal/gpu/metal on macOS")
	}
	r, err := newRenderer()
	if err != nil {
		t.Skip("no Metal:", err)
	}
	defer r.Release()
	var lib id
	pool(func() { lib = r.compiledLibrary() })
	if lib == 0 {
		t.Fatal("Metal does not load the compiled library")
	}
	pool(func() { release(&lib) })
}

// TestChangedSince checks what frames drawn in memory copy into the
// drawables, which take turns, and after the GPU drew into one.
// TestWideColors draws wide colors into a float16 target, which keeps what
// leaves 0 to 1, and an Oklab gradient between them, which does not clamp;
// and the same scene into a BGRA8 one, which draws the nearest sRGB colors.
func TestWideColors(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Skip("no Metal:", err)
	}
	defer r.Release()
	if !r.SetWide(true) {
		t.Fatal(r.wideErr)
	}
	s := &scene.Scene{}
	s.Reset(32, 16, scene.Color{})
	green, red := [4]float32{-0.25, 1.1, -0.1, 1}, [4]float32{1.15, -0.2, -0.1, 1}
	s.Wide = []scene.WideColors{{Color: green, Set: scene.WideColor}, {Color: green, Color2: red, Set: scene.WideColor | scene.WideColor2}}
	srgbGreen, srgbRed := scene.Color{G: 255, A: 255}, scene.Color{R: 255, A: 255}
	s.Ops = append(s.Ops,
		scene.Op{Kind: scene.OpFill, Rect: scene.Rect{W: 32, H: 8}, Color: srgbGreen, Wide: 1},
		scene.Op{Kind: scene.OpFill, Rect: scene.Rect{Y: 8, W: 32, H: 8}, Paint: scene.PaintOklab, Gradient: [4]float32{0, 0, 32, 0},
			Color: srgbGreen, Color2: srgbRed, Wide: 2})
	var px [2][4]float32
	pool(func() {
		r.waitLast()
		r.cur = &r.formats[1]
		tex := r.newTexture(s.Width, s.Height, pixelFormatRGBA16Float, usageRenderTarget|usageShaderRead, nil, 0)
		if tex == 0 {
			err = errors.New("no target")
			return
		}
		defer release(&tex)
		var cb id
		if cb, err = r.encode(s, tex, nil); err != nil {
			return
		}
		blit := send(cb, "blitCommandEncoder")
		send(blit, "synchronizeResource:", tex)
		send(blit, "endEncoding")
		send(cb, "commit")
		send(cb, "waitUntilCompleted")
		pix := make([]uint16, s.Width*s.Height*4)
		msgGetBytes(tex, sel("getBytes:bytesPerRow:fromRegion:mipmapLevel:"), unsafe.Pointer(&pix[0]), uint(s.Width*8),
			mtlRegion{W: uint(s.Width), H: uint(s.Height), D: 1}, 0)
		at := func(x, y int) (c [4]float32) {
			for i := range c {
				c[i] = half(pix[(y*s.Width+x)*4+i])
			}
			return c
		}
		px[0], px[1] = at(4, 2), at(16, 12)
	})
	if err != nil {
		t.Fatal(err)
	}
	// The pixel holds the straight color premultiplied by alpha 1, in the
	// order of the target's components (RGBA).
	for i, want := range green {
		if d := px[0][i] - want; d > 0.01 || d < -0.01 {
			t.Errorf("solid wide color: component %d is %v, want %v", i, px[0][i], want)
		}
	}
	// Midway between two colors, at least one component is out of 0 to 1:
	// the mix of the greens and reds of the ends, in Oklab, keeps the
	// chroma the ends have beyond sRGB.
	if m := px[1]; !(m[0] < 0 || m[0] > 1 || m[1] < 0 || m[1] > 1 || m[2] < 0 || m[2] > 1) {
		t.Errorf("gradient midpoint %v is inside sRGB", m)
	}

	// A BGRA8 target draws the sRGB colors of the ops.
	pix, err := r.renderOffscreen(s)
	if err != nil {
		t.Fatal(err)
	}
	if b, g, red, a := pix[(2*s.Width+4)*4], pix[(2*s.Width+4)*4+1], pix[(2*s.Width+4)*4+2], pix[(2*s.Width+4)*4+3]; b != 0 || g != 255 || red != 0 || a != 255 {
		t.Errorf("BGRA8 target: %v %v %v %v, want the sRGB green", b, g, red, a)
	}
}

// half converts an IEEE half-precision float.
func half(h uint16) float32 {
	sign := float32(1)
	if h&0x8000 != 0 {
		sign = -1
	}
	e, m := int(h>>10)&0x1f, float32(h&0x3ff)
	switch e {
	case 0:
		return sign * m / 1024 / 16384
	case 31:
		return sign * float32(math.Inf(1))
	}
	return sign * (1 + m/1024) * float32(math.Pow(2, float64(e-15)))
}

// renderInto draws s as frames do into target, one of the textures
// standing for the layer's drawables, and returns its premultiplied BGRA
// rows, and whether the frame was drawn whole.
func (r *Renderer) renderInto(s *scene.Scene, target id) (pix []byte, whole bool, err error) {
	pool(func() {
		r.waitLast()
		r.cur = &r.formats[0]
		var cb id
		if cb, err = r.frameInto(s, target); err != nil {
			return
		}
		whole = r.lastWhole
		blit := send(cb, "blitCommandEncoder")
		send(blit, "synchronizeResource:", target)
		send(blit, "endEncoding")
		send(cb, "commit")
		send(cb, "waitUntilCompleted")
		pix = make([]byte, s.Width*s.Height*4)
		msgGetBytes(target, sel("getBytes:bytesPerRow:fromRegion:mipmapLevel:"), unsafe.Pointer(&pix[0]), uint(s.Width*4),
			mtlRegion{W: uint(s.Width), H: uint(s.Height), D: 1}, 0)
	})
	return pix, whole, err
}

// A frame redraws the canvas only where the scene changed since the last,
// and its drawable shows exactly what drawing it whole does, whatever the
// drawable held: here fills and a stripe under panes of glass, whose
// backdrops redraw with them, change from frame to frame, and the
// drawables come back holding junk, as Core Animation's may when it
// frees or purges one.
func TestRedrawsOnlyTheDamage(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Skip("no Metal:", err)
	}
	defer r.Release()
	a := gputest.Scene()
	var targets [3]id
	pool(func() {
		for i := range targets {
			targets[i] = r.newTexture(a.Width, a.Height, pixelFormatBGRA8Unorm, usageRenderTarget|usageShaderRead, nil, 0)
		}
	})
	defer pool(func() {
		for i := range targets {
			release(&targets[i])
		}
	})
	// Each frame recolors one of these fills: small ones, and the second
	// stripe, under the glass.
	var fills []int
	for i, op := range a.Ops {
		if op.Kind == scene.OpFill && op.Rect.W <= 40 && (len(fills) < 3 || op.Rect.Y == 410 && op.Rect.X == 40) {
			fills = append(fills, i)
		}
	}
	if len(fills) != 4 {
		t.Fatalf("found %d fills, want 4", len(fills))
	}
	colors := []scene.Color{{R: 30, G: 200, B: 90, A: 255}, {R: 200, G: 30, B: 90, A: 255}, {R: 90, G: 30, B: 200, A: 255}}
	s := *a
	s.Ops = slices.Clone(a.Ops)
	for frame := range 10 {
		if frame > 0 {
			op := &s.Ops[fills[frame%len(fills)]]
			op.Color = colors[frame%len(colors)]
		}
		target := targets[frame%len(targets)]
		if frame == 5 {
			junk := make([]byte, s.Width*s.Height*4)
			for i := range junk {
				junk[i] = byte(i * 7)
			}
			pool(func() {
				msgReplaceRegion(target, sel("replaceRegion:mipmapLevel:withBytes:bytesPerRow:"), mtlRegion{W: uint(s.Width), H: uint(s.Height), D: 1}, 0, unsafe.Pointer(&junk[0]), uint(s.Width*4))
			})
		}
		got, whole, err := r.renderInto(&s, target)
		if err != nil {
			t.Fatal(err)
		}
		if whole != (frame == 0) {
			t.Fatalf("frame %d: drawn whole %v", frame, whole)
		}
		want, err := r.renderOffscreen(&s)
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			if got[i] != want[i] {
				p := i / 4
				t.Fatalf("frame %d: pixel (%d, %d) is %v, drawn whole %v", frame, p%s.Width, p/s.Width, got[p*4:p*4+4], want[p*4:p*4+4])
			}
		}
	}
	gputest.Compare(t, "metal-damage", mustRender(t, r, &s, targets[10%len(targets)]), s.Width*4, &s)
}

func mustRender(t *testing.T, r *Renderer, s *scene.Scene, target id) []byte {
	t.Helper()
	pix, _, err := r.renderInto(s, target)
	if err != nil {
		t.Fatal(err)
	}
	return pix
}
