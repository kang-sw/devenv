# Plan: dashboard git push action and badge/band agreement

Branch `delegate/git-push-button` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop. Commit here; do not merge or push (your own `git push` to the
remote is forbidden; the feature's push runs only in tests against a local
bare repository). Do not run git stash. Read
`ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`.

## Why

User (2026-11-08): the top-bar git badge showed ↑1 while the status band at
the bottom showed ↑17. Both were right at their own read time: the band's
repo figures come from the session's ws-mcp through the live snapshot
(`internal/mcp/live_snapshot.go`, `wslive.Repo`), while the badge
(`internal/wsdashboard/gitbadge.go`) is read by the daemon every 3 minutes
or on click. The user also wants to push from the dashboard by pressing
the ahead count.

## Settled (board (496), (497), lead's technical calls)

1. **(496) Badge follows the band.** Keep the daemon's own read (it serves
   pages whose session is gone) and the 3-minute period as a fallback, but
   when a session's live snapshot arrives with repo fields (branch,
   ahead/behind, modified/staged/untracked) that differ from the watched
   root's last badge read, trigger the badge's refresh for that root
   (through the existing refresh path, so the 2s `gitRefreshGap` coalescing
   applies). Find where the daemon receives live snapshots (SSE / intake)
   and hook there; match snapshot to root by the session root path.
2. **(497) Push action.** Pressing the ahead count `↑N` - in the top badge
   and in the status band - opens a small anchored confirm popover:
   `<branch> → <upstream>`, the commit count, a Push button and Cancel
   (Escape / outside click cancel). Confirm POSTs to a new guarded daemon
   route (same access guard as the existing `/s/<reply-id>/git` refresh
   route; add it to `access_test.go`'s guarded list). The daemon runs, in
   the session root, `git push <remote> <branch>` for the current branch to
   its configured upstream only (resolve remote/merge from git config; push
   `HEAD:<upstream branch>` style as needed). Never `--force` or any
   force-like option. Refuse server-side when detached, no upstream, or
   ahead is 0 at push time (re-read first). Bound it with a timeout
   (e.g. 60s) and one push per root at a time; set `GIT_TERMINAL_PROMPT=0`
   so a credential prompt fails instead of hanging the daemon.
   - On success, re-read the badge at once; the popover closes and the
     count updates.
   - On failure, show the first meaningful line of git's stderr in the
     popover (bounded length, like `gitErrMax`), never a stack or full
     output.
   - No `↑N` press target when detached or no upstream; `↑0`/in-sync shows
     no push affordance.
   - The badge's existing click (refresh) stays on the rest of the badge;
     only the ahead segment opens the popover.
   - Strings through the i18n table (`wsi18n` tables, `en.json` plus the
     existing languages the dashboard ships; machine translation is fine,
     follow how the git badge strings were added).
3. Keep both the badge and band visuals as they are now (develop merge
   `4bd9b92b7` flattened the badge's sync chip); the ahead segment gets a
   hover/focus affordance and is keyboard reachable (button semantics).

## Tests

Go: route guard; push success against a temp repo with a local bare
upstream (ahead 2 -> 0 after push, badge re-read); refusal for detached /
no upstream / ahead 0; failure message surfaced (e.g. upstream rejects a
non-fast-forward); no force flag in the command line; snapshot-driven badge
refresh triggers on a repo-field change and not on an identical snapshot.
JS: `node --check`. A headless smoke of the popover is welcome (Playwright
cached under `~/.npm/_npx`), screenshots to `/tmp/git-push-button/`.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wsdashboard/ ./internal/wsi18n/ ./cmd/ws-mcp/ && go test -count=1 ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wswhiteboard/`
(wswhiteboard: the embed size budget has ~1 KB headroom - keep new strings
out of the whiteboard embed, as `js.tk.*` already is, if they are
dashboard-only).

## Return

Commit hashes, changed files, test output, screenshot paths, anything left
open.
