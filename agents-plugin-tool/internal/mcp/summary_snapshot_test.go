package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Keep production fire/finish synchronous while retaining their real file writer.
func summarySnapshotSeams(t *testing.T) (*time.Time, *[][]byte) {
	t.Helper()
	fastLive(t, time.Hour)
	now := time.Unix(10000, 0)
	var writes [][]byte
	setLiveSeams(func(s *liveSeams) {
		s.now = func() time.Time { return now }
		s.readRepo = func(string) wslive.Repo { return wslive.Repo{Folder: "fixture"} }
		inner := s.writeSnapshot
		s.writeSnapshot = func(path string, data []byte) error {
			writes = append(writes, bytes.Clone(data))
			return inner(path, data)
		}
	})
	return &now, &writes
}

func assertSummarySnapshotDisk(t *testing.T, dir string, writes [][]byte) {
	t.Helper()
	if len(writes) == 0 {
		t.Fatal("no production snapshot write")
	}
	for _, data := range writes {
		if bytes.Contains(data, []byte("reasoningSummary")) || bytes.Contains(data, []byte("exact prefix")) {
			t.Fatalf("mutable snapshot retained live summary: %s", data)
		}
	}
	data, err := os.ReadFile(wslive.SnapshotPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, writes[len(writes)-1]) {
		t.Fatal("file differs from the last production write")
	}
	snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(dir))
	if err != nil || snap.Lead.ReasoningSummary != nil {
		t.Fatalf("disk summary = %+v, error = %v", snap.Lead.ReasoningSummary, err)
	}
}

