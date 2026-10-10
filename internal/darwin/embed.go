//go:build darwin

package darwin

import (
	"errors"

	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo/internal/platform"
)

// Embedded web views (mygo.WebView): a WKWebView of their own in a subview
// of a surface's view, above the area MyGo draws, or in no window at all to
// render offscreen. Each has its own content controller and delegate, so
// messages and scripts never reach the window or each other; events come
// back through MyGoWebViewDelegate, which finds the web view by its
// delegate.

// embedShim gives every frame window.mygoWebView.postMessage. Frames say
// whether they are the top document's children or deeper through the
// handler they post to; WebKit tells the top document itself apart.
const embedShim = `(function(){
if (window.mygoWebView) return;
var h = window.webkit && window.webkit.messageHandlers;
var name = window.parent === window.top ? "mygoWebView" : "mygoWebViewNested";
Object.defineProperty(window, "mygoWebView", {value: Object.freeze({
	postMessage: function (m) { h && h[name] && h[name].postMessage(String(m)); }
})});
})();`

type webView struct {
	b        *Backend
	s        *surface // nil when offscreen
	view     id       // WKWebView
	ucc      id       // WKUserContentController
	delegate id       // MyGoWebViewDelegate
	h        platform.WebViewHandler
	rect     platform.RectF
	// transparent: the surface shows through, Background or not.
	transparent bool
	shown       bool
	// mainFrame is the frameID of the top document's frame, for telling
	// its children from deeper frames in navigations.
	mainFrame uint64
}

func (b *Backend) NewWebView(parent platform.Window, o *platform.WebViewOptions, h platform.WebViewHandler) (platform.WebView, error) {
	v := &webView{b: b, h: h}
	if parent != nil {
		w := parent.(*window)
		if w.surface == nil {
			return nil, errors.New("mygo: embedded web views need a window of native UI")
		}
		v.s = w.surface
	}
	if b.byWebViewDelegate == nil {
		b.byWebViewDelegate = map[id]*webView{}
	}
	withPool(func() {
		cfg := alloc("WKWebViewConfiguration")
		defer release(cfg)
		prefs := send(cfg, "preferences")
		send(prefs, "setValue:forKey:", uintptr(nsBool(o.DevTools)), uintptr(nsString("developerExtrasEnabled")))
		if respondsTo(prefs, "setElementFullscreenEnabled:") {
			send(prefs, "setElementFullscreenEnabled:", 1)
		}
		v.ucc = alloc("WKUserContentController")
		v.delegate = alloc("MyGoWebViewDelegate")
		for _, name := range []string{"mygoWebView", "mygoWebViewNested"} {
			send(v.ucc, "addScriptMessageHandler:name:", uintptr(v.delegate), uintptr(nsString(name)))
		}
		scripts := append([]platform.UserScript{{Source: embedShim, AllFrames: true}}, o.Scripts...)
		for _, sc := range scripts {
			injection := uintptr(0) // WKUserScriptInjectionTimeAtDocumentStart
			if sc.AtDocumentEnd {
				injection = 1
			}
			script := send(send(class("WKUserScript"), "alloc"), "initWithSource:injectionTime:forMainFrameOnly:",
				uintptr(nsString(sc.Source)), injection, boolArg(!sc.AllFrames))
			send(v.ucc, "addUserScript:", uintptr(script))
			release(script)
		}
		send(cfg, "setUserContentController:", uintptr(v.ucc))
		frame := NSRect{}
		if v.s == nil {
			frame.Size = NSSize{Width: max(o.Width, 1), Height: max(o.Height, 1)}
		}
		v.view = msgInitRectID(send(class("WKWebView"), "alloc"), sel("initWithFrame:configuration:"), frame, cfg)
	})
	if v.view == 0 {
		v.releaseObjects()
		return nil, errors.New("mygo: cannot create a WKWebView")
	}
	send(v.view, "setNavigationDelegate:", uintptr(v.delegate))
	send(v.view, "setUIDelegate:", uintptr(v.delegate))
	if o.DevTools && respondsTo(v.view, "setInspectable:") {
		send(v.view, "setInspectable:", 1)
	}
	send(v.view, "setWantsLayer:", 1)
	v.transparent = o.Transparent
	if o.Transparent {
		withPool(func() {
			send(v.view, "setValue:forKey:", uintptr(nsBool(false)), uintptr(nsString("drawsBackground")))
		})
	} else {
		v.SetBackground(o.Background)
	}
	if layer := send(v.view, "layer"); layer != 0 && o.Radius > 0 {
		msgSetFloat(layer, sel("setCornerRadius:"), o.Radius)
		send(layer, "setMasksToBounds:", 1)
		send(layer, "setMaskedCorners:", maskedCorners(o.Corners, sendBool(v.view, "isFlipped")))
	}
	if v.s == nil {
		// Off any window, WebKit thinks the view occluded and stops
		// painting it; snapshots would wait forever.
		if respondsTo(v.view, "_setWindowOcclusionDetectionEnabled:") {
			send(v.view, "_setWindowOcclusionDetectionEnabled:", 0)
		}
	} else {
		send(v.view, "setHidden:", 1)
		send(v.view, "setAutoresizingMask:", 0)
		// The surface's view is flipped: frames are in top-left
		// coordinates, as the surface's layout.
		send(v.s.view, "addSubview:", uintptr(v.view))
	}
	b.byWebViewDelegate[v.delegate] = v
	return v, nil
}

