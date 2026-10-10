//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// Embedded web views (mygo.WebView): a WebView2 controller of their own in
// a child window of a window of native UI, above its surface, or in a
// popup window off every screen to render offscreen. Each has its own
// scripts and handlers, so messages never reach the window or each other.
//
// WebView2 creates controllers asynchronously, in the environment the
// windows of web pages share: until the controller exists, what the app
// asks for waits in order, while the host window already moves.

const webHostClass = "MyGoWebView"

var (
	procIsChild            = user32.NewProc("IsChild")
	procGetFocus           = user32.NewProc("GetFocus")
	procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	procSetWindowRgn       = user32.NewProc("SetWindowRgn")
	procCreateRoundRectRgn = gdi32.NewProc("CreateRoundRectRgn")
	procCreateRectRgn      = gdi32.NewProc("CreateRectRgn")
	procCombineRgn         = gdi32.NewProc("CombineRgn")
	webHostRegistered      bool
)

// embedShim gives every frame window.mygoWebView.postMessage. WebView2
// tells frames apart by the handler that receives the message: the top
// document's posts reach the CoreWebView2's, an iframe's its frame's.
const embedShim = `(function(){
if (window.mygoWebView) return;
Object.defineProperty(window, "mygoWebView", {value: Object.freeze({
	postMessage: function (m) { var w = window.chrome && window.chrome.webview; w && w.postMessage(String(m)); }
})});
})();`

// NavigateToString takes documents of up to 2 MB, of UTF-16: larger ones
// load through embedDocHost (see LoadHTML).
const (
	navigateToStringLimit = 1 << 20
	embedDocHost          = "mygo-webview.localhost"
)

type embedView struct {
	b    *Backend
	w    *window // nil when offscreen
	h    platform.WebViewHandler
	o    platform.WebViewOptions
	host uintptr

	controller uintptr // ICoreWebView2Controller
	webview    uintptr // ICoreWebView2
	ready      bool
	destroyed  bool
	pending    []pendingCall
	err        error // why there is no controller
	fails      int   // creations that failed

	px      rect // where it is placed, in the window's pixels
	shown   bool
	focused bool // between GotFocus and LostFocus

	// frames are the iframes followed since FrameCreated, by their
	// ICoreWebView2Frame2, and which kind each is. frameTracking is false
	// once an iframe could not be followed: navigations of frames then
	// come through FrameNavigationStarting too, and navs remembers what
	// was decided for each, which both events report.
	frames        map[uintptr]platform.WebViewFrame
	frameTracking bool
	navs          map[uint64]bool

	htmlNav  bool              // the next top navigation is LoadHTML's
	docs     map[string]string // scripts writing large LoadHTML documents, by URL
	nextDoc  int
	calls    map[int]func(string, error)
	nextCall int
}

var errWebViewClosed = errors.New("mygo: the web view was closed")

func (b *Backend) NewWebView(parent platform.Window, o *platform.WebViewOptions, h platform.WebViewHandler) (platform.WebView, error) {
	v := &embedView{
		b: b, h: h, o: *o,
		frames: map[uintptr]platform.WebViewFrame{}, navs: map[uint64]bool{},
		docs: map[string]string{}, calls: map[int]func(string, error){},
	}
	if parent != nil {
		w := parent.(*window)
		if w.surface == nil {
			return nil, errors.New("mygo: embedded web views need a window of native UI")
		}
		v.w = w
	}
	if err := b.startEnvironment(); err != nil {
		return nil, err
	}
	registerWebHostClass()
	if v.w != nil {
		// Hidden until placed, above the surface, its sibling.
		v.host = createWindow(0, webHostClass, "", wsChild|wsClipSiblings|wsClipChildren, 0, 0, 0, 0, v.w.hwnd)
	} else {
		// Shown, so that Chromium paints, but off every screen, out of
		// the taskbar and never activated.
		dpi := dpiOf(0)
		width, height := pxOf(max(o.Width, 1), dpi), pxOf(max(o.Height, 1), dpi)
		const smXVirtualScreen, smYVirtualScreen = 76, 77
		x, _, _ := procGetSystemMetrics.Call(smXVirtualScreen)
		y, _, _ := procGetSystemMetrics.Call(smYVirtualScreen)
		v.host = createWindow(wsExToolWindow|wsExNoActivate, webHostClass, "", wsPopup|wsClipChildren,
			int32(x)-width-100, int32(y)-height-100, width, height, 0)
		if v.host != 0 {
			procShowWindow.Call(v.host, swShowNoActivate)
		}
	}
	if v.host == 0 {
		return nil, errors.New("mygo: cannot create the web view's window")
	}
	b.webHosts[v.host] = v
	if v.w != nil {
		v.w.embeds = append(v.w.embeds, v)
	}
	b.whenEnvironment(v.create)
	return v, nil
}

