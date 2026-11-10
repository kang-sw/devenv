package wsexec

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeProgram writes an executable named name into a fresh directory and
// returns that directory.
func fakeProgram(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestLookPathNeverFallsBackToTheProcessPATH pins the caller-environment
// rule: a program the caller's PATH does not hold is an error even when this
// process's PATH holds it, and only a nil environ consults this process's.
func TestLookPathNeverFallsBackToTheProcessPATH(t *testing.T) {
	dir := fakeProgram(t, "ws-fake-tool")
	t.Setenv("PATH", dir)
	if _, err := exec.LookPath("ws-fake-tool"); err != nil {
		t.Fatalf("the fixture program is not on this process's PATH: %v", err)
	}

	got, err := LookPath("ws-fake-tool", []string{"PATH=" + t.TempDir()})
	if err == nil {
		t.Fatalf("a caller PATH without the program resolved it to %q from this process's PATH", got)
	}
	var execErr *exec.Error
	if !errors.As(err, &execErr) || !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("error = %v, want an *exec.Error wrapping exec.ErrNotFound", err)
	}

	if got, err := LookPath("ws-fake-tool", []string{"PATH=" + dir}); err != nil || filepath.Dir(got) != dir {
		t.Fatalf("a caller PATH holding the program = %q, %v", got, err)
	}
	if got, err := LookPath("ws-fake-tool", nil); err != nil || filepath.Dir(got) != dir {
		t.Fatalf("a nil environ (this process's PATH) = %q, %v", got, err)
	}
	if got, err := LookPath(filepath.Join(dir, "elsewhere"), []string{"PATH="}); err != nil || got != filepath.Join(dir, "elsewhere") {
		t.Fatalf("a path-qualified name = %q, %v; want it unchanged", got, err)
	}
}

// TestCommandWithAMissingProgramDoesNotStart: the command built for a
// caller environment whose PATH lacks the program fails at Start with that
// lookup error, as exec.Command does for its own, rather than running the
// program this process's PATH resolves.
func TestCommandWithAMissingProgramDoesNotStart(t *testing.T) {
	dir := fakeProgram(t, "ws-fake-tool")
	t.Setenv("PATH", dir)
	cmd := Command([]string{"PATH=" + t.TempDir()}, "ws-fake-tool")
	if cmd.Err == nil || !errors.Is(cmd.Err, exec.ErrNotFound) {
		t.Fatalf("cmd.Err = %v, want exec.ErrNotFound", cmd.Err)
	}
	if err := cmd.Start(); err == nil || !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("Start = %v, want exec.ErrNotFound", err)
	}
	if cmd := Command(nil, "ws-fake-tool"); cmd.Err != nil {
		t.Fatalf("a nil environ must resolve on this process's PATH: %v", cmd.Err)
	}
}
