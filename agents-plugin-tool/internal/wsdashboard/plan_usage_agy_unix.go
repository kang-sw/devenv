//go:build !windows

package wsdashboard

import (
	"os/exec"
	"syscall"
)

func configureAgyUsageCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
