package mcp

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// occupiedRootBranch observes symbolic HEAD without refreshing the index. The
// impl/ prefix is the occupancy authority, not session liveness or a worktree
// lease (which does not identify the checked-out branch). A detached HEAD is
// unoccupied; other inspection failures must not authorize a write.
func occupiedRootBranch(ctx context.Context, runner wsgit.Runner, root string) (string, error) {
	out, err := runner.RunGit(ctx, root, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}
	ref := strings.TrimSpace(string(out))
	if !strings.HasPrefix(ref, "refs/heads/") {
		return "", fmt.Errorf("cannot inspect occupied root: unexpected symbolic HEAD %q", ref)
	}
	branch := strings.TrimPrefix(ref, "refs/heads/")
	if !strings.HasPrefix(branch, "impl/") {
		return "", nil
	}
	return branch, nil
}

type occupiedRootError struct {
	branch string
}

func (e *occupiedRootError) resolution() string {
	base := "<the branch this worker was spawned from>"
	if parent, _, ok := parseImplBranchRoot(e.branch); ok && parent != "" {
		base = fmt.Sprintf("%q", parent)
	}
	return fmt.Sprintf("Leave this checkout unchanged. For housekeeping, use worktree.acquire(base: %s, profile: \"housekeeping\") and commit there with the returned session key; worktree.release(key: ...) it before checking that base out elsewhere.", base)
}

func (e *occupiedRootError) Error() string {
	return fmt.Sprintf("Checkout is occupied by worker branch %q (live, or not yet restored by the lead). %s", e.branch, e.resolution())
}

// guardOccupiedRoot refuses an impl checkout unless the operation explicitly
// permits that exact branch. Merge permits its resolved source and target,
// regardless of caller parenthood. A ticket-write caller skips this guard only
// for a parented session in its session-bound checkout; parent-less callers pass
// no allowed branches. Neither exemption asserts ownership or needs a lease.
// Call before status/staging/checkout, and recheck at the mutation boundary;
// these observations do not serialize arbitrary external Git processes.
func guardOccupiedRoot(ctx context.Context, runner wsgit.Runner, root string, allowedBranches ...string) error {
	branch, err := occupiedRootBranch(ctx, runner, root)
	if err != nil || branch == "" {
		return err
	}
	for _, allowed := range allowedBranches {
		if branch == allowed {
			return nil
		}
	}
	return &occupiedRootError{branch: branch}
}
