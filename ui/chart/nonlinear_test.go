package chart

import (
	"math"
	"slices"
	"testing"
)

func TestLogScale(t *testing.T) {
	for _, tc := range []struct {
		lo, hi, v float64
		want      float32
	}{
		{1, 1000, 10, 1.0 / 3}, {1, 1000, 100, 2.0 / 3}, {1000, 1, 10, 2.0 / 3},
		{-1000, -1, -10, 2.0 / 3}, {-1, -1000, -10, 1.0 / 3}, {5, 5, 100, .5},
	} {
		s := NewLog(tc.lo, tc.hi)
		if got := s.At(tc.v); math.Abs(float64(got-tc.want)) > 1e-6 {
			t.Errorf("%+v: got %v", tc, got)
		}
		if tc.lo != tc.hi {
			if got := s.Invert(s.At(tc.v)); math.Abs((got-tc.v)/tc.v) > 1e-6 {
				t.Errorf("roundtrip: %v -> %v", tc.v, got)
			}
		}
	}
	s := NewLog(1, 100)
	if got := s.Span(1, 100); got != (Span{0, 1}) {
		t.Fatal(got)
	}
	s.Clamp = true
	if s.At(-10) != 0 || s.At(1000) != 1 || s.Invert(-1) != 1 || s.Invert(2) != 100 {
		t.Fatal("clamp failed")
	}
	if got := NewLog(3, 73).Nice(); got.Lo != 1 || got.Hi != 100 {
		t.Fatal(got)
	}
	if got := NewLog(-73, -3).Nice(); got.Lo != -100 || got.Hi != -1 {
		t.Fatal(got)
	}
	s = NewLog(1, 100)
	values := tickValues(s.Ticks(10))
	want := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if !slices.Equal(values, want) {
		t.Fatalf("ticks %v", values)
	}
	s = NewLog(1, 32)
	s.Base = 2
	if got := tickValues(s.Ticks(10)); !slices.Equal(got, []float64{1, 2, 4, 8, 16, 32}) {
		t.Fatal(got)
	}
}

func tickValues(ts []Tick) []float64 {
	out := make([]float64, len(ts))
	for i, t := range ts {
		out[i] = t.Value
	}
	return out
}

func TestSymLogScale(t *testing.T) {
	s := NewSymLog(-100, 100)
	if s.At(0) != .5 || s.Invert(.5) != 0 {
		t.Fatal("zero not at center")
	}
	if math.Abs(float64(s.At(1))-(.5+math.Log(2)/(2*math.Log(101)))) > 1e-6 {
		t.Fatal(s.At(1))
	}
	if got := s.Span(-100, 100); got != (Span{0, 1}) {
		t.Fatal(got)
	}
	if !slices.Equal(tickValues(s.Ticks(4)), []float64{-100, -50, 0, 50, 100}) {
		t.Fatal(s.Ticks(4))
	}
	nice := NewSymLog(-3.2, 7.9).Nice(6)
	if nice.Lo != -4 || nice.Hi != 8 {
		t.Fatal(nice)
	}
	for _, constant := range []float64{.1, 1, 10} {
		s.Constant = constant
		for _, v := range []float64{-100, -10, -.01, 0, .01, 10, 100} {
			if got := s.Invert(s.At(v)); math.Abs(got-v) > 1e-4 {
				t.Errorf("constant %v: %v -> %v", constant, v, got)
			}
		}
	}
	s.Clamp = true
	if s.At(-1000) != 0 || s.At(1000) != 1 || s.Invert(-1) != -100 || s.Invert(2) != 100 {
		t.Fatal("clamp failed")
	}
}

func TestNonlinearInvalid(t *testing.T) {
	for _, s := range []Log{NewLog(0, 100), NewLog(-1, 1), NewLog(1, math.Inf(1)), {Lo: 1, Hi: 10, Base: 1}, {Lo: 1, Hi: 10, Base: -2}} {
		if !math.IsNaN(float64(s.At(1))) || !math.IsNaN(s.Invert(.5)) || len(s.Ticks(5)) != 0 {
			t.Fatalf("accepted invalid log %+v", s)
		}
	}
	for _, s := range []SymLog{{Lo: -1, Hi: 1, Constant: -1}, {Lo: -1, Hi: 1, Constant: math.Inf(1)}, NewSymLog(math.NaN(), 1)} {
		if !math.IsNaN(float64(s.At(0))) || !math.IsNaN(s.Invert(.5)) || len(s.Ticks(5)) != 0 {
			t.Fatalf("accepted invalid symlog %+v", s)
		}
	}
	if !math.IsNaN(float64(NewLog(1, 100).At(-1))) {
		t.Fatal("wrong sign accepted")
	}
	if !math.IsNaN(float64(NewSymLog(-1, 1).At(math.NaN()))) {
		t.Fatal("NaN did not propagate")
	}
}

