package wstrackerhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wstracker"
)

const helperEnv = "WSTRACKERHOOK_HELPER_OUT"

// helper is the argv that runs this test binary as a fake hook command in
// mode, writing what it saw under out.
func helper(t *testing.T, mode string, args ...string) []string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return append([]string{exe, "-test.run=TestHelperProcess", "--", mode}, args...)
}

// TestHelperProcess is the fake hook command, not a test: it runs only
// with helperEnv set (the daemon's environment the command inherits).
func TestHelperProcess(t *testing.T) {
	out := os.Getenv(helperEnv)
	if out == "" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 {
		os.Exit(2)
	}
	switch args[1] {
	case "record": // the working directory, the argv after the mode, and stdin
		wd, _ := os.Getwd()
		var stdin bytes.Buffer
		_, _ = stdin.ReadFrom(os.Stdin)
		rec, _ := json.Marshal(map[string]any{"wd": wd, "args": args[2:], "stdin": stdin.String()})
		f, _ := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		fmt.Fprintf(f, "%s\n", rec)
		f.Close()
	case "fail":
		fmt.Fprintln(os.Stderr, "boom")
		os.Exit(3)
	case "tree": // starts a child that outlives it, records its pid, and hangs
		exe, _ := os.Executable()
		child := exec.Command(exe, "-test.run=TestHelperProcess", "--", "sleep")
		if err := child.Start(); err != nil {
			os.Exit(4)
		}
		_ = os.WriteFile(out, []byte(fmt.Sprint(child.Process.Pid)), 0o600)
		time.Sleep(time.Minute)
	case "sleep":
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

func TestRunCommandFailureCarriesStderr(t *testing.T) {
	t.Setenv(helperEnv, filepath.Join(t.TempDir(), "out"))
	err := RunCommand(context.Background(), helper(t, "fail"), t.TempDir(), nil, 10*time.Second)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("failure: %v", err)
	}
}

// TestRunnerRunsTheCommandWithoutAShellInTheStateDir: the default runner
// runs the argv as given (a shell would expand $HOME and split at ;) in the
// daemon state directory, with the event JSON on stdin, and a timed-out or
// failing run lets the next one run.
func TestRunnerRunsTheCommandWithoutAShellInTheStateDir(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	t.Setenv(helperEnv, out)
	w := newWorld(t)
	cmd, _ := json.Marshal(helper(t, "record", "$HOME; echo hi"))
	fail, _ := json.Marshal(helper(t, "fail"))
	w.hooks(fmt.Sprintf(`{"hooks": [{"command": %s}, {"command": %s, "scope": "all"}]}`, cmd, fail))
	w.r = newRunner(Options{Dir: w.dir, Source: w.src, Log: w.log, Now: func() time.Time { return w.now }, Location: time.UTC,
		BaseURL: func() string { return "http://127.0.0.1:7000" }})
	w.r.reload()
	w.scan()
	w.src.add("team", 1, t0, wstracker.ActAssigned, "bo", "ada")
	w.src.add("team", 2, t0, wstracker.ActAssigned, "bo", "ada")
	w.scan()
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("runs: %q", lines)
	}
	for i, line := range lines {
		var rec struct {
			WD    string   `json:"wd"`
			Args  []string `json:"args"`
			Stdin string   `json:"stdin"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		wd, _ := filepath.EvalSymlinks(rec.WD)
		dir, _ := filepath.EvalSymlinks(w.dir)
		if wd != dir || len(rec.Args) != 1 || rec.Args[0] != "$HOME; echo hi" {
			t.Fatalf("run %d: wd %q (want %q), args %q", i, rec.WD, w.dir, rec.Args)
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(rec.Stdin), &ev); err != nil {
			t.Fatalf("stdin %q: %v", rec.Stdin, err)
		}
		if ev["v"] != float64(1) || ev["id"] != fmt.Sprintf("bo.pc#%d", i+1) || ev["kind"] != "assigned" || ev["url"] == "" {
			t.Fatalf("stdin event %v", ev)
		}
		for _, k := range []string{"ts", "store", "item", "actor", "detail"} {
			if _, ok := ev[k]; !ok {
				t.Fatalf("stdin event lacks %q: %v", k, ev)
			}
		}
	}
	if n := strings.Count(w.log.String(), "failed: exit status 3: boom"); n != 2 {
		t.Fatalf("failures logged %d times:\n%s", n, w.log)
	}
}
