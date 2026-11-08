//go:build js

package video

import (
	"errors"
	"syscall/js"
	"time"

	"github.com/egoist/mygo/internal/media"
	"github.com/egoist/mygo/internal/scene"
)

// In browsers, a video element plays the video, not in the page: it
// decodes, plays the sound and keeps the clock, and WebGL uploads each
// picture from it, GPU to GPU (texSubImage2D of the element). Where the
// browser has requestVideoFrameCallback, each picture the element
// presents wakes the window; elsewhere the window asks for frames at the
// display's rate while it plays. Bytes in memory play from a Blob's URL.

type elementEngine struct {
	el     js.Value
	native uintptr
	blob   string // the object URL of bytes in memory, revoked on Close
	funcs  []js.Func
	names  []string
	frameF js.Func
	rvfc   bool
	// frameReq is the pending requestVideoFrameCallback's handle.
	frameReq js.Value

	wake    func()
	state   media.State
	err     error
	fresh   bool // a picture presented since the last Frame
	lastT   float64
	shown   bool
	surface *scene.Surface
	canvas  js.Value // for reading pictures back (RGBA)
	closed  bool
}

func newEngine(s source) (media.Engine, error) {
	doc := js.Global().Get("document")
	if doc.IsUndefined() {
		return nil, ErrUnsupported
	}
	e := &elementEngine{el: doc.Call("createElement", "video")}
	el := e.el
	el.Set("playsInline", true)
	el.Set("preload", "auto")
	el.Set("crossOrigin", "anonymous") // for WebGL to read a picture of another origin that allows it
	url := s.url
	if s.data != nil {
		arr := js.Global().Get("Uint8Array").New(len(s.data))
		js.CopyBytesToJS(arr, s.data)
		blob := js.Global().Get("Blob").New([]any{arr}, map[string]any{"type": mimeOf(s.data)})
		e.blob = js.Global().Get("URL").Call("createObjectURL", blob).String()
		url = e.blob
	}
	e.native = media.AddElement(el)
	e.surface = &scene.Surface{Format: scene.SurfaceBGRA, Native: e.native}
	on := func(name string, fn func()) {
		f := js.FuncOf(func(js.Value, []js.Value) any {
			if !e.closed {
				fn()
				e.wakeWindow()
			}
			return nil
		})
		e.funcs, e.names = append(e.funcs, f), append(e.names, name)
		el.Call("addEventListener", name, f)
	}
	on("loadeddata", func() { e.fresh = true })
	on("seeked", func() { e.fresh = true })
	on("waiting", func() {
		if e.state == media.Ready {
			e.state = media.Buffering
		}
	})
	on("playing", func() {
		if e.state == media.Buffering {
			e.state = media.Ready
		}
	})
	on("ended", func() {
		if !el.Get("loop").Bool() {
			e.state = media.Ended
		}
	})
	on("error", func() {
		e.state = media.Failed
		msg := "the browser could not play the video"
		if me := el.Get("error"); !me.IsNull() && !me.IsUndefined() {
			if m := me.Get("message").String(); m != "" {
				msg = m
			}
		}
		e.err = errors.New("video: " + msg)
	})
	on("durationchange", func() {})
	if !el.Get("requestVideoFrameCallback").IsUndefined() {
		e.rvfc = true
		e.frameF = js.FuncOf(func(js.Value, []js.Value) any {
			if e.closed {
				return nil
			}
			e.fresh = true
			e.frameReq = el.Call("requestVideoFrameCallback", e.frameF)
			e.wakeWindow()
			return nil
		})
		e.frameReq = el.Call("requestVideoFrameCallback", e.frameF)
	}
	el.Set("src", url)
	return e, nil
}

// mimeOf is the MIME type of a video's bytes, as their container says.
func mimeOf(b []byte) string {
	switch {
	case len(b) >= 4 && string(b[:4]) == "\x1a\x45\xdf\xa3":
		return "video/webm"
	case len(b) >= 12 && string(b[4:8]) == "ftyp" && string(b[8:12]) == "qt  ":
		return "video/quicktime"
	}
	return "video/mp4"
}

func (e *elementEngine) wakeWindow() {
	if e.wake != nil {
		e.wake()
	}
}

func (e *elementEngine) SetWake(wake func()) { e.wake = wake }

