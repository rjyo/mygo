package raster

import (
	"image"
	"math"
	"runtime"

	"github.com/egoist/mygo/internal/scene"
)

// Renderer draws the successive scenes of a window into Image, redrawing
// only where a scene differs from the one before (Damage): a blinking
// caret, a ticking clock or a button under the pointer redraws only itself.
type Renderer struct {
	// Image holds the last scene drawn, in mem.
	Image Image
	mem   *pixels

	// d draws the bands of large damage.
	d    drawer
	diff Damage
}

// Damage finds where each scene of a window differs from the one before,
// for renderers that keep the last frame and redraw only that: Renderer,
// and the GPU renderers.
//
// It compares the operations of the two scenes from both ends, so a change
// in the middle of the display list damages the boxes of the operations
// that changed, clipped as they draw. Glyphs whose pixels changed in an
// atlas, and images whose pixels changed, count as changed.
type Damage struct {
	// valid tells that the last scene, of w×h pixels, is remembered to
	// compare the next with.
	valid   bool
	w, h    int
	clear   scene.Color
	ops     []scene.Op
	glyphs  []scene.Glyph
	effects []scene.EffectOp
	// bounds is where each operation of the last scene drew, next where
	// those of the scene compared do; versions has the versions of its
	// images.
	bounds, next []image.Rectangle
	versions     []uint64
	mask, color  atlasMark
	damage       []image.Rectangle
	clips        []image.Rectangle
}

// atlasMark is the state of an atlas a scene drew from.
type atlasMark struct {
	atlas        *scene.Atlas
	gen, version uint64
}

// changes returns the rectangles of a that changed since the mark, or
// false when everything may have.
func (m *atlasMark) changes(a *scene.Atlas) ([]image.Rectangle, bool) {
	if a == nil || m.atlas == nil {
		return nil, a == m.atlas
	}
	if a != m.atlas {
		return nil, false
	}
	rects, full := a.Changes(m.gen, m.version)
	return rects, !full
}

func (m *atlasMark) set(a *scene.Atlas) {
	m.atlas = a
	if a != nil {
		m.gen, m.version = a.Generation(), a.Version()
	}
}

// Render draws s, and returns the rectangles of Image it changed. s may
// change once Render returns.
func (r *Renderer) Render(s *scene.Scene) []image.Rectangle {
	damage, whole := r.diff.Diff(s)
	if whole || r.Image.W != s.Width || r.Image.H != s.Height {
		// A new size, or Image was released (Release).
		r.resize(s.Width, s.Height)
		damage = r.diff.Whole(s)
	}
	for _, d := range damage {
		r.d.draw(&r.Image, s, d, r.diff.Next())
	}
	r.diff.Remember(s)
	return damage
}

// pixels is the memory of a Renderer's image (allocPixels).
type pixels struct {
	b       []byte
	mapped  bool
	cleanup runtime.Cleanup
}

// resize makes Image w×h, reusing its memory when it can.
func (r *Renderer) resize(w, h int) {
	n := 4 * w * h
	if r.mem == nil || len(r.mem.b) < n {
		r.mem.free()
		r.mem = allocPixels(n)
	}
	r.Image = Image{W: w, H: h, Stride: 4 * w, Pix: r.mem.b[:n]}
}

// Release frees the image and what Render remembers: the next Render
// draws everything. Image must not be used meanwhile.
func (r *Renderer) Release() {
	r.mem.free()
	*r = Renderer{}
}

// Invalidate makes the next Render draw everything.
func (r *Renderer) Invalidate() { r.diff.Invalidate() }

// Release forgets the last scene, and the memory it kept.
func (d *Damage) Release() { *d = Damage{} }

func area(rects []image.Rectangle) int {
	n := 0
	for _, d := range rects {
		n += d.Dx() * d.Dy()
	}
	return n
}

// Diff returns the rectangles where s differs from the last scene
// remembered (Remember), or whole when s must be drawn whole: there is
// none, or s changes too much of it. The rectangles are apart from each
// other, and each holds whole the backdrops of the effects it meets.
func (d *Damage) Diff(s *scene.Scene) (rects []image.Rectangle, whole bool) {
	d.next = d.opBounds(s, d.next[:0])
	if !d.valid || s.Width != d.w || s.Height != d.h || s.Clear != d.clear || !d.diff(s) {
		return d.Whole(s), true
	}
	return d.damage, false
}

// Whole returns the damage of drawing s whole, the rectangle of the frame.
func (d *Damage) Whole(s *scene.Scene) []image.Rectangle {
	d.damage = append(d.damage[:0], image.Rect(0, 0, s.Width, s.Height))
	return d.damage
}

