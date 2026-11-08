//go:build windows

package wsinbox

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Every inbox state change starts with a rename used as an exclusive claim,
// and os.Rename (MoveFileEx) is not one on Windows: it opens the source by
// name with full sharing and renames through that handle, so two claimers
// that both opened an entry before either renamed it both succeed, the
// second moving the file the first just claimed (one entry drained twice, or
// a claimed file gone from under its claimant). rename here opens the source
// with no sharing at all and renames through that handle, so exactly one
// claimer holds it and a later one finds the name gone (ErrGone).
//
// The exclusive open, a remove, and a reader's open each fail with
// ERROR_SHARING_VIOLATION or ERROR_ACCESS_DENIED while another handle holds
// the file. Such a hold is brief (a read, a claim in flight), so those
// operations retry with back-off for about two seconds. A missing file is
// not transient, so a claim that lost still sees ErrGone.

func rename(from, to string) error {
	err := retryTransient(func() error { return renameExclusive(from, to) })
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}

// fileRenameInfo is FILE_RENAME_INFO with a one-element FileName; the name
// continues past the end of the struct.
type fileRenameInfo struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func renameExclusive(from, to string) error {
	src, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(to)
	if err != nil {
		return err
	}
	name, err := windows.UTF16FromString(abs)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(src, windows.DELETE|windows.SYNCHRONIZE, 0, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	nameBytes := (len(name) - 1) * 2 // without the terminating NUL
	buf := make([]byte, int(unsafe.Offsetof(fileRenameInfo{}.FileName))+len(name)*2)
	info := (*fileRenameInfo)(unsafe.Pointer(&buf[0]))
	info.ReplaceIfExists = 1 // os.Rename's semantics
	info.FileNameLength = uint32(nameBytes)
	copy(unsafe.Slice(&info.FileName[0], len(name)), name)
	return windows.SetFileInformationByHandle(h, windows.FileRenameInfo, &buf[0], uint32(len(buf)))
}

func remove(path string) error {
	return retryTransient(func() error { return os.Remove(path) })
}

func readFile(path string) ([]byte, error) {
	var raw []byte
	err := retryTransient(func() error {
		var err error
		raw, err = os.ReadFile(path)
		return err
	})
	return raw, err
}

func retryTransient(op func() error) error {
	deadline := time.Now().Add(2 * time.Second)
	backoff := time.Millisecond
	for {
		err := op()
		if err == nil || !isTransient(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(backoff)
		if backoff < 100*time.Millisecond {
			backoff *= 2
		}
	}
}

func isTransient(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == windows.ERROR_ACCESS_DENIED || errno == windows.ERROR_SHARING_VIOLATION
}
