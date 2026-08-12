package wsnote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepoKeyFilenameDeterministic verifies repoKeyFilename is a pure
// function of the key: same key -> same string across repeated calls, with
// no host/time dependence, so RepoErase can recompute the identical filename
// RepoWrite used.
func TestRepoKeyFilenameDeterministic(t *testing.T) {
	first := repoKeyFilename("ticket.260810")
	second := repoKeyFilename("ticket.260810")
	if first != second {
		t.Fatalf("repoKeyFilename not deterministic: %q != %q", first, second)
	}
	if first != "7469636b65742e323630383130.json" {
		t.Fatalf("repoKeyFilename(%q) = %q, want hex-encoded key + .json suffix", "ticket.260810", first)
	}
}

// TestRepoWriteLoadEraseRoundTrip verifies the core repo-layer storage
// contract: write -> load returns the record keyed by its original Key (not
// the encoded filename), and erase removes the file from disk.
func TestRepoWriteLoadEraseRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws-notes")

	if err := RepoWrite(dir, []Record{
		{Key: "roundtrip.key", Value: "hello", Priority: 2, WrittenAt: "2026-08-01T00:00:00Z"},
	}); err != nil {
		t.Fatalf("RepoWrite: %v", err)
	}

	filePath := filepath.Join(dir, repoKeyFilename("roundtrip.key"))
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected tracked file at %s: %v", filePath, err)
	}

	loaded, err := RepoLoad(dir)
	if err != nil {
		t.Fatalf("RepoLoad: %v", err)
	}
	rec, ok := loaded["roundtrip.key"]
	if !ok {
		t.Fatalf("RepoLoad = %v, want key %q present (keyed by Record.Key, not filename)", loaded, "roundtrip.key")
	}
	if rec.Value != "hello" || rec.Priority != 2 {
		t.Fatalf("RepoLoad record = %+v, want Value=hello Priority=2", rec)
	}

	if err := RepoErase(dir, []string{"roundtrip.key"}); err != nil {
		t.Fatalf("RepoErase: %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected file removed after RepoErase, stat err = %v", err)
	}

	afterErase, err := RepoLoad(dir)
	if err != nil {
		t.Fatalf("RepoLoad after erase: %v", err)
	}
	if _, ok := afterErase["roundtrip.key"]; ok {
		t.Fatalf("RepoLoad after erase still returned %q: %v", "roundtrip.key", afterErase)
	}
}

// TestRepoWriteLeavesNoLockOrTempArtifactInTrackedDir verifies the fix for
// the reviewed defect: writeRepoRecordFile's per-key flock must NOT live
// beside the target file inside the tracked dir (that would leave a
// "<hex>.json.lock" sidecar that `git status`/`git add ai-docs/ws-notes/`
// would pick up and commit as orphaned litter, and that RepoErase would
// never clean up). After RepoWrite, the tracked dir must contain exactly the
// intended ".json" file(s) and nothing else; after RepoErase, the dir must
// be completely empty — no leftover ".lock" or "*.tmp" residue either way.
func TestRepoWriteLeavesNoLockOrTempArtifactInTrackedDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws-notes")

	if err := RepoWrite(dir, []Record{
		{Key: "clean.tracking", Value: "hello", Priority: 1, WrittenAt: "2026-08-01T00:00:00Z"},
	}); err != nil {
		t.Fatalf("RepoWrite: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	if len(entries) != 1 {
		t.Fatalf("tracked dir has %d entries after RepoWrite, want exactly 1 (.json only): %v", len(entries), entries)
	}
	if got := entries[0].Name(); !strings.HasSuffix(got, ".json") {
		t.Fatalf("tracked dir entry %q is not a .json file — lock/temp artifact leaked into the tracked tree", got)
	}

	if err := RepoErase(dir, []string{"clean.tracking"}); err != nil {
		t.Fatalf("RepoErase: %v", err)
	}
	afterErase, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) after erase: %v", dir, err)
	}
	if len(afterErase) != 0 {
		t.Fatalf("tracked dir has %d leftover entries after RepoErase, want 0 (no orphaned lock/temp artifact): %v", len(afterErase), afterErase)
	}
}

// TestRepoEraseMissingKeyIsNoop verifies erasing a key with no backing file
// is a no-op, matching Erase's contract.
func TestRepoEraseMissingKeyIsNoop(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws-notes")
	if err := RepoErase(dir, []string{"never.written"}); err != nil {
		t.Fatalf("RepoErase on missing key = %v, want nil (no-op)", err)
	}
}

// TestRepoLoadMissingDirReturnsEmptyNonNilMap verifies RepoLoad on a
// directory that does not exist yet returns an empty, non-nil map, matching
// Load's "no file yet" contract.
func TestRepoLoadMissingDirReturnsEmptyNonNilMap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	got, err := RepoLoad(dir)
	if err != nil {
		t.Fatalf("RepoLoad on missing dir: %v", err)
	}
	if got == nil {
		t.Fatalf("RepoLoad on missing dir returned nil map, want empty non-nil map")
	}
	if len(got) != 0 {
		t.Fatalf("RepoLoad on missing dir = %v, want empty map", got)
	}
}

