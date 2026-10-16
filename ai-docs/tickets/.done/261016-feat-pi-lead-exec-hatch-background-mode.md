---
title: Pi lead exec hatch background mode with ws-background-abort
related:
  260905-feat-ws-pi-lead-one-liner-exec-escape-hatch: amends its fixed-30s-timeout decision for background runs only
  260725-research-goal-loop-restart-starved-by-background-delegation: goal-loop starvation risk this ticket's monitor rule avoids
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 14583fa0d4a87ba4
sage-review-design-reviewed: 14583fa0d4a87ba4
completed: 2026-10-16
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
  departs from 260905's "no unbounded exec for the lead" (the spec wording
  260905 adopted in place of "holds by construction";
  ai-docs/tickets/.done/260905-feat-ws-pi-lead-one-liner-exec-escape-hatch.md#L104-L110)
  for background runs only:
  fire-and-forget monitor jobs are a real use. Rejected: an adapter-fixed
  maximum (e.g. 30 min) the caller cannot exceed. Rejected: relying on a shell
  `timeout` wrapper inside `command` instead of `timeout_s` (not portable to
  native Windows, and only works when the agent remembers it).
- **Return and completion.** `background: true` returns the task id
  immediately. On completion the lead receives a pushed message carrying the
  `why`, the exit-code line, and the output capped by the same 4 KB rule as
  foreground, through the existing held-push path: it wakes an idle lead and
  batches into the current turn when the lead is busy. The push uses a new
  family added to `PUSH_FAMILIES` (`spawner.ts`) and the unchanged `admitPush`
  admission, so the ws-compact lever recovery (which recovers only
  `PUSH_FAMILIES` plus `ws-push-batch`, per
  261015-bug-pi-compact-lever-drops-boundary-steers) and the push renderers
  cover it. Rejected: an ad-hoc custom message type (repeats the 261015
  boundary-steer loss).
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
- **Each task is reported exactly once.** Whichever of abort or process exit
  claims the task first owns its report (the abort tool result or the
  completion push). A finished task leaves the registry, so a later abort of
  its id gets the unknown-id error. Rejected: keeping finished tasks
  abortable for a grace period (no single claim point; duplicate reports).
- **Session end kills remaining tasks.** Session shutdown and `/reload` kill
  every still-running background task. With no fixed ceiling this is the only
  backstop against orphaned processes, so it is a requirement, not a
  nicety. Cleanup attaches to the existing `session_shutdown` handling, which
  fires with `event.reason` reload/new/resume/fork/quit (`index.ts`), so every
  session switch also kills remaining tasks. The lead guide states the
  asymmetry: delegated children survive `/reload`, background tasks do not.
- **Kill scope is the whole process tree.** Abort, timeout, and session-end
  kill terminate the task's whole process tree (POSIX: spawn the task in its
  own process group and signal the group; Windows: a tree kill). Completion is
  detected on child exit, not on stream close (a descendant can hold stdout
  open). Rejected: killing the direct child only (the pipeline monitor
  `tail -f | grep -m1` would orphan `tail`). Processes the command
  deliberately detached and that outlive a normal exit of the task's own
  process are left alone, as the foreground hatch already does.
- **Compaction keeps tasks visible.** The deterministic section of the
  ws-owned compaction summary built in `lead-compaction.ts` (beside
  `## Child agents in flight`) lists the live background tasks (id, `why`,
  elapsed time), so a lead that lost an id across compaction can still abort.
  Rejected: the compaction resume message (`buildCompactionResumeMessage`),
  which is sent only for goal-less compactions that resume or set
  `continue_after_compact`, so goal-mode and user `/compact` compactions
  would drop the list.
- **Lead only.** A lateral fork rejects `background: true`. The fork still
  inherits `ws-background-abort` through the inherited tool surface, where it
  lists nothing and errors on any id. Rejected: stripping the abort tool from
  fork surfaces (fork-specific surface code for no behavior gain).
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
- **Goal-loop yield point.** A running-timed-task check is OR-ed into the
  goal-loop yield at the `onSettleTimerFire` call site in `goal-loop.ts`,
  beside `hasRunningAgents`; `hasRunningAgents` itself is unchanged. The
  completion push wakes the lead, and its turn end re-arms the settle timer
  through the ordinary `agent_settled` path. Rejected: extending
  `hasRunningAgents` (it shares `computeFanIn` with the delegated-agents
  status line so the two cannot drift, and timed tasks are not delegated
  agents). Rejected: a parallel gate outside the settle timer (the timer is
  the single place that decides to send).
- **Push status line counts tasks and monitors.** The fan-in line every
  pushed message carries (`computeRunningStatusLine`, `spawner.ts`) becomes:

  ```text
  N delegated agents, M background tasks, L monitors still running
  ```

  M counts running background tasks with `timeout_s`, L those without. The
  delegated-agents segment keeps its existing presence rule (present whenever
  the registry holds any non-threadBound member, `0` included); the task and
  monitor segments appear only when nonzero (singular forms as today for a
  count of 1); the line is present when any segment is, and when the
  delegated-agents segment is absent it starts at the first present segment
  (e.g. `1 monitor still running`). `push-render.ts`'s
  `STATUS_LINE_PATTERN` is updated to recognize the extended form. Rejected:
  always printing all three segments with zeros (changes today's line for
  sessions that never use background mode).
