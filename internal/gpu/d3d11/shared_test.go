//go:build windows

package d3d11

import (
	"errors"
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/gpu/d3d11/device"
	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// newRendererFor makes a renderer drawing s into a window of its own.
func newRendererFor(t *testing.T, s *scene.Scene) *Renderer {
	t.Helper()
	r, err := New(hiddenWindow(t, s.Width, s.Height))
	if err != nil {
		t.Skip("no Direct3D 11:", err)
	}
	return r
}

// drawAndCompare draws s whole with r, requires the CPU renderer's pixels,
// and presents it.
func drawAndCompare(t *testing.T, name string, r *Renderer, s *scene.Scene) {
	t.Helper()
	if err := r.draw(s); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	pix, stride := r.readBack(t)
	gputest.Compare(t, name, pix, stride, s)
	if err := r.present(1); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// TestRenderersShareTheDevice checks that the renderers of two windows draw
// on one device and its context, with the same shaders and states, each
// into its own swap chain, their frames taking turns; that one released
// leaves the other drawing; and that the device goes with the last.
func TestRenderersShareTheDevice(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s1, s2 := gputest.Scene(), gputest.CornerClipScene()
	r1 := newRendererFor(t, s1)
	r2 := newRendererFor(t, s2)
	defer r2.Release()
	if r1.dev != r2.dev || r1.device != r2.device || r1.ctx != r2.ctx {
		r1.Release()
		t.Fatal("the renderers of two windows draw on devices of their own")
	}
	d := r1.dev
	if d.refs != 2 {
		t.Errorf("the device counts %d renderers, want 2", d.refs)
	}
	if r1.vs != r2.vs || r1.ps != r2.ps || r1.layout != r2.layout || r1.blend != r2.blend || r1.samp != r2.samp || r1.classes != r2.classes {
		t.Error("the renderers do not share the shaders and states of their device")
	}
	if r1.cbuf == r2.cbuf || r1.passCB == r2.passCB {
		t.Error("the renderers share the buffers their frames change")
	}
	for frame := range 2 {
		drawAndCompare(t, "d3d11 shared, first window", r1, s1)
		drawAndCompare(t, "d3d11 shared, second window", r2, s2)
		if frame == 0 {
			// The swap chains are made with the first frame.
			if r1.swapChain == 0 || r2.swapChain == 0 {
				t.Fatal("no swap chain")
			}
			if r1.swapChain == r2.swapChain {
				t.Error("the renderers share a swap chain")
			}
		}
	}

	r1.Release()
	if d.refs != 1 || d.device == 0 || r2.device != d.device {
		t.Fatalf("releasing a renderer freed the device the other draws on (%d renderers)", d.refs)
	}
	for range 2 {
		drawAndCompare(t, "d3d11 shared, after the other window closed", r2, s2)
	}
	if r2.dev.replaced() {
		t.Error("the device of the remaining renderer counts as replaced")
	}

	r2.Release()
	if d.refs != 0 || d.device != 0 {
		t.Errorf("the device outlives its last renderer (%d renderers)", d.refs)
	}
	devices.Lock()
	current := devices.current
	devices.Unlock()
	if current != nil {
		t.Error("new renderers would share a device no renderer holds")
	}
}

// TestRemovedDeviceIsMadeAgain checks that a renderer made after the device
// was removed, as one a window makes after a failed frame, makes a new
// device, which the renderers made after it share; that the renderers
// holding the old one fail their next frame, so that their windows make
// them again, on the new one; and that the old device goes with the last of
// them.
func TestRemovedDeviceIsMadeAgain(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s1, s2 := gputest.Scene(), gputest.CornerClipScene()
	r1 := newRendererFor(t, s1)
	r2 := newRendererFor(t, s2)
	old := r1.dev
	defer func(f func(*sharedDevice) bool) { deviceRemoved = f }(deviceRemoved)
	deviceRemoved = func(d *sharedDevice) bool { return d == old }

	// The first window to fail makes its renderer again.
	if err := r1.Render(s1); err != nil {
		t.Fatalf("a frame failed before the device was replaced: %v", err)
	}
	r1.Release()
	r1 = newRendererFor(t, s1)
	defer r1.Release()
	if r1.dev == old {
		t.Fatal("a renderer made after the device was removed draws on it")
	}
	if old.refs != 1 || old.device == 0 {
		t.Fatalf("the removed device has %d renderers", old.refs)
	}
	drawAndCompare(t, "d3d11 on the device made again", r1, s1)

	// The other window's next frame fails, and its renderer made again
	// shares the new device.
	if err := r2.Render(s2); !errors.Is(err, errDeviceReplaced) {
		t.Fatalf("a frame on the removed device: %v, want %v", err, errDeviceReplaced)
	}
	r2.Release()
	if old.refs != 0 || old.device != 0 {
		t.Errorf("the removed device outlives its last renderer (%d renderers)", old.refs)
	}
	r2 = newRendererFor(t, s2)
	defer r2.Release()
	if r2.dev != r1.dev {
		t.Fatal("the renderers made again draw on devices of their own")
	}
	drawAndCompare(t, "d3d11 made again, first window", r1, s1)
	drawAndCompare(t, "d3d11 made again, second window", r2, s2)
	if r1.dev.refs != 2 {
		t.Errorf("the new device counts %d renderers, want 2", r1.dev.refs)
	}
}

// TestDeviceOnWARPGivesWayToTheGPU checks that renderers share a device
// made with WARP while the GPU can make none, and that a renderer made once
// it can makes a device on it, which the renderers on WARP then give way
// to, as a window drawing in software tries for the GPU from time to time.
func TestDeviceOnWARPGivesWayToTheGPU(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func(f func() (uintptr, uintptr, error)) { newDeviceOnGPU = f }(newDeviceOnGPU)
	s := gputest.Scene()
	r1 := newRendererFor(t, s)
	defer func() { r1.Release() }()
	if r1.dev.warp {
		t.Skip("no GPU: the device is WARP's")
	}
	// It stands for a device made with WARP.
	r1.dev.warp = true

	newDeviceOnGPU = func() (uintptr, uintptr, error) { return 0, 0, errors.New("no GPU") }
	r2 := newRendererFor(t, s)
	if r2.dev != r1.dev {
		r2.Release()
		t.Fatal("a renderer made while the GPU can make no device does not share the one on WARP")
	}
	r2.Release()

	newDeviceOnGPU = device.NewOnGPU
	r2 = newRendererFor(t, s)
	defer r2.Release()
	if r2.dev == r1.dev || r2.dev.warp {
		t.Fatal("a renderer made once the GPU can make a device draws with WARP")
	}
	drawAndCompare(t, "d3d11 back on the GPU", r2, s)
	if err := r1.Render(s); !errors.Is(err, errDeviceReplaced) {
		t.Fatalf("a frame on WARP once the GPU is back: %v, want %v", err, errDeviceReplaced)
	}
	r1.Release()
	r1 = newRendererFor(t, s)
	if r1.dev != r2.dev {
		t.Fatal("a renderer made again after drawing with WARP does not share the device on the GPU")
	}
}
