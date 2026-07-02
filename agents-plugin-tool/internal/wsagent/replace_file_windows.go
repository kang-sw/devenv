//go:build windows

package wsagent

import (
	"time"

	"golang.org/x/sys/windows"
)

// atomicReplaceFile replaces path with tmp using MoveFileEx(REPLACE_EXISTING),
// which is atomic on NTFS. When a concurrent reader (AV scanner, dashboard)
// holds the destination and the system returns ERROR_SHARING_VIOLATION, we
// retry a small number of times with brief exponential back-off before giving
// up.
func atomicReplaceFile(tmp, path string) error {
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	const maxAttempts = 5
	backoff := 10 * time.Millisecond
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil {
			return nil
		}
		// Only retry on sharing violations (transient AV/dashboard hold);
		// all other errors are fatal immediately.
		if !isErrorSharingViolation(err) {
			return err
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	return err
}

// isErrorSharingViolation reports whether err is a Windows sharing-violation
// error (the transient AV/dashboard hold case).
func isErrorSharingViolation(err error) bool {
	if err == nil {
		return false
	}
	errno, ok := err.(windows.Errno)
	return ok && errno == windows.ERROR_SHARING_VIOLATION
}
