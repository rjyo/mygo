package e2e

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// webViewEvents collects an embedded web view's events.
type webViewEvents struct {
	mu        sync.Mutex
	messages  []string // "frame:msg"
	navs      []mygo.WebViewNavigation
	newWindow []string
	loads     int
}

func (e *webViewEvents) options() mygo.WebViewOptions {
	return mygo.WebViewOptions{
		OnMessage: func(msg string, frame mygo.WebViewFrame) {
			e.mu.Lock()
			e.messages = append(e.messages, frame.String()+":"+msg)
			e.mu.Unlock()
		},
		OnNavigate: func(n mygo.WebViewNavigation) bool {
			e.mu.Lock()
			e.navs = append(e.navs, n)
			e.mu.Unlock()
			return n.URL != "https://blocked.example/"
		},
		OnNewWindow: func(url string) {
			e.mu.Lock()
			e.newWindow = append(e.newWindow, url)
			e.mu.Unlock()
		},
		OnLoad: func() {
			e.mu.Lock()
			e.loads++
			e.mu.Unlock()
		},
	}
}

func (e *webViewEvents) has(msg string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Contains(e.messages, msg)
}

func (e *webViewEvents) nav(url string) (mygo.WebViewNavigation, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range e.navs {
		if n.URL == url {
			return n, true
		}
	}
	return mygo.WebViewNavigation{}, false
}

// webViewServer serves a page with a nested frame on a loopback port.
func webViewServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/child", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<body>child<iframe src="/nested"></iframe>
<script>mygoWebView.postMessage("child")</script></body>`)
	})
	mux.HandleFunc("/nested", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<body>nested<script>mygoWebView.postMessage("nested")</script></body>`)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return "http://" + ln.Addr().String()
}

func callResult(t *testing.T, v *mygo.WebView, body string) string {
	t.Helper()
	type res struct {
		s   string
		err error
	}
	ch := make(chan res, 1)
	v.Call(body, func(s string, err error) { ch <- res{s, err} })
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("Call(%q): %v", body, r.err)
		}
		return r.s
	case <-time.After(10 * time.Second):
		t.Fatalf("Call(%q) did not return", body)
	}
	return ""
}

