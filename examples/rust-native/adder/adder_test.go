package adder

import (
	"os"
	"testing"
)

// TestAdder calls the crate, which mygo native build -target host or mygo
// dev builds; without it, the test is skipped.
func TestAdder(t *testing.T) {
	if err := Load(); err != nil {
		if os.Getenv("MYGO_NATIVE_TESTS") != "" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	if got := Add(40, 2); got != 42 {
		t.Errorf("Add = %d", got)
	}
	if got := Fibonacci(90); got != 2880067194370816120 {
		t.Errorf("Fibonacci = %d", got)
	}
	if got := CountVowels("MyGo calls Rust"); got != 3 {
		t.Errorf("CountVowels = %d", got)
	}
	if got := Greeting("Go"); got != "Hello, Go!" {
		t.Errorf("Greeting = %q", got)
	}
	if got := Version(); got != "adder 0.1.0" {
		t.Errorf("Version = %q", got)
	}
}
