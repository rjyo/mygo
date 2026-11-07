package flac_test

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/audio/flac"
	"github.com/egoist/mygo/audio/internal/audiotest"
)

// tone.flac is 2 s of two chirps at 44.1 kHz, one in each channel, in
// 16 bits: it decodes to exactly that, and a seek lands on the very frame
// asked for, near or far, as far as its last.
func TestDecode(t *testing.T) {
	b := audiotest.Read(t, "testdata/tone.flac")
	d := audiotest.Open(t, b, "flac")
	if d.SampleRate() != 44100 || d.Channels() != 2 || d.Len() != 88200 {
		t.Fatalf("%d Hz, %d channels, %d frames", d.SampleRate(), d.Channels(), d.Len())
	}
	got := audiotest.ReadAll(t, d)
	if len(got) != 88200*2 {
		t.Fatalf("decoded %d samples", len(got))
	}
	if got[2000] == got[2001] {
		t.Error("both channels alike")
	}
	audiotest.SeekLands(t, d, got, 0, 0, 1, 4095, 4096, 50000, 77777, 88199, 1000)
}

// A FLAC file may start with an ID3 tag: it decodes as without one.
func TestID3(t *testing.T) {
	b := audiotest.Read(t, "testdata/tone.flac")
	want := audiotest.ReadAll(t, audiotest.Open(t, b, "flac"))
	tag := append([]byte("ID3\x04\x00\x00\x00\x00\x01\x00"), make([]byte, 128)...)
	d := audiotest.Open(t, append(tag, b...), "flac")
	got := audiotest.ReadAll(t, d)
	if len(got) != len(want) || audiotest.MaxDiff(got, want) != 0 {
		t.Fatalf("decoded %d samples, off by up to %g", len(got), audiotest.MaxDiff(got, want))
	}
	audiotest.SeekLands(t, d, want, 0, 70000)
}

// FLAC in a fragmented MP4, as a lossless DASH stream carries it, decodes
// to what the .flac does.
func TestFragmentedMP4(t *testing.T) {
	want := audiotest.ReadAll(t, audiotest.Open(t, audiotest.Read(t, "testdata/tone.flac"), "flac"))
	d := audiotest.Open(t, audiotest.Read(t, "testdata/fragmented.m4a"), "mp4")
	if d.SampleRate() != 44100 || d.Len() != 88200 {
		t.Fatalf("%d Hz, %d frames", d.SampleRate(), d.Len())
	}
	got := audiotest.ReadAll(t, d)
	if len(got) != len(want) || audiotest.MaxDiff(got, want) != 0 {
		t.Fatalf("decoded %d samples, off by up to %g", len(got), audiotest.MaxDiff(got, want))
	}
	audiotest.SeekLands(t, d, want, 0, 0, 1, 4096, 30000, 88199)
}

func TestNotFLAC(t *testing.T) {
	if _, err := flac.NewDecoder(bytes.NewReader([]byte("OggS and more"))); err == nil {
		t.Fatal("decoded what isn't FLAC")
	}
}
