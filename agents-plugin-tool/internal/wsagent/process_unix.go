//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package wsagent

import (
	"errors"
	"syscall"
)

func processAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	return false, err
}
