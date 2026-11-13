package wslive

import "testing"

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
			st.Apply(Event{Kind: KindAgent, AgentID: native.AgentID, ContextTokens: f64p(84)}, 102)
			for _, a := range st.Snapshot(103).Agents {
				if a.ID == "pi:child" && a.ContextTokens != 84 {
					t.Fatalf("native updates stopped: %+v", a)
				}
			}
		}
	}
}
