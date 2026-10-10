//go:build linux && (amd64 || arm64)

package linux

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/egoist/mygo/internal/platform"
)

// Embedded web views (mygo.WebView): a WebKitWebView of their own over a
// window's surface, or in a GtkOffscreenWindow to render offscreen. The
// surface of every window of native UI is the main child of a GtkOverlay
// (window.overlay), made with the window: moving a realized GtkGLArea into
// one later would lose its GL context. The web views go into a GtkFixed
// over it, which lets input through to the surface where no web view is.
// Each web view has its own user content manager, so messages and scripts
// never reach the window or each other; signals find the web view by the
// id they carry.
//
// WebKitGTK 4.x tells neither script messages nor navigations which frame
// they come from. Frames below the top document tell their depth through
// the handler they post to, as on macOS, and the top document's
// postMessage prefixes a secret of the web view's that only a script of
// the top frame holds, in a closure no frame can read. A navigation is the
// top document's when it loads what the API asked for, or what the top
// document announced, with the secret, it was about to load (embedTopShim);
// WebKit asks about it after the announcement, which came first over the
// same connection. Other navigations are FrameUnknown: those of iframes,
// redirects, and those a script starts by setting location in a document
// of no origin, as LoadHTML's, where the Navigation API is silent.
//
// GTK 3 cannot clip a web view to rounded corners without drawing it
// itself: WebViewOptions.Radius is ignored.

var (
	embedOnce                       sync.Once
	gtkFixedNew                     func() ptr
	gtkFixedPut                     func(f, w ptr, x, y int32)
	gtkFixedMove                    func(f, w ptr, x, y int32)
	gtkOffscreenWindowNew           func() ptr
	gtkWindowGetFocus               func(w ptr) ptr
	gtkWidgetIsAncestor             func(w, ancestor ptr) bool
	gtkOverlaySetOverlayPassThrough func(o, w ptr, v bool)
	gtkOverlayReorderOverlay        func(o, w ptr, index int32)
	jscValueIsUndefined             func(v ptr) bool
	cairoImageSurfaceCreate         func(format, width, height int32) ptr
	cairoCreate                     func(s ptr) ptr
	cairoDestroy                    func(cr ptr)
	cairoScale                      func(cr ptr, x, y float64)
	cairoGetSource                  func(cr ptr) ptr
	cairoPatternSetFilter           func(p ptr, filter int32)
	cairoImageSurfaceGetWidth       func(s ptr) int32

	cbEmbedMessage, cbEmbedPolicy, cbEmbedCreate, cbEmbedLoadChanged, cbEmbedCrashed ptr
)

func loadEmbed() {
	embedOnce.Do(func() {
		loadSurface()
		t, c := libGTK, libCairo
		mustBind(t, &gtkFixedNew, "gtk_fixed_new")
		mustBind(t, &gtkFixedPut, "gtk_fixed_put")
		mustBind(t, &gtkFixedMove, "gtk_fixed_move")
		mustBind(t, &gtkOffscreenWindowNew, "gtk_offscreen_window_new")
		mustBind(t, &gtkWindowGetFocus, "gtk_window_get_focus")
		mustBind(t, &gtkWidgetIsAncestor, "gtk_widget_is_ancestor")
		// GTK 3.18.
		bind(t, &gtkOverlaySetOverlayPassThrough, "gtk_overlay_set_overlay_pass_through")
		bind(t, &gtkOverlayReorderOverlay, "gtk_overlay_reorder_overlay")
		mustBind(libJSC, &jscValueIsUndefined, "jsc_value_is_undefined")
		mustBind(c, &cairoImageSurfaceCreate, "cairo_image_surface_create")
		mustBind(c, &cairoCreate, "cairo_create")
		mustBind(c, &cairoDestroy, "cairo_destroy")
		mustBind(c, &cairoScale, "cairo_scale")
		mustBind(c, &cairoGetSource, "cairo_get_source")
		mustBind(c, &cairoPatternSetFilter, "cairo_pattern_set_filter")
		mustBind(c, &cairoImageSurfaceGetWidth, "cairo_image_surface_get_width")
	})
}

