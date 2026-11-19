//go:build windows

package wsdashboard

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureAgyUsageCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW | syscall.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
}
