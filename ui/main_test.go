package ui

import (
	"flag"
	"os"
	"testing"
)

// TestMain has the tests' frames check what the engine keeps to do less
// (checkFrames), and the benchmarks' frames cost what an app's do.
func TestMain(m *testing.M) {
	flag.Parse()
	if f := flag.Lookup("test.bench"); f == nil || f.Value.String() == "" {
		checkFrames = true
	}
	os.Exit(m.Run())
}
