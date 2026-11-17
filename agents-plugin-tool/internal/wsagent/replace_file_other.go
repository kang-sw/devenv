//go:build !windows

package wsagent

import "github.com/kang-sw/devenv/internal/wsstate"

// replaceFile renames tmp over path; POSIX rename replaces atomically even
// while readers hold path open.
func replaceFile(tmp, path string) error {
	return wsstate.AtomicReplaceFile(tmp, path)
}

// readFile reads path; POSIX readers never race a rename.
func readFile(path string) ([]byte, error) {
	return wsstate.ReadFileOnce(path)
}
