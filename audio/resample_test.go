package audio

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// Mixing one looping stereo voice, a buffer of 10 ms at a time, at the
// rates sounds come in.
func BenchmarkMix(b *testing.B) {
	for _, rate := range []int{44100, 48000, 88200, 96000, 192000} {
		b.Run(fmt.Sprint(rate), func(b *testing.B) {
			mu.Lock()
			testMode = true
			voices = nil
			volume = 1
			mu.Unlock()
			NewSound(rate, 2, sine(rate, 1, 440, 660)).NewPlayer().SetLoop(true).Play()
			buf := make([]float32, sampleRate/100*channels)
			// The first allocates the voice's ring, and its kernels.
			mix(buf)
			b.ReportAllocs()
			for b.Loop() {
				mix(buf)
			}
		})
	}
}

func TestDownsampleAlias(t *testing.T) {
	for _, tc := range []struct {
		name              string
		rate, tone, alias int // Hz
	}{
		{"88k2", 88200, 30000, 18000},
		{"96k", 96000, 30000, 18000},
		{"192k", 192000, 90000, 6000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setup(t)
			input := make([]float32, 2*tc.rate)
			for i := range tc.rate {
				phase := 2 * math.Pi * float64(i) / float64(tc.rate)
				input[2*i] = float32(0.1 * math.Sin(6000*phase))
				input[2*i+1] = float32(0.1 * math.Sin(float64(tc.tone)*phase))
			}
			NewSound(tc.rate, 2, input).Play()
			out := render(9600)[2*4800:] // Analyze 100 ms after startup.
			pass := measuredAmplitude(out, 0, 6000)
			alias := measuredAmplitude(out, 1, tc.alias)
			t.Logf("control %.6f, alias %.6f", pass, alias)
			if math.Abs(pass-0.1) > 0.005 {
				t.Fatalf("invalid passband control: %g", pass)
			}
			if alias > 0.1*0.001 {
				t.Errorf("alias at %d Hz exceeds -60 dB relative to input", tc.alias)
			}
		})
	}
}

// measuredAmplitude returns the peak amplitude of channel ch of out at hz.
func measuredAmplitude(out []float32, ch, hz int) float64 {
	var re, im float64
	n := len(out) / 2
	for i := range n {
		phase := 2 * math.Pi * float64(hz*i) / 48000
		x := float64(out[2*i+ch])
		re += x * math.Cos(phase)
		im += x * math.Sin(phase)
	}
	return 2 * math.Hypot(re, im) / float64(n)
}

// fold returns the frequency that hz plays at at the output's rate.
func fold(hz int) int {
	hz %= sampleRate
	return min(hz, sampleRate-hz)
}

// Downsampled, what is below 20 kHz plays as it is, within 0.1 dB, and
// where the source is.
func TestDownsamplePassband(t *testing.T) {
	for _, rate := range []int{88200, 96000, 192000} {
		for _, hz := range [][2]float64{{1000, 15000}, {6000, 20000}} {
			setup(t)
			p := NewSound(rate, 2, sine(rate, 0.5, hz[0], hz[1])).Play()
			out := render(9600)
			for c, f := range hz {
				got := measuredAmplitude(out[2*4800:], c, int(f))
				if db := 20 * math.Log10(got/0.5); math.Abs(db) > 0.1 {
					t.Errorf("%d Hz at %d Hz plays at %+.3f dB", int(f), rate, db)
				}
			}
			if got, want := p.Position(), 200*time.Millisecond; got-want > time.Microsecond || want-got > time.Microsecond {
				t.Errorf("at %d Hz: Position = %v, want %v", rate, got, want)
			}
		}
	}
}

