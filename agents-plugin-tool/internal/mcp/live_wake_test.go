package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestDashboardIntakeWakeAndTaskCounts(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	out := intake(t, f.server, f.keyA, map[string]any{"kind": "wake", "cause": "monitor", "source": "watch build", "caption": "changed", "text": "full output"})
	if !strings.Contains(out, "wake, task_counts") || !strings.Contains(out, "ok: 1 of 1") {
		t.Fatalf("intake = %q", out)
	}
	waitFor(t, "wake ring", func() bool {
		ring, err := wslive.ReadRing(wslive.RingDir(f.sessionDir(f.replyA)))
		return err == nil && len(ring) == 1 && ring[0].Kind == wslive.RingWake && ring[0].Cause == "monitor" && ring[0].Text == "full output"
	})
	intake(t, f.server, f.keyA, map[string]any{"kind": "status", "background": 2, "monitors": 0})
	waitFor(t, "task count snapshot", func() bool {
		snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(f.sessionDir(f.replyA)))
		return err == nil && snap.Usage != nil && snap.Usage.Background != nil && *snap.Usage.Background == 2 && snap.Usage.Monitors != nil && *snap.Usage.Monitors == 0
	})
}
