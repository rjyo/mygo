package native

import "github.com/egoist/mygo/internal/texture"

// Texture is an image on the GPU that a module's implementation draws
// into, and that native UI shows with no copy through the CPU: it is a
// ui.ImageSource, as a *ui.Bitmap is, so ui.Image shows it, and it can be
// covered, clipped, scaled, rotated and faded like any image. Pass it to a
// method of the module; in Swift it is a NativeTexture, whose present
// shows a frame (an IOSurface, or a CVPixelBuffer backed by one) in the
// next frame of the windows showing the texture, without the app calling
// Invalidate.
type Texture = texture.Texture

// NewTexture makes a texture, which shows nothing until the implementation
// presents a frame.
func NewTexture() *Texture { return texture.New() }
