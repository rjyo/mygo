package main

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/media/mediatest"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"
)

func TestMain(m *testing.M) { mediatest.Main(m) }

// The background plays behind the content, and the button pauses it and
// plays it again.
func TestVideoBackground(t *testing.T) {
	s, err := newApp()
	if err != nil {
		t.Fatal(err)
	}
	defer s.waves.Close()
	if w, h := s.waves.Size(); w != 192 || h != 108 {
		t.Errorf("the clip is %d×%d", w, h)
	}
	tt := ui.NewTester(s.view, 640, 360)
	if !tt.HasText("Wavelength") || !s.waves.Playing() || !s.waves.Loop() {
		t.Fatalf("texts %q, playing %v, looping %v", tt.Texts(), s.waves.Playing(), s.waves.Loop())
	}
	if err := tt.Click("Pause"); err != nil {
		t.Fatal(err)
	}
	if s.waves.Playing() {
		t.Fatal("Pause did not hold the clip")
	}
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	if !s.waves.Playing() {
		t.Fatal("Play did not go on")
	}
}

// The trailer opens over the background, which holds still, plays, and
// closes, where the system has a media framework.
func TestTrailer(t *testing.T) {
	s, err := newApp()
	if err != nil {
		t.Fatal(err)
	}
	defer s.waves.Close()
	tt := ui.NewTester(s.view, 640, 400)
	if err := tt.Click("Watch the trailer"); err != nil {
		t.Fatal(err)
	}
	if errors.Is(s.trailerErr, video.ErrUnsupported) {
		if !tt.HasText(s.trailerErr.Error()) {
			t.Fatal("the error does not show")
		}
		t.Skip(s.trailerErr)
	}
	if s.trailer == nil || s.waves.Playing() {
		t.Fatalf("watching: error %v, the background playing %v", s.trailerErr, s.waves.Playing())
	}
	wait.Until(t, "the trailer's first picture", func() bool {
		tt.Frame()
		if err := s.trailer.Err(); err != nil {
			t.Skip("the system does not play the trailer:", err)
		}
		return s.trailer.Ready()
	})
	// The trailer is six seconds long, and a slow machine takes most of
	// them to show its first picture: looping keeps it playing for the
	// clicks below, where it would end and Pause would show Play, or a
	// click on the last frame's Pause would play it again.
	s.trailer.SetLoop(true)
	tt.Frame()
	if w, h := s.trailer.Size(); w != 480 || h != 270 {
		t.Errorf("the trailer is %d×%d", w, h)
	}
	if d := s.trailer.Duration(); d < 5*time.Second || d > 7*time.Second {
		t.Errorf("the trailer is %v long", d)
	}
	if err := tt.Click("Mute"); err != nil || !s.trailer.Muted() {
		t.Fatalf("mute: %v", err)
	}
	if err := tt.Click("Pause"); err != nil || s.trailer.Playing() {
		t.Fatalf("pause: %v", err)
	}
	if err := tt.Click("Close"); err != nil || s.trailer != nil || !s.waves.Playing() {
		t.Fatalf("close: %v", err)
	}
}
