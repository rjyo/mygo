//go:build darwin

package metal

import (
	"math"
	"slices"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo/internal/scene"
)

// Spinners (scene.OpSpinner) are Core Animation layers over the frame's,
// each a CAShapeLayer stroking its arc, which an animation turns, in a
// layer cutting it to its clip: the window server turns them between
// frames, which draw nothing for them.

var (
	spinnerOnce      sync.Once
	spinnerErr       bool
	cgPathCreate     func() uintptr
	cgPathMoveTo     func(path, m uintptr, x, y float64)
	cgPathLineTo     func(path, m uintptr, x, y float64)
	cgColorCreate    func(space uintptr, components *[4]float64) uintptr
	msgNumber        func(cls id, sel objc.SEL, v float64) id
	msgSetFloat32    func(obj id, sel objc.SEL, v float32)
	lineCapRound     id
	cfReleaseSpinner func(obj uintptr)
)

func loadSpinners() bool {
	spinnerOnce.Do(func() {
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			spinnerErr = true
			return
		}
		qc, err := purego.Dlopen("/System/Library/Frameworks/QuartzCore.framework/QuartzCore", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			spinnerErr = true
			return
		}
		sym := func(h uintptr, name string) uintptr {
			p, err := purego.Dlsym(h, name)
			if err != nil {
				spinnerErr = true
			}
			return p
		}
		create, move, line, color := sym(cg, "CGPathCreateMutable"), sym(cg, "CGPathMoveToPoint"), sym(cg, "CGPathAddLineToPoint"), sym(cg, "CGColorCreate")
		lineCap := sym(qc, "kCALineCapRound")
		if spinnerErr {
			return
		}
		purego.RegisterFunc(&cgPathCreate, create)
		purego.RegisterFunc(&cgPathMoveTo, move)
		purego.RegisterFunc(&cgPathLineTo, line)
		purego.RegisterFunc(&cgColorCreate, color)
		purego.RegisterFunc(&msgNumber, msgSend)
		purego.RegisterFunc(&msgSetFloat32, msgSend)
		purego.RegisterFunc(&cfReleaseSpinner, cfRelease)
		lineCapRound = *(*id)(ptr(lineCap))
	})
	return !spinnerErr
}

// spinnerLayer is the layers showing a spinner, and what they show.
type spinnerLayer struct {
	clip, shape, mask id
	shows             spinnerLook
	// shown is the part of the spinner's box that shows, in points: what
	// the frame paints over the spinner after it hides the rest.
	shown []scene.Rect
	// turn is the sweep and period of the arc the animation turns.
	turn [2]float32
}

// spinnerLook is what a spinner's layers show, in points.
type spinnerLook struct {
	clip, box scene.Rect
	width     float32
	color     scene.Color
	opacity   float32
	hidden    bool
}

// spinnerAt is a spinner of a scene: its op, its box within the clip
// around it, in pixels, and the parts of that the ops after it leave.
type spinnerAt struct {
	op        int
	box, clip scene.Rect
	shown     []scene.Rect
}

// AnimatesSpinners reports whether the renderer has Core Animation turn
// spinners (scene.OpSpinner): one drawing into a layer.
func (r *Renderer) AnimatesSpinners() bool { return r.layer != 0 && loadSpinners() }

