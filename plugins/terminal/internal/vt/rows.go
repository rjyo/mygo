package vt

import (
	"bytes"
	"unsafe"
)

// What a Mirror reads of a terminal besides its rows, which a render state
// reads.

const (
	rowDataWrap           = 1
	rowDataSemanticPrompt = 6

	dataKittyKeyboardFlags = 8
	dataScrollbackRows     = 15
)

// ScrollbackRows returns how many rows the scrollback holds.
func (t *Terminal) ScrollbackRows() int {
	var n uintptr
	t.get(dataScrollbackRows, unsafe.Pointer(&n))
	return int(n)
}

// RowWrapped reports whether row y of the viewport soft-wraps into the
// next.
func (t *Terminal) RowWrapped(y int) bool {
	ref, found := t.CellAt(0, y)
	if !found {
		return false
	}
	var raw uint64
	if !ok(call(fnGridRefRow, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&raw)))) {
		return false
	}
	var v bool
	return ok(call(fnRowData, uintptr(raw), rowDataWrap, uintptr(unsafe.Pointer(&v)))) && v
}

// KittyKeyboardFlags returns the flags of the kitty keyboard protocol
// programs set.
func (t *Terminal) KittyKeyboardFlags() int {
	var f uint8
	t.get(dataKittyKeyboardFlags, unsafe.Pointer(&f))
	return int(f)
}

// ModifyOtherKeys reports whether xterm's modifyOtherKeys mode 2 is on
// (CSI > 4 ; 2 m). The library tells it in no GhosttyTerminalData, but a
// key encoder takes it from the terminal with the other modes: without the
// Kitty keyboard protocol, Ctrl+P is CSI 27 ; 5 ; 112 ~ in mode 2, and ^P
// otherwise.
func (t *Terminal) ModifyOtherKeys() bool {
	e, err := NewKeyEncoder()
	if err != nil {
		return false
	}
	defer e.Free()
	call(fnKeyEncoderSetoptFromTerminal, e.h, t.h)
	var kitty uint8
	call(fnKeyEncoderSetopt, e.h, 5, uintptr(unsafe.Pointer(&kitty)))
	b := e.Encode(KeyEvent{Action: KeyPress, Key: KeyP, Mods: ModCtrl, Text: "p", Unshifted: 'p'})
	return bytes.HasPrefix(b, []byte("\x1b[27;"))
}
