//go:build !windows

package wsstate

import "os"

// atomicReplaceFile replaces path with the contents of tmp in a single atomic
// rename. On Unix, os.Rename is atomic within the same filesystem, which is
// always the case here (tmp is produced by uniqueTempPath in the same dir).
func atomicReplaceFile(tmp, path string) error {
	return os.Rename(tmp, path)
}
