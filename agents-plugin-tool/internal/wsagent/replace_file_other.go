//go:build !windows

package wsagent

import "os"

// replaceFile renames tmp over path; POSIX rename replaces atomically even
// while readers hold path open.
func replaceFile(tmp, path string) error {
	return os.Rename(tmp, path)
}

// readFile reads path; POSIX readers never race a rename.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
