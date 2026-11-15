package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// releaseGuardServer is a worktree server with a second lead key on the same
// root, the shape of two lead sessions sharing one worktree pool.
func releaseGuardServer(t *testing.T) (s *Server, base, leadA, leadB string) {
	t.Helper()
	s, root, base, leadA := worktreeListServer(t)
	leadB, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	return s, base, leadA, leadB
}

// writeDraft leaves an uncommitted file in a worktree and returns its path.
func writeDraft(t *testing.T, wt string) string {
	t.Helper()
	draft := filepath.Join(wt, "draft.md")
	if err := os.WriteFile(draft, []byte("uncommitted work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return draft
}

// TestWorktreeReleaseRefusesAnotherHoldersLease: a lead releases a pooled
// worktree, another lead reacquires it and leaves uncommitted work, and the
// first lead's stale second release (by path, or by its old worker key) is
// refused with nothing changed; the new holder's own release works.
func TestWorktreeReleaseRefusesAnotherHoldersLease(t *testing.T) {
	s, base, leadA, leadB := releaseGuardServer(t)
	first := acquireForTest(t, s, 1, leadA, base, "impl/test/alpha")
	if rel := callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": leadA, "path": first.Path}); toolIsError(t, rel) {
		t.Fatalf("owner release failed: %s", rel)
	}
	second := acquireForTest(t, s, 3, leadB, base, "impl/test/beta")
	if second.Path != first.Path {
		t.Fatalf("expected pool reuse of %q, got %q", first.Path, second.Path)
	}
	draft := writeDraft(t, second.Path)

	for i, args := range []map[string]any{
		{"session_key": leadA, "path": first.Path},
		{"session_key": leadA, "key": first.WorkerKey},
	} {
		rel := callToolOnce(t, s, 10+i, "worktree.release", args)
		if !toolIsError(t, rel) {
			t.Fatalf("a stale release %v must be refused: %s", args, rel)
		}
		text := toolText(t, rel)
		for _, want := range []string{second.WorkerKey, leadB, "Nothing was changed", "Cleaning up a worktree whose owner is gone: pass force: true"} {
			if !strings.Contains(text, want) {
				t.Fatalf("refusal lacks %q: %s", want, text)
			}
		}
		if _, err := os.Stat(draft); err != nil {
			t.Fatalf("a refused release must keep the holder's uncommitted file: %v", err)
		}
		if b := wtBranch(t, second.Path); b != "impl/test/beta" {
			t.Fatalf("a refused release must keep the holder's branch, got %q", b)
		}
		lease, err := readWorktreeLease(context.Background(), wsgit.ExecRunner{}, second.Path)
		if err != nil || lease == nil || lease.WorkerKey != second.WorkerKey {
			t.Fatalf("a refused release must keep the lease: %+v err=%v", lease, err)
		}
	}

	if rel := callToolOnce(t, s, 20, "worktree.release", map[string]any{"session_key": leadB, "path": second.Path}); toolIsError(t, rel) {
		t.Fatalf("the leaseholder's own release failed: %s", rel)
	}
	if b := wtBranch(t, second.Path); b != "HEAD" {
		t.Fatalf("released worktree not detached: %q", b)
	}
}

// TestWorktreeReleaseByTheLeasesWorkerKey: a lead other than the acquirer
// releases by the lease's current worker key.
func TestWorktreeReleaseByTheLeasesWorkerKey(t *testing.T) {
	s, base, leadA, leadB := releaseGuardServer(t)
	acq := acquireForTest(t, s, 1, leadB, base, "impl/test/alpha")
	if rel := callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": leadA, "key": acq.WorkerKey}); toolIsError(t, rel) {
		t.Fatalf("release by the lease's worker key failed: %s", rel)
	}
	if b := wtBranch(t, acq.Path); b != "HEAD" {
		t.Fatalf("released worktree not detached: %q", b)
	}
}

// TestWorktreeReleaseForceOverridesAnotherHoldersLease: force releases a
// worktree leased to another holder, discarding its uncommitted work.
func TestWorktreeReleaseForceOverridesAnotherHoldersLease(t *testing.T) {
	s, base, leadA, leadB := releaseGuardServer(t)
	acq := acquireForTest(t, s, 1, leadB, base, "impl/test/alpha")
	draft := writeDraft(t, acq.Path)
	if rel := callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": leadA, "path": acq.Path, "force": true}); toolIsError(t, rel) {
		t.Fatalf("forced release failed: %s", rel)
	}
	if _, err := os.Stat(draft); !os.IsNotExist(err) {
		t.Fatalf("a forced release cleans the tree, but the draft is still there: %v", err)
	}
	if lease, err := readWorktreeLease(context.Background(), wsgit.ExecRunner{}, acq.Path); err != nil || lease != nil {
		t.Fatalf("a forced release removes the lease: %+v err=%v", lease, err)
	}
}

