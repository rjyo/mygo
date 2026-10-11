package mygo

import (
	"crypto/rand"
	"errors"
	"image"
	"image/draw"
	"runtime"
	"slices"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/transfer"
)

// DragOptions configures a native data source. See package transfer for
// representations, lazy providers and copy/move operations.
type DragOptions = transfer.DragOptions
type DragResult = transfer.Result

var errDragBusy = errors.New("mygo: a native data drag is already active")

// dataSource is the core's one live native drag. Main thread only. Its
// random token can resolve a Go value only inside this process and only
// for this session; native data never contains a Go pointer.
type dataSource struct {
	w          *Window
	token      string
	data       transfer.Data
	local      any
	done       func(transfer.Result)
	finished   bool
	operations transfer.Operation
}

var activeDataDrag *dataSource

// StartDataDrag starts a native drag from this window's current pointer
// gesture. The window must show native UI. It returns after scheduling the
// drag; options.Done receives completion on the main thread. Use an
// element's DragData in package ui to start after the movement threshold.
// It is safe from any goroutine. An immediate validation error starts no
// session and does not call Done.
func (w *Window) StartDataDrag(data transfer.Data, options DragOptions) error {
	pix, width, height := dragImage(options.Preview)
	return onMainValue(func() error { return w.startDataDragPrepared(data, nil, options, -1, -1, pix, width, height) })
}

// CancelDataDrag cancels a native data drag sourced by this window. It is
// safe from any goroutine and does nothing when the window has no drag.
func (w *Window) CancelDataDrag() { onMain(func() { w.cancelDataDrag() }) }

func (w *Window) startDataDrag(data transfer.Data, local any, options DragOptions, x, y float64) error {
	pix, width, height := dragImage(options.Preview)
	return w.startDataDragPrepared(data, local, options, x, y, pix, width, height)
}

func (w *Window) startDataDragPrepared(data transfer.Data, local any, options DragOptions, x, y float64, pix []byte, width, height int) error {
	if w.native == nil {
		return errDestroyed
	}
	if w.conn == nil {
		return errors.New("mygo: a data drag requires native UI")
	}
	if activeDataDrag != nil {
		return errDragBusy
	}
	formats := data.Formats()
	if len(formats) == 0 || slices.Contains(formats, platform.DragSessionFormat) {
		return errors.New("mygo: a data drag needs non-reserved representations")
	}
	operations := options.Operations.Allowed()
	if operations == transfer.None {
		return errors.New("mygo: no supported drag operation")
	}
	src := &dataSource{w: w, token: rand.Text(), data: data.Snapshot(), local: local, done: options.Done, operations: operations}
	activeDataDrag = src
	hot := options.Hotspot
	scale := float64(options.Scale)
	if scale <= 1 || pix == nil {
		scale = 1
	} else if runtime.GOOS != "darwin" {
		pix, width, height, hot = downscalePreview(pix, width, height, hot, scale)
		scale = 1
	}
	req := platform.DragRequest{Data: src.data, Session: src.token, Operations: operations,
		X: x, Y: y, Preview: pix, Width: width, Height: height, Scale: scale,
		HotX: hot.X, HotY: hot.Y, Done: src.finish}
	// OLE tracks in a nested event loop. Enter it after the input callback
	// returns, so UI input and frames are never suspended halfway through.
	postMain(func() {
		if src.finished {
			return
		}
		if w.native == nil {
			src.finish(transfer.Result{Canceled: true})
			return
		}
		w.conn.Surface.StartDataDrag(req)
	})
	return nil
}

func dragImage(img image.Image) ([]byte, int, int) {
	if img == nil {
		return nil, 0, 0
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		panic("mygo: drag preview size must be 1..4096 pixels")
	}
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	for i := 0; i < len(rgba.Pix); i += 4 {
		rgba.Pix[i], rgba.Pix[i+2] = rgba.Pix[i+2], rgba.Pix[i]
	}
	return rgba.Pix, w, h
}

// downscalePreview averages a preview of scale pixels per DIP down to one,
// for backends that place a preview one pixel per DIP.
func downscalePreview(pix []byte, width, height int, hot image.Point, scale float64) ([]byte, int, int, image.Point) {
	w, h := max(int(float64(width)/scale+0.5), 1), max(int(float64(height)/scale+0.5), 1)
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		y0, y1 := y*height/h, max((y+1)*height/h, y*height/h+1)
		for x := 0; x < w; x++ {
			x0, x1 := x*width/w, max((x+1)*width/w, x*width/w+1)
			var sum [4]int
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					i := (sy*width + sx) * 4
					for c := range 4 {
						sum[c] += int(pix[i+c])
					}
				}
			}
			n := (y1 - y0) * (x1 - x0)
			for c := range 4 {
				out[(y*w+x)*4+c] = byte(sum[c] / n)
			}
		}
	}
	return out, w, h, image.Pt(int(float64(hot.X)/scale), int(float64(hot.Y)/scale))
}

func (s *dataSource) finish(result transfer.Result) {
	if s.finished {
		return
	}
	s.finished = true
	if op := result.Operation; op != transfer.None && ((op != transfer.Copy && op != transfer.Move) || op&s.operations == 0) {
		result = transfer.Result{Err: errors.New("mygo: destination returned a disallowed drag operation")}
	}
	if result.Err != nil || result.Canceled {
		result.Operation = transfer.None
	}
	if result.Operation == transfer.None && result.Err == nil {
		result.Canceled = true
	}
	if activeDataDrag == s {
		activeDataDrag = nil
	}
	s.data, s.local = transfer.Data{}, nil
	if done := s.done; done != nil {
		s.done = nil
		done(result)
	}
}

func (w *Window) cancelDataDrag() {
	if s := activeDataDrag; s != nil && s.w == w {
		// Invalidate the registry before callbacks or a native cancel can
		// reenter. The backend may report completion later; finish is once.
		activeDataDrag = nil
		if w.conn != nil {
			w.conn.Surface.CancelDataDrag()
		}
		s.finish(transfer.Result{Canceled: true})
	}
}

func resolveDataDrag(d *platform.DataDragEvent) {
	if d == nil {
		return
	}
	d.Local = nil
	if d.Resolved {
		d.Data, d.Resolved = transfer.Data{}, false
	}
	if s := activeDataDrag; s != nil && d.Session != "" && s.token == d.Session {
		d.Local, d.Data = s.local, s.data
		d.Resolved = true
		d.Offer.Formats = s.data.Formats()
	}
}
