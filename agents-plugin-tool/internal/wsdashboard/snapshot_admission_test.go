package wsdashboard

import (
	"os"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestSnapshotCacheFreshReadsAndTargetedInvalidation(t *testing.T) {
	dir, id := t.TempDir(), "session"
	sessionDir := SessionDir(dir, id)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	original := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: clock.UnixMilli()}
	writeSnapshot(t, dir, id, &original)
	if err := wslive.AppendObserved(sessionDir, []wslive.ObservedRecord{{ModelObservation: wslive.ModelObservation{ID: "one", Model: "first", Source: "pi.assistant", At: 1000}}}); err != nil {
		t.Fatal(err)
	}
	loads, stats := 0, 0
	cache := SnapshotCache{load: func(path string) (parsedSnapshot, error) {
		loads++
		return loadSnapshot(path)
	}}
	cache.files.now = func() time.Time { return clock }
	cache.files.stat = func(path string) (os.FileInfo, error) { stats++; return os.Stat(path) }
	if warm := cache.Snapshot(dir, id); warm == nil || warm.Adapter != "pi" {
		t.Fatal("presentation cache was not warmed")
	}
	evidence := cache.observed.Load(sessionDir)
	changed := original
	changed.Adapter = "claude"
	writeSnapshot(t, dir, id, &changed)
	if fresh := cache.FreshSnapshot(dir, id); fresh == nil || fresh.Adapter != "claude" || fresh.Lead.Observed != nil {
		t.Fatal("fresh read trusted presentation evidence or loaded retained history")
	}
	if cached := cache.Snapshot(dir, id); cached == nil || cached.Adapter != "pi" {
		t.Fatal("fresh admission read unexpectedly changed presentation cache")
	}
	if loads != 2 || stats != 1 || cache.observed.Opens() != 1 {
		t.Fatalf("warm + fresh reads: loads=%d stats=%d observed opens=%d", loads, stats, cache.observed.Opens())
	}

	cache.Invalidate(dir, id)
	if known := cache.lastKnown(dir, id); known == nil || known.Adapter != "pi" {
		t.Fatal("invalidation discarded last-known evidence")
	}
	if cache.observed.Load(sessionDir) != evidence {
		t.Fatal("snapshot invalidation discarded observed history")
	}
	if refreshed := cache.Snapshot(dir, id); refreshed == nil || refreshed.Adapter != "claude" || loads != 3 || stats != 2 {
		t.Fatalf("invalidation did not force validation: loads=%d stats=%d", loads, stats)
	}

	path := wslive.SnapshotPath(sessionDir)
	if err := os.WriteFile(path, []byte("bad json"), 0600); err != nil {
		t.Fatal(err)
	}
	if cache.FreshSnapshot(dir, id) != nil || loads != 4 {
		t.Fatal("fresh read retained a corrupt snapshot")
	}
	cache.SetLive(dir, id, original)
	cache.Invalidate(dir, id)
	if fresh := cache.FreshSnapshot(dir, id); fresh == nil || fresh.Adapter != "pi" || loads != 4 {
		t.Fatal("fresh read or invalidation lost hosted RAM precedence")
	}
	if hosted := cache.Snapshot(dir, id); hosted == nil || hosted.Adapter != "pi" || stats != 2 {
		t.Fatal("presentation invalidation probed disk despite hosted RAM")
	}

	cache.ClearLive(dir, id)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if cache.Snapshot(dir, id) != nil || stats != 3 {
		t.Fatal("missing snapshot did not create failed validation")
	}
	writeSnapshot(t, dir, id, &changed)
	cache.Invalidate(dir, id)
	if got := cache.Snapshot(dir, id); got == nil || got.Adapter != "claude" || stats != 4 {
		t.Fatal("invalidation retained a failed-stat memo")
	}

	before := cache.observed.Opens()
	if err := wslive.AppendObserved(sessionDir, []wslive.ObservedRecord{{ModelObservation: wslive.ModelObservation{ID: "two", Model: "second", Source: "pi.assistant", At: 2000}}}); err != nil {
		t.Fatal(err)
	}
	cache.Invalidate(dir, id) // Evidence-only signals must bypass its TTL too.
	if got := cache.Snapshot(dir, id); got == nil || got.Lead.Observed == nil || got.Lead.Observed.Current.Model != "second" || cache.observed.Opens() != before+1 {
		t.Fatalf("evidence-only invalidation missed new evidence or reopened retained records: opens=%d, before=%d", cache.observed.Opens(), before)
	}
}
