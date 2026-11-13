//go:build !windows

package wsdashboard

import (
	"context"
	"os/exec"
)

func planUsageCommand(ctx context.Context, binary string) *exec.Cmd {
	return exec.CommandContext(ctx, binary, "app-server")
}