func registerWebHostClass() {
	if webHostRegistered {
		return
	}
	webHostRegistered = true
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	wc := wndClassEx{
		WndProc:   wndProcCallback,
		Instance:  instance(),
		Cursor:    cursor,
		ClassName: u16(webHostClass),
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
}

// pxOf converts DIPs to pixels.
func pxOf(v float64, dpi int) int32 { return int32(math.Round(v * float64(dpi) / 96)) }

// message handles a message of the host window.
func (v *embedView) message(m uint32, wp uintptr) (uintptr, bool) {
	switch m {
	case wmTimer:
		if wp == timerWebView {
			procKillTimer.Call(v.host, timerWebView)
			v.create()
			return 0, true
		}
	case wmSetFocus:
		v.moveFocus()
		return 0, true
	case wmEraseBkgnd:
		return 1, true
	}
	return 0, false
}

func (v *embedView) create() {
	if v.destroyed {
		return
	}
	if v.b.envErr != nil {
		v.failed(v.b.envErr)
		return
	}
	hr := withHandler(func(hr, controller uintptr) {
		if v.destroyed {
			if controller != 0 {
				comCall(controller, ctlClose)
			}
			return
		}
		if failed(hr) || controller == 0 {
			v.creationFailed(hr)
			return
		}
		addRef(controller)
		v.setUp(controller)
	}, func(h uintptr) uintptr { return comCall(v.b.env, envCreateController, v.host, h) })
	if failed(hr) {
		v.creationFailed(hr)
	}
}

// creationFailed asks again later, as windows do (createWebView), or
// gives up.
func (v *embedView) creationFailed(hr uintptr) {
	err := hresultError("creating the WebView2 controller", hr)
	v.fails++
	if hr == errorInvalidState || v.fails >= webViewAttempts {
		v.failed(err)
		return
	}
	log.Printf("%v; trying again", err)
	procSetTimer.Call(v.host, timerWebView, uintptr(backoff(v.fails-1)/time.Millisecond), 0)
}

// failed records why the web view has no controller and fails what waits
// for it.
func (v *embedView) failed(err error) {
	log.Print(err)
	v.err = err
	v.failPending(err)
}

func (v *embedView) failPending(err error) {
	pending := v.pending
	v.pending = nil
	for _, c := range pending {
		if c.fail != nil {
			c.fail(err)
		}
	}
}

// applyBackground paints the controller before and under the page.
// WebView2 takes only opaque or fully transparent colors, and a
// transparent one shows black in a child window, not the surface: a
// transparent web view shows Background where it has one.
func (v *embedView) applyBackground() {
	ctl2 := queryInterface(v.controller, &iidICoreWebView2Controller2)
	if ctl2 == 0 {
		return
	}
	defer release(ctl2)
	bg := v.o.Background
	switch {
	case bg.A != 0:
		comCall(ctl2, ctl2PutDefaultBackgroundColor, 0xFF|uintptr(bg.R)<<8|uintptr(bg.G)<<16|uintptr(bg.B)<<24)
	case v.o.Transparent:
		comCall(ctl2, ctl2PutDefaultBackgroundColor, 0)
	}
}

func (v *embedView) SetBackground(c platform.Color) {
	v.o.Background = c
	v.do(v.applyBackground, nil)
}

// do runs fn once the controller exists, or fail, unless nil, with the
// reason it never will.
func (v *embedView) do(fn func(), fail func(error)) {
	var err error
	switch {
	case v.destroyed:
		err = errWebViewClosed
	case v.err != nil:
		err = v.err
	case v.ready:
		fn()
		return
	default:
		v.pending = append(v.pending, pendingCall{fn, fail})
		return
	}
	if fail != nil {
		fail(err)
	}
}

func (v *embedView) setUp(controller uintptr) {
	v.controller = controller
	comCall(controller, ctlGetCoreWebView2, uintptr(unsafe.Pointer(&v.webview)))
	var s uintptr
	comCall(v.webview, wvGetSettings, uintptr(unsafe.Pointer(&s)))
	comCall(s, setPutIsScriptEnabled, 1)
	comCall(s, setPutIsWebMessageEnabled, 1)
	comCall(s, setPutAreDefaultScriptDialogsEnabled, 1)
	comCall(s, setPutIsStatusBarEnabled, 0)
	comCall(s, setPutAreDevToolsEnabled, boolArg(v.o.DevTools))
	comCall(s, setPutAreDefaultContextMenusEnabled, 1)
	comCall(s, setPutIsZoomControlEnabled, 0)
	release(s)
	v.applyBackground()

	addUserScript(v.webview, platform.UserScript{Source: embedShim, AllFrames: true})
	for _, us := range v.o.Scripts {
		addUserScript(v.webview, us)
	}

	wv := v.webview
	v.subscribe(wv, wvAddNavigationStarting, func(_, args uintptr) { v.navigationStarting(args, platform.FrameMain) })
	v.subscribe(wv, wvAddFrameNavigationStarting, func(_, args uintptr) {
		if !v.frameTracking {
			v.navigationStarting(args, platform.FrameUnknown)
		}
	})
	v.subscribe(wv, wvAddNavigationCompleted, func(_, args uintptr) {
		var ok int32
		comCall(args, navCompletedGetIsSuccess, uintptr(unsafe.Pointer(&ok)))
		if ok != 0 {
			v.h.WebViewLoaded()
		}
	})
	v.subscribe(wv, wvAddWebMessageReceived, func(_, args uintptr) { v.postedMessage(args, platform.FrameMain) })
	v.subscribe(wv, wvAddNewWindowRequested, func(_, args uintptr) {
		comCall(args, newWinPutHandled, 1) // no window of WebView2's
		var p uintptr
		comCall(args, newWinGetURI, uintptr(unsafe.Pointer(&p)))
		v.h.WebViewNewWindow(takeWstr(p))
	})
	v.subscribe(wv, wvAddProcessFailed, func(_, args uintptr) {
		var kind int32
		comCall(args, procFailedGetKind, uintptr(unsafe.Pointer(&kind)))
		if kind == 0 || kind == 1 { // the browser's process, or the page's
			v.h.WebViewCrashed()
		}
	})
	v.subscribe(wv, wvAddWebResourceRequested, v.resourceRequested)
	comCall(wv, wvAddWebResourceRequestedFilter, uintptr(unsafe.Pointer(u16("http://"+embedDocHost+"/*"))), 0)
	if wv4 := queryInterface(wv, &iidICoreWebView2_4); wv4 != 0 {
		v.frameTracking = true
		v.subscribe(wv4, wv4AddFrameCreated, func(_, args uintptr) { v.frameCreated(args, platform.FrameChild) })
		release(wv4)
	}

	v.subscribe(controller, ctlAddGotFocus, func(_, _ uintptr) {
		v.focused = true
		if v.w != nil {
			v.w.embedFocus = v
		}
	})
	v.subscribe(controller, ctlAddLostFocus, func(_, _ uintptr) { v.focused = false })
	if v.w != nil {
		// The window's menu shortcuts work while the page has the
		// keyboard.
		v.subscribe(controller, ctlAddAcceleratorKeyPressed, v.w.acceleratorKeyPressed)
	}

	if v.w == nil {
		var r rect
		procGetClientRect.Call(v.host, uintptr(unsafe.Pointer(&r)))
		putBounds(controller, r)
		comCall(controller, ctlPutIsVisible, 1)
	} else {
		putBounds(controller, rect{0, 0, v.px.Right - v.px.Left, v.px.Bottom - v.px.Top})
		comCall(controller, ctlPutIsVisible, boolArg(v.shown))
	}

	v.ready = true
	pending := v.pending
	v.pending = nil
	for i, c := range pending {
		if v.destroyed {
			v.pending = pending[i:]
			v.failPending(errWebViewClosed)
			return
		}
		c.run()
	}
}

// subscribe adds a handler of the event of obj whose add method is index;
// it runs while the web view lives. Closing the controller removes it.
func (v *embedView) subscribe(obj uintptr, index int, fn func(sender, args uintptr)) {
	var token int64
	withHandler(func(sender, args uintptr) {
		if !v.destroyed {
			fn(sender, args)
		}
	}, func(h uintptr) uintptr { return comCall(obj, index, h, uintptr(unsafe.Pointer(&token))) })
}

// postedMessage passes on a message a page posted, if it is a string.
func (v *embedView) postedMessage(args uintptr, frame platform.WebViewFrame) {
	var p uintptr
	if failed(comCall(args, msgTryGetWebMessageAsString, uintptr(unsafe.Pointer(&p)))) {
		return
	}
	v.h.WebViewMessage(takeWstr(p), frame)
}

// frameCreated follows a new iframe, of kind, through its
// ICoreWebView2Frame2, and its own iframes through ICoreWebView2Frame7
// (newer runtimes), as nested frames.
func (v *embedView) frameCreated(args uintptr, kind platform.WebViewFrame) {
	var frame uintptr
	if failed(comCall(args, frameCreatedGetFrame, uintptr(unsafe.Pointer(&frame)))) || frame == 0 {
		return
	}
	defer release(frame)
	f2 := queryInterface(frame, &iidICoreWebView2Frame2)
	if f2 == 0 {
		v.frameTracking = false
		return
	}
	v.frames[f2] = kind
	v.subscribe(f2, frame2AddWebMessageReceived, func(_, args uintptr) { v.postedMessage(args, kind) })
	v.subscribe(f2, frame2AddNavigationStarting, func(_, args uintptr) { v.navigationStarting(args, kind) })
	v.subscribe(f2, frameAddDestroyed, func(_, _ uintptr) {
		if _, ok := v.frames[f2]; ok {
			delete(v.frames, f2)
			release(f2)
		}
	})
	if f7 := queryInterface(frame, &iidICoreWebView2Frame7); f7 != 0 {
		v.subscribe(f7, frame7AddFrameCreated, func(_, args uintptr) { v.frameCreated(args, platform.FrameNested) })
		release(f7)
	} else {
		v.frameTracking = false
	}
}

// navigationStarting asks the handler whether a navigation of frame goes
// ahead.
func (v *embedView) navigationStarting(args uintptr, frame platform.WebViewFrame) {
	var id uint64
	comCall(args, navStartingGetNavigationID, uintptr(unsafe.Pointer(&id)))
	if frame == platform.FrameMain {
		clear(v.navs)
	} else if !v.frameTracking {
		// Reported by the frame and by the CoreWebView2: decide once.
		if allow, ok := v.navs[id]; ok {
			if !allow {
				comCall(args, navStartingPutCancel, 1)
			}
			return
		}
	}
	var p uintptr
	comCall(args, navStartingGetURI, uintptr(unsafe.Pointer(&p)))
	uri := takeWstr(p)
	if frame == platform.FrameMain && v.htmlNav {
		v.htmlNav = false
		uri = "about:blank"
	}
	var user int32
	comCall(args, navStartingGetIsUserInitiated, uintptr(unsafe.Pointer(&user)))
	allow := v.h.WebViewNavigate(platform.WebViewNavigation{URL: uri, Frame: frame, UserInitiated: user != 0})
	if frame != platform.FrameMain && !v.frameTracking {
		v.navs[id] = allow
	}
	if !allow {
		comCall(args, navStartingPutCancel, 1)
	}
}

// resourceRequested serves the scripts that write large LoadHTML
// documents.
func (v *embedView) resourceRequested(_, args uintptr) {
	var req uintptr
	if comCall(args, resReqGetRequest, uintptr(unsafe.Pointer(&req))) != sOK || req == 0 {
		return
	}
	var p uintptr
	comCall(req, reqGetURI, uintptr(unsafe.Pointer(&p)))
	release(req)
	uri := takeWstr(p)
	js, ok := v.docs[uri]
	if !ok {
		v.b.respond(args, 0, http.StatusNotFound, nil, nil)
		return
	}
	delete(v.docs, uri)
	v.b.respond(args, 0, http.StatusOK, http.Header{"Content-Type": {"text/javascript; charset=utf-8"}}, []byte(js))
}

func (v *embedView) LoadURL(raw string) {
	v.do(func() { comCall(v.webview, wvNavigate, uintptr(unsafe.Pointer(u16(raw)))) }, nil)
}

// LoadHTML navigates to the document as a string, of no origin. A
// document too large for NavigateToString loads as a small one, also of
// no origin, whose parser-blocking script writes it in with
// document.write: it is parsed in that document, after the web view's
// scripts ran there, as if it had come from the network. Its own doctype
// is ignored there: the document is in standards mode.
func (v *embedView) LoadHTML(html string) {
	v.do(func() {
		v.htmlNav = true
		if len(html) < navigateToStringLimit {
			comCall(v.webview, wvNavigateToString, uintptr(unsafe.Pointer(u16(html))))
			return
		}
		v.nextDoc++
		src := fmt.Sprintf("http://%s/%d.js", embedDocHost, v.nextDoc)
		doc, _ := json.Marshal(html) // escapes <, > and &
		clear(v.docs)                // those of earlier loads, never asked for
		v.docs[src] = "document.write(" + string(doc) + ");"
		comCall(v.webview, wvNavigateToString, uintptr(unsafe.Pointer(u16(`<!doctype html><script src="`+src+`"></script>`))))
	}, nil)
}

// LoadFile navigates to the file's URL. Chromium lets file pages load
// other files as subresources whatever readAccess says.
func (v *embedView) LoadFile(path, _ string) {
	u := url.URL{Scheme: "file", Path: "/" + strings.ReplaceAll(path, `\`, "/")}
	v.LoadURL(u.String())
}

func (v *embedView) Eval(js string) {
	v.do(func() {
		withHandler(func(uintptr, uintptr) {}, func(h uintptr) uintptr {
			return comCall(v.webview, wvExecuteScript, uintptr(unsafe.Pointer(u16(js))), h)
		})
	}, nil)
}

// Call returns the result as JSON, as on every platform.
func (v *embedView) Call(body string, done func(string, error)) {
	v.do(func() {
		callAsync(v.devtools, "return JSON.stringify(await (async () => {"+body+"\n})())", done)
	}, func(err error) { done("", err) })
}

// devtools calls a DevTools protocol method.
func (v *embedView) devtools(method, params string, done func(string, error)) {
	v.nextCall++
	id := v.nextCall
	v.calls[id] = done
	hr := withHandler(func(hr, result uintptr) {
		cb := v.calls[id]
		delete(v.calls, id)
		if cb == nil {
			return // destroyed
		}
		if failed(hr) {
			cb("", hresultError(method, hr))
			return
		}
		cb(wstr(result), nil)
	}, func(h uintptr) uintptr {
		return comCall(v.webview, wvCallDevToolsProtocolMethod, uintptr(unsafe.Pointer(u16(method))), uintptr(unsafe.Pointer(u16(params))), h)
	})
	if failed(hr) {
		if cb := v.calls[id]; cb != nil {
			delete(v.calls, id)
			cb("", hresultError(method, hr))
		}
	}
}

func (v *embedView) Place(r platform.RectF) {
	if v.w == nil || v.destroyed {
		return
	}
	dpi := dpiOf(v.w.hwnd)
	px := rect{pxOf(r.X, dpi), pxOf(r.Y, dpi), pxOf(r.X+r.W, dpi), pxOf(r.Y+r.H, dpi)}
	if v.shown && px == v.px {
		return
	}
	width, height := px.Right-px.Left, px.Bottom-px.Top
	resized := width != v.px.Right-v.px.Left || height != v.px.Bottom-v.px.Top
	flags := uintptr(swpNoActivate | swpShowWindow)
	if v.shown {
		flags |= swpNoZOrder
	}
	// HWND_TOP (0): above the surface.
	procSetWindowPos.Call(v.host, 0, uintptr(px.Left), uintptr(px.Top), uintptr(width), uintptr(height), flags)
	v.px = px
	if resized || !v.shown {
		v.clip(dpi)
	}
	wasShown := v.shown
	v.shown = true
	if v.ready {
		putBounds(v.controller, rect{0, 0, width, height})
		if !wasShown {
			comCall(v.controller, ctlPutIsVisible, 1)
		}
	}
	if !wasShown && v.w.caption != nil {
		v.w.caption.layout() // its controls stay above
	}
}

// clip rounds the host window's corners with a window region, which clips
// WebView2's windows inside it too.
func (v *embedView) clip(dpi int) {
	if v.o.Radius <= 0 {
		return
	}
	width, height := v.px.Right-v.px.Left, v.px.Bottom-v.px.Top
	d := 2 * pxOf(v.o.Radius, dpi)
	rgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(width+1), uintptr(height+1), uintptr(d), uintptr(d))
	if rgn == 0 {
		return
	}
	corners := v.o.Corners
	if corners == 0 {
		corners = platform.CornerTopLeft | platform.CornerTopRight | platform.CornerBottomLeft | platform.CornerBottomRight
	}
	r := d / 2
	square := func(c platform.Corners, left, top int32) {
		if corners&c != 0 {
			return
		}
		sq, _, _ := procCreateRectRgn.Call(uintptr(left), uintptr(top), uintptr(left+r), uintptr(top+r))
		const rgnOr = 2
		procCombineRgn.Call(rgn, rgn, sq, rgnOr)
		procDeleteObject.Call(sq)
	}
	square(platform.CornerTopLeft, 0, 0)
	square(platform.CornerTopRight, width-r, 0)
	square(platform.CornerBottomLeft, 0, height-r)
	square(platform.CornerBottomRight, width-r, height-r)
	procSetWindowRgn.Call(v.host, rgn, 1) // the window owns the region
}

func (v *embedView) Hide() {
	if v.w == nil || !v.shown || v.destroyed {
		return
	}
	v.Blur()
	procShowWindow.Call(v.host, swHide)
	if v.ready {
		comCall(v.controller, ctlPutIsVisible, 0)
	}
	v.shown = false
}

func (v *embedView) Focus() {
	if v.w == nil || !v.shown || v.destroyed {
		return
	}
	// The window gives the keyboard back here when it is activated.
	v.w.embedFocus = v
	v.do(v.moveFocus, nil)
}

func (v *embedView) moveFocus() {
	if v.ready {
		comCall(v.controller, ctlMoveFocus, 0) // COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC
	}
}

func (v *embedView) Blur() {
	if v.w == nil {
		return
	}
	if v.w.embedFocus == v {
		v.w.embedFocus = nil
	}
	if v.Focused() {
		v.focused = false
		procSetFocus.Call(v.w.surface.hwnd)
	}
}

func (v *embedView) Focused() bool {
	if v.w == nil || !v.shown || v.destroyed {
		return false
	}
	if v.focused {
		return true
	}
	f, _, _ := procGetFocus.Call()
	if f == 0 {
		return false
	}
	in, _, _ := procIsChild.Call(v.host, f)
	return f == v.host || in != 0
}

// Snapshot captures r through the DevTools protocol, which renders it
// whether or not it shows.
func (v *embedView) Snapshot(r platform.RectF, pixelWidth float64, done func([]byte, error)) {
	if r.W <= 0 || r.H <= 0 {
		done(nil, errors.New("mygo: snapshot of an empty rectangle"))
		return
	}
	// The image has the clip's size times its scale, times the device
	// pixel ratio.
	dpr := float64(dpiOf(v.host)) / 96
	params, _ := json.Marshal(map[string]any{
		"format":                "png",
		"captureBeyondViewport": true,
		"clip":                  map[string]float64{"x": r.X, "y": r.Y, "width": r.W, "height": r.H, "scale": pixelWidth / r.W / dpr},
	})
	v.do(func() {
		v.devtools("Page.captureScreenshot", string(params), func(res string, err error) {
			if err != nil {
				done(nil, err)
				return
			}
			var out struct {
				Data string `json:"data"`
			}
			if err := json.Unmarshal([]byte(res), &out); err != nil {
				done(nil, err)
				return
			}
			png, err := base64.StdEncoding.DecodeString(out.Data)
			done(png, err)
		})
	}, func(err error) { done(nil, err) })
}

func (v *embedView) Destroy() {
	if v.destroyed {
		return
	}
	v.Blur()
	v.destroyed = true
	for id, cb := range v.calls {
		delete(v.calls, id)
		cb("", errWebViewClosed)
	}
	v.failPending(errWebViewClosed)
	for f := range v.frames {
		release(f)
	}
	clear(v.frames)
	if v.controller != 0 {
		comCall(v.controller, ctlClose)
		release(v.webview)
		release(v.controller)
		v.controller, v.webview = 0, 0
	}
	delete(v.b.webHosts, v.host)
	procDestroyWindow.Call(v.host)
	if w := v.w; w != nil {
		if w.embedFocus == v {
			w.embedFocus = nil
		}
		for i, e := range w.embeds {
			if e == v {
				w.embeds = append(w.embeds[:i], w.embeds[i+1:]...)
				break
			}
		}
	}
}
