package execjob

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstore"
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
	if _, err := Launch(LaunchOptions{Root: root, WorkingDir: "../escape", Cmd: "/bin/sh", Args: []string{"-c", "true"}}); err == nil {
		t.Fatal("relative working_dir escaped worktree root")
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
	if aborted.Status != stateCancelled {
		t.Fatalf("abort did not become cancelled: %#v", aborted)
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

func TestReconcileLostRunningWorker(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := gitRoot(t)
	key := "exec-1-0000000000000001"
	dir, err := jobDir(root, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdout"), []byte("partial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stderr"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "combined"), []byte("partial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := ts()
	if err := writeRecord(root, Record{SchemaVersion: schemaVersion, ExecKey: key, Status: stateRunning, Root: root, WorkingDir: root, PID: 999999999, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	got, err := Status(root, key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != stateFailed || !strings.Contains(got.Error, "worker is no longer active") || got.StdoutBytes == 0 {
		t.Fatalf("reconciled status = %#v", got)
	}
}

func TestSQLiteMetadataSurvivesRestartAndNoStateJSONAuthority(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := gitRoot(t)
	res, err := Launch(LaunchOptions{Root: root, Cmd: os.Args[0], Args: []string{"-test.run=TestHelperProcess", "--", "execjob-sqlite"}, Env: map[string]string{"GO_WANT_HELPER_PROCESS": "1"}, Stdin: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != stateSucceeded || !strings.Contains(res.Stdout, "execjob-sqlite") {
		t.Fatalf("launch = %#v", res)
	}
	state, err := statePath(root, res.ExecKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("state.json write authority still present, stat err=%v", err)
	}
	store, err := wsstore.NewManager(wsstore.Options{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	job, ok, err := store.ExecJob(context.Background(), res.ExecKey)
	store.Close()
	if err != nil || !ok {
		t.Fatalf("sqlite exec job ok=%t err=%v", ok, err)
	}
	if job.EnvJSON == "" || !job.StdinPresent || job.StdinBytes != 3 || job.StdoutPath == "" || job.CleanupState != "completed" {
		t.Fatalf("stored job = %#v", job)
	}
	active.Delete(activeKey(root, res.ExecKey))
	got, err := Result(root, res.ExecKey)
	if err != nil || !strings.Contains(got.Stdout, "execjob-sqlite") {
		t.Fatalf("result after restart = %#v err=%v", got, err)
	}
}

func TestShellLaunchUsesSQLiteMetadataAndNoStateJSON(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := gitRoot(t)
	res, err := Launch(LaunchOptions{Root: root, Command: "echo shell-sqlite", ShellMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != stateSucceeded || !strings.Contains(res.Stdout, "shell-sqlite") {
		t.Fatalf("shell launch = %#v", res)
	}
	state, err := statePath(root, res.ExecKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("shell state.json write authority present, stat err=%v", err)
	}
	store, err := wsstore.NewManager(wsstore.Options{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	job, ok, err := store.ExecJob(context.Background(), res.ExecKey)
	store.Close()
	if err != nil || !ok {
		t.Fatalf("shell sqlite job ok=%t err=%v", ok, err)
	}
	if job.Command == "" || job.Shell == "" || job.StdoutPath == "" {
		t.Fatalf("shell stored job = %#v", job)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			_, _ = os.Stdout.WriteString(args[i+1] + "\n")
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestLegacyFileBackedImportAndCorruptRecovery(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := gitRoot(t)
	key := "exec-1-00000000000000aa"
	now := ts()
	rec := Record{SchemaVersion: schemaVersion, ExecKey: key, Status: stateSucceeded, Root: root, WorkingDir: root, StartedAt: now, UpdatedAt: now, CompletedAt: now, StdoutBytes: 7, CombinedBytes: 7}
	writeLegacyState(t, root, key, rec, []byte("legacy\n"), nil, []byte("legacy\n"))
	got, err := Result(root, key)
	if err != nil || got.Stdout != "legacy\n" {
		t.Fatalf("legacy result = %#v err=%v", got, err)
	}
	store, err := wsstore.NewManager(wsstore.Options{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err := store.ExecJob(context.Background(), key)
	store.Close()
	if err != nil || !ok {
		t.Fatalf("legacy not imported ok=%t err=%v", ok, err)
	}

	badKey := "exec-1-00000000000000bb"
	badDir, err := jobDir(root, badKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "state.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad, err := Status(root, badKey)
	if err != nil || bad.Status != stateFailed || !strings.Contains(bad.Error, "cannot be migrated") {
		t.Fatalf("corrupt legacy status = %#v err=%v", bad, err)
	}
}

func TestMissingPayloadReportedAsRecoverableConsistencyState(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := gitRoot(t)
	cases := []struct {
		stream string
		arg    string
	}{
		{"stdout", "payload"},
		{"stderr", "stderr-payload"},
		{"combined", "payload"},
	}
	for _, tc := range cases {
		t.Run(tc.stream, func(t *testing.T) {
			res, err := Launch(LaunchOptions{Root: root, Cmd: os.Args[0], Args: []string{"-test.run=TestHelperProcess", "--", tc.arg}, Env: map[string]string{"GO_WANT_HELPER_PROCESS": "1"}})
			if err != nil {
				t.Fatal(err)
			}
			p, _, err := streamPathFromStore(root, res.ExecKey, tc.stream)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			status, err := Status(root, res.ExecKey)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(status.Error, "recoverable consistency state") || !strings.Contains(status.Error, tc.stream) {
				t.Fatalf("missing payload status = %#v", status)
			}
			got, err := Result(root, res.ExecKey)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got.Error, "recoverable consistency state") || !strings.Contains(got.Error, tc.stream) {
				t.Fatalf("missing payload result = %#v", got)
			}
			if _, err := Tail(root, res.ExecKey, tc.stream, 1); err == nil || !strings.Contains(err.Error(), "recoverable consistency state") {
				t.Fatalf("Tail missing %s err=%v", tc.stream, err)
			}
			if _, err := Read(root, res.ExecKey, tc.stream, 0, 100); err == nil || !strings.Contains(err.Error(), "recoverable consistency state") {
				t.Fatalf("Read missing %s err=%v", tc.stream, err)
			}
			if _, err := Grep(root, res.ExecKey, tc.stream, "payload", 0, 0, 5, false); err == nil || !strings.Contains(err.Error(), "recoverable consistency state") {
				t.Fatalf("Grep missing %s err=%v", tc.stream, err)
			}
		})
	}
}

func writeLegacyState(t *testing.T, root, key string, rec Record, stdout, stderr, combined []byte) {
	t.Helper()
	dir, err := jobDir(root, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"stdout": stdout, "stderr": stderr, "combined": combined} {
		if data == nil {
			data = []byte{}
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
