package wsnote

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestLoadMissingFileReturnsEmptyMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")

	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if records == nil || len(records) != 0 {
		t.Fatalf("Load(missing) = %#v, want empty non-nil map", records)
	}
}

func TestLoadMalformedFileFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatalf("Load(malformed) = nil error, want failure")
	}
}

func TestWriteThenLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")

	err := Write(path, []Record{
		{Key: "a", Value: "alpha", Priority: 5, WrittenAt: "2026-08-01T00:00:00Z"},
		{Key: "b", Value: "beta", Priority: 1, WrittenAt: "2026-08-02T00:00:00Z"},
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Load after Write = %d records, want 2: %#v", len(records), records)
	}
	if got := records["a"]; got.Value != "alpha" || got.Priority != 5 {
		t.Fatalf("records[a] = %#v, want alpha/5", got)
	}
}

// TestWriteFullOverwriteUpdatesPriority verifies note.write's "full-overwrite
// (updates priority)" contract: writing the same key again replaces the
// whole record, including priority, rather than merging fields.
func TestWriteFullOverwriteUpdatesPriority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")

	if err := Write(path, []Record{{Key: "a", Value: "v1", Priority: 1, WrittenAt: "t1"}}); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if err := Write(path, []Record{{Key: "a", Value: "v2", Priority: 9, WrittenAt: "t2"}}); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := records["a"]
	if got.Value != "v2" || got.Priority != 9 || got.WrittenAt != "t2" {
		t.Fatalf("records[a] = %#v, want fully-overwritten v2/9/t2", got)
	}
}

func TestEraseRemovesKeyAndToleratesMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")

	if err := Write(path, []Record{{Key: "a", Value: "alpha", Priority: 1, WrittenAt: "t1"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := Erase(path, []string{"a", "does-not-exist"}); err != nil {
		t.Fatalf("Erase: %v", err)
	}

	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("Load after Erase = %#v, want empty", records)
	}
}

func TestWriteIsConcurrencySafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")

	const n = 20
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			key := "k" + string(rune('a'+i%26))
			errs <- Write(path, []Record{{Key: key, Value: "v", Priority: i, WrittenAt: "t"}})
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent Write: %v", err)
		}
	}

	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load after concurrent writes: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("Load after concurrent writes = empty, want survivors")
	}
}

func TestMachinePathIsSiblingOfGlobalConfig(t *testing.T) {
	home := t.TempDir()
	opts := wsconfig.Options{ConfigHome: home}

	got, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	want := filepath.Join(home, "notes.json")
	if got != want {
		t.Fatalf("MachinePath = %q, want %q", got, want)
	}
}

// TestWriteSetsVisibleTrueOnNewKey verifies Write's default-on-new-key half
// of the visible contract: a brand new key is always visible, regardless of
// whatever the caller's Record literal happened to carry (note.write has no
// wire-level way to set visible at all).
func TestWriteSetsVisibleTrueOnNewKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")

	if err := Write(path, []Record{{Key: "fresh", Value: "v", Priority: 1, WrittenAt: "t1"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !records["fresh"].Visible {
		t.Fatalf("Write(new key) Visible = false, want true")
	}
}

// TestWritePreservesVisibleOnExistingMutedKey is the ticket-mandated
// write-over-a-muted-key regression: mute a key via SetVisible, then Write a
// content-only update to the same key, and assert Visible stays false while
// Value/Priority/WrittenAt update as normal.
func TestWritePreservesVisibleOnExistingMutedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")

	if err := Write(path, []Record{{Key: "k", Value: "v1", Priority: 1, WrittenAt: "t1"}}); err != nil {
		t.Fatalf("Write initial: %v", err)
	}
	if err := SetVisible(path, []string{"k"}, false); err != nil {
		t.Fatalf("SetVisible(mute): %v", err)
	}

	if err := Write(path, []Record{{Key: "k", Value: "v2", Priority: 9, WrittenAt: "t2"}}); err != nil {
		t.Fatalf("Write over muted key: %v", err)
	}

	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := records["k"]
	if got.Visible {
		t.Fatalf("Write over a muted key set Visible = true, want the mute to survive (false)")
	}
	if got.Value != "v2" || got.Priority != 9 || got.WrittenAt != "t2" {
		t.Fatalf("Write over a muted key did not update content fields: %+v", got)
	}
}

// TestSetVisibleIdempotentAndLeavesWrittenAtUnchanged verifies muting an
// already-muted key is a no-op (idempotent set-state), and that SetVisible
// never touches WrittenAt regardless of how many times it is called.
func TestSetVisibleIdempotentAndLeavesWrittenAtUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")

	if err := Write(path, []Record{{Key: "k", Value: "v", Priority: 1, WrittenAt: "original-timestamp"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := SetVisible(path, []string{"k"}, false); err != nil {
		t.Fatalf("SetVisible(mute): %v", err)
	}
	if err := SetVisible(path, []string{"k"}, false); err != nil {
		t.Fatalf("SetVisible(mute again, idempotent): %v", err)
	}

	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := records["k"]
	if got.Visible {
		t.Fatalf("SetVisible(false) twice left Visible = true, want false")
	}
	if got.WrittenAt != "original-timestamp" {
		t.Fatalf("SetVisible restamped WrittenAt: got %q, want unchanged %q", got.WrittenAt, "original-timestamp")
	}
}

// TestSetVisibleMissingKeyIsNoop verifies SetVisible on a key with no
// backing record is a no-op, matching Erase's missing-key contract.
func TestSetVisibleMissingKeyIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")
	if err := Write(path, []Record{{Key: "a", Value: "v", Priority: 1, WrittenAt: "t"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := SetVisible(path, []string{"never-written"}, false); err != nil {
		t.Fatalf("SetVisible on missing key: %v", err)
	}
	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("SetVisible on missing key mutated the store: %#v", records)
	}
}

// TestSetVisibleUnmuteRestoresVisibility verifies the unmute direction: a
// muted key set back to true reads as visible again.
func TestSetVisibleUnmuteRestoresVisibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")
	if err := Write(path, []Record{{Key: "k", Value: "v", Priority: 1, WrittenAt: "t"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := SetVisible(path, []string{"k"}, false); err != nil {
		t.Fatalf("SetVisible(mute): %v", err)
	}
	if err := SetVisible(path, []string{"k"}, true); err != nil {
		t.Fatalf("SetVisible(unmute): %v", err)
	}
	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !records["k"].Visible {
		t.Fatalf("SetVisible(true) after mute left Visible = false, want true (unmuted)")
	}
}
