package wsdashboard

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func overlayConsumerFixture(t *testing.T) (pageReader, *SnapshotCache) {
	t.Helper()
	dir := t.TempDir()
	cache := &SnapshotCache{}
	index := NewIndex(IndexOptions{Now: func() time.Time { return testNow }})
	for _, id := range []string{"peer", "delegate", "unknown"} {
		s := liveSession(id, "/p", testNow)
		index.sessions[id] = &indexEntry{session: s}
		if err := os.MkdirAll(SessionDir(dir, id), 0700); err != nil {
			t.Fatal(err)
		}
		cache.SetLive(dir, id, *liveSnap(testNow, wslive.Lead{},
			wslive.Agent{ID: "child", Status: wslive.StatusRunning},
			wslive.Agent{ID: "unknown", Status: wslive.StatusDone}))
		if id == "unknown" {
			continue
		}
		if err := wslive.AppendObserved(SessionDir(dir, id), []wslive.ObservedRecord{
			{ModelObservation: wslive.ModelObservation{ID: "lead1", Model: "old", Source: "pi.assistant", At: 1000}},
			{ModelObservation: wslive.ModelObservation{ID: "lead2", Model: "new", Source: "pi.assistant", At: 2000}},
			{AgentID: "child", ModelObservation: wslive.ModelObservation{ID: "child1", Model: "old-child", Source: "pi.assistant", At: 1000}},
			{AgentID: "child", ModelObservation: wslive.ModelObservation{ID: "child2", Model: "new-child", Source: "pi.assistant", At: 2000}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return pageReader{dir: dir, snapshots: cache, index: index, now: func() time.Time { return testNow }}, cache
}

func TestOverlayConsumersRawAndCountOnlyDoNotOpenEvidence(t *testing.T) {
	r, cache := overlayConsumerFixture(t)
	lookup := r.lookup()
	for i := 0; i < 3; i++ {
		if snap := r.readSnapshotWithoutObserved("peer"); snap == nil || snap.Lead.Observed != nil {
			t.Fatalf("raw snapshot unexpectedly overlaid: %+v", snap)
		}
		if lookup.snap("peer").Agents[0].Observed != nil {
			t.Fatal("lookup base unexpectedly overlaid")
		}
		r.indexView()
		got := subagentCounts("self", []wslive.Agent{{ID: "linked", ReplyID: "delegate", Status: wslive.StatusRunning}}, lookup, testNow)
		if got != (SubagentCount{Direct: 1, Deep: 1}) {
			t.Fatalf("delegate counts = %+v", got)
		}
	}
	if got := cache.observed.Opens(); got != 0 {
		t.Fatalf("non-history reads opened %d evidence files", got)
	}
	if r.readSnapshotWithoutObserved("absent") != nil || lookup.observedSnap("absent") != nil {
		t.Fatal("absent snapshot acquired evidence")
	}
	if r.readSnapshot("unknown").Lead.Observed != nil {
		t.Fatal("absent evidence inherited a model")
	}
	// Only the history accessor opens retained records, memoizing its result,
	// including nil. Overlaying must not mutate the shared base agents.
	observed := lookup.observedSnap("peer")
	if got := cache.observed.Opens(); got != 4 {
		t.Fatalf("history opened %d files, want 4", got)
	}
	cache.observed.Forget(SessionDir(r.dir, "peer"))
	if lookup.observedSnap("peer") != observed || cache.observed.Opens() != 4 {
		t.Fatal("observed lookup was not memoized")
	}
	if lookup.snap("peer").Agents[0].Observed != nil {
		t.Fatal("history contaminated the base snapshot")
	}
}

func TestOverlayConsumersPeerHistoryParity(t *testing.T) {
	for _, mode := range []string{"noncompact", "compact", "no-adapter", "stale", "unserved"} {
		t.Run(mode, func(t *testing.T) {
			r, cache := overlayConsumerFixture(t)
			project := "/p"
			if mode == "compact" {
				project = "/other"
			}
			if mode == "no-adapter" || mode == "stale" {
				snap := *r.readSnapshotWithoutObserved("peer")
				if mode == "no-adapter" {
					snap.Adapter = ""
				} else {
					snap.UpdatedAt = testNow.Add(-time.Hour).UnixMilli()
				}
				cache.SetLive(r.dir, "peer", snap)
			}
			if mode == "unserved" {
				delete(r.index.sessions, "peer")
			}
			contacts := []Contact{{ReplyID: "peer", Address: "peer@machine"}}
			got := peerGroup(contacts, project, r.lookup(), testNow)
			wantOpens := int64(0)
			if mode == "noncompact" {
				wantOpens = 4
				if got.Live[0].Agents[0].Observed == nil || len(got.Live[0].Agents[0].Observed.History) != 2 || got.Live[0].Agents[1].Observed != nil {
					t.Fatal("peer child history or unknown behavior lost")
				}
			}
			if cache.observed.Opens() != wantOpens {
				t.Fatalf("%s opened %d, want %d", mode, cache.observed.Opens(), wantOpens)
			}
			// Compare rendered peer markup with the former always-overlaid lookup.
			legacy := r.lookup()
			legacy.snapshot = r.readSnapshot
			legacy.observed = nil
			want := peerGroup(contacts, project, legacy, testNow)
			gotView := historyView(nil, "", PageContent{}, sessionLookup{})
			wantView := gotView
			gotView.Tree.Peers, wantView.Tree.Peers = got, want
			gotHTML, err := renderSessionRegions(gotView, newEntryCache())
			if err != nil {
				t.Fatal(err)
			}
			wantHTML, err := renderSessionRegions(wantView, newEntryCache())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotHTML, wantHTML) {
				t.Fatal("peer rendered output changed")
			}
		})
	}
}

func TestOverlayConsumersFileSnapshotDoesNotSweep(t *testing.T) {
	r, cache := overlayConsumerFixture(t)
	base := *r.readSnapshotWithoutObserved("peer")
	writeSnapshot(t, r.dir, "peer", &base)
	cache.ClearLive(r.dir, "peer")
	for i := 0; i < 3; i++ {
		if got := r.readSnapshotWithoutObserved("peer"); !reflect.DeepEqual(got, &base) {
			t.Fatal("file-backed raw snapshot changed")
		}
	}
	if cache.observed.Opens() != 0 {
		t.Fatal("file-backed raw reads swept retained evidence")
	}
	if r.readSnapshot("peer").Lead.Observed == nil || cache.observed.Opens() != 4 {
		t.Fatal("file-backed history accessor failed to load evidence")
	}
	if got := r.readSnapshotWithoutObserved("peer"); !reflect.DeepEqual(got, &base) {
		t.Fatal("overlay contaminated cached file snapshot")
	}
	// Overlay-free does not strip evidence already in a live snapshot.
	base.Lead.Observed = &wslive.ObservedModel{Current: wslive.ModelObservation{Model: "live"}}
	cache.SetLive(r.dir, "peer", base)
	if r.readSnapshotWithoutObserved("peer").Lead.Observed.Current.Model != "live" {
		t.Fatal("raw read stripped live evidence")
	}
}

func TestOverlayConsumersDetailAndRailRetainHistory(t *testing.T) {
	r, cache := overlayConsumerFixture(t)
	snap := r.readSnapshot("peer")
	if cache.observed.Opens() != 4 || len(snap.Lead.Observed.History) != 2 || len(snap.Agents[0].Observed.History) != 2 {
		t.Fatal("detail/rail accessor lost retained history")
	}
	if snap.Lead.Observed.Change() == nil {
		t.Fatal("rail hover lost latest transition")
	}
}
