package wsgit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Resolver counts live in wsexec; this facade pins observable retry, reuse,
// and key changes without maintaining a second resolver seam.
func TestExecutableResolvesOnce(t *testing.T) {
	t.Setenv("NoDefaultCurrentDirectoryInExePath", "1")
	t.Setenv("PATHEXT", ".EXE")
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	for i := 0; i < 3; i++ {
		if got := Executable(); got != "git" {
			t.Fatalf("missing Git = %q", got)
		}
	}
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if got := Executable(); got != path {
			t.Fatalf("resolved Git = %q, want %q", got, path)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := Executable(); got != path {
		t.Fatalf("stale success = %q, want %q", got, path)
	}
	other := t.TempDir()
	next := filepath.Join(other, name)
	if err := os.WriteFile(next, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", other)
	if got := Executable(); got != next {
		t.Fatalf("changed PATH = %q, want %q", got, next)
	}
	t.Setenv("PATH", t.TempDir())
	if got := Executable(); got != "git" {
		t.Fatalf("missing Git fallback = %q", got)
	}
}
