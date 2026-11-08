//go:build windows

package video

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"
)

// On arm64, Windows passes and returns doubles in the floating point
// registers, which syscall neither sets nor reads. Two thunks of machine
// code, made once in memory of their own, move them between those and the
// integer registers syscall has:
//
//	setDouble(obj X0, bits X1, fn X2): FMOV D0, X1; BR X2
//	getDouble(obj X0, fn X1): STP X29, X30, [SP, #-16]!; MOV X29, SP;
//	                          BLR X1; FMOV X0, D0; LDP X29, X30, [SP], #16; RET

var setDoubleThunk, getDoubleThunk uintptr

func loadFloatCalls() error {
	code := []uint32{
		0x9e670020, 0xd61f0040, // setDouble
		0xa9bf7bfd, 0x910003fd, 0xd63f0020, 0x9e660000, 0xa8c17bfd, 0xd65f03c0, // getDouble
	}
	k := syscall.NewLazyDLL("kernel32.dll")
	const memCommitReserve, pageReadWrite, pageExecuteRead = 0x3000, 0x04, 0x20
	mem, _, err := k.NewProc("VirtualAlloc").Call(0, uintptr(4*len(code)), memCommitReserve, pageReadWrite)
	if mem == 0 {
		return fmt.Errorf("video: VirtualAlloc: %v", err)
	}
	copy(unsafe.Slice((*uint32)(cptr(mem)), len(code)), code)
	var old uint32
	if r, _, err := k.NewProc("VirtualProtect").Call(mem, uintptr(4*len(code)), pageExecuteRead, uintptr(unsafe.Pointer(&old))); r == 0 {
		return fmt.Errorf("video: VirtualProtect: %v", err)
	}
	proc, _, _ := k.NewProc("GetCurrentProcess").Call()
	k.NewProc("FlushInstructionCache").Call(proc, mem, uintptr(4*len(code)))
	setDoubleThunk, getDoubleThunk = mem, mem+8
	return nil
}

// callSetDouble calls method i of a COM object taking a double.
func callSetDouble(obj uintptr, i int, v float64) uintptr {
	r, _, _ := syscall.SyscallN(setDoubleThunk, obj, uintptr(math.Float64bits(v)), method(obj, i))
	return r
}

// callGetDouble calls method i of a COM object returning a double.
func callGetDouble(obj uintptr, i int) float64 {
	r, _, _ := syscall.SyscallN(getDoubleThunk, obj, method(obj, i))
	return math.Float64frombits(uint64(r))
}
