//go:build windows

package wsstate

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// AtomicReplaceFile replaces path with tmp using MoveFileEx(REPLACE_EXISTING),
// which is atomic on NTFS. The replace fails while another handle holds path
// without FILE_SHARE_DELETE, which every plain Go open (os.ReadFile) is: the
// dashboard or another ws-mcp process reading the file, or an AV scanner
// inspecting the previous write. MoveFileEx reports that hold as
// ERROR_ACCESS_DENIED (an open target) or ERROR_SHARING_VIOLATION. The holds
// are brief, so both are retried with back-off for about two seconds before
// giving up; any other error is returned at once.
func AtomicReplaceFile(tmp, path string) error {
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return retryTransient(func() error {
		return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
	})
}

// ReadFile reads path, retrying the brief holds AtomicReplaceFile retries:
// an open that meets another process's replace or removal of path in flight
// fails with ERROR_SHARING_VIOLATION or ERROR_ACCESS_DENIED (a file pending
// deletion). A missing file is reported at once.
func ReadFile(path string) ([]byte, error) {
	var data []byte
	err := retryTransient(func() error {
		var err error
		data, err = os.ReadFile(path)
		return err
	})
	return data, err
}

// RemoveFile removes path, retrying the brief holds AtomicReplaceFile
// retries: a reader holding path without FILE_SHARE_DELETE fails the removal
// with ERROR_SHARING_VIOLATION or ERROR_ACCESS_DENIED.
func RemoveFile(path string) error {
	return retryTransient(func() error { return os.Remove(path) })
}

// retryTransient runs op until it succeeds, fails with an error other than a
// transient hold, or about two seconds pass, backing off between attempts.
func retryTransient(op func() error) error {
	deadline := time.Now().Add(2 * time.Second)
	backoff := 5 * time.Millisecond
	for {
		err := op()
		if err == nil || !isTransientReplaceError(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(backoff)
		if backoff < 200*time.Millisecond {
			backoff *= 2
		}
	}
}

// isTransientReplaceError reports whether err is the brief hold of another
// handle on the replace target.
func isTransientReplaceError(err error) bool {
	var errno windows.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == windows.ERROR_ACCESS_DENIED || errno == windows.ERROR_SHARING_VIOLATION
}
