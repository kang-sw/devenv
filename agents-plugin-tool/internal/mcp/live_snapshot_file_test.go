package mcp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLiveSnapshotReplacementRetriesSharingViolations(t *testing.T) {
	original, retryable, backoff := liveSnapshotRename, liveSnapshotRenameRetryable, liveSnapshotRenameBackoff
	t.Cleanup(func() {
		liveSnapshotRename, liveSnapshotRenameRetryable, liveSnapshotRenameBackoff = original, retryable, backoff
	})
	busy := errors.New("sharing violation")
	liveSnapshotRenameRetryable = func(err error) bool { return errors.Is(err, busy) }
	liveSnapshotRenameBackoff = 0
	path := filepath.Join(t.TempDir(), "snapshot.json")
	attempts := 0
	liveSnapshotRename = func(from, to string) error {
		attempts++
		if attempts < 3 {
			return busy
		}
		return os.Rename(from, to)
	}
	if err := writeLiveSnapshotFile(path, []byte("complete")); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || string(readFileForTest(t, path)) != "complete" {
		t.Fatal("snapshot retry lost the write")
	}
	liveSnapshotRename = func(from, to string) error { return busy }
	if err := writeLiveSnapshotFile(path, []byte("replacement")); !errors.Is(err, busy) {
		t.Fatal("permanent sharing violation not reported", err)
	}
	if string(readFileForTest(t, path)) != "complete" {
		t.Fatal("failed replacement damaged snapshot")
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(files) != 1 {
		t.Fatal("failed replacement leaked temporary files", files, err)
	}
}