func (e *elementEngine) Frame(time.Time) (media.Picture, bool, bool) {
	if e.closed || e.el.Get("readyState").Int() < 2 { // HAVE_CURRENT_DATA
		return media.Picture{}, false, false
	}
	t := e.el.Get("currentTime").Float()
	changed := e.fresh || !e.shown || (!e.rvfc && t != e.lastT)
	e.fresh, e.shown, e.lastT = false, true, t
	if e.state == media.Loading {
		e.state = media.Ready
	}
	w, h := e.Size()
	return media.Picture{Surface: e.surface, W: w, H: h, RGBA: func(dst []byte) []byte { return e.readBack(dst, w, h) }}, changed, true
}

// readBack draws the picture into a canvas of its own, for the CPU
// renderer, and copies its pixels, premultiplied as they are opaque.
func (e *elementEngine) readBack(dst []byte, w, h int) []byte {
	if e.closed || w <= 0 || h <= 0 {
		return nil
	}
	if e.canvas.IsUndefined() || e.canvas.IsNull() {
		e.canvas = js.Global().Get("document").Call("createElement", "canvas")
	}
	e.canvas.Set("width", w)
	e.canvas.Set("height", h)
	ctx := e.canvas.Call("getContext", "2d", map[string]any{"willReadFrequently": true})
	if ctx.IsNull() {
		return nil
	}
	ctx.Call("drawImage", e.el, 0, 0, w, h)
	data := ctx.Call("getImageData", 0, 0, w, h).Get("data")
	n := 4 * w * h
	if cap(dst) < n {
		dst = make([]byte, n)
	}
	dst = dst[:n]
	js.CopyBytesToGo(dst, js.Global().Get("Uint8Array").New(data.Get("buffer"), data.Get("byteOffset"), n))
	return dst
}

// Interval is 0 where each picture the element presents wakes the
// window, else the display's rate.
func (e *elementEngine) Interval() time.Duration {
	if e.rvfc {
		return 0
	}
	return time.Second / 60
}

func (e *elementEngine) State() (media.State, error) { return e.state, e.err }

func (e *elementEngine) Size() (int, int) {
	return e.el.Get("videoWidth").Int(), e.el.Get("videoHeight").Int()
}

func (e *elementEngine) Duration() time.Duration {
	d := e.el.Get("duration").Float()
	if d != d || d <= 0 || d > 1e12 { // NaN, or Infinity for a live stream
		return 0
	}
	return time.Duration(d * float64(time.Second))
}

func (e *elementEngine) Position() time.Duration {
	return time.Duration(e.el.Get("currentTime").Float() * float64(time.Second))
}

func (e *elementEngine) Play() {
	if e.state == media.Ended {
		e.state = media.Ready
		e.el.Set("currentTime", 0)
	}
	// A browser that does not let a page play sound before the user
	// interacted with it rejects the promise: the video plays muted.
	p := e.el.Call("play")
	if p.Truthy() && !p.Get("catch").IsUndefined() {
		var f js.Func
		f = js.FuncOf(func(js.Value, []js.Value) any {
			f.Release()
			if !e.closed && !e.el.Get("muted").Bool() {
				e.el.Set("muted", true)
				e.el.Call("play")
			}
			return nil
		})
		p.Call("catch", f)
	}
}

func (e *elementEngine) Pause() { e.el.Call("pause") }

func (e *elementEngine) Seek(d time.Duration) {
	if e.state == media.Ended {
		e.state = media.Ready
	}
	e.el.Set("currentTime", d.Seconds())
}

func (e *elementEngine) SetLoop(loop bool) { e.el.Set("loop", loop) }

func (e *elementEngine) SetRate(r float64) {
	e.el.Set("defaultPlaybackRate", r)
	e.el.Set("playbackRate", r)
}

func (e *elementEngine) SetVolume(v float64) { e.el.Set("volume", v) }

func (e *elementEngine) SetMuted(m bool) { e.el.Set("muted", m) }

func (e *elementEngine) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	e.wake = nil
	for i, f := range e.funcs {
		e.el.Call("removeEventListener", e.names[i], f)
		f.Release()
	}
	if e.rvfc {
		e.el.Call("cancelVideoFrameCallback", e.frameReq)
		e.frameF.Release()
	}
	e.el.Call("pause")
	e.el.Call("removeAttribute", "src")
	e.el.Call("load")
	if e.blob != "" {
		js.Global().Get("URL").Call("revokeObjectURL", e.blob)
	}
	media.RemoveElement(e.native)
	return nil
}