// maskedCorners maps corners to CACornerMask, whose MinY corners are at
// the top of a flipped view and at the bottom of others.
func maskedCorners(c platform.Corners, flipped bool) uintptr {
	if c == 0 {
		return 15
	}
	const minXMinY, maxXMinY, minXMaxY, maxXMaxY = 1, 2, 4, 8
	top, bottom := [2]uintptr{minXMinY, maxXMinY}, [2]uintptr{minXMaxY, maxXMaxY}
	if !flipped {
		top, bottom = bottom, top
	}
	var m uintptr
	if c&platform.CornerTopLeft != 0 {
		m |= top[0]
	}
	if c&platform.CornerTopRight != 0 {
		m |= top[1]
	}
	if c&platform.CornerBottomLeft != 0 {
		m |= bottom[0]
	}
	if c&platform.CornerBottomRight != 0 {
		m |= bottom[1]
	}
	return m
}

func (v *webView) LoadURL(url string) {
	withPool(func() {
		send(v.view, "loadRequest:", uintptr(send(class("NSURLRequest"), "requestWithURL:", uintptr(nsURL(url)))))
	})
}

func (v *webView) LoadHTML(html string) {
	withPool(func() { send(v.view, "loadHTMLString:baseURL:", uintptr(nsString(html)), 0) })
}

func (v *webView) LoadFile(path, readAccess string) {
	withPool(func() {
		file := send(class("NSURL"), "fileURLWithPath:", uintptr(nsString(path)))
		access := send(class("NSURL"), "fileURLWithPath:", uintptr(nsString(readAccess)))
		send(v.view, "loadFileURL:allowingReadAccessToURL:", uintptr(file), uintptr(access))
	})
}

func (v *webView) Eval(js string) {
	withPool(func() { send(v.view, "evaluateJavaScript:completionHandler:", uintptr(nsString(js)), 0) })
}

func (v *webView) Call(body string, done func(string, error)) {
	if !respondsTo(v.view, "callAsyncJavaScript:arguments:inFrame:inContentWorld:completionHandler:") {
		done("", errors.New("mygo: WebView.Call requires macOS 11 or later"))
		return
	}
	// The result comes back as JSON on every platform.
	js := "return JSON.stringify(await (async () => {" + body + "\n})())"
	withPool(func() {
		blk := newBlock(func(_ objc.Block, result id, err id) {
			if err != 0 {
				done("", nsError(err))
				return
			}
			done(goString(result), nil)
		})
		defer blk.Release()
		send(v.view, "callAsyncJavaScript:arguments:inFrame:inContentWorld:completionHandler:",
			uintptr(nsString(js)), 0, 0, uintptr(send(class("WKContentWorld"), "pageWorld")), uintptr(blk))
	})
}

func (v *webView) Place(r platform.RectF) {
	if v.s == nil || r == v.rect && v.shown {
		return
	}
	msgSetRect(v.view, sel("setFrame:"), NSRect{Origin: NSPoint{X: r.X, Y: r.Y}, Size: NSSize{Width: r.W, Height: r.H}})
	if !v.shown {
		send(v.view, "setHidden:", 0)
	}
	v.rect, v.shown = r, true
}

