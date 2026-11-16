package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsgit"
)

// A release reserves its target outside the pool-wide lock, including while
// it is detached and unleased. The file lives outside reset/clean's reach and
// stays in place so no contender can lock a second inode at the same path.
func worktreeReleaseLock(ctx context.Context, runner wsgit.Runner, target string) (*flock.Flock, error) {
	dir, err := worktreeAdminDir(ctx, runner, target)
	if err != nil {
		return nil, err
	}
	return flock.New(filepath.Join(dir, "ws-worktree-release.lock")), nil
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
	target, _, _, err := ownedPoolWorktree(ctx, runner, path, pool, "worktree.release")
	if err != nil {
		return "", worktreeEntry{}, nil, err
	}
	lock, err := worktreeReleaseLock(ctx, runner, target)
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
	unlockPool, err := lockWorktreePool(ctx, runner, target, "worktree.release")
	if err != nil {
		unlock()
		return "", worktreeEntry{}, nil, fmt.Errorf("%w; nothing was changed, release again", err)
	}
	defer unlockPool()
	// Refresh checkout facts after any preceding acquire finished. The facts
	// read before admission may describe a previous holder or branch.
	target, entry, _, err := ownedPoolWorktree(ctx, runner, target, pool, "worktree.release")
	if err == nil && caller != nil && !caller.Force {
		err = releaseHolderCheck(ctx, runner, target, *caller)
	}
	if err != nil {
		unlock()
		return "", worktreeEntry{}, nil, err
	}
	return target, entry, unlock, nil
}
