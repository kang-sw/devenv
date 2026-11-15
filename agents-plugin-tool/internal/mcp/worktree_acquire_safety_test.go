package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestWorktreeAcquirePreservesIneligibleCandidates(t *testing.T) {
	s, root, base, parent := worktreeListServer(t)
	pool := acquireTempDir(t)
	full, docs := builtinProfile(t, worktreeProfileFull), builtinProfile(t, worktreeProfileHousekeeping)
	acquire := func(branch string, profile resolvedWorktreeProfile) worktreeAcquireResult {
		t.Helper()
		acq, err := acquireWithServer(context.Background(), s, s.gitRunner(), root, base, branch, profile, pool, parent)
		if err != nil {
			t.Fatal(err)
		}
		return acq
	}
	release := func(acq worktreeAcquireResult) {
		t.Helper()
		if _, err := releaseWorktree(context.Background(), s.gitRunner(), s.environ(), acq.Path, pool, builtinProfileLookup(t)); err != nil {
			t.Fatal(err)
		}
	}
	held := acquire("impl/test/held", full)
	writeFileForTest(t, held.Path, "f.txt", "tracked work")
	writeFileForTest(t, held.Path, "worker.log", "ignored work")
	leaseBefore, err := readWorktreeLease(context.Background(), s.gitRunner(), held.Path)
	if err != nil {
		t.Fatal(err)
	}
	dirty := acquire("impl/test/dirty", full)
	release(dirty)
	writeFileForTest(t, dirty.Path, "f.txt", "dirty detached work")
	writeFileForTest(t, dirty.Path, "worker.log", "ignored detached work")
	other := acquire("impl/test/docs", docs)
	release(other)
	markerBefore, err := readWorktreeMarker(context.Background(), s.gitRunner(), other.Path)
	if err != nil {
		t.Fatal(err)
	}
	fresh := acquire("impl/test/fresh", full)
	if fresh.Reused || fresh.Path == held.Path || fresh.Path == dirty.Path || fresh.Path == other.Path {
		t.Fatalf("acquired an ineligible candidate: %+v", fresh)
	}
	assertAcquiredLease(t, s, fresh, "impl/test/fresh", parent)
	assertAcquiredLease(t, s, held, "impl/test/held", parent)
	leaseAfter, err := readWorktreeLease(context.Background(), s.gitRunner(), held.Path)
	if err != nil || !reflect.DeepEqual(leaseBefore, leaseAfter) {
		t.Fatalf("held lease changed: %+v -> %+v, %v", leaseBefore, leaseAfter, err)
	}
	for path, want := range map[string]string{held.Path: "tracked work", dirty.Path: "dirty detached work"} {
		data, err := os.ReadFile(filepath.Join(path, "f.txt"))
		if err != nil || string(data) != want {
			t.Fatalf("candidate's work changed: %s, %q, %v", path, data, err)
		}
		assertExists(t, path, "worker.log", true)
	}
	if wtBranch(t, dirty.Path) != "HEAD" || wtBranch(t, other.Path) != "HEAD" {
		t.Fatal("skipped detached candidates changed branch")
	}
	markerAfter, err := readWorktreeMarker(context.Background(), s.gitRunner(), other.Path)
	if err != nil || !reflect.DeepEqual(markerBefore, markerAfter) {
		t.Fatalf("other profile marker changed: %+v -> %+v, %v", markerBefore, markerAfter, err)
	}
}

// This hook is the test executable, not a shell or an external dependency.
func TestWorktreeAcquireGateHook(t *testing.T) {
	gate := os.Getenv("WS_TEST_ACQUIRE_HOOK_GATE")
	if gate == "" {
		return
	}
	if err := os.WriteFile(gate+".held", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := acquireFileWait(ctx, gate+".resume"); err != nil {
		t.Fatal(err)
	}
}

func TestWorktreeAcquireSlowHookRetainsLock(t *testing.T) {
	s, root, base, parent := worktreeListServer(t)
	gate, pool := filepath.Join(t.TempDir(), "hook"), acquireTempDir(t)
	t.Setenv("WS_TEST_ACQUIRE_HOOK_GATE", gate)
	profile := hookProfile(t, "slow", nil, map[string][]map[string]any{
		worktreeHookPostAcquire: {hook([]string{os.Args[0], "-test.run=^TestWorktreeAcquireGateHook$"})},
	})
	type outcome struct {
		acq worktreeAcquireResult
		err error
	}
	done := make(chan outcome, 1)
	t.Cleanup(func() { _ = os.WriteFile(gate+".resume", nil, 0o600) })
	go func() {
		acq, err := acquireWithServer(context.Background(), s, s.gitRunner(), root, base, "impl/test/slow", profile, pool, parent)
		done <- outcome{acq, err}
	}()
	waitAcquireGate(t, gate+".held")
	// Wait out the entire normal contention budget while a real hook still
	// runs, then show that the holder can finish and publish its own lease.
	_, err := acquireWithServer(context.Background(), s, s.gitRunner(), root, base, "impl/test/waiter", profile, pool, parent)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow hook waiter must time out: %v", err)
	}
	select {
	case res := <-done:
		t.Fatalf("holder ended before its hook was released: %+v", res)
	default:
	}
	if err := os.WriteFile(gate+".resume", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	res := <-done
	if res.err != nil {
		t.Fatal(res.err)
	}
	assertAcquiredLease(t, s, res.acq, "impl/test/slow", parent)
	profile.Definition.Hooks = nil
	later, err := acquireWithServer(context.Background(), s, s.gitRunner(), root, base, "impl/test/later", profile, pool, parent)
	if err != nil || later.Path == res.acq.Path {
		t.Fatalf("later acquire did not progress independently: %+v, %v", later, err)
	}
	assertAcquiredLease(t, s, later, "impl/test/later", parent)
	assertAcquiredLease(t, s, res.acq, "impl/test/slow", parent)
}
