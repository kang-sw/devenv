# Hosted Claude: the lead's turn ends at `result`, not only at `idle`

## Problem

Owner report 2026-11-13, dashboard-hosted Claude lead (row
`shrankhumpbackuncannyelaborate`):

1. The lead is idle, yet the session header keeps showing the busy/stop
   (중단) control (`#ibadge`, `dashboard.js:1992-2010` `leadWorking`, phase
   from `wslive/state.go:710-725`: `thinking` whenever `turnStartedAt > 0`).
2. The per-prompt turn summary line ("N tool calls · duration", ring
   `kind:"turn"`) is missing for most prompts (10 turn entries for 33 user
   entries in that row's ring); surviving ones carry merged counts (e.g.
   `done:130` in 36 s).

Diagnosis (lead's delegate, read-only, ~60% confidence): with state events
on, `wsagent/claude.go:744-760` ends a turn only on `session_state_changed:
idle`; the `result` case (`claude.go:866-885`) only closes the tool batch and
defers to "the idle state that follows". This lead routinely ends its turn
while background Agent-tool subagents still run; if Claude Code withholds
`idle` until those finish, `TurnEnd` never arrives, so `turnStartedAt` stays
set (stuck busy badge) and each next prompt's `TurnStart`
(`runner.go:1633`, `claude.go:845`) overwrites it and zeroes `done`
(`state.go:258-259`) without writing the earlier turn's ring entry
(`state.go:275`). Fixture `testdata/claude-lead-background-task.jsonl` shows
`result` (stop_reason end_turn) arriving while a background task runs, but it
was recorded without state events.

## Outcome

- First confirm the mechanism with real evidence: whether Claude Code (the
  installed native `claude.exe`, version as reported) with state events on
  emits `idle` after `result` while a background Agent task is still
  running, or withholds it. A minimal local run is acceptable (cheapest model,
  a trivial prompt that starts one short background Agent task, the same
  stream-json flags the hosted runner uses — read them from
  `wsagent/claude.go`), capturing the raw lines to D:/Temp/hosted-turn/.
  Do not attach to or disturb the running hosted holders/daemon. Record the
  observed line order in the commit's `## AI Context`, and add it as a test
  fixture if it differs from existing ones.
- If confirmed (or if the evidence shows another cause, fix that one and say
  so): the lead's turn ends at its `result`, so the busy phase clears and the
  turn's ring entry is written per prompt. A later `idle` must be harmless
  (no second turn entry, no double `turnSettled` side effect — check
  `runner.go:2198-2215`). A following agent-completion turn without a user
  prompt still reopens via the existing `TurnStart` paths.
- Make sure ending at `result` does not end a turn the harness still runs
  (e.g. `result` for a sub-turn followed by more assistant output without a
  new `running`): cover the observed order in a test.
- Background tasks still show in their own badge (`Host.BackgroundTasks`);
  do not fold them into the lead's busy state.
- Out of scope unless trivially the same fix: `applyQuiet` replay
  (`record.go:296`) dropping turn entries after a daemon restart — note it in
  the report.
- T0 (behaviour of the hosted feed; no store/protocol change).

## Out of scope

- Hosted Pi (`pi.go`) unless the same gap is evident there; report only.
- Restarting the dashboard daemon or any Claude/Pi session (lead does the
  live check).

## Verification

- `go test ./internal/wsagent/... ./internal/wslive/... ./internal/wsdashboard/... -count=1`
  on the remote w5090 (clone note `remote-build-w5090`).
- A mapper/runner test replaying the observed line order: busy clears at
  `result` while a background task runs; one turn entry per prompt; a late
  `idle` adds nothing.
