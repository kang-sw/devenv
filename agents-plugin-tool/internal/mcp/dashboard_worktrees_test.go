package mcp

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// TestDashboardReadWorktrees reads a pool holding one leased and one
// released (idle, detached) worktree through the dashboard session source,
// as the daemon does: the session's resolved pool, both worktrees with
// their profile and branch state, the lease's keys as reply-ids only, and
// the activity times.
func TestDashboardReadWorktrees(t *testing.T) {
	s, root, base, leadKey := worktreeListServer(t)
	before := time.Now().Add(-time.Second)
	busy := acquireForTest(t, s, 1, leadKey, base, "impl/test/busy")
	free := acquireForTest(t, s, 2, leadKey, base, "impl/test/free")
	if rel := callToolOnce(t, s, 3, "worktree.release", map[string]any{"session_key": leadKey, "key": free.WorkerKey}); toolIsError(t, rel) {
		t.Fatalf("release: %s", rel)
	}

	canon := canonicalRootForTest(t, root)
	reg := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID(leadKey), Project: canon}
	pool, err := NewDashboardSessionSource().ReadWorktrees(reg)
	if err != nil {
		t.Fatalf("ReadWorktrees: %v", err)
	}
	if want := resolvePoolRoot("", canon); pool.Pool != want {
		t.Errorf("pool = %q, want %q", pool.Pool, want)
	}
	if len(pool.Worktrees) != 2 {
		t.Fatalf("worktrees = %+v, want two", pool.Worktrees)
	}
	byPath := map[string]wsdashboard.PooledWorktree{}
	for _, wt := range pool.Worktrees {
		byPath[wt.Path] = wt
	}
	leased, idle := byPath[busy.Path], byPath[free.Path]
	if leased.Name != filepath.Base(busy.Path) || leased.Profile != "full" || leased.Branch != "impl/test/busy" || leased.Detached || !leased.Leased {
		t.Errorf("leased worktree = %+v", leased)
	}
	if leased.Holder != wsmailbox.ReplyID(busy.WorkerKey) || leased.Parent != wsmailbox.ReplyID(leadKey) {
		t.Errorf("lease holders = %q/%q, want the keys' reply-ids", leased.Holder, leased.Parent)
	}
	if leased.AcquiredAt < before.Truncate(time.Second).UnixMilli() || leased.ActiveAt < leased.AcquiredAt {
		t.Errorf("leased times = acquired %d, active %d", leased.AcquiredAt, leased.ActiveAt)
	}
	if idle.Profile != "full" || !idle.Detached || idle.Branch != "" || idle.Leased || idle.Holder != "" || idle.ActiveAt == 0 {
		t.Errorf("idle worktree = %+v", idle)
	}
	for _, wt := range pool.Worktrees {
		for _, key := range []string{leadKey, busy.WorkerKey, free.WorkerKey} {
			if strings.Contains(wt.Holder+wt.Parent+wt.Path+wt.Name, key) {
				t.Errorf("worktree %+v carries session key %q", wt, key)
			}
		}
	}

	// A registration no session record backs reads nothing.
	if _, err := NewDashboardSessionSource().ReadWorktrees(wsdashboard.Registration{ReplyID: "nobody", Project: canon}); err == nil {
		t.Error("ReadWorktrees of an unknown session succeeded")
	}
}
