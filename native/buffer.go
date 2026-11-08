package native

import (
	"runtime"
	"unsafe"
)

// Buffer is memory that Go and the implementation both use, which crosses
// without a copy: pass one to have the implementation fill it, as
// io.Reader fills a slice, or range over the buffers a stream yields. In
// Swift it is a NativeBuffer, whose bytes are the same memory.
type Buffer struct {
	p uintptr
	n int
	// keep is the Go memory of a buffer where nothing native holds it.
	keep []byte
}

// NewBuffer makes a buffer of n bytes, zeroed.
func NewBuffer(n int) *Buffer {
	if n < 0 {
		panic("native: NewBuffer with a negative size")
	}
	p := allocate(n)
	if p == 0 {
		b := make([]byte, n)
		return &Buffer{keep: b, n: n, p: bytesPointer(b)}
	}
	b := &Buffer{p: p, n: n}
	clear(b.Bytes())
	runtime.AddCleanup(b, func(p uintptr) { deallocate(p) }, p)
	return b
}

// Bytes returns the buffer's contents. A buffer a stream yields is lent
// until the loop asks for the next value: copy what you keep, as with
// bufio.Scanner.Bytes.
func (b *Buffer) Bytes() []byte {
	if b == nil || b.n == 0 {
		return nil
	}
	if b.keep != nil {
		return b.keep
	}
	return unsafe.Slice(bytesAt(b.p), b.n)
}

// Len returns the size of the buffer.
func (b *Buffer) Len() int {
	if b == nil {
		return 0
	}
	return b.n
}

// bytesAt converts the address of native memory, which Go's collector
// does not manage, to a pointer.
func bytesAt(p uintptr) *byte { return *(**byte)(unsafe.Pointer(&p)) }
