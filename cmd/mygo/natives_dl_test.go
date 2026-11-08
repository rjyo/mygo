//go:build (darwin || linux) && (amd64 || arm64)

package main

import "github.com/ebitengine/purego"

// loadAdd loads a library built by a test and binds its add function. The
// CLI links purego, not package ffi, which would link package mygo.
func loadAdd(path string) (func(a, b int32) int32, error) {
	h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	var add func(a, b int32) int32
	purego.RegisterLibFunc(&add, h, "add")
	return add, nil
}
