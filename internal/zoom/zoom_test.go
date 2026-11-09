package zoom

import (
	"math"
	"testing"
)

func TestToRaw(t *testing.T) {
	r := Range{Min: 100, Max: 400, Step: 1}
	cases := []struct {
		mag, max float64
		want     int32
	}{
		{1, 4, 100}, {4, 4, 400}, {2, 4, 200}, {2.5, 4, 250},
		{0, 4, 100}, {9, 4, 400}, {math.NaN(), 4, 100},
		{2, 1, 100}, // no magnification available
	}
	for _, c := range cases {
		if got := ToRaw(c.mag, c.max, r); got != c.want {
			t.Errorf("ToRaw(%v,%v) = %d; want %d", c.mag, c.max, got, c.want)
		}
	}
}

func TestToRawStepAligned(t *testing.T) {
	r := Range{Min: 0, Max: 10, Step: 4}
	if got := ToRaw(1.5, 2, r); got != 4 { // 5 -> nearest step 4
		t.Errorf("got %d", got)
	}
	if got := ToRaw(2, 2, r); got != 10 { // 12 clamped to max
		t.Errorf("got %d", got)
	}
}

func TestInvalidRange(t *testing.T) {
	r := Range{Min: 5, Max: 5}
	if ToRaw(2, 2, r) != 5 || FromRaw(5, 2, r) != 1 {
		t.Error("invalid range not handled")
	}
}

func TestRoundTrip(t *testing.T) {
	r := Range{Min: 100, Max: 500, Step: 1}
	for _, m := range []float64{1, 1.5, 2, 3, 4} {
		if got := FromRaw(ToRaw(m, 4, r), 4, r); math.Abs(got-m) > 0.01 {
			t.Errorf("%v -> %v", m, got)
		}
	}
}
