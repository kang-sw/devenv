//go:build !windows

package wsdashboard

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestPlanUsageAgyDaemonShutdownKillsInFlightProbe(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	dir := filepath.Join(root, "state")
	binDir := filepath.Join(root, "bin")
	for _, p := range []string{home, dir, binDir} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pidFile := filepath.Join(root, "agy.pid")
	now := time.Now().UTC()
	payload := validAgyFixtureJSON(now.Add(3*time.Hour).Format(time.RFC3339), now.Add(4*24*time.Hour).Format(time.RFC3339))
	// Fake agy records its PID atomically, then blocks far longer than daemon
	// shutdown; if not killed it would emit valid usage JSON.
	script := fmt.Sprintf(`#!/bin/sh
tmp=%s
printf '%%d\n' "$$" > "$tmp"
mv "$tmp" %s
sleep 30
cat <<'EOF'
%s
EOF
`, shellQuote(pidFile+".tmp"), shellQuote(pidFile), payload)
	if err := os.WriteFile(filepath.Join(binDir, "agy"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/usr/bin:/bin")

	var probeStillRunningAtUnlock atomic.Bool
	var d *Daemon
	var err error
	d, err = StartDaemon(DaemonOptions{
		Dir:        dir,
		Home:       home,
		noGitBadge: true,
		AgyCommand: func() (string, []string, error) {
			p, lookErr := exec.LookPath("agy")
			return p, nil, lookErr
		},
		BeforeUnlock: func() {
			d.planUsage.mu.Lock()
			probeStillRunningAtUnlock.Store(d.planUsage.agyProbing)
			d.planUsage.mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	defer d.Close()

	var writeCount atomic.Int32
	d.planUsage.writeFile = func(dir, name string, data []byte) error {
		writeCount.Add(1)
		return writePrivateFile(dir, name, data)
	}

	req := httptest.NewRequest("GET", "/rail/usage?refresh=1", nil)
	req.Header.Set("X-WS-Usage-Refresh", "1")
	w := httptest.NewRecorder()
	d.handlePlanUsage(w, req)
	if w.Code != 200 {
		t.Fatalf("forced /rail/usage status = %d", w.Code)
	}

	// Wait until the fake agy subprocess is running and has published its PID.
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(pidFile); err == nil {
			if parsed, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil && parsed > 0 {
				pid = parsed
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("fake agy probe never started")
	}

	closeStart := time.Now()
	d.Close()
	if elapsed := time.Since(closeStart); elapsed > 5*time.Second {
		t.Fatalf("daemon Close did not bound in-flight agy probe: took %s", elapsed)
	}
	if probeStillRunningAtUnlock.Load() {
		t.Fatal("in-flight agy probe had not finished before daemon released its lock")
	}

	// Subprocess must be dead after shutdown returns.
	killDeadline := time.Now().Add(500 * time.Millisecond)
	alive := true
	for time.Now().Before(killDeadline) {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			alive = false
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if alive {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("fake agy process %d still alive after daemon Close", pid)
	}

	// No store file may be written during or after shutdown, and a late refresh
	// after Close must be a no-op.
	d.planUsage.refresh(context.Background(), true)
	d.planUsage.probes.Wait()
	if got := writeCount.Load(); got != 0 {
		t.Fatalf("cancelled probe performed %d store writes, want 0", got)
	}
	for _, name := range []string{planUsageAgyCache, planUsageAgyHistoryCache} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("cancelled probe created %s", name)
		}
	}
}
