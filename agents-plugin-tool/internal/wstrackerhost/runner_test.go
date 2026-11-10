package wstrackerhost

import (
	"context"
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
