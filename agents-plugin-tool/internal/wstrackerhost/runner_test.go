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

// The ssh command probe reads GIT_SSH_COMMAND, GIT_SSH, and core.sshCommand
// from the environment it is given (a hosted session's), not the process's:
// the config probe runs the git on that environment's PATH, under that
// environment.
func TestBatchSSHCommandReadsTheGivenEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake git is a POSIX shell script")
	}
	fakeDir := t.TempDir()
	// The fake answers the core.sshCommand probe from its own environment
	// and fails every other command, so a probe on the process environment
	// (no SSH_FROM) is visible as a different answer.
	script := "#!/bin/sh\nif [ \"$3\" = config ] && [ -n \"$SSH_FROM\" ]; then echo \"ssh-from-$SSH_FROM\"; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(fakeDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeDir)
	t.Setenv("SSH_FROM", "process")
	t.Setenv("GIT_SSH_COMMAND", "process-ssh")
	t.Setenv("GIT_SSH", "")
	ctx := context.Background()
	root := t.TempDir()

	env := []string{"PATH=" + fakeDir, "SSH_FROM=hosted"}
	if got := batchSSHCommand(ctx, env, root); got != "ssh-from-hosted -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("core.sshCommand probe = %q, want the answer from the given environment", got)
	}
	env = append(env, "GIT_SSH_COMMAND=hosted-ssh")
	if got := batchSSHCommand(ctx, env, root); got != "hosted-ssh -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("GIT_SSH_COMMAND from the given environment = %q", got)
	}
	if got := batchSSHCommand(ctx, []string{"PATH=" + fakeDir, "GIT_SSH=plink"}, root); got != "'plink' -batch" {
		t.Fatalf("GIT_SSH from the given environment = %q", got)
	}
}

// Windows environment variable names are case-insensitive, so a hello
// environment spelling Git_Ssh_Command still names the ssh command there;
// elsewhere the name is exact.
func TestLookupEnvFoldsCaseOnlyWhenAsked(t *testing.T) {
	env := []string{"Git_Ssh_Command=folded", "PATH=/bin", "GIT_SSH=exact"}
	if got := lookupEnv(env, "GIT_SSH_COMMAND", true); got != "folded" {
		t.Fatalf("folded lookup = %q", got)
	}
	if got := lookupEnv(env, "GIT_SSH_COMMAND", false); got != "" {
		t.Fatalf("exact lookup found a differently cased key: %q", got)
	}
	if got := lookupEnv(env, "GIT_SSH", false); got != "exact" {
		t.Fatalf("exact lookup = %q", got)
	}
	if got := lookupEnv(env, "git_ssh", true); got != "exact" {
		t.Fatalf("folded lookup of a lower-case key = %q", got)
	}
	if got := lookupEnv(env, "MISSING", true); got != "" {
		t.Fatalf("missing key = %q", got)
	}
}
