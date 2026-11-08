//go:build windows && (amd64 || arm64)

package windows

import (
	"bytes"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo/internal/schemestream"
)

func comInit(t *testing.T) {
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	procCoInitializeEx.Call(0, coinitApartmentThreaded)
}

func streamBody(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 13)
	}
	return b
}

// writeChunks writes b into p as the shared layer does, 256 KB at a time.
func writeChunks(p *schemestream.Pipe, b []byte) error {
	for len(b) > 0 {
		k := min(256<<10, len(b))
		if err := p.Write(b[:k]); err != nil {
			return err
		}
		b = b[k:]
	}
	p.Close()
	return nil
}

// TestBodyStreamRead reads a body larger than the pipe holds through the
// IStream, as WebView2 does, while the handler writes it.
func TestBodyStreamRead(t *testing.T) {
	comInit(t)
	want := streamBody(3<<20 + 77)
	var fulls atomic.Int32
	full := make(chan struct{})
	pipe := schemestream.NewPipe(streamLimit, func() {
		if fulls.Add(1) == 1 {
			close(full)
		}
	})
	var canceled atomic.Bool
	wrote := make(chan error, 1)
	go func() { wrote <- writeChunks(pipe, want) }()
	<-full // WebView2 gets the stream once the handler filled the pipe
	s := newBodyStream(pipe, int64(len(want)), func() { canceled.Store(true) })
	got := readStream(s) // reads until S_FALSE
	if err := <-wrote; err != nil {
		t.Fatalf("the handler's writes: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("read %d bytes, not the %d written", len(got), len(want))
	}
	if fulls.Load() != 1 {
		t.Errorf("the pipe filled %d times before a reader came, want 1", fulls.Load())
	}
	// Reads at the end stay at the end.
	var buf [16]byte
	var n uint32
	if hr := comCall(s, 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n))); hr != sFalse || n != 0 {
		t.Errorf("Read at the end = %#x, %d bytes; want S_FALSE, 0", hr, n)
	}
	var pos uint64
	if hr := comCall(s, 5, 0, streamSeekCur, uintptr(unsafe.Pointer(&pos))); hr != sOK || pos != uint64(len(want)) {
		t.Errorf("Seek(0, STREAM_SEEK_CUR) = %#x, %d; want S_OK, %d", hr, pos, len(want))
	}
	if hr := comCall(s, 5, 0, streamSeekSet, 0); hr == sOK {
		t.Error("Seek back to the start succeeded")
	}
	if canceled.Load() {
		t.Error("the context was canceled before WebView2 released the stream")
	}
	release(s)
	if !canceled.Load() {
		t.Error("releasing the stream did not cancel the context")
	}
	if lookupBodyStream(s) != nil {
		t.Error("the stream is still registered after its final Release")
	}
}

// TestBodyStreamInterfaces checks what the stream says it implements: an
// agile IStream, which marshals as the free-threaded marshaler does.
func TestBodyStreamInterfaces(t *testing.T) {
	comInit(t)
	pipe := schemestream.NewPipe(streamLimit, nil)
	var canceled atomic.Bool
	s := newBodyStream(pipe, -1, func() { canceled.Store(true) })
	for name, iid := range map[string]*GUID{
		"IUnknown": &iidIUnknown, "ISequentialStream": &iidISequentialStream,
		"IStream": &iidIStream, "IAgileObject": &iidIAgileObject,
	} {
		q := queryInterface(s, iid)
		if q != s {
			t.Errorf("QueryInterface(%s) = %#x, want the stream %#x", name, q, s)
		}
		release(q)
	}
	if m := queryInterface(s, &iidIMarshal); m == 0 {
		t.Error("no IMarshal: the stream would be called through a proxy on the UI thread")
	} else {
		release(m) // counted on the stream, through the aggregation
	}
	if q := queryInterface(s, &iidINoMarshal); q != 0 {
		t.Error("the stream says it is INoMarshal")
		release(q)
	}
	var st statstg
	if hr := comCall(s, 12, uintptr(unsafe.Pointer(&st)), 1); hr != eNotImpl {
		t.Errorf("Stat without a length = %#x, want E_NOTIMPL", hr)
	}
	if canceled.Load() {
		t.Fatal("the references taken and dropped by the queries released the stream")
	}
	release(s)
	if !canceled.Load() {
		t.Error("the final Release did not cancel the context")
	}

	s = newBodyStream(schemestream.NewPipe(streamLimit, nil), 12345, nil)
	if hr := comCall(s, 12, uintptr(unsafe.Pointer(&st)), 1); hr != sOK || st.size != 12345 || st.typ != stgtyStream {
		t.Errorf("Stat = %#x, %+v; want S_OK, a stream of 12345 bytes", hr, st)
	}
	release(s)
	if unsafe.Sizeof(statstg{}) != 80 {
		t.Errorf("STATSTG takes %d bytes, want 80", unsafe.Sizeof(statstg{}))
	}
}

