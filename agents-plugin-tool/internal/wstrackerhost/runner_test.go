package wstrackerhost

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	if got := batchSSHCommand(ctx, os.Environ(), root); got != "ssh -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("default = %q", got)
	}
	t.Setenv("GIT_SSH", `C:\tools\plink.exe`)
	if got := batchSSHCommand(ctx, os.Environ(), root); got != `'C:\tools\plink.exe' -batch` {
		t.Fatalf("GIT_SSH plink = %q", got)
	}
	if out, err := exec.Command("git", "-C", root, "config", "core.sshCommand", "ssh -i /k/id").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v: %s", err, out)
	}
	if got := batchSSHCommand(ctx, os.Environ(), root); got != "ssh -i /k/id -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("core.sshCommand over GIT_SSH = %q", got)
	}
	t.Setenv("GIT_SSH_COMMAND", "myssh -p 22")
	if got := batchSSHCommand(ctx, os.Environ(), root); got != "myssh -p 22 -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("GIT_SSH_COMMAND over all = %q", got)
	}
}

// The default runner's git is the one on the host's environment PATH, and
// the run inherits that environment: a hosted session's git, not the
// daemon's.
func TestBatchRunnerRunsGitFromItsEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake git is a POSIX shell script")
	}
	fakeDir := t.TempDir()
	script := "#!/bin/sh\necho \"fake-git $TRACKER_MARK $*\"\n"
	if err := os.WriteFile(filepath.Join(fakeDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + fakeDir, "TRACKER_MARK=hosted"}
	out, err := batchRunner{environ: func() []string { return env }}.RunGit(context.Background(), t.TempDir(), "status")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); !strings.HasPrefix(got, "fake-git hosted -C ") || !strings.HasSuffix(got, " status") {
		t.Fatalf("output = %q, want the fake git under the host environment", got)
	}
	t.Setenv("TRACKER_MARK", "process")
	t.Setenv("PATH", fakeDir)
	out, err = batchRunner{}.RunGit(context.Background(), t.TempDir(), "status")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); !strings.HasPrefix(got, "fake-git process -C ") {
		t.Fatalf("output = %q, want the process environment without environ", got)
	}
}
