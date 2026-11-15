package wsdashboard

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestObservedSnapshotOverlaySurvivesTypedLegacyFileAndLiveCopies(t *testing.T) {
	dir := t.TempDir()
	id := "observedsession"
	sessionDir := SessionDir(dir, id)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		t.Fatal(err)
	}
	// This is the shape both old writers reconstruct: their typed snapshot has
	// no new nested evidence fields. Its heartbeat is not an observation time.
	legacy := &wslive.Snapshot{SchemaVersion: 1, ReplyID: id, UpdatedAt: time.Now().UnixMilli(), Adapter: "codex", Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}, Agents: []wslive.Agent{{ID: "child", Status: wslive.StatusDone}, {ID: "unknown", Status: wslive.StatusRunning}}}
	writeSnapshot(t, dir, id, legacy)
	cache := &SnapshotCache{}
	var before bytes.Buffer
	cache.HashLiveStores(&before, dir, id, time.Now())
	records := []wslive.ObservedRecord{{ModelObservation: wslive.ModelObservation{ID: "one", Model: "own-output", Source: "claude.assistant", At: 1000}}, {AgentID: "child", ModelObservation: wslive.ModelObservation{ID: "two", Model: "child-output", Source: "pi.assistant", At: 2000}}}
	records = append(records, wslive.ObservedRecord{AgentID: "retired", ModelObservation: wslive.ModelObservation{ID: "three", Model: "retired-output", Source: "pi.assistant", At: 3000}})
	if err := wslive.AppendObserved(sessionDir, records); err != nil {
		t.Fatal(err)
	}
	var after bytes.Buffer
	cache.HashLiveStores(&after, dir, id, time.Now())
	if bytes.Equal(before.Bytes(), after.Bytes()) {
		t.Fatal("evidence did not invalidate session change fingerprint")
	}
	for _, live := range []bool{false, true} {
		if live {
			cache.SetLive(dir, id, *legacy)
		}
		got := cache.Snapshot(dir, id)
		if got.Lead.Observed == nil || got.Lead.Observed.Current.At != 1000 || got.Lead.Observed.Current.Model != "own-output" {
			t.Fatalf("lost retained own evidence: %+v", got)
		}
		if got.Agents[0].Observed == nil || got.Agents[0].Observed.Current.Model != "child-output" || got.Agents[1].Observed != nil {
			t.Fatalf("mixed child evidence: %+v", got.Agents)
		}
		if len(got.Agents) != 2 || got.ObservedAgents["retired"] == nil || got.ObservedAgents["retired"].Current.Model != "retired-output" {
			t.Fatal("retired history lost or activity revived")
		}
		if got.Adapter != "codex" || got.Lead.Phase != wslive.PhaseIdle {
			t.Fatal("overlay changed legacy adapter/activity")
		}
	}
	// No evidence for a separate Codex session: explicit unknown, independently.
	if err := os.MkdirAll(SessionDir(dir, "unknowncodex"), 0700); err != nil {
		t.Fatal(err)
	}
	writeSnapshot(t, dir, "unknowncodex", &wslive.Snapshot{Adapter: "codex"})
	if snap := cache.Snapshot(dir, "unknowncodex"); snap.Lead.Observed != nil {
		t.Fatal("Codex inherited another session's identity")
	}
}