func TestNonlinearEdgesAndLabels(t *testing.T) {
	for _, count := range []int{0, -1} {
		if len(NewLog(1, 100).Ticks(count)) != 0 || len(NewSymLog(-1, 1).Ticks(count)) != 0 {
			t.Fatal("nonpositive count has ticks")
		}
		s := NewSymLog(-1, 1).Nice(count)
		if s.Lo != -1 || s.Hi != 1 {
			t.Fatal("nonpositive nice count changed the domain")
		}
	}
	for _, base := range []float64{2, math.E, 10} {
		for _, value := range []float64{math.SmallestNonzeroFloat64, 5, math.MaxFloat64} {
			s := NewLog(value, value)
			s.Base = base
			ts := s.Ticks(5)
			if len(ts) != 1 || ts[0].Value != value || ts[0].At != .5 || ts[0].Label == "" {
				t.Fatal("constant domain lost its tick", ts)
			}
		}
	}
	s := NewLog(1, 100)
	blank := false
	for _, tick := range s.Ticks(5) {
		blank = blank || tick.Label == ""
	}
	// A denser minor grid can omit some labels, and a caller can label all.
	s = NewLog(1, 100)
	s.Format = func(float64) string { return "custom" }
	for _, tick := range s.Ticks(5) {
		if tick.Label != "custom" {
			t.Fatal(tick)
		}
	}
	if !blank {
		t.Fatal("crowded minor tick labels were not suppressed")
	}
	sy := NewSymLog(-10, 10)
	sy.Format = s.Format
	for _, tick := range sy.Ticks(4) {
		if tick.Label != "custom" {
			t.Fatal(tick)
		}
	}
	lo := 1e300
	hi := math.Nextafter(lo, math.Inf(1))
	close := NewLog(lo, hi)
	if close.At(lo) != 0 || close.At(hi) != 1 || close.Invert(0) != lo || close.Invert(1) != hi {
		t.Fatal("close endpoints lost precision")
	}
	wide := NewLog(math.SmallestNonzeroFloat64, math.MaxFloat64)
	if wide.At(wide.Lo) != 0 || wide.At(wide.Hi) != 1 || !finiteScale(wide.Invert(.5)) {
		t.Fatal("wide log domain failed")
	}
	n := wide.Nice()
	if !n.valid() {
		t.Fatal("nice made an extreme domain invalid")
	}
	for _, constant := range []float64{math.SmallestNonzeroFloat64, 1e-100, 1, math.MaxFloat64} {
		s := NewSymLog(-1e300, 1e300)
		s.Constant = constant
		if s.At(0) != .5 || s.Invert(.5) != 0 || s.At(s.Lo) != 0 || s.At(s.Hi) != 1 {
			t.Fatalf("extreme constant %v failed", constant)
		}
		if !finiteScale(s.Invert(.75)) {
			t.Fatalf("extreme constant %v overflowed inverse", constant)
		}
	}
	sparse := NewSymLog(-100, 100).Nice(1)
	if sparse.Lo != -100 || sparse.Hi != 100 {
		t.Fatal("sparse nice request expanded without converging")
	}
}

func FuzzNonlinearRoundtrip(f *testing.F) {
	for _, v := range [][3]float64{{-10, 20, .3}, {-300, 300, .5}, {-.001, .001, .75}, {0, 1, .25}} {
		f.Add(v[0], v[1], v[2])
	}
	f.Fuzz(func(t *testing.T, a, b, fraction float64) {
		if !finiteScale(a) || !finiteScale(b) || !finiteScale(fraction) || a < -300 || a > 300 || b < -300 || b > 300 || math.Abs(a-b) < .01 {
			return
		}
		fraction = math.Abs(fraction - math.Trunc(fraction))
		s := NewLog(math.Pow(10, a), math.Pow(10, b))
		point := float32(fraction)
		value := s.Invert(point)
		if got := s.At(value); math.IsNaN(float64(got)) || math.Abs(float64(got-point)) > 2e-5 {
			t.Fatalf("log %v %v: %v -> %v -> %v", a, b, point, value, got)
		}
		sy := NewSymLog(-math.Abs(a), math.Abs(b))
		value = sy.Invert(point)
		if got := sy.At(value); math.IsNaN(float64(got)) || math.Abs(float64(got-point)) > 2e-5 {
			t.Fatalf("symlog: %v -> %v -> %v", point, value, got)
		}
	})
}

func TestNonlinearMappingAllocs(t *testing.T) {
	logScale, signed := NewLog(.01, 1000), NewSymLog(-1000, 1000)
	if n := testing.AllocsPerRun(100, func() {
		logScale.At(10)
		logScale.Invert(.25)
		signed.At(-10)
		signed.Invert(.75)
	}); n != 0 {
		t.Fatalf("scale mapping allocates %v times", n)
	}
}

func TestNonlinearSubnormal(t *testing.T) {
	// Equal binary exponent spans must map to equal distances even below
	// the smallest normal float64; the midpoint is exactly 2^-537.
	mid := math.Exp2(-537)
	s := NewLog(math.SmallestNonzeroFloat64, 1)
	if got := s.At(mid); math.Abs(float64(got)-.5) > 1e-6 {
		t.Fatalf("subnormal log midpoint: %v", got)
	}
	if got := s.Invert(.5); math.Abs(got/mid-1) > 1e-12 {
		t.Fatalf("subnormal log inverse: %g want %g", got, mid)
	}
	sy := NewSymLog(-1, 1)
	sy.Constant = math.SmallestNonzeroFloat64
	if got := sy.Invert(.75); math.Abs(got/mid-1) > 1e-12 {
		t.Fatalf("subnormal symlog constant: %g want %g", got, mid)
	}
}