// embedShim gives every frame below the top document
// window.mygoWebView.postMessage, posting to the handler that tells its
// depth. It keeps what it calls from the page's later changes.
const embedShim = `(function(){
if (window.mygoWebView || window === window.top) return;
var name = window.parent === window.top ? "mygoWebView" : "mygoWebViewNested";
var h = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers[name];
var post = h && h.postMessage, apply = Reflect.apply, S = String;
Object.defineProperty(window, "mygoWebView", {value: Object.freeze({
	postMessage: function (m) { post && apply(post, h, [S(m)]); }
})});
})();`

// embedTopShim is the top document's postMessage, which prefixes the
// secret %q and ":". The navigations the document makes go to Go first,
// prefixed with the secret and "n", before WebKit asks whether they may go
// ahead: the Navigation API's navigate event tells them, or, in a document
// of no origin (as LoadHTML's), where it is silent, the links and forms
// that start them; scripts that set location there go unannounced.
const embedTopShim = `(function(){
if (window.mygoWebView) return;
var h = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.mygoWebView;
var post = h && h.postMessage, apply = Reflect.apply, S = String, key = %q;
function send(m) { post && apply(post, h, [key + m]); }
Object.defineProperty(window, "mygoWebView", {value: Object.freeze({
	postMessage: function (m) { send(":" + S(m)); }
})});
var nav = window.navigation;
if (nav && nav.addEventListener) apply(nav.addEventListener, nav, ["navigate", function (e) {
	if (!e.hashChange) send("n" + e.destination.url);
}]);
if (window.origin !== "null") return;
// The Navigation API is silent in a document of no origin: links and forms
// tell where they go.
var self = function (t) { return !t || t === "_self" || t === "_top"; };
addEventListener("click", function (e) {
	var a = e.target && e.target.closest && e.target.closest("a[href]");
	if (a && self(a.target) && !a.hasAttribute("download")) send("n" + a.href);
}, true);
addEventListener("submit", function (e) {
	var f = e.target, u = new URL(f.action);
	if (!self(f.target)) return;
	if (f.method === "get") u.search = new URLSearchParams(new FormData(f)).toString();
	send("n" + u.href);
}, true);
})();`

type webView struct {
	b      *Backend
	id     int
	w      *window // nil when offscreen
	web    ptr     // WebKitWebView, a reference of ours
	ucm    ptr     // WebKitUserContentManager
	offWin ptr     // GtkOffscreenWindow of an offscreen web view
	h      platform.WebViewHandler
	secret string
	// transparent keeps the web view's background clear (Transparent).
	transparent bool
	rect        platform.RectF
	shown       bool
	// width and height are an offscreen web view's size; snapshots run
	// one at a time, as they may resize and zoom it.
	width, height float64
	snapshots     []func()
	// mainNavs are the URLs the top document is about to navigate to:
	// those loaded through the API, and those the document announced.
	mainNavs []string
}

