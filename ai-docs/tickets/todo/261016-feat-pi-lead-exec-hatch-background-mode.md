---
title: Pi lead exec hatch background mode with ws-background-abort
related:
  260905-feat-ws-pi-lead-one-liner-exec-escape-hatch: amends its fixed-30s-timeout decision for background runs only
  260725-research-goal-loop-restart-starved-by-background-delegation: goal-loop starvation risk this ticket's monitor rule avoids
---

# Pi lead exec hatch background mode with ws-background-abort

## Background

The Pi lead's one-liner exec hatch, `do-i-really-have-to-run-this-myself`
(`ONE_LINER_EXEC_TOOL_NAME` in `agents-plugin-pi/src/execute-gateway.ts`),
runs one command with a fixed 30 s timeout and a 4 KB output cap. Jobs that
wait on a timer (sleep-then-check, CI or deploy polling, waiting for a log
line) cannot fit in 30 s, and `ws-execute` spawns a full approval-gated worker
for what is a single wait. The lead can already detach a process with
`nohup ... &` (the 30 s bound covers only the direct command), but nothing
tells the lead when it finishes. The missing piece is completion notification,
not the ability to run.

This ticket adds a background mode to the hatch: the call returns a readable
task id immediately, the result arrives later as a pushed message, and one new
tool aborts or lists background tasks.

## Decisions

- **Schema: two optional fields on the existing hatch.**

  ```text
  background?: boolean
  timeout_s?: integer   // only with background: true
  ```

  Rejected: one union field `background: null | true | int`. Pi serves
  several model providers, and type unions (`oneOf` / type arrays) are
  unevenly supported in tool schemas (Gemini function declarations are the
  known case); two fields also read more plainly to the model.
- **`timeout_s` is rejected in foreground.** A foreground call carrying
  `timeout_s` returns an error result; the foreground 30 s timeout stays a
  constant the caller cannot change, as 260905 decided. Rejected: accepting
  `timeout_s` in foreground clamped to 30 s (widens the surface for no use).
- **No fixed ceiling on background runs.** `timeout_s` omitted means the task
  runs until it exits, is aborted, or the session ends. This deliberately
  departs from 260905's "bounded by construction" for background runs only:
  fire-and-forget monitor jobs are a real use. Rejected: an adapter-fixed
  maximum (e.g. 30 min) the caller cannot exceed. Rejected: relying on a shell
  `timeout` wrapper inside `command` instead of `timeout_s` (not portable to
  native Windows, and only works when the agent remembers it).
- **Return and completion.** `background: true` returns the task id
  immediately. On completion the lead receives a pushed message carrying the
  `why`, the exit-code line, and the output capped by the same 4 KB rule as
  foreground, through the existing held-push path: it wakes an idle lead and
  batches into the current turn when the lead is busy.
- **Task id: a readable word-chain stem**, returned by the hatch and
  accepted by the abort tool. Format matches every other ws readable stem:
  three lowercase words from the EFF large wordlist joined by `-`, as
  `wskey.Generate` (`agents-plugin-tool/internal/wskey/`) produces. Because
  that generator is Go-only and the Pi extension is TypeScript, the generator
  is ported to TS inside `agents-plugin-pi` with a copy of the same wordlist.
  The id must be unique among the session's live tasks (regenerate on
  collision). Rejected: four words (three already give about 4.7e11
  combinations for a small live set). Rejected: a new MCP tool returning
  `wskey.Generate` output (new tool surface just to mint an id). Rejected: a
  UUID or counter (not readable).
