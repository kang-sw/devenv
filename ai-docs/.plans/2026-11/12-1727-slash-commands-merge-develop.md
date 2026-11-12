# Merge develop into the slash-commands branch and resolve its two conflicts

## Outcome

Branch `impl/develop/rebel-zen-sprig` (ticket
`261112-feat-ws-dashboard-slash-commands`, commits d471c0063..5f76ac5a8) is
ready to land on `develop`, but `develop` moved while it was in flight and a
merge preview shows two content conflicts:

1. `agents-plugin-tool/internal/wsagent/pi.go` (around the
   `piAgentsWidgetKey` / `piAgentEvents` block): develop's side is the peer
   lead's hosted Pi subagent visibility fix (merge 1006072df), which maps the
   ws extension's subagent widget lines into live agent/permission events.
   The branch side adds Pi `get_commands` handling in the event-stream mapper
   (f460afb7b). Both are wanted; keep both behaviours.
2. `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl` line ~21
   (the composer `<div id="composer">` line): develop's side moved the new
   terminal button into the status line (c3373fe85: `{{region . "sline"}}`
   wrapped in `<div class="cfoot">` plus `<span class="tslot" id="tslot">`
   when terminals are offered); the branch adds `{{region . "cmds"}}` after
   `{{region . "ctx"}}` and its own composer changes. Keep both.

Also check auto-merged files for semantic interactions with develop's recent
merges: `dashboard.js` / `dashboard.css` (term-button, pin column, ignored
HTML), `wsagent/lead_runner_test.go` and `pi_test.go` (peer's tests), i18n
tables.

Do: in this worktree (branch `impl/develop/rebel-zen-sprig` checked out),
`git merge develop`, resolve, run verification, and commit the merge with this
plan file in the same commit (`## AI Context` stating how each conflict was
resolved).

## Boundaries

- Resolution only: no new behaviour, no refactors. If keeping both sides is
  not possible without a design choice, stop and report.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing.
- Never run go build/test/vet locally (it freezes this machine).

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 20 -- test ./internal/wsagent ./internal/wslive ./internal/wsinbox ./internal/wsdashboard ./internal/mcp -count=1`
  from the worktree root. `internal/mcp` `TestDashboardReadPageServesTheBoard`
  is a known pre-existing failure on develop; any other failure is yours.
- `claude plugin test .` inside `agents-plugin` if any claude-mod file
  changed in the resolution (it should not).

## Return

Commit on `impl/develop/rebel-zen-sprig` in
`/home/swkang/.ws-worktrees/devenv/aim-hydration-granite`. Do not merge into
develop and do not push. Report the merge commit, how each conflict was
resolved, any semantic interaction found, and test results.
