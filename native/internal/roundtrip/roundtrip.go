// Package roundtrip is a native module for the tests of the runtime and
// the generator: every kind of value that crosses, sent to the platform
// and back, with calls of each shape. It is implemented three times, in
// swift/ (with Foundation only, so that it builds on Linux too), rust/ and
// zig/, each of which the tests run.
package roundtrip

import (
	"context"
	"io"
	"iter"
	"time"

	"github.com/egoist/mygo/native"
	"github.com/egoist/mygo/ui"
)

// Echo returns what it is given.
//
//mygo:native
type Echo interface {
	// Scalars returns its arguments, one more each.
	Scalars(i int, i8 int8, u16 uint16, i32 int32, u64 uint64, f32 float32, f float64, b bool) Scalars
	// Add is a plain sum, the cheapest call.
	Add(a, b int) int
	// Text returns s twice, and b reversed.
	Text(s string, b []byte) Texts
	// Record returns r with its counts doubled.
	Record(r Record) (Record, error)
	// Lists returns the lengths of the words, and a map of each to its
	// length.
	Lists(words []string, nested [][]int) (map[string]int, error)
	// Maybe returns p's value plus one, or nil for nil.
	Maybe(p *int) *int
	// Times returns t moved by d, and d doubled.
	Times(t time.Time, d time.Duration) (Moment, error)
	// Kind returns the next kind, and the next level.
	Kind(k Kind, l Level) (Kinds, error)
	// Fail throws the error named, or nothing for "".
	Fail(name string) error
	// Wait waits d, or until ctx is done, then returns s.
	Wait(ctx context.Context, d time.Duration, s string) (string, error)
	// Count yields 0 to n-1, failing at fail when it is not -1.
	Count(ctx context.Context, n, fail int) iter.Seq2[int, error]
	// Records yields n records.
	Records(ctx context.Context, n int) iter.Seq2[Record, error]
	// Counter makes a counter starting at n.
	Counter(n int) (Counter, error)
	// Each calls f with each word and its index, and returns how many.
	Each(words []string, f func(word string, i int)) int
	// Ask asks f for the double of each number, and returns their sum.
	Ask(ns []int, f func(n int) (int, error)) (int, error)
	// Ticks calls f every interval until closed.
	Ticks(interval time.Duration, f func(n int)) io.Closer
	// Use asks s for a score of each word, and returns their sum.
	Use(ctx context.Context, s Scorer, words []string) (float64, error)
	// Fill writes b's index into each byte of buf, xor x.
	Fill(buf *native.Buffer, x int) (int, error)
	// Mix sums numbers of each kind, a call of numbers only.
	Mix(f float32, d float64, b bool, i int8) float64
	// Box returns r moved by its size.
	Box(r ui.Rect) (ui.Rect, error)
	// Nothing returns how many structs without fields it was given.
	Nothing(e []Empty) int
	// OnMain runs on the main thread, which go test does not run: the
	// tests only build it.
	//
	//mygo:mainthread
	OnMain(r Record, f func(in Inner)) (Texts, error)
}

// Empty has no fields.
type Empty struct{}

// Counter counts.
type Counter interface {
	io.Closer
	// Next returns the count, then adds one.
	Next() int
	// Same returns another counter of the same count.
	Same() Counter
}

// Scorer is implemented by Go.
type Scorer interface {
	// Score scores a word.
	Score(word string) float64
	// Explain explains a score, which may wait.
	Explain(ctx context.Context, word string) (string, error)
}

// Scalars are the scalars back.
type Scalars struct {
	I   int
	I8  int8
	U16 uint16
	I32 int32
	U64 uint64
	F32 float32
	F   float64
	B   bool
}

// Texts are a string and bytes back.
type Texts struct {
	S string
	B []byte
}

// Record is a struct with every kind of field.
type Record struct {
	Name   string
	Counts []int
	Tags   map[string]string
	Inner  Inner
	Next   *Inner
	Kind   Kind
	Data   []byte
}

// Inner is nested.
type Inner struct {
	X, Y float64
}

// Moment is a time and a duration back.
type Moment struct {
	At    time.Time
	Twice time.Duration
}

// Kinds are an enum of each kind back.
type Kinds struct {
	Kind  Kind
	Level Level
}

// Kind is a string enum.
type Kind string

const (
	KindA Kind = "a"
	KindB Kind = "b"
	KindC Kind = "c"
)

// Level is an integer enum.
type Level int

const (
	Low  Level = 1
	High Level = 2
)

var (
	// ErrNamed is the error Fail throws for "named".
	ErrNamed = native.NewError("named")
)
