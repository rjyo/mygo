package fake

import (
	"errors"
	"sync"

	"github.com/egoist/mygo/internal/platform"
)

// WebView is an embedded web view that records what it is asked to do.
// Tests fire its events through H.
type WebView struct {
	Parent *Window // nil when offscreen
	Opts   platform.WebViewOptions
	H      platform.WebViewHandler

	mu        sync.Mutex
	loads     []string
	evals     []string
	rect      platform.RectF
	shown     bool
	focused   bool
	destroyed bool
	// CallResult and SnapshotPNG answer Call and Snapshot.
	CallResult  string
	SnapshotPNG []byte
}

// NewWebView embeds a fake web view; the parent must have a surface.
func (b *Backend) NewWebView(parent platform.Window, o *platform.WebViewOptions, h platform.WebViewHandler) (platform.WebView, error) {
	v := &WebView{Opts: *o, H: h}
	if parent != nil {
		w := parent.(*Window)
		if w.surface == nil {
			return nil, errors.New("fake: embedded web views need a window of native UI")
		}
		v.Parent = w
	}
	b.mu.Lock()
	b.webviews = append(b.webviews, v)
	b.mu.Unlock()
	return v, nil
}

// WebViews returns the embedded web views created so far.
func (b *Backend) WebViews() []*WebView {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*WebView(nil), b.webviews...)
}

func (v *WebView) record(load string) {
	v.mu.Lock()
	v.loads = append(v.loads, load)
	v.mu.Unlock()
}

func (v *WebView) LoadURL(url string)           { v.record("url:" + url) }
func (v *WebView) LoadHTML(html string)         { v.record("html:" + html) }
func (v *WebView) LoadFile(path, access string) { v.record("file:" + path + "|" + access) }
func (v *WebView) Eval(js string)               { v.mu.Lock(); v.evals = append(v.evals, js); v.mu.Unlock() }
func (v *WebView) Call(body string, done func(string, error)) {
	v.Eval(body)
	done(v.CallResult, nil)
}

func (v *WebView) Place(r platform.RectF) {
	v.mu.Lock()
	v.rect, v.shown = r, true
	v.mu.Unlock()
}

func (v *WebView) Hide() {
	v.mu.Lock()
	v.shown, v.focused = false, false
	v.mu.Unlock()
}

func (v *WebView) Focus() {
	v.mu.Lock()
	v.focused = v.shown
	v.mu.Unlock()
}

func (v *WebView) Blur() {
	v.mu.Lock()
	v.focused = false
	v.mu.Unlock()
}

func (v *WebView) Focused() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.focused
}

func (v *WebView) SetBackground(c platform.Color) {
	v.mu.Lock()
	v.Opts.Background = c
	v.mu.Unlock()
}

func (v *WebView) Snapshot(_ platform.RectF, _ float64, done func([]byte, error)) {
	done(v.SnapshotPNG, nil)
}

func (v *WebView) Destroy() {
	v.mu.Lock()
	v.destroyed, v.shown = true, false
	v.mu.Unlock()
}

// Loads returns what it was asked to load: "url:…", "html:…" or
// "file:path|access".
func (v *WebView) Loads() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.loads...)
}

// Evals returns the scripts passed to Eval and Call.
func (v *WebView) Evals() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.evals...)
}

// Rect returns where it was last placed and whether it shows.
func (v *WebView) Rect() (platform.RectF, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.rect, v.shown
}

// Destroyed reports whether Destroy was called.
func (v *WebView) Destroyed() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.destroyed
}
