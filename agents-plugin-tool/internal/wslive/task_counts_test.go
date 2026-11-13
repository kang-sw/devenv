package wslive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusTaskCounts(t *testing.T) {
	st := NewState("key")
	bg, mon := 2, 1
	st.Apply(Event{Kind: KindStatus, Background: &bg, Monitors: &mon}, 1)
	bg = 99
	snap := st.Snapshot(2)
	if snap.Usage == nil || *snap.Usage.Background != 2 || *snap.Usage.Monitors != 1 {
		t.Fatalf("counts: %+v", snap.Usage)
	}
	*snap.Usage.Monitors = 99
	if *st.Snapshot(3).Usage.Monitors != 1 {
		t.Fatal("snapshot shared pointer")
	}
	zero := 0
	st.Apply(Event{Kind: KindStatus, Background: &zero, Monitors: &zero}, 4)
	raw, _ := json.Marshal(st.Snapshot(5))
	if !strings.Contains(string(raw), `"background":0`) || !strings.Contains(string(raw), `"monitors":0`) {
		t.Fatalf("zero lost: %s", raw)
	}
	negative := -1
	st.Apply(Event{Kind: KindStatus, Background: &negative}, 6)
	if st.Snapshot(7).Usage != nil {
		t.Fatal("invalid or absent counts not unknown")
	}
	st.Apply(Event{Kind: KindStatus, Background: &zero}, 8)
	st.Apply(Event{Kind: KindReset}, 9)
	if st.Snapshot(10).Usage != nil {
		t.Fatal("reset retained counts")
	}
}

func TestHostSplitCountsPreserveAggregate(t *testing.T) {
	st := NewState("key")
	bg, mon := 2, 1
	st.SetHost(&Host{BackgroundTasks: 5, Background: &bg, Monitors: &mon})
	bg = 99
	snap := st.Snapshot(1)
	if snap.Host.BackgroundTasks != 5 || *snap.Host.Background != 2 || *snap.Host.Monitors != 1 {
		t.Fatalf("host: %+v", snap.Host)
	}
	*snap.Host.Monitors = 99
	if *st.Snapshot(2).Host.Monitors != 1 {
		t.Fatal("snapshot shared host pointer")
	}
}