func (b *Backend) NewWebView(parent platform.Window, o *platform.WebViewOptions, h platform.WebViewHandler) (platform.WebView, error) {
	if err := webKit(); err != nil {
		return nil, err
	}
	loadEmbed()
	v := &webView{b: b, h: h}
	if parent != nil {
		w := parent.(*window)
		if w.surface == nil || w.overlay == 0 {
			return nil, errors.New("mygo: embedded web views need a window of native UI")
		}
		if w.closed {
			return nil, errors.New("mygo: the window is closed")
		}
		v.w = w
	}
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		return nil, err
	}
	v.secret = hex.EncodeToString(key[:])
	if b.webViews == nil {
		b.webViews = map[int]*webView{}
	}
	b.nextWebView++
	v.id = b.nextWebView
	data := ptr(v.id)

	v.ucm = webkitUserContentManagerNew()
	for i, name := range []string{"mygoWebView", "mygoWebViewNested"} {
		webkitUserContentManagerRegisterHandler(v.ucm, cs(name))
		connect(v.ucm, "script-message-received::"+name, cbEmbedMessage, data<<1|ptr(i))
	}
	scripts := append([]platform.UserScript{
		{Source: embedShim, AllFrames: true},
		{Source: fmt.Sprintf(embedTopShim, v.secret)},
	}, o.Scripts...)
	for _, s := range scripts {
		frames, when := int32(1), int32(0) // top frame, document start
		if s.AllFrames {
			frames = 0
		}
		if s.AtDocumentEnd {
			when = 1
		}
		script := webkitUserScriptNew(cs(s.Source), frames, when, 0, 0)
		webkitUserContentManagerAddScript(v.ucm, script)
		webkitUserScriptUnref(script)
	}
	v.web = gObjectRefSink(webkitWebViewNewWithUserContentManager(v.ucm))
	webkitSettingsSetEnableDeveloperExtras(webkitWebViewGetSettings(v.web), o.DevTools)
	v.transparent = o.Transparent
	v.SetBackground(o.Background)
	connect(v.web, "decide-policy", cbEmbedPolicy, data)
	connect(v.web, "load-changed", cbEmbedLoadChanged, data)
	connect(v.web, "web-process-terminated", cbEmbedCrashed, data)
	// Links to new windows come to decide-policy, window.open to "create".
	connect(v.web, "create", cbEmbedCreate, data)
	hookDownloads() // to cancel the downloads of its links

	if v.w == nil {
		v.width, v.height = max(o.Width, 1), max(o.Height, 1)
		v.offWin = gtkOffscreenWindowNew()
		gtkWidgetSetSizeRequest(v.web, int32(math.Ceil(v.width)), int32(math.Ceil(v.height)))
		gtkContainerAdd(v.offWin, v.web)
		// It renders only while it shows, as a window would.
		gtkWidgetShowAll(v.offWin)
	} else {
		gtkWidgetSetNoShowAll(v.web, true)
		gtkFixedPut(v.w.embedFixed(), v.web, 0, 0)
		v.w.webViews = append(v.w.webViews, v)
	}
	b.webViews[v.id] = v
	return v, nil
}

// embedFixed returns the GtkFixed over the surface that holds the
// window's web views, making it the first time.
func (w *window) embedFixed() ptr {
	if w.fixed != 0 {
		return w.fixed
	}
	w.fixed = gtkFixedNew()
	gtkOverlayAddOverlay(w.overlay, w.fixed)
	if gtkOverlaySetOverlayPassThrough != nil {
		// The surface gets the input where no web view is.
		gtkOverlaySetOverlayPassThrough(w.overlay, w.fixed, true)
	}
	if gtkOverlayReorderOverlay != nil {
		// Below the title buttons of a hidden title bar.
		gtkOverlayReorderOverlay(w.overlay, w.fixed, 0)
	}
	gtkWidgetShow(w.fixed)
	return w.fixed
}

func (v *webView) LoadURL(url string) {
	if v.web != 0 {
		v.expectMain(url)
		webkitWebViewLoadURI(v.web, cs(url))
	}
}

func (v *webView) LoadHTML(html string) {
	if v.web != 0 {
		v.expectMain("about:blank")
		webkitWebViewLoadHTML(v.web, cs(html), nil)
	}
}

// LoadFile loads a file URL. WebKitGTK has no way to limit what a file
// page reads to readAccess: a directory lets it fetch the files of any
// directory.
func (v *webView) LoadFile(path, readAccess string) {
	if v.web == 0 {
		return
	}
	webkitSettingsSetAllowFileAccessFromFileURLs(webkitWebViewGetSettings(v.web), readAccess != path)
	f := gFileNewForPath(cs(path))
	uri := takeStr(gFileGetURI(f))
	gObjectUnref(f)
	v.expectMain(uri)
	webkitWebViewLoadURI(v.web, cs(uri))
}

func (v *webView) Eval(js string) {
	switch {
	case v.web == 0:
	case webkitWebViewEvaluateJavascript != nil:
		webkitWebViewEvaluateJavascript(v.web, unsafe.StringData(js), len(js), nil, nil, 0, 0, 0)
	case webkitWebViewRunJavascript != nil:
		webkitWebViewRunJavascript(v.web, cs(js), 0, 0, 0)
	}
}

func (v *webView) Call(body string, done func(string, error)) {
	// The result comes back as JSON on every platform.
	v.call("return JSON.stringify(await (async () => {"+body+"\n})())", done)
}

