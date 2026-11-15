package wsdashboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestDashboardGitLookupConstruction(t *testing.T) {
	t.Setenv("NoDefaultCurrentDirectoryInExePath", "1")
	t.Setenv("PATHEXT", ".EXE")
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	root := t.TempDir()
	for i := 0; i < 2; i++ {
		dir := t.TempDir()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("fake"), 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		for j := 0; j < 3; j++ {
			cmd := gitActionCommand(context.Background(), root, "status", "arg with spaces")
			if cmd.Path != path || !reflect.DeepEqual(cmd.Args, []string{path, "-C", root, "status", "arg with spaces"}) {
				t.Fatalf("command: %q %q", cmd.Path, cmd.Args)
			}
			if cmd.Cancel == nil || cmd.WaitDelay <= 0 {
				t.Fatal("command bounds missing")
			}
			found := false
			for _, entry := range cmd.Env {
				if entry == "LC_ALL=C" {
					found = true
				}
			}
			if !found {
				t.Fatal("C locale missing")
			}
		}
	}
}

func TestDashboardGitLookupExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX script execution; construction tested separately")
	}
	fixture := func(label string) (string, string) {
		dir := t.TempDir()
		path := filepath.Join(dir, "git")
		script := "#!/bin/sh\nprintf '" + label + ":%s:%s:%s:%s:%s\\n' \"$LC_ALL\" \"$LOOKUP_VALUE\" \"$1\" \"$2\" \"$3\"\nprintf 'stderr\\n' >&2\nif [ \"$3\" = fail ]; then exit 7; fi\n"
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
		return dir, path
	}
	a, first := fixture("A")
	b, _ := fixture("B")
	root := t.TempDir()
	t.Setenv("LOOKUP_VALUE", "process")
	for _, dir := range []string{a, a, b, a} {
		t.Setenv("PATH", dir)
		label := "A"
		if dir == b {
			label = "B"
		}
		want := label + ":C:process:-C:" + root + ":arg with spaces\n"
		out, cut, err := runGitCapped(context.Background(), root, 4096, "arg with spaces")
		if err != nil || cut || string(out) != want {
			t.Fatalf("capped output %q, cut %v, error %v", out, cut, err)
		}
		out, err = gitActionCommand(context.Background(), root, "arg with spaces").Output()
		if err != nil || string(out) != want {
			t.Fatalf("action output %q, error %v", out, err)
		}
	}
	out, cut, err := runGitCapped(context.Background(), root, 3, "arg")
	if err != nil || !cut || string(out) != "A:C" {
		t.Fatalf("output bound: %q, %v, %v", out, cut, err)
	}
	if _, _, err := runGitCapped(context.Background(), root, 4096, "fail"); err == nil || err.Error() != "stderr" {
		t.Fatalf("stderr error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := runGitCapped(ctx, root, 4096, "arg"); !errors.Is(err, context.Canceled) {
		t.Fatalf("capped cancellation: %v", err)
	}
	if err := gitActionCommand(ctx, root, "arg").Run(); !errors.Is(err, context.Canceled) {
		t.Fatalf("action cancellation: %v", err)
	}
	t.Setenv("PATH", a+string(os.PathListSeparator)+b)
	if _, _, err := runGitCapped(context.Background(), root, 4096, "arg"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGitCapped(context.Background(), root, 4096, "arg"); err == nil {
		t.Fatal("capped launch re-resolved stale success")
	}
	cmd := gitActionCommand(context.Background(), root, "arg")
	if cmd.Path != first {
		t.Fatalf("action re-resolved stale success: %q", cmd.Path)
	}
	if err := cmd.Run(); err == nil {
		t.Fatal("stale action unexpectedly ran")
	}
}
