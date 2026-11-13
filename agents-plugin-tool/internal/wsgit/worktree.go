package wsgit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// The worktree checks a removal guards with, shared by the worktree.remove
// tool and the dashboard's worktree delete: the two callers live in packages
// that cannot import each other, and two copies of a guard drift.

// PorcelainPaths returns the working-tree-relative paths `git status
// --porcelain` reports, taking a rename's new path. Untracked directories stay
// collapsed (`dir/`): -uall would reopen a full-tree walk over content such as
// node_modules. -z keeps paths unquoted; the raw (untrimmed) output is parsed
// because trimming would eat the first record's leading status space.
// --no-optional-locks keeps status from taking index.lock to refresh the index,
// so listing never makes a live worker's concurrent git add/commit fail.
func PorcelainPaths(ctx context.Context, runner Runner, wtPath string) ([]string, error) {
	raw, err := runner.RunGit(ctx, wtPath, "--no-optional-locks", "status", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var paths []string
	fields := strings.Split(string(raw), "\x00")
	for i := 0; i < len(fields); i++ {
		rec := fields[i]
		if len(rec) < 4 {
			continue
		}
		paths = append(paths, rec[3:])
		// In -z form a rename or copy record is followed by its original path
		// as a separate field; skip it so only the new path is kept.
		if rec[0] == 'R' || rec[0] == 'C' || rec[1] == 'R' || rec[1] == 'C' {
			i++
		}
	}
	return paths, nil
}

// CountUnreachedCommits counts commits reachable from tip but from none of
// the refs the exclusion arguments select, run in wtPath.
func CountUnreachedCommits(ctx context.Context, runner Runner, wtPath, tip string, not ...string) (int, error) {
	args := append([]string{"rev-list", "--count", tip, "--not"}, not...)
	raw, err := runner.RunGit(ctx, wtPath, args...)
	if err != nil {
		return 0, err
	}
	out := strings.TrimSpace(string(raw))
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("parse rev-list count %q: %v", out, err)
	}
	return n, nil
}
