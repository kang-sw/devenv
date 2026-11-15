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
	opens := 0
	cache := ObservedCache{open: func(path string) (*os.File, error) { opens++; return os.Open(path) }}
	first := cache.Load(dir)
	if opens != 1 {
		t.Fatalf("cold opens=%d", opens)
	}
	for i := 0; i < 20; i++ {
		if cache.Load(dir) != first {
			t.Fatal("unchanged projection rebuilt")
		}
	}
	if opens != 1 {
		t.Fatalf("unchanged files opened %d times", opens-1)
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
	changed := cache.Load(dir)
	if opens != 2 || changed == first || changed.models[""].Current.Model != "second-longer" {
		t.Fatalf("child edit missed: opens=%d", opens)
	}
	// Atomic replacement with identical size, mode and timestamp must reload.
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
	if got := cache.Load(dir); opens != 3 || got.models[""].Current.Model != "third-longerr" {
		t.Fatalf("replacement missed: opens=%d", opens)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := cache.Load(dir); len(got.models) != 0 || opens != 3 {
		t.Fatal("deleted evidence retained or opened")
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
