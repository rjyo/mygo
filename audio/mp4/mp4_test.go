package mp4_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	_ "github.com/egoist/mygo/audio/flac"
	"github.com/egoist/mygo/audio/internal/audiotest"
	"github.com/egoist/mygo/audio/mp4"
)

// alac.m4a is the first second of ../flac/testdata/tone.flac in Apple
// Lossless: it decodes to exactly those samples, and a seek lands on the
// very frame asked for.
func TestALAC(t *testing.T) {
	b := audiotest.Read(t, "testdata/alac.m4a")
	tr, err := mp4.ReadTrack(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Codec != "alac" || tr.Duration != time.Second || tr.Fragmented || !tr.Decodable() {
		t.Fatalf("%+v", tr)
	}
	d := audiotest.Open(t, b, "mp4")
	if d.SampleRate() != 44100 || d.Channels() != 2 || d.Len() != 44100 {
		t.Fatalf("%d Hz, %d channels, %d frames", d.SampleRate(), d.Channels(), d.Len())
	}
	got := audiotest.ReadAll(t, d)
	want := audiotest.ReadAll(t, audiotest.Open(t, audiotest.Read(t, "../flac/testdata/tone.flac"), "flac"))
	if len(got) != 44100*2 || audiotest.MaxDiff(got, want) != 0 {
		t.Fatalf("decoded %d samples, off from the FLAC by up to %g", len(got), audiotest.MaxDiff(got, want))
	}
	audiotest.SeekLands(t, d, got, 0, 0, 1, 4095, 4096, 30000, 44099)
}

// A track of a codec none registered is described, and fails to decode:
// this test doesn't import package aac.
func TestNoCodec(t *testing.T) {
	b := audiotest.Read(t, "../aac/testdata/tone.m4a")
	tr, err := mp4.ReadTrack(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Codec != "mp4a" || len(tr.Config) < 2 || tr.Duration != 2*time.Second || tr.Decodable() {
		t.Fatalf("%+v", tr)
	}
	if _, err := mp4.NewDecoder(bytes.NewReader(b)); err == nil {
		t.Fatal("decoded AAC with no codec for it")
	}
}

// coming reads a file of which only the first n bytes are in, as one
// still downloading, failing to read past them.
type coming struct {
	*bytes.Reader
	n int64
}

var errNotYet = errors.New("not in yet")

func (c *coming) ReadAt(b []byte, off int64) (int, error) {
	if off+int64(len(b)) > c.n {
		return 0, errNotYet
	}
	return c.Reader.ReadAt(b, off)
}

// A fragmented MP4 plays with only its first fragment in, its length from
// its segment index: the fragments after are read as it plays them.
func TestFragmentedComingIn(t *testing.T) {
	b := audiotest.Read(t, "../flac/testdata/fragmented.m4a")
	r := &coming{bytes.NewReader(b), 16 << 10}
	tr, err := mp4.ReadTrack(r, int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Fragmented || tr.Duration != 2*time.Second {
		t.Fatalf("%+v", tr)
	}
	d, err := mp4.NewDecoder(r)
	if err != nil {
		t.Fatal(err)
	}
	if d.Len() != 88200 {
		t.Fatalf("length %d", d.Len())
	}
	buf := make([]float32, 2*4096)
	if n, err := d.Read(buf); n != len(buf) || err != nil {
		t.Fatalf("read %d samples: %v", n, err)
	}
	// Then the rest comes in.
	r.n = int64(len(b))
	got := append(buf, audiotest.ReadAll(t, d)...)
	if len(got) != 88200*2 {
		t.Fatalf("decoded %d samples", len(got))
	}
}

// A fragmented MP4 with no segment index is read through for its length.
func TestFragmentedNoIndex(t *testing.T) {
	b := bytes.Clone(audiotest.Read(t, "../flac/testdata/fragmented.m4a"))
	at := bytes.Index(b, []byte("sidx"))
	copy(b[at:], "free")
	d := audiotest.Open(t, b, "mp4")
	if d.Len() != 88200 {
		t.Fatalf("length %d", d.Len())
	}
}

func TestNotMP4(t *testing.T) {
	_, err := mp4.NewDecoder(bytes.NewReader([]byte("\x00\x00\x00\x10ftypM4A \x00\x00\x00\x00")))
	if err == nil {
		t.Fatal("decoded an MP4 with no movie header")
	}
	if _, err := mp4.NewDecoder(bytes.NewReader(nil)); err == nil {
		t.Fatal("decoded nothing")
	}
}
