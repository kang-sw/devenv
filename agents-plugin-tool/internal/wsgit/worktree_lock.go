package wsgit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

// ErrWorktreeReleasing reports that a worktree's per-worktree release
// reservation is currently held, so a concurrent spawn may not bind a child to
// it.
var ErrWorktreeReleasing = errors.New("worktree is currently being released")

// WorktreeReleaseLock returns the per-worktree reservation lock shared by
// worktree.release (exclusive lock) and agent.spawn(worktree: ...) (shared
// lock). The lock file lives in the worktree's Git admin directory outside
// reset/clean's reach and stays in place so contenders always lock one inode.
func WorktreeReleaseLock(ctx context.Context, runner Runner, target string) (*flock.Flock, error) {
	// Resolve linked-worktree gitfiles and primary .git directories without
	// spawning Git so short contention deadlines reach the lock instead of
	// expiring during process startup.
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("worktree path is required")
	}
	dir := ""
	dotGit := filepath.Join(target, ".git")
	if info, err := os.Stat(dotGit); err == nil {
		if info.IsDir() {
			dir = dotGit
		} else if data, readErr := os.ReadFile(dotGit); readErr == nil {
			if pointer, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:"); ok {
				dir = filepath.FromSlash(strings.TrimSpace(pointer))
				if dir != "" && !filepath.IsAbs(dir) {
					dir = filepath.Join(target, dir)
				}
			}
		}
	}
	if dir == "" {
		if runner == nil {
			return nil, fmt.Errorf("resolve git admin dir for %q: .git is not a directory or gitdir file", target)
		}
		out, err := runner.RunGit(ctx, target, "rev-parse", "--path-format=absolute", "--git-dir")
		if err != nil {
			return nil, fmt.Errorf("resolve git admin dir for %q: %w", target, err)
		}
		dir = strings.TrimSpace(string(out))
		if dir == "" {
			return nil, fmt.Errorf("resolve git admin dir for %q: empty git-dir", target)
		}
	}
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	return flock.New(filepath.Join(dir, "ws-worktree-release.lock")), nil
}

// ReserveWorktreeSpawn takes a shared lock on target's per-worktree release
// reservation while agent.spawn validates and registers a child in target.
// Concurrent spawns into the same worktree share the lock; an active exclusive
// release immediately refuses the spawn with ErrWorktreeReleasing, and a
// concurrent release waits for the in-flight spawn to register its child before
// checking bound children.
func ReserveWorktreeSpawn(target string) (func(), error) {
	lock, err := WorktreeReleaseLock(context.Background(), ExecRunner{}, target)
	if err != nil {
		return nil, err
	}
	locked, err := lock.TryRLock()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, fmt.Errorf("%w: %s", ErrWorktreeReleasing, target)
	}
	return func() { _ = lock.Unlock() }, nil
}