// Next returns where each operation of the scene last compared (Diff)
// draws, within its clips.
func (d *Damage) Next() []image.Rectangle { return d.next }

// Invalidate makes the next Diff draw everything.
func (d *Damage) Invalidate() { d.valid = false }

// diff sets the damage between the last scene and s, and reports false
// when s must be drawn whole.
func (r *Damage) diff(s *scene.Scene) bool {
	maskRects, ok := r.mask.changes(s.MaskAtlas)
	if !ok {
		return false
	}
	colorRects, ok := r.color.changes(s.ColorAtlas)
	if !ok {
		return false
	}
	same := func(i, j int) bool { return r.same(i, s, j, maskRects, colorRects) }
	n, m := len(r.ops), len(s.Ops)
	pre := 0
	for pre < n && pre < m && same(pre, pre) {
		pre++
	}
	suf := 0
	for suf < n-pre && suf < m-pre && same(n-1-suf, m-1-suf) {
		suf++
	}
	r.damage = r.damage[:0]
	if n == m {
		for k := pre; k < n-suf; k++ {
			if !same(k, k) {
				r.damage = addRect(r.damage, r.bounds[k])
				r.damage = addRect(r.damage, r.next[k])
			}
		}
	} else {
		for i := pre; i < n-suf; i++ {
			r.damage = addRect(r.damage, r.bounds[i])
		}
		for j := pre; j < m-suf; j++ {
			r.damage = addRect(r.damage, r.next[j])
		}
	}
	r.addBackdrops(s)
	// Past half the window, drawing it whole costs about the same.
	return area(r.damage)*2 < s.Width*s.Height
}

// addBackdrops adds to the damage the effects of s that read their
// backdrops it meets, with those backdrops: draw reads a backdrop after
// drawing the operations before its effect within the area drawn, which
// must hold all of it, and then no other area may draw the effect. So the
// damage becomes rectangles apart from each other.
func (r *Damage) addBackdrops(s *scene.Scene) {
	if len(s.Effects) == 0 || len(r.damage) == 0 {
		return
	}
	for changed := true; changed; {
		changed = false
		for i := range s.Ops {
			op := &s.Ops[i]
			if op.Kind != scene.OpEffect || int(op.Start) >= len(s.Effects) || r.next[i].Empty() {
				continue
			}
			fx := &s.Effects[op.Start]
			if fx.Effect == nil || !fx.Effect.Backdrop {
				continue
			}
			need := scene.BackdropOf(op.Transform.Bounds(op.Rect), fx.Blur, s.Width, s.Height).Area.Union(r.next[i])
			for _, d := range r.damage {
				if d.Overlaps(need) && !need.In(d) {
					r.damage = addRect(r.damage, need)
					changed = true
					break
				}
			}
		}
		// Rectangles that overlap become one.
		for i := 0; i < len(r.damage); i++ {
			for j := i + 1; j < len(r.damage); j++ {
				if r.damage[i].Overlaps(r.damage[j]) {
					r.damage[i] = r.damage[i].Union(r.damage[j])
					r.damage = append(r.damage[:j], r.damage[j+1:]...)
					changed = true
					j = i
				}
			}
		}
	}
}

// same reports whether operation i of the last scene draws what operation
// j of s does.
func (r *Damage) same(i int, s *scene.Scene, j int, maskRects, colorRects []image.Rectangle) bool {
	a, b := r.ops[i], s.Ops[j]
	// Glyphs and effects are compared by what Start and End point at.
	sa, sb := a.Start, b.Start
	ga, gb := r.glyphs[:0], s.Glyphs[:0]
	if a.Kind == scene.OpGlyphs && b.Kind == scene.OpGlyphs {
		ga, gb = r.glyphs[a.Start:a.End], s.Glyphs[b.Start:b.End]
	}
	a.Start, a.End, b.Start, b.End = 0, 0, 0, 0
	// The CPU draws the sRGB colors, not those outside its gamut, which
	// Wide points at in each scene's own table.
	a.Wide, b.Wide = 0, 0
	// A spinner turning or a pulse fading is the system's to show, not the
	// frame's: only where and how it shows draws anew.
	if a.Kind == b.Kind && (a.Kind == scene.OpSpinner || a.Kind == scene.OpPulse) {
		a.Gradient[0], b.Gradient[0] = 0, 0
	}
	if a != b {
		return false
	}
	switch b.Kind {
	case scene.OpGlyphs:
		if len(ga) != len(gb) {
			return false
		}
		for k := range gb {
			g := &gb[k]
			if ga[k] != *g {
				x := ga[k]
				x.Wide = g.Wide
				if x != *g {
					return false
				}
			}
			rects := maskRects
			if g.Colored {
				rects = colorRects
			}
			at := image.Rect(int(g.U), int(g.V), int(g.U)+int(g.UW), int(g.V)+int(g.VH))
			for _, c := range rects {
				if c.Overlaps(at) {
					return false
				}
			}
		}
	case scene.OpImage:
		if b.Image != nil && b.Image.Version() != r.versions[i] {
			return false
		}
	case scene.OpEffect:
		if int(sa) >= len(r.effects) || int(sb) >= len(s.Effects) || r.effects[sa] != s.Effects[sb] {
			return false
		}
	}
	return true
}

