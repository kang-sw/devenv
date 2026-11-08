//go:build !windows

package wstrackerhost

import (
	"os/exec"
	"syscall"
)

// configureKill puts git in its own process group and kills the whole group
// when the context ends, so a transport child (ssh, git-remote-https) dies
// with it instead of holding the pipes open.
func configureKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
