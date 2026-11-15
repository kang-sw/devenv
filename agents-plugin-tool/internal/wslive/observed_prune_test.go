package wslive

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

// withPruneLimit sets the prune threshold for one test, with no least time
// between prunes.
func withPruneLimit(t *testing.T, limit int) {
	t.Helper()
	prev, every := observedPruneLimit, observedPruneEvery
	observedPruneLimit, observedPruneEvery = limit, 0
	t.Cleanup(func() { observedPruneLimit, observedPruneEvery = prev, every })
}

func evidencePath(dir string, r ObservedRecord) string {
	sum := sha256.Sum256([]byte(observationKey(r)))
	return filepath.Join(dir, ObservedDirName, hex.EncodeToString(sum[:])+".json")
}

// pruneFixture is one agent's records, a minute apart and ending at now:
// early model changes, a long repeated run, and a recent change.
func pruneFixture(agent string, n int, now time.Time) []ObservedRecord {
	out := make([]ObservedRecord, n)
	for i := range out {
		model := "m1"
		switch {
		case i >= 3 && i < 6:
			model = "m2"
		case i >= n-5:
			model = "m3"
		}
		at := now.Add(-time.Duration(n-1-i) * time.Minute)
		out[i] = ObservedRecord{AgentID: agent, ModelObservation: ModelObservation{ID: fmt.Sprintf("%s-%d", agent, i), Model: model, Source: "claude.assistant", At: at.UnixMilli()}}
	}
	return out
}

// writeFixture appends records without pruning and dates each file at its At.
func writeFixture(t *testing.T, dir string, records []ObservedRecord) {
	t.Helper()
	prev := observedPruneLimit
	observedPruneLimit = 1 << 30
	err := AppendObserved(dir, records)
	observedPruneLimit = prev
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		at := time.UnixMilli(r.At)
		if err := os.Chtimes(evidencePath(dir, r), at, at); err != nil {
			t.Fatal(err)
		}
	}
}

// visibleModels is what every reader shows: the models SeedObserved
// summarizes, and the uncached overlay onto a snapshot naming the agents.
func visibleModels(t *testing.T, dir string) (map[string]*ObservedModel, Snapshot) {
	t.Helper()
	records, err := ReadObserved(dir)
	if err != nil {
		t.Fatal(err)
	}
	st := NewState("")
	st.SeedObserved(records)
	snap := Snapshot{Agents: []Agent{{ID: "child-a"}, {ID: "child-b"}}}
	OverlayObserved(&snap, records)
	return st.observed, snap
}

func evidenceCount(t *testing.T, dir string) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, ObservedDirName, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

func TestObservedPruneKeepsVisibleModels(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name   string
		agents []string
	}{{"one agent", []string{""}}, {"agents", []string{"", "child-a", "child-b"}}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var all []ObservedRecord
			for _, agent := range tc.agents {
				all = append(all, pruneFixture(agent, 150, now)...)
			}
			writeFixture(t, dir, all)
			wantModels, wantSnap := visibleModels(t, dir)
			withPruneLimit(t, 20)
			pruneObserved(filepath.Join(dir, ObservedDirName))
			gotModels, gotSnap := visibleModels(t, dir)
			if !reflect.DeepEqual(gotModels, wantModels) || !reflect.DeepEqual(gotSnap, wantSnap) {
				t.Fatalf("prune changed the visible models:\n%+v\n%+v", gotModels, wantModels)
			}
			// Per agent: the first record and the changes into m2 and back
			// survive, and so does every record within the hour before the
			// newest (61 at a minute apart, the change to m3 and the newest
			// among them).
			kept := evidenceCount(t, dir)
			if want := len(tc.agents) * (3 + 61); kept != want {
				t.Fatalf("kept %d records, want %d", kept, want)
			}
			for _, r := range all {
				_, err := os.Stat(evidencePath(dir, r))
				if now.UnixMilli()-r.At <= observedPruneAge.Milliseconds() && err != nil {
					t.Fatalf("recent record %s deleted", r.ID)
				}
			}
			var cache ObservedCache
			snap := Snapshot{Agents: []Agent{{ID: "child-a"}, {ID: "child-b"}}}
			cache.Load(dir).Overlay(&snap)
			if !reflect.DeepEqual(snap, wantSnap) {
				t.Fatal("cached overlay of the pruned directory differs")
			}
		})
	}
}

func TestObservedPruneSurvivesChildReplay(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	child := pruneFixture("child-a", 150, now)
	all := append(pruneFixture("", 150, now), child...)
	writeFixture(t, dir, all)
	wantModels, wantSnap := visibleModels(t, dir)
	withPruneLimit(t, 20)
	pruneObserved(filepath.Join(dir, ObservedDirName))
	pruned := evidenceCount(t, dir)
	// A child's replay re-adds the repeats its parent pruned; the models stay
	// and the next prune removes them again.
	writeFixture(t, dir, child)
	if gotModels, gotSnap := visibleModels(t, dir); !reflect.DeepEqual(gotModels, wantModels) || !reflect.DeepEqual(gotSnap, wantSnap) {
		t.Fatal("replay changed the visible models")
	}
	pruneObserved(filepath.Join(dir, ObservedDirName))
	if gotModels, gotSnap := visibleModels(t, dir); !reflect.DeepEqual(gotModels, wantModels) || !reflect.DeepEqual(gotSnap, wantSnap) {
		t.Fatal("second prune changed the visible models")
	}
	if got := evidenceCount(t, dir); got != pruned {
		t.Fatalf("second prune kept %d records, first %d", got, pruned)
	}
}

