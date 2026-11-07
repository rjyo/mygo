package native

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// lastModule numbers the modules as they load.
var lastModule atomic.Uint64

// library is the loaded implementation of a module.
type library struct {
	handle uintptr
	mu     sync.Mutex
	syms   map[string]uintptr
}

var libc struct {
	once     sync.Once
	malloc   uintptr
	free     uintptr
	mainNP   uintptr
	callback uintptr
	err      error
}

func loadLibc() error {
	libc.once.Do(func() {
		h, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			libc.err = err
			return
		}
		for name, p := range map[string]*uintptr{"malloc": &libc.malloc, "free": &libc.free, "pthread_main_np": &libc.mainNP} {
			if *p, err = purego.Dlsym(h, name); err != nil {
				libc.err = err
				return
			}
		}
		// One callback for every module, created once: purego's
		// callbacks are never freed.
		libc.callback = purego.NewCallback(func(kind, id uint64, status int64, data, n uintptr) uintptr {
			var in []byte
			if n > 0 {
				in = append([]byte(nil), unsafe.Slice(bytesAt(data), n)...)
			}
			st, out, ok := handle(kind, id, status, in)
			if !ok {
				return 0
			}
			return newBlock(st, out)
		})
	})
	return libc.err
}

func isMainThread() bool {
	if loadLibc() != nil {
		return false
	}
	r, _, _ := purego.SyscallN(libc.mainNP)
	return r != 0
}

// newBlock copies a result into memory the platform frees: its status, its
// length, then its bytes.
func newBlock(status int64, data []byte) uintptr {
	p, _, _ := purego.SyscallN(libc.malloc, uintptr(16+len(data)))
	if p == 0 {
		return 0
	}
	b := unsafe.Slice(bytesAt(p), 16+len(data))
	binary.LittleEndian.PutUint64(b, uint64(status))
	binary.LittleEndian.PutUint64(b[8:], uint64(len(data)))
	copy(b[16:], data)
	return p
}

// readBlock reads and frees a block the platform made.
func readBlock(p uintptr) reply {
	if p == 0 {
		return reply{}
	}
	head := unsafe.Slice(bytesAt(p), 16)
	r := reply{status: int64(binary.LittleEndian.Uint64(head))}
	if n := binary.LittleEndian.Uint64(head[8:]); n > 0 {
		r.data = append([]byte(nil), unsafe.Slice(bytesAt(p+16), n)...)
	}
	purego.SyscallN(libc.free, p)
	return r
}

func (m *Module) load() (*library, error) {
	m.once.Do(func() { m.lib, m.err = m.open() })
	return m.lib, m.err
}

func (m *Module) open() (*library, error) {
	if m.MinMacOS != "" && olderMacOS(m.MinMacOS) {
		return nil, errors.ErrUnsupported
	}
	if err := loadLibc(); err != nil {
		return nil, err
	}
	// The modules it uses first, whose libraries it links to: dyld then
	// finds them loaded, by their install names, wherever they are.
	for _, name := range m.Deps {
		dep := registered(name)
		if dep == nil {
			return nil, fmt.Errorf("native: %s uses the native module %s, which the program does not link", m.Name, name)
		}
		if _, err := dep.load(); err != nil {
			return nil, err
		}
	}
	path, err := m.find()
	if err != nil {
		return nil, err
	}
	h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("native: loading %s: %w", path, err)
	}
	lib := &library{handle: h, syms: map[string]uintptr{}}
	init, err := lib.sym(m, "init")
	if err != nil {
		return nil, err
	}
	// The module's index makes its continuations' ids unique.
	idx := lastModule.Add(1)
	modules.Store(idx, m)
	purego.SyscallN(init, libc.callback, uintptr(idx<<contBits))
	return lib, nil
}

// LibraryName is the file of a module's implementation on macOS.
func LibraryName(module string) string { return "lib" + module + ".dylib" }

