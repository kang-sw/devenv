//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package wsagent

import (
	"os/exec"
	"syscall"
)

func configureRunnerCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