// TestRepoWriteSlashAndDottedKeysStayFlat verifies keys containing '/' and
// '.' cannot nest a subdirectory or otherwise escape the repo dir: every
// written file lands directly under dir as a flat hex-named file.
func TestRepoWriteSlashAndDottedKeysStayFlat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws-notes")

	slashKey := "a/b/c"
	dottedKey := "../escape.attempt"
	if err := RepoWrite(dir, []Record{
		{Key: slashKey, Value: "slash", Priority: 0, WrittenAt: "t"},
		{Key: dottedKey, Value: "dotted", Priority: 0, WrittenAt: "t"},
	}); err != nil {
		t.Fatalf("RepoWrite: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	var jsonFiles int
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("RepoWrite created a subdirectory entry %q under %s, want flat files only", entry.Name(), dir)
		}
		if filepath.Ext(entry.Name()) == ".json" {
			jsonFiles++
		}
	}
	if jsonFiles != 2 {
		t.Fatalf("RepoWrite wrote %d .json files directly under %s, want 2", jsonFiles, dir)
	}

	loaded, err := RepoLoad(dir)
	if err != nil {
		t.Fatalf("RepoLoad: %v", err)
	}
	if _, ok := loaded[slashKey]; !ok {
		t.Fatalf("RepoLoad missing slash-bearing key %q: %v", slashKey, loaded)
	}
	if _, ok := loaded[dottedKey]; !ok {
		t.Fatalf("RepoLoad missing dotted key %q: %v", dottedKey, loaded)
	}

	// The slash-bearing key must not have created a directory entry named
	// after its prefix (e.g. "a/") anywhere under dir's parent.
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatalf("slash-bearing key created a nested path entry: stat err = %v", err)
	}
}

// TestRepoWriteSetsVisibleTrueOnNewKey is the repo-layer counterpart of
// TestWriteSetsVisibleTrueOnNewKey: a brand new key is always visible.
func TestRepoWriteSetsVisibleTrueOnNewKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws-notes")

	if err := RepoWrite(dir, []Record{{Key: "fresh", Value: "v", Priority: 1, WrittenAt: "t1"}}); err != nil {
		t.Fatalf("RepoWrite: %v", err)
	}
	loaded, err := RepoLoad(dir)
	if err != nil {
		t.Fatalf("RepoLoad: %v", err)
	}
	if !loaded["fresh"].Visible {
		t.Fatalf("RepoWrite(new key) Visible = false, want true")
	}
}

// TestRepoWritePreservesVisibleOnExistingMutedKey is the repo-layer
// counterpart of the ticket-mandated write-over-a-muted-key regression.
func TestRepoWritePreservesVisibleOnExistingMutedKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws-notes")

	if err := RepoWrite(dir, []Record{{Key: "k", Value: "v1", Priority: 1, WrittenAt: "t1"}}); err != nil {
		t.Fatalf("RepoWrite initial: %v", err)
	}
	if err := RepoSetVisible(dir, []string{"k"}, false); err != nil {
		t.Fatalf("RepoSetVisible(mute): %v", err)
	}
	if err := RepoWrite(dir, []Record{{Key: "k", Value: "v2", Priority: 9, WrittenAt: "t2"}}); err != nil {
		t.Fatalf("RepoWrite over muted key: %v", err)
	}

	loaded, err := RepoLoad(dir)
	if err != nil {
		t.Fatalf("RepoLoad: %v", err)
	}
	got := loaded["k"]
	if got.Visible {
		t.Fatalf("RepoWrite over a muted key set Visible = true, want the mute to survive (false)")
	}
	if got.Value != "v2" || got.Priority != 9 || got.WrittenAt != "t2" {
		t.Fatalf("RepoWrite over a muted key did not update content fields: %+v", got)
	}
}

// TestRepoSetVisibleIdempotentAndLeavesWrittenAtUnchanged is the repo-layer
// counterpart of the idempotency/no-restamp regression.
func TestRepoSetVisibleIdempotentAndLeavesWrittenAtUnchanged(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws-notes")

	if err := RepoWrite(dir, []Record{{Key: "k", Value: "v", Priority: 1, WrittenAt: "original-timestamp"}}); err != nil {
		t.Fatalf("RepoWrite: %v", err)
	}
	if err := RepoSetVisible(dir, []string{"k"}, false); err != nil {
		t.Fatalf("RepoSetVisible(mute): %v", err)
	}
	if err := RepoSetVisible(dir, []string{"k"}, false); err != nil {
		t.Fatalf("RepoSetVisible(mute again, idempotent): %v", err)
	}

	loaded, err := RepoLoad(dir)
	if err != nil {
		t.Fatalf("RepoLoad: %v", err)
	}
	got := loaded["k"]
	if got.Visible {
		t.Fatalf("RepoSetVisible(false) twice left Visible = true, want false")
	}
	if got.WrittenAt != "original-timestamp" {
		t.Fatalf("RepoSetVisible restamped WrittenAt: got %q, want unchanged %q", got.WrittenAt, "original-timestamp")
	}
}

// TestRepoSetVisibleMissingKeyIsNoop is the repo-layer counterpart of the
// missing-key no-op regression.
func TestRepoSetVisibleMissingKeyIsNoop(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws-notes")
	if err := RepoWrite(dir, []Record{{Key: "a", Value: "v", Priority: 1, WrittenAt: "t"}}); err != nil {
		t.Fatalf("RepoWrite: %v", err)
	}
	if err := RepoSetVisible(dir, []string{"never-written"}, false); err != nil {
		t.Fatalf("RepoSetVisible on missing key: %v", err)
	}
	loaded, err := RepoLoad(dir)
	if err != nil {
		t.Fatalf("RepoLoad: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("RepoSetVisible on missing key mutated the store: %#v", loaded)
	}
}
