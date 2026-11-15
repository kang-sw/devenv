package wsgit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func lookupGitFixture(t *testing.T, label string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "git")
	script := "#!/bin/sh\nif [ \"$3\" = wait ]; then exec /bin/sleep 30; fi\nprintf '" + label + ":%s:%s:%s:%s\\n' \"$LOOKUP_VALUE\" \"$1\" \"$2\" \"$3\"\nprintf 'stderr\\n' >&2\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

func TestExecRunnerGitLookup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX script execution; facade construction is covered separately")
	}
	a, first := lookupGitFixture(t, "A")
	b, _ := lookupGitFixture(t, "B")
	t.Setenv("PATH", a)
	t.Setenv("LOOKUP_VALUE", "process")
	root := t.TempDir()
	check := func(r ExecRunner, label, value string) {
		t.Helper()
		out, err := r.RunGit(context.Background(), root, "arg with spaces")
		want := label + ":" + value + ":-C:" + root + ":arg with spaces\nstderr\n"
		if err != nil || string(out) != want {
			t.Fatalf("output %q, error %v; want %q", out, err, want)
		}
	}
	for i := 0; i < 3; i++ {
		check(ExecRunner{}, "A", "process")
	}
	env := []string{"PATH=" + a, "LOOKUP_VALUE=caller"}
	caller := ExecRunner{Environ: func() []string { return env }, Env: []string{"LOOKUP_VALUE=override"}}
	for _, dir := range []string{a, b, a, b, a} {
		env = []string{"PATH=" + dir, "LOOKUP_VALUE=caller"}
		label := "A"
		if dir == b {
			label = "B"
		}
		check(caller, label, "override")
	}
	// A nil callback result means process lookup; an explicit empty slice does not.
	check(ExecRunner{Environ: func() []string { return nil }}, "A", "process")
	for _, missing := range [][]string{{}, {"PATH=" + t.TempDir()}} {
		_, err := (ExecRunner{Environ: func() []string { return missing }}).RunGit(context.Background(), root, "arg")
		if !errors.Is(err, exec.ErrNotFound) {
			t.Fatalf("missing caller Git: %v", err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (ExecRunner{}).RunGit(context.Background(), root, "arg"); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("missing process Git: %v", err)
	}
	t.Setenv("PATH", a)
	check(ExecRunner{}, "A", "process")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (ExecRunner{}).RunGit(ctx, root, "wait"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	// Warm process resolution, then remove the selected binary without changing PATH.
	fallback := filepath.Join(b, "git")
	t.Setenv("PATH", a+string(os.PathListSeparator)+b)
	check(ExecRunner{}, "A", "process")
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	if out, err := (ExecRunner{}).RunGit(context.Background(), root, "arg"); err == nil || strings.Contains(string(out), "B:") {
		t.Fatalf("stale launch unexpectedly selected %s: %q, %v", fallback, out, err)
	}
	if got := Executable(); got != first {
		t.Fatalf("launch error re-resolved Git: %q", got)
	}
}

func TestLookPathEnvNonGitIsUncached(t *testing.T) {
	dir := t.TempDir()
	name := "lookup-helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + dir, "PATHEXT=.EXE"}
	if got, err := LookPathEnv("lookup-helper", env); err != nil || got != path {
		t.Fatalf("lookup = %q, %v", got, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := LookPathEnv("lookup-helper", env); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("generic lookup cached: %v", err)
	}
}
