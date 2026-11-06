//go:build windows

package wsdashboard

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// startDetached starts exe as a detached, console-less process in its own
// process group. It first asks to break away from the caller's job object,
// so a host that kills its job on exit does not take the daemon with it; a
// job that forbids breakaway refuses that start, and the daemon then starts
// inside the job (it ends with the host, and the next ws-mcp starts another).
func startDetached(exe string, args []string, dir string) (*exec.Cmd, error) {
	base := uint32(windows.DETACHED_PROCESS | syscall.CREATE_NEW_PROCESS_GROUP)
	start := func(flags uint32) (*exec.Cmd, error) {
		cmd := exec.Command(exe, args...)
		cmd.Dir = dir
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: true}
		return cmd, cmd.Start()
	}
	if cmd, err := start(base | windows.CREATE_BREAKAWAY_FROM_JOB); err == nil {
		return cmd, nil
	}
	return start(base)
}
