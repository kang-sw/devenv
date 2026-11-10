package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// TestCloseReleasesTheNudgeGoroutineAndTrackerHost pins Server.Close: the
// dashboard nudge goroutine exits (a nudge in flight when Close closes the
// queue included: the fold loop used to spin on the closed channel), a
// nudge queued afterwards is dropped without a panic, the tracker host is
// closed, and a second Close is a no-op.
func TestCloseReleasesTheNudgeGoroutineAndTrackerHost(t *testing.T) {
	t.Setenv(wsstate.EnvCacheHome, filepath.Join(t.TempDir(), "cache"))
	t.Setenv(wsconfig.EnvConfigHome, filepath.Join(t.TempDir(), "config"))
	s := NewServer(t.TempDir(), "test")
	s.tracker.opts = wstrackerhost.Options{Root: filepath.Join(t.TempDir(), "tracker")}
	host, err := s.trackerHost()
	if err != nil {
		t.Fatal(err)
	}
	d := &s.dashboard
	d.mu.Lock()
	s.queueDashboardNudge(&dashboardSession{replyID: "r1", project: "p"})
	done := d.nudgesDone
	d.mu.Unlock()
	if done == nil {
		t.Fatal("queueing a nudge started no goroutine")
	}

	s.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the nudge goroutine did not exit after Close")
	}
	d.mu.Lock()
	s.queueDashboardNudge(&dashboardSession{replyID: "r2", project: "p"})
	d.mu.Unlock()
	if _, err := host.View(context.Background(), "store", 0); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("tracker host after Close: View error = %v, want closed", err)
	}
	s.Close()
}
