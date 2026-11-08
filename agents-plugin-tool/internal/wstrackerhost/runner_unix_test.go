//go:build !windows

package wstrackerhost

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestRunnerTimeoutKillsTransportChildren: a git whose transport child
// keeps the output pipes open (a hung ssh or https helper) still returns
// when the context ends, so a check's timeout bounds how long it holds the
// store. The fake git backgrounds a sleeper that inherits the pipes.
func TestRunnerTimeoutKillsTransportChildren(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nsleep 30 &\nsleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, args := range [][]string{{"rev-list", "--count", "HEAD"}, {"-c", "credential.interactive=false", "fetch", "origin"}} {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		start := time.Now()
		_, err := batchRunner{}.RunGit(ctx, t.TempDir(), args...)
		cancel()
		if err == nil {
			t.Fatalf("%v: a killed git must fail", args)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("%v: returned after %s; the timeout must bound a lingering transport child", args, elapsed)
		}
	}
}

// TestRunnerBatchesOnlyRemoteCommands: a remote command runs with the
// credential flag (once, whether or not the caller passed it) and the
// non-interactive environment; a local command runs as given, with the
// caller's environment. The fake git records its argv and environment.
func TestRunnerBatchesOnlyRemoteCommands(t *testing.T) {
	bin, logs := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in *\"config --get core.sshCommand\"*) exit 1;; esac\n" +
		"printf '%s\\n' \"$@\" > \"$RUNNER_LOG/args\"\nenv > \"$RUNNER_LOG/env\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RUNNER_LOG", logs)
	t.Setenv("GIT_SSH_COMMAND", "myssh -p 22")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	root := t.TempDir()
	read := func(name string) []string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(logs, name))
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}
	batchEnv := []string{"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "GCM_INTERACTIVE=never", "GIT_SSH_COMMAND=myssh -p 22 -o BatchMode=yes -o ConnectTimeout=5"}
	for _, args := range [][]string{{"fetch", "origin"}, {"-c", "credential.interactive=false", "push", "origin", "a:a"}} {
		if _, err := (batchRunner{}).RunGit(context.Background(), root, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		want := append([]string{"-C", root}, args...)
		if args[0] != "-c" {
			want = append([]string{"-C", root, "-c", "credential.interactive=false"}, args...)
		}
		if got := read("args"); !slices.Equal(got, want) {
			t.Fatalf("remote argv = %q, want %q", got, want)
		}
		env := read("env")
		for _, kv := range batchEnv {
			if !slices.Contains(env, kv) {
				t.Fatalf("%v: remote env lacks %q:\n%s", args, kv, strings.Join(env, "\n"))
			}
		}
	}
	if _, err := (batchRunner{}).RunGit(context.Background(), root, "rev-parse", "HEAD"); err != nil {
		t.Fatal(err)
	}
	if got, want := read("args"), []string{"-C", root, "rev-parse", "HEAD"}; !slices.Equal(got, want) {
		t.Fatalf("local argv = %q, want %q", got, want)
	}
	env := read("env")
	if !slices.Contains(env, "GIT_TERMINAL_PROMPT=1") || !slices.Contains(env, "GIT_SSH_COMMAND=myssh -p 22") {
		t.Fatalf("a local command must keep the caller's environment:\n%s", strings.Join(env, "\n"))
	}
}
