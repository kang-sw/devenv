package execjob

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out))
	}
}

func gitRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustRun(t, root, "git", "init")
	mustRun(t, root, "git", "config", "user.email", "a@example.com")
	mustRun(t, root, "git", "config", "user.name", "A")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-m", "init")
	return root
}

func TestLaunchResultRawReadersAndWorkingDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell snippets")
	}
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := gitRoot(t)
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := Launch(LaunchOptions{Root: root, WorkingDir: "sub", Cmd: "/bin/sh", Args: []string{"-c", "pwd; echo errline >&2; printf 'alpha\\nbeta42\\n'"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != stateSucceeded || !strings.Contains(res.Stdout, filepath.Join(root, "sub")) || !strings.Contains(res.Stderr, "errline") {
		t.Fatalf("launch response = %#v", res)
	}
	got, err := Result(root, res.ExecKey)
	if err != nil || !strings.Contains(got.Stdout, "beta42") {
		t.Fatalf("Result = %#v, %v", got, err)
	}
	tail, err := Tail(root, res.ExecKey, "stdout", 1)
	if err != nil || tail.Text != "beta42\n" {
		t.Fatalf("Tail = %#v, %v", tail, err)
	}
	read, err := Read(root, res.ExecKey, "stderr", 0, 100)
	if err != nil || !strings.Contains(read.Text, "errline") || read.NextOffset == 0 {
		t.Fatalf("Read = %#v, %v", read, err)
	}
	grep, err := Grep(root, res.ExecKey, "stdout", `beta\d+`, 0, 0, 5, true)
	if err != nil || len(grep.Matches) != 1 {
		t.Fatalf("Grep = %#v, %v", grep, err)
	}
}

func TestLongLargeAndAbort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell snippets")
	}
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := gitRoot(t)
	long, err := Launch(LaunchOptions{Root: root, Command: "echo start; sleep 6; echo done", ShellMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if long.Status != stateRunning || long.Stdout != "" || !strings.Contains(long.Guidance, "exec.ask") {
		t.Fatalf("long launch = %#v", long)
	}
	aborted, err := Abort(root, long.ExecKey)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !aborted.ResultReady && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		aborted, _ = Status(root, long.ExecKey)
	}
	if !aborted.ResultReady {
		t.Fatalf("abort did not become terminal: %#v", aborted)
	}
	partial, err := Tail(root, long.ExecKey, "stdout", 10)
	if err != nil || !strings.Contains(partial.Text, "start") {
		t.Fatalf("partial tail = %#v, %v", partial, err)
	}

	large, err := Launch(LaunchOptions{Root: root, Command: "python3 - <<'PY'\nprint('x'*5000)\nPY", ShellMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if large.Stdout != "" || large.CombinedBytes <= InlineBudget || !strings.Contains(large.Guidance, "exec.raw.*") {
		t.Fatalf("large launch = %#v", large)
	}
	largeResult, err := Result(root, large.ExecKey)
	if err != nil {
		t.Fatal(err)
	}
	if largeResult.Stdout != "" || !strings.Contains(largeResult.Guidance, "exec.ask") {
		t.Fatalf("large result = %#v", largeResult)
	}
}
