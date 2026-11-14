//go:build !windows

package wsstate

import "os"

// AtomicReplaceFile replaces path with the contents of tmp in a single atomic
// rename. On Unix, os.Rename is atomic within one filesystem, so the caller
// creates tmp in path's directory.
func AtomicReplaceFile(tmp, path string) error {
	return os.Rename(tmp, path)
}

// ReadFile reads path. POSIX opens never meet a sharing violation, so there
// is nothing to retry (see the Windows variant).
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// RemoveFile removes path. POSIX unlinks a file other handles hold open, so
// there is nothing to retry (see the Windows variant).
func RemoveFile(path string) error {
	return os.Remove(path)
}
