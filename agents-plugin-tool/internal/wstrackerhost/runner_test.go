package wstrackerhost

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestIsRemoteCommand(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"fetch", "origin"}, true},
		{[]string{"push", "--porcelain", "origin", "a:a"}, true},
		{[]string{"ls-remote", "origin"}, true},
		{[]string{"-c", "credential.interactive=false", "fetch", "origin"}, true},
		{[]string{"-c", "a=1", "-c", "b=2", "push"}, true},
		{[]string{"-c", "fetch", "rev-parse"}, false}, // "fetch" here is -c's value
		{[]string{"rev-parse", "HEAD"}, false},
		{[]string{"config", "--get", "remote.origin.url"}, false},
		{[]string{"update-ref", "refs/x", "fetch"}, false},
		{[]string{"-c"}, false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := isRemoteCommand(tc.args); got != tc.want {
			t.Fatalf("isRemoteCommand(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

// batchSSHCommand reads the store clone's core.sshCommand between
// GIT_SSH_COMMAND and GIT_SSH, and the plink variant gets -batch.
func TestBatchSSHCommandPrecedence(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	ctx := context.Background()
	t.Setenv("GIT_SSH_COMMAND", "")
	t.Setenv("GIT_SSH", "")
	if got := batchSSHCommand(ctx, root); got != "ssh -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("default = %q", got)
	}
	t.Setenv("GIT_SSH", `C:\tools\plink.exe`)
	if got := batchSSHCommand(ctx, root); got != `'C:\tools\plink.exe' -batch` {
		t.Fatalf("GIT_SSH plink = %q", got)
	}
	if out, err := exec.Command("git", "-C", root, "config", "core.sshCommand", "ssh -i /k/id").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v: %s", err, out)
	}
	if got := batchSSHCommand(ctx, root); got != "ssh -i /k/id -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("core.sshCommand over GIT_SSH = %q", got)
	}
	t.Setenv("GIT_SSH_COMMAND", "myssh -p 22")
	if got := batchSSHCommand(ctx, root); got != "myssh -p 22 -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("GIT_SSH_COMMAND over all = %q", got)
	}
}

// withGitLookPath replaces the git resolution for one test and forgets the
// resolved path before and after it.
func withGitLookPath(t *testing.T, look func(string) (string, error)) {
	t.Helper()
	prev := gitLookPath
	reset := func() {
		gitResolved.Lock()
		gitResolved.path, gitResolved.under = "", ""
		gitResolved.Unlock()
	}
	reset()
	gitLookPath = look
	t.Cleanup(func() {
		gitLookPath = prev
		reset()
	})
}

// TestGitExecutableResolvesOnce: many commands resolve git once and run
// the resolved path; a failed resolution is retried by the next command,
// which meanwhile runs a bare "git", and the first success is kept.
func TestGitExecutableResolvesOnce(t *testing.T) {
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	calls := 0
	fail := true
	withGitLookPath(t, func(name string) (string, error) {
		calls++
		if fail {
			return "", errors.New("not found")
		}
		return real, nil
	})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if cmd := gitCommand(ctx, t.TempDir(), "status"); cmd.Args[0] != "git" {
			t.Fatalf("a failed resolution must leave the bare name, got %q", cmd.Args[0])
		}
	}
	if calls != 3 {
		t.Fatalf("a failed resolution must be retried per command: %d resolutions for 3 commands", calls)
	}
	fail = false
	calls = 0
	for i := 0; i < 20; i++ {
		cmd := gitCommand(ctx, t.TempDir(), "status")
		if cmd.Path != real || cmd.Args[0] != real {
			t.Fatalf("command runs %q (args[0] %q), want the resolved %q", cmd.Path, cmd.Args[0], real)
		}
	}
	if calls != 1 {
		t.Fatalf("git resolved %d times across 20 commands, want once", calls)
	}
	// Another search path resolves again, once.
	t.Setenv("PATH", t.TempDir()+string(os.PathListSeparator)+os.Getenv("PATH"))
	calls = 0
	for i := 0; i < 5; i++ {
		gitCommand(ctx, t.TempDir(), "status")
	}
	if calls != 1 {
		t.Fatalf("a changed PATH resolved git %d times across 5 commands, want once", calls)
	}
}
