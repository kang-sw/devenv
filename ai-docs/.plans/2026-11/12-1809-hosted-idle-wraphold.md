# Plan: an idle hosted Pi lead is never held forever after a rejoin; D3 retry cleanups

Source: 0.49 pre-ship re-review `62304312b..be7860ad8` (stamped concern,
7d9d27305). Findings file (read "Peer-owned" P1 and the two D3 Minor lines):
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/339c1c41-01-findings.md`.
Prior plans this builds on: `ai-docs/.plans/2026-11/12-1747-hosted-sweep-049.md`
(D1 `first > 1` rejoin hold, D3 one-shot retry; merge 8ba842f29) and
`ai-docs/.plans/2026-11/12-1749-pi-widget-coalesce.md` (merge f432f1ee6).

## Problem (P1)

An idle hosted Pi lead with any registered subagent (done ones included)
emits a full `ws-dashboard-agents` widget resend every 30 s
(`agents-plugin-pi/src/hosted-agent-rows.ts` `resend` / `FULL_RESEND_MS`).
A background child streaming while the lead is idle also emits batch
updates. Either way the 32 MiB lead ring can wrap with only widget lines
after the last turn end. After a daemon restart the replay has first seq > 1
and no turn boundary, so `replayDone` (`internal/wsagent/runner.go` ~1736)
sets `wrapHold` + `beginTurn(false)`; an idle Pi never emits a turn end, so
every dashboard message is held indefinitely while the page shows running.

## Outcome

1. **wrapHold gets an exit for Pi (the authoritative fix).** When a Pi lead
   rejoin ends in `wrapHold`, the daemon asks the harness for its state
   (Pi RPC `get_state`; verify the command and its response field for
   "streaming / turn running" against the Pi package's RPC docs or source
   under `/home/swkang/devenv/agents-plugin-pi/node_modules/@mariozechner/pi-coding-agent`,
   do not guess). A response saying no turn runs ends the held turn the same
   way a harness idle signal does (held/queued input then goes out once); a
   response saying a turn runs keeps the hold until the real turn end; a
   refused/failed probe keeps today's behaviour (hold). Backends without a
   probe (Claude) keep today's behaviour. Keep the probe's mapping in
   `pi.go` beside the existing `get_commands` / `get_session_stats`
   handling; keep the runner side backend-neutral (an optional capability,
   not a Pi branch in the runner).
2. **The idle resend stops when no subagent runs.** In
   `hosted-agent-rows.ts`, the 30 s full resend runs only while at least one
   child is running (or otherwise non-terminal); a tree of only finished
   children sends nothing periodic. Change-driven sends (status, permission,
   label, model, effort, parent, session key, caption; batch/progress
   coalescing at 2 s; done-for-vanished) are unchanged. CLI dashboard feed
   unchanged.
3. **D3 minor: a failed-then-retried send shows the message once.** Today
   `kick` records the `KindUser` entry and `KindTurnStart` before
   `Send`/`runCommand`; a failed send followed by the auto-retry leaves a
   duplicate user message and an empty turn line. After the fix, a failed
   send leaves no user entry and no turn line of its own (its error stays
   visible through `lastErr`), and the successful retry records the message
   once. Choose the simplest ordering that keeps the live event order
   correct (the mapper's events for the new turn are applied on the same
   loop after `kick` returns; verify).
4. **D3 minor: mail-only turns retry too; runCommand failure is pinned.**
   `Record.Mail` is cleared only on a successful send, so a mail-only turn
   whose send fails keeps its mail but gets no retry (gate is
   `len(c.rec.Queued) > 0`). The one-shot retry covers pending mail as well.
   Add tests for the `runCommand` failure branch and the mail-only retry.

## Settled constraints

- Peer and reviewer suggested (a)+(c); the lead chose both: the resend skip
  alone does not close the hole (a background child streaming for hours
  while the lead idles still wraps the ring).
- Rejected: exempting widget-only records from the boundary rule (a lead
  blocked in a long tool call while children stream also produces only
  widget lines, so "only widget lines" does not mean idle); persisting turn
  state in the Record (rejected earlier, board (34)).
- Compatibility tier T0: no record, registry, snapshot, holder protocol,
  widget key/event shape, or MCP shape change. Stop and report if needed.
- Files: `internal/wsagent` (runner.go, pi.go, the backend/process
  interface if the probe capability needs it, tests) and
  `agents-plugin-pi/src/hosted-agent-rows.ts` plus its test. Stop and report
  if more is needed.
- Replace fixed `sleep` negative assertions only in tests you touch.
- Heavy `go build`/`go test`/`go vet` only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- <go args>`.
- The worktree has no node_modules; run the Pi tests with a temporary
  symlink to `/home/swkang/devenv/agents-plugin-pi/node_modules` and remove
  it before committing (never commit it).

## Verification

- Go: a Pi lead rejoin over a wrapped ring with no turn boundary sends the
  state probe; an idle answer releases the hold and a queued message is
  delivered exactly once; a streaming answer keeps the hold until the turn
  end; a failed probe keeps the hold; a Claude rejoin is unchanged. A failed
  send then successful retry shows one user entry and one turn; the
  runCommand failure retries once; a mail-only failed send retries once and
  clears the mail after success; an always-failing send still does not spin.
- TS: a tree of only finished children emits no periodic resend; a running
  child still gets the 30 s resend; existing hosted-agent-rows,
  dashboard-feed, and hosted-session.integration tests pass.
- `go test ./internal/wsagent/... ./internal/wsholder/...` and `go vet`
  through ws-rgo; `git diff` reviewed against this plan before commit on
  `impl/develop/hosted-idle-wraphold` (one commit per numbered item or a
  logical grouping; the plan rides the first commit).
