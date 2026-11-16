package wsdashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestWakeStampAndSnapshotUnchangedFilesAreNotOpened(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	index := NewIndex(IndexOptions{})
	opens := 0
	cache := &SnapshotCache{load: func(path string) (parsedSnapshot, error) { opens++; return loadSnapshot(path) }}
	ids := []string{"sessiona", "sessionb", "sessionc"}
	for _, id := range ids {
		index.sessions[id] = &indexEntry{session: Session{Registration: Registration{ReplyID: id, HeartbeatAt: now.UTC().Format(time.RFC3339)}}}
		if err := os.MkdirAll(SessionDir(dir, id), 0700); err != nil {
			t.Fatal(err)
		}
		writeSnapshot(t, dir, id, &wslive.Snapshot{Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000})
		if err := wslive.AppendObserved(SessionDir(dir, id), []wslive.ObservedRecord{{ModelObservation: wslive.ModelObservation{ID: "response", Model: "model", Source: "pi.assistant", At: 1}}}); err != nil {
			t.Fatal(err)
		}
		if cache.Snapshot(dir, id) == nil {
			t.Fatal("snapshot absent")
		}
	}
	reader := pageReader{dir: dir, index: index, snapshots: cache, now: func() time.Time { return now }}
	in := refreshInputs{ids: ids}
	first, _ := reader.sessionStamp(ids[0], in)
	firstIndex, _ := reader.indexStamp()
	if opens != len(ids) || cache.observed.Opens() != int64(len(ids)) {
		t.Fatalf("cold opens snapshot=%d observed=%d", opens, cache.observed.Opens())
	}
	for i := 0; i < 20; i++ {
		for _, id := range ids {
			cache.Snapshot(dir, id)
		}
		if next, _ := reader.sessionStamp(ids[0], in); next != first {
			t.Fatal("unchanged session stamp moved")
		}
		if next, _ := reader.indexStamp(); next != firstIndex {
			t.Fatal("unchanged index stamp moved")
		}
	}
	if opens != len(ids) || cache.observed.Opens() != int64(len(ids)) {
		t.Fatalf("unchanged wake reopened snapshot=%d observed=%d", opens-len(ids), cache.observed.Opens()-int64(len(ids)))
	}
	// A same-size/time atomic replacement changes adapter presence once the
	// index/backstop refreshes its evidence. Stamps themselves never reopen it.
	path := wslive.SnapshotPath(SessionDir(dir, ids[0]))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snap wslive.Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	snap.Adapter = "xx"
	raw, err = json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(raw)) != info.Size() {
		t.Fatal("replacement size differs")
	}
	// Keep a valid adapter but make it stale without changing any file stats.
	now = now.Add(2 * time.Minute)
	expired, _ := reader.sessionStamp(ids[0], in)
	expiredIndex, _ := reader.indexStamp()
	if expired == first || expiredIndex == firstIndex || opens != len(ids) {
		t.Fatal("clock expiry was lost or reopened a file")
	}
	now = now.Add(-2 * time.Minute)
	snap.UpdatedAt = now.Add(-time.Hour).UnixMilli()
	raw, err = json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(raw)) != info.Size() {
		t.Fatal("stale replacement must preserve size too")
	}
	tmp := path + ".replacement"
	if err := os.WriteFile(tmp, raw, info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmp, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	if next, _ := reader.sessionStamp(ids[0], in); next != first || opens != len(ids) {
		t.Fatal("stamp unexpectedly probed the replaced snapshot")
	}
	cache.snapshotWithoutObserved(dir, ids[0]) // the existing index/backstop read
	if next, _ := reader.sessionStamp(ids[0], in); next == first {
		t.Fatal("refreshed evidence did not move session stamp")
	}
	if next, _ := reader.indexStamp(); next == firstIndex {
		t.Fatal("replaced snapshot did not move index stamp")
	}
	if opens != len(ids)+1 || cache.observed.Opens() != int64(len(ids)) {
		t.Fatalf("replacement opens snapshot=%d observed=%d", opens, cache.observed.Opens())
	}
	// Forget discards both caches (without touching a hosted live copy).
	cache.Forget(dir, ids[0])
	cache.Snapshot(dir, ids[0])
	if opens != len(ids)+2 || cache.observed.Opens() != int64(len(ids)+1) {
		t.Fatal("Forget kept evidence or snapshot")
	}
	// A removed snapshot stops adapter presence and is not served from cache.
	if err := os.Remove(filepath.Clean(path)); err != nil {
		t.Fatal(err)
	}
	if cache.Snapshot(dir, ids[0]) != nil {
		t.Fatal("removed snapshot retained")
	}
}
