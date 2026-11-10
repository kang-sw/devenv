package mcp

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/execjob"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// TestDrainEndsAnExecResultWaitAtOnce pins Decision 7's drain: a blocking
// exec.result (timeout_seconds) in flight when Drain starts returns the
// still-running job right away, so a daemon drain never waits out the
// call's timeout.
func TestDrainEndsAnExecResultWaitAtOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the long job is sleep(1)")
	}
	t.Setenv(wsstate.EnvCacheHome, filepath.Join(t.TempDir(), "cache"))
	root := t.TempDir()
	mustWrite(t, root, "README.md", "# Test\n")
	initGit(t, root)
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	spawn := toolText(t, keyedCall(t, server, 2, key, "exec.spawn", map[string]any{"cmd": "sleep", "args": []any{"30"}}))
	execKey := ""
	for _, line := range strings.Split(spawn, "\n") {
		if v, ok := strings.CutPrefix(line, "exec_key: "); ok {
			execKey = strings.TrimSpace(v)
		}
	}
	if execKey == "" {
		t.Fatalf("exec.spawn = %q", spawn)
	}
	defer func() { _, _ = execjob.Abort(root, execKey) }()

	drained := make(chan bool, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		drained <- server.Drain(context.Background())
	}()
	start := time.Now()
	out := toolText(t, keyedCall(t, server, 3, key, "exec.result", map[string]any{"exec_key": execKey, "timeout_seconds": 20}))
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("exec.result returned after %v; the drain should have ended its wait", elapsed)
	}
	if !strings.Contains(out, "status: running") {
		t.Fatalf("exec.result after the drain = %q", out)
	}
	if !<-drained {
		t.Fatal("Drain did not report the server idle")
	}
}
