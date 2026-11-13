package wsagent

import (
	"encoding/json"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestChildActiveTimePersistsAndExcludesIdle(t *testing.T) {
	rec := Record{Parent: &ParentRef{Kind: ParentLead}, TurnOpen: true, TurnStartedAt: 1000}
	finishActiveTurn(&rec, 1500)
	rec.TurnOpen = false
	// A duplicate settlement or time held for children cannot add time.
	finishActiveTurn(&rec, 5000)
	if rec.ActiveMs != 500 || rec.TurnStartedAt != 0 {
		t.Fatalf("first turn: %+v", rec)
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var restored Record
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	restored.TurnOpen, restored.TurnStartedAt = true, 10000
	finishActiveTurn(&restored, 10750)
	if restored.ActiveMs != 1250 {
		t.Fatalf("total = %d, want 1250", restored.ActiveMs)
	}
	finishActiveTurn(&restored, 20000)
	if restored.ActiveMs != 1250 {
		t.Fatalf("double settlement: %d", restored.ActiveMs)
	}
}

func TestSpawnAgentEventActiveTime(t *testing.T) {
	st := State{ID: "child", Status: StatusRunning, ActiveMs: 500, TurnStartedAt: 10000}
	ev := SpawnAgentEvent(st, "")
	if ev.ActiveMs != 500 || ev.TurnStartedAt != 10000 {
		t.Fatalf("running: %+v", ev)
	}
	st.Status, st.Held = StatusIdle, 1
	ev = SpawnAgentEvent(st, "")
	if ev.Status != wslive.StatusRunning || ev.ActiveMs != 500 || ev.TurnStartedAt != 0 {
		t.Fatalf("held idle: %+v", ev)
	}
	st.Status, st.Held = StatusClosed, 0
	ev = SpawnAgentEvent(st, "")
	if ev.Status != wslive.StatusDone || ev.ActiveMs != 500 || ev.TurnStartedAt != 0 {
		t.Fatalf("closed: %+v", ev)
	}
}
