package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsgit"
)

// A release reserves its target outside the pool-wide lock, including while
// it is detached and unleased. The lock file is shared with agent.spawn's
// worktree reservation via wsgit.WorktreeReleaseLock so a spawn cannot slip in
// between release's bound-child check and its reset/clean.
func worktreeReleaseLock(ctx context.Context, runner wsgit.Runner, target string) (*flock.Flock, error) {
	return wsgit.WorktreeReleaseLock(ctx, runner, target)
}

// worktreeReleasing is checked under the acquisition lock. Acquirers skip a
// releasing target rather than waiting for its hooks. If a release starts
// after this check, its holder check waits for the acquire to publish its
// lease under that same pool lock before it may change anything.
func worktreeReleasing(ctx context.Context, runner wsgit.Runner, target string) (bool, error) {
	lock, err := worktreeReleaseLock(ctx, runner, target)
	if err != nil {
		return false, err
	}
	locked, err := lock.TryLock()
	if locked {
		_ = lock.Unlock()
	}
	return !locked, err
}

func reserveWorktreeRelease(ctx context.Context, runner wsgit.Runner, path, pool string, caller *worktreeReleaseCaller) (string, worktreeEntry, func(), error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", worktreeEntry{}, nil, fmt.Errorf("worktree.release requires a path")
	}
	lock, err := worktreeReleaseLock(ctx, runner, path)
	if err != nil {
		return "", worktreeEntry{}, nil, fmt.Errorf("worktree.release: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, worktreeAcquireLockTimeout)
	defer cancel()
	locked, err := lock.TryLockContext(waitCtx, 50*time.Millisecond)
	if !locked || err != nil {
		if err == nil {
			err = context.DeadlineExceeded
		}
		return "", worktreeEntry{}, nil, fmt.Errorf("worktree.release: contention waiting for target release lock: %w; nothing was changed, release again", err)
	}
	unlock := func() { _ = lock.Unlock() }
	unlockPool, err := lockWorktreePool(ctx, runner, path, "worktree.release")
	if err != nil {
		unlock()
		return "", worktreeEntry{}, nil, fmt.Errorf("%w; nothing was changed, release again", err)
	}
	// Validate ownership and read checkout facts only after admission and
	// after any preceding acquire has published its holder and branch.
	target, entry, _, err := ownedPoolWorktree(ctx, runner, path, pool, "worktree.release")
	if err == nil && caller != nil && !caller.Force {
		err = releaseHolderCheck(ctx, runner, target, *caller)
	}
	unlockPool()
	// Check daemon-wide bound children while still holding the exclusive
	// per-worktree lock (and after releasing the pool lock so a forwarded
	// query never stalls unrelated pool operations). Because agent.spawn takes
	// a shared lock on this same file while registering its child, no spawn
	// can slip in between this check and release hygiene.
	if err == nil && caller != nil && !caller.Force && caller.CheckChildren != nil {
		err = caller.CheckChildren(ctx, target)
	}
	if err != nil {
		unlock()
		return "", worktreeEntry{}, nil, err
	}
	return target, entry, unlock, nil
}
