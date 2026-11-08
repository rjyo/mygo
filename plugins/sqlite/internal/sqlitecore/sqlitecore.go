// Package sqlitecore is the plugin's SQLite: a native module implemented
// in Zig, which compiles SQLite's amalgamation into the module's library
// for every platform. Package sqlite wraps it in its connections, which
// serialize and validate the calls; nothing else uses it.
package sqlitecore

import "io"

// Engine opens connections.
//
//mygo:native
type Engine interface {
	// Open opens path, or ":memory:", creating a missing file unless
	// readOnly, with extended result codes on. busyMillis is how long an
	// operation waits for another connection's lock.
	Open(path string, readOnly bool, busyMillis int) (Conn, error)
	// Version is SQLite's version, as 3.53.4.
	Version() string
}

// Conn is a connection. Its methods other than Interrupt are called one
// at a time.
type Conn interface {
	io.Closer
	// Run prepares sql, which must hold exactly one statement and no
	// transaction control or ATTACH, binds args to its parameters, and
	// steps it to the end, returning its rows when query is true.
	Run(sql string, args []Value, query bool) (Outcome, error)
	// Control runs a statement of transaction control, such as BEGIN
	// IMMEDIATE, COMMIT or ROLLBACK.
	Control(sql string) error
	// Autocommit reports whether the connection is outside a transaction.
	Autocommit() bool
	// Interrupt stops the statement running, and its wait for another
	// connection's lock, from any goroutine: the call running fails with
	// SQLite's SQLITE_INTERRUPT, or SQLITE_BUSY.
	Interrupt()
}

// Value is a value of SQLite: a parameter, or a column of a row.
type Value struct {
	Type Type
	Int  int64
	Real float64
	Text string
	Blob []byte
}

// Type is the type of a Value, by SQLite's fundamental datatype codes.
type Type int

const (
	Null    Type = 5
	Integer Type = 1
	Real    Type = 2
	Text    Type = 3
	Blob    Type = 4
)

// Outcome is what a statement did. Values holds the rows one after the
// other, each the length of Columns.
type Outcome struct {
	Changes      int64
	LastInsertID int64
	Columns      []string
	Values       []Value
}
