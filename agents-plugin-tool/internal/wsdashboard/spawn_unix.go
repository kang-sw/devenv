//go:build !windows

package wsdashboard

import (
	"os/exec"
	"syscall"
)

// startDetached starts exe in a new session, so it has no controlling
// terminal and no hangup or job-control signal aimed at the caller reaches it.
func startDetachedStrict(exe string, args []string, dir string) (*exec.Cmd, error) {
	return startDetached(exe, args, dir)
}

func startDetached(exe string, args []string, dir string) (*exec.Cmd, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd, cmd.Start()
}