// findSpinners returns the spinners of s, in the order it paints them,
// each with the parts of its box the ops painted after it leave: the
// layers show over the frame, which must hide what it paints over them.
// Shadows hide nothing, as a pane's spreads far around it, and the
// corners of rounded rectangles count as theirs.
func (r *Renderer) findSpinners(s *scene.Scene) []spinnerAt {
	out := r.spinAt[:0]
	clip := scene.Rect{W: float32(s.Width), H: float32(s.Height)}
	stack := r.spinClips[:0]
	cover := func(rc scene.Rect) {
		rc = rc.Intersect(clip)
		if rc.Empty() {
			return
		}
		for k := range out {
			out[k].shown = subtract(out[k].shown, rc)
		}
	}
	for i := range s.Ops {
		op := &s.Ops[i]
		switch op.Kind {
		case scene.OpPushClip:
			stack = append(stack, clip)
			clip = clip.Intersect(op.Transform.Bounds(op.Rect))
		case scene.OpPopClip:
			if len(stack) > 0 {
				clip, stack = stack[len(stack)-1], stack[:len(stack)-1]
			}
		case scene.OpSpinner:
			box := op.Transform.Bounds(op.Rect)
			at := spinnerAt{op: i, box: box, clip: clip}
			if k := len(out); k < cap(out) {
				at.shown = out[:k+1][k].shown[:0]
			}
			if v := box.Intersect(clip); !v.Empty() {
				at.shown = append(at.shown, v)
			}
			out = append(out, at)
		case scene.OpFill:
			if len(out) == 0 {
				continue
			}
			if op.Color.A > 0 || op.Paint != scene.PaintSolid {
				cover(op.Transform.Bounds(op.Rect))
			} else if scene.HasBorder(op.Border) && op.BorderColor.A > 0 {
				// Only the border: its sides.
				rc, b := op.Rect, op.Border
				for _, side := range [4]scene.Rect{
					{X: rc.X, Y: rc.Y, W: rc.W, H: b[0]},
					{X: rc.X + rc.W - b[1], Y: rc.Y, W: b[1], H: rc.H},
					{X: rc.X, Y: rc.Y + rc.H - b[2], W: rc.W, H: b[2]},
					{X: rc.X, Y: rc.Y, W: b[3], H: rc.H},
				} {
					if !side.Empty() {
						cover(op.Transform.Bounds(side))
					}
				}
			}
		case scene.OpImage, scene.OpEffect, scene.OpHole:
			if len(out) > 0 {
				cover(op.Transform.Bounds(op.Rect))
			}
		case scene.OpGlyphs:
			if len(out) == 0 {
				continue
			}
			for _, g := range s.Glyphs[op.Start:op.End] {
				cover(op.Transform.Mul(g.Transform).Bounds(scene.Rect{X: g.X, Y: g.Y, W: g.W, H: g.H}))
			}
		}
	}
	r.spinClips = stack[:0]
	r.spinAt = out
	return out
}

// subtract returns the parts of rects outside o, in place.
func subtract(rects []scene.Rect, o scene.Rect) []scene.Rect {
	n := len(rects)
	for k := 0; k < n; k++ {
		rc := rects[k]
		in := rc.Intersect(o)
		if in.Empty() {
			continue
		}
		// What is left above, below, left and right of the part covered.
		parts := [4]scene.Rect{
			{X: rc.X, Y: rc.Y, W: rc.W, H: in.Y - rc.Y},
			{X: rc.X, Y: in.Y + in.H, W: rc.W, H: rc.Y + rc.H - in.Y - in.H},
			{X: rc.X, Y: in.Y, W: in.X - rc.X, H: in.H},
			{X: in.X + in.W, Y: in.Y, W: rc.X + rc.W - in.X - in.W, H: in.H},
		}
		rects[k] = rects[n-1]
		rects = rects[:n-1]
		n--
		k--
		for _, p := range parts {
			if !p.Empty() {
				rects = append(rects, p)
			}
		}
	}
	return rects
}