// TestEmbeddedWebView embeds a web view in a window of native UI: frames
// post messages, navigations go through OnNavigate, new windows through
// OnNewWindow, and the keyboard moves between it and the surface.
func TestEmbeddedWebView(t *testing.T) {
	var frames atomic.Int32
	w := newWindow(t, mygo.WindowOptions{Title: "WebView", Width: 400, Height: 300, Content: ui.View(func(c *ui.Context) {
		frames.Add(1)
		ui.Box(c).Fill().Background(ui.RGB(30, 144, 255))
	})})
	eventually(t, "a frame", func() bool { return frames.Load() > 0 })

	base := webViewServer(t)
	var ev webViewEvents
	opts := ev.options()
	opts.Scripts = []mygo.WebViewScript{{Source: `window.injected = (window.injected || 0) + 1`}}
	v, err := w.NewWebView(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	v.LoadHTML(fmt.Sprintf(`<body style="background:#f00">main<iframe src="%s/child"></iframe>
<script>mygoWebView.postMessage("main")</script></body>`, base))
	v.Place(20, 20, 300, 200)

	eventually(t, "the main document's message", func() bool { return ev.has("main:main") })
	eventually(t, "the child frame's message", func() bool { return ev.has("child:child") })
	eventually(t, "the nested frame's message", func() bool { return ev.has("nested:nested") })
	if got := callResult(t, v, "return window.injected"); got != "1" {
		t.Errorf("the script ran %s times in the top document, want once", got)
	}
	if got := callResult(t, v, "return 6 * 7"); got != "42" {
		t.Errorf("Call returned %q", got)
	}
	// WebKitGTK tells no navigation which frame it is for: Linux knows the
	// top document's from the API and from what the document announces,
	// and reports the others as FrameUnknown.
	unknownOnLinux := func(f mygo.WebViewFrame) bool { return runtime.GOOS == "linux" && f == mygo.FrameUnknown }
	if n, ok := ev.nav(base + "/child"); !ok || n.Frame != mygo.FrameChild && !unknownOnLinux(n.Frame) {
		t.Errorf("the child frame's navigation is %+v (seen %v)", n, ok)
	}
	if n, ok := ev.nav(base + "/nested"); !ok || n.Frame != mygo.FrameNested && n.Frame != mygo.FrameUnknown {
		t.Errorf("the nested frame's navigation is %+v (seen %v)", n, ok)
	}

	v.Eval(`location.href = "https://blocked.example/"`)
	eventually(t, "the blocked navigation", func() bool { _, ok := ev.nav("https://blocked.example/"); return ok })
	// A script setting location in a document of no origin goes
	// unannounced on Linux (the Navigation API is silent there); a link it
	// follows does not.
	if n, _ := ev.nav("https://blocked.example/"); n.Frame != mygo.FrameMain && !unknownOnLinux(n.Frame) {
		t.Errorf("the top document's navigation is in frame %v", n.Frame)
	}
	if got := callResult(t, v, "return document.body.firstChild.textContent"); got != `"main"` {
		t.Errorf("a cancelled navigation left the page at %s", got)
	}
	if runtime.GOOS == "linux" {
		v.Eval(`var a = document.createElement("a"); a.href = "https://blocked.example/"; document.body.append(a); a.click()`)
		eventually(t, "the blocked link", func() bool {
			ev.mu.Lock()
			defer ev.mu.Unlock()
			return len(ev.navs) > 0 && ev.navs[len(ev.navs)-1].Frame == mygo.FrameMain && ev.navs[len(ev.navs)-1].URL == "https://blocked.example/"
		})
	}

	v.Eval(`window.open("https://example.com/new")`)
	eventually(t, "the new window request", func() bool {
		ev.mu.Lock()
		defer ev.mu.Unlock()
		return slices.Contains(ev.newWindow, "https://example.com/new")
	})

	w.Focus()
	v.Focus()
	eventually(t, "the page to take the keyboard", v.Focused)
	v.Hide()
	if v.Focused() {
		t.Error("a hidden web view keeps the keyboard")
	}

	// The page shows over the surface where it is placed.
	v.Place(20, 20, 300, 200)
	time.Sleep(300 * time.Millisecond)
	data, err := w.CapturePage()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	s := deviceScale(w)
	at := func(x, y float64) color.RGBA {
		b := img.Bounds()
		return color.RGBAModel.Convert(img.At(b.Min.X+int(x*s), b.Min.Y+int(y*s))).(color.RGBA)
	}
	// What a capture shows of the web view itself differs by platform;
	// the surface beside it must still show.
	if c := at(10, 10); c.B < 200 {
		t.Errorf("the surface beside the web view is %v", c)
	}
}

// TestOffscreenWebView renders a page in no window to an image.
func TestOffscreenWebView(t *testing.T) {
	var ev webViewEvents
	opts := ev.options()
	opts.Width, opts.Height = 200, 100
	v, err := mygo.NewOffscreenWebView(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	v.LoadHTML(`<body style="margin:0"><div style="width:50px;height:50px;background:rgb(255,0,0)"></div>
<div style="width:50px;height:50px;background:rgb(0,0,255)"></div></body>`)
	eventually(t, "the page to load", func() bool { ev.mu.Lock(); defer ev.mu.Unlock(); return ev.loads > 0 })
	if got := callResult(t, v, "return document.body.children.length"); got != "2" {
		t.Fatalf("the page has %s children", got)
	}

	type res struct {
		png []byte
		err error
	}
	ch := make(chan res, 1)
	v.Snapshot(0, 0, 50, 100, 100, func(b []byte, err error) { ch <- res{b, err} })
	var r res
	select {
	case r = <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("Snapshot did not return")
	}
	if r.err != nil {
		t.Fatal(r.err)
	}
	img, err := png.Decode(bytes.NewReader(r.png))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 100 || b.Dy() != 200 {
		t.Fatalf("the snapshot is %v, want 100×200", b.Size())
	}
	red := color.RGBAModel.Convert(img.At(b.Min.X+50, b.Min.Y+50)).(color.RGBA)
	blue := color.RGBAModel.Convert(img.At(b.Min.X+50, b.Min.Y+150)).(color.RGBA)
	if red.R < 200 || red.B > 60 || blue.B < 200 || blue.R > 60 {
		t.Errorf("the snapshot shows %v over %v, want red over blue", red, blue)
	}
}

// TestWebViewLargeHTML loads a document larger than WebView2 takes as a
// string (2 MB), as a page with inlined fonts is.
func TestWebViewLargeHTML(t *testing.T) {
	var ev webViewEvents
	opts := ev.options()
	opts.Width, opts.Height = 200, 100
	opts.Scripts = []mygo.WebViewScript{
		{Source: `window.injected = (window.injected || 0) + 1`},
		{Source: `mygoWebView.postMessage("end:" + document.getElementById("pad").textContent.length)`, AtDocumentEnd: true, MainFrameOnly: true},
	}
	v, err := mygo.NewOffscreenWebView(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	const n = 3 << 20
	v.LoadHTML(`<!doctype html><html><head><meta charset="utf-8"></head><body><div id="pad" hidden>` + strings.Repeat("x", n) +
		`</div><p>héllo</p><script>mygoWebView.postMessage("main")</script></body></html>`)
	eventually(t, "the document's message", func() bool { return ev.has("main:main") })
	eventually(t, "the script at the document's end", func() bool { return ev.has(fmt.Sprintf("main:end:%d", n)) })
	for body, want := range map[string]string{
		"return window.injected":                         "1",
		"return location.href":                           `"about:blank"`,
		`return document.querySelector("p").textContent`: `"héllo"`,
		"return document.compatMode":                     `"CSS1Compat"`,
	} {
		if got := callResult(t, v, body); got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}
}
