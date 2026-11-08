//go:build !darwin

package texture

func useSurface(uintptr)                               {}
func retainSurface(uintptr)                            {}
func releaseSurface(uintptr)                           {}
func surfaceSize(uintptr) (int, int)                   { return 0, 0 }
func readSurface(uintptr) []byte                       { return nil }
func metalTexture(device, s uintptr, w, h int) uintptr { return 0 }
