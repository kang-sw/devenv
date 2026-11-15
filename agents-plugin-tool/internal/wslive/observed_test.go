package wslive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func observedEvent(id, model string, at int64) Event {
	return Event{Kind: KindStatus, Observed: &ModelObservation{ID: id, Model: model, Source: "pi.assistant", At: at}}
}

func TestObservedResponseOrderReplayAndReset(t *testing.T) {
	st := NewState("test-session")
	// Delivery order and receipt time are not observation order. A replay with
	// a later receipt must not manufacture another change or move its timestamp.
	for _, ev := range []Event{observedEvent("b", "provider/new", 200), observedEvent("a", "provider/old", 100), observedEvent("b", "provider/new", 999)} {
		if _, err := st.Apply(ev, 9000); err != nil {
			t.Fatal(err)
		}
	}
	got := st.Snapshot(10000).Lead.Observed
	if got == nil || got.Current.At != 200 || got.Current.Model != "provider/new" || len(got.History) != 2 || got.History[1].Previous != "provider/old" || got.History[1].At != 200 {
		t.Fatalf("observed: %+v", got)
	}
	if _, err := st.Apply(Event{Kind: KindReset}, 11000); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, st.Snapshot(12000).Lead.Observed) {
		t.Fatal("adapter reset destroyed retained evidence")
	}
	// State/request telemetry never supplies output identity, and neither a
	// parent nor a sibling can supply a missing child's value.
	_, _ = st.Apply(Event{Kind: KindAgent, AgentID: "child", Model: "configured-alias", Effort: "high"}, 300)
	ev := observedEvent("c", "provider/child", 300)
	ev.Kind, ev.AgentID = KindAgent, "independent"
	_, _ = st.Apply(ev, 500)
	snap := st.Snapshot(600)
	for _, a := range snap.Agents {
		if a.ID == "child" && a.Observed != nil {
			t.Fatal("inherited/request identity")
		}
		if a.ID == "independent" && (a.Observed == nil || a.Observed.Current.Model != "provider/child") {
			t.Fatal("lost child evidence")
		}
	}
	if _, err := st.Apply(Event{Kind: KindAgent, Model: "forbidden"}, 600); err == nil {
		t.Fatal("legacy lead rejection changed")
	}
}

func TestObservedValidationRateAndRedaction(t *testing.T) {
	st := NewState("secret-session-key")
	ev := observedEvent("a", "provider/model", 100)
	rate := 42.0
	ev.Observed.OutputTps = &rate
	ev.Observed.Effort = "high"
	ev.Observed.EffortSource = "message"
	res, err := st.Apply(ev, 200)
	if err != nil || len(res.Observed) != 1 {
		t.Fatalf("apply: %+v %v", res, err)
	}
	rate = 999 // caller ownership ends at Apply
	if got := st.Snapshot(300).Lead.Observed.Current; got.OutputTps == nil || *got.OutputTps != 42 || got.Effort != "high" {
		t.Fatalf("evidence: %+v", got)
	}
	_, _ = st.Apply(observedEvent("b", "provider/model", 400), 500)
	got := st.Snapshot(600).Lead.Observed
	if got.Current.OutputTps != nil || got.Current.Effort != "" || len(got.History) != 1 {
		t.Fatalf("absent evidence inherited: %+v", got)
	}
	for _, mutate := range []func(*ModelObservation){func(o *ModelObservation) { o.At = 0 }, func(o *ModelObservation) { o.ID = "" }, func(o *ModelObservation) { o.Source = "state" }, func(o *ModelObservation) { o.Model = "" }} {
		ev := observedEvent("bad", "model", 700)
		mutate(ev.Observed)
		res, err := st.Apply(ev, 800)
		if err != nil || len(res.Observed) != 0 {
			t.Fatalf("malformed optional evidence must be ignored: %+v %v", res, err)
		}
	}
	ev = observedEvent("secret-session-key", "secret-session-key", 900)
	res, _ = st.Apply(ev, 1000)
	raw, _ := json.Marshal(res.Observed)
	if string(raw) == "" || strings.Contains(string(raw), "secret-session-key") {
		t.Fatalf("leaked key: %s", raw)
	}
}

