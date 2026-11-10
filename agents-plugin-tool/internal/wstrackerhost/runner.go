package wstrackerhost

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/kang-sw/devenv/internal/gitbatch"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsledger"
)

// batchRunner is the wsledger git runner for every store: the user's git,
// with remote commands (fetch, push) under the non-interactive bound the
// ticket index uses too (internal/gitbatch): no credential, terminal, or
// askpass prompt, and BatchMode appended to whatever ssh command the user
// configured, so a background check never waits on a prompt.
type batchRunner struct {
	// environ is the base environment of every git run; nil means the
	// process environment.
	environ func() []string
}

// env is the runner's base environment.
func (r batchRunner) env() []string {
	if r.environ == nil {
		return os.Environ()
	}
	return r.environ()
}

func (r batchRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	remote := isRemoteCommand(args)
	if remote {
		args = gitbatch.RemoteArgs(args...)
	}
	env := r.env()
	cmd := gitCommand(ctx, env, root, args...)
	// wsledger.GitEnv on every run: a store clone is a partial clone, and
	// only the ledger's explicit attachment read may fetch a missing blob.
	cmd.Env = append(append([]string(nil), env...), wsledger.GitEnv...)
	if remote {
		cmd.Env = append(cmd.Env, gitbatch.RemoteEnv(batchSSHCommand(ctx, env, root))...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// gitCommand is git in root, bounded by ctx (gitbatch.Bound), so a hung
// transport cannot outlast the check's timeout and keep the store's lock.
// The executable is looked up on env's PATH; a caller environment without
// git leaves the command unstartable (cmd.Err) rather than running the
// daemon's git.
func gitCommand(ctx context.Context, env []string, root string, args ...string) *exec.Cmd {
	bin, lookErr := gitExecutable(env)
	cmd := exec.CommandContext(ctx, bin, append([]string{"-C", root}, args...)...)
	if lookErr != nil {
		cmd.Err = lookErr
	}
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

// gitExecutable is git on env's PATH (an error when that PATH has none),
// or wsgit.Executable() for a nil env, the process environment.
func gitExecutable(env []string) (string, error) {
	if env == nil {
		return wsgit.Executable(), nil
	}
	return wsgit.LookPathEnv("git", env)
}

// lookupEnv is key's value in env, matched case-insensitively when fold is
// set (Windows environment variable names), or "" when absent.
func lookupEnv(env []string, key string, fold bool) string {
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok && (k == key || (fold && strings.EqualFold(k, key))) {
			return v
		}
	}
	return ""
}

// batchSSHCommand is gitbatch.SSHCommand over env and the store clone's
// core.sshCommand.
func batchSSHCommand(ctx context.Context, env []string, root string) string {
	getenv := func(key string) string { return lookupEnv(env, key, runtime.GOOS == "windows") }
	return gitbatch.SSHCommand(getenv, func() string {
		probe := gitCommand(ctx, env, root, "config", "--get", "core.sshCommand")
		probe.Env = env
		out, err := probe.Output()
		if err != nil {
			return ""
		}
		return string(out)
	})
}
