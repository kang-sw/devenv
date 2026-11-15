package wsindex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestExecRunnerGitLookup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake Git; native executable selection covered by resolver tests")
	}
	fixture := func(label string) (string, string) {
		dir := t.TempDir()
		path := filepath.Join(dir, "git")
		script := "#!/bin/sh\nif [ \"$3\" = wait ]; then exec /bin/sleep 30; fi\nprintf '" + label + ":%s:%s:%s:%s\\n' \"$LOOKUP_VALUE\" \"$1\" \"$2\" \"$3\"\nIFS= read -r line\nprintf '%s\\n' \"$line\"\nprintf 'stderr\\n' >&2\nif [ \"$3\" = fail ]; then exit 7; fi\n"
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
		return dir, path
	}
	a, first := fixture("A")
	b, _ := fixture("B")
	t.Setenv("PATH", a)
	t.Setenv("LOOKUP_VALUE", "process")
	root := t.TempDir()
	command := Command{Args: []string{"arg with spaces"}, Stdin: []byte("input with spaces\n")}
	check := func(r ExecRunner, label, value string) {
		t.Helper()
		out, err := r.Run(context.Background(), root, command)
		want := label + ":" + value + ":-C:" + root + ":arg with spaces\ninput with spaces\n"
		if err != nil || string(out) != want {
			t.Fatalf("output %q, error %v; want %q", out, err, want)
		}
	}
	for i := 0; i < 3; i++ {
		check(ExecRunner{}, "A", "process")
	}
	env := []string{"PATH=" + a, "LOOKUP_VALUE=caller"}
	r := ExecRunner{Environ: func() []string { return env }}
	command.Env = []string{"LOOKUP_VALUE=override"}
	for _, dir := range []string{a, b, a, b, a} {
		env = []string{"PATH=" + dir, "LOOKUP_VALUE=caller"}
		label := "A"
		if dir == b {
			label = "B"
		}
		check(r, label, "override")
	}
	command.Env = nil
	check(ExecRunner{Environ: func() []string { return nil }}, "A", "process")
	for _, missing := range [][]string{{}, {"PATH=" + t.TempDir()}} {
		env = missing
		if _, err := r.Run(context.Background(), root, command); !errors.Is(err, exec.ErrNotFound) {
			t.Fatalf("caller missing Git: %v", err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (ExecRunner{}).Run(context.Background(), root, command); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("process missing Git: %v", err)
	}
	t.Setenv("PATH", a)
	_, err := (ExecRunner{}).Run(context.Background(), root, Command{Args: []string{"fail"}, Stdin: []byte("input\n")})
	var ge *GitError
	if !errors.As(err, &ge) || ge.Stderr != "stderr\n" || ge.Stdout != "A:process:-C:"+root+":fail\ninput\n" {
		t.Fatalf("separate failure streams: %#v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := (ExecRunner{}).Run(ctx, root, Command{Args: []string{"wait"}}); !errors.As(err, &ge) || !ge.TimedOut {
		t.Fatalf("deadline: %v", err)
	}
	t.Setenv("PATH", a+string(os.PathListSeparator)+b)
	check(ExecRunner{}, "A", "process")
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if out, err := (ExecRunner{}).Run(context.Background(), root, command); err == nil || len(out) != 0 {
			t.Fatalf("stale launch re-resolved: %q, %v", out, err)
		}
	}
}
