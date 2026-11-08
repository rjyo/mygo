//go:build windows

package video

import (
	"math"
	"syscall"
)

// On amd64, syscall passes each of the first four arguments in the
// floating point register of its place too, and returns the first one
// (XMM0), as Windows passes and returns doubles.

func loadFloatCalls() error { return nil }

// callSetDouble calls method i of a COM object taking a double.
func callSetDouble(obj uintptr, i int, v float64) uintptr {
	r, _, _ := syscall.SyscallN(method(obj, i), obj, uintptr(math.Float64bits(v)))
	return r
}

// callGetDouble calls method i of a COM object returning a double.
func callGetDouble(obj uintptr, i int) float64 {
	_, r2, _ := syscall.SyscallN(method(obj, i), obj)
	return math.Float64frombits(uint64(r2))
}