// TestWorktreeReleaseWithoutALeaseRefusesOnlyADirtyTree: a worktree whose
// lease is gone (a crashed or pre-lease holder) has an unknown holder, so its
// uncommitted work is kept unless force is passed; a clean one releases.
func TestWorktreeReleaseWithoutALeaseRefusesOnlyADirtyTree(t *testing.T) {
	s, base, leadA, _ := releaseGuardServer(t)
	acq := acquireForTest(t, s, 1, leadA, base, "impl/test/alpha")
	if err := removeWorktreeLease(context.Background(), wsgit.ExecRunner{}, acq.Path); err != nil {
		t.Fatal(err)
	}
	draft := writeDraft(t, acq.Path)
	rel := callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": leadA, "path": acq.Path})
	if !toolIsError(t, rel) {
		t.Fatalf("release of a dirty unleased worktree must be refused: %s", rel)
	}
	if text := toolText(t, rel); !strings.Contains(text, "no worktree lease") || !strings.Contains(text, "draft.md") || !strings.Contains(text, "Cleaning up a worktree whose owner is gone: pass force: true") {
		t.Fatalf("refusal must name the missing lease, the dirty path, and force: %s", text)
	}
	if _, err := os.Stat(draft); err != nil {
		t.Fatalf("a refused release must keep the uncommitted file: %v", err)
	}
	if rel := callToolOnce(t, s, 3, "worktree.release", map[string]any{"session_key": leadA, "path": acq.Path, "force": true}); toolIsError(t, rel) {
		t.Fatalf("forced release failed: %s", rel)
	}
	// Released: clean, detached, and unleased; releasing it again is allowed.
	if rel := callToolOnce(t, s, 4, "worktree.release", map[string]any{"session_key": leadA, "path": acq.Path}); toolIsError(t, rel) {
		t.Fatalf("release of a clean unleased worktree failed: %s", rel)
	}
}

// TestWorktreeReleaseByTheHoldersOwnSessionKey: the worker key an acquire
// minted is a session key of its own, and releasing by path under it is the
// holder's own release.
func TestWorktreeReleaseByTheHoldersOwnSessionKey(t *testing.T) {
	s, base, leadA, _ := releaseGuardServer(t)
	acq := acquireForTest(t, s, 1, leadA, base, "impl/test/alpha")
	if rel := callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": acq.WorkerKey, "path": acq.Path}); toolIsError(t, rel) {
		t.Fatalf("the holder's release under its own worker key failed: %s", rel)
	}
	if b := wtBranch(t, acq.Path); b != "HEAD" {
		t.Fatalf("released worktree not detached: %q", b)
	}
}

// TestWorktreeReleaseWaitsOutAnAcquire: a release takes the pool's
// acquisition lock before its holder check, so while an acquire holds it the
// release fails with nothing changed.
func TestWorktreeReleaseWaitsOutAnAcquire(t *testing.T) {
	s, base, leadA, _ := releaseGuardServer(t)
	acq := acquireForTest(t, s, 1, leadA, base, "impl/test/alpha")
	draft := writeDraft(t, acq.Path)
	unlock, err := lockWorktreeAcquire(context.Background(), s.gitRunner(), acq.Path)
	if err != nil {
		t.Fatal(err)
	}
	rel := callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": leadA, "path": acq.Path})
	unlock()
	if !toolIsError(t, rel) || !strings.Contains(toolText(t, rel), "nothing was changed") {
		t.Fatalf("a release during an acquire must fail with nothing changed: %s", rel)
	}
	if _, err := os.Stat(draft); err != nil {
		t.Fatalf("a failed release must keep the tree: %v", err)
	}
	if rel := callToolOnce(t, s, 3, "worktree.release", map[string]any{"session_key": leadA, "path": acq.Path}); toolIsError(t, rel) {
		t.Fatalf("release after the acquire let go failed: %s", rel)
	}
}
