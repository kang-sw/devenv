//go:build !windows

package wstrackerhook

import (
	"os/exec"
	"syscall"
)

// configureCommand gives the command its own process group, so a timeout
// kills everything it started (execjob.KillTree).
func configureCommand(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
