//go:build !mygo_nodownload

package ffi

import (
	"context"
	"time"

	"github.com/egoist/mygo/internal/download"
	"github.com/egoist/mygo/internal/nativelib"
)

// fetch downloads a published library into the user's cache, for programs
// that are not packaged apps. Production builds of the CLI leave it out
// (mygo_nodownload), and HTTP with it, as packaged apps never download.
var fetch = func(f nativelib.File, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return download.Cached(ctx, download.File{URL: f.URL, SHA256: f.SHA256}, path)
}
