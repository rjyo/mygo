// Package places is a native module of views: a map of the platform shown
// in native UI, and the system's share menu, anchored to what the window
// shows. Its implementation is the Swift package in swift/, with MapKit.
package places

import (
	"io"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/examples/native-module/modules/language"
	"github.com/egoist/mygo/native"
	"github.com/egoist/mygo/ui"
)

// Map shows a map, which ui.NativeView places in a window of native UI.
//
//mygo:native
type Map interface {
	native.View
	// Show centers the map on a place, span degrees across.
	Show(p Place, span float64)
	// Center returns the place at the center of the map.
	Center() Place
	// Pin puts a pin at p, titled with text and its language, which a of
	// the native module language tells: the platform calls that module's
	// object itself.
	Pin(p Place, a language.Analyzer, text string)
	// OnMove calls f with the center of the map once the user moved it,
	// until the subscription is closed.
	OnMove(f func(center Place)) io.Closer
}

// Sharing shows the system's share menu.
//
//mygo:native
//mygo:mainthread
type Sharing interface {
	// Share shows the share menu for text, from rect of win.
	Share(win *mygo.Window, rect ui.Rect, text string)
	// ShareFrom shows the share menu for text, from a view of a module.
	ShareFrom(view native.View, text string)
}

// Place is a point on Earth.
type Place struct {
	Latitude, Longitude float64
}