- **New lead tool `ws-background-abort`.** Given a task id, it aborts that
  task. Called without an id, it returns the list of running background tasks
  (id, `why`, elapsed time), so no separate list tool exists. Rejected name:
  `abort-background-task` (lead tools use the `ws-` prefix; the deliberately
  ugly name is the hatch's own deterrent and does not extend to its abort).
- **Session end kills remaining tasks.** Session shutdown and `/reload` kill
  every still-running background task. With no fixed ceiling this is the only
  backstop against orphaned processes, so it is a requirement, not a
  nicety.
- **Compaction keeps tasks visible.** The compaction resume message the
  adapter injects lists the live background tasks (id, `why`, elapsed time)
  automatically, so a lead that lost the id in its summary can still abort.
- **Lead only.** A lateral fork rejects `background: true`.
- **Goal-loop interaction is decided by `timeout_s`.**
  - A background task **with** `timeout_s` is a bounded wait and counts as
    outstanding work: the goal loop yields (does not re-prompt the lead)
    while it runs, the same way it yields for a running delegated agent.
  - A background task **without** `timeout_s` is a monitor and never blocks
    goal-loop re-firing.
  - The `timeout_s` schema description states this in one line, in substance:
    "A background task with no timeout is treated as a monitor and does not
    block goal-loop re-firing." No other prompt text carries the rule.

  Rejected: every background task blocks the goal loop (an unbounded monitor
  would starve it indefinitely, the shape of
  260725-research-goal-loop-restart-starved-by-background-delegation).
  Rejected: no background task blocks the goal loop (the loop would nag a lead
  that is legitimately waiting on a timer). Accepted risk: a lead can pass a
  very large `timeout_s`; that is the lead's explicit choice.
- **Monitors must self-terminate on their condition.** Completion is pushed
  once, at process exit, so the hatch description tells the lead that a
  wake-me-on-event monitor must exit when the event occurs (e.g.
  `tail -f <log> | grep -m1 ERROR`).
- **Posture of the hatch is unchanged.** Background mode is for waiting and
  watching, not for builds or mutation; the existing "changes nothing that
  would need review, otherwise use `ws-execute`" rule and the required `why`
  apply to background calls as they do to foreground ones.

- **Timeout reporting.** A timed-out background task reports like a
  foreground timeout: the killed exit-code annotation plus
  `(timed out after <N>s)` in the completion push.
- **Abort returns the result inline.** `ws-background-abort` returns the
  aborted task's exit line and capped output in its own tool result and sends
  no completion push for that task. Rejected: also pushing a completion (the
  lead already holds the result; a second report is a duplicate).
- **Bounded output capture.** Each task's in-memory output capture is bounded
  at the 4 KB cap with the foreground head-keeping semantics. Rejected:
  buffering all output until exit (a monitor with no timeout could grow memory
  without limit). This likely means spawning directly rather than through
  `pi.exec`, which buffers the full output.
- **Goal-loop yield point.** Running timed background tasks join the existing
  goal-loop yield predicate (`hasRunningAgents`, consulted by
  `onSettleTimerFire` in `goal-loop.ts`). The completion push wakes the lead,
  and its turn end re-arms the settle timer through the ordinary
  `agent_settled` path. Rejected: a parallel gate (the settle timer relies on
  one yield point).
- **Non-positive `timeout_s` is rejected** with an error result. Rejected:
  treating it as "no timeout" (zero would silently become a monitor and flip
  the goal-loop semantics).
- **Lead guide.** Background mode and `ws-background-abort` are documented in
  `agents-plugin-pi/pi-lead-guide.md` beside the existing hatch entry, where
  260905 documented the hatch.

## Constraints

- Shipped surface: `agents-plugin-pi/` text (tool descriptions, schema
  descriptions, the lead guide, injected messages) ships downstream; keep it
  free of this repository's own names and procedures.
- Foreground behavior of the hatch is unchanged: fixed 30 s timeout, 4 KB cap,
  required `why`, no cwd/env override.

## Phases

### Phase 1: Background mode, abort tool, lifecycle, and goal-loop gate

Implement every decision above in one slice: the two schema fields and their
validation, background spawn with task id return, completion via held push,
`ws-background-abort` (abort by id, list without id), kill on session shutdown
and `/reload`, live-task listing in the compaction resume message, fork
rejection, the goal-loop yield for timed tasks only, the two description
additions (monitor self-termination; the one-line `timeout_s` goal-loop
rule), and the lead guide entry.

Verification expectations:

- Unit tests (`agents-plugin-pi/test/`) cover: foreground call with
  `timeout_s` rejected; non-positive `timeout_s` rejected; background returns a
  readable word-chain id; completion push carries `why`, exit line, capped output;
  timeout expiry reported as a timeout; abort by id stops the process and
  returns its result; abort without id lists running tasks; unknown id
  returns an error; fork rejects `background: true`; shutdown kills remaining
  tasks; the compaction resume message lists live tasks; the goal-loop yield
  predicate is true with a running timed task and false with only a running
  monitor.
- Tool-surface registration tests confirm `ws-background-abort` is present
  for the lead and absent from workers, execute-workers, and explore leaves.
- The package's existing test suite passes.
