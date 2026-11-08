package uses_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/native/internal/roundtrip"
	"github.com/egoist/mygo/native/internal/roundtrip/uses"
)

// mixer makes the module's implementation, skipping where it is not built:
// mygo native build -target host native/internal/roundtrip/uses.
func mixer(t *testing.T) uses.Mixer {
	t.Helper()
	m, err := uses.NewMixer()
	if errors.Is(err, errors.ErrUnsupported) {
		t.Skip("no implementation on this platform")
	}
	if err != nil && strings.Contains(err.Error(), "mygo native build") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestOtherModule(t *testing.T) {
	m := mixer(t)
	p, err := m.Pair(uses.Pair{R: roundtrip.Record{Name: "r", Counts: []int{1, 2, 3}, Inner: roundtrip.Inner{X: 1, Y: 2}, Kind: roundtrip.KindC}})
	if err != nil || p.N != 6 || p.R.Name != "r" || p.R.Kind != roundtrip.KindC || p.Inner == nil || *p.Inner != (roundtrip.Inner{X: 1, Y: 2}) {
		t.Fatalf("Pair = %+v, %v", p, err)
	}
	if got := m.Kinds([]roundtrip.Record{{Kind: roundtrip.KindB}, {Kind: roundtrip.KindA}}); !slices.Equal(got, []roundtrip.Kind{roundtrip.KindB, roundtrip.KindA}) {
		t.Fatalf("Kinds = %v", got)
	}
	e, err := roundtrip.NewEcho()
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.Counter(7)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := m.Next(c), c.Next(); a != 7 || b != 8 {
		t.Fatalf("Next = %d, then %d", a, b)
	}
}
