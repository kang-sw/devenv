package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFolderCacheRevisionIdentity(t *testing.T) {
	f := FolderStore{Dir: filepath.Join(t.TempDir(), "board")}
	var a, b FolderCache
	if state, rev, err := a.OpenRevision(f, nil); err != nil || state != nil || rev != 0 {
		t.Fatalf("absent: state=%v revision=%d err=%v", state, rev, err)
	}
	seen := map[uint64]bool{}
	fresh := func(cache *FolderCache, legacy *State) (*State, uint64) {
		t.Helper()
		state, rev, err := cache.OpenRevision(f, legacy)
		if err != nil || state == nil || rev == 0 || seen[rev] {
			t.Fatalf("expected fresh nonzero revision: %d, err=%v", rev, err)
		}
		seen[rev] = true
		return state, rev
	}
	// Conversion is uncached; the next read must acquire a different identity.
	fresh(&a, &State{Seq: 5, Topics: []TopicDoc{{Name: "topic"}}})
	first, rev := fresh(&a, nil)
	reads := countFolderReads(t)
	for i := 0; i < 3; i++ {
		state, got, err := a.OpenRevision(f, nil)
		if err != nil || got != rev || state == first {
			t.Fatalf("hit must return a separate header with revision %d: %d %v", rev, got, err)
		}
	}
	if *reads != 0 {
		t.Fatal("revision lookup added authoritative file reads on a hit")
	}
	fresh(&b, nil)
	fresh(nil, nil)
	fresh(nil, nil)
	a.Forget(f.Dir)
	fresh(&a, nil)
	if err := f.Update(nil, func(s *State) error { s.Labels = map[string]string{"topic": "new"}; return nil }); err != nil {
		t.Fatal(err)
	}
	fresh(&a, nil)
	// Evicting the slot must not restart an instance-local revision counter.
	root := t.TempDir()
	for i := 0; i < 512; i++ {
		other := FolderStore{Dir: filepath.Join(root, fmt.Sprint(i))}
		if _, err := other.Open(&State{}); err != nil {
			t.Fatal(err)
		}
		if _, rev, err := a.OpenRevision(other, nil); err != nil || rev == 0 || seen[rev] {
			t.Fatalf("eviction fill collided: revision=%d err=%v", rev, err)
		} else {
			seen[rev] = true
		}
	}
	if a.entries[f.Dir] != nil {
		t.Fatal("fixture did not evict the original slot")
	}
	fresh(&a, nil)
}

func TestFolderCacheRevisionRootStampedChangesAtSameSeq(t *testing.T) {
	f := FolderStore{Dir: t.TempDir()}
	seed := &State{Seq: 7, EntrySeq: 2, PanelCounter: 1,
		Topics: []TopicDoc{{Name: "topic", Entries: []Entry{
			{ID: "e1", Kind: EntryAgent, Text: "log-before", At: "2026-11-15T00:00:00Z"},
			{ID: "e2", Kind: EntryPanel, PanelID: "p1", Text: "panel-before", At: "2026-11-15T00:00:00Z"},
		}}}, Labels: map[string]string{"topic": "label-before"}}
	if _, err := f.Open(seed); err != nil {
		t.Fatal(err)
	}
	var cache FolderCache
	_, previous, err := cache.OpenRevision(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct{ path, before, after string }{
		{"t1/meta.json", "label-before", "label-after!"},
		{"t1/log.jsonl", "log-before", "log-after!"},
		{"t1/panels/e2.json", "panel-before", "panel-after!"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			path := filepath.Join(f.Dir, filepath.FromSlash(tc.path))
			before := folderRead(t, path)
			after := bytes.ReplaceAll(before, []byte(tc.before), []byte(tc.after))
			if bytes.Equal(before, after) || len(before) != len(after) {
				t.Fatal("fixture must change content without changing committed log length")
			}
			if err := os.WriteFile(path, after, 0600); err != nil {
				t.Fatal(err)
			}
			at := time.Now().Add(time.Duration(i+1) * time.Second)
			if err := os.Chtimes(path, at, at); err != nil {
				t.Fatal(err)
			}
			// Leaf-only fault injection is not a folder-store commit. Signal
			// the completed external change by atomically replacing the root
			// receipt, without relying on the logical State.Seq or generation.
			folderReplaceRootReceipt(t, f)
			if filepath.Base(filepath.Dir(path)) == "panels" {
				// A committed panel is immutable, so the topic memo keeps its
				// decoded body; a leaf edit to one must Forget the board.
				cache.Forget(f.Dir)
			}
			state, rev, err := cache.OpenRevision(f, nil)
			if err != nil || state == nil || rev <= previous || state.Seq != seed.Seq {
				t.Fatalf("changed authority at unchanged Seq: revision=%d previous=%d err=%v", rev, previous, err)
			}
			raw, _ := json.Marshal(state)
			if !bytes.Contains(raw, []byte(tc.after)) {
				t.Fatalf("revision and state snapshot disagree: missing %q", tc.after)
			}
			if _, hit, err := cache.OpenRevision(f, nil); err != nil || hit != rev {
				t.Fatalf("new read was not cached: %d %v", hit, err)
			}
			previous = rev
		})
	}
}

func TestFolderCacheRevisionConcurrentReaders(t *testing.T) {
	f := FolderStore{Dir: t.TempDir()}
	if _, err := f.Open(&State{Seq: 4}); err != nil {
		t.Fatal(err)
	}
	var cache FolderCache
	var wg sync.WaitGroup
	revisions := make(chan uint64, 16)
	for i := 0; i < cap(revisions); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state, rev, err := cache.OpenRevision(f, nil)
			if err != nil || state == nil || state.Seq != 4 || rev == 0 {
				t.Errorf("concurrent read: state=%v revision=%d err=%v", state, rev, err)
			}
			revisions <- rev
		}()
	}
	wg.Wait()
	close(revisions)
	var want uint64
	for rev := range revisions {
		if want != 0 && rev != want {
			t.Fatalf("same immutable read had revisions %d and %d", want, rev)
		}
		want = rev
	}
}
