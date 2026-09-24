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
	"path/filepath"
	"strings"
	"time"
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

// ExecRunner runs the real git binary.
type ExecRunner struct{}

// Run executes git in dir. A context deadline kills git; WaitDelay bounds the
// wait for a lingering child (an ssh subprocess can hold the pipes open after
// git itself is killed), so a hard timeout is a hard timeout.
func (ExecRunner) Run(ctx context.Context, dir string, c Command) ([]byte, error) {
	argv := append([]string{"-C", dir}, c.Args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	cmd.Env = append(os.Environ(), c.Env...)
	cmd.WaitDelay = 300 * time.Millisecond
	configureKill(cmd)
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

// remoteArgs prefixes a remote git subcommand with config that disables
// credential prompting on git versions that understand it (older versions
// ignore the unknown key).
func remoteArgs(args ...string) []string {
	return append([]string{"-c", "credential.interactive=false"}, args...)
}

// remoteEnv builds the non-interactive environment every remote command runs
// under: no terminal prompt, no askpass helper, and BatchMode ssh appended to
// whatever ssh command the user configured.
func (c *Client) remoteEnv(ctx context.Context) []string {
	env := []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"GCM_INTERACTIVE=never",
	}
	return append(env, "GIT_SSH_COMMAND="+c.batchSSHCommand(ctx))
}

// batchSSHCommand preserves the configured ssh command with git's own
// precedence (GIT_SSH_COMMAND, then core.sshCommand, then GIT_SSH, then ssh)
// and appends the non-interactive options for its variant.
func (c *Client) batchSSHCommand(ctx context.Context) string {
	base := strings.TrimSpace(c.getenv("GIT_SSH_COMMAND"))
	if base == "" {
		if out, err := c.git(ctx, nil, "config", "--get", "core.sshCommand"); err == nil {
			base = strings.TrimSpace(out)
		}
	}
	if base == "" {
		if program := strings.TrimSpace(c.getenv("GIT_SSH")); program != "" {
			base = shellQuote(program)
		}
	}
	if base == "" {
		base = "ssh"
	}
	if isPlink(base) {
		return base + " -batch"
	}
	return base + " -o BatchMode=yes -o ConnectTimeout=5"
}

func isPlink(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	name := strings.ToLower(filepath.Base(strings.Trim(fields[0], `'"`)))
	return strings.Contains(name, "plink")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// remote runs a remote git command under the non-interactive environment.
// The caller's ctx carries the hard timeout.
func (c *Client) remote(ctx context.Context, args ...string) (string, error) {
	env := append(append([]string{}, localEnv...), c.remoteEnv(ctx)...)
	out, err := c.runner.Run(ctx, c.root, Command{Args: remoteArgs(args...), Env: env, Remote: true})
	return string(out), err
}
