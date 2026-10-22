//go:build !windows

package wsmailbox

import "os"

// replaceFile renames tmp over path; POSIX rename replaces atomically even
// while readers hold path open.
func replaceFile(tmp, path string) error {
	return os.Rename(tmp, path)
}

// readFile reads path; POSIX opens never fail with a sharing violation, so
// there is nothing to retry.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
