package wsexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
)

func changeDir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
}

func gitName() string {
	if runtime.GOOS == "windows" {
		return "git.exe"
	}
	return "git"
}

func TestGitPathCacheSharesConcurrentColdLookup(t *testing.T) {
	var cache gitPathCache
	dir := fakeProgram(t, "git")
	env := []string{"PATH=" + dir}
	calls := 0
	lookup := func(name string, env []string) (string, error) {
		calls++ // The cache must serialize lookup as well as publication.
		return LookPath(name, env)
	}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := cache.resolve(env, lookup)
			if err != nil || got != filepath.Join(dir, gitName()) {
				t.Errorf("resolve = %q, %v", got, err)
			}
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatalf("40 identical cold calls searched %d times, want 1", calls)
	}
}

func TestGitPathCacheKeysAndFailures(t *testing.T) {
	var cache gitPathCache
	a, b := fakeProgram(t, "git"), fakeProgram(t, "git")
	calls := 0
	lookup := func(name string, env []string) (string, error) {
		calls++
		return LookPath(name, env)
	}
	for _, dir := range []string{a, a, b, b, a} {
		got, err := cache.resolve([]string{"PATH=" + dir}, lookup)
		if err != nil || got != filepath.Join(dir, gitName()) {
			t.Fatalf("resolve(%q) = %q, %v", dir, got, err)
		}
	}
	if calls != 3 {
		t.Fatalf("single successful entry: %d lookups, want 3", calls)
	}
	missing := t.TempDir()
	for i := 0; i < 2; i++ {
		if _, err := cache.resolve([]string{"PATH=" + missing}, lookup); !errors.Is(err, exec.ErrNotFound) {
			t.Fatalf("missing git: %v", err)
		}
	}
	if calls != 5 {
		t.Fatalf("failures must retry: %d lookups, want 5", calls)
	}
	if err := os.WriteFile(filepath.Join(missing, gitName()), []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.resolve([]string{"PATH=" + missing}, lookup); err != nil {
		t.Fatalf("repaired git: %v", err)
	}
	// Removal does not invalidate an unchanged successful key.
	if err := os.Remove(filepath.Join(missing, gitName())); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		got, err := cache.resolve([]string{"PATH=" + missing}, lookup)
		if err != nil || got != filepath.Join(missing, gitName()) {
			t.Fatalf("stale success = %q, %v", got, err)
		}
	}
	if calls != 6 {
		t.Fatalf("unchanged success must not re-stat: %d lookups, want 6", calls)
	}
}

func TestGitPathCacheEffectiveEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("NoDefaultCurrentDirectoryInExePath", "")
	}
	var cache gitPathCache
	dir := fakeProgram(t, "git")
	t.Setenv("PATH", dir)
	t.Setenv("PATHEXT", ".EXE")
	calls := 0
	lookup := func(name string, env []string) (string, error) {
		calls++
		return LookPath(name, env)
	}
	// Nil and explicit environments stay distinct even with equal PATH values.
	for _, env := range [][]string{nil, nil, {"PATH=" + dir, "PATHEXT=.EXE"}, {"PATH=ignored", "PATH=" + dir, "PATHEXT=.EXE"}, {"PATH=" + dir, "PATHEXT=.EXE", "OTHER=changed"}} {
		if _, err := cache.resolve(env, lookup); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("effective last-entry key and separate modes: %d searches, want 2", calls)
	}
	for _, env := range [][]string{{}, {"PATH="}} {
		if _, err := cache.resolve(env, lookup); !errors.Is(err, exec.ErrNotFound) {
			t.Fatalf("explicit empty env fell back to process git: %v", err)
		}
	}
	if _, err := cache.resolve([]string{"PATH=" + dir, "PATHEXT=.EXE;.CMD"}, lookup); err != nil {
		t.Fatal(err)
	}
	if calls != 5 {
		t.Fatalf("empty env retries and PATHEXT change: %d searches, want 5", calls)
	}
	if runtime.GOOS == "windows" {
		if _, err := cache.resolve([]string{"path=ignored", "Path=" + dir, "pathext=.EXE;.CMD"}, lookup); err != nil {
			t.Fatal(err)
		}
		if calls != 5 {
			t.Fatalf("Windows keys must fold case: %d lookups", calls)
		}
	}
}

