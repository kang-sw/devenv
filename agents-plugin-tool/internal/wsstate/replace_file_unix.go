//go:build !windows

package wsstate

import "os"

// AtomicReplaceFile replaces path with the contents of tmp in a single atomic
// rename. On Unix, os.Rename is atomic within one filesystem, so the caller
// creates tmp in path's directory.
func AtomicReplaceFile(tmp, path string) error {
	return os.Rename(tmp, path)
}
