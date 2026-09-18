package mcp

import (
	"context"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// occupiedRootAnnouncement returns the occupied-root banner for a session whose
// worktree root has a worker's impl/* branch checked out, or "" when it does
// not. The root is "occupied" while HEAD points at an impl/* branch: a commit,
// ticket move, or branch switch there lands on the worker's branch, not the
// lead's, whether the worker is still live or has finished without the lead
// restoring the checkout. Detection is by branch pattern (one git subprocess),
// which needs no child-session liveness model and also covers the
// worker-done-lead-not-yet-restored state. The banner is advisory: a detached
// HEAD (git symbolic-ref --quiet exits non-zero), any resolution error, or a
// non-impl/ branch all yield "" and never a render failure.
//
// The caller (handleWorkflowManual) gates this to top-level lead keys: a worker
// also reads the manual, its key is minted roleLead, and on its own impl/* root
// it would otherwise be told "do not commit here" about the very branch it
// owns. The discriminator is the session record's parent — a top-level lead key
// has an empty parent — so the caller injects this only for a parent-less key.
func occupiedRootAnnouncement(root string) string {
	out, err := wsgit.ExecRunner{}.RunGit(context.Background(), root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if !strings.HasPrefix(branch, "impl/") {
		return ""
	}
	if parent, _, ok := parseImplBranchRoot(branch); ok && parent != "" {
		return "> **Occupied root.** `HEAD` here is on `" + branch + "`, a worker's branch (live, or not yet restored by the lead). Do not commit, move tickets, or switch branches in this root: the write lands on the worker's branch. Housekeeping meanwhile: `worktree.acquire(base: \"" + parent + "\", sparse_paths: [\"ai-docs\"])` checks `" + parent + "` out in a sparse pooled worktree; commit there with the returned key, and `worktree.release(key: ...)` it before anything checks `" + parent + "` out elsewhere."
	}
	// Rootless impl/<stem>: no parent segment to name as a base.
	return "> **Occupied root.** `HEAD` here is on `" + branch + "`, a worker's branch (live, or not yet restored by the lead). Do not commit, move tickets, or switch branches in this root: the write lands on the worker's branch. Housekeeping meanwhile: `worktree.acquire(base: <the branch this worker was spawned from>, sparse_paths: [\"ai-docs\"])` checks that branch out in a sparse pooled worktree; commit there with the returned key, and `worktree.release(key: ...)` it before anything checks that branch out elsewhere."
}
