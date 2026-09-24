package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// worktreeListResult is the return shape of worktree.list: facts about every
// pooled worktree ws owns, with no stale/alive verdict. Judging whether a
// holder is finished is left to the reader.
type worktreeListResult struct {
	Pools     []string            `json:"pools"`
	Worktrees []worktreeListEntry `json:"worktrees"`
}

type worktreeListEntry struct {
	Path string `json:"path"`
	// Branch is the checked-out branch's short name; null when HEAD is detached.
	Branch *string `json:"branch"`
	// Dirty counts untracked files as dirty; null when git status could not be
	// read (the entry then carries a warning and no release nudge).
	Dirty *bool `json:"dirty"`
	// HeadCommitTime is HEAD's committer time (RFC 3339 UTC).
	HeadCommitTime string `json:"head_commit_time,omitempty"`
	// NewestDirtyMtime is the newest mtime over the paths `git status
	// --porcelain` reports; absent on a clean tree, where HeadCommitTime is the
	// activity signal.
	NewestDirtyMtime string `json:"newest_dirty_mtime,omitempty"`
	// WorktreeLease is null when the worktree has no lease record (acquired
	// before worktree leases existed, or released).
	WorktreeLease *worktreeListLease `json:"worktree_lease"`
	// Release is the release nudge and its discard warning, present only on a
	// clean entry.
	Release  string   `json:"release,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type worktreeListLease struct {
	WorkerKey string `json:"worker_key"`
	// WorkerKeyRecordMtime is null when the key's record is missing (pruned by
	// key retention, or unreadable).
	WorkerKeyRecordMtime *string `json:"worker_key_record_mtime"`
	ParentKey            string  `json:"parent_key"`
	ParentKeyRecordMtime *string `json:"parent_key_record_mtime"`
	AcquiredAt           string  `json:"acquired_at"`
}

// keyRecordMtimeFunc reports a session key record's mtime without refreshing
// it, and false when the record is missing.
type keyRecordMtimeFunc func(key string) (time.Time, bool)

func worktreeReleaseNudge(path string) string {
	return fmt.Sprintf("releasable via worktree.release(path: %q) once its holder is done with it; warning: release discards uncommitted changes and untracked or ignored files (git reset --hard + git clean -ffdx)", path)
}

func (r worktreeListResult) text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "pools: %s\n", strings.Join(r.Pools, ", "))
	if len(r.Worktrees) == 0 {
		b.WriteString("worktrees: none\n")
		return b.String()
	}
	keyFact := func(key string, mtime *string) string {
		if mtime == nil {
			return key + " (record missing)"
		}
		return key + " (record mtime " + *mtime + ")"
	}
	for _, e := range r.Worktrees {
		fmt.Fprintf(&b, "\nworktree: %s\n", e.Path)
		if e.Branch != nil {
			fmt.Fprintf(&b, "  branch: %s\n", *e.Branch)
		} else {
			b.WriteString("  branch: (detached)\n")
		}
		switch {
		case e.Dirty == nil:
			b.WriteString("  state: unknown\n")
		case *e.Dirty:
			b.WriteString("  state: dirty\n")
		default:
			b.WriteString("  state: clean\n")
		}
		if e.HeadCommitTime != "" {
			fmt.Fprintf(&b, "  head_commit_time: %s\n", e.HeadCommitTime)
		}
		if e.NewestDirtyMtime != "" {
			fmt.Fprintf(&b, "  newest_dirty_mtime: %s\n", e.NewestDirtyMtime)
		}
		if l := e.WorktreeLease; l != nil {
			fmt.Fprintf(&b, "  worktree_lease: worker_key %s; parent_key %s; acquired_at %s\n",
				keyFact(l.WorkerKey, l.WorkerKeyRecordMtime), keyFact(l.ParentKey, l.ParentKeyRecordMtime), l.AcquiredAt)
		} else {
			b.WriteString("  worktree_lease: no lease record\n")
		}
		if e.Release != "" {
			fmt.Fprintf(&b, "  release: %s\n", e.Release)
		}
		for _, w := range e.Warnings {
			fmt.Fprintf(&b, "  warning: %s\n", w)
		}
	}
	return b.String()
}

// listPoolWorktrees enumerates the worktrees release treats as owned (see
// ownedPoolRoots), skipping the primary worktree and prunable records whose
// directory is gone, and reports facts per entry ordered by path. It is
// read-only: key record mtimes come from keyMtime, which must not touch them.
func listPoolWorktrees(ctx context.Context, runner wsgit.Runner, root, poolConfigValue string, keyMtime keyRecordMtimeFunc) (worktreeListResult, error) {
	res := worktreeListResult{Worktrees: []worktreeListEntry{}}
	entries, err := listWorktrees(ctx, runner, root)
	if err != nil {
		return res, err
	}
	if len(entries) == 0 {
		return res, fmt.Errorf("could not resolve the primary worktree for %q", root)
	}
	mainRoot := entries[0].Path
	res.Pools = ownedPoolRoots(poolConfigValue, mainRoot)
	for i, e := range entries {
		if i == 0 || e.Path == mainRoot || e.Prunable || !underAnyPool(res.Pools, e.Path) {
			continue
		}
		res.Worktrees = append(res.Worktrees, describePoolWorktree(ctx, runner, e, keyMtime))
	}
	sort.Slice(res.Worktrees, func(i, j int) bool { return res.Worktrees[i].Path < res.Worktrees[j].Path })
	return res, nil
}

func describePoolWorktree(ctx context.Context, runner wsgit.Runner, e worktreeEntry, keyMtime keyRecordMtimeFunc) worktreeListEntry {
	out := worktreeListEntry{Path: e.Path}
	if !e.Detached && e.Branch != "" {
		branch := strings.TrimPrefix(e.Branch, "refs/heads/")
		out.Branch = &branch
	}

	if dirtyPaths, err := porcelainPaths(ctx, runner, e.Path); err != nil {
		out.Warnings = append(out.Warnings, "cannot read git status: "+err.Error())
	} else {
		dirty := len(dirtyPaths) > 0
		out.Dirty = &dirty
		if newest, ok := newestMtime(e.Path, dirtyPaths); ok {
			out.NewestDirtyMtime = formatFactTime(newest)
		}
	}

	if secs, err := wtRun(ctx, runner, e.Path, "log", "-1", "--format=%ct", "HEAD"); err != nil {
		out.Warnings = append(out.Warnings, "cannot read HEAD commit time: "+err.Error())
	} else if n, perr := strconv.ParseInt(secs, 10, 64); perr != nil {
		out.Warnings = append(out.Warnings, fmt.Sprintf("cannot parse HEAD commit time %q", secs))
	} else {
		out.HeadCommitTime = formatFactTime(time.Unix(n, 0))
	}

	lease, err := readWorktreeLease(ctx, runner, e.Path)
	if err != nil {
		out.Warnings = append(out.Warnings, "cannot read worktree lease: "+err.Error())
	} else if lease != nil {
		mtime := func(key string) *string {
			if t, ok := keyMtime(key); ok {
				s := formatFactTime(t)
				return &s
			}
			return nil
		}
		out.WorktreeLease = &worktreeListLease{
			WorkerKey:            lease.WorkerKey,
			WorkerKeyRecordMtime: mtime(lease.WorkerKey),
			ParentKey:            lease.ParentKey,
			ParentKeyRecordMtime: mtime(lease.ParentKey),
			AcquiredAt:           lease.AcquiredAt,
		}
	}

	if out.Dirty != nil && !*out.Dirty {
		out.Release = worktreeReleaseNudge(e.Path)
	}
	return out
}

// porcelainPaths returns the working-tree-relative paths `git status
// --porcelain` reports, taking a rename's new path. Untracked directories stay
// collapsed (`dir/`): -uall would reopen a full-tree walk over content such as
// node_modules. -z keeps paths unquoted; the raw (untrimmed) output is parsed
// because trimming would eat the first record's leading status space.
// --no-optional-locks keeps status from taking index.lock to refresh the index,
// so listing never makes a live worker's concurrent git add/commit fail.
func porcelainPaths(ctx context.Context, runner wsgit.Runner, wtPath string) ([]string, error) {
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

// newestMtime returns the newest mtime over the given worktree-relative paths,
// skipping paths that no longer exist (deletions). A collapsed untracked
// directory contributes its own mtime.
func newestMtime(wtPath string, paths []string) (time.Time, bool) {
	var newest time.Time
	found := false
	for _, p := range paths {
		info, err := os.Lstat(filepath.Join(wtPath, filepath.FromSlash(strings.TrimSuffix(p, "/"))))
		if err != nil {
			continue
		}
		if !found || info.ModTime().After(newest) {
			newest = info.ModTime()
			found = true
		}
	}
	return newest, found
}

func formatFactTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
