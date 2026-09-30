//go:build !windows

package wsconfig

import "os"

// replaceConfigFile renames tmp over path; POSIX rename replaces atomically
// even while readers hold path open.
func replaceConfigFile(tmp, path string) error {
	return os.Rename(tmp, path)
}
