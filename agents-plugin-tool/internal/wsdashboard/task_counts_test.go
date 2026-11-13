package wsdashboard

import (
	"github.com/kang-sw/devenv/internal/wslive"
	"strings"
	"testing"
)

func TestTaskCountsLiveMarkup(t *testing.T) {
	for _, n := range []int{0, 3} {
		snap := liveSnap(testNow, wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}})
		snap.Usage = &wslive.Usage{Background: &n, Monitors: &n}
		v := buildSessionView(pageInputs{Session: liveSession("self", "/proj", testNow), Snapshot: snap, Now: testNow})
		if v.Background != n || v.Monitors != n {
			t.Fatalf("counts: %+v", v)
		}
		set, err := renderSessionRegions(v, newEntryCache())
		if err != nil {
			t.Fatal(err)
		}
		want := `data-bg="0" data-monitors="0"`
		if n > 0 {
			want = `data-bg="3" data-monitors="3"`
		}
		if !strings.Contains(string(set[regionConvLive].HTML), want) {
			t.Fatalf("missing %s: %s", want, set[regionConvLive].HTML)
		}
	}
	bg, mon := 2, 1
	for _, tc := range []struct {
		h       *wslive.Host
		bg, mon int
	}{
		{&wslive.Host{BackgroundTasks: 5}, 5, 0},
		{&wslive.Host{BackgroundTasks: 5, Background: &bg, Monitors: &mon}, 2, 1},
	} {
		b, m := taskBadgeCounts(&wslive.Snapshot{Host: tc.h})
		if b != tc.bg || m != tc.mon {
			t.Fatalf("host counts: %d %d", b, m)
		}
	}
}