func applySnapshotSummary(t *testing.T, state *wslive.State) {
	t.Helper()
	for _, ev := range []wslive.Event{{Kind: wslive.KindTurnStart, At: 1000}, {Kind: wslive.KindAgent, ReasoningSummary: intakeSummaryFixture()}} {
		if _, err := state.Apply(ev, 1000); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSummarySnapshotDirectFireKeepsSummaryOnlyInRAM(t *testing.T) {
	now, writes := summarySnapshotSeams(t)
	h := &liveHub{}
	ls, _ := h.session("direct-summary", t.TempDir(), "", func() {}, func() bool { return true }, func(string, map[string]any) {})
	t.Cleanup(h.close)
	applySnapshotSummary(t, ls.state)
	before := ls.state.Snapshot(now.UnixMilli())
	ls.fire()
	assertSummarySnapshotDisk(t, ls.dir, *writes)
	if after := ls.state.Snapshot(now.UnixMilli()); !reflect.DeepEqual(before, after) || after.Lead.ReasoningSummary == nil {
		t.Fatal("direct disk projection mutated full RAM state")
	}
	ls.stop()
	reopened, _ := h.session("direct-summary", ls.dir, "", func() {}, func() bool { return true }, func(string, map[string]any) {})
	if reopened.state.Snapshot(now.UnixMilli()).Lead.ReasoningSummary != nil {
		t.Fatal("new session recovered live summary from disk")
	}
	applySnapshotSummary(t, reopened.state)
	if reopened.state.Snapshot(now.UnixMilli()).Lead.ReasoningSummary == nil {
		t.Fatal("new admissible summary did not repopulate RAM")
	}
}

func TestSummarySnapshotHostedFireHeartbeatFinishAndClose(t *testing.T) {
	for _, end := range []string{"finish", "close"} {
		t.Run(end, func(t *testing.T) {
			now, writes := summarySnapshotSeams(t)
			f, slot := newSummaryIntakeFixture(t)
			for _, ev := range []map[string]any{{"kind": "turn_start", "at": 1000}, {"kind": "agent", "reasoningSummary": intakeSummaryFixture()}} {
				if r := summaryIntake(t, f.server, f.keyA, ev); responseIsError(r) {
					t.Fatal(summaryIntakeText(t, r))
				}
			}
			ls := f.server.live.existing(f.keyA)
			before := ls.state.Snapshot(now.UnixMilli())
			ls.fire() // First dirty hosted flush.
			if len(*writes) != 1 {
				t.Fatalf("first flush writes = %d", len(*writes))
			}
			*now = now.Add(time.Hour)
			ls.fire() // Unchanged heartbeat publishes but does not write.
			if len(*writes) != 1 {
				t.Fatal("idle heartbeat wrote a snapshot")
			}
			ls.mu.Lock()
			ls.dirty = true
			ls.mu.Unlock()
			ls.fire() // Dirty heartbeat flush.
			if len(*writes) != 2 {
				t.Fatal("dirty heartbeat did not flush")
			}
			for _, snap := range slot.published() {
				expected := before
				expected.UpdatedAt = snap.UpdatedAt
				expected.Repo = snap.Repo
				if snap.Lead.ReasoningSummary == nil || !reflect.DeepEqual(snap, expected) {
					t.Fatal("hosted publication lost or changed full RAM projection")
				}
				raw, err := json.Marshal(snap)
				if err != nil || !bytes.Contains(raw, []byte("reasoningSummary")) {
					t.Fatal("normal publication JSON lost summary", err)
				}
			}
			if end == "finish" {
				ls.finish()
			} else {
				f.server.Close()
			}
			if len(*writes) != 3 {
				t.Fatalf("final flush writes = %d", len(*writes))
			}
			assertSummarySnapshotDisk(t, ls.dir, *writes)
			if !reflect.DeepEqual(before.Lead, ls.state.Snapshot(before.UpdatedAt).Lead) {
				t.Fatal("finish/close changed RAM summary")
			}
			if slot.closed != 1 {
				t.Fatalf("slot closed %d times", slot.closed)
			}
			h := &liveHub{}
			reopened, _ := h.session(f.keyA, ls.dir, "", func() {}, func() bool { return true }, func(string, map[string]any) {})
			t.Cleanup(h.close)
			if reopened.state.Snapshot(now.UnixMilli()).Lead.ReasoningSummary != nil {
				t.Fatal("finish/close restored live summary on reopen")
			}
			applySnapshotSummary(t, reopened.state)
			if reopened.state.Snapshot(now.UnixMilli()).Lead.ReasoningSummary == nil {
				t.Fatal("next summary did not repopulate reopened RAM")
			}
		})
	}
}

func TestSummarySnapshotNaturalHistorySurvivesReopenWithoutLiveRecovery(t *testing.T) {
	now, writes := summarySnapshotSeams(t)
	f, _ := newSummaryIntakeFixture(t)
	final := intakeSummaryFixture()
	final.Revision = 2
	final.ActiveRequest = nil
	final.Requests[0].Status = wslive.SummaryAborted
	final.Requests[0].Parts[0].Status = wslive.SummaryAborted
	for _, ev := range []map[string]any{{"kind": "turn_start", "at": 1000}, {"kind": "agent", "reasoningSummary": intakeSummaryFixture()}, {"kind": "turn_end", "at": 2000, "reason": "aborted", "reasoningSummary": final}} {
		if r := summaryIntake(t, f.server, f.keyA, ev); responseIsError(r) {
			t.Fatal(summaryIntakeText(t, r))
		}
	}
	dir := f.sessionDir(f.replyA)
	history := readLiveRing(t, dir)
	if len(history) != 1 || history[0].SummaryHistory == nil || history[0].SummaryHistory.Requests[0].Parts[0].Text != "exact prefix" {
		t.Fatalf("natural summary history missing: %+v", history)
	}
	ls := f.server.live.existing(f.keyA)
	ls.fire() // Natural turn end's flushNow path.
	ls.finish()
	assertSummarySnapshotDisk(t, dir, *writes)
	reopened, _ := f.server.live.session(f.keyA, dir, f.root, func() {}, func() bool { return true }, func(string, map[string]any) {})
	if reopened.state.Snapshot(now.UnixMilli()).Lead.ReasoningSummary != nil {
		t.Fatal("immutable history became live recovery")
	}
	applySnapshotSummary(t, reopened.state)
	if reopened.state.Snapshot(now.UnixMilli()).Lead.ReasoningSummary == nil {
		t.Fatal("next admissible summary missing")
	}
	if !reflect.DeepEqual(history, readLiveRing(t, dir)) {
		t.Fatal("snapshot flush/reopen changed immutable natural-turn history")
	}
}
