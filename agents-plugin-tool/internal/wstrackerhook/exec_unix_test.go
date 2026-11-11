//go:build !windows

package wstrackerhook

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// alive reports a process that exists and is not a zombie.
func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return !os.IsNotExist(err)
	}
	f := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
	return len(f) == 0 || f[0] != "Z"
}

func TestRunCommandTimeoutKillsTheProcessTree(t *testing.T) {
	out := filepath.Join(t.TempDir(), "pid")
	t.Setenv(helperEnv, out)
	start := time.Now()
	err := RunCommand(context.Background(), helper(t, "tree"), t.TempDir(), nil, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout: %v", err)
	}
	if took := time.Since(start); took > 15*time.Second {
		t.Fatalf("the timed-out run took %s", took)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(data))
	deadline := time.Now().Add(5 * time.Second)
	for alive(pid) {
		if time.Now().After(deadline) {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the command's child %d outlived the timeout", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
