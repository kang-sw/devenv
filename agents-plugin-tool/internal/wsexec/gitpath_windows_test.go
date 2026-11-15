package wsexec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitPathWindowsImplicitCwdBypassesReuse(t *testing.T) {
	dir := fakeProgram(t, "git")
	changeDir(t, t.TempDir())
	t.Setenv("PATH", dir)
	t.Setenv("PATHEXT", ".EXE")
	t.Setenv("NoDefaultCurrentDirectoryInExePath", "")
	if err := os.Unsetenv("NoDefaultCurrentDirectoryInExePath"); err != nil {
		t.Fatal(err)
	}
	var cache gitPathCache
	calls := 0
	lookup := func(name string, env []string) (string, error) {
		calls++
		return LookPath(name, env)
	}
	for i := 0; i < 3; i++ {
		if got, err := cache.resolve(nil, lookup); err != nil || got != filepath.Join(dir, "git.exe") {
			t.Fatalf("implicit cwd lookup: %q, %v", got, err)
		}
	}
	if calls != 3 {
		t.Fatalf("implicit cwd enabled: %d searches, want 3", calls)
	}
	// Presence, not truthiness, disables Go's implicit cwd lookup.
	t.Setenv("NoDefaultCurrentDirectoryInExePath", "")
	for i := 0; i < 3; i++ {
		if _, err := cache.resolve(nil, lookup); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 4 {
		t.Fatalf("implicit cwd disabled: %d searches, want 4 total", calls)
	}
}

func TestGitPathWindowsPATHEXTSelection(t *testing.T) {
	dir := fakeProgram(t, "git")
	if err := os.WriteFile(filepath.Join(dir, "git.cmd"), []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	var cache gitPathCache
	for _, tc := range []struct{ ext, name string }{
		{".EXE;.CMD", "git.exe"},
		{".CMD;.EXE", "git.cmd"},
		{"", "git.exe"}, // Explicit lookup's default extensions.
		{".CMD;.EXE", "git.cmd"},
	} {
		for i := 0; i < 3; i++ {
			got, err := cache.resolve([]string{"PATH=ignored", "Path=" + dir, "PATHEXT=ignored", "pathext=" + tc.ext}, LookPath)
			if err != nil || got != filepath.Join(dir, tc.name) {
				t.Fatalf("PATHEXT %q: %q, %v", tc.ext, got, err)
			}
		}
	}
}