func TestGitPathCacheConcurrentCallersStayIsolated(t *testing.T) {
	var cache gitPathCache
	dirs := []string{fakeProgram(t, "git"), fakeProgram(t, "git")}
	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		dir := dirs[i%2]
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := cache.resolve([]string{"PATH=" + dir}, LookPath)
			if err != nil || got != filepath.Join(dir, gitName()) {
				t.Errorf("caller %q selected %q, %v", dir, got, err)
			}
		}()
	}
	wg.Wait()
}

func TestGitPathCacheBypassesCwdSensitiveSearch(t *testing.T) {
	root := t.TempDir()
	for _, sub := range []string{"a", "b"} {
		dir := filepath.Join(root, sub, "bin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, gitName()), []byte("fake"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var cache gitPathCache
	calls := 0
	lookup := func(name string, env []string) (string, error) {
		calls++
		return LookPath(name, env)
	}
	for _, sub := range []string{"a", "a", "b"} {
		changeDir(t, filepath.Join(root, sub))
		got, err := cache.resolve([]string{"PATH=bin"}, lookup)
		if err != nil || got != filepath.Join("bin", gitName()) {
			t.Fatalf("relative lookup changed result: %q, %v", got, err)
		}
	}
	if calls != 3 {
		t.Fatalf("relative PATH reused %d searches, want 3", calls)
	}
	for _, path := range []string{".", "bin", ""} {
		if _, ok := gitSearchContext([]string{"PATH=" + path}); ok && path != "" {
			t.Fatalf("relative PATH %q was cacheable", path)
		}
	}
	if runtime.GOOS == "windows" {
		for _, path := range []string{`C:bin`, `\bin`} {
			if _, ok := gitSearchContext([]string{"PATH=" + path}); ok {
				t.Fatalf("Windows cwd-sensitive PATH %q was cacheable", path)
			}
		}
	} else {
		t.Setenv("PATH", string(os.PathListSeparator)+root)
		if _, ok := gitSearchContext(nil); ok {
			t.Fatal("process empty PATH entry must bypass reuse")
		}
		// Explicit lookup skips empty entries, unlike Unix exec.LookPath.
		if _, ok := gitSearchContext([]string{"PATH=" + string(os.PathListSeparator) + root}); !ok {
			t.Fatal("explicit empty entries are not searched")
		}
	}
}

func TestGitCommandUsesSharedResolver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("NoDefaultCurrentDirectoryInExePath", "")
		t.Setenv("PATHEXT", ".EXE")
	}
	a, b := fakeProgram(t, "git"), fakeProgram(t, "git")
	path := a + string(os.PathListSeparator) + b
	t.Setenv("PATH", path)
	for _, env := range [][]string{nil, {"PATH=" + path, "PATHEXT=.EXE", "CUSTOM=caller"}} {
		want := filepath.Join(a, gitName())
		cmd := Command(env, "git", "arg with spaces")
		if cmd.Path != want || !reflect.DeepEqual(cmd.Env, env) || !reflect.DeepEqual(cmd.Args, []string{want, "arg with spaces"}) {
			t.Fatalf("command changed path/args/env: %+v", cmd)
		}
		if err := os.Remove(want); err != nil {
			t.Fatal(err)
		}
		cmd = CommandContext(context.Background(), env, "git", "arg with spaces")
		if cmd.Path != want {
			t.Fatalf("command did not share cached success: %q", cmd.Path)
		}
		if err := cmd.Run(); err == nil {
			t.Fatal("stale launch must fail without resolving the fallback Git")
		}
		if err := os.WriteFile(want, []byte("fake"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := Command([]string{}, "git")
	if cmd.Env == nil || !errors.Is(cmd.Err, exec.ErrNotFound) {
		t.Fatalf("explicit empty command changed mode: env=%v, err=%v", cmd.Env, cmd.Err)
	}
}

func TestGitPathCachePreservesProcessCwdPolicy(t *testing.T) {
	dir := fakeProgram(t, "git")
	changeDir(t, dir)
	t.Setenv("PATH", ".")
	var cache gitPathCache
	for _, godebug := range []string{"execerrdot=1", "execerrdot=0", "execerrdot=1"} {
		t.Setenv("GODEBUG", godebug)
		want, wantErr := exec.LookPath("git")
		got, err := cache.resolve(nil, LookPath)
		if got != want || errors.Is(err, exec.ErrDot) != errors.Is(wantErr, exec.ErrDot) || (err == nil) != (wantErr == nil) {
			t.Fatalf("%s: got %q, %v; want %q, %v", godebug, got, err, want, wantErr)
		}
	}
}
