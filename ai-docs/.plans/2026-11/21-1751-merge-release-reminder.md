# git.merge: remind to release the merged branch's pooled worktree

Owner decision (board item 1477=a, 2026-11-21): implement now, compatibility tier T0.

## Problem
After `ws/git.merge` lands `impl/...` into its parent, nothing reminds the
caller that the source branch is still checked out in a pooled worktree
(acquired by `worktree.acquire`). Leads forget `worktree.release`, leaving
leased worktrees behind. Today the only worktree nudges are the once-a-day
stale-pool nudge on `worktree.acquire` (internal/mcp/worktree_nudge.go) and
the release nudge in `worktree.list` (internal/mcp/worktree_list.go:36-47).

## Change
- In `agents-plugin-tool/internal/mcp/git_merge.go`, after a successful
  (landed) merge, if the source branch is checked out in a ws pooled
  worktree that is still leased (reuse whatever worktree_list.go /
  worktree_marker.go already use to find pooled worktrees and their lease),
  append one advisory `implMergeDiagnostic` alongside the existing
  post-landing advisory diagnostics (`mergeSizeWarnings`, git_merge.go:402):
  - Code: `worktree_release`
  - Classification: the same advisory classification `mergeSizeWarnings` uses
    (read git_merge_size.go; do not invent a new class)
  - Reason: names the worktree path and branch
  - Resolution: tells the caller to call `worktree.release` for that worktree
    (with the key bound to it) now that the branch is merged.
- Only for a landed merge; never on refusal or dry paths. Never block or
  fail the merge; a lookup error yields no diagnostic.
- No new MCP argument, no schema change (additive output only -> T0).

## Tests
- Add a focused test next to existing git_merge tests: merging a branch
  checked out in a leased pooled worktree yields the `worktree_release`
  advisory; merging a branch with no pooled worktree yields none.
- Run only narrow filters, prefixed with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, e.g.
  `go test ./internal/mcp -run 'GitMerge|ImplMerge|MergeSize' -timeout 20m`
  from agents-plugin-tool. Never run the whole ./internal/mcp package.
  If the Go cache is not writable in the sandbox, set GOCACHE under /tmp.

## Constraints
- Read ai-docs/manuals/ws-mcp.md, release-compatibility.md,
  performance.md and shipped-surface-boundary.md before editing (the
  diagnostic text ships to downstream agents: no repo-specific names).
- Keep the lookup cheap: at most one worktree listing per merge; no new
  persisted state.
- Surgical change; follow existing style. One commit, with `## AI Context`.
- Do not touch the main checkout, do not push, do not run daemons.
