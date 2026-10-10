package mygo

import (
	"errors"

	"github.com/egoist/mygo/internal/platform"
)

// WebView is a system web view (WKWebView, WebView2, WebKitGTK) embedded
// in a window of native UI: the app places it over a rectangle of the
// window each frame, beside what MyGo draws, and hides it to keep its page
// loaded out of sight. An offscreen WebView (NewOffscreenWebView) is in no
// window and only renders pages into images.
//
// Unlike a window's page, an embedded page gets no bridge and can't call
// bound services: it talks to Go through window.mygoWebView.postMessage
// (a string), which every frame has, and Go answers with Eval or Call.
//
// The web view is a native view above the window's surface, so nothing
// MyGo draws covers it: hide it while a menu or dialog drawn in the window
// overlaps it. Methods are safe from any goroutine; callbacks run on the
// main thread.
//
// Platform differences: on Windows, Transparent clears the page's
// background but the window doesn't show through, and corners are clipped
// without antialiasing; on Linux, corners aren't rounded and LoadFile
// can't limit what a page reads to readAccess.
type WebView struct {
	native platform.WebView
	h      *webViewHandler
}

// WebViewOptions configure a WebView.
type WebViewOptions struct {
	// Scripts run in every page, in order.
	Scripts []WebViewScript
	// DevTools allows the web inspector (right click, Inspect Element).
	DevTools bool
	// Transparent shows the window through until the page paints a
	// background (macOS, Linux). Windows shows BackgroundColor there
	// instead, or black without one.
	Transparent bool
	// BackgroundColor (a CSS color: #rgb, #rrggbb, rgb(…)) is shown where
	// the page paints nothing: under a transparent page on Windows, and
	// under any page elsewhere unless Transparent. It defaults to white.
	BackgroundColor string
	// CornerRadius rounds the corners in Corners, or all of them.
	CornerRadius float64
	Corners      Corners
	// Width and Height size an offscreen web view, in DIPs.
	Width, Height float64

	// OnMessage receives what a page posts with
	// window.mygoWebView.postMessage, and which frame posted it. Only
	// messages from FrameMain come from the top document for sure:
	// anything inside an iframe can claim anything.
	OnMessage func(msg string, frame WebViewFrame)
	// OnNavigate decides whether a navigation goes ahead; nil allows
	// every one.
	OnNavigate func(n WebViewNavigation) bool
	// OnNewWindow is a page asking for a new window (window.open,
	// target=_blank). No window opens; the app may open url elsewhere.
	OnNewWindow func(url string)
	// OnLoad: the top document finished loading.
	OnLoad func()
	// OnCrash: the page's process ended, leaving the web view blank until
	// it loads again.
	OnCrash func()
}

// WebViewScript is a script run in every page of a WebView.
type WebViewScript struct {
	Source string
	// AtDocumentEnd runs it once the document is parsed instead of before
	// any of the page's scripts.
	AtDocumentEnd bool
	// MainFrameOnly keeps it out of iframes.
	MainFrameOnly bool
}

// Corners is a set of a rectangle's corners; 0 is all of them.
type Corners uint8

const (
	CornerTopLeft     = Corners(platform.CornerTopLeft)
	CornerTopRight    = Corners(platform.CornerTopRight)
	CornerBottomLeft  = Corners(platform.CornerBottomLeft)
	CornerBottomRight = Corners(platform.CornerBottomRight)
)

// WebViewFrame tells which frame of a page something came from.
type WebViewFrame uint8

const (
	// FrameUnknown: the platform can't tell. On Linux, WebKitGTK doesn't
	// say which frame a navigation is for: iframes' navigations,
	// redirects and scripted navigations of a document of no origin are
	// unknown. A navigation is never reported as FrameMain wrongly.
	FrameUnknown = WebViewFrame(platform.FrameUnknown)
	// FrameMain is the top document.
	FrameMain = WebViewFrame(platform.FrameMain)
	// FrameChild is an iframe of the top document.
	FrameChild = WebViewFrame(platform.FrameChild)
	// FrameNested is an iframe inside an iframe.
	FrameNested = WebViewFrame(platform.FrameNested)
)

func (f WebViewFrame) String() string {
	switch f {
	case FrameMain:
		return "main"
	case FrameChild:
		return "child"
	case FrameNested:
		return "nested"
	}
	return "unknown"
}

// WebViewNavigation is a navigation a page asks for.
type WebViewNavigation struct {
	URL   string
	Frame WebViewFrame
	// UserInitiated: a link click or a form submission, not a script or a
	// redirect. On Linux, any navigation during a user gesture counts.
	UserInitiated bool
}

var errNoContent = errors.New("mygo: embedded web views need a window of native UI (WindowOptions.Content)")

// NewWebView embeds a web view in w, which must show native UI. It is
// hidden until placed with Place.
func (w *Window) NewWebView(opts WebViewOptions) (*WebView, error) {
	return onMainValue(func() webViewResult {
		if w.native == nil {
			return webViewResult{err: errDestroyed}
		}
		if w.content == nil {
			return webViewResult{err: errNoContent}
		}
		return newWebView(w.native, opts)
	}).get()
}

// NewOffscreenWebView makes a web view in no window, opts.Width by
// opts.Height, to render pages with Snapshot and Call.
func NewOffscreenWebView(opts WebViewOptions) (*WebView, error) {
	return onMainValue(func() webViewResult { return newWebView(nil, opts) }).get()
}

type webViewResult struct {
	v   *WebView
	err error
}

func (r webViewResult) get() (*WebView, error) { return r.v, r.err }