- **Non-positive `timeout_s` is rejected** with an error result. Rejected:
  treating it as "no timeout" (zero would silently become a monitor and flip
  the goal-loop semantics).
- **Lead guide.** Background mode and `ws-background-abort` are documented in
  `agents-plugin-pi/pi-lead-guide.md` beside the existing hatch entry, where
  260905 documented the hatch. The guide's pushed-status-line paragraph
  (which reads `0 delegated agents still running` as the synthesis cue) is
  updated for the extended form: the cue additionally requires no running
  timed background task, while running monitors do not affect it, matching
  the goal-loop yield.
- **Terminology.** "Background task" in the abort tool, its list, and the
  compaction section covers both timed tasks and monitors, and each listed
  entry is marked timed (with its `timeout_s`) or monitor; only the status
  line splits them into "background tasks" (timed) and "monitors". Shipped
  descriptions and the guide state this so the lead does not read M as the
  whole abortable set.

## Constraints

- Shipped surface: `agents-plugin-pi/` text (tool descriptions, schema
  descriptions, the lead guide, injected messages) ships downstream; keep it
  free of this repository's own names and procedures.
- Cleanup hook: Pi emits `session_shutdown` for exit and every session
  switch (Pi `docs/extensions.md#L316-L349`), and `index.ts`'s handler already
  receives `event.reason` reload/new/resume/fork/quit, so attaching cleanup to
  `session_shutdown` covers `/reload`.
- Module boundary: `spawner.ts` never imports from `execute-gateway.ts`
  (the rule stated in `execute-gateway.ts`), yet the status line is built in
  `spawner.ts`. The background-task registry therefore lives in its own module
  that `execute-gateway.ts`, `spawner.ts`, `goal-loop.ts`, and
  `lead-compaction.ts` all read, so the existing `computeRunningStatusLine`
  call sites keep their shape and the goal-loop check reads the same source.
- Foreground behavior of the hatch is unchanged: fixed 30 s timeout, 4 KB cap,
  required `why`, no cwd/env override.
- Convention: no `AGENTS.md` Implementation Conventions row covers
  `agents-plugin-pi/`; the shipped-surface line above applies, following
  `ai-docs/manuals/shipped-surface-boundary.md`.

## Prior Decisions

- 260905-feat-ws-pi-lead-one-liner-exec-escape-hatch (2026-09-06, Result): "Spec: `{#260905-pi-lead-tool-surface-execute-gateway}` lists the hatch and its limits and now reads "no unbounded exec for the lead"; id unchanged. Guide verb table gained one row." — bearing: constrains
- 261015-bug-pi-compact-lever-drops-boundary-steers (2026-10-15, Decisions): "`admitPush` sends a push directly as a steer only when `heldPushQueue` is empty, so every item held at compaction time was admitted after the lost steer; prepending preserves admission order." — bearing: constrains
- 1e5fb107 (2026-10-15, commit): "The commit runs synchronously in session_compact: Pi emits it before the lever's onComplete and before the setImmediate-deferred release, so the recovered items are in the queue when the release flush requests a wake." — bearing: constrains
- 22b39547 (2026-10-08, commit): "User asked whether ws-skill args had any consumer; none did beyond the `User: <args>` echo and the dedupe key, so the user approved removing it." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/execute-gateway.ts, agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/src/spawner.ts hasRunningAgents, compaction summary deterministic section in agents-plugin-pi/src/lead-compaction.ts, status line in spawner.ts computeRunningStatusLine and push-render.ts STATUS_LINE_PATTERN, agents-plugin-pi/pi-lead-guide.md, new TS port of agents-plugin-tool/internal/wskey plus copied eff_large_wordlist.txt |
| scope.surface | public-interface | lead tool surface: do-i-really-have-to-run-this-myself schema and new ws-background-abort tool ship downstream |
| scope.new_public_symbol | yes | ws-background-abort lead tool |
| scope.new_type_contract | yes | hatch schema fields background?: boolean and timeout_s?: integer; abort tool input and list output |
| scope.test_surface | existing | agents-plugin-pi/test/execute-gateway.test.ts, native-tool-registration.test.ts, goal-loop.test.ts, lead-compaction.test.ts |
| complexity.reuse_points | confirmed | ONE_LINER_TIMEOUT_MS and ONE_LINER_OUTPUT_CAP_BYTES in execute-gateway.ts#L197-L210, heldPushQueue in spawner.ts, hasRunningAgents spawner.ts#L1075 consulted at goal-loop.ts#L1129, wskey.Generate wskey.go#L56-L59 |
| complexity.side_effect_risk | high | spawns detached long-lived processes with no fixed ceiling; kill on shutdown and reload is the only orphan backstop |
| risk.correctness | high | process lifecycle, bounded capture without pi.exec, abort-vs-completion race, goal-loop yield and compaction interplay |
| risk.fit | moderate | widens 260905 bounded hatch, ORs into the single goal-loop yield point, and extends the pushed fan-in status line |
| risk.test | moderate | timing-dependent process spawn, timeout, kill, and push-wake tests |
| risk.security_or_contract | moderate | unreviewed lead exec gains unbounded background runtime; new shipped tool schema |