func TestObservedPruneGuardsRecentRecords(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	// Sixty records a minute apart: all inside the hour before the newest.
	records := pruneFixture("", 60, now)
	writeFixture(t, dir, records)
	// A repeat with an old At but a fresh file time stays too.
	old := ObservedRecord{ModelObservation: ModelObservation{ID: "old-at", Model: "m1", Source: "claude.assistant", At: now.Add(-3 * time.Hour).UnixMilli()}}
	first := ObservedRecord{ModelObservation: ModelObservation{ID: "older", Model: "m1", Source: "claude.assistant", At: now.Add(-4 * time.Hour).UnixMilli()}}
	writeFixture(t, dir, []ObservedRecord{first})
	prev := observedPruneLimit
	observedPruneLimit = 1 << 30
	if err := AppendObserved(dir, []ObservedRecord{old}); err != nil {
		t.Fatal(err)
	}
	observedPruneLimit = prev
	withPruneLimit(t, 10)
	pruneObserved(filepath.Join(dir, ObservedDirName))
	if got := evidenceCount(t, dir); got != 62 {
		t.Fatalf("guard deleted records: %d left of 62", got)
	}
}

func TestObservedPruneToleratesFailedDeletes(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	records := pruneFixture("", 150, now)
	writeFixture(t, dir, records)
	want, _ := visibleModels(t, dir)
	vanished, locked := evidencePath(dir, records[20]), evidencePath(dir, records[21])
	prev := removeObserved
	removeObserved = func(path string) error {
		switch path {
		case vanished:
			// Another pruner got there first: the delete finds nothing.
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Remove(path)
		case locked:
			return errors.New("sharing violation")
		}
		return os.Remove(path)
	}
	t.Cleanup(func() { removeObserved = prev })
	withPruneLimit(t, 20)
	pruneObserved(filepath.Join(dir, ObservedDirName))
	if _, err := os.Stat(locked); err != nil {
		t.Fatal("locked record not left for a later prune")
	}
	if got := evidenceCount(t, dir); got != 3+61+1 {
		t.Fatalf("prune stopped at a failed delete: %d left", got)
	}
	if got, _ := visibleModels(t, dir); !reflect.DeepEqual(got, want) {
		t.Fatal("prune with failed deletes changed the visible models")
	}
}

func TestObservedPruneOnlyAboveTheLimitAndOnlyOnAppend(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	records := pruneFixture("", 100, now)
	writeFixture(t, dir, records[:99])
	withPruneLimit(t, 100)
	// The directory reaches the limit: no prune.
	if err := AppendObserved(dir, records[99:]); err != nil {
		t.Fatal(err)
	}
	if got := evidenceCount(t, dir); got != 100 {
		t.Fatalf("pruned at the limit: %d", got)
	}
	// Reads never prune, even over the limit.
	withPruneLimit(t, 50)
	if _, err := ReadObserved(dir); err != nil {
		t.Fatal(err)
	}
	var cache ObservedCache
	cache.Load(dir)
	if got := evidenceCount(t, dir); got != 100 {
		t.Fatalf("a read pruned: %d", got)
	}
	// An append that writes nothing new does not prune either.
	if err := AppendObserved(dir, records[:1]); err != nil {
		t.Fatal(err)
	}
	if got := evidenceCount(t, dir); got != 100 {
		t.Fatalf("an append of a known record pruned: %d", got)
	}
	// The next new record over the limit prunes.
	next := ObservedRecord{ModelObservation: ModelObservation{ID: "next", Model: "m3", Source: "claude.assistant", At: now.Add(time.Second).UnixMilli()}}
	if err := AppendObserved(dir, []ObservedRecord{next}); err != nil {
		t.Fatal(err)
	}
	if got := evidenceCount(t, dir); got >= 100 {
		t.Fatalf("append over the limit did not prune: %d", got)
	}
}

func TestObservedPruneConcurrentReads(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	records := append(pruneFixture("", 200, now), pruneFixture("child-a", 200, now)...)
	writeFixture(t, dir, records)
	_, want := visibleModels(t, dir)
	withPruneLimit(t, 20)
	var cache ObservedCache
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := ReadObserved(dir); err != nil {
					t.Error(err)
					return
				}
				if cache.Load(dir) == nil {
					t.Error("evidence vanished during the prune")
					return
				}
			}
		}()
	}
	pruneObserved(filepath.Join(dir, ObservedDirName))
	close(stop)
	wg.Wait()
	if _, got := visibleModels(t, dir); !reflect.DeepEqual(got, want) {
		t.Fatal("prune under concurrent reads changed the visible models")
	}
}

func TestObservedPruneRunsOncePerIntervalPerDirectory(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	records := pruneFixture("", 150, now)
	writeFixture(t, dir, records)
	withPruneLimit(t, 20)
	observedPruneEvery = time.Hour
	evidenceDir := filepath.Join(dir, ObservedDirName)
	pruneObserved(evidenceDir)
	pruned := evidenceCount(t, dir)
	// The replayed repeats wait for the interval, though still over the limit.
	writeFixture(t, dir, records)
	pruneObserved(evidenceDir)
	if got := evidenceCount(t, dir); got != len(records) {
		t.Fatalf("pruned again inside the interval: %d", got)
	}
	pruneMu.Lock()
	prunedAt[evidenceDir] = time.Now().Add(-2 * time.Hour)
	pruneMu.Unlock()
	pruneObserved(evidenceDir)
	if got := evidenceCount(t, dir); got != pruned {
		t.Fatalf("prune after the interval kept %d, want %d", got, pruned)
	}
}
