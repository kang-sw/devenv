//go:build windows

package wsholder

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// breakawayApplies reports whether a holder start first asks to leave the
// starter's job object; only Windows has one.
const breakawayApplies = true

// startDetached starts exe in its own process group with its own hidden
// console (CREATE_NO_WINDOW, as the daemon's own start does, so the holder's
// children open no window and share a console a control event can reach)
// and its standard streams on the null device. With breakaway it also asks
// to leave the caller's job object, so a host that kills its job on exit
// does not take the holder with it; a job that forbids breakaway refuses
// that start, and the caller starts again without it.
func startDetached(exe string, args []string, dir string, breakaway bool) (*exec.Cmd, error) {
	flags := uint32(windows.CREATE_NO_WINDOW | syscall.CREATE_NEW_PROCESS_GROUP)
	if breakaway {
		flags |= windows.CREATE_BREAKAWAY_FROM_JOB
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: true}
	return cmd, cmd.Start()
}
