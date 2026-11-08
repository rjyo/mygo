// Video plays video in native UI, as a launcher's store page does: a
// short GIF loops behind the window's content, and a trailer with sound
// plays over it, with controls to pause it, seek and mute it.
//
//	go run ./examples/video
//
// The GIF decodes in Go (ui.NewGIFDecoder), on every system: its frames
// decode on a goroutine a few ahead, and each reaches the screen as it
// comes due, so the window draws one frame for each of the clip's and none
// between. It stands still while the window is out of sight, and shows its
// first frame while the desktop asks for less motion.
//
// The trailer, an MP4 of H.264 and AAC embedded in the app, plays with the
// system's media framework (package ui/video): AVFoundation on macOS,
// Media Foundation on Windows, GStreamer on Linux and the video element in
// browsers, which decode it in hardware where they can and play its sound
// in sync. Its pictures reach the window's renderer where the decoder left
// them, on the GPU.
package main

import (
	_ "embed"
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"
)

var (
	//go:embed waves.gif
	wavesGIF []byte
	//go:embed trailer.mp4
	trailerMP4 []byte
)

// app is the state the window shows.
type app struct {
	waves *ui.VideoPlayer
	// trailer is the trailer's player once opened, and trailerErr what
	// stopped it from opening, as a system without a media framework.
	trailer    *ui.VideoPlayer
	trailerErr error
}

func newApp() (*app, error) {
	d, err := ui.NewGIFDecoder(wavesGIF)
	if err != nil {
		return nil, err
	}
	waves, err := ui.NewVideoPlayer(d)
	if err != nil {
		return nil, err
	}
	return &app{waves: waves.SetLoop(true).SetMuted(true)}, nil
}

// watch opens the trailer, which plays once a Video shows it.
func (s *app) watch() {
	s.trailer, s.trailerErr = video.OpenData(trailerMP4)
	if s.trailer != nil {
		// The background holds still while the trailer plays.
		s.waves.Pause()
	}
}

// closeTrailer stops the trailer and its sound.
func (s *app) closeTrailer() {
	s.trailer.Close()
	s.trailer = nil
	s.waves.Play()
}

// view builds the interface from the state, on the main thread, whenever
// the window needs a frame.
func (s *app) view(c *ui.Context) {
	white := ui.RGB(255, 255, 255)
	ui.Box(c).Fill().Children(func() {
		ui.Video(c, s.waves).Fill().Fit(ui.Cover)
		// Over the clip, which takes the window.
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Gap(16).Children(func() {
			if s.trailer != nil {
				s.trailerView(c)
				return
			}
			ui.Text(c, "Wavelength").FontSize(44).Bold().TextColor(white)
			ui.Row(c).Gap(8).Children(func() {
				label := "Pause"
				if !s.waves.Playing() {
					label = "Play"
				}
				if ui.Button(c.Key("play"), label).Width(96).Clicked() {
					if s.waves.Playing() {
						s.waves.Pause()
					} else {
						s.waves.Play()
					}
				}
				if ui.Button(c.Key("watch"), "Watch the trailer").Clicked() {
					s.watch()
				}
			})
			if s.trailerErr != nil {
				ui.Text(c, s.trailerErr.Error()).TextColor(white)
			}
		})
	})
}

// trailerView shows the trailer and its controls.
func (s *app) trailerView(c *ui.Context) {
	p := s.trailer
	white := ui.RGB(255, 255, 255)
	ui.Column(c).Width(480).Gap(8).Padding(12).Background(ui.RGBA(0, 0, 0, 200)).Radius(12).Children(func() {
		ui.Box(c).Width(456).Height(256).Center().Children(func() {
			ui.Video(c, p).Width(456).Height(256).Fit(ui.Contain).Radius(6).Label("Trailer")
			switch {
			case p.Err() != nil:
				ui.Text(c, p.Err().Error()).TextColor(white).Absolute()
			case !p.Ready() || p.Buffering():
				ui.Spinner(c).Absolute()
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			label := "Pause"
			if !p.Playing() {
				label = "Play"
			}
			if ui.Button(c.Key("trailer-play"), label).Width(72).Clicked() {
				if p.Playing() {
					p.Pause()
				} else {
					p.Play()
				}
			}
			pos, dur := p.Position().Seconds(), p.Duration().Seconds()
			if ui.Slider(c.Key("seek"), &pos, 0, max(dur, 0.001)).Grow(1).Label("Position").Changed() {
				p.Seek(time.Duration(pos * float64(time.Second)))
			}
			ui.Text(c, clock(p.Position())+" / "+clock(p.Duration())).TextColor(white).FontFeatures("tnum")
			mute := "Mute"
			if p.Muted() {
				mute = "Unmute"
			}
			if ui.Button(c.Key("mute"), mute).Width(92).Clicked() {
				p.SetMuted(!p.Muted())
			}
			if ui.Button(c.Key("close"), "Close").Clicked() {
				s.closeTrailer()
			}
		})
	})
}

// clock is a time as minutes and seconds.
func clock(d time.Duration) string {
	sec := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

func main() {
	s, err := newApp()
	if err != nil {
		log.Fatal(err)
	}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:   "Video",
			Width:   640,
			Height:  400,
			Content: ui.View(s.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
