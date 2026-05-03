//go:build windows

package wsagent

import "os"

func processAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}
	if process == nil {
		return false, nil
	}
	return true, nil
}
