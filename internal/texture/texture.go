// Package texture is native.Texture: an image on the GPU that a native
// module's implementation draws into, as IOSurfaces on macOS, and that
// native UI shows as any image, with no copy through the CPU but for the
// CPU renderer's.
package texture

import (
	"runtime"
	"sync"
	"sync/atomic"
	"weak"

	"github.com/egoist/mygo/internal/imagesrc"
	"github.com/egoist/mygo/internal/scene"
)

type mark = imagesrc.Mark

// Texture is an image a native module presents frames into.
type Texture struct {
	mark
	id uint64
	s  *state
}

// state is what a texture's cleanup releases: the surfaces it holds.
type state struct {
	mu sync.Mutex
	// pending is the surface presented last, retained, which the next
	// frame shows; seq counts presents.
	pending uintptr
	seq     uint64
	w, h    int
	// watch asks the windows that show the texture for a frame, by their
	// key.
	watch map[any]*watcher

	// Main thread only: the image frames draw, and the surface it shows,
	// retained.
	img      *scene.Image
	shown    uintptr
	shownSeq uint64
	// past are the surfaces frames showed before, held, in use, while
	// frames in flight on the GPU may still read them.
	past [inFlight]uintptr
}

// inFlight bounds the frames a renderer has in flight that may still read
// a surface the texture no longer shows (the Metal renderer's ring of two,
// and one more).
const inFlight = 3

// watcher is a window that drew the texture: invalidate asks it for a
// frame, and seen is the present it drew last.
type watcher struct {
	invalidate func()
	seen       uint64
}

// staleAfter is how many presents a window may let pass without drawing
// the texture before it no longer counts as showing it. It draws it again
// as it shows it again, which watches it again.
const staleAfter = 128

var (
	lastID   atomic.Uint64
	textures sync.Map // uint64 → weak.Pointer[Texture]
)

// New makes a texture, which shows nothing until a frame is presented.
func New() *Texture {
	t := &Texture{id: lastID.Add(1), s: &state{watch: map[any]*watcher{}}}
	textures.Store(t.id, weak.Make(t))
	runtime.AddCleanup(t, func(s *state) { s.release() }, t.s)
	return t
}

// Lookup returns the texture of id, or nil once it is gone.
func Lookup(id uint64) *Texture {
	if p, ok := textures.Load(id); ok {
		return p.(weak.Pointer[Texture]).Value()
	}
	return nil
}

// ID identifies the texture for the platform.
func (t *Texture) ID() uint64 { return t.id }

// Present shows surface, an IOSurfaceRef whose reference it takes, in the
// next frame of the windows showing the texture, from any thread.
func (t *Texture) Present(surface uintptr) {
	if surface == 0 {
		return
	}
	// In use while the texture holds it: a pool of pixel buffers gives it
	// to the producer again only once native UI shows another.
	useSurface(surface)
	w, h := surfaceSize(surface)
	s := t.s
	s.mu.Lock()
	old := s.pending
	s.pending, s.w, s.h = surface, w, h
	s.seq++
	watch := make([]func(), 0, len(s.watch))
	for key, w := range s.watch {
		if s.seq-w.seen > staleAfter {
			// Its window stopped drawing it, or is gone.
			delete(s.watch, key)
			continue
		}
		watch = append(watch, w.invalidate)
	}
	s.mu.Unlock()
	releaseSurface(old)
	for _, f := range watch {
		f()
	}
}

// Size returns the size of the frames presented last, in pixels, which
// Image lays the texture out at, as DIPs.
func (t *Texture) Size() (w, h float32) {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	return float32(t.s.w), float32(t.s.h)
}

// Frame returns the image a frame draws, showing the surface presented
// last, or nil before any. invalidate asks the window of key for a frame,
// which the next present calls. Main thread only.
func (t *Texture) Frame(key any, invalidate func()) *scene.Image {
	s := t.s
	s.mu.Lock()
	if w := s.watch[key]; w != nil {
		w.invalidate, w.seen = invalidate, s.seq
	} else {
		s.watch[key] = &watcher{invalidate: invalidate, seen: s.seq}
	}
	surface, seq, w, h := s.pending, s.seq, s.w, s.h
	if surface != 0 && seq != s.shownSeq {
		retainSurface(surface)
	}
	s.mu.Unlock()
	if surface == 0 {
		return nil
	}
	if s.img == nil {
		s.img = scene.NewImageRGBA(w, h, nil)
		// The CPU renderer reads the surface's pixels; GPU renderers wrap
		// it, where they can (GPUTexture).
		s.img.Restore = func() []byte { return readSurface(s.shown) }
		s.img.GPU = s
	}

	if seq != s.shownSeq {
		releaseSurface(s.past[0])
		copy(s.past[:], s.past[1:])
		s.past[inFlight-1] = s.shown
		s.shown, s.shownSeq = surface, seq
		s.img.W, s.img.H = w, h
		s.img.Pix = nil
		s.img.Changed()
	}
	return s.img
}

// GPUTexture wraps the surface shown in a texture of the renderer, with no
// copy: Metal's, on macOS. Main thread only.
func (s *state) GPUTexture(api scene.GPUAPI, device uintptr) uintptr {
	if api != scene.Metal || s.img == nil {
		return 0
	}
	return metalTexture(device, s.shown, s.img.W, s.img.H)
}

func (s *state) release() {
	releaseSurface(s.pending)
	releaseSurface(s.shown)
	for _, p := range s.past {
		releaseSurface(p)
	}
}
