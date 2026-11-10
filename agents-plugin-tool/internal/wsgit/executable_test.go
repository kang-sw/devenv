package wsgit

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

// withLookPath replaces the git resolution for one test and forgets the
// resolved path before and after it.
func withLookPath(t *testing.T, look func(string) (string, error)) {
	t.Helper()
	prev := lookPath
	reset := func() {
		resolved.Lock()
		resolved.path, resolved.under = "", ""
		resolved.Unlock()
	}
	reset()
	lookPath = look
	t.Cleanup(func() {
		lookPath = prev
		reset()
	})
}

// TestExecutableResolvesOnce: many calls resolve git once and return the
// resolved path; a failed resolution is retried by the next call, which
// meanwhile returns a bare "git", and the first success is kept.
func TestExecutableResolvesOnce(t *testing.T) {
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	calls := 0
	fail := true
	withLookPath(t, func(name string) (string, error) {
		calls++
		if fail {
			return "", errors.New("not found")
		}
		return real, nil
	})
	for i := 0; i < 3; i++ {
		if got := Executable(); got != "git" {
			t.Fatalf("a failed resolution must leave the bare name, got %q", got)
		}
	}
	if calls != 3 {
		t.Fatalf("a failed resolution must be retried per call: %d resolutions for 3 calls", calls)
	}
	fail = false
	calls = 0
	for i := 0; i < 20; i++ {
		if got := Executable(); got != real {
			t.Fatalf("Executable() = %q, want the resolved %q", got, real)
		}
	}
	if calls != 1 {
		t.Fatalf("git resolved %d times across 20 calls, want once", calls)
	}
	// Another search path resolves again, once.
	t.Setenv("PATH", t.TempDir()+string(os.PathListSeparator)+os.Getenv("PATH"))
	calls = 0
	for i := 0; i < 5; i++ {
		Executable()
	}
	if calls != 1 {
		t.Fatalf("a changed PATH resolved git %d times across 5 calls, want once", calls)
	}
}
