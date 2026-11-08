//go:build mygo_nodownload

package ffi

import "github.com/egoist/mygo/internal/nativelib"

var fetch func(f nativelib.File, path string) error
