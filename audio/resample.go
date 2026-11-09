package audio

import (
	"math"
	"sync"
)

// A voice that plays more than one source frame an output frame, a sound
// of more than 48 kHz or one sped up, would fold what it has above the
// output's Nyquist frequency, 24 kHz, back below it: a spline does not
// remove it. Such a voice interpolates with a windowed sinc instead, a
// low-pass at the output's rate: a Kaiser window over sincReach output
// frames either side, flat to 20 kHz, -6 dB at 22 kHz and at least 80 dB
// down from 24 kHz. In source frames, the kernel is as much wider as the
// source is faster, and its cutoff as much lower.
const (
	sincReach  = 32                   // output frames either side
	sincCutoff = 22000.0 / sampleRate // of the output's rate
	sincBeta   = 7.857                // the Kaiser window's: 80 dB
	sincRes    = 256                  // entries of sincTable an output frame
	// The kernels a voice keeps for the phases between two source frames
	// (as many an output frame, so fewer the faster), and interpolates
	// between.
	sincPhases = 128

	// The most source frames the kernel reaches either side: the whole
	// kernel up to 8 source frames an output frame. Faster, the window
	// narrows to these, which widens the band from what it passes to what
	// it stops as much, and the cutoff comes down for that band to end at
	// 24 kHz still, as far as half of it.
	maxReach = 256
	// The frames a voice keeps around where it is: enough for the kernel
	// to grow to maxReach while it holds frames as far ahead.
	ringFrames = 2 * maxReach

	// The most weights a sincBank holds at any step: (rows+1)×2×reach.
	maxWeights = 2*sincPhases*sincReach + 2*sincPhases + 4*maxReach + 4
)

// A sincBank is the kernel at a step, in source frames an output frame:
// the weights of the 2×reach frames from reach-1 before the output's to
// reach after it, for each of rows+1 phases from 0 to 1, each with the
// difference to the next phase's.
type sincBank struct {
	step   float64
	reach  int
	rows   int
	cutoff float64 // of the output's rate
	w      [][2]float32
}

// set makes the bank's kernels those at step, unless they are.
func (b *sincBank) set(step float64) {
	if b.step == step {
		return
	}
	if b.w == nil {
		b.w = make([][2]float32, 0, maxWeights)
	}
	b.step = step
	b.reach = min(int(math.Ceil(sincReach*step)), maxReach)
	b.rows = int(math.Ceil(sincPhases / step))
	b.cutoff = sincCutoff
	if sincReach*step > maxReach {
		// Half the band from what it passes to what it stops, as many
		// times wider as the window is narrower.
		half := (0.5 - sincCutoff) * sincReach * step / maxReach
		b.cutoff = max(0.5-half, half)
	}
	n := 2 * b.reach
	b.w = b.w[:(b.rows+1)*n]
	for p := range b.rows + 1 {
		b.kernel(b.w[p*n:(p+1)*n], float64(p)/float64(b.rows))
	}
	for p := range b.rows {
		w, next := b.w[p*n:(p+1)*n], b.w[(p+1)*n:(p+2)*n]
		for j := range w {
			w[j][1] = next[j][0] - w[j][0]
		}
	}
}

// kernel sets the weights in w for an output phase past the frame reach-1
// after w's first, summing to 1 so that what is constant stays as it is.
func (b *sincBank) kernel(w [][2]float32, phase float64) {
	d0 := float64(1-b.reach) - phase // source frames from the output to w's first
	var sum float64
	if sincReach*b.step <= maxReach {
		t := sincTable()
		dx := sincRes / b.step
		for j := range w {
			x := math.Abs(d0+float64(j)) * dx
			i := int(x)
			h := float64(t[i][0]) + (x-float64(i))*float64(t[i][1])
			w[j][0] = float32(h)
			sum += h
		}
	} else {
		// The ring cannot hold the whole kernel: the sinc at the cutoff,
		// by rotation from one frame to the next, in a window as wide as
		// the frames reached.
		k := kaiserTable()
		end := len(k) - 1
		dk := float64(end) / float64(b.reach)
		a := math.Pi * 2 * b.cutoff / b.step // radians a source frame
		s, c := math.Sincos(a * d0)
		sa, ca := math.Sincos(a)
		for j := range w {
			d := d0 + float64(j)
			h := 0.0
			if u := math.Abs(d) * dk; int(u) < end {
				i := int(u)
				h = k[i] + (u-float64(i))*(k[i+1]-k[i])
				if th := a * d; math.Abs(th) > 1e-9 {
					h *= s / th
				}
			}
			w[j][0] = float32(h)
			sum += h
			s, c = s*ca+c*sa, c*ca-s*sa
		}
	}
	for j := range w {
		w[j][0] = float32(float64(w[j][0]) / sum)
	}
}

// sinc interpolates the voice's frames where the output is with its bank's
// kernel, between those of the phases either side.
func (v *voice) sinc() (l, r float32) {
	b := &v.bank
	n := 2 * b.reach
	u := v.phase * float64(b.rows)
	p := int(u)
	f := float32(u - float64(p))
	w := b.w[p*n : (p+1)*n]
	x := v.ring[(v.idx-int64(b.reach)+1)&(ringFrames-1):]
	x = x[:len(w)]
	switch {
	case v.mono:
		for j, e := range w {
			l += (e[0] + f*e[1]) * x[j][0]
		}
		return l, l
	case f == 0:
		// On a phase of the bank, as every frame at a whole number of
		// source frames an output frame.
		for j, e := range w {
			l += e[0] * x[j][0]
			r += e[0] * x[j][1]
		}
	default:
		for j, e := range w {
			h, s := e[0]+f*e[1], x[j]
			l += h * s[0]
			r += h * s[1]
		}
	}
	return l, r
}

// sincTable is the kernel from 0 to sincReach output frames, sincRes
// entries a frame, each with the difference to the next for interpolating
// between them, and zeros as far after as a kernel's last frame reaches.
var sincTable = sync.OnceValue(func() [][2]float32 {
	n := sincReach * sincRes
	t := make([][2]float32, n+2*sincRes)
	next := 0.0
	for i := n; i >= 0; i-- {
		x := float64(i) / sincRes
		h := sinc(2*sincCutoff*x) * kaiser(x/sincReach)
		t[i] = [2]float32{float32(h), float32(next - h)}
		next = h
	}
	return t
})

// kaiserTable is the window from its middle to its edge, for kernels the
// ring cannot hold whole.
var kaiserTable = sync.OnceValue(func() []float64 {
	t := make([]float64, 1025)
	for i := range t {
		t[i] = kaiser(float64(i) / float64(len(t)-1))
	}
	return t
})

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

// kaiser returns the Kaiser window at u, from -1 to 1.
func kaiser(u float64) float64 {
	return bessel0(sincBeta*math.Sqrt(max(1-u*u, 0))) / bessel0(sincBeta)
}

// bessel0 returns the modified Bessel function of the first kind, of order
// zero.
func bessel0(x float64) float64 {
	sum, term := 1.0, 1.0
	for k := 1; term > 1e-12*sum; k++ {
		h := x / 2 / float64(k)
		term *= h * h
		sum += term
	}
	return sum
}