func newWebView(parent platform.Window, opts WebViewOptions) webViewResult {
	bg, err := webViewColor(opts.BackgroundColor)
	if err != nil {
		return webViewResult{err: err}
	}
	po := &platform.WebViewOptions{
		Background:  bg,
		DevTools:    opts.DevTools,
		Transparent: opts.Transparent,
		Radius:      opts.CornerRadius,
		Corners:     platform.Corners(opts.Corners),
		Width:       opts.Width,
		Height:      opts.Height,
	}
	for _, s := range opts.Scripts {
		po.Scripts = append(po.Scripts, platform.UserScript{Source: s.Source, AtDocumentEnd: s.AtDocumentEnd, AllFrames: !s.MainFrameOnly})
	}
	h := &webViewHandler{opts: opts}
	n, err := backend().NewWebView(parent, po, h)
	if err != nil {
		return webViewResult{err: err}
	}
	return webViewResult{v: &WebView{native: n, h: h}}
}

// webViewColor parses a CSS color; "" is none.
func webViewColor(css string) (platform.Color, error) {
	if css == "" {
		return platform.Color{}, nil
	}
	return parseColor(css)
}

// do runs fn on the main thread unless the web view is closed.
func (v *WebView) do(fn func(n platform.WebView)) {
	onMain(func() {
		if !v.h.closed {
			fn(v.native)
		}
	})
}

// LoadURL navigates the top document to url.
func (v *WebView) LoadURL(url string) { v.do(func(n platform.WebView) { n.LoadURL(url) }) }

// LoadHTML shows html as a document of no origin (about:blank).
func (v *WebView) LoadHTML(html string) { v.do(func(n platform.WebView) { n.LoadHTML(html) }) }

// LoadFile shows a local file, letting the page read readAccess, a file
// or a directory ("" is the file alone).
func (v *WebView) LoadFile(path, readAccess string) {
	if readAccess == "" {
		readAccess = path
	}
	v.do(func(n platform.WebView) { n.LoadFile(path, readAccess) })
}

// Eval runs js in the top document.
func (v *WebView) Eval(js string) { v.do(func(n platform.WebView) { n.Eval(js) }) }

// Call runs body as the body of an async function in the top document and
// passes done what it returns, as JSON, on the main thread.
func (v *WebView) Call(body string, done func(result string, err error)) {
	ran := false
	v.do(func(n platform.WebView) {
		ran = true
		n.Call(body, done)
	})
	if !ran {
		postMain(func() { done("", errDestroyed) })
	}
}

// Place shows the web view over x, y, w, h of its window, in DIPs from the
// top left of the content (the coordinates of the window's layout).
func (v *WebView) Place(x, y, w, h float64) {
	v.do(func(n platform.WebView) { n.Place(platform.RectF{X: x, Y: y, W: w, H: h}) })
}

// Hide hides the web view, keeping its page loaded, and gives the keyboard
// back to the window if the page had it.
func (v *WebView) Hide() { v.do(func(n platform.WebView) { n.Hide() }) }

// Focus gives the page the keyboard, if the web view shows.
func (v *WebView) Focus() { v.do(func(n platform.WebView) { n.Focus() }) }

// Blur gives the keyboard back to the window if the page has it.
func (v *WebView) Blur() { v.do(func(n platform.WebView) { n.Blur() }) }

// Focused reports whether the page has the keyboard.
func (v *WebView) Focused() bool {
	return onMainValue(func() bool { return !v.h.closed && v.native.Focused() })
}

// SetBackgroundColor changes WebViewOptions.BackgroundColor (a theme
// switch).
func (v *WebView) SetBackgroundColor(css string) error {
	c, err := webViewColor(css)
	if err != nil {
		return err
	}
	v.do(func(n platform.WebView) { n.SetBackground(c) })
	return nil
}

// Snapshot renders x, y, w, h of the page, in its CSS pixels, to a PNG
// pixelWidth pixels wide, and passes it to done on the main thread.
func (v *WebView) Snapshot(x, y, w, h, pixelWidth float64, done func(png []byte, err error)) {
	ran := false
	v.do(func(n platform.WebView) {
		ran = true
		n.Snapshot(platform.RectF{X: x, Y: y, W: w, H: h}, pixelWidth, done)
	})
	if !ran {
		postMain(func() { done(nil, errDestroyed) })
	}
}

// Close destroys the web view. It does nothing once closed.
func (v *WebView) Close() {
	onMain(func() {
		if v.h.closed {
			return
		}
		v.h.closed = true
		v.native.Destroy()
	})
}

// webViewHandler passes the native web view's events to the options'
// callbacks.
type webViewHandler struct {
	opts   WebViewOptions
	closed bool
}

func (h *webViewHandler) WebViewMessage(msg string, frame platform.WebViewFrame) {
	if !h.closed && h.opts.OnMessage != nil {
		h.opts.OnMessage(msg, WebViewFrame(frame))
	}
}

func (h *webViewHandler) WebViewNavigate(n platform.WebViewNavigation) bool {
	if h.closed {
		return false
	}
	if h.opts.OnNavigate == nil {
		return true
	}
	return h.opts.OnNavigate(WebViewNavigation{URL: n.URL, Frame: WebViewFrame(n.Frame), UserInitiated: n.UserInitiated})
}

func (h *webViewHandler) WebViewNewWindow(url string) {
	if !h.closed && h.opts.OnNewWindow != nil {
		h.opts.OnNewWindow(url)
	}
}

func (h *webViewHandler) WebViewLoaded() {
	if !h.closed && h.opts.OnLoad != nil {
		h.opts.OnLoad()
	}
}

func (h *webViewHandler) WebViewCrashed() {
	if !h.closed && h.opts.OnCrash != nil {
		h.opts.OnCrash()
	}
}
