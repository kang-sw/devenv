package wstracker

import (
	"math"
	"testing"
)

// TestIntOfBounds pins intOf at its edges: Go's float-to-int conversion of an
// out-of-range value is platform-defined, so an integer beyond the int32
// range must report not-ok on every platform rather than convert.
func TestIntOfBounds(t *testing.T) {
	for _, c := range []struct {
		in   any
		want int
		ok   bool
	}{
		{0.0, 0, true},
		{-3.0, -3, true},
		{float64(math.MaxInt32), math.MaxInt32, true},
		{float64(math.MinInt32), math.MinInt32, true},
		{float64(math.MaxInt32) + 1, 0, false},
		{float64(math.MinInt32) - 1, 0, false},
		{1e19, 0, false},
		{-1e19, 0, false},
		{1.5, 0, false},
		{math.Inf(1), 0, false},
		{math.NaN(), 0, false},
		{"3", 0, false},
		{nil, 0, false},
	} {
		got, ok := obj{"v": c.in}.intOf("v")
		if got != c.want || ok != c.ok {
			t.Errorf("intOf(%v) = %d, %v; want %d, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestPosIntOf pins the max_bytes reading: any positive integer the
// reference engine accepts (Number.isInteger && > 0), held as the engine's
// float64 with no upper bound.
func TestPosIntOf(t *testing.T) {
	for _, c := range []struct {
		in   any
		want float64
		ok   bool
	}{
		{1.0, 1, true},
		{1e19, 1e19, true},
		{1e300, 1e300, true},
		{0.0, 0, false},
		{-1.0, 0, false},
		{2.5, 0, false},
		{math.Inf(1), 0, false},
		{math.NaN(), 0, false},
		{"5", 0, false},
	} {
		got, ok := obj{"v": c.in}.posIntOf("v")
		if got != c.want || ok != c.ok {
			t.Errorf("posIntOf(%v) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestJSNum pins the engine's number-to-string form (Number.prototype.toString)
// at the exponent thresholds.
func TestJSNum(t *testing.T) {
	for in, want := range map[float64]string{
		0: "0", 100: "100", 1e19: "10000000000000000000", 1e20: "100000000000000000000",
		1e21: "1e+21", 2.5e21: "2.5e+21", -1e21: "-1e+21", 1.5: "1.5", 1e-6: "0.000001", 1e-7: "1e-7",
	} {
		if got := jsNum(in); got != want {
			t.Errorf("jsNum(%v) = %q, want %q", in, got, want)
		}
	}
}
