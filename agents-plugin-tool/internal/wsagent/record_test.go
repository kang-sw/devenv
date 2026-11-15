package wsagent

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// TestHostedObservedLegacySnapshotRewrite runs a typed old-reader rewrite,
// rather than merely deleting an observed JSON field in a modern snapshot.
func TestHostedObservedLegacySnapshotRewrite(t *testing.T) {
	dashboard := t.TempDir()
	dir := wsdashboard.SessionDir(dashboard, "retainedchild")
	now := func() time.Time { return time.UnixMilli(900000) }
	writer := newLiveWriter(dir, "retained-key", "pi", now, io.Discard)
	first := wslive.ModelObservation{ID: "first", Model: "output-one", Source: "pi.assistant", At: 1000, Effort: "low", EffortSource: "message"}
	last := wslive.ModelObservation{ID: "last", Model: "output-two", Source: "pi.assistant", At: 2000}
	for _, o := range []wslive.ModelObservation{first, last} {
		writer.applyQuiet(wslive.Event{Kind: wslive.KindStatus, Observed: &o})
	}
	child := wslive.ModelObservation{ID: "child", Model: "independent-child", Source: "claude.assistant", At: 1500}
	writer.apply(wslive.Event{Kind: wslive.KindAgent, AgentID: "native-child", Status: wslive.StatusDone, Observed: &child})
	writer.writeSnapshot()

	// This is the pre-evidence typed shape for the members needed by this
	// fixture. Old writers reconstruct state, then marshal known fields only.
	var legacy struct {
		SchemaVersion int    `json:"schemaVersion"`
		ReplyID       string `json:"replyId"`
		UpdatedAt     int64  `json:"updatedAt"`
		Lead          struct {
			wslive.Activity
		} `json:"lead"`
		Agents []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"agents"`
	}
	raw, err := os.ReadFile(wslive.SnapshotPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	rewritten, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rewritten, []byte(`"observed"`)) {
		t.Fatalf("legacy typed writer unexpectedly knows evidence: %s", rewritten)
	}
	if err := os.WriteFile(wslive.SnapshotPath(dir), rewritten, 0o600); err != nil {
		t.Fatal(err)
	}
	old, err := wslive.ReadSnapshot(wslive.SnapshotPath(dir))
	if err != nil || old.Lead.Observed != nil {
		t.Fatalf("rewrite did not discard nested evidence: %+v, %v", old.Lead.Observed, err)
	}
	restored := newLiveWriter(dir, "retained-key", "pi", now, io.Discard)
	// Replayed older blocks do not move the first original observation time.
	first.At = 800000
	restored.applyQuiet(wslive.Event{Kind: wslive.KindStatus, Observed: &first})
	snap := restored.st.Snapshot(900000)
	if own := snap.Lead.Observed; own == nil || own.Current.ID != "last" || own.Current.At != 2000 || own.Current.Effort != "" || len(own.History) != 2 || own.History[0].At != 1000 {
		t.Fatalf("old-writer/replay lost original evidence: %+v", own)
	}
	if len(snap.Agents) != 1 || snap.Agents[0].Observed == nil || snap.Agents[0].Observed.Current.Model != child.Model {
		t.Fatalf("finished child evidence lost: %+v", snap.Agents)
	}
	// A closed daemon conversation also reads the retained own evidence,
	// even before a modern writer has replaced the legacy snapshot.
	r := NewRunner(Options{Dir: dashboard, Now: now})
	st := r.closedState(Record{ReplyID: "retainedchild", ClosedAt: "2026-11-15T00:00:00Z", Model: "requested", Parent: &ParentRef{Kind: ParentLead, ReplyID: "parent"}})
	if st.Observed == nil || st.Observed.ID != "last" || st.Observed.Model != "output-two" || st.Observed.At != 2000 {
		t.Fatalf("closed state lost retained own evidence: %+v", st)
	}
	row := SpawnAgentEvent(st, "child-key")
	if row.Observed == nil || row.Observed.Model != "output-two" || row.Model != "requested" || row.OutputTps != nil {
		t.Fatalf("closed spawn row lost response provenance: %+v", row)
	}
}

func TestRecordUnknownMembersRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra string
	}{
		{name: "legacy"},
		{name: "newer", extra: `,
			"future_recovery": {"sequence": 9007199254740993, "items": [true, "saved", null]},
			"future_null": null,
			"future_string": "keep me",
			"unknown": [1, 2, 3]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "recordroundtrip")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			fixture := `{
				"schema_version": 1,
				"reply_id": "recordroundtrip",
				"profile": "chat",
				"backend": "pi",
				"root": "workspace",
				"ws_session_key": "private-key",
				"created_at": "2026-11-13T00:00:00Z",
				"holder_id": "old-holder",
				"MODEL": "old-model"` + tc.extra + `}`
			if err := os.WriteFile(recordPath(dir), []byte(fixture), 0o600); err != nil {
				t.Fatal(err)
			}
			var original map[string]json.RawMessage
			if err := json.Unmarshal([]byte(fixture), &original); err != nil {
				t.Fatal(err)
			}
			for pass := 0; pass < 2; pass++ {
				rec, err := readRecord(dir)
				if err != nil {
					t.Fatal(err)
				}
				if pass == 0 && (rec.HolderID != "old-holder" || rec.Model != "old-model") {
					t.Fatalf("known fields were not decoded: holder %q, model %q", rec.HolderID, rec.Model)
				}
				rec.Label = "updated"
				rec.HolderID, rec.Model = "", ""
				if err := writeRecord(dir, rec); err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(recordPath(dir))
				if err != nil {
					t.Fatal(err)
				}
				var saved map[string]json.RawMessage
				if err := json.Unmarshal(raw, &saved); err != nil {
					t.Fatal(err)
				}
				if string(saved["schema_version"]) != "1" || string(saved["label"]) != `"updated"` {
					t.Fatalf("schema or known-field edit changed: %s", raw)
				}
				for _, key := range []string{"holder_id", "model", "MODEL"} {
					if _, exists := saved[key]; exists {
						t.Fatalf("cleared optional field %q resurrected: %s", key, raw)
					}
				}
				for _, key := range []string{"future_recovery", "future_null", "future_string", "unknown"} {
					want, exists := original[key]
					got, kept := saved[key]
					if exists != kept {
						t.Fatalf("unknown member %q presence = %v, want %v", key, kept, exists)
					}
					if !exists {
						continue
					}
					var compactWant, compactGot bytes.Buffer
					if err := json.Compact(&compactWant, want); err != nil {
						t.Fatal(err)
					}
					if err := json.Compact(&compactGot, got); err != nil {
						t.Fatal(err)
					}
					if compactWant.String() != compactGot.String() {
						t.Fatalf("unknown member %q = %s, want %s", key, got, want)
					}
				}
			}
		})
	}
}
