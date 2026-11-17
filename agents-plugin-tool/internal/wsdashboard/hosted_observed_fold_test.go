package wsdashboard

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// evidenceReadIO sums the file I/O counted at the evidence reader's own sites:
// the directory stat, the listing, and any record stat. Record opens are
// ObservedCache.Opens.
func evidenceReadIO() int64 {
	var n int64
	for site, v := range wsstate.FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:wslive/observed_cache.go:") {
			n += v
		}
	}
	return n
}

// hostedFoldWriter builds a session's evidence as a hosted writer leaves it:
// an earlier writer's records, then a writer that seeded its state from the
// directory and stores what each applied observation returns (the C1 subset:
// repeats stay in memory alone). It returns that writer's snapshot.
func hostedFoldWriter(t *testing.T, sessionDir string) wslive.Snapshot {
	t.Helper()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	obs := func(id, model string, at int64) wslive.ModelObservation {
		return wslive.ModelObservation{ID: id, Model: model, Source: "pi.assistant", At: at}
	}
	if err := wslive.AppendObserved(sessionDir, []wslive.ObservedRecord{
		{ModelObservation: obs("a", "m1", 1000)},
		{ModelObservation: obs("b", "m2", 2000)},
		{AgentID: "child", ModelObservation: obs("c1", "m4", 1200)},
		{AgentID: "retired", ModelObservation: obs("r", "m3", 1500)},
	}); err != nil {
		t.Fatal(err)
	}
	st := wslive.NewState("hostedfoldkey")
	records, err := wslive.ReadObserved(sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	st.SeedObserved(records)
	for _, step := range []struct {
		agent string
		o     wslive.ModelObservation
	}{
		{"", obs("b2", "m2", 3000)},
		{"", obs("late", "m1", 2500)}, // a late change: b2 changes back
		{"", obs("d", "m1", 4000)},
		{"", obs("e", "m1", 5000)},
		{"", obs("x", "m1", 4500)}, // a repeat inside the fold: memory only
		{"child", obs("c2", "m5", 1300)},
		{"child", obs("c3", "m5", 1400)},
		{"child", obs("c4", "m5", 1350)}, // a repeat: memory only
	} {
		o := step.o
		ev := wslive.Event{Kind: wslive.KindStatus, Observed: &o}
		if step.agent != "" {
			ev.Kind, ev.AgentID = wslive.KindAgent, step.agent
		}
		res, err := st.Apply(ev, o.At)
		if err != nil {
			t.Fatal(err)
		}
		if err := wslive.AppendObserved(sessionDir, res.Observed); err != nil {
			t.Fatal(err)
		}
	}
	snap := st.Snapshot(6000)
	if snap.Lead.Observed == nil || snap.Lead.Observed.Current.ID != "e" || len(snap.Lead.Observed.History) != 5 {
		t.Fatalf("writer fold = %+v, want e current after 5 changes", snap.Lead.Observed)
	}
	return snap
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestHostedFoldedSnapshotReadsNoEvidence: a hosted copy whose writer seeded
// from the evidence directory is served with no evidence I/O, and its fold is
// the one the directory overlay builds: the same snapshot as the cache path,
// and per producer the same Current and History as the directory alone.
func TestHostedFoldedSnapshotReadsNoEvidence(t *testing.T) {
	dir, id := t.TempDir(), "hostedfold"
	sessionDir := SessionDir(dir, id)
	snap := hostedFoldWriter(t, sessionDir)
	cache := &SnapshotCache{}
	cache.setLive(dir, id, snap, true)

	copied, ok := cache.foldedCopy(wslive.SnapshotPath(sessionDir))
	if !ok {
		t.Fatal("a folded copy does not stand for its directory")
	}
	// The cache path over the same copy, through a separate evidence cache.
	var oracle wslive.ObservedCache
	viaCache := copied.snap
	oracle.Load(sessionDir).Overlay(&viaCache)
	// The directory alone, overlaid on a snapshot with no evidence of its own.
	var dirOnly wslive.Snapshot
	oracle.Load(sessionDir).Overlay(&dirOnly)

	beforeIO, beforeOpens := evidenceReadIO(), cache.observed.Opens()
	got := cache.Snapshot(dir, id)
	if io, opens := evidenceReadIO()-beforeIO, cache.observed.Opens()-beforeOpens; io != 0 || opens != 0 {
		t.Fatalf("a folded hosted read made %d evidence stats/listings and %d opens, want none", io, opens)
	}
	if jsonOf(t, got) != jsonOf(t, &viaCache) {
		t.Fatalf("folded read differs from the cache path:\n%s\n%s", jsonOf(t, got), jsonOf(t, &viaCache))
	}
	if jsonOf(t, got.Lead.Observed) != jsonOf(t, dirOnly.Lead.Observed) {
		t.Fatalf("lead fold differs from the directory's:\n%s\n%s", jsonOf(t, got.Lead.Observed), jsonOf(t, dirOnly.Lead.Observed))
	}
	if jsonOf(t, got.ObservedAgents) != jsonOf(t, dirOnly.ObservedAgents) {
		t.Fatalf("agent folds differ from the directory's:\n%s\n%s", jsonOf(t, got.ObservedAgents), jsonOf(t, dirOnly.ObservedAgents))
	}
	if len(got.ObservedAgents) != 2 || got.ObservedAgents["retired"] == nil || got.ObservedAgents["child"].Current.ID != "c3" {
		t.Fatalf("agent folds = %s", jsonOf(t, got.ObservedAgents))
	}
}

// TestHostedSnapshotFallsBackToEvidence: every copy that may not stand for
// the directory is overlaid with it, so a record another writer added shows:
// an undeclared copy, a shared one, and the file once the copy is cleared.
// Clearing the copy clears its share too.
func TestHostedSnapshotFallsBackToEvidence(t *testing.T) {
	dir, id := t.TempDir(), "hostedfallback"
	sessionDir := SessionDir(dir, id)
	snap := hostedFoldWriter(t, sessionDir)
	disk := snap.ForDisk()
	writeSnapshot(t, dir, id, &disk)
	foreign := wslive.ModelObservation{ID: "foreign", Model: "m9", Source: "pi.assistant", At: 7000}
	if err := wslive.AppendObserved(sessionDir, []wslive.ObservedRecord{{ModelObservation: foreign}}); err != nil {
		t.Fatal(err)
	}
	shows := func(cache *SnapshotCache) bool {
		t.Helper()
		got := cache.Snapshot(dir, id)
		return got != nil && got.Lead.Observed != nil && got.Lead.Observed.Current.ID == "foreign"
	}

	undeclared := &SnapshotCache{}
	undeclared.SetLive(dir, id, snap)
	if !shows(undeclared) {
		t.Error("an undeclared copy was not overlaid")
	}
	shared := &SnapshotCache{}
	shared.setLive(dir, id, snap, true)
	shared.shareObserved(dir, id)
	if !shows(shared) {
		t.Error("a shared copy was not overlaid")
	}
	shared.ClearLive(dir, id)
	if !shows(shared) {
		t.Error("the file after the copy cleared was not overlaid")
	}
	shared.setLive(dir, id, snap, true)
	if _, ok := shared.foldedCopy(wslive.SnapshotPath(sessionDir)); !ok {
		t.Error("ClearLive kept the share of the next copy")
	}
}

// TestHostedSlotObservedFoldWiring pins the daemon side: a slot's copies
// stand for the directory only once its writer declares the fold; an
// in-process notification keeps them so, while an HTTP nudge (another
// process) and a takeover share them until the owner closes.
func TestHostedSlotObservedFoldWiring(t *testing.T) {
	f := startPageFixture(t)
	sessionDir := SessionDir(f.dir, "livesession")
	path := wslive.SnapshotPath(sessionDir)
	snap := hostedFoldWriter(t, sessionDir)
	folded := func() bool {
		_, ok := f.d.snapshots.foldedCopy(path)
		return ok
	}

	slot, _ := f.d.OpenHostedSlot(sessionDir)
	slot.Publish(snap)
	if folded() {
		t.Fatal("an undeclared slot's copy stands for the directory")
	}
	slot.(ObservedFoldSlot).FoldsObserved()
	slot.Publish(snap)
	if !folded() {
		t.Fatal("a declared slot's copy does not stand for the directory")
	}
	f.d.NotifySessionChange(f.dir, NudgeBody{ReplyID: "livesession", Project: f.project})
	if !folded() {
		t.Fatal("an in-process notification shared the copy")
	}
	if err := SendNudge(context.Background(), testClient(), f.d.Endpoint(), NudgeBody{ReplyID: "livesession", Project: f.project}); err != nil {
		t.Fatal(err)
	}
	if folded() {
		t.Fatal("another process's nudge left the copy standing for the directory")
	}

	slot.Close()
	slot, _ = f.d.OpenHostedSlot(sessionDir)
	slot.(ObservedFoldSlot).FoldsObserved()
	slot.Publish(snap)
	if !folded() {
		t.Fatal("the share outlived the slot that owned the copy")
	}
	taker, _ := f.d.OpenHostedSlot(sessionDir)
	taker.(ObservedFoldSlot).FoldsObserved()
	taker.Publish(snap)
	if folded() {
		t.Fatal("a takeover left the copy standing for the directory")
	}
	taker.Close()
	slot.Close()
}
