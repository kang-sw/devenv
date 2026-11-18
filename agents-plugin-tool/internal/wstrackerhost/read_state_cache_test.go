package wstrackerhost

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

func readStateHost(t *testing.T) (*Host, StoreEntry) {
	t.Helper()
	h, err := New(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	e := StoreEntry{ID: "team", Dir: filepath.Join(h.Root(), "team"), Member: "me", Nonce: "registration-1"}
	if err := os.MkdirAll(e.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.addEntry(e); err != nil {
		t.Fatal(err)
	}
	return h, e
}

// No parallel tests or watcher may run during these process-wide measurements.
func assertNoReadStateIO(t *testing.T, action func()) {
	t.Helper()
	before := wsstate.FileIOCounters()
	action()
	after := wsstate.FileIOCounters()
	for operation, count := range after {
		if delta := count - before[operation]; delta != 0 {
			t.Errorf("memory read %s = %d, want 0", operation, delta)
		}
	}
	t.Logf("memory reads: read_file=%d stat=%d total=%d",
		after["file_io_read_file"]-before["file_io_read_file"],
		after["file_io_stat"]-before["file_io_stat"],
		after["file_io_total"]-before["file_io_total"])
}

func TestTrackerReadStateCacheAvoidsUnchangedRecoveryReads(t *testing.T) {
	h, e := readStateHost(t)
	if err := writeJSON(filepath.Join(e.Dir, seenFile), seenDoc{Version: 1, Badge: []string{"old"}, Read: []string{}}); err != nil {
		t.Fatal(err)
	}
	// Do not age stores.json: even a just-written registry must be I/O-free.
	if _, err := h.Seen(e.ID); err != nil {
		t.Fatal(err)
	}
	// Execute the former recovery-read path as a supporting I/O baseline.
	baseline := wsstate.FileIOCounters()
	for range 5 {
		var doc registryDoc
		if err := readJSON(filepath.Join(h.Root(), registryFile), &doc); err != nil {
			t.Fatal(err)
		}
		if _, err := readSeen(e.Dir); err != nil {
			t.Fatal(err)
		}
	}
	baselineReads := wsstate.FileIOCounters()["file_io_read_file"] - baseline["file_io_read_file"]
	if baselineReads != 10 {
		t.Fatalf("former read path = %d read_file, want 10", baselineReads)
	}
	t.Logf("five former Seen read paths: read_file=%d", baselineReads)
	assertNoReadStateIO(t, func() {
		for range 5 {
			got, err := h.Seen(e.ID)
			if err != nil || !reflect.DeepEqual(got.Badge, []string{"old"}) {
				t.Fatalf("seen = %+v, %v", got, err)
			}
			// Callers may not corrupt the retained recovery state.
			got.Badge[0] = "caller mutation"
		}
	})
	// A recent external modification must not trigger the cache's conservative
	// two-second reread path either. Only an explicit/background refresh does.
	changed := e
	changed.Label = "external label"
	if err := writeJSON(filepath.Join(h.Root(), registryFile), registryDoc{Version: 1, Stores: []StoreEntry{changed}}); err != nil {
		t.Fatal(err)
	}
	assertNoReadStateIO(t, func() {
		for range 5 {
			if got, err := h.Seen(e.ID); err != nil || !reflect.DeepEqual(got.Badge, []string{"old"}) {
				t.Fatalf("recent registry modification: %+v, %v", got, err)
			}
		}
	})

	// A local seen write updates memory immediately, including while its file
	// is in the coarse-timestamp racy window. No recovery reread is necessary.
	if err := h.MarkSeen(e.ID, Seen{Read: []string{"new"}}); err != nil {
		t.Fatal(err)
	}
	want := Seen{Badge: []string{"old"}, Read: []string{"new"}}
	assertNoReadStateIO(t, func() {
		if got, err := h.Seen(e.ID); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("after write = %+v, %v", got, err)
		}
	})

	restarted, err := New(Options{Root: h.Root()})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restored, err := restarted.Seen(e.ID); err != nil || !reflect.DeepEqual(restored, want) {
		t.Fatalf("restart = %+v, %v; want %+v", restored, err, want)
	}
}

func TestTrackerReadStateCachePublishesLocalRegistryWrites(t *testing.T) {
	h, e := readStateHost(t)
	if err := h.MarkSeen(e.ID, Seen{Badge: []string{"old registration"}}); err != nil {
		t.Fatal(err)
	}
	check := func(want StoreEntry) {
		t.Helper()
		assertNoReadStateIO(t, func() {
			if got, err := h.memoryEntry(e.ID); err != nil || got != want {
				t.Fatalf("memory entry = %+v, %v; want %+v", got, err, want)
			}
			if got, err := h.Seen(e.ID); err != nil || !reflect.DeepEqual(got.Badge, []string{"old registration"}) {
				t.Fatalf("marks = %+v, %v", got, err)
			}
		})
	}
	check(e) // addEntry publishes without an intervening Stores/Entry call.
	entries, err := h.Stores()
	if err != nil {
		t.Fatal(err)
	}
	entries[0].Label = "caller mutation"
	check(e) // Stores must not hand out mutable snapshot storage.
	if _, err := h.Relabel(e.ID, "local label"); err != nil {
		t.Fatal(err)
	}
	e.Label = "local label"
	check(e)
	if _, err := h.SetHidden(e.ID, true); err != nil {
		t.Fatal(err)
	}
	e.Hidden = true
	check(e)
	at, err := h.removeEntry(e.ID, e.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	assertNoReadStateIO(t, func() {
		if _, err := h.memoryEntry(e.ID); !errors.Is(err, ErrNotRegistered) {
			t.Fatalf("removed memory entry: %v", err)
		}
		if _, err := h.Seen(e.ID); !errors.Is(err, ErrNotRegistered) {
			t.Fatalf("removed registration reused marks: %v", err)
		}
	})
	if err := h.restoreEntry(e, at); err != nil {
		t.Fatal(err)
	}
	check(e)

	// Reusing an id/directory with a new nonce must recover the new marks.
	if _, err := h.removeEntry(e.ID, e.Nonce); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(e.Dir, seenFile), seenDoc{Version: 1, Badge: []string{"new registration"}, Read: []string{}}); err != nil {
		t.Fatal(err)
	}
	e.Nonce = "registration-2"
	if err := h.addEntry(e); err != nil {
		t.Fatal(err)
	}
	assertNoReadStateIO(t, func() {
		if got, err := h.memoryEntry(e.ID); err != nil || got != e {
			t.Fatalf("replacement memory entry = %+v, %v", got, err)
		}
	})
	if got, err := h.Seen(e.ID); err != nil || !reflect.DeepEqual(got.Badge, []string{"new registration"}) {
		t.Fatalf("replacement recovery = %+v, %v", got, err)
	}
	assertNoReadStateIO(t, func() {
		if got, err := h.Seen(e.ID); err != nil || !reflect.DeepEqual(got.Badge, []string{"new registration"}) {
			t.Fatalf("warm replacement = %+v, %v", got, err)
		}
	})
}

func TestTrackerReadStateCacheStoreSnapshotOwnsOrderedEntries(t *testing.T) {
	h, e := readStateHost(t)
	second := StoreEntry{ID: "second", Dir: t.TempDir(), Nonce: "second-nonce"}
	if err := h.addEntry(second); err != nil {
		t.Fatal(err)
	}
	assertNoReadStateIO(t, func() {
		got, err := h.StoreSnapshot()
		if err != nil || !reflect.DeepEqual(got, []StoreEntry{e, second}) {
			t.Fatalf("ordered snapshot = %+v, %v", got, err)
		}
		got[0].Label = "caller mutation"
		got[0], got[1] = got[1], got[0]
		again, err := h.StoreSnapshot()
		if err != nil || !reflect.DeepEqual(again, []StoreEntry{e, second}) {
			t.Fatalf("caller changed snapshot = %+v, %v", again, err)
		}
	})
}

func TestTrackerReadStateCacheRetriesFailedRecovery(t *testing.T) {
	h, e := readStateHost(t)
	path := filepath.Join(e.Dir, seenFile)
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Seen(e.ID); err == nil {
		t.Fatal("corrupt recovery file was accepted")
	}
	if _, ok := h.seen[e.ID]; ok {
		t.Fatal("failed recovery was cached")
	}
	if err := writeJSON(path, seenDoc{Version: 1, Badge: []string{"repaired"}, Read: []string{}}); err != nil {
		t.Fatal(err)
	}
	if got, err := h.Seen(e.ID); err != nil || !reflect.DeepEqual(got.Badge, []string{"repaired"}) {
		t.Fatalf("recovery retry = %+v, %v", got, err)
	}
}

func TestTrackerReadStateCacheDoesNotPublishFailedSeenWrite(t *testing.T) {
	h, e := readStateHost(t)
	if err := h.MarkSeen(e.ID, Seen{Badge: []string{"persisted"}}); err != nil {
		t.Fatal(err)
	}
	aside := e.Dir + "-aside"
	if err := os.Rename(e.Dir, aside); err != nil {
		t.Fatal(err)
	}
	if err := h.MarkSeen(e.ID, Seen{Badge: []string{"failed"}}); err == nil {
		t.Fatal("write to a removed directory succeeded")
	}
	if err := os.Rename(aside, e.Dir); err != nil {
		t.Fatal(err)
	}
	if got, err := h.Seen(e.ID); err != nil || !reflect.DeepEqual(got.Badge, []string{"persisted"}) {
		t.Fatalf("failed write entered memory: %+v, %v", got, err)
	}
}

func TestTrackerReadStateCacheRecoversRegistryAtStartup(t *testing.T) {
	root := t.TempDir()
	e := StoreEntry{ID: "existing", Dir: filepath.Join(root, "existing"), Nonce: "existing-nonce"}
	if err := os.MkdirAll(e.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, registryFile), registryDoc{Version: 1, Stores: []StoreEntry{e}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(e.Dir, seenFile), seenDoc{Version: 1, Badge: []string{"recovered"}}); err != nil {
		t.Fatal(err)
	}
	h, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	// No Stores/Entry call may prime the registration after New.
	assertNoReadStateIO(t, func() {
		if got, err := h.memoryEntry(e.ID); err != nil || got != e {
			t.Fatalf("startup entry = %+v, %v", got, err)
		}
	})
	if got, err := h.Seen(e.ID); err != nil || !reflect.DeepEqual(got.Badge, []string{"recovered"}) {
		t.Fatalf("startup marks = %+v, %v", got, err)
	}
}

func TestTrackerReadStateCacheDoesNotWaitForRegistryIO(t *testing.T) {
	h, e := readStateHost(t)
	if err := h.MarkSeen(e.ID, Seen{Badge: []string{"warm"}}); err != nil {
		t.Fatal(err)
	}
	// Model a slow background refresh/write. Only registryMu may be taken
	// by interactive lookups, not this mutex held across disk operations.
	h.registryLoadMu.Lock()
	defer h.registryLoadMu.Unlock()
	done := make(chan error, 1)
	assertNoReadStateIO(t, func() {
		go func() {
			if got, err := h.memoryEntry(e.ID); err != nil || got != e {
				done <- errors.New("memoryEntry failed during background I/O")
				return
			}
			got, err := h.Seen(e.ID)
			if err == nil && !reflect.DeepEqual(got.Badge, []string{"warm"}) {
				err = errors.New("Seen returned wrong marks during background I/O")
			}
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("memory reads waited for registryLoadMu")
		}
	})
}

func TestTrackerReadStateCacheFailedRegistryWriteDoesNotPublish(t *testing.T) {
	h, e := readStateHost(t)
	if _, err := h.Seen(e.ID); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h.Root(), registryFile)
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	// An atomic replacement of a nonempty directory fails even as root.
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "blocker"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	changed := e
	changed.Label, changed.Hidden, changed.Nonce = "failed", true, "failed-nonce"
	for _, stores := range [][]StoreEntry{{changed}, {}, {e, {ID: "failed-add"}}} {
		if err := h.writeRegistry(registryDoc{Version: 1, Stores: stores}); err == nil {
			t.Fatal("registry write unexpectedly succeeded")
		}
		assertNoReadStateIO(t, func() {
			if got, err := h.memoryEntry(e.ID); err != nil || got != e {
				t.Fatalf("failed write published entry = %+v, %v", got, err)
			}
			if _, err := h.memoryEntry("failed-add"); !errors.Is(err, ErrNotRegistered) {
				t.Fatalf("failed addition published: %v", err)
			}
			if _, err := h.Seen(e.ID); err != nil {
				t.Fatalf("failed write invalidated warm Seen: %v", err)
			}
		})
	}
}