func (v *webView) Hide() {
	if v.s == nil || !v.shown {
		return
	}
	// A hidden first responder would swallow every key.
	v.Blur()
	send(v.view, "setHidden:", 1)
	v.shown = false
}

func (v *webView) Focus() {
	if v.s != nil && v.shown {
		send(v.s.w.win, "makeFirstResponder:", uintptr(v.view))
	}
}

func (v *webView) Blur() {
	if v.Focused() {
		send(v.s.w.win, "makeFirstResponder:", uintptr(v.s.view))
	}
}

func (v *webView) Focused() bool {
	if v.s == nil {
		return false
	}
	fr := send(v.s.w.win, "firstResponder")
	return fr != 0 && sendBool(fr, "isKindOfClass:", uintptr(class("NSView"))) &&
		sendBool(fr, "isDescendantOf:", uintptr(v.view))
}

// SetBackground paints c behind the page: WebKit draws no background of
// its own and the view's layer shows c.
func (v *webView) SetBackground(c platform.Color) {
	if v.transparent || c.A == 0 {
		return
	}
	withPool(func() {
		send(v.view, "setValue:forKey:", uintptr(nsBool(false)), uintptr(nsString("drawsBackground")))
		if layer := send(v.view, "layer"); layer != 0 {
			send(layer, "setBackgroundColor:", uintptr(send(nsColor(c), "CGColor")))
		}
		if respondsTo(v.view, "setUnderPageBackgroundColor:") {
			send(v.view, "setUnderPageBackgroundColor:", uintptr(nsColor(c)))
		}
	})
}

func (v *webView) Snapshot(r platform.RectF, pixelWidth float64, done func([]byte, error)) {
	if v.s == nil {
		// Grow an offscreen view to take in r: WebKit only paints what
		// the view shows.
		b := msgRect(v.view, sel("bounds"))
		if w, h := max(b.Size.Width, r.X+r.W), max(b.Size.Height, r.Y+r.H); w > b.Size.Width || h > b.Size.Height {
			msgSetRect(v.view, sel("setFrame:"), NSRect{Size: NSSize{Width: w, Height: h}})
		}
	}
	scale := 0.0
	if v.s != nil {
		scale = v.s.scale()
	} else if screen := send(class("NSScreen"), "mainScreen"); screen != 0 {
		scale = msgFloat(screen, sel("backingScaleFactor"))
	}
	if scale <= 0 {
		scale = 1
	}
	withPool(func() {
		cfg := send(alloc("WKSnapshotConfiguration"), "autorelease")
		msgSetRect(cfg, sel("setRect:"), NSRect{Origin: NSPoint{X: r.X, Y: r.Y}, Size: NSSize{Width: r.W, Height: r.H}})
		// snapshotWidth is in points: the image has it times the
		// backing scale in pixels.
		send(cfg, "setSnapshotWidth:", uintptr(msgFloatID(class("NSNumber"), sel("numberWithDouble:"), pixelWidth/scale)))
		if respondsTo(cfg, "setAfterScreenUpdates:") {
			send(cfg, "setAfterScreenUpdates:", 1)
		}
		blk := newBlock(func(_ objc.Block, img id, err id) {
			if err != 0 || img == 0 {
				done(nil, nsError(err))
				return
			}
			withPool(func() { done(pngFromImage(img), nil) })
		})
		defer blk.Release()
		send(v.view, "takeSnapshotWithConfiguration:completionHandler:", uintptr(cfg), uintptr(blk))
	})
}

func (v *webView) Destroy() {
	if v.view == 0 {
		return
	}
	v.Blur()
	delete(v.b.byWebViewDelegate, v.delegate)
	withPool(func() {
		// The content controller retains its handler: break the cycle.
		for _, name := range []string{"mygoWebView", "mygoWebViewNested"} {
			send(v.ucc, "removeScriptMessageHandlerForName:", uintptr(nsString(name)))
		}
	})
	send(v.view, "setNavigationDelegate:", 0)
	send(v.view, "setUIDelegate:", 0)
	send(v.view, "stopLoading")
	send(v.view, "removeFromSuperview")
	release(v.view)
	v.view = 0
	v.releaseObjects()
}

