package schemestream

import (
	"bytes"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

// readAll reads p as WebView2 reads a response stream: in blocks, until a
// read comes back short.
func readAll(t *testing.T, p *Pipe, block int) ([]byte, error) {
	t.Helper()
	var out []byte
	buf := make([]byte, block)
	for {
		n, err := p.Read(buf, 0)
		out = append(out, buf[:n]...)
		if err != nil {
			return out, err
		}
	}
}

func body(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 7)
	}
	return b
}

func TestPipeStreams(t *testing.T) {
	const limit, chunk = 64 << 10, 16 << 10
	want := body(1<<20 + 123)
	var fulls atomic.Int32
	p := NewPipe(limit, func() { fulls.Add(1) })
	held := make(chan int, 1024)
	go func() {
		for b := want; len(b) > 0; {
			k := min(chunk, len(b))
			if err := p.Write(b[:k]); err != nil {
				t.Errorf("Write: %v", err)
				return
			}
			b = b[k:]
			p.mu.Lock()
			held <- len(p.buf)
			p.mu.Unlock()
		}
		p.Close()
		close(held)
	}()
	got, err := readAll(t, p, 10_000)
	if err != io.EOF {
		t.Fatalf("the last Read: %v, want io.EOF", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("read %d bytes, not the %d written", len(got), len(want))
	}
	for n := range held {
		if n > limit+chunk {
			t.Errorf("the pipe held %d bytes, more than limit + a chunk (%d)", n, limit+chunk)
		}
	}
	if n := fulls.Load(); n != 1 {
		t.Errorf("onFull called %d times, want 1", n)
	}
}

func TestPipeWriteWaitsForReader(t *testing.T) {
	full := make(chan struct{})
	p := NewPipe(8, func() { close(full) })
	if err := p.Write(body(8)); err != nil {
		t.Fatal(err)
	}
	wrote := make(chan error, 1)
	go func() { wrote <- p.Write(body(4)) }()
	<-full
	select {
	case err := <-wrote:
		t.Fatalf("Write returned (%v) while the pipe was full", err)
	case <-time.After(50 * time.Millisecond):
	}
	buf := make([]byte, 6)
	if n, err := p.Read(buf, 0); n != 6 || err != nil {
		t.Fatalf("Read = %d, %v", n, err)
	}
	if err := <-wrote; err != nil {
		t.Fatalf("Write after a read: %v", err)
	}
}

func TestPipeReadWaitsForFullBlock(t *testing.T) {
	p := NewPipe(1<<20, nil)
	done := make(chan int, 1)
	go func() {
		n, _ := p.Read(make([]byte, 10), 0)
		done <- n
	}()
	p.Write(body(4))
	select {
	case n := <-done:
		t.Fatalf("Read returned %d bytes before the block was there", n)
	case <-time.After(30 * time.Millisecond):
	}
	p.Write(body(6))
	if n := <-done; n != 10 {
		t.Fatalf("Read = %d, want 10", n)
	}
}

// A Read asking more than the limit is not left waiting on a writer
// waiting on it.
func TestPipeReadLargerThanLimit(t *testing.T) {
	p := NewPipe(4, nil)
	go func() {
		for range 10 {
			if err := p.Write(body(3)); err != nil {
				return
			}
		}
		p.Close()
	}()
	got, err := readAll(t, p, 16)
	if err != io.EOF || len(got) != 30 {
		t.Fatalf("read %d bytes, %v; want 30, io.EOF", len(got), err)
	}
}

func TestPipeShortReadAtEnd(t *testing.T) {
	p := NewPipe(100, nil)
	p.Write([]byte("hello"))
	p.Close()
	buf := make([]byte, 3)
	if n, err := p.Read(buf, 0); n != 3 || err != nil {
		t.Fatalf("Read = %d, %v; want 3, nil", n, err)
	}
	if n, err := p.Read(buf, 0); n != 2 || err != io.EOF {
		t.Fatalf("Read = %d, %v; want 2, io.EOF", n, err)
	}
	if n, err := p.Read(buf, 0); n != 0 || err != io.EOF {
		t.Fatalf("Read at the end = %d, %v; want 0, io.EOF", n, err)
	}
	if err := p.Write([]byte("x")); err == nil {
		t.Error("Write after Close succeeded")
	}
}

func TestPipeReleaseFailsWrites(t *testing.T) {
	p := NewPipe(4, nil)
	p.Write(body(4))
	wrote := make(chan error, 1)
	go func() { wrote <- p.Write(body(4)) }() // waits: full
	time.Sleep(20 * time.Millisecond)
	p.Release()
	if err := <-wrote; !errors.Is(err, ErrReleased) {
		t.Fatalf("a waiting Write after Release: %v, want ErrReleased", err)
	}
	if err := p.Write(body(1)); !errors.Is(err, ErrReleased) {
		t.Fatalf("Write after Release: %v, want ErrReleased", err)
	}
}

func TestPipeFail(t *testing.T) {
	p := NewPipe(100, nil)
	p.Write([]byte("abc"))
	boom := errors.New("boom")
	done := make(chan error, 1)
	go func() {
		_, err := p.Read(make([]byte, 10), 0)
		done <- err
	}()
	time.Sleep(10 * time.Millisecond)
	p.Fail(boom)
	if err := <-done; err != boom {
		t.Fatalf("Read after Fail: %v, want boom", err)
	}
	if err := p.Write([]byte("x")); err != boom {
		t.Fatalf("Write after Fail: %v, want boom", err)
	}
}

func TestPipeReadTimeout(t *testing.T) {
	p := NewPipe(100, nil)
	start := time.Now()
	n, err := p.Read(make([]byte, 10), 20*time.Millisecond)
	if n != 0 || err != ErrTimeout {
		t.Fatalf("Read of an empty pipe = %d, %v; want 0, ErrTimeout", n, err)
	}
	if d := time.Since(start); d < 20*time.Millisecond {
		t.Fatalf("Read returned after %v, before the timeout", d)
	}
	p.Write([]byte("ab"))
	if n, err := p.Read(make([]byte, 10), 20*time.Millisecond); n != 2 || err != nil {
		t.Fatalf("Read with some data = %d, %v; want 2, nil", n, err)
	}
}

func TestPipeTake(t *testing.T) {
	p := NewPipe(100, nil)
	p.Write([]byte("ab"))
	p.Write([]byte("cd"))
	if got := string(p.Take()); got != "abcd" {
		t.Fatalf("Take = %q", got)
	}
	if got := p.Take(); len(got) != 0 {
		t.Fatalf("Take again = %q", got)
	}
}

func TestClampRange(t *testing.T) {
	const mb = 1 << 20
	for _, c := range []struct{ in, want string }{
		{"bytes=0-", "bytes=0-1048575"},
		{"bytes=318963712-", "bytes=318963712-320012287"},
		{"bytes=0-99", "bytes=0-99"},                                 // closed: as asked
		{"bytes=0-99999999", "bytes=0-99999999"},                     // closed, however long
		{"bytes=-500", "bytes=-500"},                                 // the last bytes
		{"bytes=0-,10-", "bytes=0-,10-"},                             // several ranges
		{"bytes=+5-", "bytes=+5-"},                                   // not a number
		{"bytes=x-", "bytes=x-"},                                     // not a number
		{"items=0-", "items=0-"},                                     // another unit
		{"", ""},                                                     // no range
		{"bytes=9223372036854775807-", "bytes=9223372036854775807-"}, // would overflow
	} {
		if got := ClampRange(c.in, mb); got != c.want {
			t.Errorf("ClampRange(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := ClampRange("bytes=0-", 0); got != "bytes=0-" {
		t.Errorf("ClampRange with n = 0: %q", got)
	}
}
