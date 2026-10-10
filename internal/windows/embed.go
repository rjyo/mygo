//go:build windows && (amd64 || arm64)

package windows

import (
	"errors"

	"github.com/egoist/mygo/internal/platform"
)

// NewWebView is not implemented on this platform yet.
func (b *Backend) NewWebView(platform.Window, *platform.WebViewOptions, platform.WebViewHandler) (platform.WebView, error) {
	return nil, errors.New("mygo: embedded web views are not implemented on windows yet")
}
