//go:build !darwin

package mediatest

// runLoop is nil where the media framework needs no run loop of the main
// thread's.
var runLoop func()
