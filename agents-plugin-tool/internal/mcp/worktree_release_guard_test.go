package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
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

type fakeWorktreeChildren struct {
	mu       sync.Mutex
	byTarget map[string][]wsagent.BoundWorktreeChild
}

func (f *fakeWorktreeChildren) set(target string, children ...wsagent.BoundWorktreeChild) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byTarget == nil {
		f.byTarget = map[string][]wsagent.BoundWorktreeChild{}
	}
	key := canonicalWorktreeTestPath(target)
	if len(children) == 0 {
		delete(f.byTarget, key)
		return
	}
	f.byTarget[key] = append([]wsagent.BoundWorktreeChild(nil), children...)
}

func (f *fakeWorktreeChildren) ActiveChildrenInWorktree(worktree string) []wsagent.BoundWorktreeChild {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]wsagent.BoundWorktreeChild(nil), f.byTarget[canonicalWorktreeTestPath(worktree)]...)
}

func canonicalWorktreeTestPath(p string) string {
	p = filepath.Clean(strings.TrimSpace(p))
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved)
	}
	return p
}

// TestWorktreeReleaseRefusesLiveDirectAndNestedBoundChildren verifies that
// non-forced release refuses while a direct child, nested grandchild, or
// waiting non-closed child is bound to the target worktree — even when the
// caller owns the worktree lease — naming each child address and leaving the
// worktree untouched, while force: true still overrides.
func TestWorktreeReleaseRefusesLiveDirectAndNestedBoundChildren(t *testing.T) {
	s, base, leadA, _ := releaseGuardServer(t)
	children := &fakeWorktreeChildren{}
	s.pi.worktreeChildren = children

	acq := acquireForTest(t, s, 1, leadA, base, "impl/test/bound-child")
	draft := writeDraft(t, acq.Path)

	for i, tc := range []struct {
		name   string
		active []wsagent.BoundWorktreeChild
		want   []string
	}{
		{
			name:   "live direct child",
			active: []wsagent.BoundWorktreeChild{{Address: "id:directchild01", State: wsagent.AgentRunning, Cwd: acq.Path}},
			want:   []string{"id:directchild01", "running", "Nothing was changed", "force: true"},
		},
		{
			name: "live nested grandchild",
			active: []wsagent.BoundWorktreeChild{
				{Address: "id:directchild01", State: wsagent.AgentWaiting, Cwd: acq.Path},
				{Address: "id:grandchild02", State: wsagent.AgentRunning, Cwd: acq.Path},
			},
			want: []string{"id:directchild01", "waiting", "id:grandchild02", "running", "Nothing was changed"},
		},
		{
			name:   "waiting non-closed child",
			active: []wsagent.BoundWorktreeChild{{Address: "id:waitingchild03", State: wsagent.AgentWaiting, Cwd: acq.Path}},
			want:   []string{"id:waitingchild03", "waiting", "Nothing was changed"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			children.set(acq.Path, tc.active...)
			rel := callToolOnce(t, s, 10+i, "worktree.release", map[string]any{"session_key": leadA, "path": acq.Path})
			if !toolIsError(t, rel) {
				t.Fatalf("release with %s must be refused: %s", tc.name, rel)
			}
			text := toolText(t, rel)
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Fatalf("refusal missing %q: %s", w, text)
				}
			}
			if _, err := os.Stat(draft); err != nil {
				t.Fatalf("refused release wiped uncommitted draft: %v", err)
			}
			if b := wtBranch(t, acq.Path); b != "impl/test/bound-child" {
				t.Fatalf("refused release changed branch to %q", b)
			}
			lease, err := readWorktreeLease(context.Background(), wsgit.ExecRunner{}, acq.Path)
			if err != nil || lease == nil || lease.WorkerKey != acq.WorkerKey {
				t.Fatalf("refused release dropped lease: %+v err=%v", lease, err)
			}
		})
	}

	// force: true still releases despite a non-closed bound child.
	children.set(acq.Path, wsagent.BoundWorktreeChild{Address: "id:forcedchild04", State: wsagent.AgentRunning, Cwd: acq.Path})
	if rel := callToolOnce(t, s, 30, "worktree.release", map[string]any{"session_key": leadA, "path": acq.Path, "force": true}); toolIsError(t, rel) {
		t.Fatalf("forced release with bound child failed: %s", rel)
	}
	if _, err := os.Stat(draft); !os.IsNotExist(err) {
		t.Fatalf("forced release did not clean draft: %v", err)
	}
	if b := wtBranch(t, acq.Path); b != "HEAD" {
		t.Fatalf("forced release did not detach HEAD: %q", b)
	}
}

