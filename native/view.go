package native

import (
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/viewhost"
)

// The windows of apps that use native modules host their views, which
// ui.NativeView shows.
func init() { viewhost.Embed = embed }

// gone are what to run as a view hosted in windows closes or is released,
// by the view, which takes it out of them. Main thread only.
var gone = map[uintptr][]func(){}

func embed(s platform.Surface, view uintptr, onGone func()) platform.WebView {
	vs, ok := s.(platform.ViewSurface)
	if !ok {
		return nil
	}
	e, err := vs.EmbedView(view)
	if err != nil {
		return nil
	}
	gone[view] = append(gone[view], onGone)
	return e
}

// viewGone tells the windows hosting view that it is closed or released.
// Main thread only.
func viewGone(view uintptr) {
	fs := gone[view]
	delete(gone, view)
	for _, f := range fs {
		f()
	}
}
