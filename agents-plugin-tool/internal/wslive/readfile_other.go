//go:build !windows

package wslive

import "github.com/kang-sw/devenv/internal/wsstate"

// readReplacedFile reads a file its writer replaces atomically; POSIX readers
// never race the rename.
func readReplacedFile(path string) ([]byte, error) {
	return wsstate.ReadFileOnce(path)
}
