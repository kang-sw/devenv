package mcp

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsgit"
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

// TestDashboardReadRepoWorktrees lists a repository for the session rail:
// every worktree git lists, the primary first, with its short branch or
// detached state, and the pool roots its session's worktree_pool owns; a
// registration no record backs falls back to the default pool value, and a
// folder outside git fails.
func TestDashboardReadRepoWorktrees(t *testing.T) {
	s, root, base, leadKey := worktreeListServer(t)
	busy := acquireForTest(t, s, 1, leadKey, base, "impl/test/busy")
	free := acquireForTest(t, s, 2, leadKey, base, "impl/test/free")
	if rel := callToolOnce(t, s, 3, "worktree.release", map[string]any{"session_key": leadKey, "key": free.WorkerKey}); toolIsError(t, rel) {
		t.Fatalf("release: %s", rel)
	}
	canon := canonicalRootForTest(t, root)
	src := NewDashboardSessionSource()
	for _, regs := range [][]wsdashboard.Registration{
		{{ReplyID: wsmailbox.ReplyID(leadKey), Project: canon}},
		{{ReplyID: "nobody", Project: canon}},
	} {
		got, err := src.ReadRepoWorktrees(canon, regs)
		if err != nil {
			t.Fatalf("ReadRepoWorktrees: %v", err)
		}
		if len(got.Worktrees) != 3 || got.Worktrees[0].Path != canon {
			t.Fatalf("worktrees = %+v, want the primary first and two pooled", got.Worktrees)
		}
		byPath := map[string]wsdashboard.RepoWorktree{}
		for _, wt := range got.Worktrees {
			byPath[wt.Path] = wt
		}
		if wt := byPath[busy.Path]; wt.Branch != "impl/test/busy" || wt.Detached || wt.Prunable {
			t.Errorf("leased worktree = %+v", wt)
		}
		if wt := byPath[free.Path]; wt.Branch != "" || !wt.Detached {
			t.Errorf("released worktree = %+v", wt)
		}
		pool := resolvePoolRoot("", canon)
		found := false
		for _, p := range got.Pools {
			found = found || p == pool
		}
		if !found {
			t.Errorf("pools = %v, want %q among them", got.Pools, pool)
		}
	}
	if _, err := src.ReadRepoWorktrees(t.TempDir(), nil); err == nil {
		t.Error("a folder outside git listed worktrees")
	}
}

// TestDashboardCommonDir pins the registration's git common directory: a
// worktree's is its repository's .git, absolute; outside git it is empty.
func TestDashboardCommonDir(t *testing.T) {
	s, root, base, leadKey := worktreeListServer(t)
	busy := acquireForTest(t, s, 1, leadKey, base, "impl/test/busy")
	canon := canonicalRootForTest(t, root)
	runner := wsgit.ExecRunner{}
	for _, dir := range []string{canon, busy.Path} {
		got := dashboardCommonDir(runner, dir)
		if real, err := filepath.EvalSymlinks(got); err != nil || real != filepath.Join(canon, ".git") {
			t.Errorf("common dir of %s = %q, want %s", dir, got, filepath.Join(canon, ".git"))
		}
	}
	if got := dashboardCommonDir(runner, t.TempDir()); got != "" {
		t.Errorf("common dir outside git = %q", got)
	}
}
