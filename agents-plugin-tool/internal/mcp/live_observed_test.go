package mcp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wslive"
)

// The rejected-call test in dashboard.test.ts pins the same observed shape
// (id, model, source, at, effort, effortSource, outputTps) and turn ordering.
// This hand-reduced fixture is not the adapter's full recovered event stream.
func TestObservedClaudeRetryAdapterToReducer(t *testing.T) {
	raw := []byte(`[{"kind":"reset"},{"kind":"turn_start","at":1000},{"kind":"status","observed":{"id":"rate-response","model":"claude-opus-5-5","source":"claude.step","at":2000,"effort":"high","effortSource":"request","outputTps":100}},{"kind":"reply","text":"done","final":true},{"kind":"turn_end","at":2000}]`)
	var events []map[string]any
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	f := newLiveFixture(t, true)
	fastLive(t, time.Hour)
	t.Cleanup(f.server.live.close)
	intake(t, f.server, f.keyA, events...)
	snap := f.server.live.existing(f.keyA).state.Snapshot(900000)
	var at int64
	for _, ev := range events {
		if o, ok := ev["observed"].(map[string]any); ok {
			at = int64(o["at"].(float64))
		}
	}
	o := snap.Lead.LastTurnOutput
	if snap.Lead.Phase != wslive.PhaseIdle || o == nil || o.At != at || o.OutputTps == nil || *o.OutputTps != 100 || o.ID != "rate-response" {
		t.Fatalf("recovered adapter output lost idle turn rate/original time: %+v", snap.Lead)
	}
}

func TestObservedInternalRowsPersistThroughWatcherReconnectAndLegacyRewrite(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, time.Hour)
	agents := newFakePiAgents()
	s := NewServerWithOptions(ServerOptions{Root: f.root, Version: "test", Agents: agents})
	intake(t, s, f.keyA, map[string]any{"kind": "reset"})
	parent := wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: f.replyA}
	child, _ := agents.Spawn(wsagent.SpawnSpec{Root: f.root, Parent: &parent})
	for i, model := range []string{"physical-A", "physical-B"} {
		agents.set(child.ID, func(st *wsagent.State) {
			o := wslive.ModelObservation{ID: model, Model: model, Source: "pi.assistant", At: int64(100 + i)}
			st.Observed = &o
			st.Observations = append(st.Observations, o)
		})
	}
	s.Close()
	dir := f.sessionDir(f.replyA)
	records, err := wslive.ReadObserved(dir)
	if err != nil || len(records) != 2 {
		t.Fatalf("watcher failed to persist child history: %+v / %v", records, err)
	}
	// A pre-evidence typed writer drops every optional snapshot field, and
	// the runner no longer holds the child: only the parent's sidecar remains.
	raw, _ := json.Marshal(wslive.Snapshot{SchemaVersion: 1, ReplyID: f.replyA})
	if err := os.WriteFile(wslive.SnapshotPath(dir), raw, 0600); err != nil {
		t.Fatal(err)
	}
	s2 := NewServerWithOptions(ServerOptions{Root: f.root, Version: "test"})
	defer s2.Close()
	intake(t, s2, f.keyA, map[string]any{"kind": "reset"})
	snap := s2.live.existing(f.keyA).state.Snapshot(1000)
	got := snap.ObservedAgents["pi:"+child.ID]
	if got == nil || len(got.History) != 2 || got.Current.Model != "physical-B" || got.Current.At != 101 || snap.Lead.Observed != nil {
		t.Fatalf("internal-row history lost on reconnect: %+v", snap)
	}
}

func TestObservedIntakePersistsAcrossReconnectAndLegacySnapshotRewrite(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, time.Hour) // no timer/git/heartbeat races: inspect the intake state
	t.Cleanup(f.server.live.close)
	evidence := func(id, model string, at int64) map[string]any {
		return map[string]any{"kind": "status", "context": map[string]any{"tokens": 1234, "window": 200000}, "observed": map[string]any{"id": id, "model": model, "source": "pi.assistant", "at": at, "outputTps": 42}}
	}
	out := intake(t, f.server, f.keyA, map[string]any{"kind": "reset"}, map[string]any{"kind": "turn_start", "at": 50}, evidence("a", "old", 100), evidence("b", "new", 200), map[string]any{"kind": "turn_end", "at": 300})
	if !strings.Contains(out, "ok: 5 of 5 events applied") {
		t.Fatal(out)
	}
	dir := f.sessionDir(f.replyA)
	records, err := wslive.ReadObserved(dir)
	if err != nil || len(records) != 2 {
		t.Fatalf("persisted records: %+v %v", records, err)
	}
	snap := f.server.live.existing(f.keyA).state.Snapshot(400)
	if snap.Lead.LastTurnOutput == nil || snap.Lead.LastTurnOutput.At != 200 || snap.Usage.Context == nil || *snap.Usage.Context.Tokens != 1234 {
		t.Fatalf("intake evidence/usage: %+v", snap)
	}
	// Model/effort legacy lead telemetry remains refused, while unknown optional
	// evidence fields remain tolerated and don't alter BatchView JSON.
	out = intake(t, f.server, f.keyA, map[string]any{"kind": "agent", "model": "request-alias"})
	if !strings.Contains(out, "the lead takes only batches and caption") {
		t.Fatal(out)
	}
	f.server.live.close()
	legacy := wslive.Snapshot{SchemaVersion: 1, ReplyID: f.replyA, Adapter: "pi", Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}}
	raw, _ := json.Marshal(legacy)
	if err := os.WriteFile(wslive.SnapshotPath(dir), raw, 0600); err != nil {
		t.Fatal(err)
	}
	// A reconstructed typed writer discarded optional snapshot evidence; this
	// new intake writer recovers from the separate immutable metadata store.
	replay := evidence("b", "new", 999)
	replay["futureOptional"] = map[string]any{"ignored": true}
	intake(t, f.server, f.keyA, map[string]any{"kind": "reset"}, replay)
	got := f.server.live.existing(f.keyA).state.Snapshot(1000).Lead.Observed
	if got == nil || got.Current.At != 200 || len(got.History) != 2 {
		t.Fatalf("reconnect fabricated a change: %+v", got)
	}
	// Another registered session stays independent.
	intake(t, f.server, f.keyB, map[string]any{"kind": "reset"})
	if f.server.live.existing(f.keyB).state.Snapshot(1000).Lead.Observed != nil {
		t.Fatal("borrowed another session's evidence")
	}
}
