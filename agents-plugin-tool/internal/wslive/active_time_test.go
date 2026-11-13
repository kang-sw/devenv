package wslive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentActiveTimeAndSeed(t *testing.T) {
	st := NewState(ownerKey)
	apply := func(status string, at int64) {
		mustApply(t, st, Event{Kind: KindAgent, AgentID: "agent", Status: status}, at)
	}
	apply(StatusRunning, 1000)
	apply(StatusRunning, 1500) // repeated updates do not reset the segment
	apply(StatusDone, 3000)
	apply(StatusDone, 4000) // duplicate ends do not add time
	apply(StatusRunning, 9000)
	a := st.Snapshot(9500).Agents[0]
	if a.ActiveMs != 2000 || a.TurnStartedAt != 9000 {
		t.Fatalf("running: %+v", a)
	}
	apply(StatusFailed, 10000)
	a = st.Snapshot(11000).Agents[0]
	if a.ActiveMs != 3000 || a.TurnStartedAt != 0 {
		t.Fatalf("finished: %+v", a)
	}
	seeded := NewState(ownerKey)
	seeded.SeedFinished(st.Snapshot(11000))
	a = seeded.Snapshot(12000).Agents[0]
	if a.ActiveMs != 3000 || a.TurnStartedAt != 0 {
		t.Fatalf("seeded: %+v", a)
	}
	mustApply(t, seeded, Event{Kind: KindAgent, AgentID: "agent", Status: StatusRunning}, 20000)
	mustApply(t, seeded, Event{Kind: KindAgent, AgentID: "agent", Status: StatusDone}, 21000)
	if a = seeded.Snapshot(22000).Agents[0]; a.ActiveMs != 4000 {
		t.Fatalf("resumed seed: %+v", a)
	}
}

func TestPiAgentAuthoritativeActiveTime(t *testing.T) {
	st := NewState(ownerKey)
	ev := Event{Kind: KindAgent, AgentID: "pi:child", AgentKind: AgentKindPi, Status: StatusRunning, TurnStartedAt: 1000}
	mustApply(t, st, ev, 1000)
	ev.ActiveMs, ev.TurnStartedAt = 2000, 0 // held, but idle
	mustApply(t, st, ev, 3000)
	if a := st.Snapshot(8000).Agents[0]; a.ActiveMs != 2000 || a.TurnStartedAt != 0 {
		t.Fatalf("held idle: %+v", a)
	}
	ev.TurnStartedAt = 9000
	mustApply(t, st, ev, 9000)
	ev.Status, ev.ActiveMs, ev.TurnStartedAt = StatusDone, 3000, 0
	mustApply(t, st, ev, 10000)
	if a := st.Snapshot(11000).Agents[0]; a.ActiveMs != 3000 || a.TurnStartedAt != 0 {
		t.Fatalf("finished: %+v", a)
	}
	b, err := json.Marshal(st.Snapshot(11000).Agents[0])
	if err != nil || !strings.Contains(string(b), `"activeMs":3000`) || strings.Contains(string(b), `"turnStartedAt"`) {
		t.Fatalf("JSON: %s, %v", b, err)
	}
}