// TestWorktreeReleaseFromTokenBearingWorkerReachesDaemonGuard verifies that a
// release issued from a spawned agent's attached MCP server (no local runner
// handle, only EnvPiToken + EnvPiEndpoint) queries the daemon's token route
// rather than skipping the bound-child guard, fails closed if the daemon
// rejects/errors, and succeeds once no bound child remains (or with force).
func TestWorktreeReleaseFromTokenBearingWorkerReachesDaemonGuard(t *testing.T) {
	sLead, root, base, leadKey := worktreeListServer(t)
	acq := acquireForTest(t, sLead, 1, leadKey, base, "impl/test/token-worker")
	draft := writeDraft(t, acq.Path)

	var mu sync.Mutex
	var active []wsagent.BoundWorktreeChild
	var statusCode int
	var queriedWorktrees []string
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wsagent.RoutePrefix+"/by-token/worktree.children" || r.Header.Get("Authorization") != "Bearer worker-tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var body struct {
			Worktree string `json:"worktree"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		queriedWorktrees = append(queriedWorktrees, body.Worktree)
		code := statusCode
		list := append([]wsagent.BoundWorktreeChild(nil), active...)
		mu.Unlock()
		if code != 0 && code != http.StatusOK {
			http.Error(w, "daemon check failed", code)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"children": list})
	}))
	defer daemon.Close()

	// Build a connection-local server with NO local runner or WorktreeChildren,
	// only the worker capability token and daemon endpoint.
	workerServer := NewServerWithOptions(ServerOptions{
		Root: root, Version: "test",
		Env: append(sLead.environ(),
			wsagent.EnvPiToken+"=worker-tok",
			wsagent.EnvPiEndpoint+"="+daemon.URL,
		),
	})
	defer workerServer.Close()

	// 1. Live bound child reported by daemon blocks non-forced release.
	mu.Lock()
	active = []wsagent.BoundWorktreeChild{{Address: "id:nestedkid99", State: wsagent.AgentRunning, Cwd: acq.Path}}
	mu.Unlock()
	rel := callToolOnce(t, workerServer, 10, "worktree.release", map[string]any{"session_key": acq.WorkerKey, "path": acq.Path})
	if !toolIsError(t, rel) || !strings.Contains(toolText(t, rel), "id:nestedkid99") || !strings.Contains(toolText(t, rel), "Nothing was changed") {
		t.Fatalf("token-bearing worker release must be blocked by daemon guard: %s", rel)
	}
	if _, err := os.Stat(draft); err != nil {
		t.Fatalf("blocked release deleted draft: %v", err)
	}

	// 2. Daemon error fails closed on non-forced release.
	mu.Lock()
	active = nil
	statusCode = http.StatusInternalServerError
	mu.Unlock()
	relErr := callToolOnce(t, workerServer, 11, "worktree.release", map[string]any{"session_key": acq.WorkerKey, "path": acq.Path})
	if !toolIsError(t, relErr) || !strings.Contains(toolText(t, relErr), "could not check bound child agents") {
		t.Fatalf("daemon error must fail closed: %s", relErr)
	}
	if _, err := os.Stat(draft); err != nil {
		t.Fatalf("failed-closed release deleted draft: %v", err)
	}

	// 3. Once daemon reports no bound children, release succeeds.
	mu.Lock()
	statusCode = http.StatusOK
	mu.Unlock()
	if relOK := callToolOnce(t, workerServer, 12, "worktree.release", map[string]any{"session_key": acq.WorkerKey, "path": acq.Path}); toolIsError(t, relOK) {
		t.Fatalf("release after bound child closed failed: %s", relOK)
	}
	if b := wtBranch(t, acq.Path); b != "HEAD" {
		t.Fatalf("released worktree not detached: %q", b)
	}
	mu.Lock()
	if len(queriedWorktrees) < 3 {
		t.Fatalf("expected daemon worktree.children queries, got %v", queriedWorktrees)
	}
	mu.Unlock()
}

// TestWorktreeReleaseRacesAgentSpawnOnSameWorktree verifies both directions of
// the shared per-worktree admission boundary:
//  1. While release holds the exclusive per-worktree lock (paused in a
//     pre_release hook before hygiene), agent.spawn's worktree reservation is
//     immediately refused with ErrWorktreeReleasing.
//  2. While agent.spawn holds the shared worktree reservation and registers a
//     bound child before releasing it, a concurrent non-forced worktree.release
//     waits for the spawn to publish the child, sees it under the lock, and
//     refuses without modifying the worktree.
func TestWorktreeReleaseRacesAgentSpawnOnSameWorktree(t *testing.T) {
	t.Run("spawn blocked while release holds reservation", func(t *testing.T) {
		s, root, base, parent := worktreeListServer(t)
		pool := acquireTempDir(t)
		gate := filepath.Join(t.TempDir(), "release-race")
		t.Setenv("WS_TEST_ACQUIRE_HOOK_GATE", gate)
		profile := hookProfile(t, "race-release", nil, map[string][]map[string]any{
			worktreeHookPreRelease: {hook([]string{os.Args[0], "-test.run=^TestWorktreeAcquireGateHook$"})},
		})
		target, err := acquireWithServer(context.Background(), s, s.gitRunner(), root, base, "impl/test/race-a", profile, pool, parent)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		var releaseErr error
		t.Cleanup(func() { _ = os.WriteFile(gate+".resume", nil, 0o600); <-done })
		go func() {
			_, releaseErr = releaseWorktreeAs(context.Background(), s.gitRunner(), s.environ(), target.Path, pool, lookupOf(profile), &worktreeReleaseCaller{LeadKey: parent})
			close(done)
		}()
		waitAcquireGate(t, gate+".held")
		if unlock, err := wsgit.ReserveWorktreeSpawn(target.Path); !errors.Is(err, wsgit.ErrWorktreeReleasing) {
			if unlock != nil {
				unlock()
			}
			t.Fatalf("spawn reservation during active release = %v, want ErrWorktreeReleasing", err)
		}
		if err := os.WriteFile(gate+".resume", nil, 0o600); err != nil {
			t.Fatal(err)
		}
		<-done
		if releaseErr != nil {
			t.Fatalf("release failed: %v", releaseErr)
		}
	})

	t.Run("release waits for in-flight spawn and sees newly bound child", func(t *testing.T) {
		s, base, leadA, _ := releaseGuardServer(t)
		children := &fakeWorktreeChildren{}
		s.pi.worktreeChildren = children
		acq := acquireForTest(t, s, 1, leadA, base, "impl/test/race-b")
		draft := writeDraft(t, acq.Path)

		unlockSpawn, err := wsgit.ReserveWorktreeSpawn(acq.Path)
		if err != nil {
			t.Fatal(err)
		}
		releaseDone := make(chan string, 1)
		go func() {
			releaseDone <- callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": leadA, "path": acq.Path})
		}()
		// Give release time to contend on the per-worktree lock while the
		// in-flight spawn registers its child, then release the spawn lock.
		time.Sleep(100 * time.Millisecond)
		children.set(acq.Path, wsagent.BoundWorktreeChild{Address: "id:racedchild01", State: wsagent.AgentStarting, Cwd: acq.Path})
		unlockSpawn()

		rel := <-releaseDone
		if !toolIsError(t, rel) || !strings.Contains(toolText(t, rel), "id:racedchild01") {
			t.Fatalf("release racing in-flight spawn must see newly registered child and refuse: %s", rel)
		}
		if _, err := os.Stat(draft); err != nil {
			t.Fatalf("raced release wiped uncommitted draft: %v", err)
		}
		if b := wtBranch(t, acq.Path); b != "impl/test/race-b" {
			t.Fatalf("raced release changed branch to %q", b)
		}
	})
}
