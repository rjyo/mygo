//go:build !((darwin || linux || windows) && (amd64 || arm64))

package ffi

import "errors"

const supported = false

func open(string) (uintptr, error)            { return 0, errors.ErrUnsupported }
func symbol(uintptr, string) (uintptr, error) { return 0, errors.ErrUnsupported }
func register(any, uintptr, string) error     { return errors.ErrUnsupported }