// BuildDir is where `mygo generate` puts the implementation of the module
// in dir for go test and go run.
func BuildDir(dir string) string { return filepath.Join(dir, "apple", ".build", "mygo") }

// find returns the path of the implementation: in the app's bundle, in
// MYGO_NATIVE_PATH, or where `mygo generate` built it.
func (m *Module) find() (string, error) {
	name := LibraryName(m.Name)
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "..", "Frameworks", name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	for _, dir := range filepath.SplitList(os.Getenv("MYGO_NATIVE_PATH")) {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if m.Dir == "" {
		return "", fmt.Errorf("native: %s is not in the app's Frameworks: build the app with mygo build or mygo dev", name)
	}
	p := filepath.Join(BuildDir(m.Dir), name)
	st, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("native: the implementation of %s is not built: run `mygo generate` in %s", m.Name, m.Dir)
	}
	if newer := newestSource(filepath.Join(m.Dir, "apple")); newer.After(st.ModTime()) {
		return "", fmt.Errorf("native: the implementation of %s is older than its sources: run `mygo generate` in %s", m.Name, m.Dir)
	}
	return p, nil
}

// newestSource returns when a source of a Swift package last changed.
func newestSource(dir string) time.Time {
	var t time.Time
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".swift") {
			if info, err := d.Info(); err == nil && info.ModTime().After(t) {
				t = info.ModTime()
			}
		}
		return nil
	})
	return t
}

// olderMacOS reports whether the system is older than version, as "12" or
// "13.3".
func olderMacOS(version string) bool {
	have, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return false
	}
	return compareVersions(have, version) < 0
}

func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func (l *library) sym(m *Module, name string) (uintptr, error) {
	full := "mygo_" + m.Symbol + "_" + name
	l.mu.Lock()
	defer l.mu.Unlock()
	if p, ok := l.syms[full]; ok {
		return p, nil
	}
	p, err := purego.Dlsym(l.handle, full)
	if err != nil {
		return 0, fmt.Errorf("native: %s has no %s: regenerate it with mygo generate", LibraryName(m.Name), full)
	}
	l.syms[full] = p
	return p, nil
}

func (l *library) invoke(m *Module, name string, h uint64, args []byte, call uint64) reply {
	fn, err := l.sym(m, name)
	if err != nil {
		return reply{statusError, encodeError(err)}
	}
	p, _, _ := purego.SyscallN(fn, uintptr(h), bytesPointer(args), uintptr(len(args)), uintptr(call))
	runtime.KeepAlive(args)
	if p == 0 {
		return reply{status: statusOK}
	}
	return readBlock(p)
}

func (l *library) simple(m *Module, name string, args ...uintptr) {
	if fn, err := l.sym(m, name); err == nil {
		purego.SyscallN(fn, args...)
	}
}

func (l *library) release(m *Module, h uint64)   { l.simple(m, "release", uintptr(h)) }
func (l *library) cancel(m *Module, call uint64) { l.simple(m, "cancel", uintptr(call)) }
func (l *library) next(m *Module, call uint64)   { l.simple(m, "next", uintptr(call)) }

func (l *library) resume(m *Module, cont uint64, status int64, data []byte) {
	l.simple(m, "resume", uintptr(cont), uintptr(status), bytesPointer(data), uintptr(len(data)))
	runtime.KeepAlive(data)
}

func allocate(n int) uintptr {
	if loadLibc() != nil {
		return 0
	}
	p, _, _ := purego.SyscallN(libc.malloc, uintptr(max(n, 1)))
	return p
}

func deallocate(p uintptr) { purego.SyscallN(libc.free, p) }

// releaseSurface lets go of a surface presented to a texture that is gone.
func releaseSurface(s uintptr) {
	if s == 0 {
		return
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	if release, err := purego.Dlsym(cf, "CFRelease"); err == nil {
		purego.SyscallN(release, s)
	}
}