// Downsampled, at the sound's rate or its speed, what is above 24 kHz
// does not fold back below it: 80 dB down from 24.5 kHz.
func TestDownsampleStopband(t *testing.T) {
	// A sound at rate played at speed: a tone of pass Hz on the left, one
	// of stop Hz on the right.
	type tones struct {
		rate       int
		speed      float64
		pass, stop int
	}
	var cases []tones
	for _, rate := range []int{88200, 96000, 192000} {
		for hz := 24500; hz < rate/2; hz += 3000 + hz/4 {
			cases = append(cases, tones{rate, 1, 1000, hz})
		}
	}
	cases = append(cases,
		tones{48000, 1.25, 1000, 20000}, // 25 kHz
		tones{44100, 2, 1000, 15000},    // 30 kHz
		tones{96000, 3, 1000, 9000},     // 27 kHz
		// Wider kernels than the ring holds.
		tones{48000, 12, 1000, 2500}, // 30 kHz
		tones{96000, 16, 500, 2000},  // 32 kHz
		tones{192000, 16, 250, 4000}, // 64 kHz
		tones{96000, 16, 500, 1580},  // 25.3 kHz
	)
	for _, tc := range cases {
		setup(t)
		p := NewSound(tc.rate, 2, sine(tc.rate, 0.25*tc.speed, float64(tc.pass), float64(tc.stop))).NewPlayer()
		p.SetSpeed(tc.speed).Play()
		out := render(9600)[2*4800:]
		heard := int(float64(tc.stop) * tc.speed)
		pass := measuredAmplitude(out, 0, int(float64(tc.pass)*tc.speed))
		alias := measuredAmplitude(out, 1, fold(heard))
		if db := 20 * math.Log10(pass/0.5); math.Abs(db) > 0.1 {
			t.Errorf("%d Hz at %v×: %v Hz plays at %+.3f dB", tc.rate, tc.speed, float64(tc.pass)*tc.speed, db)
		}
		if db := 20 * math.Log10(alias/0.5); db > -80 {
			t.Errorf("%d Hz at %v×: %d Hz folds to %d Hz at %.1f dB", tc.rate, tc.speed, heard, fold(heard), db)
		}
	}
}

// Downsampling goes on from one buffer to the next as if it mixed them
// all at once.
func TestDownsampleAcrossBuffers(t *testing.T) {
	const n = 9600
	samples := sine(88200, 0.2, 1000, 17000)
	for i := 1000; i < 1100; i++ {
		// A step, which rings.
		samples[2*i] += 0.3
	}
	setup(t)
	NewSound(88200, 2, samples).Play()
	whole := make([]float32, n*channels)
	mix(whole)
	setup(t)
	NewSound(88200, 2, samples).Play()
	parts := make([]float32, n*channels)
	sizes := []int{1, 7, 64, 333, 2, 480, 129}
	for i, k := 0, 0; i < n; k++ {
		m := min(sizes[k%len(sizes)], n-i)
		mix(parts[i*channels : (i+m)*channels])
		i += m
	}
	for i := range whole {
		if whole[i] != parts[i] {
			t.Fatalf("frame %d channel %d = %v in parts, %v at once", i/2, i%2, parts[i], whole[i])
		}
	}
}

// Changing speed, a voice goes from the spline to the sinc and back, and
// from one kernel to another, without a click.
func TestDownsampleSpeedChanges(t *testing.T) {
	setup(t)
	p := NewSound(sampleRate, 1, sine(sampleRate, 2, 1000)).NewPlayer()
	p.Play()
	var out []float32
	for _, s := range []float64{1, 1.5, 3, 0.8, 2, 9, 1} {
		p.SetSpeed(s)
		out = append(out, render(480)...)
	}
	// The steepest a sine of 0.5 at 9 kHz is.
	steep := 2 * math.Pi * 9000 / sampleRate * 0.5
	for i := 1; i < len(out)/2; i++ {
		if d := math.Abs(float64(out[2*i] - out[2*(i-1)])); d > 1.05*steep {
			t.Fatalf("a click at frame %d: %v to %v", i, out[2*(i-1)], out[2*i])
		}
	}
}

// Downsampling allocates nothing as it mixes, as its speed changes.
func TestDownsampleAllocatesNothing(t *testing.T) {
	setup(t)
	p := NewSound(96000, 2, sine(96000, 0.5, 200, 300)).NewPlayer().SetLoop(true)
	p.Play()
	buf := make([]float32, 480*channels)
	speeds := []float64{1, 1.3, 0.7, 3.7, 12, 1.001}
	i := 0
	if n := testing.AllocsPerRun(600, func() {
		mix(buf)
		if i++; i%50 == 0 {
			p.SetSpeed(speeds[i/50%len(speeds)])
		}
	}); n != 0 {
		t.Errorf("%v allocations a buffer", n)
	}
}
