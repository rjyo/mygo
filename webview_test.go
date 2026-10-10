package mygo

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/ui"
)

func TestWebViewNeedsContent(t *testing.T) {
	w, _ := testWindow(t, WindowOptions{})
	if _, err := w.NewWebView(WebViewOptions{}); !errors.Is(err, errNoContent) {
		t.Fatalf("a web view in a page window: %v", err)
	}
}

func TestWebView(t *testing.T) {
	w, _, _ := contentWindow(t, func(c *ui.Context) {})
	var msgs []string
	var frames []WebViewFrame
	v, err := w.NewWebView(WebViewOptions{
		Scripts:      []WebViewScript{{Source: "a"}, {Source: "b", MainFrameOnly: true, AtDocumentEnd: true}},
		CornerRadius: 8,
		Corners:      CornerBottomLeft | CornerBottomRight,
		OnMessage: func(msg string, f WebViewFrame) {
			msgs = append(msgs, msg)
			frames = append(frames, f)
		},
		OnNavigate: func(n WebViewNavigation) bool { return n.Frame != FrameMain },
	})
	if err != nil {
		t.Fatal(err)
	}
	views := fb.WebViews()
	fv := views[len(views)-1]
	if fv.Parent == nil {
		t.Fatal("the web view has no parent window")
	}
	want := []platform.UserScript{{Source: "a", AllFrames: true}, {Source: "b", AtDocumentEnd: true}}
	if len(fv.Opts.Scripts) != 2 || fv.Opts.Scripts[0] != want[0] || fv.Opts.Scripts[1] != want[1] {
		t.Errorf("scripts %+v", fv.Opts.Scripts)
	}
	if fv.Opts.Radius != 8 || fv.Opts.Corners != platform.CornerBottomLeft|platform.CornerBottomRight {
		t.Errorf("corners %+v", fv.Opts)
	}

	v.LoadURL("http://localhost:3000/")
	v.LoadFile("/tmp/a.html", "")
	v.Place(1, 2, 3, 4)
	if r, shown := fv.Rect(); !shown || r != (platform.RectF{X: 1, Y: 2, W: 3, H: 4}) {
		t.Errorf("placed at %v, shown %v", r, shown)
	}
	v.Focus()
	if !v.Focused() {
		t.Error("not focused")
	}
	v.Hide()
	if _, shown := fv.Rect(); shown || v.Focused() {
		t.Error("hiding keeps it shown or focused")
	}
	if got := fv.Loads(); len(got) != 2 || got[0] != "url:http://localhost:3000/" || got[1] != "file:/tmp/a.html|/tmp/a.html" {
		t.Errorf("loads %q", got)
	}

	fv.H.WebViewMessage("hi", platform.FrameChild)
	if len(msgs) != 1 || msgs[0] != "hi" || frames[0] != FrameChild {
		t.Errorf("messages %q %v", msgs, frames)
	}
	if fv.H.WebViewNavigate(platform.WebViewNavigation{URL: "x", Frame: platform.FrameMain}) {
		t.Error("OnNavigate's answer was ignored")
	}

	v.Close()
	v.Close()
	if !fv.Destroyed() {
		t.Error("not destroyed")
	}
	fv.H.WebViewMessage("late", platform.FrameMain)
	if len(msgs) != 1 {
		t.Error("a closed web view still delivers messages")
	}
	done := make(chan error, 1)
	v.Call("return 1", func(_ string, err error) { done <- err })
	if err := <-done; !errors.Is(err, errDestroyed) {
		t.Errorf("Call after Close: %v", err)
	}
}