// call runs body as the body of an async function in the top document and
// passes done its result as a string, "" for undefined.
func (v *webView) call(body string, done func(string, error)) {
	if v.web == 0 {
		done("", errors.New("mygo: the web view is closed"))
		return
	}
	if webkitWebViewCallAsyncJavascriptFunction == nil {
		done("", errors.New("mygo: WebView.Call requires WebKitGTK 2.40 or later"))
		return
	}
	id := pending.add(func(source, res ptr) {
		var gerr ptr
		r := webkitWebViewCallAsyncJavascriptFunctionFinish(source, res, &gerr)
		if gerr != 0 {
			done("", gErr(gerr))
			return
		}
		s := ""
		if !jscValueIsUndefined(r) {
			s = takeStr(jscValueToString(r))
		}
		gObjectUnref(r)
		done(s, nil)
	})
	webkitWebViewCallAsyncJavascriptFunction(v.web, unsafe.StringData(body), len(body), 0, nil, nil, 0, cbAsyncReady, id)
}

func (v *webView) Place(r platform.RectF) {
	if v.w == nil || v.web == 0 || r == v.rect && v.shown {
		return
	}
	gtkFixedMove(v.w.fixed, v.web, int32(math.Round(r.X)), int32(math.Round(r.Y)))
	gtkWidgetSetSizeRequest(v.web, max(int32(math.Round(r.W)), 1), max(int32(math.Round(r.H)), 1))
	if !v.shown {
		gtkWidgetShow(v.web)
	}
	v.rect, v.shown = r, true
}

func (v *webView) Hide() {
	if v.w == nil || v.web == 0 || !v.shown {
		return
	}
	v.Blur()
	gtkWidgetHide(v.web)
	v.shown = false
}

func (v *webView) Focus() {
	if v.w != nil && v.web != 0 && v.shown {
		gtkWidgetGrabFocus(v.web)
	}
}

func (v *webView) Blur() {
	if v.Focused() {
		gtkWidgetGrabFocus(v.w.surface.area)
	}
}

func (v *webView) Focused() bool {
	if v.w == nil || v.web == 0 {
		return false
	}
	f := gtkWindowGetFocus(v.w.win)
	return f != 0 && (f == v.web || gtkWidgetIsAncestor(f, v.web))
}

// SetBackground shows c where the page paints nothing, opaque, unless the
// web view is transparent; a 0 alpha is WebKit's white.
func (v *webView) SetBackground(c platform.Color) {
	switch {
	case v.web == 0:
	case v.transparent:
		webkitWebViewSetBackgroundColor(v.web, &gdkRGBA{})
	case c.A == 0:
		webkitWebViewSetBackgroundColor(v.web, &gdkRGBA{1, 1, 1, 1})
	default:
		webkitWebViewSetBackgroundColor(v.web, &gdkRGBA{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, 1})
	}
}

// Snapshot renders what the web view shows. An offscreen one first grows
// to take in r and, for more pixels than it has, zooms in with its size
// zoomed as much, so that the page lays out as before and renders sharp.
func (v *webView) Snapshot(r platform.RectF, pixelWidth float64, done func([]byte, error)) {
	if v.web == 0 {
		done(nil, errors.New("mygo: the web view is closed"))
		return
	}
	if r.W <= 0 || r.H <= 0 || pixelWidth < 1 {
		done(nil, errors.New("mygo: an empty snapshot"))
		return
	}
	if v.w != nil {
		v.snapshot(r, 1, pixelWidth, done)
		return
	}
	v.snapshots = append(v.snapshots, func() {
		finish := func(png []byte, err error) {
			if v.web != 0 {
				webkitWebViewSetZoomLevel(v.web, 1)
				v.resize(v.width, v.height)
			}
			done(png, err)
			v.snapshots = v.snapshots[1:]
			if len(v.snapshots) > 0 {
				v.snapshots[0]()
			}
		}
		if v.web == 0 {
			finish(nil, errors.New("mygo: the web view is closed"))
			return
		}
		// The page's device pixel ratio, at zoom 1, is WebKit's device
		// scale, which on Wayland is the monitor's, not GTK's 1 of an
		// offscreen window.
		v.call("return devicePixelRatio", func(ratio string, err error) {
			if err != nil {
				finish(nil, err)
				return
			}
			scale, _ := strconv.ParseFloat(ratio, 64)
			scale = max(scale, 1)
			zoom := min(max(pixelWidth/(r.W*scale), 1), 8)
			width, height := max(v.width, r.X+r.W), max(v.height, r.Y+r.H)
			if zoom == 1 && width == v.width && height == v.height {
				v.snapshot(r, 1, pixelWidth, finish)
				return
			}
			webkitWebViewSetZoomLevel(v.web, zoom)
			v.resize(width*zoom, height*zoom)
			// Snapshot once the page has the new size and zoom: its
			// viewport in CSS pixels is then width by height.
			v.call(fmt.Sprintf(`for (let i = 0; i < 120 && (Math.abs(innerWidth - %g) >= 1 || Math.abs(innerHeight - %g) >= 1); i++)
	await new Promise(r => requestAnimationFrame(r))`, math.Ceil(width), math.Ceil(height)), func(_ string, err error) {
				if err != nil {
					finish(nil, err)
					return
				}
				v.snapshot(r, zoom, pixelWidth, finish)
			})
		})
	})
	if len(v.snapshots) == 1 {
		v.snapshots[0]()
	}
}

