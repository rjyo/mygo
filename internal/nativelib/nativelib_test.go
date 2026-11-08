package nativelib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	ok := `{"libraries": [{"name": "adder", "build": {"cargo": "./rust/adder", "args": ["--features", "x"]}, "release": "https://example.com/r",
		"files": {"linux-amd64": {"name": "libadder.so", "url": "https://example.com/r/adder-linux-amd64.so", "sha256": "` + sum + `"}}}]}`
	m, err := Parse([]byte(ok))
	if err != nil {
		t.Fatal(err)
	}
	l, found := m.Lookup("adder")
	if tool, dir := l.Build.Tool(); !found || tool != "cargo" || dir != "./rust/adder" {
		t.Fatalf("Lookup = %+v, %v", l, found)
	}
	if l.FileName("linux", "amd64") != "libadder.so" || l.FileName("windows", "arm64") != "adder.dll" {
		t.Error("FileName")
	}
	for _, bad := range []string{
		`{"libraries": [{"name": ""}]}`,
		`{"libraries": [{"name": "../x"}]}`,
		`{"libraries": [{"name": "my lib"}]}`,
		`{"libraries": [{"name": ".hidden"}]}`,
		`{"libraries": [{"name": "x"}, {"name": "x"}]}`,
		`{"libraries": [{"name": "x", "build": {}}]}`,
		`{"libraries": [{"name": "x", "build": {"cargo": "a", "zig": "b"}}]}`,
		`{"libraries": [{"name": "x", "build": {"zig": "../elsewhere"}}]}`,
		`{"libraries": [{"name": "x", "build": {"zig": "/abs"}}]}`,
		`{"libraries": [{"name": "x", "files": {"linux-amd64": {"name": "..", "sha256": "` + sum + `"}}}]}`,
		`{"libraries": [{"name": "x", "files": {"linux-amd64": {"name": "x.so", "sha256": "` + strings.ToUpper(sum) + `"}}}]}`,
		`{"libraries": [{"name": "x", "files": {"linux-amd64": {"name": "x.so", "sha256": "../../etc"}}}]}`,
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%s) accepted it", bad)
		}
	}
}

func TestFileName(t *testing.T) {
	for _, c := range []struct{ name, goos, want string }{
		{"adder", "darwin", "libadder.dylib"},
		{"adder", "linux", "libadder.so"},
		{"adder", "windows", "adder.dll"},
		{"libghostty-vt", "darwin", "libghostty-vt.dylib"},
		{"libghostty-vt", "windows", "ghostty-vt.dll"},
		{"mygo-sqlite3", "linux", "libmygo-sqlite3.so"},
	} {
		if got := FileName(c.name, c.goos); got != c.want {
			t.Errorf("FileName(%q, %q) = %q, want %q", c.name, c.goos, got, c.want)
		}
	}
}

func TestMinMacOS(t *testing.T) {
	for _, c := range []struct{ app, lib, want string }{
		{"", "", "12.0"},
		{"", "13.0", "13.0"},
		{"14.0", "13.0", "14.0"},
		{"13.2", "13.10", "13.10"},
		{"15", "", "15"},
	} {
		if got := (&Build{MacOS: c.lib}).MinMacOS(c.app); got != c.want {
			t.Errorf("MinMacOS(%q) with %q = %q, want %q", c.app, c.lib, got, c.want)
		}
	}
	if _, err := Parse([]byte(`{"libraries": [{"name": "x", "build": {"zig": ".", "macOS": "13; rm"}}]}`)); err == nil {
		t.Error("accepted an invalid macOS version")
	}
}

func TestModuleRootRelative(t *testing.T) {
	// The package's directory, below the module's root, as a relative
	// path whose walk up passes the working directory.
	root, err := ModuleRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("ModuleRoot(\".\") = %s, which has no go.mod", root)
	}
}