// TestBodyStreamReleaseStopsHandler releases the stream while the handler
// waits on a full pipe, as WebView2 does when the page no longer wants the
// response: the handler's write fails and its context ends.
func TestBodyStreamReleaseStopsHandler(t *testing.T) {
	comInit(t)
	full := make(chan struct{})
	pipe := schemestream.NewPipe(streamLimit, func() { close(full) })
	var canceled atomic.Bool
	s := newBodyStream(pipe, -1, func() { canceled.Store(true) })
	wrote := make(chan error, 1)
	go func() { wrote <- writeChunks(pipe, streamBody(8<<20)) }()
	<-full
	// Read a little, as WebView2 might before the page aborts.
	buf := make([]byte, 64<<10)
	var n uint32
	if hr := comCall(s, 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n))); hr != sOK || n != uint32(len(buf)) {
		t.Fatalf("Read = %#x, %d bytes", hr, n)
	}
	release(s)
	select {
	case err := <-wrote:
		if !errors.Is(err, schemestream.ErrReleased) {
			t.Errorf("the handler's write after the release: %v, want ErrReleased", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the handler still waits after the stream was released")
	}
	if !canceled.Load() {
		t.Error("the release did not cancel the context")
	}
}

// TestBodyStreamFail breaks a body off: WebView2's read fails after it
// read what came before.
func TestBodyStreamFail(t *testing.T) {
	comInit(t)
	pipe := schemestream.NewPipe(streamLimit, nil)
	s := newBodyStream(pipe, -1, nil)
	defer release(s)
	pipe.Write([]byte("abc"))
	pipe.Fail(errors.New("the handler panicked"))
	buf := make([]byte, 10)
	var n uint32
	if hr := comCall(s, 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n))); hr != sOK || n != 3 {
		t.Fatalf("Read = %#x, %d bytes; want S_OK, 3", hr, n)
	}
	if hr := comCall(s, 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n))); !failed(hr) || n != 0 {
		t.Fatalf("Read after the failure = %#x, %d bytes; want a failure", hr, n)
	}
}

// TestBodyStreamUIThreadRead checks that a Read on the UI thread waits
// for the handler only so long, breaking the response off then.
func TestBodyStreamUIThreadRead(t *testing.T) {
	comInit(t)
	old := onUIThread
	onUIThread = func() bool { return true }
	defer func() { onUIThread = old }()
	pipe := schemestream.NewPipe(streamLimit, nil)
	var canceled atomic.Bool
	s := newBodyStream(pipe, -1, func() { canceled.Store(true) })
	defer release(s)
	buf := make([]byte, 10)
	var n uint32
	start := time.Now()
	hr := comCall(s, 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n)))
	if d := time.Since(start); d > uiReadWait+2*time.Second {
		t.Errorf("Read on the UI thread waited %v", d)
	}
	if !failed(hr) || n != 0 {
		t.Errorf("Read on the UI thread with nothing written = %#x, %d bytes; want a failure", hr, n)
	}
	if !canceled.Load() {
		t.Error("the context was not canceled")
	}
	if err := pipe.Write([]byte("late")); err == nil {
		t.Error("the handler's write succeeded after the response broke off")
	}
}
