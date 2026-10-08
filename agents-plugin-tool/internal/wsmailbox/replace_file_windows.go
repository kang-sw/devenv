//go:build windows

package wsmailbox

import (
	"errors"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// replaceFile renames tmp over path. On Windows the replace fails with
// ERROR_ACCESS_DENIED or ERROR_SHARING_VIOLATION while any other handle holds
// path without FILE_SHARE_DELETE: the unlocked Load/LoadReplyStore reads of a
// mailbox wait poll or a tool-response unread badge in another ws-mcp
// process, or an AV scanner inspecting the previous write. Those holds are
// brief, so retry with back-off for about two seconds before giving up.
// Mirrors wsconfig.replaceConfigFile.
func replaceFile(tmp, path string) error {
	deadline := time.Now().Add(2 * time.Second)
	backoff := 5 * time.Millisecond
	for {
		err := os.Rename(tmp, path)
		if err == nil || !isTransientReplaceError(err) || time.Now().After(deadline) {
			return err
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
