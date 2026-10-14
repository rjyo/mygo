package ui

import "weak"

// Services provides persistent window services for custom controls and
// asynchronous work. It owns no frame data and does not retain the window.
// Clipboard and URL methods run on the UI thread; Invalidate and Repaint
// may run on any goroutine. Models are still updated through Window.Update.
type Services struct{ owner weak.Pointer[engine] }

// Services returns the persistent services of this frame's window.
func (c *Context) Services() Services {
	if c == nil {
		return Services{}
	}
	return c.services
}

func (s Services) Invalidate() {
	if rt := s.owner.Value(); rt != nil {
		rt.host.invalidate()
	}
}

// Repaint asks for a frame painting again the elements whose drawings
// noted that they change on their own (Painter.Live), as a terminal's grid
// as output comes, without building the view: a frame built anew comes
// instead when something else asked for one. It may run on any goroutine.
func (s Services) Repaint() {
	if rt := s.owner.Value(); rt != nil && rt.repaintPosted.CompareAndSwap(false, true) {
		rt.host.post(rt.repaint)
	}
}

func (s Services) ReadClipboard() string {
	if rt := s.owner.Value(); rt != nil && !rt.closed {
		return rt.host.readClipboard()
	}
	return ""
}
func (s Services) WriteClipboard(text string) {
	if rt := s.owner.Value(); rt != nil && !rt.closed {
		rt.host.writeClipboard(text)
	}
}
func (s Services) OpenURL(url string) {
	if rt := s.owner.Value(); rt != nil && !rt.closed {
		rt.host.openURL(url, nil)
	}
}
