package native

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"time"
	"unsafe"
)

// Values cross between Go and the implementation in a binary encoding,
// in the order of the declarations, which the generated code of both
// sides writes and reads: every integer as 8 bytes, little-endian, two's
// complement; floats as float64; a bool as one byte; strings and []byte
// as their length then their bytes; slices and maps as their length then
// their elements (a map's as key, value); a pointer as a bool then the
// value if any; a struct as its fields; an enum as its value; a time as
// its Unix seconds and nanoseconds; a duration as nanoseconds; a buffer
// as its address and size; a texture, an object, a window, a view or a
// callback as its handle.

// Encoder writes the values of a call, for generated code.
type Encoder struct{ b []byte }

var encoders = sync.Pool{New: func() any { return &Encoder{b: make([]byte, 0, 128)} }}

// NewEncoder returns an empty encoder, which the call it is given to
// releases.
func NewEncoder() *Encoder {
	e := encoders.Get().(*Encoder)
	e.b = e.b[:0]
	return e
}

// release puts the encoder back for another call.
func (e *Encoder) release() {
	if e != nil && cap(e.b) <= 64<<10 {
		encoders.Put(e)
	}
}

func (e *Encoder) bytes() []byte {
	if e == nil {
		return nil
	}
	return e.b
}

func (e *Encoder) Bool(v bool) {
	var b byte
	if v {
		b = 1
	}
	e.b = append(e.b, b)
}

func (e *Encoder) Int(v int64)     { e.b = binary.LittleEndian.AppendUint64(e.b, uint64(v)) }
func (e *Encoder) Uint(v uint64)   { e.b = binary.LittleEndian.AppendUint64(e.b, v) }
func (e *Encoder) Float(v float64) { e.Uint(math.Float64bits(v)) }

// Len writes the length of a slice or a map, whose elements follow.
func (e *Encoder) Len(n int) { e.Int(int64(n)) }

func (e *Encoder) String(s string) {
	e.Len(len(s))
	e.b = append(e.b, s...)
}

func (e *Encoder) Bytes(b []byte) {
	e.Len(len(b))
	e.b = append(e.b, b...)
}

func (e *Encoder) Time(t time.Time) {
	e.Int(t.Unix())
	e.Int(int64(t.Nanosecond()))
}

func (e *Encoder) Duration(d time.Duration) { e.Int(int64(d)) }

// Rect writes a ui.Rect.
func (e *Encoder) Rect(x, y, w, h float32) {
	e.Float(float64(x))
	e.Float(float64(y))
	e.Float(float64(w))
	e.Float(float64(h))
}

// Buffer writes a buffer's address and size, which the implementation
// uses without a copy.
func (e *Encoder) Buffer(b *Buffer) {
	if b == nil {
		e.Uint(0)
		e.Uint(0)
		return
	}
	e.Uint(uint64(b.p))
	e.Uint(uint64(b.n))
}

func (e *Encoder) Texture(t *Texture) {
	if t == nil {
		e.Uint(0)
		return
	}
	e.Uint(t.ID())
}

var errShort = errors.New("native: a value from the implementation ends early")

// Decoder reads the values the implementation returned, for generated
// code. A value that does not decode reads as its zero, and Close and Err
// report it.
type Decoder struct {
	b   []byte
	err error
	// block is the memory the implementation gave the values in, freed
	// with Close.
	block uintptr
}

var decoders = sync.Pool{New: func() any { return new(Decoder) }}

func newDecoder(b []byte, block uintptr) *Decoder {
	d := decoders.Get().(*Decoder)
	d.b, d.err, d.block = b, nil, block
	return d
}

// Close lets go of what the decoder read from, and reports whether every
// value decoded.
func (d *Decoder) Close() error {
	if d == nil {
		return nil
	}
	err := d.err
	if d.block != 0 {
		freeBlock(d.block)
	}
	d.b, d.err, d.block = nil, nil, 0
	decoders.Put(d)
	return err
}

// Err reports whether a value failed to decode.
func (d *Decoder) Err() error { return d.err }

func (d *Decoder) take(n int) []byte {
	if n < 0 || n > len(d.b) {
		if d.err == nil {
			d.err = errShort
		}
		d.b = nil
		return nil
	}
	v := d.b[:n:n]
	d.b = d.b[n:]
	return v
}

func (d *Decoder) Bool() bool {
	b := d.take(1)
	return len(b) == 1 && b[0] != 0
}

func (d *Decoder) Uint() uint64 {
	b := d.take(8)
	if len(b) < 8 {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

func (d *Decoder) Int() int64     { return int64(d.Uint()) }
func (d *Decoder) Float() float64 { return math.Float64frombits(d.Uint()) }

// Len reads the length of a slice or a map. It is no more than the bytes
// left, as every element takes one at least (a struct without fields
// crosses as a zero byte), so that a value that does not decode makes
// nothing large.
func (d *Decoder) Len() int {
	n := d.Int()
	if n < 0 || n > int64(len(d.b)) {
		d.take(-1)
		return 0
	}
	return int(n)
}

func (d *Decoder) String() string {
	n := d.Int()
	if n > int64(len(d.b)) {
		d.take(-1)
		return ""
	}
	return string(d.take(int(n)))
}

// Bytes reads a []byte, nil when empty.
func (d *Decoder) Bytes() []byte {
	n := d.Int()
	if n > int64(len(d.b)) {
		d.take(-1)
		return nil
	}
	b := d.take(int(n))
	if len(b) == 0 {
		return nil
	}
	return append([]byte(nil), b...)
}

// Time reads a time, in UTC.
func (d *Decoder) Time() time.Time {
	s, ns := d.Int(), d.Int()
	return time.Unix(s, ns).UTC()
}

func (d *Decoder) Duration() time.Duration { return time.Duration(d.Int()) }

// Buffer reads a buffer the implementation lends.
func (d *Decoder) Buffer() *Buffer {
	p, n := d.Uint(), d.Uint()
	return &Buffer{p: uintptr(p), n: int(n)}
}

// bytesOf returns the n bytes at p, memory of the platform's.
func bytesOf(p uintptr, n int) []byte {
	if n <= 0 {
		return nil
	}
	return unsafe.Slice(bytesAt(p), n)
}
