//go:build windows

package wslive

import (
	"errors"
	"syscall"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
	"golang.org/x/sys/windows"
)

// readReplacedFile reads a file its writer replaces atomically (temp file plus
// rename). On Windows an open racing that rename fails with
// ERROR_SHARING_VIOLATION or ERROR_ACCESS_DENIED; the hold is brief, so retry
// with back-off for about two seconds. Mirrors wsagent's readFile.
func readReplacedFile(path string) ([]byte, error) {
	deadline := time.Now().Add(2 * time.Second)
	backoff := 5 * time.Millisecond
	for {
		raw, err := wsstate.ReadFileOnce(path)
		if err == nil || !isTransientReplaceError(err) || time.Now().After(deadline) {
			return raw, err
		}
		time.Sleep(backoff)
		if backoff < 200*time.Millisecond {
			backoff *= 2
		}
	}
}

func isTransientReplaceError(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == windows.ERROR_ACCESS_DENIED || errno == windows.ERROR_SHARING_VIOLATION
}