func (v *webView) releaseObjects() {
	if v.ucc != 0 {
		release(v.ucc)
		v.ucc = 0
	}
	if v.delegate != 0 {
		release(v.delegate)
		v.delegate = 0
	}
}

// frameID returns the frameID of a WKFrameInfo's frame through
// _handle (SPI), 0 where WebKit lacks it.
func frameID(info id) uint64 {
	if info == 0 || !respondsTo(info, "_handle") {
		return 0
	}
	h := send(info, "_handle")
	if h == 0 || !respondsTo(h, "frameID") {
		return 0
	}
	return uint64(send(h, "frameID"))
}

// navigationFrame tells which frame a navigation targets.
func (v *webView) navigationFrame(target id) platform.WebViewFrame {
	if sendBool(target, "isMainFrame") {
		if f := frameID(target); f != 0 {
			v.mainFrame = f
		}
		return platform.FrameMain
	}
	if v.mainFrame == 0 || !respondsTo(target, "_parentFrameHandle") {
		return platform.FrameUnknown
	}
	parent := send(target, "_parentFrameHandle")
	if parent == 0 || !respondsTo(parent, "frameID") {
		return platform.FrameUnknown
	}
	if uint64(send(parent, "frameID")) == v.mainFrame {
		return platform.FrameChild
	}
	return platform.FrameNested
}

func (b *Backend) webViewFor(delegate id) *webView { return b.byWebViewDelegate[delegate] }

func registerWebViewClasses() {
	b := func() *Backend { return theBackend }
	const policyCancel, policyAllow = 0, 1
	classDef("MyGoWebViewDelegate", "NSObject",
		[]string{"WKNavigationDelegate", "WKUIDelegate", "WKScriptMessageHandler"},
		[]objc.MethodDef{
			method("userContentController:didReceiveScriptMessage:", func(self id, _ objc.SEL, ucc, msg id) {
				v := b().webViewFor(self)
				if v == nil {
					return
				}
				body := send(msg, "body")
				if body == 0 || !sendBool(body, "isKindOfClass:", uintptr(class("NSString"))) {
					return
				}
				frame := platform.FrameChild
				if info := send(msg, "frameInfo"); info != 0 && sendBool(info, "isMainFrame") {
					frame = platform.FrameMain
				} else if goString(send(msg, "name")) == "mygoWebViewNested" {
					frame = platform.FrameNested
				}
				v.h.WebViewMessage(goString(body), frame)
			}),
			method("webView:decidePolicyForNavigationAction:decisionHandler:", func(self id, _ objc.SEL, web, action id, handler uintptr) {
				v := b().webViewFor(self)
				target := send(action, "targetFrame")
				if v == nil || target == 0 {
					// No target: a new window, which createWebView… gets.
					callBlock(handler, policyCancel)
					return
				}
				navType := sendInt(action, "navigationType")
				n := platform.WebViewNavigation{
					URL:           goString(send(send(send(action, "request"), "URL"), "absoluteString")),
					Frame:         v.navigationFrame(target),
					UserInitiated: navType == 0 || navType == 1, // link activated, form submitted
				}
				if v.h.WebViewNavigate(n) {
					callBlock(handler, policyAllow)
				} else {
					callBlock(handler, policyCancel)
				}
			}),
			method("webView:createWebViewWithConfiguration:forNavigationAction:windowFeatures:", func(self id, _ objc.SEL, web, cfg, action, features id) id {
				if v := b().webViewFor(self); v != nil {
					v.h.WebViewNewWindow(goString(send(send(send(action, "request"), "URL"), "absoluteString")))
				}
				return 0
			}),
			method("webView:didFinishNavigation:", func(self id, _ objc.SEL, web, nav id) {
				if v := b().webViewFor(self); v != nil {
					v.h.WebViewLoaded()
				}
			}),
			method("webViewWebContentProcessDidTerminate:", func(self id, _ objc.SEL, web id) {
				if v := b().webViewFor(self); v != nil {
					v.h.WebViewCrashed()
				}
			}),
		})
}
