package wsnote

import (
	"os"
	"path/filepath"
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
