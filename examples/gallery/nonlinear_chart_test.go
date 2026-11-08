package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNonlinearCharts(t *testing.T) {
	g := &gallery{chart: newCharts()}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).Gap(12).Children(func() { g.latencyChart(c); g.signedChart(c) })
	}, 700, 650)
	point := func(label string) ui.Rect {
		t.Helper()
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("missing mark %q", label)
		}
		return r
	}
	logGap := math.Abs(float64(point("Local latency").Y - point("Cache latency").Y))
	if err := tt.Click("Logarithmic scale"); err != nil {
		t.Fatal(err)
	}
	linearGap := math.Abs(float64(point("Local latency").Y - point("Cache latency").Y))
	if logGap < 20 || linearGap > 1 {
		t.Fatalf("small samples: log gap %v, linear gap %v", logGap, linearGap)
	}
	before := math.Abs(float64(point("Change +1").Y - point("Change +0").Y))
	if err := tt.Click("c=100"); err != nil {
		t.Fatal(err)
	}
	after := math.Abs(float64(point("Change +1").Y - point("Change +0").Y))
	if after >= before {
		t.Fatalf("larger linear region did not bring the small change closer to zero: %v -> %v", before, after)
	}
}
