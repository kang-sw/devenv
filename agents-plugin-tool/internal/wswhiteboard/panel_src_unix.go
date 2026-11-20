//go:build unix

package wswhiteboard

import (
	"errors"
	"os"
	"syscall"
)

func openPanelSourceFile(abs string) (*os.File, error) {
	return os.OpenFile(abs, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}

func isSymlinkOpenErr(err error) bool {
	return errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.EMLINK)
}

func verifyPanelSourceHandle(_ *os.File, _ string) error {
	return nil
}
