package video

import (
	_ "embed"
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// rgb.mp4 is 1.2 s of H.264 at 10 fps, 64×48, and silent AAC: 0.4 s red,
// then green, then blue. It has no B-frames, so no edit list shifts its
// pictures' times: the MPEG-4 source of Windows on Arm ignores one, and
// would show each picture two frames late.
//
//go:embed testdata/rgb.mp4
var rgbMP4 []byte

// played opens rgb.mp4 as open does and shows it in a Tester, skipping
// the test on a system that has no engine or no H.264 decoder.
func played(t *testing.T, open func() (*ui.VideoPlayer, error)) (*ui.VideoPlayer, *ui.Tester, func() color.RGBA) {
	t.Helper()
	p, err := open()
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	p.SetMuted(true)
	tt := ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(64, 48) }, 64, 48)
	shows := func() color.RGBA { return tt.Image().RGBAAt(32, 24) }
	wait.Until(t, "the first picture", func() bool {
		tt.Frame()
		if err := p.Err(); err != nil {
			if strings.Contains(err.Error(), "missing") || strings.Contains(err.Error(), "decoder") {
				t.Skip("no H.264 decoder:", err)
			}
			t.Fatal(err)
		}
		return p.Ready()
	})
	return p, tt, shows
}

// near reports whether c is the color want, give or take what H.264 and
// the conversion to RGB change.
func near(c, want color.RGBA) bool {
	d := func(a, b uint8) bool { return max(a, b)-min(a, b) <= 24 }
	return d(c.R, want.R) && d(c.G, want.G) && d(c.B, want.B) && c.A == 255
}

var (
	red   = color.RGBA{255, 0, 0, 255}
	green = color.RGBA{0, 255, 0, 255}
	blue  = color.RGBA{0, 0, 255, 255}
)

// TestPlays checks that a video in memory plays its frames, as the system
// decodes them, in order and to its end, reporting its length.
func TestPlays(t *testing.T) {
	p, tt, shows := played(t, func() (*ui.VideoPlayer, error) { return OpenData(rgbMP4) })
	if w, h := p.Size(); w != 64 || h != 48 {
		t.Fatalf("size %d×%d, want 64×48", w, h)
	}
	if d := p.Duration(); d < 1100*time.Millisecond || d > 1300*time.Millisecond {
		t.Errorf("duration %v, want 1.2s", d)
	}
	for _, want := range []color.RGBA{red, green, blue} {
		wait.Until(t, "the picture "+colorName(want), func() bool {
			tt.Frame()
			return near(shows(), want)
		})
	}
	wait.Until(t, "the end", func() bool { tt.Frame(); return p.Ended() })
	if p.Playing() || !near(shows(), blue) {
		t.Fatalf("at the end: playing %v, showing %v", p.Playing(), shows())
	}
}

// TestSeek checks that a paused video shows the picture a seek asks for,
// and reports the position, and that one played again after its end
// starts over.
func TestSeek(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a video.mp4")
	if err := os.WriteFile(path, rgbMP4, 0o644); err != nil {
		t.Fatal(err)
	}
	p, tt, shows := played(t, func() (*ui.VideoPlayer, error) { return Open(path) })
	p.Pause()
	for _, s := range []struct {
		at   time.Duration
		want color.RGBA
	}{{900 * time.Millisecond, blue}, {500 * time.Millisecond, green}, {100 * time.Millisecond, red}} {
		p.Seek(s.at)
		wait.Until(t, "the picture at "+s.at.String(), func() bool {
			tt.Frame()
			return near(shows(), s.want)
		})
		if pos := p.Position(); pos < s.at-100*time.Millisecond || pos > s.at+100*time.Millisecond {
			t.Errorf("sought to %v: position %v", s.at, pos)
		}
		if p.Playing() {
			t.Fatal("a seek played a paused video")
		}
	}
	p.Seek(time.Second)
	p.Play()
	wait.Until(t, "the end", func() bool { tt.Frame(); return p.Ended() })
	p.Play()
	wait.Until(t, "the start, played again", func() bool { tt.Frame(); return near(shows(), red) })
}

func colorName(c color.RGBA) string {
	switch c {
	case red:
		return "red"
	case green:
		return "green"
	}
	return "blue"
}
