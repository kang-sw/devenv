package wstrackerhost

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kang-sw/devenv/internal/gitbatch"
	"github.com/kang-sw/devenv/internal/wsledger"
)

// batchRunner is the wsledger git runner for every store: the user's git,
// with remote commands (fetch, push) under the non-interactive bound the
// ticket index uses too (internal/gitbatch): no credential, terminal, or
// askpass prompt, and BatchMode appended to whatever ssh command the user
// configured, so a background check never waits on a prompt.
type batchRunner struct{}

func (batchRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	remote := isRemoteCommand(args)
	if remote {
		args = gitbatch.RemoteArgs(args...)
	}
	cmd := gitCommand(ctx, root, args...)
	// wsledger.GitEnv on every run: a store clone is a partial clone, and
	// only the ledger's explicit attachment read may fetch a missing blob.
	cmd.Env = append(os.Environ(), wsledger.GitEnv...)
	if remote {
		cmd.Env = append(cmd.Env, gitbatch.RemoteEnv(batchSSHCommand(ctx, root))...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// gitCommand is git in root, bounded by ctx (gitbatch.Bound), so a hung
// transport cannot outlast the check's timeout and keep the store's lock.
func gitCommand(ctx context.Context, root string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	gitbatch.Bound(cmd)
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

// batchSSHCommand is gitbatch.SSHCommand over the process environment and
// the store clone's core.sshCommand.
func batchSSHCommand(ctx context.Context, root string) string {
	return gitbatch.SSHCommand(os.Getenv, func() string {
		out, err := gitCommand(ctx, root, "config", "--get", "core.sshCommand").Output()
		if err != nil {
			return ""
		}
		return string(out)
	})
}
