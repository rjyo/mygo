// Package motion is a native module drawing into a texture on the GPU:
// frames of an animation, made with Core Graphics in pixel buffers backed
// by IOSurfaces, which native UI shows as any image, with no copy.
package motion

import (
	"io"

	"github.com/egoist/mygo/native"
)

// Animation draws an animation.
//
//mygo:native
type Animation interface {
	// Attach draws the animation into tex, size pixels square, a frame
	// per refresh of the display, until the subscription is closed.
	Attach(tex *native.Texture, size int) io.Closer
}
