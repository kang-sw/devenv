//go:build !windows

package wsstate

import "os"

func sameFileIdentity(a, b os.FileInfo) bool {
	return os.SameFile(a, b)
}
