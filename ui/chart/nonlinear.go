package chart

import (
	"math"
	"slices"
	"strconv"
)

// Log maps a domain of positive numbers, or of negative numbers, onto
// the plot with equal ratios the same distance apart, as d3.scaleLog.
// Zero and domains crossing zero are invalid; use SymLog for those.
// Lo/Hi may be reversed. An invalid domain or base produces NaN positions
// and no ticks, rather than a misleading linear plot.
type Log struct {
	Lo, Hi float64
	// Base selects the powers used by Nice and Ticks, 10 when zero.
	// It must be finite and greater than 1. Changing it does not change At.
	Base  float64
	Clamp bool
	// Format labels every tick when set. The default uses six significant
	// digits and omits crowded minor labels while retaining their ticks.
	Format func(float64) string
}

// NewLog returns a logarithmic scale from lo to hi, with base 10.
func NewLog(lo, hi float64) Log { return Log{Lo: lo, Hi: hi} }

func finiteScale(v float64) bool           { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func clampValue(v, lo, hi float64) float64 { return max(min(lo, hi), min(max(lo, hi), v)) }
func (s Log) base() float64 {
	if s.Base == 0 {
		return 10
	}
	return s.Base
}
func (s Log) valid() bool {
	b := s.base()
	return finiteScale(s.Lo) && finiteScale(s.Hi) && finiteScale(b) && b > 1 &&
		(s.Lo > 0 && s.Hi > 0 || s.Lo < 0 && s.Hi < 0)
}

// Normalize subnormals before Log, whose architecture-specific paths can
// otherwise lose their exponent. Scaling by a power of two is exact.
func positiveLog(v float64) float64 {
	if v > 0 && v < 0x1p-1022 {
		return math.Log(v*0x1p52) - 52*math.Ln2
	}
	return math.Log(v)
}

// logRatio avoids overflowing a ratio spanning the float64 range, and
// preserves the difference of endpoints whose logarithms round alike.
func logRatio(v, lo float64) float64 {
	relative := (v - lo) / lo
	if math.Abs(relative) < .5 {
		return math.Log1p(relative)
	}
	return positiveLog(v) - positiveLog(lo)
}

// At returns where v is along the plot. Without Clamp, a value of the
// opposite sign produces NaN, and zero maps to infinity off the plot.
// NaN propagates, so chart paths can leave a gap there.
func (s Log) At(v float64) float32 {
	if !s.valid() || math.IsNaN(v) {
		return float32(math.NaN())
	}
	if s.Clamp {
		v = clampValue(v, s.Lo, s.Hi)
	}
	if s.Lo > 0 && v < 0 || s.Lo < 0 && v > 0 {
		return float32(math.NaN())
	}
	if s.Lo == s.Hi {
		return .5
	}
	return float32(logRatio(math.Abs(v), math.Abs(s.Lo)) / logRatio(math.Abs(s.Hi), math.Abs(s.Lo)))
}

// Invert returns the value at a fraction of the plot.
func (s Log) Invert(f float32) float64 {
	if !s.valid() || math.IsNaN(float64(f)) {
		return math.NaN()
	}
	if s.Clamp {
		f = max(0, min(1, f))
	}
	if f == 0 || s.Lo == s.Hi {
		return s.Lo
	}
	if f == 1 {
		return s.Hi
	}
	lo := math.Abs(s.Lo)
	r := logRatio(math.Abs(s.Hi), lo)
	var v float64
	if math.Abs(r) < .5 {
		v = lo + lo*math.Expm1(float64(f)*r)
	} else {
		v = math.Exp(positiveLog(lo) + float64(f)*r)
	}
	return math.Copysign(v, s.Lo)
}

// Span returns the stretch from a to b along the plot. A logarithmic bar
// needs a nonzero baseline; zero cannot be its lower edge.
func (s Log) Span(a, b float64) Span { return Span{s.At(a), s.At(b)} }

func logBase(v, base float64) float64 {
	if v < 0x1p-1022 {
		return positiveLog(v) / math.Log(base)
	}
	switch base {
	case 10:
		return math.Log10(v)
	case 2:
		return math.Log2(v)
	case math.E:
		return math.Log(v)
	}
	return math.Log(v) / math.Log(base)
}
func powerBase(exponent, base float64) float64 {
	if base == 10 && exponent == math.Trunc(exponent) && exponent >= -324 && exponent <= 309 {
		return math.Pow10(int(exponent))
	}
	if base == 2 {
		return math.Exp2(exponent)
	}
	if base == math.E {
		return math.Exp(exponent)
	}
	return math.Pow(base, exponent)
}

// Nice extends the domain out to powers of Base. At the float64 limits,
// an edge whose next power is zero or infinite keeps its original value.
// Invalid scales are unchanged.
func (s Log) Nice() Log {
	if !s.valid() {
		return s
	}
	lo, hi := min(math.Abs(s.Lo), math.Abs(s.Hi)), max(math.Abs(s.Lo), math.Abs(s.Hi))
	a, b := powerBase(math.Floor(logBase(lo, s.base())), s.base()), powerBase(math.Ceil(logBase(hi, s.base())), s.base())
	if a > 0 && finiteScale(a) && a <= lo {
		lo = a
	}
	if b > 0 && finiteScale(b) && b >= hi {
		hi = b
	}
	if math.Abs(s.Lo) > math.Abs(s.Hi) {
		lo, hi = hi, lo
	}
	s.Lo, s.Hi = math.Copysign(lo, s.Lo), math.Copysign(hi, s.Hi)
	return s
}

// Ticks marks powers of Base and, for integer bases on a short domain,
// their multiples, as d3.scaleLog does. Count is a density hint; minor
// ticks may outnumber it. Sparse domains fall back to round linear ticks.
// Nonpositive counts return no ticks.
func (s Log) Ticks(count int) []Tick {
	if !s.valid() || count <= 0 {
		return nil
	}
	lo, hi := min(math.Abs(s.Lo), math.Abs(s.Hi)), max(math.Abs(s.Lo), math.Abs(s.Hi))
	base := s.base()
	a, b := logBase(lo, base), logBase(hi, base)
	var values []float64
	// Bound the work of minor ticks for unusually large integer bases;
	// powers still work for every valid base.
	if base == math.Trunc(base) && b-a < float64(count) && base*math.Ceil(b-a+2) <= 10000 {
		for exponent := math.Floor(a); exponent <= math.Ceil(b); exponent++ {
			p := powerBase(exponent, base)
			for k := 1.0; k < base; k++ {
				v := k * p
				if exponent < 0 {
					if divisor := powerBase(-exponent, base); finiteScale(divisor) {
						v = k / divisor
					}
				}
				if v >= lo && v <= hi && v > 0 && finiteScale(v) {
					values = append(values, v)
				}
			}
		}
		if len(values)*2 < count {
			values = ticks(lo, hi, float64(count))
		}
	} else {
		for _, e := range ticks(a, b, min(b-a, float64(count))) {
			v := powerBase(e, base)
			if v > 0 && finiteScale(v) {
				values = append(values, v)
			}
		}
	}
	if lo == hi {
		values = []float64{lo}
	}
	if s.Lo < 0 {
		for i := range values {
			values[i] = -values[i]
		}
		slices.Reverse(values)
	}
	if s.Hi < s.Lo {
		slices.Reverse(values)
	}
	out := make([]Tick, len(values))
	threshold := max(1, base*float64(count)/float64(len(values)))
	for i, v := range values {
		label := ""
		if s.Format != nil {
			label = s.Format(v)
		} else {
			exponent := math.Floor(logBase(math.Abs(v), base) + 1e-12)
			coefficient := math.Abs(v) / powerBase(exponent, base)
			if len(values) == 1 || coefficient <= threshold+1e-10 {
				label = strconv.FormatFloat(v, 'g', 6, 64)
			}
		}
		out[i] = Tick{At: s.At(v), Value: v, Label: label}
	}
	return out
}

// SymLog is a symmetric logarithmic scale: sign(v)*log1p(abs(v)/Constant).
// Unlike Log it includes zero and negative values. It is nearly linear
// near zero and logarithmic for larger magnitudes, as d3.scaleSymlog.
// Ticks and Nice choose round values in the original data units.
type SymLog struct {
	Lo, Hi float64
	// Constant sets the size of the linear region, 1 when zero. It must
	// be finite and positive; a larger constant makes more of the plot linear.
	Constant float64
	Clamp    bool
	Format   func(float64) string
}

// NewSymLog returns a symmetric logarithmic scale from lo to hi.
func NewSymLog(lo, hi float64) SymLog { return SymLog{Lo: lo, Hi: hi} }
func (s SymLog) constant() float64 {
	if s.Constant == 0 {
		return 1
	}
	return s.Constant
}
func (s SymLog) valid() bool {
	return finiteScale(s.Lo) && finiteScale(s.Hi) && finiteScale(s.constant()) && s.constant() > 0
}
func symLog(v, c float64) float64 {
	r := math.Abs(v) / c
	if math.IsInf(r, 0) {
		return math.Copysign(positiveLog(math.Abs(v))-positiveLog(c), v)
	}
	return math.Copysign(math.Log1p(r), v)
}
func symExp(v, c float64) float64 {
	if math.Abs(v) > 700 {
		return math.Copysign(math.Exp(positiveLog(c)+math.Abs(v)), v)
	}
	return math.Copysign(c*math.Expm1(math.Abs(v)), v)
}

// At returns where v is along the plot; NaN propagates as a missing value.
func (s SymLog) At(v float64) float32 {
	if !s.valid() || math.IsNaN(v) {
		return float32(math.NaN())
	}
	if s.Clamp {
		v = clampValue(v, s.Lo, s.Hi)
	}
	a, b := symLog(s.Lo, s.constant()), symLog(s.Hi, s.constant())
	if a == b {
		return (Linear{Lo: s.Lo, Hi: s.Hi}).At(v)
	}
	return float32((symLog(v, s.constant()) - a) / (b - a))
}

// Invert returns the value at a fraction of the plot.
func (s SymLog) Invert(f float32) float64 {
	if !s.valid() || math.IsNaN(float64(f)) {
		return math.NaN()
	}
	if s.Clamp {
		f = max(0, min(1, f))
	}
	if f == 0 || s.Lo == s.Hi {
		return s.Lo
	}
	if f == 1 {
		return s.Hi
	}
	a, b := symLog(s.Lo, s.constant()), symLog(s.Hi, s.constant())
	if a == b {
		return (Linear{Lo: s.Lo, Hi: s.Hi}).Invert(f)
	}
	return symExp(a+float64(f)*(b-a), s.constant())
}

// Span returns the stretch from a to b along the plot.
func (s SymLog) Span(a, b float64) Span { return Span{s.At(a), s.At(b)} }

// Nice extends the domain to round data values, as Linear.Nice does.
func (s SymLog) Nice(count int) SymLog {
	if !s.valid() || count <= 0 {
		return s
	}
	n := NewLinear(s.Lo, s.Hi).Nice(count)
	if !finiteScale(n.Lo) || !finiteScale(n.Hi) {
		return s
	}
	// Very sparse requests can fail to converge while round domains
	// repeatedly expand. Keep the original domain rather than returning
	// the linear helper's last intermediate result.
	again := n.Nice(count)
	if again.Lo != n.Lo || again.Hi != n.Hi {
		return s
	}
	s.Lo, s.Hi = n.Lo, n.Hi
	return s
}

// Ticks returns round values, spaced nonlinearly along the plot.
func (s SymLog) Ticks(count int) []Tick {
	if !s.valid() {
		return nil
	}
	out := (Linear{Lo: s.Lo, Hi: s.Hi, Format: s.Format}).Ticks(count)
	for i := range out {
		out[i].At = s.At(out[i].Value)
	}
	return out
}
