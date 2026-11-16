package wslive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
	opens, stats := 0, 0
	// The clock runs well past every file time, so the directory stat is
	// trusted between sweeps; the test advances it to force one.
	clock := time.Now().Add(time.Hour)
	cache := ObservedCache{
		open: func(path string) (*os.File, error) { opens++; return os.Open(path) },
		stat: func(path string) (os.FileInfo, error) { stats++; return os.Stat(path) },
		now:  func() time.Time { return clock },
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
	if opens != 1 || stats != 1 {
		t.Fatalf("unchanged files opened %d times, stat'ed %d times", opens-1, stats-1)
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
	if got := cache.Load(dir); len(got.models) != 0 || opens != 3 {
		t.Fatal("deleted evidence retained or opened")
	}
	settle()
	if got := cache.Load(dir); len(got.models) != 0 || opens != 3 {
		t.Fatal("a backdated empty directory re-read records")
	}
	if err := AppendObserved(dir, []ObservedRecord{observedCacheRecord("two", "new")}); err != nil {
		t.Fatal(err)
	}
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
	stats, opens := 0, 0
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
		stat: func(path string) (os.FileInfo, error) { stats++; return os.Stat(path) },
		now:  func() time.Time { return clock },
	}
	// A directory mtime inside the racy window of its stat costs a listing,
	// never a record stat.
	cache.Load(dir)
	cache.Load(dir)
	if stats != 1 || opens != 1 {
		t.Fatalf("racy listing stat'ed known records: stats=%d opens=%d", stats, opens)
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
	var cache ObservedCache
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
	if cache.Load(dir) != nil {
		t.Fatal("removed directory served cached evidence")
	}
	if err := AppendObserved(dir, []ObservedRecord{observedCacheRecord("new", "restored")}); err != nil {
		t.Fatal(err)
	}
	if got := cache.Load(dir); got.models[""].Current.Model != "restored" {
		t.Fatal("directory recreation missed")
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
