// Package adder binds a Rust crate, built by the CLI from ./rust as
// mygo-plugin.json says, with package ffi.
package adder

import (
	_ "embed"
	"unsafe"

	"github.com/egoist/mygo/ffi"
)

// Builds the crate for go run and go test: go generate ./examples/rust-native/...
//
//go:generate go run ../../../cmd/mygo native build -target host .

//go:embed mygo-plugin.json
var manifest []byte

var lib = ffi.New("adder", manifest)

// fns are the crate's functions, bound by their names in snake case.
var fns struct {
	Add         func(a, b int32) int32
	Fibonacci   func(n uint32) uint64
	CountVowels func(s string) uint32
	Version     func() string `ffi:"adder_version"`
	Greeting    func(name string) unsafe.Pointer
	FreeString  func(s unsafe.Pointer)
}

// Load loads the library and binds its functions; the others need it.
func Load() error { return lib.Bind(&fns) }

// Add returns a + b, wrapping around.
func Add(a, b int32) int32 { return fns.Add(a, b) }

// Fibonacci returns the nth Fibonacci number.
func Fibonacci(n uint32) uint64 { return fns.Fibonacci(n) }

// CountVowels returns how many ASCII vowels s has.
func CountVowels(s string) uint32 { return fns.CountVowels(s) }

// Version returns the crate's name and version.
func Version() string { return fns.Version() }

// Greeting returns a greeting of name, which the crate allocates and frees.
func Greeting(name string) string {
	p := fns.Greeting(name)
	defer fns.FreeString(p)
	return ffi.GoString(p)
}
