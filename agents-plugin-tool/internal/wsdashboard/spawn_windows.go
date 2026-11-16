//go:build windows

package wsdashboard

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// startDetached starts exe in its own process group with its own hidden
// console. It first asks to break away from the caller's job object,
// so a host that kills its job on exit does not take the daemon with it; a
// job that forbids breakaway refuses that start, and the daemon then starts
// inside the job (it ends with the host, and the next ws-mcp starts another).
//
// CREATE_NO_WINDOW, not DETACHED_PROCESS: a console-less parent makes Windows
// allocate a new, visible console for every console-subsystem child, so a
// DETACHED_PROCESS daemon flashes a window for each git/ssh/shell it runs.
// With a hidden console of its own, children and grandchildren inherit it and
// open no window, and the console is still not the host terminal's.
func startDetached(exe string, args []string, dir string) (*exec.Cmd, error) {
	return startWindowsDetached(exe, args, dir, false)
}

func startDetachedStrict(exe string, args []string, dir string) (*exec.Cmd, error) {
	return startWindowsDetached(exe, args, dir, true)
}

func startWindowsDetached(exe string, args []string, dir string, strict bool) (*exec.Cmd, error) {
	return startWindowsDetachedWith(strict, func(flags uint32) (*exec.Cmd, error) {
		cmd := exec.Command(exe, args...)
		cmd.Dir = dir
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: true}
		return cmd, cmd.Start()
	})
}

func startWindowsDetachedWith(strict bool, start func(uint32) (*exec.Cmd, error)) (*exec.Cmd, error) {
	base := uint32(windows.CREATE_NO_WINDOW | syscall.CREATE_NEW_PROCESS_GROUP)
	if cmd, err := start(base | windows.CREATE_BREAKAWAY_FROM_JOB); err == nil {
		return cmd, nil
	} else if strict {
		return nil, err // Never retry inside Electron's job.
	}
	return start(base)
}
