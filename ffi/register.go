//go:build (darwin || linux || windows) && (amd64 || arm64)

package ffi

import (
	"fmt"

	"github.com/ebitengine/purego"
)

const supported = true

// register makes the function fptr points to call the C function at addr,
// reporting the signatures purego cannot pass as errors rather than panics.
func register(fptr any, addr uintptr, name string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ffi: binding %s: %v", name, r)
		}
	}()
	purego.RegisterFunc(fptr, addr)
	return nil
}
