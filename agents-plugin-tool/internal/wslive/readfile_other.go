//go:build !windows

package wslive

import "os"

// readReplacedFile reads a file its writer replaces atomically; POSIX readers
// never race the rename.
func readReplacedFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
