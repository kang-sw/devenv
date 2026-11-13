//go:build !windows

package wsagent

import (
	"os"
	"syscall"
)

func claudeSettingsOwnedByUser(path string, noFollow bool) bool {
	stat := os.Stat
	if noFollow {
		stat = os.Lstat
	}
	info, err := stat(path)
	if err != nil {
		return false
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == uint32(os.Geteuid())
}