// resize sizes an offscreen web view at once.
func (v *webView) resize(width, height float64) {
	gtkWidgetSetSizeRequest(v.web, int32(math.Ceil(width)), int32(math.Ceil(height)))
	gtkWindowResize(v.offWin, 1, 1)
	gtkContainerCheckResize(v.offWin)
}

// snapshot renders r of the visible page, at zoom, to a PNG pixelWidth
// pixels wide.
func (v *webView) snapshot(r platform.RectF, zoom, pixelWidth float64, done func([]byte, error)) {
	id := pending.add(func(source, res ptr) {
		var gerr ptr
		surface := webkitWebViewGetSnapshotFinish(source, res, &gerr)
		if gerr != 0 {
			done(nil, gErr(gerr))
			return
		}
		defer cairoSurfaceDestroy(surface)
		width := int32(math.Round(pixelWidth))
		height := max(int32(math.Round(pixelWidth*r.H/r.W)), 1)
		const formatARGB32, filterGood, filterBest = 0, 1, 2
		out := cairoImageSurfaceCreate(formatARGB32, width, height)
		defer cairoSurfaceDestroy(out)
		cr := cairoCreate(out)
		// The snapshot has the web view's size times WebKit's device scale,
		// which needn't be GTK's, nor be set on it: an offscreen window on
		// Wayland has GTK's scale of 1 and WebKit's of the monitor. Its
		// width over the web view's tells it.
		var a gdkRectangle
		gtkWidgetGetAllocation(v.web, &a)
		scale := 1.0
		if a.Width > 1 { // laid out
			scale = float64(cairoImageSurfaceGetWidth(surface)) / float64(a.Width)
		}
		cairoSurfaceSetDeviceScale(surface, scale, scale)
		// In the web view's units (GTK's logical pixels).
		cairoScale(cr, float64(width)/(r.W*zoom), float64(height)/(r.H*zoom))
		cairoSetSourceSurface(cr, surface, -r.X*zoom, -r.Y*zoom)
		filter := int32(filterGood)
		if float64(width) < r.W*zoom*scale {
			filter = filterBest // shrinking: average the pixels
		}
		cairoPatternSetFilter(cairoGetSource(cr), filter)
		cairoPaint(cr)
		cairoDestroy(cr)
		png, err := surfaceToPNG(out)
		done(png, err)
	})
	const regionVisible, optionsNone = 0, 0
	webkitWebViewGetSnapshot(v.web, regionVisible, optionsNone, 0, cbAsyncReady, id)
}

func (v *webView) Destroy() {
	if v.web == 0 {
		return
	}
	v.Blur()
	delete(v.b.webViews, v.id)
	if v.w != nil {
		for i, o := range v.w.webViews {
			if o == v {
				v.w.webViews = append(v.w.webViews[:i], v.w.webViews[i+1:]...)
				break
			}
		}
	}
	for _, name := range []string{"mygoWebView", "mygoWebViewNested"} {
		webkitUserContentManagerUnregisterHandler(v.ucm, cs(name))
	}
	webkitUserContentManagerRemoveAllScripts(v.ucm)
	webkitWebViewStopLoading(v.web)
	if v.offWin != 0 {
		gtkWidgetDestroy(v.offWin)
		v.offWin = 0
	} else {
		gtkWidgetDestroy(v.web) // out of the fixed
	}
	gObjectUnref(v.web)
	gObjectUnref(v.ucm)
	v.web, v.ucm = 0, 0
}

