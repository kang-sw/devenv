package wsstate

import (
	"os"
	"syscall"
)

func sameFileIdentity(a, b os.FileInfo) bool {
	x, ok := a.Sys().(*syscall.Win32FileAttributeData)
	if !ok || x == nil {
		return false
	}
	y, ok := b.Sys().(*syscall.Win32FileAttributeData)
	return ok && y != nil && x.CreationTime == y.CreationTime
}