func TestTrackerReadStateCacheExternalRegistryChangesAtWatchNotification(t *testing.T) {
	h, e := readStateHost(t)
	if err := h.MarkSeen(e.ID, Seen{Badge: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	other, err := New(Options{Root: h.Root()})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	// Before the watch boundary, Seen deliberately retains the old snapshot.
	if _, err := other.Relabel(e.ID, "external"); err != nil {
		t.Fatal(err)
	}
	assertNoReadStateIO(t, func() {
		if got, err := h.memoryEntry(e.ID); err != nil || got != e {
			t.Fatalf("external edit refreshed without watch: %+v, %v", got, err)
		}
		if _, err := h.Seen(e.ID); err != nil {
			t.Fatal(err)
		}
	})
	// Establish the initial stamp before starting the sequence of changes.
	e.Label = "external"
	type notification struct {
		entry    StoreEntry
		entryErr error
		marks    Seen
		seenErr  error
	}
	updates := make(chan notification, 16)
	stop := h.StartWatch(10*time.Millisecond, func() {
		entry, entryErr := h.memoryEntry(e.ID)
		marks, seenErr := h.Seen(e.ID)
		updates <- notification{entry, entryErr, marks, seenErr}
	})
	defer stop()
	await := func(want StoreEntry, absent bool, badge string) {
		t.Helper()
		select {
		case got := <-updates:
			if absent {
				if !errors.Is(got.entryErr, ErrNotRegistered) || !errors.Is(got.seenErr, ErrNotRegistered) {
					t.Fatalf("removal notification preceded publication: %+v", got)
				}
			} else if got.entryErr != nil || got.entry != want || got.seenErr != nil || !reflect.DeepEqual(got.marks.Badge, []string{badge}) {
				t.Fatalf("notification = %+v; want entry %+v, badge %q", got, want, badge)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no StartWatch notification for external registry change")
		}
	}
	if _, err := other.Relabel(e.ID, "watched relabel"); err != nil {
		t.Fatal(err)
	}
	e.Label = "watched relabel"
	await(e, false, "old")
	if _, err := other.SetHidden(e.ID, true); err != nil {
		t.Fatal(err)
	}
	e.Hidden = true
	await(e, false, "old")
	at, err := other.removeEntry(e.ID, e.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	await(StoreEntry{}, true, "")
	if err := other.restoreEntry(e, at); err != nil {
		t.Fatal(err)
	}
	await(e, false, "old")
	if _, err := other.removeEntry(e.ID, e.Nonce); err != nil {
		t.Fatal(err)
	}
	await(StoreEntry{}, true, "")
	if err := writeJSON(filepath.Join(e.Dir, seenFile), seenDoc{Version: 1, Badge: []string{"replacement"}}); err != nil {
		t.Fatal(err)
	}
	e.Nonce = "external-replacement"
	if err := other.addEntry(e); err != nil {
		t.Fatal(err)
	}
	await(e, false, "replacement")
}

func TestTrackerReadStateCacheMutationsRevalidateExternalRemoval(t *testing.T) {
	h, e := readStateHost(t)
	if err := h.MarkSeen(e.ID, Seen{Badge: []string{"persisted"}}); err != nil {
		t.Fatal(err)
	}
	other, err := New(Options{Root: h.Root()})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.removeEntry(e.ID, e.Nonce); err != nil {
		t.Fatal(err)
	}
	// No watch has run: the read snapshot still exists, but writes must not
	// trust it. Keep the directory present to detect writes into an orphan.
	assertNoReadStateIO(t, func() {
		if got, err := h.Seen(e.ID); err != nil || !reflect.DeepEqual(got.Badge, []string{"persisted"}) {
			t.Fatalf("pre-refresh snapshot = %+v, %v", got, err)
		}
	})
	if err := h.MarkSeen(e.ID, Seen{Badge: []string{"orphan write"}}); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("MarkSeen did not revalidate removal: %v", err)
	}
	if _, err := h.Entry(e.ID); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("Entry did not revalidate removal: %v", err)
	}
	if _, err := h.Relabel(e.ID, "orphan"); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("Relabel did not revalidate removal: %v", err)
	}
	if _, err := h.SetHidden(e.ID, true); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("SetHidden did not revalidate removal: %v", err)
	}
	if got, err := readSeen(e.Dir); err != nil || !reflect.DeepEqual(got.Badge, []string{"persisted"}) {
		t.Fatalf("external removal allowed orphan write: %+v, %v", got, err)
	}
}