// placeSpinners shows the spinners of s in layers, in the order it paints
// them, and reports whether any layer changed, which the frame then
// presents with.
func (r *Renderer) placeSpinners(s *scene.Scene, scale float64) bool {
	if r.layer == 0 || !loadSpinners() {
		return false
	}
	changed := false
	inv := float32(1 / scale)
	pts := func(rc scene.Rect) scene.Rect {
		return scene.Rect{X: rc.X * inv, Y: rc.Y * inv, W: rc.W * inv, H: rc.H * inv}
	}
	began := false
	begin := func() {
		if !began {
			began = true
			tx := class("CATransaction")
			send(tx, "begin")
			send(tx, "setDisableActions:", 1)
		}
	}
	spinners := r.findSpinners(s)
	for n := range spinners {
		at := &spinners[n]
		op := &s.Ops[at.op]
		opacity := op.Opacity
		if opacity == 0 {
			opacity = 1
		}
		look := spinnerLook{clip: pts(at.clip), box: pts(at.box), width: op.Border[0] * inv, color: op.Color, opacity: opacity,
			hidden: len(at.shown) == 0}
		r.spinShown = r.spinShown[:0]
		for _, v := range at.shown {
			r.spinShown = append(r.spinShown, pts(v))
		}
		if n == len(r.spinners) || r.spinners[n].shows != look || r.spinners[n].turn != [2]float32{op.Gradient[1], op.Gradient[2]} ||
			!slices.Equal(r.spinners[n].shown, r.spinShown) {
			begin()
			r.showSpinner(n, op, look, r.spinShown, scale)
			changed = true
		}
	}
	for len(r.spinners) > len(spinners) {
		begin()
		l := r.spinners[len(r.spinners)-1]
		send(l.clip, "removeFromSuperlayer")
		release(&l.mask)
		release(&l.shape)
		release(&l.clip)
		r.spinners = r.spinners[:len(r.spinners)-1]
		changed = true
	}
	if began {
		send(class("CATransaction"), "commit")
	}
	return changed
}

// showSpinner has the layers of spinner n show look, only where shown,
// making them first, and turn the arc of op.
func (r *Renderer) showSpinner(n int, op *scene.Op, look spinnerLook, shown []scene.Rect, scale float64) {
	if n == len(r.spinners) {
		c := send(send(class("CALayer"), "alloc"), "init")
		send(c, "setMasksToBounds:", 1)
		sh := send(send(class("CAShapeLayer"), "alloc"), "init")
		send(sh, "setFillColor:", 0)
		send(sh, "setLineCap:", lineCapRound)
		send(c, "addSublayer:", sh)
		send(r.superlayer, "addSublayer:", c)
		r.spinners = append(r.spinners, spinnerLayer{clip: c, shape: sh})
	}
	l := &r.spinners[n]
	// Above the frame's layer, whose geometry is the view's: flipped,
	// from the top-left corner, or from the bottom-left.
	flipped := send(r.superlayer, "isGeometryFlipped") != 0
	height := float32(msgRect(r.superlayer, sel("bounds")).H)
	sign := float64(1)
	if !flipped {
		sign = -1
	}
	if l.shows != look {
		send(l.clip, "setHidden:", boolArg(look.hidden))
		cy := look.clip.Y
		if !flipped {
			cy = height - look.clip.Y - look.clip.H
		}
		msgSetRect(l.clip, sel("setFrame:"), cgRect{float64(look.clip.X), float64(cy), float64(look.clip.W), float64(look.clip.H)})
		by := look.box.Y - look.clip.Y
		if !flipped {
			by = look.clip.Y + look.clip.H - look.box.Y - look.box.H
		}
		msgSetRect(l.shape, sel("setFrame:"), cgRect{float64(look.box.X - look.clip.X), float64(by), float64(look.box.W), float64(look.box.H)})
		msgSetFloat(l.shape, sel("setContentsScale:"), scale)
		msgSetFloat(l.shape, sel("setLineWidth:"), float64(look.width))
		c := look.color
		comps := [4]float64{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, float64(c.A) / 255 * float64(look.opacity)}
		color := cgColorCreate(srgb, &comps)
		send(l.shape, "setStrokeColor:", color)
		cfReleaseSpinner(color)
	}
	if !slices.Equal(l.shown, shown) || l.shows.clip != look.clip {
		r.maskSpinner(l, look, shown, flipped)
	}
	turn := [2]float32{op.Gradient[1], op.Gradient[2]}
	if l.shows.box.W != look.box.W || l.shows.box.H != look.box.H || l.shows.width != look.width || l.turn != turn {
		// The arc from 0 degrees, in the shape's own geometry.
		w, h := float64(look.box.W), float64(look.box.H)
		radius := (min(w, h) - float64(look.width)) / 2
		path := cgPathCreate()
		steps := max(8, int(turn[0]/3))
		for i := 0; i <= steps; i++ {
			a := float64(turn[0]) * float64(i) / float64(steps) * math.Pi / 180
			x, y := w/2+radius*math.Cos(a), h/2+sign*radius*math.Sin(a)
			if i == 0 {
				cgPathMoveTo(path, 0, x, y)
			} else {
				cgPathLineTo(path, 0, x, y)
			}
		}
		send(l.shape, "setPath:", path)
		cfReleaseSpinner(path)
	}
	if l.turn != turn {
		// Turning from where the arc starts at the frame's time.
		from := float64(op.Gradient[0]) * math.Pi / 180
		anim := send(class("CABasicAnimation"), "animationWithKeyPath:", nsString("transform.rotation.z"))
		send(anim, "setFromValue:", msgNumber(class("NSNumber"), sel("numberWithDouble:"), sign*from))
		send(anim, "setToValue:", msgNumber(class("NSNumber"), sel("numberWithDouble:"), sign*(from+2*math.Pi)))
		msgSetFloat(anim, sel("setDuration:"), float64(turn[1]))
		msgSetFloat32(anim, sel("setRepeatCount:"), float32(math.Inf(1)))
		send(anim, "setRemovedOnCompletion:", 0)
		send(l.shape, "addAnimation:forKey:", anim, nsString("turn"))
		l.turn = turn
	}
	l.shows = look
	l.shown = append(l.shown[:0], shown...)
}

