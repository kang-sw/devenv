//go:build !windows

package wsagent

import "os"

// replaceFile renames tmp over path; POSIX rename replaces atomically even
// while readers hold path open.
func replaceFile(tmp, path string) error {
	return os.Rename(tmp, path)
}
