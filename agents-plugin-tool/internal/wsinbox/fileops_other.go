//go:build !windows

package wsinbox

import (
	"os"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// POSIX renames, removes, and opens never fail because another handle holds
// the file, so the inbox file operations need no retry.

func rename(from, to string) error {
	defer bumpNames()
	return wsstate.AtomicReplaceFile(from, to)
}

func remove(path string) error {
	defer bumpNames()
	return os.Remove(path)
}

func readFile(path string) ([]byte, error) { return wsstate.ReadFileOnce(path) }