func TestObservedLastTurnRateAndSnapshotOwnership(t *testing.T) {
	st := NewState("test-session")
	_, _ = st.Apply(Event{Kind: KindTurnStart, At: 100}, 100)
	ev := observedEvent("answer", "physical", 200)
	rate := 42.0
	ev.Observed.OutputTps = &rate
	_, _ = st.Apply(ev, 250)
	_, _ = st.Apply(Event{Kind: KindTurnEnd, At: 300}, 300)
	snap := st.Snapshot(400)
	if snap.Lead.LastTurnOutput == nil || *snap.Lead.LastTurnOutput.OutputTps != 42 || snap.Lead.LastTurnOutput.At != 200 {
		t.Fatalf("last-turn evidence: %+v", snap.Lead)
	}
	snap.Lead.Observed.History[0].Model = "tampered"
	*snap.Lead.LastTurnOutput.OutputTps = 999
	if got := st.Snapshot(500); got.Lead.Observed.History[0].Model != "physical" || *got.Lead.LastTurnOutput.OutputTps != 42 {
		t.Fatal("snapshot aliases mutable state")
	}
	// A later turn with no eligible output cannot keep the old turn's rate.
	_, _ = st.Apply(Event{Kind: KindTurnStart, At: 600}, 600)
	_, _ = st.Apply(Event{Kind: KindTurnEnd, At: 700}, 700)
	if st.Snapshot(800).Lead.LastTurnOutput != nil {
		t.Fatal("empty turn inherited old rate")
	}
	for _, alias := range []string{"opus", "sonnet", "haiku", "fable", "default"} {
		ev := observedEvent("alias-"+alias, alias, 900)
		ev.Observed.Source = "claude.step"
		res, err := st.Apply(ev, 900)
		if err != nil || len(res.Observed) != 0 {
			t.Fatalf("configured alias promoted: %+v %v", res, err)
		}
	}
}

func TestObservedDurableHistorySurvivesLegacySnapshotWriters(t *testing.T) {
	dir := t.TempDir()
	st := NewState("test-session")
	for _, ev := range []Event{observedEvent("a", "old", 100), observedEvent("b", "new", 200)} {
		res, _ := st.Apply(ev, 500)
		if err := AppendObserved(dir, res.Observed); err != nil {
			t.Fatal(err)
		}
	}
	// Both pre-feature writers reconstruct typed State.Snapshot and marshal it:
	// mcp.liveSession.fire/finish and wsagent.liveWriter.writeSnapshot at
	// 171bf6b26. Their old wire schema cannot retain new nested members. Exercise
	// those exact destructive operations, not a lenient unknown-field Record.
	for _, writer := range []string{"mcp.liveSession.fire", "mcp.liveSession.finish", "wsagent.liveWriter.writeSnapshot"} {
		t.Run(writer, func(t *testing.T) {
			legacy := struct {
				SchemaVersion int    `json:"schemaVersion"`
				ReplyID       string `json:"replyId"`
				Lead          struct {
					Phase string `json:"phase"`
				} `json:"lead"`
			}{SchemaVersion: 1, ReplyID: st.ReplyID()}
			legacy.Lead.Phase = PhaseIdle
			raw, _ := json.Marshal(legacy)
			if err := os.WriteFile(SnapshotPath(dir), raw, 0600); err != nil {
				t.Fatal(err)
			}
			records, err := ReadObserved(dir)
			if err != nil {
				t.Fatal(err)
			}
			snap, err := ReadSnapshot(SnapshotPath(dir))
			if err != nil {
				t.Fatal(err)
			}
			OverlayObserved(&snap, records)
			if snap.Lead.Observed == nil || snap.Lead.Observed.Current.At != 200 || len(snap.Lead.Observed.History) != 2 {
				t.Fatalf("old writer lost history: %+v", snap.Lead.Observed)
			}
			// Repeat intake after restart: first original metadata wins, and the
			// durable store does not acquire an extra event/change.
			ev := observedEvent("b", "new", 999)
			restarted := NewState("test-session")
			restarted.SeedObserved(records)
			res, _ := restarted.Apply(ev, 1000)
			if len(res.Observed) != 0 {
				t.Fatal("replay created new durable evidence")
			}
		})
	}
	// Files contain only scrubbed bounded evidence, not a parallel transcript.
	entries, _ := os.ReadDir(filepath.Join(dir, ObservedDirName))
	if len(entries) != 2 {
		t.Fatalf("dedup: %d entries", len(entries))
	}
}
