//go:build !unix && !windows

package wswhiteboard

import "os"

func openPanelSourceFile(abs string) (*os.File, error) {
	return os.Open(abs)
}

func isSymlinkOpenErr(_ error) bool {
	return false
}

func verifyPanelSourceHandle(_ *os.File, _ string) error {
	return nil
}
