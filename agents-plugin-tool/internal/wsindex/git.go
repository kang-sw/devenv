// Package wsindex keeps the origin-backed ticket registration and ownership
// index: one custom ref on the git remote, written with compare-and-swap
// pushes under the user's own git credentials, mirrored into a clone-shared
// local cache ref, with a clone-shared pending log for writes made while the
// remote is unreachable.
//
// Directory position stays the ticket status authority; the index only
// overlays registration and ownership. When the remote has no index ref every
// caller must behave exactly as if this package did not exist.
package wsindex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kang-sw/devenv/internal/gitbatch"
	"github.com/kang-sw/devenv/internal/wsgit"
)

// Namespace is the protocol-named ref namespace. It is deliberately not
// derived from any distribution identity (marketplace, repository, or user
// name); the trailing word only makes it unique. It avoids every
// host-reserved namespace (GitHub/Gitea refs/pull/, Gitea refs/for/, GitLab
// refs/merge-requests/, refs/pipelines/, refs/environments/,
// refs/keep-around/, refs/tmp/, refs/remotes/).
const Namespace = "ticket-index-larkspur"

const (
	// RemoteRef is the tier-1 index ref on the remote.
	RemoteRef = "refs/" + Namespace + "/v1/index"

	localPrefix = "refs/" + Namespace + "-local/v1/"
	// CacheRef mirrors the last remote index tip this clone observed. It is
	// advanced only after a push is accepted or a fetch returns, so it never
	// holds a commit the remote has not seen.
	CacheRef = localPrefix + "cache"
	// PendingRef holds the offline pending log.
	PendingRef = localPrefix + "pending"
	// fetchTempPrefix names per-call scratch refs a fetch lands into before
	// the continuity check decides what the cache ref becomes.
	fetchTempPrefix = localPrefix + "fetch/"

	// RemoteName is the remote the index lives on. ws has no configured
	// remote concept of its own; the review-track heuristic already reads
	// origin, so the index does too.
	RemoteName = "origin"

	indexFile   = "index.json"
	pendingFile = "pending.json"

	// CloneIDConfigKey stores the per-clone random id in the clone's shared
	// .git/config.
	CloneIDConfigKey = "ticketindex.cloneid"

	stateDirName = Namespace
)

// discoveryCandidates is the ordered list one ls-remote checks. Only tier 1
// exists today; a later tier-2 branch fallback (refs/heads/__<ns>_v1) is
// appended here, and every consumer already walks the list in order.
var discoveryCandidates = []string{RemoteRef}

// Command is one git invocation. Remote marks a command that talks to the
// remote (ls-remote, fetch, push); it is what tests count.
type Command struct {
	Args   []string
	Env    []string
	Stdin  []byte
	Remote bool
}

// Runner executes git commands. Tests wrap it to count remote invocations or
// inject failures.
type Runner interface {
	Run(ctx context.Context, dir string, cmd Command) (stdout []byte, err error)
}

// GitError carries a failed git command's stderr so callers can classify
// remote rejections apart from transport failures.
type GitError struct {
	Args     []string
	Stderr   string
	Stdout   string
	Err      error
	TimedOut bool
}

func (e *GitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if e.TimedOut {
		msg = "timed out; " + msg
	}
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.Args, " "), e.Err, msg)
}

func (e *GitError) Unwrap() error { return e.Err }

// ExecRunner runs the real git binary. Environ, when set, is the base
// environment of every run, git resolved from its PATH (the caller
// process's, for a server the dashboard daemon hosts for another process);
// nil means this process's.
type ExecRunner struct {
	Environ func() []string
}

// Run executes git in dir. A context deadline kills git; WaitDelay bounds the
// wait for a lingering child (an ssh subprocess can hold the pipes open after
// git itself is killed), so a hard timeout is a hard timeout.
func (r ExecRunner) Run(ctx context.Context, dir string, c Command) ([]byte, error) {
	argv := append([]string{"-C", dir}, c.Args...)
	base, bin := os.Environ(), "git"
	var lookErr error
	if r.Environ != nil {
		base = r.Environ()
		bin, lookErr = wsgit.LookPathEnv("git", base)
	}
	cmd := exec.CommandContext(ctx, bin, argv...)
	if lookErr != nil {
		cmd.Err = lookErr
	}
	cmd.Env = append(base, c.Env...)
	gitbatch.Bound(cmd)
	if c.Stdin != nil {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.Bytes(), &GitError{
			Args:     c.Args,
			Stderr:   stderr.String(),
			Stdout:   stdout.String(),
			Err:      err,
			TimedOut: errors.Is(ctx.Err(), context.DeadlineExceeded),
		}
	}
	return stdout.Bytes(), nil
}

// git runs a local git command and returns trimmed stdout.
func (c *Client) git(ctx context.Context, stdin []byte, args ...string) (string, error) {
	out, err := c.runner.Run(ctx, c.root, Command{Args: args, Stdin: stdin, Env: localEnv})
	return strings.TrimSpace(string(out)), err
}

// localEnv pins the identity git needs for commit-tree so an index write never
// fails on a clone without user.name. The index commit carries no author
// information callers rely on; lease identity lives in the records.
var localEnv = []string{
	"GIT_AUTHOR_NAME=ticket-index",
	"GIT_AUTHOR_EMAIL=ticket-index@localhost",
	"GIT_COMMITTER_NAME=ticket-index",
	"GIT_COMMITTER_EMAIL=ticket-index@localhost",
}

// remoteEnv builds the non-interactive environment every remote command runs
// under (gitbatch.RemoteEnv) with the ssh command this clone resolves.
func (c *Client) remoteEnv(ctx context.Context) []string {
	return gitbatch.RemoteEnv(c.batchSSHCommand(ctx))
}

// batchSSHCommand is gitbatch.SSHCommand over this client's environment and
// the clone's core.sshCommand.
func (c *Client) batchSSHCommand(ctx context.Context) string {
	return gitbatch.SSHCommand(c.getenv, func() string {
		out, err := c.git(ctx, nil, "config", "--get", "core.sshCommand")
		if err != nil {
			return ""
		}
		return out
	})
}

// remote runs a remote git command under the non-interactive environment.
// The caller's ctx carries the hard timeout.
func (c *Client) remote(ctx context.Context, args ...string) (string, error) {
	env := append(append([]string{}, localEnv...), c.remoteEnv(ctx)...)
	out, err := c.runner.Run(ctx, c.root, Command{Args: gitbatch.RemoteArgs(args...), Env: env, Remote: true})
	return string(out), err
}
