package texture

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo/internal/scene"
)

// newSurface makes a w×h 32-bit BGRA IOSurface filled with b, g, r, a.
func newSurface(t *testing.T, w, h int, bgra [4]byte) uintptr {
	t.Helper()
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	io, err := purego.Dlopen("/System/Library/Frameworks/IOSurface.framework/IOSurface", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	sym := func(lib uintptr, name string) uintptr {
		p, err := purego.Dlsym(lib, name)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	const kCFNumberSInt32Type = 3
	number := func(v int32) uintptr {
		n, _, _ := purego.SyscallN(sym(cf, "CFNumberCreate"), 0, kCFNumberSInt32Type, uintptr(unsafe.Pointer(&v)))
		return n
	}
	key := func(name string) uintptr {
		p := sym(io, name) // the address of a global CFStringRef
		return **(**uintptr)(unsafe.Pointer(&p))
	}
	keys := []uintptr{key("kIOSurfaceWidth"), key("kIOSurfaceHeight"), key("kIOSurfaceBytesPerElement"), key("kIOSurfacePixelFormat")}
	values := []uintptr{number(int32(w)), number(int32(h)), number(4), number(pixelFormatBGRA)}
	dict, _, _ := purego.SyscallN(sym(cf, "CFDictionaryCreate"), 0, uintptr(unsafe.Pointer(&keys[0])), uintptr(unsafe.Pointer(&values[0])), uintptr(len(keys)),
		sym(cf, "kCFTypeDictionaryKeyCallBacks"), sym(cf, "kCFTypeDictionaryValueCallBacks"))
	runtime.KeepAlive(keys)
	runtime.KeepAlive(values)
	s, _, _ := purego.SyscallN(sym(io, "IOSurfaceCreate"), dict)
	if s == 0 {
		t.Fatal("IOSurfaceCreate failed")
	}
	purego.SyscallN(iosurfaceLock(), s, 0, 0)
	base, _, _ := purego.SyscallN(iosurface.base, s)
	stride, _, _ := purego.SyscallN(iosurface.bytesPerRow, s)
	pix := unsafe.Slice(*(**byte)(unsafe.Pointer(&base)), int(stride)*h)
	for y := range h {
		for x := range w {
			copy(pix[y*int(stride)+4*x:], bgra[:])
		}
	}
	purego.SyscallN(iosurface.unlock, s, 0, 0)
	return s
}

func iosurfaceLock() uintptr {
	loadIOSurface()
	return iosurface.lock
}

func useCount(t *testing.T, s uintptr) int {
	io, _ := purego.Dlopen("/System/Library/Frameworks/IOSurface.framework/IOSurface", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	f, err := purego.Dlsym(io, "IOSurfaceGetUseCount")
	if err != nil {
		t.Fatal(err)
	}
	n, _, _ := purego.SyscallN(f, s)
	return int(int32(n))
}

func TestPresentAndFrame(t *testing.T) {
	tex := New()
	key := new(int)
	if img := tex.Frame(key, func() {}); img != nil {
		t.Fatal("a frame before any present")
	}
	asked := 0
	tex.Frame(key, func() { asked++ })

	// Each present takes a reference, which the producer gives it.
	red := newSurface(t, 3, 2, [4]byte{0, 0, 255, 255})
	purego.SyscallN(iosurface.retain, red) // ours, to read its use count after
	tex.Present(red)
	if asked != 1 {
		t.Fatalf("the present asked for %d frames", asked)
	}
	if w, h := tex.Size(); w != 3 || h != 2 {
		t.Fatalf("Size = %v×%v", w, h)
	}
	img := tex.Frame(key, func() { asked++ })
	if img == nil || img.W != 3 || img.H != 2 || img.GPU == nil {
		t.Fatalf("frame %+v", img)
	}
	if mtl, err := purego.Dlopen("/System/Library/Frameworks/Metal.framework/Metal", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
		if create, err := purego.Dlsym(mtl, "MTLCreateSystemDefaultDevice"); err == nil {
			if device, _, _ := purego.SyscallN(create); device != 0 {
				tex := objc.ID(img.GPU.GPUTexture(scene.Metal, device))
				if tex == 0 || tex.Send(objc.RegisterName("width")) != 3 || tex.Send(objc.RegisterName("iosurface")) != objc.ID(red) {
					t.Errorf("the Metal texture %x does not show the surface", tex)
				}
				tex.Send(objc.RegisterName("release"))
			}
		}
	}
	if px := img.Pixels(); len(px) != 24 || px[0] != 255 || px[1] != 0 || px[2] != 0 || px[3] != 255 {
		t.Fatalf("the CPU's pixels %v, want red RGBA", px)
	}
	v := img.Version()

	blue := newSurface(t, 3, 2, [4]byte{255, 0, 0, 255})
	tex.Present(blue)
	if useCount(t, red) == 0 {
		t.Error("the surface shown went back to its producer while frames may still draw it")
	}
	if img := tex.Frame(key, func() {}); img.Version() == v || img.Pixels()[2] != 255 {
		t.Fatalf("the next frame: version %d (was %d), pixels %v", img.Version(), v, img.Pixels())
	}
	// Frames in flight past, the red surface goes back.
	for range inFlight {
		tex.Present(newSurface(t, 3, 2, [4]byte{}))
		tex.Frame(key, func() {})
	}
	if n := useCount(t, red); n != 0 {
		t.Errorf("the red surface is still in use %d times", n)
	}

	// A window that stops drawing the texture stops being asked to.
	asked = 0
	for range 2 * staleAfter {
		tex.Present(newSurface(t, 1, 1, [4]byte{}))
	}
	if asked > staleAfter || len(tex.s.watch) != 0 {
		t.Errorf("a window that no longer draws the texture was asked %d times, and %d windows watch it", asked, len(tex.s.watch))
	}
	purego.SyscallN(iosurface.release, red)
}
