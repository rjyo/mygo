package mediatest

import "github.com/ebitengine/purego"

// runLoop is CFRunLoopRun, which runs the main thread's run loop, the
// main queue's blocks with it, for ever.
var runLoop func()

func init() {
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&runLoop, cf, "CFRunLoopRun")
}
