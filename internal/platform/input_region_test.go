package platform

import "testing"

func TestInRegion(t *testing.T) {
	region := []Rect{{X: 10, Y: 20, Width: 30, Height: 40}, {X: 100, Y: 0, Width: 5, Height: 5}}
	for _, c := range []struct {
		x, y float64
		want bool
	}{
		{10, 20, true},     // the near corner
		{39.9, 59.9, true}, // just inside the far one
		{40, 30, false},    // the far edges are out
		{20, 60, false},
		{9.9, 30, false},
		{102, 2, true}, // the second rectangle
		{-1, -1, false},
	} {
		if got := InRegion(region, c.x, c.y); got != c.want {
			t.Errorf("InRegion(%v, %v) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	if InRegion(nil, 0, 0) || InRegion([]Rect{}, 0, 0) {
		t.Error("an empty region holds a point")
	}
	if InRegion([]Rect{{X: 5, Y: 5}}, 5, 5) {
		t.Error("a rectangle without area holds a point")
	}
}

func TestLetsMouseThrough(t *testing.T) {
	region := []Rect{{X: 0, Y: 0, Width: 10, Height: 10}}
	for _, c := range []struct {
		name          string
		x, y          float64
		held, through bool
		want          bool
	}{
		{"on the region", 5, 5, false, true, false},
		{"off the region", 50, 5, false, false, true},
		{"a press on the window dragged off it", 50, 5, true, false, false},
		{"a press behind dragged onto the region", 5, 5, true, true, true},
	} {
		if got := LetsMouseThrough(region, c.x, c.y, c.held, c.through); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if !LetsMouseThrough([]Rect{}, 5, 5, false, false) {
		t.Error("an empty region takes the mouse")
	}
}
