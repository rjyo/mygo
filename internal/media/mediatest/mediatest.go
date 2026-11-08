// Package mediatest runs the tests of packages that play video with the
// system's media framework (package ui/video) as an app runs: on macOS
// while the main thread runs its run loop, for AVFoundation, which loads
// and plays a video through the main queue.
//
//	func TestMain(m *testing.M) { mediatest.Main(m) }
package mediatest

import (
	"os"
	"runtime"
	"testing"
)

// The main goroutine stays on the main thread, for Main to run its run
// loop there.
func init() { runtime.LockOSThread() }

// Main runs the tests and exits with their status, on macOS while the
// main thread runs its run loop.
func Main(m *testing.M) {
	if runLoop == nil {
		os.Exit(m.Run())
	}
	go func() { os.Exit(m.Run()) }()
	runLoop()
}
