# Plan: dashboard context meter follows the terminal band

## User report (2026-11-10, whiteboard)

"이 컨텍스트 미터기가 status band 대비해서 갱신이 엄청나게 느리고 부정확하거든요?
아마 페이지 전환을 잘 못 따라가는 거 같기도 하고요." The dashboard page's
context meter (template `r-ctx` in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl:146`) lags far
behind the Claude Code terminal status band and looks wrong; the user also
suspects it does not follow page switches. Hotfix if possible; otherwise a
ticket plus root-cause notes.

## Lead's finding (verify before relying on it)

- Terminal band (`agents-plugin/hooks/claude-mod/band.tsx`, `readUsage`) re-reads
  `$.session.usage` after every model tool call (`toolCalled`, byModel) and on
  `measured()`.
- The dashboard feed (`register.ts` `feedStatus`) reads usage only at feed
  start, on `session.measure` (pushed after each main-thread turn and on
  rate-limit moves), and after a landed compaction. So during a long turn the
  page meter stays at the previous turn's figure. Commit 7e6b2a712's AI Context
  gives no reason for the narrower cadence.

## Assignment

1. Hotfix the cadence: in `register.ts` `tool.call` post-handling (near
   `band.toolCalled(..., next.origin.plugin === 'engine')`), also call
   `feedStatus()` for model-originated calls, so the feed refreshes on the same
   events as the band. `feedStatus` already coalesces reads and the engine
   skips unchanged figures (by JSON) and batches into its flush cadence; confirm
   that holds. Mirror the change to `agents-plugin-wsflow/hooks/claude-mod/`
   per `ai-docs/manuals/wsflow-mirroring.md`. Add/adjust a test pinning the
   refresh after a model tool call (and none for the mod's own calls).
2. Investigate "does not follow page switches": how the dashboard page updates
   the `r-ctx` fragment (server render plus stream/SSE refresh, client JS
   under `internal/wsdashboard/assets/`) when the user switches sessions or
   pages, and whether it can show another session's or a stale snapshot.
   Also check whether the Pi adapter (`agents-plugin-pi/src/dashboard-feed.ts`)
   has the same per-turn-only cadence; report it, do not change Pi unless it
   is the same one-line pattern.
   If you find a bounded bug with a clear fix, fix it with a test in a separate
   commit. If the cause is unclear or the fix is structural, do not change it:
   report the evidence (file:line), and the lead files a ticket.
3. Any other inaccuracy source you find (different math from the band, window
   choice) - report it with evidence; fix only if it is a clear bug.

## Constraints

- Work only in this worktree, branch `fix/dashboard-ctx-meter-cadence`.
- Never run heavy go build/test/vet locally: use
  `~/.local/bin/ws-rgo -C /home/swkang/.ws-worktrees/devenv/aim-hydration-granite/agents-plugin-tool -m 30 -- test ./internal/wsdashboard/...`
  style. Local: `nice -n 19` for TS/JS tests of claude-mod (find how the
  package runs them; keep them niced and scoped to the touched test files),
  `nice -n 19 node --check`, `gofmt -l`.
- Do not touch the running dashboard daemons (ports 29417 / 45743).
- No `git stash`. Commit per logical unit with `## AI Context`.
- Read `ai-docs/manuals/wsflow-mirroring.md` and, for Go/template changes,
  `ai-docs/manuals/shipped-surface-boundary.md` before editing.

## Return

Commits (hashes), what changed, verification output, and the page-switch
findings with file:line evidence and whether it is fixed or needs a ticket.
