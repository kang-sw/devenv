//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package wsagent

import (
	"os/exec"
	"syscall"
)

func configureAsyncCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
