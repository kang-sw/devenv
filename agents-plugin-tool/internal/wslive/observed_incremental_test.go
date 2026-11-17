package wslive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Writes metadata directly so this read-path fixture is independent of retention.
func writeObservedFixture(tb testing.TB, dir, name string, r ObservedRecord) {
	tb.Helper()
	path := filepath.Join(dir, ObservedDirName)
	if err := os.MkdirAll(path, 0700); err != nil {
		tb.Fatal(err)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, name+".json"), raw, 0600); err != nil {
		tb.Fatal(err)
	}
}

func TestObservedCacheIncrementalCanonicalParity(t *testing.T) {
	dir := t.TempDir()
	clock := time.Now()
	cache := ObservedCache{now: func() time.Time { return clock }}
	record := func(id, model, agent string, at int64) ObservedRecord {
		r := observedCacheRecord(id, model)
		r.AgentID = agent
		r.At = at
		return r
	}
	writeObservedFixture(t, dir, "m", record("one", "first", "", 1000))
	writeObservedFixture(t, dir, "child", record("one", "child-model", "child", 1000))
	old := cache.Load(dir)
	frozen, _ := json.Marshal(old.models)
	assertParity := func() *ObservedEvidence {
		t.Helper()
		clock = clock.Add(observedResweep) // replacements and coarse-mtime files too
		got := cache.Load(dir)
		raw, err := ReadObserved(dir)
		if err != nil {
			t.Fatal(err)
		}
		state := NewState("")
		state.SeedObserved(raw)
		if !reflect.DeepEqual(got.models, state.observed) {
			t.Fatalf("incremental differs from canonical: %+v vs %+v", got.models, state.observed)
		}
		after, _ := json.Marshal(old.models)
		if string(after) != string(frozen) {
			t.Fatal("published projection mutated")
		}
		return got
	}
	writeObservedFixture(t, dir, "z", record("two", "second", "", 2000))
	next := assertParity()
	if next == old || next.models["child"] != old.models["child"] || len(next.models[""].History) != 2 {
		t.Fatal("append did not incrementally share untouched producer")
	}
	// A late response splits an existing same-model span.
	writeObservedFixture(t, dir, "late", record("late", "middle", "", 1500))
	assertParity()
	// Equal times order by source/ID, never by file name.
	writeObservedFixture(t, dir, "equal-z", record("z", "tie-last", "", 2000))
	writeObservedFixture(t, dir, "equal-a", record("a", "tie-first", "", 2000))
	assertParity()
	// Lexically later duplicate loses; earlier duplicate takes precedence.
	writeObservedFixture(t, dir, "zz-duplicate", record("two", "must-lose", "", 9000))
	assertParity()
	writeObservedFixture(t, dir, "a-duplicate", record("two", "winner", "", 1100))
	assertParity()
	// Removing that winner restores the previously shadowed duplicate.
	if err := os.Remove(filepath.Join(dir, ObservedDirName, "a-duplicate.json")); err != nil {
		t.Fatal(err)
	}
	assertParity()
	// Same-count deletion/addition and same-name replacement must not take the
	// append fast path, including replacement by invalid/unsupported evidence.
	if err := os.Remove(filepath.Join(dir, ObservedDirName, "z.json")); err != nil {
		t.Fatal(err)
	}
	writeObservedFixture(t, dir, "new-count", record("new-count", "latest", "", 10000))
	assertParity()
	writeObservedFixture(t, dir, "m", record("one", "replacement", "", 3000))
	assertParity()
	invalid := record("invalid", "unknown", "", 4000)
	invalid.Source = "future.source"
	writeObservedFixture(t, dir, "m", invalid)
	assertParity()
	// Newer live evidence is never overwritten, and no absent agent is revived.
	snap := Snapshot{Lead: Lead{Observed: &ObservedModel{Current: ModelObservation{Model: "live", At: 20000}}}, Agents: []Agent{{ID: "absent"}}}
	cache.Load(dir).Overlay(&snap)
	if snap.Lead.Observed.Current.Model != "live" || snap.Agents[0].Observed != nil || len(snap.Agents) != 1 {
		t.Fatal("overlay changed newer or absent evidence")
	}
}

func BenchmarkObservedCacheNewResponse4000(b *testing.B) {
	dir := b.TempDir()
	for i := 0; i < 4000; i++ {
		r := observedCacheRecord(fmt.Sprint(i), fmt.Sprintf("model-%d", i%3))
		r.At = int64(i + 1)
		writeObservedFixture(b, dir, fmt.Sprintf("%06d", i), r)
	}
	clock := time.Now().Add(time.Hour)
	cache := ObservedCache{now: func() time.Time { return clock }}
	cache.Load(dir)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		r := observedCacheRecord(fmt.Sprintf("new-%d", i), "new-model")
		r.At = int64(5000 + i)
		writeObservedFixture(b, dir, "new", r)
		// Replace the prior iteration's name only outside the measured load. A
		// distinct name makes each measured load the ordinary new-response path.
		path := filepath.Join(dir, ObservedDirName, "new.json")
		next := filepath.Join(dir, ObservedDirName, fmt.Sprintf("new-%06d.json", i))
		if err := os.Rename(path, next); err != nil {
			b.Fatal(err)
		}
		clock = clock.Add(observedValidationInterval)
		if err := os.Chtimes(filepath.Join(dir, ObservedDirName), clock, clock); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		cache.Load(dir)
	}
}
