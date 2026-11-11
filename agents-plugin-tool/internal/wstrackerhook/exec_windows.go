//go:build windows

package wstrackerhook

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureCommand gives the command its own process group, as a tree
// execjob.KillTree can end, and no console window: the daemon has no
// console, so a console program would otherwise open one per run.
func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW | syscall.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
}
