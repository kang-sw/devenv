package wsdashboard

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestSingleNudgeInsideSnapshotTTLReachesIndexAndStream(t *testing.T) {
	f := startPageFixture(t) // default 15 s stream refresh; no repair tick
	now := time.Now()
	var clock atomic.Int64
	clock.Store(now.UnixNano())
	f.d.snapshots.files.mu.Lock()
	f.d.snapshots.files.now = func() time.Time { return time.Unix(0, clock.Load()) }
	f.d.snapshots.files.mu.Unlock()
	reg := register(t, f.dir, "nudgedagent", f.project, now)
	reg.Agent = true
	if err := WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	write := func(text string) {
		writeSnapshot(t, f.dir, reg.ReplyID, &wslive.Snapshot{
			SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
			Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: text}},
		})
	}
	write("before nudge")
	f.d.Index().Refresh() // warm the real index and shared snapshot cache
	events := openStream(t, f.base+"/s/"+reg.ReplyID+"/events?frame=1")
	waitRegionHolding(t, events, regionConvLive, "before nudge")
	before, _ := f.d.Index().Session(reg.ReplyID)
	write("after exactly one nudge")
	// The frozen validation clock cannot expire: this must work because of
	// the normal signal, not a second nudge, TTL advancement or Refresh.
	body := fmt.Sprintf(`{"reply_id":%q,"project":%q}`, reg.ReplyID, reg.Project)
	if code, _, raw := postJSON(t, f.base+"/nudge", f.base, body); code != http.StatusNoContent {
		t.Fatalf("nudge = %d: %s", code, raw)
	}
	f.d.Index().Settle()
	after, _ := f.d.Index().Session(reg.ReplyID)
	if after.Rev != before.Rev+1 {
		t.Fatalf("single nudge did not converge index revision: %d -> %d", before.Rev, after.Rev)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		ev, ok := nextRegion(t, events, time.Until(deadline))
		if ok && ev.Name == regionConvLive && strings.Contains(ev.HTML, "after exactly one nudge") {
			return
		}
	}
	t.Fatal("single nudge did not reach stream within one second")
}

func TestSingleEvidenceNudgeRefreshesObservedOverlayInsideTTL(t *testing.T) {
	f := startPageFixture(t)
	now := time.Now()
	reg := register(t, f.dir, "evidenceagent", f.project, now)
	reg.Agent = true
	if err := WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	writeSnapshot(t, f.dir, reg.ReplyID, &wslive.Snapshot{
		SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
	})
	f.d.Index().Refresh()
	if f.d.snapshots.Snapshot(f.dir, reg.ReplyID) == nil {
		t.Fatal("missing warm snapshot")
	}
	before, _ := f.d.Index().Session(reg.ReplyID)
	if err := wslive.AppendObserved(SessionDir(f.dir, reg.ReplyID), []wslive.ObservedRecord{
		{ModelObservation: wslive.ModelObservation{ID: "new", Model: "nudged-model", Source: "pi.assistant", At: now.UnixMilli()}},
	}); err != nil {
		t.Fatal(err)
	}
	if !f.d.Index().Nudge(reg.ReplyID, reg.Project) {
		t.Fatal("nudge refused")
	}
	f.d.Index().Settle()
	after, _ := f.d.Index().Session(reg.ReplyID)
	snap := f.d.snapshots.Snapshot(f.dir, reg.ReplyID)
	if after.Rev != before.Rev+1 || snap == nil || snap.Lead.Observed == nil || snap.Lead.Observed.Current.Model != "nudged-model" {
		t.Fatalf("evidence-only nudge did not refresh revision and overlay: rev %d -> %d, snapshot %+v", before.Rev, after.Rev, snap)
	}
}
