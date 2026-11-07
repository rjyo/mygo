package aac_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	_ "github.com/egoist/mygo/audio/aac"
	_ "github.com/egoist/mygo/audio/flac"
	"github.com/egoist/mygo/audio/internal/audiotest"
	"github.com/egoist/mygo/audio/mp4"
)

// The source the files were encoded from.
func source(t *testing.T) []float32 {
	return audiotest.ReadAll(t, audiotest.Open(t, audiotest.Read(t, "../flac/testdata/tone.flac"), "flac"))
}

// AAC's encoder adds a packet of priming before the sound.
const prime = 1024

// An AAC .m4a drops the encoder's priming, as its edit list says, so it
// lines up with what it was encoded from and is as long. A seek decodes the
// packet before, whose sound overlaps the one sought to.
func TestMP4(t *testing.T) {
	b := audiotest.Read(t, "testdata/tone.m4a")
	tr, err := mp4.ReadTrack(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Codec != "mp4a" || tr.Duration != 2*time.Second || !tr.Decodable() {
		t.Fatalf("%+v", tr)
	}
	d := audiotest.Open(t, b, "mp4")
	if d.SampleRate() != 44100 || d.Channels() != 2 || d.Len() != 88200 {
		t.Fatalf("%d Hz, %d channels, %d frames", d.SampleRate(), d.Channels(), d.Len())
	}
	got := audiotest.ReadAll(t, d)
	if len(got) != 88200*2 {
		t.Fatalf("decoded %d samples", len(got))
	}
	if db := audiotest.SNR(got, source(t)); db < 20 {
		t.Errorf("%.1f dB from the source", db)
	}
	// Noise the encoder substituted is random each time it's decoded.
	audiotest.SeekLands(t, d, got, 0.05, 0, 1, 4096, 50000, 77777)
}

// An AAC .m4a cut into fragments, as a stream for DASH is, plays what it
// was encoded from too, with its length from its segment index. Its movie
// header has no edit list, so the encoder's priming plays; the padding of
// its last packet doesn't, as its last packet's duration says.
func TestFragmented(t *testing.T) {
	b := audiotest.Read(t, "testdata/fragmented.m4a")
	d := audiotest.Open(t, b, "mp4")
	length := d.Len()
	got := audiotest.ReadAll(t, d)
	src := source(t)
	if len(got) != len(src)+prime*2 {
		t.Fatalf("decoded %d samples, the source %d", len(got), len(src))
	}
	if length*2 != int64(len(got)) || d.Len() != length {
		t.Errorf("length %d, then %d, though %d frames decode", length, d.Len(), len(got)/2)
	}
	if db := audiotest.SNR(got[prime*2:], src); db < 20 {
		t.Errorf("%.1f dB from the source", db)
	}
	audiotest.SeekLands(t, d, got, 0.05, 0, 1, 4096, 50000, 77777)
}

// AAC's packets each decode to 1024 frames, whatever the file says they
// last: a seek lands where decoding straight through has the frame. Some
// files have a packet near the start a few frames shorter, which would put
// every seek past it that much off.
func TestPacketsOfOtherDurations(t *testing.T) {
	b := bytes.Clone(audiotest.Read(t, "testdata/fragmented.m4a"))
	// The first fragment's packets last 1016 frames, its header says.
	at := bytes.Index(b, []byte("tfhd"))
	flags := binary.BigEndian.Uint32(b[at+4:]) & 0xffffff
	if flags&0x08 == 0 {
		t.Fatal("the fragment has no default duration")
	}
	at += 4 + 8
	if flags&0x01 != 0 {
		at += 8
	}
	if flags&0x02 != 0 {
		at += 4
	}
	if binary.BigEndian.Uint32(b[at:]) != 1024 {
		t.Fatal("no default duration of 1024 where expected")
	}
	binary.BigEndian.PutUint32(b[at:], 1016)
	d := audiotest.Open(t, b, "mp4")
	got := audiotest.ReadAll(t, d)
	for _, frame := range []int64{30000, 60000, 60001} {
		if err := d.SetPosition(frame); err != nil {
			t.Fatal(err)
		}
		var read []float32
		buf := make([]float32, 4096)
		for len(read) < 44100 {
			n, err := d.Read(buf)
			read = append(read, buf[:n]...)
			if err != nil || n == 0 {
				break
			}
		}
		if db := audiotest.SNR(read, got[frame*2:]); db < 20 {
			t.Errorf("seek to %d: %.1f dB from decoding straight through", frame, db)
		}
	}
}

// An .aac of ADTS frames plays what it was encoded from, after the
// encoder's priming, which nothing says to drop.
func TestADTS(t *testing.T) {
	b := audiotest.Read(t, "testdata/tone.aac")
	d := audiotest.Open(t, b, "aac")
	if d.SampleRate() != 44100 || d.Channels() != 2 {
		t.Fatalf("%d Hz, %d channels", d.SampleRate(), d.Channels())
	}
	got := audiotest.ReadAll(t, d)
	if int64(len(got)) != d.Len()*2 || len(got) < (88200+prime)*2 {
		t.Fatalf("decoded %d samples, length %d", len(got), d.Len())
	}
	if db := audiotest.SNR(got[prime*2:], source(t)); db < 20 {
		t.Errorf("%.1f dB from the source", db)
	}
	audiotest.SeekLands(t, d, got, 0.05, 0, 1, 4096, 50000, 77777)

	// After an ID3 tag too.
	tag := append([]byte("ID3\x04\x00\x00\x00\x00\x00\x20"), make([]byte, 32)...)
	d = audiotest.Open(t, append(tag, b...), "aac")
	if again := audiotest.ReadAll(t, d); len(again) != len(got) {
		t.Errorf("decoded %d samples after an ID3 tag, %d without", len(again), len(got))
	}
}

// HE-AAC, with SBR, isn't decoded: a track of it says so.
func TestHEAAC(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config []byte
		want   bool
	}{
		{"AAC-LC", []byte{0x12, 0x10}, true},
		{"HE-AAC", []byte{0x2b, 0x12, 0x08}, false},
	} {
		tr := mp4.Track{Codec: "mp4a", Config: tt.config}
		if got := tr.Decodable(); got != tt.want {
			t.Errorf("%s: decodable %v", tt.name, got)
		}
	}
}
