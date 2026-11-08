//go:build !((darwin || linux) && (amd64 || arm64))

package main

// loadAdd returns nil: the tests only check that the library was built.
func loadAdd(string) (func(a, b int32) int32, error) { return nil, nil }
