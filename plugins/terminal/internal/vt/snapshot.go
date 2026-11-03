package vt

import (
	"errors"
	"runtime"
	"unsafe"
)

// Snapshots (snapshot.h): the complete state of a terminal — its screens,
// their scrollback, the cursor and the modes — in Ghostty's binary format,
// as a server keeping the terminal (Rex) hands a client that shows it.
var fnSnapshotDecoderNewBuf, fnSnapshotDecoderDecode, fnSnapshotDecoderFree uintptr

var snapshotSymbols = []struct {
	name string
	addr *uintptr
}{
	{"ghostty_snapshot_decoder_new_buf", &fnSnapshotDecoderNewBuf},
	{"ghostty_snapshot_decoder_decode", &fnSnapshotDecoderDecode},
	{"ghostty_snapshot_decoder_free", &fnSnapshotDecoderFree},
}

// Restore replaces what the terminal holds with the snapshot's: its
// screens, scrollback, cursor and modes. The settings made on the terminal
// — callbacks, colors, the scrollback's limit, defaults — stay. The
// snapshot's size stays too, until a Resize.
func (t *Terminal) Restore(snap []byte) error {
	if fnSnapshotDecoderNewBuf == 0 || fnSnapshotDecoderDecode == 0 || fnSnapshotDecoderFree == 0 {
		return errors.New("libghostty-vt has no snapshots")
	}
	if len(snap) == 0 {
		return errors.New("empty snapshot")
	}
	// The decoder borrows snap until it is freed.
	var pin runtime.Pinner
	pin.Pin(&snap[0])
	defer pin.Unpin()
	var d uintptr
	if err := result(call(fnSnapshotDecoderNewBuf, 0, uintptr(unsafe.Pointer(&d)), uintptr(unsafe.Pointer(&snap[0])), uintptr(len(snap)))); err != nil {
		return err
	}
	var h uintptr
	err := result(call(fnSnapshotDecoderDecode, d, uintptr(unsafe.Pointer(&h))))
	call(fnSnapshotDecoderFree, d)
	if err != nil {
		return err
	}
	old := t.h
	t.h = h
	for _, name := range t.keptOrder {
		t.kept[name]()
	}
	call(fnTerminalFree, old)
	t.deferred, t.silent = nil, false
	return nil
}
