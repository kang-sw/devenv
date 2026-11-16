package wsstate

import "os"

// SameFileMetadata compares regular-file metadata for cache validation, not
// security or authority. No filesystem operation is performed on Windows.
// Equal metadata is not proof against an in-place edit preserving timestamps.
func SameFileMetadata(a, b os.FileInfo) bool {
	if a == nil || b == nil || !a.Mode().IsRegular() || !b.Mode().IsRegular() || a.Mode() != b.Mode() || a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime()) {
		return false
	}
	return sameFileIdentity(a, b)
}
