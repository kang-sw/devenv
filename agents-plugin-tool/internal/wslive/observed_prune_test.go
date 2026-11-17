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

// withoutPruneInterval allows each explicit prune in a test to run.
func withoutPruneInterval(t *testing.T) {
	t.Helper()
	every := observedPruneEvery
	observedPruneEvery = 0
	t.Cleanup(func() { observedPruneEvery = every })
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
	// A fixture write must not prune, even with no directory threshold.
	pruneMu.Lock()
	err := AppendObserved(dir, records)
	pruneMu.Unlock()
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
			withoutPruneInterval(t)
			pruneObserved(filepath.Join(dir, ObservedDirName))
			gotModels, gotSnap := visibleModels(t, dir)
			if !reflect.DeepEqual(gotModels, wantModels) || !reflect.DeepEqual(gotSnap, wantSnap) {
				t.Fatalf("prune changed the visible models:\n%+v\n%+v", gotModels, wantModels)
			}
			// Per agent: first, the changes into m2 and back, and recent eight
			// (including the change to m3 and the latest). This replaces the
			// old age-policy count, not the equivalence assertions above.
			kept := evidenceCount(t, dir)
			if want := len(tc.agents) * (3 + observedPruneRecent); kept != want {
				t.Fatalf("kept %d records, want %d", kept, want)
			}
			for _, r := range all {
				_, err := os.Stat(evidencePath(dir, r))
				if now.UnixMilli()-r.At < int64(observedPruneRecent)*time.Minute.Milliseconds() && err != nil {
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
	withoutPruneInterval(t)
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

func TestObservedPruneBoundsShortLivedAgentsAndFreshReplay(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	var records []ObservedRecord
	for _, agent := range []string{"", "child-a", "child-b"} {
		part := pruneFixture(agent, 150, now)
		for i := range part {
			// Each agent's entire life fits within three minutes.
			part[i].At = now.Add(time.Duration(i-149) * time.Second).UnixMilli()
		}
		records = append(records, part...)
	}
	writeFixture(t, dir, records)
	wantModels, wantSnap := visibleModels(t, dir)
	withoutPruneInterval(t)
	pruneObserved(filepath.Join(dir, ObservedDirName))
	if got := evidenceCount(t, dir); got != 3*(3+observedPruneRecent) {
		t.Fatalf("short-lived repeats not bounded after prune: %d", got)
	}
	// Replay old At in a burst with fresh mtimes. Neither age guard remains.
	writeFixture(t, dir, records)
	for _, r := range records {
		if err := os.Chtimes(evidencePath(dir, r), now, now); err != nil {
			t.Fatal(err)
		}
	}
	pruneObserved(filepath.Join(dir, ObservedDirName))
	if got := evidenceCount(t, dir); got != 3*(3+observedPruneRecent) {
		t.Fatalf("fresh replay repeats not bounded after prune: %d", got)
	}
	if gotModels, gotSnap := visibleModels(t, dir); !reflect.DeepEqual(gotModels, wantModels) || !reflect.DeepEqual(gotSnap, wantSnap) {
		t.Fatal("short-lived/fresh replay prune changed visible models")
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
	withoutPruneInterval(t)
	pruneObserved(filepath.Join(dir, ObservedDirName))
	if _, err := os.Stat(locked); err != nil {
		t.Fatal("locked record not left for a later prune")
	}
	if got := evidenceCount(t, dir); got != 3+observedPruneRecent+1 {
		t.Fatalf("prune stopped at a failed delete: %d left", got)
	}
	if got, _ := visibleModels(t, dir); !reflect.DeepEqual(got, want) {
		t.Fatal("prune with failed deletes changed the visible models")
	}
}

func TestObservedPruneOnlyOnNewAppendWithoutThreshold(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	records := pruneFixture("", 100, now)
	writeFixture(t, dir, records)
	withoutPruneInterval(t)
	// Reads never prune, even when the directory has deletable repeats.
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
	// A new record prunes even far below the retired 2256 threshold.
	next := ObservedRecord{ModelObservation: ModelObservation{ID: "next", Model: "m3", Source: "claude.assistant", At: now.Add(time.Second).UnixMilli()}}
	if err := AppendObserved(dir, []ObservedRecord{next}); err != nil {
		t.Fatal(err)
	}
	if got := evidenceCount(t, dir); got != 3+observedPruneRecent {
		t.Fatalf("new append did not prune below the old threshold: %d", got)
	}
}

func TestObservedPruneConcurrentReads(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	records := append(pruneFixture("", 200, now), pruneFixture("child-a", 200, now)...)
	writeFixture(t, dir, records)
	_, want := visibleModels(t, dir)
	withoutPruneInterval(t)
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
	withoutPruneInterval(t)
	observedPruneEvery = time.Hour
	evidenceDir := filepath.Join(dir, ObservedDirName)
	pruneObserved(evidenceDir)
	pruned := evidenceCount(t, dir)
	// The replayed repeats exceed the keep-set until an eligible pass.
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

// This is the accepted late-different-model residual, not a static-prune or
// same-model replay equivalence assertion. Deleted original evidence cannot
// reconstruct the earlier return transition after a new model splits its span.
func TestObservedPruneLateDifferentModelRedatesReturn(t *testing.T) {
	dir := t.TempDir()
	var records []ObservedRecord
	for i := 1; i <= 12; i++ {
		records = append(records, ObservedRecord{ModelObservation: ModelObservation{
			ID: fmt.Sprintf("y-%02d", i), Model: "Y", Source: "pi.assistant", At: int64(i * 1000),
		}})
	}
	writeFixture(t, dir, records)
	want, _ := visibleModels(t, dir)
	clock := time.Now()
	cache := ObservedCache{now: func() time.Time { return clock }}
	cache.Load(dir)
	withoutPruneInterval(t)
	pruneObserved(filepath.Join(dir, ObservedDirName))
	if got, _ := visibleModels(t, dir); !reflect.DeepEqual(got, want) {
		t.Fatal("static prune changed the fold before late insertion")
	}
	if got := evidenceCount(t, dir); got != 1+observedPruneRecent {
		t.Fatalf("kept %d, want first plus eight", got)
	}
	late := ObservedRecord{ModelObservation: ModelObservation{ID: "late-x", Model: "X", Source: "pi.assistant", At: 1500}}
	if err := AppendObserved(dir, []ObservedRecord{late}); err != nil {
		t.Fatal(err)
	}
	full := NewState("")
	full.SeedObserved(append(records, late))
	retained, _ := visibleModels(t, dir)
	wantFull := []ModelChange{
		{Model: "Y", At: 1000, Source: "pi.assistant"},
		{Previous: "Y", Model: "X", At: 1500, Source: "pi.assistant"},
		{Previous: "X", Model: "Y", At: 2000, Source: "pi.assistant"},
	}
	wantRetained := append([]ModelChange(nil), wantFull...)
	wantRetained[2].At = 5000
	if !reflect.DeepEqual(full.observed[""].History, wantFull) || !reflect.DeepEqual(retained[""].History, wantRetained) {
		t.Fatalf("expected Y/X/Y return drift: full=%+v retained=%+v", full.observed[""].History, retained[""].History)
	}
	if !reflect.DeepEqual(retained[""].Current, records[11].ModelObservation) || !reflect.DeepEqual(full.observed[""].Current, retained[""].Current) {
		t.Fatal("late insertion changed latest Current")
	}
	clock = clock.Add(observedValidationInterval)
	if got := cache.Load(dir); !reflect.DeepEqual(got.models, retained) {
		t.Fatal("warm cache did not rebuild the accepted retained-history drift")
	}
}
