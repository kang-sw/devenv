# Plan: ws/git.merge disables directory-rename detection

Branch `delegate/git-merge-directory-renames` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push.

Authority: `ai-docs/tickets/idea/261108-bug-ws-git-merge-directory-rename-misplaces-tickets.md`
(user approved the fix on the board, (454) ok). Read
`ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/ws-mcp.md`
before editing `agents-plugin-tool/internal/mcp/`.

## Fix

1. Every merge `ws/git.merge` runs (`internal/mcp/git_merge.go` ~L341, the
   `run("merge", "--no-ff", ...)` call) and every merge-tree / preflight or
   dry-run merge in the same tool or in `internal/wsgit` that the merge path
   uses passes `-c merge.directoryRenames=false`. Ticket status moves are
   always explicit, so git's directory-rename heuristic (which misreads the
   move of the last file out of a near-empty `ready/` as a directory rename)
   must never apply. Find all such call sites with grep (`merge-tree`,
   `"merge"`), and put a one-line comment at the site naming the trap.
2. Test (fails before the fix): a temp repo where the source branch moves the
   only file of `ai-docs/tickets/ready/` to `ai-docs/tickets/.done/`, and the
   target branch adds a new file to `ready/`. `ws/git.merge` (through the
   existing git_merge test harness) succeeds without conflict and leaves the
   new file in `ready/`.

## Ticket housekeeping (same branch, after the fix commit)

- Close the keeper `261108-bug-ws-git-merge-directory-rename-misplaces-tickets`
  with `ws/tickets.close` (or `git mv` to `.done/` if the tool errors), with
  a short `## Result` naming the fix commit.
- Drop the three duplicates to `ai-docs/tickets/.dropped/` with
  `ws/tickets.move` (`to: dropped`), each with a one-line note
  "Duplicate of 261108-bug-ws-git-merge-directory-rename-misplaces-tickets":
  `261106-bug-git-merge-directory-rename-relocates-tickets`,
  `261107-bug-git-merge-directory-rename-false-conflict`,
  `261108-bug-git-merge-directory-rename-moves-tickets`.
  Read each first; if one describes a distinct failure the keeper does not
  cover, leave it and report.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/mcp/ ./internal/wsgit/ && go test -count=1 -run 'Merge' ./internal/mcp/ ./internal/wsgit/`

## Return

Commit hashes, changed files, test output (and that the new test failed
before the fix), and anything left open.
