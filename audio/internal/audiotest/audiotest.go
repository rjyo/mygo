// Package audiotest has what the tests of package audio's formats share.
package audiotest

import (
	"bytes"
	"math"
	"os"
	"testing"

	"github.com/egoist/mygo/audio"
)

// Read reads a file of testdata.
func Read(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Open opens b as package audio does, and checks the format's name.
func Open(t testing.TB, b []byte, name string) audio.Decoder {
	t.Helper()
	d, got, err := audio.NewDecoder(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if got != name {
		t.Fatalf("format %q, not %q", got, name)
	}
	return d
}

// ReadAll decodes what d has left.
func ReadAll(t testing.TB, d audio.Decoder) []float32 {
	t.Helper()
	var out []float32
	buf := make([]float32, 4096)
	for {
		n, err := d.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil || n == 0 {
			return out
		}
	}
}

// SeekLands checks that a seek to each frame reads what decoding straight
// through, ref, has there, to within tol, for a second or to the end.
func SeekLands(t testing.TB, d audio.Decoder, ref []float32, tol float64, frames ...int64) {
	t.Helper()
	ch := int64(d.Channels())
	for _, frame := range frames {
		if err := d.SetPosition(frame); err != nil {
			t.Fatal(err)
		}
		want := ref[frame*ch:]
		want = want[:min(len(want), d.SampleRate()*int(ch))]
		got := make([]float32, 0, len(want))
		buf := make([]float32, 4096)
		for len(got) < len(want) {
			n, err := d.Read(buf)
			got = append(got, buf[:n]...)
			if err != nil || n == 0 {
				break
			}
		}
		if len(got) < len(want) {
			t.Errorf("seek to %d: read %d samples of %d", frame, len(got), len(want))
			continue
		}
		if diff := MaxDiff(got, want); diff > tol {
			t.Errorf("seek to %d: off by up to %.4f", frame, diff)
		}
	}
}

// MaxDiff returns how far apart a and b are at most, over the length of
// the shorter.
func MaxDiff(a, b []float32) float64 {
	d := 0.0
	for i := range min(len(a), len(b)) {
		d = math.Max(d, math.Abs(float64(a[i]-b[i])))
	}
	return d
}

// SNR is how far below ref the difference between got and ref is, in dB,
// over the length of the shorter.
func SNR(got, ref []float32) float64 {
	var sig, noise float64
	for i := range min(len(got), len(ref)) {
		sig += float64(ref[i]) * float64(ref[i])
		d := float64(got[i] - ref[i])
		noise += d * d
	}
	return 10 * math.Log10(sig/noise)
}
