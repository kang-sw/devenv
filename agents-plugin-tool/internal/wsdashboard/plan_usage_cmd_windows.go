package wsdashboard

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func planUsageCommand(ctx context.Context, binary string) *exec.Cmd {
	if ext := filepath.Ext(binary); strings.EqualFold(ext, ".cmd") || strings.EqualFold(ext, ".bat") {
		cmd := exec.CommandContext(ctx, "cmd.exe")
		// cmd.exe does not use CommandLineToArgvW escaping. Preserve its outer
		// /s /c quote pair and the inner quoted path, including spaces.
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /s /c ""` + binary + `" app-server"`}
		return cmd
	}
	return exec.CommandContext(ctx, binary, "app-server")
}