// addRect adds a rectangle to damage, merged with one it touches.
func addRect(damage []image.Rectangle, b image.Rectangle) []image.Rectangle {
	if b.Empty() {
		return damage
	}
	for i, d := range damage {
		if d.Inset(-8).Overlaps(b) {
			damage[i] = d.Union(b)
			return damage
		}
	}
	damage = append(damage, b)
	if len(damage) > 8 {
		u := damage[0]
		for _, d := range damage[1:] {
			u = u.Union(d)
		}
		damage = append(damage[:0], u)
	}
	return damage
}

// opBounds appends where each operation of s draws, within the clips
// around it.
func (r *Damage) opBounds(s *scene.Scene, out []image.Rectangle) []image.Rectangle {
	clip := image.Rect(0, 0, s.Width, s.Height)
	r.clips = r.clips[:0]
	for i := range s.Ops {
		op := &s.Ops[i]
		var b image.Rectangle
		switch op.Kind {
		case scene.OpFill, scene.OpImage, scene.OpEffect, scene.OpHole, scene.OpSpinner, scene.OpPulse:
			b = outset(op.Transform.Bounds(op.Rect), 1)
		case scene.OpShadow:
			if op.Inset {
				b = outset(op.Transform.Bounds(op.Cast), 1)
			} else {
				e := 1.5*op.Blur + 1
				box := scene.Rect{X: op.Rect.X - e, Y: op.Rect.Y - e, W: op.Rect.W + 2*e, H: op.Rect.H + 2*e}
				b = outset(op.Transform.Bounds(box), 1)
			}
		case scene.OpGlyphs:
			for _, g := range s.Glyphs[op.Start:op.End] {
				b = b.Union(outset(op.Transform.Mul(g.Transform).Bounds(scene.Rect{X: g.X, Y: g.Y, W: g.W, H: g.H}), 1))
			}
		case scene.OpPushClip:
			// A clip that changes changes everything it cuts.
			b = outset(op.Transform.Bounds(op.Rect), 1)
			r.clips = append(r.clips, clip)
			clip = clip.Intersect(b)
		case scene.OpPopClip:
			if len(r.clips) > 0 {
				clip = r.clips[len(r.clips)-1]
				r.clips = r.clips[:len(r.clips)-1]
			}
		}
		out = append(out, b.Intersect(clip))
	}
	return out
}

func outset(rc scene.Rect, d float32) image.Rectangle {
	return image.Rect(int(math.Floor(float64(rc.X-d))), int(math.Floor(float64(rc.Y-d))),
		int(math.Ceil(float64(rc.X+rc.W+d))), int(math.Ceil(float64(rc.Y+rc.H+d))))
}

// Remember keeps what Diff needs of s, drawn, to compare the next scene
// with.
func (r *Damage) Remember(s *scene.Scene) {
	r.valid = true
	r.w, r.h = s.Width, s.Height
	r.clear = s.Clear
	if len(r.ops) > len(s.Ops) {
		clear(r.ops[len(s.Ops):])
	}
	r.ops = append(r.ops[:0], s.Ops...)
	r.glyphs = append(r.glyphs[:0], s.Glyphs...)
	if len(r.effects) > len(s.Effects) {
		clear(r.effects[len(s.Effects):])
	}
	r.effects = append(r.effects[:0], s.Effects...)
	r.bounds, r.next = r.next, r.bounds
	r.versions = r.versions[:0]
	for i := range s.Ops {
		v := uint64(0)
		if img := s.Ops[i].Image; img != nil {
			v = img.Version()
		}
		r.versions = append(r.versions, v)
	}
	r.mask.set(s.MaskAtlas)
	r.color.set(s.ColorAtlas)
}