// maskSpinner cuts the layers of l to the parts shown, unless that is all
// its box within the clip.
func (r *Renderer) maskSpinner(l *spinnerLayer, look spinnerLook, shown []scene.Rect, flipped bool) {
	whole := len(shown) == 1 && shown[0] == look.box.Intersect(look.clip)
	if whole || len(shown) == 0 {
		if l.mask != 0 {
			send(l.clip, "setMask:", 0)
			release(&l.mask)
		}
		return
	}
	if l.mask == 0 {
		l.mask = send(send(class("CAShapeLayer"), "alloc"), "init")
		send(l.clip, "setMask:", l.mask)
	}
	msgSetRect(l.mask, sel("setFrame:"), cgRect{0, 0, float64(look.clip.W), float64(look.clip.H)})
	path := cgPathCreate()
	for _, v := range shown {
		x, y := float64(v.X-look.clip.X), float64(v.Y-look.clip.Y)
		if !flipped {
			y = float64(look.clip.H) - y - float64(v.H)
		}
		w, h := float64(v.W), float64(v.H)
		cgPathMoveTo(path, 0, x, y)
		cgPathLineTo(path, 0, x+w, y)
		cgPathLineTo(path, 0, x+w, y+h)
		cgPathLineTo(path, 0, x, y+h)
		cgPathLineTo(path, 0, x, y)
	}
	send(l.mask, "setPath:", path)
	cfReleaseSpinner(path)
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// releaseSpinners takes the spinners' layers away.
func (r *Renderer) releaseSpinners() {
	if len(r.spinners) == 0 {
		return
	}
	tx := class("CATransaction")
	send(tx, "begin")
	send(tx, "setDisableActions:", 1)
	for i := range r.spinners {
		l := &r.spinners[i]
		send(l.clip, "removeFromSuperlayer")
		release(&l.mask)
		release(&l.shape)
		release(&l.clip)
	}
	send(tx, "commit")
	r.spinners = r.spinners[:0]
}
