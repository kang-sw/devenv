//go:build windows

package wswhiteboard

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

func openPanelSourceFile(abs string) (*os.File, error) {
	return os.Open(abs)
}

func isSymlinkOpenErr(_ error) bool {
	return false
}

// verifyPanelSourceHandle resolves the kernel final path of both the opened
// file handle and the session root directory handle. Unlike filepath.EvalSymlinks
// on Go 1.23+, GetFinalPathNameByHandle resolves both symlinks and NTFS
// junctions/mount points on the already-opened handle itself.
func verifyPanelSourceHandle(f *os.File, root string) error {
	fileFinal, err := finalPathByHandle(windows.Handle(f.Fd()))
	if err != nil {
		return fmt.Errorf("the file cannot be read: %v", err)
	}
	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return fmt.Errorf("the file cannot be read: %v", err)
	}
	rootHandle, err := windows.CreateFile(
		rootPtr,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return fmt.Errorf("the file cannot be read: %v", err)
	}
	defer windows.CloseHandle(rootHandle)
	rootFinal, err := finalPathByHandle(rootHandle)
	if err != nil {
		return fmt.Errorf("the file cannot be read: %v", err)
	}
	prefix := rootFinal
	if !strings.HasSuffix(prefix, `\`) {
		prefix += `\`
	}
	if len(fileFinal) <= len(prefix) || !strings.EqualFold(fileFinal[:len(prefix)], prefix) {
		return fmt.Errorf("the path is outside the session root %s; keep the file inside it", root)
	}
	return nil
}

func finalPathByHandle(h windows.Handle) (string, error) {
	const fileNameNormalized = 0x0
	buf := make([]uint16, windows.MAX_PATH)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), fileNameNormalized)
		if err != nil {
			return "", err
		}
		if int(n) < len(buf) {
			return windows.UTF16ToString(buf[:n]), nil
		}
		buf = make([]uint16, n+1)
	}
}
