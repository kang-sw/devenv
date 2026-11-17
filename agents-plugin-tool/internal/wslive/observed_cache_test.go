package wslive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func observedCacheRecord(id, model string) ObservedRecord {
	return ObservedRecord{ModelObservation: ModelObservation{ID: id, Model: model, Source: "pi.assistant", At: 1000}}
}

func TestObservedCacheUnchangedOpensAndProjection(t *testing.T) {
	dir := t.TempDir()
	if err := AppendObserved(dir, []ObservedRecord{observedCacheRecord("one", "first")}); err != nil {
		t.Fatal(err)
	}
	opens, stats, dirStats := 0, 0, 0
	// The clock runs well past every file time, so the directory stat is
	// trusted between sweeps; the test advances it to force one.
	clock := time.Now().Add(time.Hour)
	cache := ObservedCache{
		open: func(path string) (*os.File, error) { opens++; return os.Open(path) },
		stat: func(path string) (os.FileInfo, error) {
			if filepath.Base(path) == ObservedDirName {
				dirStats++
			} else {
				stats++
			}
			return os.Stat(path)
		},
		now: func() time.Time { return clock },
	}
	first := cache.Load(dir)
	if opens != 1 || stats != 1 {
		t.Fatalf("cold opens=%d stats=%d", opens, stats)
	}
	for i := 0; i < 20; i++ {
		if cache.Load(dir) != first {
			t.Fatal("unchanged projection rebuilt")
		}
	}
	if opens != 1 || stats != 1 || dirStats != 1 {
		t.Fatalf("unchanged opens=%d record stats=%d directory stats=%d, want 1 each", opens, stats, dirStats)
	}
	files, _ := filepath.Glob(filepath.Join(dir, ObservedDirName, "*.json"))
	path := files[0]
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	// An in-place child edit keeps the directory identity/mtime unchanged.
	raw, _ := json.Marshal(observedCacheRecord("one", "second-longer"))
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Dir(path), dirInfo.ModTime(), dirInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	// Write-once files make this edit illegitimate: the directory stat hides
	// it until the forced re-sweep, which picks it up.
	if cache.Load(dir) != first || opens != 1 {
		t.Fatalf("trusted directory re-read a child: opens=%d", opens)
	}
	clock = clock.Add(observedResweep)
	changed := cache.Load(dir)
	if opens != 2 || changed == first || changed.models[""].Current.Model != "second-longer" {
		t.Fatalf("child edit missed by the forced re-sweep: opens=%d", opens)
	}
	// Atomic replacement with identical size, mode and timestamp is no
	// legitimate write either: a listing reuses the known name, and the
	// forced re-sweep's SameFile check reloads it.
	info, _ := os.Stat(path)
	raw, _ = json.Marshal(observedCacheRecord("one", "third-longerr"))
	if int64(len(raw)) != info.Size() {
		t.Fatal("test replacement must preserve size")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmp, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	if got := cache.Load(dir); opens != 2 || got != changed {
		t.Fatalf("listing re-read a known name: opens=%d", opens)
	}
	// The fake clock trusts every real mtime as old, so the real writes
	// below must move the directory mtime away from the one the cache
	// recorded. Windows stamps file times from a coarse system tick, and the
	// rename above and the remove below can share one; production's racy
	// window (real clock) distrusts such a recent mtime, the fake clock
	// cannot. settle backdates the directory before the load that records
	// it, as the hour the clock skipped would have.
	evidenceDir := filepath.Dir(path)
	backdate := time.Now().Add(-time.Hour)
	settle := func() {
		t.Helper()
		backdate = backdate.Add(-time.Minute)
		if err := os.Chtimes(evidenceDir, backdate, backdate); err != nil {
			t.Fatal(err)
		}
	}
	settle()
	clock = clock.Add(observedResweep)
	if got := cache.Load(dir); opens != 3 || got.models[""].Current.Model != "third-longerr" {
		t.Fatalf("replacement missed by the forced re-sweep: opens=%d", opens)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(observedValidationInterval)
	if got := cache.Load(dir); len(got.models) != 0 || opens != 3 {
		t.Fatal("deleted evidence retained or opened")
	}
	settle()
	clock = clock.Add(observedValidationInterval)
	if got := cache.Load(dir); len(got.models) != 0 || opens != 3 {
		t.Fatal("a backdated empty directory re-read records")
	}
	if err := AppendObserved(dir, []ObservedRecord{observedCacheRecord("two", "new")}); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(observedValidationInterval)
	cache.Load(dir)
	if opens != 4 {
		t.Fatal("new record not read")
	}
	cache.Forget(dir)
	cache.Load(dir)
	if opens != 5 {
		t.Fatal("Forget kept records")
	}
}

func TestObservedCacheDirectoryGate(t *testing.T) {
	dir := t.TempDir()
	if err := AppendObserved(dir, []ObservedRecord{observedCacheRecord("one", "first")}); err != nil {
		t.Fatal(err)
	}
	stats, opens, dirStats := 0, 0, 0
	clock := time.Now()
	// Keep directory writes on the injected clock's timeline. Otherwise a
	// real Windows mtime tick can cover both append and removal while the
	// advanced fake clock incorrectly makes that tick look safe to trust.
	stamp := func(at time.Time) {
		t.Helper()
		if err := os.Chtimes(filepath.Join(dir, ObservedDirName), at, at); err != nil {
			t.Fatal(err)
		}
	}
	stamp(clock)
	cache := ObservedCache{
		open: func(path string) (*os.File, error) { opens++; return os.Open(path) },
		stat: func(path string) (os.FileInfo, error) {
			if filepath.Base(path) == ObservedDirName {
				dirStats++
			} else {
				stats++
			}
			return os.Stat(path)
		},
		now: func() time.Time { return clock },
	}
	// A directory mtime inside the racy window of its stat costs a listing
	// once per validation interval, never a known record stat.
	cache.Load(dir)
	cache.Load(dir)
	if stats != 1 || opens != 1 || dirStats != 1 {
		t.Fatalf("racy listing: record stats=%d opens=%d directory stats=%d", stats, opens, dirStats)
	}
	// Once the mtime is old enough, an unchanged directory is trusted.
	clock = clock.Add(time.Second * 3)
	first := cache.Load(dir)
	if cache.Load(dir) != first || stats != 1 {
		t.Fatalf("unchanged directory re-stat'ed its records: stats=%d", stats)
	}
	// An added record moves the directory: only the new name is stat'ed
	// and read.
	r := observedCacheRecord("two", "second")
	r.At = 2000
	if err := AppendObserved(dir, []ObservedRecord{r}); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	stamp(clock)
	got := cache.Load(dir)
	if got == first || got.models[""].Current.Model != "second" {
		t.Fatal("added record missed")
	}
	if stats != 2 || opens != 2 {
		t.Fatalf("listing stat'ed known records: stats=%d opens=%d", stats, opens)
	}
	want := Snapshot{}
	retained, err := ReadObserved(dir)
	if err != nil {
		t.Fatal(err)
	}
	OverlayObserved(&want, retained)
	snap := Snapshot{}
	got.Overlay(&snap)
	if !reflect.DeepEqual(snap, want) {
		t.Fatalf("gated overlay differs: %+v vs %+v", snap, want)
	}
	// A vanished record drops on the next listing, even when removal keeps
	// the append's directory mtime tick. It was recent at that listing, so
	// the racy window must prevent trusting it now.
	files, _ := filepath.Glob(filepath.Join(dir, ObservedDirName, "*.json"))
	for _, path := range files {
		raw, _ := os.ReadFile(path)
		var rec ObservedRecord
		if json.Unmarshal(raw, &rec) == nil && rec.ID == "two" {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
	}
	clock = clock.Add(time.Second)
	stamp(clock.Add(-time.Second))
	dropped := cache.Load(dir)
	if dropped.models[""].Current.Model != "first" || len(dropped.models[""].History) != 1 || stats != 2 {
		t.Fatalf("vanished record kept: stats=%d", stats)
	}
	// The forced re-sweep stats every record even when nothing moved, and
	// rebuilds nothing.
	clock = clock.Add(time.Second * 3)
	trusted := cache.Load(dir)
	before := stats
	clock = clock.Add(observedResweep)
	if cache.Load(dir) != trusted || stats != before+1 {
		t.Fatalf("forced re-sweep: stats=%d rebuilt=%t", stats-before, cache.Load(dir) != trusted)
	}
	// A clock that steps backwards forces a sweep too.
	clock = clock.Add(-time.Hour)
	cache.Load(dir)
	if stats != before+2 {
		t.Fatalf("backward clock trusted the directory: %d", stats-before)
	}
}

func TestObservedCacheMatchesUncachedValidationAndOverlay(t *testing.T) {
	dir := t.TempDir()
	records := []ObservedRecord{observedCacheRecord("one", "first"), observedCacheRecord("two", "second"), {AgentID: "child", ModelObservation: ModelObservation{ID: "child-output", Model: "child-model", Source: "claude.assistant", At: 2000}}, observedCacheRecord("bad", "sonnet")}
	records[1].At = 3000
	records[3].Source = "future.source"
	if err := AppendObserved(dir, records); err != nil {
		t.Fatal(err)
	}
	evidenceDir := filepath.Join(dir, ObservedDirName)
	for name, raw := range map[string]string{"bad.json": "{", "large.json": string(make([]byte, 4097)), "ignored.tmp": "{}", "future.json": `{"id":"future","model":"x","source":"future","at":1}`} {
		if err := os.WriteFile(filepath.Join(evidenceDir, name), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	base := Snapshot{Lead: Lead{Observed: &ObservedModel{Current: ModelObservation{Model: "newer-live", At: 4000}}}, Agents: []Agent{{ID: "child"}, {ID: "missing"}}}
	want := base
	retained, err := ReadObserved(dir)
	if err != nil {
		t.Fatal(err)
	}
	OverlayObserved(&want, retained)
	clock := time.Now()
	cache := ObservedCache{now: func() time.Time { return clock }}
	for i := 0; i < 3; i++ {
		got := base
		cache.Load(dir).Overlay(&got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("cached overlay differs: %+v vs %+v", got, want)
		}
	}
	if cache.Opens() != int64(len(records)+3) {
		t.Fatalf("invalid records reopened: %d", cache.Opens())
	}
	// A vanished directory does not keep evidence alive and may reappear.
	if err := os.RemoveAll(evidenceDir); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(observedValidationInterval)
	if cache.Load(dir) != nil {
		t.Fatal("removed directory served cached evidence")
	}
	if err := AppendObserved(dir, []ObservedRecord{observedCacheRecord("new", "restored")}); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(observedValidationInterval)
	if got := cache.Load(dir); got.models[""].Current.Model != "restored" {
		t.Fatal("directory recreation missed")
	}
}

func TestObservedCacheMissingDirectoryValidation(t *testing.T) {
	dir := t.TempDir()
	clock := time.Now()
	var stats atomic.Int64
	cache := ObservedCache{
		now: func() time.Time { return clock },
		stat: func(path string) (os.FileInfo, error) {
			stats.Add(1)
			return os.Stat(path)
		},
	}
	burst := func() {
		t.Helper()
		var wg sync.WaitGroup
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if cache.Load(dir) != nil {
					t.Error("missing directory returned evidence")
				}
			}()
		}
		wg.Wait()
	}
	burst()
	clock = clock.Add(observedValidationInterval - time.Nanosecond)
	burst()
	if stats.Load() != 1 {
		t.Fatalf("missing burst: stats=%d, want 1", stats.Load())
	}
	clock = clock.Add(time.Nanosecond)
	burst()
	if stats.Load() != 2 {
		t.Fatalf("missing validation: stats=%d, want 2", stats.Load())
	}
	if err := AppendObserved(dir, []ObservedRecord{observedCacheRecord("one", "new")}); err != nil {
		t.Fatal(err)
	}
	burst() // Creation shares the previous missing result until validation.
	cache.Forget(dir)
	if got := cache.Load(dir); got == nil || got.models[""].Current.Model != "new" || stats.Load() != 4 {
		t.Fatalf("Forget did not expose new evidence: stats=%d", stats.Load())
	}
}

func TestObservedCacheValidationSharedAcrossReaders(t *testing.T) {
	dir := t.TempDir()
	appendRecord := func(id, model string, at int64) {
		t.Helper()
		r := observedCacheRecord(id, model)
		r.At = at
		if err := AppendObserved(dir, []ObservedRecord{r}); err != nil {
			t.Fatal(err)
		}
	}
	appendRecord("one", "first", 1000)
	clock := time.Now()
	var dirStats, recordStats atomic.Int64
	cache := ObservedCache{
		now: func() time.Time { return clock },
		stat: func(path string) (os.FileInfo, error) {
			if filepath.Base(path) == ObservedDirName {
				dirStats.Add(1)
			} else {
				recordStats.Add(1)
			}
			return os.Stat(path)
		},
	}
	burst := func(want string) {
		t.Helper()
		// Warm concurrent readers may use the prior immutable evidence while
		// a refresh is in flight. Complete the refresh before pinning the
		// new projection for this validation-sharing assertion.
		if got := cache.Load(dir); got == nil || got.models[""].Current.Model != want {
			t.Fatalf("refresh did not return %s", want)
		}
		var wg sync.WaitGroup
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if got := cache.Load(dir); got == nil || got.models[""].Current.Model != want {
					t.Errorf("concurrent load did not return %s", want)
				}
			}()
		}
		wg.Wait()
	}
	burst("first")
	first := cache.Load(dir)
	clock = clock.Add(observedValidationInterval - time.Nanosecond)
	burst("first")
	if dirStats.Load() != 1 || recordStats.Load() != 1 || cache.Opens() != 1 {
		t.Fatalf("within interval: directory=%d record=%d opens=%d", dirStats.Load(), recordStats.Load(), cache.Opens())
	}
	clock = clock.Add(time.Nanosecond)
	burst("first")
	if dirStats.Load() != 2 || recordStats.Load() != 1 {
		t.Fatalf("boundary: directory=%d record=%d", dirStats.Load(), recordStats.Load())
	}
	appendRecord("two", "second", 2000)
	burst("first") // External appends wait for the next shared validation.
	clock = clock.Add(observedValidationInterval)
	burst("second")
	if dirStats.Load() != 3 || recordStats.Load() != 2 || cache.Opens() != 2 {
		t.Fatalf("append: directory=%d record=%d opens=%d", dirStats.Load(), recordStats.Load(), cache.Opens())
	}
	second := cache.Load(dir)
	cache.Invalidate(dir)
	if cache.Load(dir) != second || dirStats.Load() != 4 || recordStats.Load() != 2 || cache.Opens() != 2 {
		t.Fatalf("unchanged invalidation discarded decoded evidence: directory=%d record=%d opens=%d", dirStats.Load(), recordStats.Load(), cache.Opens())
	}
	appendRecord("three", "third", 3000)
	// Keep the directory stamp on the fake clock, including coarse-mtime hosts.
	if err := os.Chtimes(filepath.Join(dir, ObservedDirName), clock, clock); err != nil {
		t.Fatal(err)
	}
	cache.Invalidate(dir)
	burst("third") // Explicit invalidation bypasses the interval, retaining reads.
	if dirStats.Load() != 5 || recordStats.Load() != 3 || cache.Opens() != 3 {
		t.Fatalf("Invalidate: directory=%d record=%d opens=%d", dirStats.Load(), recordStats.Load(), cache.Opens())
	}
	if first.models[""].Current.Model != "first" {
		t.Fatal("published evidence mutated")
	}
}

func TestObservedCacheConcurrentSnapshots(t *testing.T) {
	dir := t.TempDir()
	if err := AppendObserved(dir, []ObservedRecord{observedCacheRecord("one", "first")}); err != nil {
		t.Fatal(err)
	}
	var cache ObservedCache
	evidence := cache.Load(dir)
	done := make(chan struct{}, 8)
	for n := 0; n < 8; n++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 20; j++ {
				snap := Snapshot{Agents: []Agent{{ID: "child"}}}
				cache.Load(dir).Overlay(&snap)
				evidence.Overlay(&snap)
				if snap.Lead.Observed == nil {
					t.Error("missing evidence")
				}
			}
		}()
	}
	r := observedCacheRecord("two", "second")
	r.At = time.Now().UnixMilli()
	if err := AppendObserved(dir, []ObservedRecord{r}); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 8; n++ {
		<-done
	}
	if evidence.models[""].Current.Model != "first" {
		t.Fatal("published evidence mutated")
	}
}

func TestObservedCacheEvictsLeastRecentlyUsedSession(t *testing.T) {
	base := t.TempDir()
	dirs := make([]string, observedSessionCap+1)
	for i := range dirs {
		dirs[i] = filepath.Join(base, "s"+strconv.Itoa(i))
	}
	cache := ObservedCache{}
	for _, dir := range dirs[:observedSessionCap] {
		cache.Load(dir)
	}
	hot := dirs[0]
	cache.Load(hot) // most recently used now; dirs[1] is the oldest
	hotSlot := cache.sessions[hot]
	cache.Load(dirs[observedSessionCap])
	if len(cache.sessions) != observedSessionCap {
		t.Fatalf("slots = %d, want %d", len(cache.sessions), observedSessionCap)
	}
	if cache.sessions[hot] != hotSlot {
		t.Fatal("recently used session was evicted")
	}
	if _, ok := cache.sessions[dirs[1]]; ok {
		t.Fatal("least recently used session survived")
	}
	for _, dir := range dirs[2:] {
		if cache.sessions[dir] == nil {
			t.Fatalf("session %s evicted out of order", dir)
		}
	}
}
