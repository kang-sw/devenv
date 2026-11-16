package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsgit"
)

// A release reserves its target outside the pool-wide lock, including while
// it is detached and unleased. The file lives outside reset/clean's reach and
// stays in place so no contender can lock a second inode at the same path.
func worktreeReleaseLock(ctx context.Context, runner wsgit.Runner, target string) (*flock.Flock, error) {
	// A pooled linked worktree has a gitfile. Resolve it without spawning Git:
	// a short contention deadline must reach the guard, not expire during
	// process startup (especially on Windows). Git still validates ownership
	// after admission; this pointer alone never authorizes cleanup.
	dir := ""
	if data, err := os.ReadFile(filepath.Join(strings.TrimSpace(target), ".git")); err == nil {
		if pointer, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:"); ok {
			dir = strings.TrimSpace(pointer)
			if dir != "" && !filepath.IsAbs(dir) {
				dir = filepath.Join(strings.TrimSpace(target), dir)
			}
		}
	}
	if dir == "" {
		var err error
		dir, err = worktreeAdminDir(ctx, runner, target)
		if err != nil {
			return nil, err
		}
	}
	dir, err := filepath.EvalSymlinks(dir)
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
	defer unlockPool()
	// Validate ownership and read checkout facts only after admission and
	// after any preceding acquire has published its holder and branch.
	target, entry, _, err := ownedPoolWorktree(ctx, runner, path, pool, "worktree.release")
	if err == nil && caller != nil && !caller.Force {
		err = releaseHolderCheck(ctx, runner, target, *caller)
	}
	if err != nil {
		unlock()
		return "", worktreeEntry{}, nil, err
	}
	return target, entry, unlock, nil
}
