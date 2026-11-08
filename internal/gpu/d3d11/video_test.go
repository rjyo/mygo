//go:build windows

package d3d11

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/gpu/d3d11/device"
	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// TestDrawsVideoAsTheCPURenderer draws frames of video in NV12 from their
// planes in memory, which the shader converts as the CPU renderer does,
// then frames in BGRA that another device drew into textures it shares,
// as Media Foundation does, which the renderer reads where they are.
func TestDrawsVideoAsTheCPURenderer(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s := gputest.VideoScene()
	r, err := New(hiddenWindow(t, s.Width, s.Height))
	if err != nil {
		t.Skip("no Direct3D 11:", err)
	}
	defer r.Release()
	for range 2 {
		for _, op := range s.Ops {
			op.Image.Changed()
		}
		if err := r.draw(s); err != nil {
			t.Fatal(err)
		}
		pix, stride := r.readBack(t)
		gputest.Compare(t, "d3d11-video", pix, stride, s)
	}
	for _, op := range s.Ops {
		if tex := r.images[op.Image.ID()]; tex == nil || tex.chroma == nil {
			t.Fatal("a frame of video drew from RGBA, not its planes")
		}
	}

	// The frames converted to BGRA, in textures of another device.
	dev, ctx, _, err := device.New()
	if err != nil {
		t.Fatal(err)
	}
	defer free(&dev)
	defer free(&ctx)
	var shared []uintptr
	defer func() {
		for _, h := range shared {
			syscall.CloseHandle(syscall.Handle(h))
		}
	}()
	done := map[*scene.Image]bool{}
	for i, op := range s.Ops {
		img := op.Image
		if done[img] {
			continue
		}
		done[img] = true
		rgba := img.Surface.RGBA(nil, img.W, img.H)
		bgra := make([]byte, len(rgba))
		for k := 0; k < len(rgba); k += 4 {
			bgra[k], bgra[k+1], bgra[k+2], bgra[k+3] = rgba[k+2], rgba[k+1], rgba[k], rgba[k+3]
		}
		h := newSharedTexture(t, dev, ctx, img.W, img.H, bgra)
		shared = append(shared, h)
		img.Surface = &scene.Surface{Format: scene.SurfaceBGRA, Native: h, NativeID: uint64(i + 1)}
		img.Restore = func() []byte { return rgba }
		img.Pix = nil
		img.Changed()
	}
	for range 3 {
		if err := r.draw(s); err != nil {
			t.Fatal(err)
		}
		pix, stride := r.readBack(t)
		gputest.Compare(t, "d3d11-video-shared", pix, stride, s)
	}
	if len(r.video.shared) != len(shared) {
		t.Fatalf("the renderer opened %d shared textures, not %d", len(r.video.shared), len(shared))
	}
	if len(r.video.acquired) != 0 {
		t.Fatal("the frame kept the keyed mutexes of the textures it read")
	}
}

// newSharedTexture returns the NT handle of a w×h texture in BGRA of dev with
// a keyed mutex, holding pix, which dev wrote under the mutex.
func newSharedTexture(t *testing.T, dev, ctx uintptr, w, h int, pix []byte) uintptr {
	const (
		miscSharedKeyedMutex = 0x100
		miscSharedNTHandle   = 0x800
		sharedRead           = 0x80000000
		sharedWrite          = 1
		createSharedHandle   = 13 // IDXGIResource1
	)
	iidIDXGIResource1 := guid{0x30961379, 0x4609, 0x4a41, [8]byte{0x99, 0x8e, 0x54, 0xfe, 0x56, 0x7e, 0xe0, 0xc1}}
	desc := texture2DDesc{Width: uint32(w), Height: uint32(h), MipLevels: 1, ArraySize: 1, Format: formatB8G8R8A8Unorm, SampleCount: 1,
		Usage: usageDefault, BindFlags: bindShaderResource, MiscFlags: miscSharedKeyedMutex | miscSharedNTHandle}
	var tex, mutex, res uintptr
	if hr := call(dev, devCreateTexture2D, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&tex))); failed(hr) {
		t.Fatalf("cannot create a shared texture: %#x", uint32(hr))
	}
	defer free(&tex)
	if failed(call(tex, 0, uintptr(unsafe.Pointer(&iidIDXGIKeyedMutex)), uintptr(unsafe.Pointer(&mutex)))) {
		t.Fatal("no keyed mutex")
	}
	defer free(&mutex)
	call(mutex, keyedAcquireSync, 0, 1000)
	call(ctx, ctxUpdateSubresource, tex, 0, 0, uintptr(unsafe.Pointer(&pix[0])), uintptr(4*w), 0)
	call(mutex, keyedReleaseSync, 0)
	call(ctx, ctxFlush)
	if failed(call(tex, 0, uintptr(unsafe.Pointer(&iidIDXGIResource1)), uintptr(unsafe.Pointer(&res)))) {
		t.Fatal("no IDXGIResource1")
	}
	defer free(&res)
	var handle uintptr
	if hr := call(res, createSharedHandle, 0, sharedRead|sharedWrite, 0, uintptr(unsafe.Pointer(&handle))); failed(hr) {
		t.Fatalf("cannot share the texture: %#x", uint32(hr))
	}
	return handle
}
