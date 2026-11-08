//go:build (darwin || linux) && (amd64 || arm64)

package ffi

import "github.com/ebitengine/purego"

func open(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
}

func symbol(h uintptr, name string) (uintptr, error) { return purego.Dlsym(h, name) }
