package ffi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/nativelib"
)

func TestSnakeCase(t *testing.T) {
	for in, want := range map[string]string{
		"Add":        "add",
		"AddNumbers": "add_numbers",
		"ParseURL":   "parse_url",
		"URLParse":   "url_parse",
		"Sha256Sum":  "sha256_sum",
		"Already_ok": "already_ok",
	} {
		if got := SnakeCase(in); got != want {
			t.Errorf("SnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnvVar(t *testing.T) {
	if got := EnvVar("libghostty-vt"); got != "MYGO_NATIVE_LIBGHOSTTY_VT" {
		t.Errorf("EnvVar = %q", got)
	}
}

// libc returns the C library of the system, which the binding tests call.
func libc(t *testing.T) string {
	if !supported {
		t.Skip("unsupported platform")
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		return "/usr/lib/libSystem.B.dylib" // in the shared cache: dlopen finds it
	case "windows":
		return filepath.Join(os.Getenv("SystemRoot"), "System32", "msvcrt.dll")
	case "linux":
		candidates, _ = filepath.Glob("/lib/*-linux-gnu/libc.so.6")
		more, _ := filepath.Glob("/usr/lib/*-linux-gnu/libc.so.6")
		candidates = append(candidates, append(more, "/lib64/libc.so.6", "/usr/lib/libc.so.6")...)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("no C library found")
	return ""
}

func TestBind(t *testing.T) {
	t.Setenv(EnvVar("c"), libc(t))
	lib := New("c", nil)
	var fns struct {
		Abs     func(int32) int32
		Strlen  func(string) uintptr `ffi:"strlen"`
		Missing func()               `ffi:"mygo_no_such_function,optional"`
		Skipped func()               `ffi:"-"`
		Other   int
	}
	if err := lib.Bind(&fns); err != nil {
		t.Fatal(err)
	}
	if got := fns.Abs(-7); got != 7 {
		t.Errorf("abs(-7) = %d", got)
	}
	if got := fns.Strlen("hello"); got != 5 {
		t.Errorf("strlen = %d", got)
	}
	if fns.Missing != nil || fns.Skipped != nil {
		t.Error("bound an optional missing function or a skipped one")
	}
	var labs func(int64) int64
	if err := lib.BindFunc(&labs, "labs"); err != nil || labs(-3) != 3 {
		t.Errorf("BindFunc: %v", err)
	}
	var missing struct{ MygoNoSuchFunction func() }
	if err := lib.Bind(&missing); err == nil {
		t.Error("bound a missing function")
	}
	if err := lib.Bind(fns); err == nil {
		t.Error("bound a struct that is not a pointer")
	}
}

// TestPath finds a library where the CLI builds it and where it downloads
// published ones, checking their SHA-256.
func TestPath(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)
	t.Setenv("LocalAppData", cache)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(root, "sub", "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("a library")
	sum := sha256.Sum256(content)
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads++
		w.Write(content)
	}))
	defer srv.Close()
	target := runtime.GOOS + "-" + runtime.GOARCH
	file := nativelib.FileName("x", runtime.GOOS)
	manifest := fmt.Appendf(nil, `{"libraries": [{"name": "x", "build": {"cargo": "./rust"}, "files": {%q: {"name": %q, "url": %q, "sha256": %q}}}]}`,
		target, file, srv.URL, hex.EncodeToString(sum[:]))

	// The published build, downloaded once.
	t.Chdir(pkg)
	path, err := New("x", manifest).Path()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != string(content) || downloads != 1 {
		t.Fatalf("%s holds %q after %d downloads", path, b, downloads)
	}
	if _, err := New("x", manifest).Path(); err != nil || downloads != 1 {
		t.Fatalf("downloaded again: %v", err)
	}

	// The CLI's build in the module, found from a package below its root.
	local := filepath.Join(nativelib.LocalDir(root, runtime.GOOS, runtime.GOARCH), file)
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("built"), 0o755); err != nil {
		t.Fatal(err)
	}
	if path, err := New("x", manifest).Path(); err != nil || path != local {
		t.Fatalf("Path = %s, %v; want %s", path, err, local)
	}

	// Without a build or a manifest naming one.
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	if _, err := New("y", nil).Path(); err == nil {
		t.Error("found a library that is nowhere")
	}
	if _, err := New("y", manifest).Path(); err == nil {
		t.Error("found a library the manifest does not name")
	}
	unbuilt := []byte(`{"libraries": [{"name": "z", "build": {"zig": "."}}]}`)
	if err := New("z", unbuilt).Load(); err == nil || errors.Is(err, errors.ErrUnsupported) && supported {
		t.Errorf("Load = %v", err)
	}
}

// TestBindConcurrently binds and calls from many goroutines at once, as
// functions that call Bind before each use do; -race checks it.
func TestBindConcurrently(t *testing.T) {
	t.Setenv(EnvVar("c"), libc(t))
	lib := New("c", nil)
	var fns struct{ Abs func(int32) int32 }
	abs := func(x int32) int32 {
		if err := lib.Bind(&fns); err != nil {
			t.Error(err)
			return 0
		}
		return fns.Abs(x)
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			if got := abs(int32(-i)); got != int32(i) {
				t.Errorf("abs(%d) = %d", -i, got)
			}
		})
	}
	wg.Wait()
}

func TestBindErrors(t *testing.T) {
	t.Setenv(EnvVar("c"), libc(t))
	var typo struct {
		Abs func(int32) int32 `ffi:"abs,optinal"`
	}
	if err := New("c", nil).Bind(&typo); err == nil || !strings.Contains(err.Error(), "unknown option") {
		t.Errorf("an unknown option: %v", err)
	}
	// An invalid manifest is reported even when the environment names the
	// library.
	if err := New("c", []byte(`{"libraries": [{"name": "c", "build": {}}]}`)).Load(); err == nil {
		t.Error("loaded with an invalid manifest")
	}
	if err := New("d", []byte(`{"libraries": [{"name": "c"}]}`)).Load(); err == nil {
		t.Error("loaded a library the manifest does not name")
	}
}

func TestGoString(t *testing.T) {
	b := []byte("hello\x00world")
	if got := GoString(unsafe.Pointer(&b[0])); got != "hello" {
		t.Errorf("GoString = %q", got)
	}
	if GoString(nil) != "" {
		t.Error("GoString(nil)")
	}
}
