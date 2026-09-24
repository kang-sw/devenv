//go:build !windows

package wsindex

import (
	"os/exec"
	"syscall"
)

// configureKill puts git in its own process group and kills the whole group
// on timeout, so an ssh child dies with it instead of lingering.
func configureKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
