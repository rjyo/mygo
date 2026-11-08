// Package uses is a native module that uses another, roundtrip: its
// structs, enums and objects cross from one module to the other.
package uses

import "github.com/egoist/mygo/native/internal/roundtrip"

// Mixer takes roundtrip's values.
//
//mygo:native
type Mixer interface {
	// Pair returns p with its record's counts summed into N.
	Pair(p Pair) (Pair, error)
	// Kinds returns the kinds of the records.
	Kinds(rs []roundtrip.Record) []roundtrip.Kind
	// Next asks c, roundtrip's, for its next count.
	Next(c roundtrip.Counter) int
}

// Pair holds a record of roundtrip's.
type Pair struct {
	R     roundtrip.Record
	Inner *roundtrip.Inner
	N     int
}
