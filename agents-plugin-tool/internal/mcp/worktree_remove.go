package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// worktreeRemoveResult is the return shape of worktree.remove.
type worktreeRemoveResult struct {
	Path    string `json:"path"`
	Removed bool   `json:"removed"`
	Profile string `json:"profile"`
	// Branch is the branch the worktree had checked out; remove keeps it.
	Branch string            `json:"branch,omitempty"`
	Hooks  []worktreeHookRun `json:"hooks,omitempty"`
	Notes  []string          `json:"notes,omitempty"`
}

func (r worktreeRemoveResult) text() string {
	text := fmt.Sprintf("removed: %s\nprofile: %s\n", r.Path, r.Profile)
	if r.Branch != "" {
		text += "branch kept: " + r.Branch + "\n"
	}
	for _, h := range r.Hooks {
		text += h.text()
	}
	for _, n := range r.Notes {
		text += "note: " + n + "\n"
	}
	return text
}

// removeWorktree deletes a pooled worktree: every refusal is checked first,
// then the marker's profile's pre_remove hooks run, then git removes the
// worktree together with its admin directory (lease and marker included). The
// checked-out branch is kept, so the refusals guard unmerged work: a dirty
// tree, a branch holding commits no other branch reaches, a detached HEAD
// holding commits no ref reaches, and a lease whose activity is newer than
// staleAfter (a live worker's tree). It also refuses what release refuses: the
// primary worktree and any path outside the owned pool.
func removeWorktree(ctx context.Context, runner wsgit.Runner, wtPath, poolConfigValue string, staleAfter time.Duration, now time.Time, lookup worktreeProfileLookup) (worktreeRemoveResult, error) {
	var res worktreeRemoveResult
	target, entry, mainRoot, err := ownedPoolWorktree(ctx, runner, wtPath, poolConfigValue, "worktree.remove")
	if err != nil {
		return res, err
	}
	res.Path = target
	if !entry.Detached {
		res.Branch = strings.TrimPrefix(entry.Branch, "refs/heads/")
	}

	dirty, err := porcelainPaths(ctx, runner, target)
	if err != nil {
		return res, fmt.Errorf("worktree.remove: read the status of %s: %w", target, err)
	}
	if len(dirty) > 0 {
		if len(dirty) > 5 {
			dirty = append(dirty[:5], "...")
		}
		return res, fmt.Errorf("worktree.remove refuses %s: it has uncommitted changes or untracked files (%s); commit or discard them first", target, strings.Join(dirty, ", "))
	}

	if res.Branch != "" {
		n, err := countUnreachedCommits(ctx, runner, target, "refs/heads/"+res.Branch, "--exclude="+res.Branch, "--branches", "--remotes")
		if err != nil {
			return res, fmt.Errorf("worktree.remove: %w", err)
		}
		if n > 0 {
			return res, fmt.Errorf("worktree.remove refuses %s: its branch %q holds %d commit(s) no other branch reaches; merge them first (remove keeps the branch, but the unmerged work loses its checkout)", target, res.Branch, n)
		}
	} else {
		n, err := countUnreachedCommits(ctx, runner, target, "HEAD", "--branches", "--tags", "--remotes")
		if err != nil {
			return res, fmt.Errorf("worktree.remove: %w", err)
		}
		if n > 0 {
			return res, fmt.Errorf("worktree.remove refuses %s: its detached HEAD holds %d commit(s) no ref reaches; put them on a branch or discard them first", target, n)
		}
	}

	lease, err := readWorktreeLease(ctx, runner, target)
	if err != nil {
		return res, fmt.Errorf("worktree.remove: %w", err)
	}
	if lease != nil {
		activity, err := worktreeActivity(ctx, runner, target, lease)
		if err != nil {
			return res, fmt.Errorf("worktree.remove refuses %s: cannot tell how recently its leaseholder was active: %w", target, err)
		}
		if now.Sub(activity) < staleAfter {
			return res, fmt.Errorf("worktree.remove refuses %s: it is leased to worker %s, last active %s, within the worktree.nudge_stale_days threshold; release it with worktree.release once its holder is done", target, lease.WorkerKey, formatFactTime(activity))
		}
	}

	marker, err := readWorktreeMarker(ctx, runner, target)
	if err != nil {
		return res, fmt.Errorf("worktree.remove: %w", err)
	}
	if marker == nil {
		res.Profile = worktreeLegacyLabel
	} else {
		res.Profile = marker.Profile
		profile, found, err := lookup(marker.Profile)
		if err != nil {
			return res, fmt.Errorf("worktree.remove: resolve profile %q of %s: %w", marker.Profile, target, err)
		}
		switch {
		case !found:
			res.Notes = append(res.Notes, fmt.Sprintf("profile %q no longer resolves; its pre_remove hooks were skipped", marker.Profile))
		case profile.Definition.Hooks != nil:
			runs, failure := runWorktreeHooks(ctx, worktreeHookPreRemove, profile.Definition.Hooks.PreRemove, profile.Scope, worktreeHookEnv{WorktreeDir: target, TargetBranch: res.Branch})
			res.Hooks = runs
			if failure != nil {
				return res, fmt.Errorf("worktree.remove: profile %q: %w. %s was not removed; fix the hook or the profile (config.tune), then remove again", marker.Profile, failure, target)
			}
		}
		if marker.Quarantine != nil {
			res.Notes = append(res.Notes, fmt.Sprintf("cleared its quarantine (%s hook failed at %s)", marker.Quarantine.Point, marker.Quarantine.At))
		}
	}

	// --force is safe here because the refusals above already guard
	// uncommitted and unmerged work, and git refuses a worktree with
	// initialized submodules without it.
	if _, err := wtRun(ctx, runner, mainRoot, "worktree", "remove", "--force", target); err != nil {
		return res, fmt.Errorf("worktree.remove: %w", err)
	}
	res.Removed = true
	return res, nil
}

// countUnreachedCommits counts commits reachable from tip but from none of
// the refs the exclusion arguments select.
func countUnreachedCommits(ctx context.Context, runner wsgit.Runner, wtPath, tip string, not ...string) (int, error) {
	args := append([]string{"rev-list", "--count", tip, "--not"}, not...)
	out, err := wtRun(ctx, runner, wtPath, args...)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("parse rev-list count %q: %v", out, err)
	}
	return n, nil
}

// worktreeActivity is a worktree's last activity: the later of its lease's
// acquired_at (when leased) and its HEAD commit time.
func worktreeActivity(ctx context.Context, runner wsgit.Runner, wtPath string, lease *worktreeLease) (time.Time, error) {
	secs, err := wtRun(ctx, runner, wtPath, "log", "-1", "--format=%ct", "HEAD")
	if err != nil {
		return time.Time{}, err
	}
	n, err := strconv.ParseInt(secs, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse HEAD commit time %q: %v", secs, err)
	}
	activity := time.Unix(n, 0)
	if lease != nil {
		acquired, err := time.Parse(time.RFC3339, lease.AcquiredAt)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse lease acquired_at %q: %v", lease.AcquiredAt, err)
		}
		if acquired.After(activity) {
			activity = acquired
		}
	}
	return activity, nil
}
