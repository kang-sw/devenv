//go:build !windows

package wsholder

import (
	"os/exec"
	"syscall"
)

// breakawayApplies reports whether a holder start first asks to leave the
// starter's job object; only Windows has one.
const breakawayApplies = false

// startDetached starts exe in a new session with its standard streams on
// the null device, so it has no controlling terminal, leads its own process
// group, and outlives the caller. breakaway is ignored.
func startDetached(exe string, args []string, dir string, _ bool) (*exec.Cmd, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd, cmd.Start()
}
