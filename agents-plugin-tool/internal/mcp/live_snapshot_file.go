package mcp

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// Snapshot replacement retries brief Windows sharing violations.
var (
	liveSnapshotRename          = os.Rename
	liveSnapshotRenameRetryable = renameSharingViolation
	liveSnapshotRenameAttempts  = 5
	liveSnapshotRenameBackoff   = 40 * time.Millisecond
)

// writeLiveSnapshotFile replaces a recovery snapshot atomically so readers
// never observe a torn file.
func writeLiveSnapshotFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// CreateTemp makes the file 0600; a host reading the snapshot may
	// run as another user.
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := renameWithRetry(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// renameWithRetry retries a rename the platform refused because another
// process briefly holds the target open, before giving up.
func renameWithRetry(from, to string) error {
	var err error
	for attempt := 0; attempt < liveSnapshotRenameAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(liveSnapshotRenameBackoff)
		}
		err = liveSnapshotRename(from, to)
		if err == nil || !liveSnapshotRenameRetryable(err) {
			return err
		}
	}
	return err
}

// renameSharingViolation reports a Windows rename refused because the target
// is open elsewhere: ERROR_SHARING_VIOLATION (32), or ERROR_ACCESS_DENIED (5),
// which MoveFileEx returns for a target opened without delete sharing.
func renameSharingViolation(err error) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == 32 || errno == 5
	}
	return false
}
