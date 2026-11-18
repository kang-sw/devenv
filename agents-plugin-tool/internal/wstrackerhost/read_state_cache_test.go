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

func settleRegistry(t *testing.T, h *Host) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	path := filepath.Join(h.Root(), registryFile)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Stores(); err != nil {
		t.Fatal(err)
	}
}

func TestTrackerReadStateCacheAvoidsUnchangedRecoveryReads(t *testing.T) {
	h, e := readStateHost(t)
	if err := writeJSON(filepath.Join(e.Dir, seenFile), seenDoc{Version: 1, Badge: []string{"old"}, Read: []string{}}); err != nil {
		t.Fatal(err)
	}
	settleRegistry(t, h)
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
	before := wsstate.FileIOCounters()
	for range 5 {
		got, err := h.Seen(e.ID)
		if err != nil || !reflect.DeepEqual(got.Badge, []string{"old"}) {
			t.Fatalf("seen = %+v, %v", got, err)
		}
		// Callers may not corrupt the retained recovery state.
		got.Badge[0] = "caller mutation"
	}
	after := wsstate.FileIOCounters()
	reads, stats := after["file_io_read_file"]-before["file_io_read_file"], after["file_io_stat"]-before["file_io_stat"]
	if reads != 0 || stats != 5 {
		t.Fatalf("five warm Seen calls: read_file=%d stat=%d, want 0 and 5", reads, stats)
	}
	t.Logf("five warm Seen calls: read_file=%d stat=%d (registration validation only; ui-seen is memory-owned)", reads, stats)

	// A local seen write updates memory immediately, including while its file
	// is in the coarse-timestamp racy window. No recovery reread is necessary.
	if err := h.MarkSeen(e.ID, Seen{Read: []string{"new"}}); err != nil {
		t.Fatal(err)
	}
	before = wsstate.FileIOCounters()
	got, err := h.Seen(e.ID)
	after = wsstate.FileIOCounters()
	if err != nil || !reflect.DeepEqual(got, Seen{Badge: []string{"old"}, Read: []string{"new"}}) {
		t.Fatalf("after write = %+v, %v", got, err)
	}
	if reads := after["file_io_read_file"] - before["file_io_read_file"]; reads != 0 {
		t.Fatalf("Seen reread after its own write: %d", reads)
	}

	restarted, err := New(Options{Root: h.Root()})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restored, err := restarted.Seen(e.ID); err != nil || !reflect.DeepEqual(restored, got) {
		t.Fatalf("restart = %+v, %v; want %+v", restored, err, got)
	}
}

func TestTrackerReadStateCacheTracksRegistrationReplacement(t *testing.T) {
	h, e := readStateHost(t)
	if err := h.MarkSeen(e.ID, Seen{Badge: []string{"old registration"}}); err != nil {
		t.Fatal(err)
	}
	settleRegistry(t, h)
	entries, err := h.Stores()
	if err != nil {
		t.Fatal(err)
	}
	entries[0].Label = "caller mutation"
	if got, err := h.Entry(e.ID); err != nil || got.Label != "" {
		t.Fatalf("caller mutated registry cache: %+v, %v", got, err)
	}

	other, err := New(Options{Root: h.Root()})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Relabel(e.ID, "external label"); err != nil {
		t.Fatal(err)
	}
	if got, err := h.Entry(e.ID); err != nil || got.Label != "external label" {
		t.Fatalf("external relabel = %+v, %v", got, err)
	}
	if _, err := other.SetHidden(e.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, err := h.Entry(e.ID); err != nil || !got.Hidden {
		t.Fatalf("external hidden = %+v, %v", got, err)
	}
	if _, err := other.removeEntry(e.ID, e.Nonce); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Seen(e.ID); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("removed registration reused marks: %v", err)
	}

	// Reusing an id/directory with a new nonce must reload the new clone's
	// recovery file, not reuse the previous registration's cached marks.
	if err := writeJSON(filepath.Join(e.Dir, seenFile), seenDoc{Version: 1, Badge: []string{"new registration"}, Read: []string{}}); err != nil {
		t.Fatal(err)
	}
	e.Nonce = "registration-2"
	if err := other.addEntry(e); err != nil {
		t.Fatal(err)
	}
	if got, err := h.Seen(e.ID); err != nil || !reflect.DeepEqual(got.Badge, []string{"new registration"}) {
		t.Fatalf("replacement = %+v, %v", got, err)
	}
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
