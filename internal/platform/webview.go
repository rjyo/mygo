package platform

// Embedded web views: a system web view inside a window of native UI,
// placed over a rectangle of its surface, or kept offscreen to render
// pages into images. The window's own web page (WindowOptions without
// Surface) is a different thing; these never get the window's bridge or
// its bound services.

// WebViewFrame says which frame of an embedded web view's page something
// came from.
type WebViewFrame uint8

const (
	// FrameUnknown: the backend can't tell. WebKitGTK doesn't say which
	// frame a navigation is for: iframes' navigations, redirects and
	// scripted navigations of a document of no origin are unknown there.
	FrameUnknown WebViewFrame = iota
	// FrameMain is the top document, the one LoadURL or LoadHTML loads.
	FrameMain
	// FrameChild is an iframe of the top document.
	FrameChild
	// FrameNested is an iframe inside an iframe.
	FrameNested
)

// WebViewNavigation is a navigation a page asks for.
type WebViewNavigation struct {
	URL   string
	Frame WebViewFrame
	// UserInitiated: a link click or a form submission, as opposed to a
	// script or a redirect. WebKitGTK counts any navigation during a user
	// gesture, Eval's included.
	UserInitiated bool
}

// WebViewOptions configure an embedded web view.
type WebViewOptions struct {
	// Scripts run in its pages, in order, after the backend's own
	// window.mygoWebView shim.
	Scripts []UserScript
	// DevTools allows the web inspector.
	DevTools bool
	// Transparent: no background of its own until the page paints one,
	// so the window shows through (macOS, Linux; on Windows it shows
	// Background, or black).
	Transparent bool
	// Background is shown where the page paints nothing: under a
	// transparent page on Windows, under any page elsewhere unless
	// Transparent. A 0 alpha leaves the platform's default (white).
	Background Color
	// Radius rounds the corners in Corners (all of them when 0).
	Radius  float64
	Corners Corners
	// Width and Height size an offscreen web view (no parent window).
	Width, Height float64
}

// Corners is a set of corners of a rectangle.
type Corners uint8

const (
	CornerTopLeft Corners = 1 << iota
	CornerTopRight
	CornerBottomLeft
	CornerBottomRight
)

// WebViewHandler receives an embedded web view's events, on the main
// thread.
type WebViewHandler interface {
	// WebViewMessage is a string the page posted with
	// window.mygoWebView.postMessage, and the frame it posted from. Only
	// FrameMain can be trusted to come from the top document.
	WebViewMessage(msg string, frame WebViewFrame)
	// WebViewNavigate decides whether a navigation goes ahead.
	WebViewNavigate(n WebViewNavigation) bool
	// WebViewNewWindow is a page asking for a new window (window.open,
	// target=_blank). No window opens.
	WebViewNewWindow(url string)
	// WebViewLoaded: the top document finished loading.
	WebViewLoaded()
	// WebViewCrashed: the page's process ended. The web view is blank
	// until it loads again.
	WebViewCrashed()
}

// WebView is an embedded web view. Its methods run on the main thread.
type WebView interface {
	LoadURL(url string)
	// LoadHTML loads html as a document of no origin (about:blank).
	LoadHTML(html string)
	// LoadFile loads a local file, giving the page read access to
	// readAccess (a file or a directory).
	LoadFile(path, readAccess string)
	// Eval runs js in the top document.
	Eval(js string)
	// Call runs body as the body of an async function in the top
	// document and passes done its result as JSON.
	Call(body string, done func(result string, err error))
	// Place shows the web view over r, in the surface's coordinates.
	Place(r RectF)
	// Hide hides it, keeping its page, and gives the keyboard back to the
	// surface if the page had it.
	Hide()
	Focus()
	// Blur gives the keyboard back to the surface if the page has it.
	Blur()
	Focused() bool
	// SetBackground changes Background.
	SetBackground(c Color)
	// Snapshot renders r of the page, in its coordinates, to a PNG
	// pixelWidth pixels wide.
	Snapshot(r RectF, pixelWidth float64, done func(png []byte, err error))
	Destroy()
}
