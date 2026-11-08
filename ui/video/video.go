// Package video plays video files and streams with the system's own media
// framework, for ui.Video to show: AVFoundation on macOS, Media
// Foundation on Windows, GStreamer on Linux and the video element in
// browsers. The system decodes the video, in hardware where it can, and
// plays its sound in sync with the picture; its frames reach the window's
// renderer where the decoder left them, on the GPU, without a copy
// through memory where the system allows it.
//
//	//go:embed trailer.mp4
//	var trailer []byte
//
//	app.trailer, err = video.OpenData(trailer)
//	...
//	ui.Video(c, app.trailer).Fill().Fit(ui.Contain)
//
// A video opens at once and loads in the background: the player is not
// Ready, and its Size is 0×0, until its first picture came, which builds
// a frame of the window that shows it, as a change of its state does.
// What formats play is the system's: H.264 and AAC in MP4 or MOV play on
// all of them; HEVC, VP9 and AV1 where the system has their decoders, and
// HLS streams on macOS, Windows and browsers.
//
// Apps that do not import the package link none of it. GIFs and animated
// PNGs play without it, decoded in Go (ui.NewGIFDecoder,
// ui.NewAPNGDecoder), on every system.
package video

import (
	"errors"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

// ErrUnsupported reports a system without a media framework that plays
// video: a Linux without GStreamer 1.x and its base plugins, or an OS of
// none. An app may show a GIF in its place.
var ErrUnsupported = errors.New("video: the system has no media framework that plays video")

// source is what a player opens: a URL, or bytes in memory.
type source struct {
	url  string
	data []byte
}

// Open opens a video file, playing once a Video shows it.
func Open(path string) (*ui.VideoPlayer, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return open(source{url: fileURL(abs)})
}

// OpenURL opens a video at a URL: a file:, http: or https: URL, or an HLS
// playlist (.m3u8) where the system streams them. It plays once a Video
// shows it.
func OpenURL(url string) (*ui.VideoPlayer, error) {
	return open(source{url: url})
}

// OpenData opens a video in memory, as one embedded in the app, without
// writing it to a file: the system reads it from data, which must stay as
// it is while the player is open. It plays once a Video shows it.
func OpenData(data []byte) (*ui.VideoPlayer, error) {
	if len(data) == 0 {
		return nil, errors.New("video: no data")
	}
	return open(source{data: data})
}

func open(s source) (*ui.VideoPlayer, error) {
	e, err := newEngine(s)
	if err != nil {
		return nil, err
	}
	return ui.NewEngineVideoPlayer(e), nil
}

// fileURL is the file: URL of an absolute path.
func fileURL(abs string) string {
	p := filepath.ToSlash(abs)
	if len(p) > 0 && p[0] != '/' {
		p = "/" + p // C:/… on Windows
	}
	const hex = "0123456789ABCDEF"
	b := []byte("file://")
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '/', c == '-', c == '_', c == '.', c == '~', c == ':':
			b = append(b, c)
		default:
			b = append(b, '%', hex[c>>4], hex[c&15])
		}
	}
	return string(b)
}
