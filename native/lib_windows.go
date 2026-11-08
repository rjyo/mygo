package native

import "syscall"

// openLibc returns the malloc and free of the Universal C Runtime, which
// every Windows MyGo runs on has, with which Go and the implementations
// allocate the blocks they give each other.
func openLibc() (malloc, free, isMain uintptr, err error) {
	dll, err := syscall.LoadDLL("ucrtbase.dll")
	if err != nil {
		return 0, 0, 0, err
	}
	m, err := dll.FindProc("malloc")
	if err != nil {
		return 0, 0, 0, err
	}
	f, err := dll.FindProc("free")
	if err != nil {
		return 0, 0, 0, err
	}
	return m.Addr(), f.Addr(), 0, nil
}

// mainThread is false: on Windows, the methods of the main thread run once
// the app runs, which takes them there.
func mainThread() bool { return false }

// olderMacOS is false off macOS: a module's minimum is macOS's.
func olderMacOS(string) bool { return false }

// releaseSurface has no surfaces to release off macOS.
func releaseSurface(uintptr) {}
