package wstrackerhost

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// batchRunner is the wsledger git runner for every store: the user's git,
// with remote commands (fetch, push) under a non-interactive environment so
// a background check never waits on a credential prompt. It mirrors the
// ticket index's remote environment (internal/wsindex remoteEnv): no
// terminal prompt or askpass helper, and BatchMode appended to whatever ssh
// command the user configured.
type batchRunner struct{}

func (batchRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := gitCommand(ctx, root, args...)
	if isRemoteCommand(args) {
		cmd.Env = append(os.Environ(), batchRemoteEnv(ctx, root)...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// gitCommand is git in root, bounded by ctx: the context's end kills git's
// whole process group, and WaitDelay bounds the wait for a child that still
// holds the pipes, so a hung transport cannot outlast the check's timeout
// (and keep the store's lock) the way a plain CommandContext lets it. The
// same bound as the ticket index's runner (internal/wsindex ExecRunner).
func gitCommand(ctx context.Context, root string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	cmd.WaitDelay = 300 * time.Millisecond
	configureKill(cmd)
	return cmd
}

// isRemoteCommand reports whether args run a git subcommand that reaches the
// remote. wsledger puts "-c" options before the subcommand.
func isRemoteCommand(args []string) bool {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c":
			i++
		case "fetch", "push", "ls-remote":
			return true
		default:
			return false
		}
	}
	return false
}

func batchRemoteEnv(ctx context.Context, root string) []string {
	return []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"GCM_INTERACTIVE=never",
		"GIT_SSH_COMMAND=" + batchSSHCommand(ctx, root),
	}
}

// batchSSHCommand keeps git's own precedence for the ssh command
// (GIT_SSH_COMMAND, core.sshCommand, GIT_SSH, ssh) and appends the
// non-interactive options for its variant.
func batchSSHCommand(ctx context.Context, root string) string {
	base := strings.TrimSpace(os.Getenv("GIT_SSH_COMMAND"))
	if base == "" {
		if out, err := gitCommand(ctx, root, "config", "--get", "core.sshCommand").Output(); err == nil {
			base = strings.TrimSpace(string(out))
		}
	}
	if base == "" {
		if program := strings.TrimSpace(os.Getenv("GIT_SSH")); program != "" {
			base = "'" + strings.ReplaceAll(program, "'", `'\''`) + "'"
		}
	}
	if base == "" {
		base = "ssh"
	}
	if fields := strings.Fields(base); len(fields) > 0 && strings.Contains(strings.ToLower(filepath.Base(strings.Trim(fields[0], `'"`))), "plink") {
		return base + " -batch"
	}
	return base + " -o BatchMode=yes -o ConnectTimeout=5"
}
