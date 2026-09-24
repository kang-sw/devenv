package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
)

func TestWorktreeAcquireWritesLeaseAndReleaseRemovesIt(t *testing.T) {
	s, _, base, leadKey := worktreeListServer(t)
	before := time.Now().Add(-time.Second)
	acq := acquireForTest(t, s, 1, leadKey, base, "impl/test/alpha")

	lease, err := readWorktreeLease(context.Background(), wsgit.ExecRunner{}, acq.Path)
	if err != nil || lease == nil {
		t.Fatalf("acquire must write a worktree lease: lease=%v err=%v", lease, err)
	}
	if lease.SchemaVersion != worktreeLeaseSchemaVersion || lease.WorkerKey != acq.WorkerKey || lease.ParentKey != leadKey {
		t.Fatalf("lease = %+v, want worker_key %q and parent_key %q", lease, acq.WorkerKey, leadKey)
	}
	at, err := time.Parse(time.RFC3339, lease.AcquiredAt)
	if err != nil || at.Before(before.Truncate(time.Second)) || !strings.HasSuffix(lease.AcquiredAt, "Z") {
		t.Fatalf("acquired_at = %q (err=%v), want a current RFC 3339 UTC time", lease.AcquiredAt, err)
	}
	// The lease lives in the Git admin dir, not the working tree, so it never
	// shows in status and survives the hygiene clean.
	adminDir := strings.TrimSpace(string(runGitOutput(t, acq.Path, "rev-parse", "--absolute-git-dir")))
	if _, err := os.Stat(filepath.Join(adminDir, worktreeLeaseFileName)); err != nil {
		t.Fatalf("lease file not in the git admin dir %q: %v", adminDir, err)
	}
	if st := strings.TrimSpace(string(runGitOutput(t, acq.Path, "status", "--porcelain"))); st != "" {
		t.Fatalf("lease must not dirty the worktree: %q", st)
	}

	rel := callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": leadKey, "key": acq.WorkerKey})
	if toolIsError(t, rel) {
		t.Fatalf("release failed: %s", rel)
	}
	if lease, err := readWorktreeLease(context.Background(), wsgit.ExecRunner{}, acq.Path); err != nil || lease != nil {
		t.Fatalf("release must remove the worktree lease: lease=%v err=%v", lease, err)
	}
	// The worker key is not retired by release.
	if _, ok := s.sessions.lookup(acq.WorkerKey); !ok {
		t.Fatal("release must not retire the worker key")
	}

	// Reacquiring the released worktree writes a fresh lease for the new key.
	acq2 := acquireForTest(t, s, 3, leadKey, base, "impl/test/beta")
	if acq2.Path != acq.Path {
		t.Fatalf("expected pool reuse of %q, got %q", acq.Path, acq2.Path)
	}
	lease2, err := readWorktreeLease(context.Background(), wsgit.ExecRunner{}, acq2.Path)
	if err != nil || lease2 == nil || lease2.WorkerKey != acq2.WorkerKey {
		t.Fatalf("reacquire must record the new holder: lease=%+v err=%v", lease2, err)
	}
}

func TestReleaseWorktreeToleratesMissingLease(t *testing.T) {
	root, base := worktreeFixture(t)
	// provisionWorktree alone writes no lease: the shape of a worktree acquired
	// before worktree leases existed.
	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if lease, err := readWorktreeLease(context.Background(), wsgit.ExecRunner{}, res.Path); err != nil || lease != nil {
		t.Fatalf("precondition: no lease expected, got %v err=%v", lease, err)
	}
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, res.Path, ""); err != nil {
		t.Fatalf("release without a lease file must succeed: %v", err)
	}
	if b := wtBranch(t, res.Path); b != "HEAD" {
		t.Fatalf("released worktree not detached: %q", b)
	}
}

func TestWorktreeAcquireFailedLeaseWriteDetaches(t *testing.T) {
	s, _, base, leadKey := worktreeListServer(t)
	acq := acquireForTest(t, s, 1, leadKey, base, "impl/test/alpha")
	if rel := callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": leadKey, "path": acq.Path}); toolIsError(t, rel) {
		t.Fatalf("release failed: %s", rel)
	}
	// Occupy the lease path with a non-empty directory so the atomic rename
	// onto it fails on the next acquire, which reuses this released worktree.
	adminDir := strings.TrimSpace(string(runGitOutput(t, acq.Path, "rev-parse", "--absolute-git-dir")))
	blocker := filepath.Join(adminDir, worktreeLeaseFileName)
	if err := os.MkdirAll(filepath.Join(blocker, "x"), 0o755); err != nil {
		t.Fatal(err)
	}

	resp := callToolOnce(t, s, 3, "worktree.acquire", map[string]any{
		"session_key": leadKey, "base": base, "target_branch": "impl/test/beta",
	})
	if !toolIsError(t, resp) {
		t.Fatalf("acquire must fail when the worktree lease cannot be written: %s", resp)
	}
	if !strings.Contains(toolText(t, resp), "worktree lease") {
		t.Fatalf("failure must name the worktree lease: %s", resp)
	}
	if b := wtBranch(t, acq.Path); b != "HEAD" {
		t.Fatalf("failed lease write must leave the worktree detached, branch = %q", b)
	}
}

func TestWriteWorktreeLeaseOverwritesUnreleasedLease(t *testing.T) {
	root, base := worktreeFixture(t)
	ctx := context.Background()
	res, err := provisionWorktree(ctx, wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	// A worktree detached outside ws keeps its stale lease; the next acquire's
	// write must replace it, not fail or merge with it.
	if err := writeWorktreeLease(ctx, wsgit.ExecRunner{}, res.Path, worktreeLease{WorkerKey: "old-worker", ParentKey: "old-lead", AcquiredAt: "2020-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := writeWorktreeLease(ctx, wsgit.ExecRunner{}, res.Path, worktreeLease{WorkerKey: "new-worker", ParentKey: "new-lead", AcquiredAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("overwriting an existing lease must succeed: %v", err)
	}
	lease, err := readWorktreeLease(ctx, wsgit.ExecRunner{}, res.Path)
	if err != nil || lease == nil || lease.WorkerKey != "new-worker" || lease.ParentKey != "new-lead" || lease.AcquiredAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("lease after overwrite = %+v err=%v, want the new holder", lease, err)
	}
}
