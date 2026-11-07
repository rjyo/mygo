// Package clash names its parameters as the generated code names its own
// identifiers and packages, which must not clash.
package clash

import (
	"context"
	"io"
	"iter"

	"github.com/egoist/mygo/native"
)

// A takes parameters named as the generated code's identifiers.
//
//mygo:native
type A interface {
	M(x, err, r, h, a, m, v, o, id, ctx, p0, f0, g0 int) error
	P(native, runtime, context, fmt, filepath, iter, io int) (int, error)
	S(ctx context.Context, x int, a func(err string, x int)) iter.Seq2[int, error]
	C(err int, x func(r int) (int, error)) io.Closer
	B(native *native.Buffer, x int) (int, error)
	G(ctx context.Context, id G, x int) error
	D(ctx context.Context, h B) (B, error)
}

// B is an object the platform returns.
type B interface {
	N(x, err int) int
}

// G is an interface Go implements.
type G interface {
	N(ctx context.Context, id int, self string) (int, error)
	Q(x, err, a, m, v int) string
}
