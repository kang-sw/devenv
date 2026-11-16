package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if !toolIsError(t, rel) || !strings.Contains(toolText(t, rel), "nothing was changed") || !strings.Contains(toolText(t, rel), "worktree.release: contention") || strings.Contains(toolText(t, rel), "worktree.acquire:") {
		t.Fatalf("a release during an acquire must fail with nothing changed: %s", rel)
	}
	if _, err := os.Stat(draft); err != nil {
		t.Fatalf("a failed release must keep the tree: %v", err)
	}
	if rel := callToolOnce(t, s, 3, "worktree.release", map[string]any{"session_key": leadA, "path": acq.Path}); toolIsError(t, rel) {
		t.Fatalf("release after the acquire let go failed: %s", rel)
	}
}

func TestWorktreeReleaseSlowHookAllowsOtherWorktrees(t *testing.T) {
	// Include a detached, unleased target: branch/lease checks alone cannot
	// reserve it against reuse while its release hook is still running.
	for _, detached := range []bool{false, true} {
		t.Run(map[bool]string{false: "leased", true: "unleased-detached"}[detached], func(t *testing.T) {
			s, root, base, parent := worktreeListServer(t)
			pool := acquireTempDir(t)
			gate := filepath.Join(t.TempDir(), "release")
			t.Setenv("WS_TEST_ACQUIRE_HOOK_GATE", gate)
			profile := hookProfile(t, "slow-release", nil, map[string][]map[string]any{
				worktreeHookPreRelease: {hook([]string{os.Args[0], "-test.run=^TestWorktreeAcquireGateHook$"})},
			})
			acquire := func(branch string) worktreeAcquireResult {
				t.Helper()
				res, err := acquireWithServer(context.Background(), s, s.gitRunner(), root, base, branch, profile, pool, parent)
				if err != nil {
					t.Fatal(err)
				}
				return res
			}
			target := acquire("impl/test/releasing")
			other := acquire("impl/test/other")
			if detached {
				runGit(t, target.Path, "checkout", "--detach")
				if err := removeWorktreeLease(context.Background(), s.gitRunner(), target.Path); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan struct{})
			var releaseErr error
			t.Cleanup(func() { _ = os.WriteFile(gate+".resume", nil, 0o600); <-done })
			go func() {
				_, releaseErr = releaseWorktreeAs(context.Background(), s.gitRunner(), s.environ(), target.Path, pool, lookupOf(profile), &worktreeReleaseCaller{LeadKey: parent})
				close(done)
			}()
			waitAcquireGate(t, gate+".held")
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			_, err := releaseWorktreeAs(ctx, s.gitRunner(), s.environ(), target.Path, pool, lookupOf(profile), &worktreeReleaseCaller{LeadKey: parent, Force: true})
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "worktree.release: contention") {
				t.Fatalf("a second release must not enter the same target: %v", err)
			}
			// Outlast the pool lock's contention budget with the hook gated.
			time.Sleep(worktreeAcquireLockTimeout + 100*time.Millisecond)
			later := acquire("impl/test/later")
			if later.Path == target.Path {
				t.Fatal("acquire reused a worktree whose release is still running")
			}
			plain := profile
			plain.Definition.Hooks = nil
			if _, err := releaseWorktreeAs(context.Background(), s.gitRunner(), s.environ(), other.Path, pool, lookupOf(plain), &worktreeReleaseCaller{LeadKey: parent}); err != nil {
				t.Fatalf("release of another worktree: %v", err)
			}
			if err := os.WriteFile(gate+".resume", nil, 0o600); err != nil {
				t.Fatal(err)
			}
			<-done
			if releaseErr != nil {
				t.Fatalf("slow release: %v", releaseErr)
			}
			assertAcquiredLease(t, s, later, "impl/test/later", parent)
		})
	}
}