// destroyWebViews tears down the web views of a window that closes.
func (w *window) destroyWebViews() {
	for len(w.webViews) > 0 {
		w.webViews[len(w.webViews)-1].Destroy()
	}
}

func (b *Backend) webView(data ptr) *webView { return b.webViews[int(data)] }

// expectMain notes that the top document is about to navigate to url.
func (v *webView) expectMain(url string) {
	v.mainNavs = append(v.mainNavs, url)
	if len(v.mainNavs) > 8 {
		v.mainNavs = v.mainNavs[1:]
	}
}

// navigationFrame tells the frame a navigation is for: the top document
// when it was expected there (expectMain), or one WebKit does not say.
func (v *webView) navigationFrame(url string) platform.WebViewFrame {
	trim := func(u string) string { return strings.TrimSuffix(u, "/") }
	for i, u := range v.mainNavs {
		if trim(u) == trim(url) {
			v.mainNavs = v.mainNavs[i+1:]
			return platform.FrameMain
		}
	}
	return platform.FrameUnknown
}

func initEmbedCallbacks() {
	b := func() *Backend { return theBackend }
	cbEmbedMessage = purego.NewCallback(func(ucm, result, data ptr) {
		v := b().webView(data >> 1)
		if v == nil {
			return
		}
		msg := takeStr(jscValueToString(webkitJavascriptResultGetJSValue(result)))
		frame := platform.FrameChild
		if data&1 != 0 {
			frame = platform.FrameNested
		} else if m, ok := strings.CutPrefix(msg, v.secret); ok {
			switch {
			case strings.HasPrefix(m, ":"):
				msg, frame = m[1:], platform.FrameMain
			case strings.HasPrefix(m, "n"):
				v.expectMain(m[1:])
				return
			default:
				return
			}
		}
		v.h.WebViewMessage(msg, frame)
	})
	cbEmbedPolicy = purego.NewCallback(func(web, decision ptr, kind int32, data ptr) bool {
		v := b().webView(data)
		if v == nil {
			return false
		}
		switch kind {
		case 0, 1: // WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION, _NEW_WINDOW_ACTION
			action := webkitNavigationPolicyDecisionGetNavigationAction(decision)
			request := webkitNavigationActionGetRequest(action)
			url := goStr(webkitURIRequestGetURI(request))
			if kind == 1 {
				webkitPolicyDecisionIgnore(decision)
				v.h.WebViewNewWindow(url)
				return true
			}
			navType := webkitNavigationActionGetNavigationType(action)
			n := platform.WebViewNavigation{
				URL:   url,
				Frame: v.navigationFrame(url),
				// A link clicked, a form submitted, or anything during a
				// user gesture.
				UserInitiated: navType == 0 || navType == 1 || webkitNavigationActionIsUserGesture(action),
			}
			if v.h.WebViewNavigate(n) {
				webkitPolicyDecisionUse(decision)
			} else {
				webkitPolicyDecisionIgnore(decision)
			}
			return true
		case 2: // WEBKIT_POLICY_DECISION_TYPE_RESPONSE
			// Embedded web views download nothing: what WebKit cannot show
			// is not loaded.
			if webkitResponsePolicyDecisionIsMIMETypeSupported(decision) {
				return false
			}
			webkitPolicyDecisionIgnore(decision)
			return true
		}
		return false
	})
	cbEmbedCreate = purego.NewCallback(func(web, action, data ptr) ptr {
		if v := b().webView(data); v != nil {
			v.h.WebViewNewWindow(goStr(webkitURIRequestGetURI(webkitNavigationActionGetRequest(action))))
		}
		return 0 // no window
	})
	cbEmbedLoadChanged = purego.NewCallback(func(web ptr, event int32, data ptr) {
		if v := b().webView(data); v != nil && event == 3 { // WEBKIT_LOAD_FINISHED
			v.h.WebViewLoaded()
		}
	})
	cbEmbedCrashed = purego.NewCallback(func(web ptr, reason int32, data ptr) {
		if v := b().webView(data); v != nil {
			v.h.WebViewCrashed()
		}
	})
}

// isEmbedded reports an embedded web view's WebKitWebView.
func (b *Backend) isEmbedded(web ptr) bool {
	for _, v := range b.webViews {
		if v.web == web {
			return true
		}
	}
	return false
}
