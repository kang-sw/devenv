package wslive

import (
	"fmt"
	"testing"
)

func TestSpawnAliasesShareRetentionAndCannotReviveAnEvictedLifecycle(t *testing.T) {
	st := NewState("lead")
	apply := func(ev Event, at int64) {
		t.Helper()
		if _, err := st.Apply(ev, at); err != nil {
			t.Fatal(err)
		}
	}
	apply(Event{Kind: KindAgent, AgentID: "pi:child", AgentKind: AgentKindPi, SessionKey: "child-key", Status: StatusRunning}, 1)
	apply(Event{Kind: KindAgent, AgentID: "native-child", SessionKey: "child-key", Status: StatusRunning}, 2)
	apply(Event{Kind: KindAgent, AgentID: "pi:child", AgentKind: AgentKindPi, Status: StatusDone}, 3)
	for i := range FinishedAgentCap {
		apply(Event{Kind: KindAgent, AgentID: fmt.Sprintf("finished-%d", i), Status: StatusDone}, int64(i+10))
	}
	// Both mutable producer rows are evicted as one finished lifecycle.
	for _, a := range st.Snapshot(30).Agents {
		if a.Status == StatusRunning || a.ID == "native-child" || a.ID == "pi:child" {
			t.Fatalf("eviction revived/retained an old worker: %+v", a)
		}
	}
	for _, ev := range []Event{
		{Kind: KindAgent, AgentID: "native-child", Status: StatusRunning},
		{Kind: KindAgent, AgentID: "late-alias", SessionKey: "child-key", Status: StatusRunning},
		{Kind: KindPermission, AgentID: "native-child", Tool: "bash"},
		{Kind: KindAgent, AgentID: "pi:child", AgentKind: AgentKindPi, Status: StatusDone},
	} {
		apply(ev, 31)
	}
	if got := st.Snapshot(32); len(got.Agents) != FinishedAgentCap {
		t.Fatalf("a stale producer resurrected its evicted lifecycle: %+v", got.Agents)
	}
	// Only a genuine daemon next turn revives the canonical lifecycle.
	apply(Event{Kind: KindAgent, AgentID: "pi:child", AgentKind: AgentKindPi, SessionKey: "child-key", Status: StatusRunning}, 33)
	apply(Event{Kind: KindAgent, AgentID: "native-child", Status: StatusDone}, 34)
	var running int
	for _, a := range st.Snapshot(35).Agents {
		if a.Status == StatusRunning {
			running++
			if a.ID != "pi:child" || a.AgentKind != AgentKindPi {
				t.Fatalf("lost canonical identity: %+v", a)
			}
		}
	}
	if running != 1 {
		t.Fatalf("running after daemon revival = %d", running)
	}
}

func TestCompletedSpawnAliasPairsUseOneFinishedHistorySlot(t *testing.T) {
	st := NewState("lead")
	for i := range FinishedAgentCap + 2 {
		id, key := fmt.Sprintf("child%d", i), fmt.Sprintf("key%d", i)
		for _, ev := range []Event{
			{Kind: KindAgent, AgentID: "pi:" + id, AgentKind: AgentKindPi, SessionKey: key, Status: StatusRunning},
			{Kind: KindAgent, AgentID: id, SessionKey: key, Status: StatusDone},
			{Kind: KindAgent, AgentID: "pi:" + id, AgentKind: AgentKindPi, Status: StatusDone},
		} {
			if _, err := st.Apply(ev, int64(i+1)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := st.Snapshot(20); len(got.Agents) != FinishedAgentCap {
		t.Fatalf("aliases consumed separate finished-history slots: %+v", got.Agents)
	}
}

func TestSpawnRowsMergeNativeDuplicatesInEitherArrivalOrder(t *testing.T) {
	for _, nativeFirst := range []bool{false, true} {
		for _, linked := range []bool{false, true} {
			st := NewState("lead-key")
			native := Event{Kind: KindAgent, AgentID: "child", Status: StatusRunning, Model: "native-model", ContextTokens: f64p(42)}
			if linked {
				native.AgentID, native.SessionKey = "native-worker", "child-key"
			}
			daemon := Event{Kind: KindAgent, AgentID: "pi:child", AgentKind: AgentKindPi, SessionKey: "child-key", Status: StatusDone}
			events := []Event{daemon, native}
			if nativeFirst {
				events = []Event{native, daemon}
			}
			events = append(events, Event{Kind: KindAgent, AgentID: "leaf", ParentID: native.AgentID, Status: StatusRunning})
			for _, ev := range events {
				if _, err := st.Apply(ev, 100); err != nil {
					t.Fatal(err)
				}
			}
			snap := st.Snapshot(101)
			if len(snap.Agents) != 2 {
				t.Fatalf("nativeFirst=%v linked=%v: duplicate rows: %+v", nativeFirst, linked, snap.Agents)
			}
			rows := map[string]Agent{}
			for _, a := range snap.Agents {
				rows[a.ID] = a
			}
			if a := rows["pi:child"]; a.Status != StatusDone || a.ContextTokens != 42 || a.Model != "native-model" || rows["leaf"].ParentID != a.ID {
				t.Fatalf("merged row/nesting: %+v", rows)
			}
			// Snapshot merging must not consume the native event target.
			if _, err := st.Apply(Event{Kind: KindAgent, AgentID: native.AgentID, ContextTokens: f64p(84)}, 102); err != nil {
				t.Fatal(err)
			}
			for _, a := range st.Snapshot(103).Agents {
				if a.ID == "pi:child" && a.ContextTokens != 84 {
					t.Fatalf("native updates stopped: %+v", a)
				}
			}
		}
	}
}
