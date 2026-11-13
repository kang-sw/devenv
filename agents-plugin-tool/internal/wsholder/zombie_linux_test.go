//go:build linux

package wsholder

import "testing"

func TestZombieStat(t *testing.T) {
	for _, tc := range []struct {
		stat string
		want bool
	}{
		{"12 (holder) Z 1", true},
		{"12 (holder ) name) Z 1", true},
		{"12 (holder) S 1", false},
		{"invalid", false},
		{"12 (holder)", false},
	} {
		if got := zombieStat(tc.stat); got != tc.want {
			t.Errorf("zombieStat(%q) = %v, want %v", tc.stat, got, tc.want)
		}
	}
}