## Phases

### Phase 1: Background mode, abort tool, lifecycle, and goal-loop gate

Implement every decision above in one slice: the two schema fields and their
validation, the TS word-chain id generator, background spawn in its own
process group with task id return, bounded capture, completion via a new
`PUSH_FAMILIES` push, `ws-background-abort` (abort by id, list without id,
single-report claim), process-tree kill on abort, timeout, and
`session_shutdown`, live-task listing in the compaction summary, fork
rejection, the goal-loop yield for timed tasks only, the extended push status
line, the two description additions (monitor self-termination; the one-line
`timeout_s` goal-loop rule), and the lead guide entry.

Verification expectations:

- Unit tests (`agents-plugin-pi/test/`) cover: foreground call with
  `timeout_s` rejected; non-positive `timeout_s` rejected; background returns a
  readable word-chain id; completion push carries `why`, exit line, capped output;
  timeout expiry reported as a timeout; abort by id stops the process and
  returns its result; abort without id lists running tasks; unknown id
  returns an error; abort of an already-finished id returns the unknown-id
  error; an aborted task produces no later push; abort racing process exit
  yields exactly one report; fork rejects `background: true`, and fork abort
  lists nothing and errors on any id; `session_shutdown` with reason `quit`
  and with reason `reload` kills remaining tasks; a pipeline descendant (e.g.
  the `tail` of `tail -f | grep -m1`) is gone after abort, after timeout, and
  after shutdown; the compaction summary's deterministic section lists live
  tasks; the goal-loop yield is true with a running timed task and false with
  only a running monitor; the push status line renders each segment
  combination (agents only unchanged, tasks and monitors appended only when
  nonzero) and `push-render.ts` recognizes every form.
- Tool-surface registration tests confirm `ws-background-abort` is present
  for the lead and lateral forks and absent from workers, execute-workers,
  and explore leaves.
- The package's existing test suite passes.

### Result (92790af25) - 2026-10-16

Landed in c5b6d5c79 (implementation) and 92790af25 (lite-review fixes).

- New `agents-plugin-pi/src/background-tasks.ts`: the process-wide task
  registry. It spawns tasks directly in their own process group, keeps a
  bounded `capBytes + 1` capture (stdout and stderr interleaved in arrival
  order), and gives each task a single report claim: exit, abort, or
  session-end kill. Abort, timeout, and shutdown kill the whole tree with a
  group SIGKILL (`taskkill /T` on Windows). It also owns
  `registerBackgroundTaskShutdown` (factory scope in `index.ts`), counts,
  listing, and `formatBackgroundTaskLine`. Timers longer than Node's
  2^31-1 ms limit re-arm in steps.
- New `src/word-chain-id.ts` with a byte-identical copy of
  `eff_large_wordlist.txt`; a test pins it against the Go copy.
- `execute-gateway.ts`:
  - `validateOneLinerMode` rejects foreground `timeout_s`, non-positive or
    non-integer values, and fork background.
  - The hatch's background branch pushes `ws-background-task` as a steer via
    `pushToLead`.
  - New `ws-background-abort` (`BACKGROUND_ABORT_TOOL_NAME`) is added to
    `computeLeadActiveTools`; forks inherit it.
  - `backgroundTaskReportFields` is shared by the push payload and the
    inline abort result.
- `spawner.ts`: `ws-background-task` is added to `PUSH_FAMILIES`.
  `computeRunningStatusLine` gains the background and monitor segments.
  `push-render.ts`'s `STATUS_LINE_PATTERN` recognizes every form, and a
  `<task_id> · background task` head was added.
- `goal-loop.ts`: `hasRunningTimedBackgroundTask()` is ORed into the yield
  at `onSettleTimerFire`.
- `lead-compaction.ts`: a `## Background tasks running` section, present
  only while tasks run.
- `pi-lead-guide.md`: hatch background row, `ws-background-abort` row, the
  `ws-background-task` push row, and the extended status-line paragraph
  (synthesis cue, monitor exemption, `/reload` asymmetry).

Verification: `npm test` in `agents-plugin-pi`: 2313 tests, 2310 pass,
0 fail, 3 opt-in skips. New `test/background-tasks.test.ts` (28 tests)
plus additions to execute-gateway, goal-loop, lead-compaction, push-render,
native-tool-registration, and lead-bootstrap tests. Lite review: 0
Critical. The 1 Important (timer overflow) and 3 Minor findings were fixed
in 92790af25.

Decisions:

- Completion is delivered as `steer`, matching "batches into the current
  turn when busy".
- Capture interleaves stdout and stderr; the cap semantics are unchanged.
- Kill is an immediate SIGKILL with no SIGTERM grace, mirroring the host's
  `killProcessTree`.
- The compaction section is omitted when no task runs, so it has no
  `(none)` line.
- A completion that lands in a session end's drain window is dropped.
