package texture

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var iosurface struct {
	once                                           sync.Once
	retain, release                                uintptr
	lock, unlock, base, bytesPerRow, width, height uintptr
	pixelFormat                                    uintptr
	incrementUse, decrementUse                     uintptr
}

func loadIOSurface() bool {
	s := &iosurface
	s.once.Do(func() {
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		io, err := purego.Dlopen("/System/Library/Frameworks/IOSurface.framework/IOSurface", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		for _, f := range []struct {
			lib  uintptr
			name string
			p    *uintptr
		}{
			{cf, "CFRetain", &s.retain}, {cf, "CFRelease", &s.release},
			{io, "IOSurfaceLock", &s.lock}, {io, "IOSurfaceUnlock", &s.unlock},
			{io, "IOSurfaceGetBaseAddress", &s.base}, {io, "IOSurfaceGetBytesPerRow", &s.bytesPerRow},
			{io, "IOSurfaceGetWidth", &s.width}, {io, "IOSurfaceGetHeight", &s.height},
			{io, "IOSurfaceGetPixelFormat", &s.pixelFormat},
			{io, "IOSurfaceIncrementUseCount", &s.incrementUse}, {io, "IOSurfaceDecrementUseCount", &s.decrementUse},
		} {
			if *f.p, err = purego.Dlsym(f.lib, f.name); err != nil {
				*f.p = 0
			}
		}
	})
	return s.release != 0 && s.width != 0
}

// useSurface marks a surface whose reference the texture took as in use:
// a pool of pixel buffers gives it to the producer again only once native
// UI shows another, rather than have it drawn while on screen.
func useSurface(s uintptr) {
	if s != 0 && loadIOSurface() && iosurface.incrementUse != 0 {
		purego.SyscallN(iosurface.incrementUse, s)
	}
}

// retainSurface takes another reference to a surface, in use.
func retainSurface(s uintptr) {
	if s != 0 && loadIOSurface() {
		purego.SyscallN(iosurface.retain, s)
		useSurface(s)
	}
}

// releaseSurface lets go of a reference retainSurface or useSurface took.
func releaseSurface(s uintptr) {
	if s != 0 && loadIOSurface() {
		if iosurface.decrementUse != 0 {
			purego.SyscallN(iosurface.decrementUse, s)
		}
		purego.SyscallN(iosurface.release, s)
	}
}

func surfaceSize(s uintptr) (int, int) {
	if s == 0 || !loadIOSurface() {
		return 0, 0
	}
	w, _, _ := purego.SyscallN(iosurface.width, s)
	h, _, _ := purego.SyscallN(iosurface.height, s)
	return int(w), int(h)
}

// pixelFormatBGRA is kCVPixelFormatType_32BGRA, 'BGRA'.
const pixelFormatBGRA = 0x42475241

// readSurface copies the pixels of a 32-bit BGRA surface as premultiplied
// RGBA rows, for the CPU renderer.
func readSurface(s uintptr) []byte {
	if s == 0 || !loadIOSurface() {
		return nil
	}
	w, h := surfaceSize(s)
	if f, _, _ := purego.SyscallN(iosurface.pixelFormat, s); f != pixelFormatBGRA || w == 0 || h == 0 {
		return make([]byte, 4*w*h)
	}
	const readOnly = 1
	purego.SyscallN(iosurface.lock, s, readOnly, 0)
	defer purego.SyscallN(iosurface.unlock, s, readOnly, 0)
	base, _, _ := purego.SyscallN(iosurface.base, s)
	stride, _, _ := purego.SyscallN(iosurface.bytesPerRow, s)
	if base == 0 {
		return make([]byte, 4*w*h)
	}
	src := unsafe.Slice(*(**byte)(unsafe.Pointer(&base)), int(stride)*h)
	out := make([]byte, 4*w*h)
	for y := range h {
		row, dst := src[y*int(stride):], out[4*w*y:]
		for x := range w {
			dst[4*x], dst[4*x+1], dst[4*x+2], dst[4*x+3] = row[4*x+2], row[4*x+1], row[4*x], row[4*x+3]
		}
	}
	return out
}

var (
	selDescriptor = objc.RegisterName("texture2DDescriptorWithPixelFormat:width:height:mipmapped:")
	selSetUsage   = objc.RegisterName("setUsage:")
	selNewTexture = objc.RegisterName("newTextureWithDescriptor:iosurface:plane:")
)

// metalTexture returns a new texture of the Metal device showing the
// 32-bit BGRA surface s, w×h, which it retains while frames draw it.
func metalTexture(device, s uintptr, w, h int) uintptr {
	if device == 0 || s == 0 {
		return 0
	}
	const pixelFormatBGRA8Unorm, usageShaderRead = 80, 1
	desc := objc.ID(objc.GetClass("MTLTextureDescriptor")).Send(selDescriptor, pixelFormatBGRA8Unorm, w, h, false)
	if desc == 0 {
		return 0
	}
	desc.Send(selSetUsage, usageShaderRead)
	return uintptr(objc.ID(device).Send(selNewTexture, desc, s, 0))
}
