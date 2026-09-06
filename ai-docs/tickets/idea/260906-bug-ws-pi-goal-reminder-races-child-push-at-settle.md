---
title: Pi goal-loop reminder races a child push at agent_settled and spins the runaway backstop
spec:
  - pi-adapter-runtime
related:
  - 260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction
  - 260906-bug-ws-pi-lead-cannot-see-or-load-skills
  - 260906-feat-ws-pi-lead-explore-as-async-rpc-child
sage-review-design: required
sage-review-completeness: recommended
---

# Pi goal-loop reminder races a child push at agent_settled and spins the runaway backstop

## Background

Owner dogfood, 2026-09-06, on a `/goal` drain run. Right after the lead
ended a turn with the drain skill's continuing line, the TUI printed, in
order: one `Extension "<runtime>" error: Agent is already processing a
prompt. Use steer() or followUp() to queue messages, or wait for
completion.`, the `[ws-agent-settled]` push from a just-finished child
(`reason: idle`, `0 delegated agents still running`), then nine more of the
same error, `Warning: Goal loop force-stopped: 10 consecutive re-fires with
no tool call`, and two further errors. The lead's real turn kept running
underneath (it went on to `ws-agent-send`, `ws__todo_check`,
`ws__playbook_render`), but Esc did not interrupt it, and one more of the
same error appeared under `ws__todo_check`.

Mechanism, verified against the installed Pi runtime
(`node_modules/@earendil-works/pi-coding-agent/dist/core/agent-session.js`
and the bundled `pi-agent-core` `Agent.prompt`):

1. `_runAgentPrompt`'s `finally` calls `_emitAgentSettled`, which sets
   `_isAgentRunActive = false` BEFORE awaiting the extension
   `agent_settled` handlers. During that emit the session reports idle.
2. Two adapter callers can start a lead turn in that window: the goal-loop's
   `agent_settled` handler (`goal-loop.ts`, `pi.sendUserMessage(reminder)`
   → `prompt()`), and `pushToLead` (`spawner.ts`, `pi.sendMessage(custom,
   {deliverAs: "followUp", triggerTurn: true})` → `sendCustomMessage`), fired
   by the child's settle notification arriving at the same moment.
3. `prompt()` checks `isStreaming` once, then awaits four things
   (`checkAuth`, `_checkCompaction`, `emitBeforeAgentStart`, plus the input
   hook) before `_runAgentPrompt` flips the flag. `sendCustomMessage` checks
   `isStreaming` and enters `_runAgentPrompt` in the same microtask. So the
   push starts the run while the reminder is still in its pre-run awaits;
   the reminder then calls `_runAgentPrompt` too, and core `Agent.prompt`
   throws `Agent is already processing a prompt` because `activeRun` is
   set.
4. The throwing `_runAgentPrompt`'s `finally` runs anyway: it clears
   `_isAgentRunActive` (which the OTHER, live run owns) and emits
   `agent_settled` again. The goal-loop sees a settle with no tool call,
   re-injects the reminder, `prompt()` sees idle, `_runAgentPrompt` →
   `Agent.prompt` throws again → settle again. Each cycle increments the
   runaway streak until the threshold (10) force-stops the loop.
5. The flag stays false while the live run continues. Consequences: the
   TUI's Esc handler is gated on `session.isStreaming`, so Esc is a no-op;
   every later push in that turn (`sendCustomMessage` sees idle →
   `_runAgentPrompt` → throw) is dropped instead of queued, which is the
   error under `ws__todo_check` — child reports during that turn were lost.

Why the goal loop fired at all: its yield gate (`hasRunningAgents`,
`spawner.ts` `computeFanIn`) counts registry records with `running` set.
`applyRpcEvent` clears `record.running` synchronously on the child's
`agent_settled` event, but the `ws-agent-settled` push is issued only after
`await harvestLastMessage(record)` (an RPC round trip) in the settle IIFE.
Between those two points the child is neither "running" nor "pushed", so a
lead settle landing in that window sees zero running agents and fires the
reminder even though a wake for the lead is already on its way. The gate
asks "is anything running", not "is anything about to wake the lead".

Steps 4 and 5 are Pi's own bug (a failed start should not clear a flag it
never set, nor emit a settle for a run that never began). The adapter
cannot change that, but it owns both racing callers, so it can make sure no
two adapter-initiated turn starts ever overlap. That is the fix here.

A second, related finding from the same reading: a lead turn started by a
custom push while the lead is idle (`sendCustomMessage` with `triggerTurn`)
goes straight to `_runAgentPrompt` and never through `prompt()`, so
`before_agent_start` does not fire for it. `registerLeadBootstrap`
(`lead-bootstrap.ts`) appends the `<ws>` block (manual snapshot, guide,
`<available_skills>`) from that hook. `_runAgentPrompt`'s `finally` clears
only `_systemPromptOverride`, not `agent.state.systemPrompt`, so the first
turn of a push-woken run still carries the previous run's block through
`createContextSnapshot()`; from the second turn on,
`prepareNextTurnWithContext` rebuilds with `_systemPromptOverride ??
_baseSystemPrompt` and the block is gone. A push-woken run that makes more
than one model call therefore continues without the ws block, and the
block it did have is stale (previous turn's skill list). This is the same
wake path and belongs to the same fix, so it is Phase 2 rather than a
separate ticket.

## Proposed direction

Principle, set by the owner on 2026-09-06: the goal-loop reminder is the
lowest-priority wake. It exists only to keep the lead moving when nothing
else will; whenever any other item could still wake the lead, the reminder
must not fire. Owner's chosen shape (same day): a deliberate settle delay.
The reminder is not issued from `agent_settled` itself but from a timer
armed there; when the timer fires, the reminder goes out only if the lead
is still idle and every other wake condition is exhausted.

**Lands after `260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction`
Phase 1** and builds on what it introduces: the `leadCompactingRef` flag,
`HeldPush.deliverAs`, `steer` holds, `injectDiscussionSummary` routed
through `pushToLead`, and the `releaseAfterCompaction` routine. This
ticket touches the same seams only as stated below; it does not
re-specify them.

Adapter-only change under `agents-plugin-pi/` (golden rule: no ws-mcp
change). Pi is not patched.

- **One reminder emission path: the settle timer.** `agent_settled` (goal
  mode active, lead process) arms a single timer, `settle_delay_ms` from
  `goal-loop-config.json` (built-in default 5000, read fresh per settle
  like the threshold). The timer closes over the settle handler's `ctx`
  (for `isIdle()` and `setStatus`), is `unref`'d so it never keeps the
  process alive, and is cancelled by re-arming, `agent_start`,
  `goal-achieved`, `goal-blocked`, force-stop, `/goal` re-arm (the new
  announcement starts a turn; the next settle arms afresh), and
  `session_shutdown`. While it is pending the status line reads
  `Goal loop: settling`. The compaction ticket's `releaseAfterCompaction`
  idle branch arms this same timer instead of sending the reminder
  directly, so the timer is the only place a reminder is emitted; its
  `pendingRearm` payload (failure reason, carry-forward) is consumed by the
  timer's reminder. The delay absorbs every short-lived post-settle wake
  the adapter issues (the `harvestLastMessage` window, the liveness probe,
  auto-park, a child's own final report, the settle-time
  `flushHeldPushes` re-send, which itself starts a turn with
  `triggerTurn: true`) and any it does not yet know about.
- **Fire condition at the timer.** The reminder is issued only when
  `ctx.isIdle()` is true, `leadCompactingRef` is clear, and no RPC child is
  `running` and not `terminalThisTurn` (today's gate; a child blocked in a
  gated command awaiting `ws-approve` is still `running`, so no separate
  approval clause). Otherwise the tick is a yield: no reminder, no streak
  advance, the status line stays, and the next `agent_settled` re-arms the
  timer. Async explore needs no code here: explore leaves live in their
  own `AgentRegistry`, invisible to `computeFanIn`, and their completion is
  never pushed, so the reminder is what brings the lead back to poll
  them; the spec records this as documentation only.
- **Reminder send.** `pi.sendUserMessage(reminder, { deliverAs:
  "followUp" })`: if a turn started between the idle check and the send,
  Pi queues the reminder instead of throwing (`prompt()` throws on a
  streaming session without `streamingBehavior`).
- **Boundary guard.** The delay shrinks the race to the reminder's own
  `prompt()` await window (auth check, compaction check,
  `before_agent_start`). A push landing there would still collide, and the
  failure is severe, so it is closed: the goal loop sets a
  `reminderStartPending` flag immediately before `pi.sendUserMessage`.
  `pushToLead` reads it through a `leadIdleRef`-style seam and, when set,
  sends with `triggerTurn: false`: Pi's `sendCustomMessage` falls through
  to `_appendCustomMessage` (not streaming, no trigger), the message lands
  in `agent.state.messages`, and the reminder run's `createContextSnapshot`
  picks it up ahead of the reminder text. One turn start, no collision.
  The flag is cleared on `agent_start`, on `agent_settled`, and by a
  fallback timeout of `settle_delay_ms`: the extension-facing
  `sendUserMessage` is fire-and-forget (Pi's runtime wrapper swallows the
  rejection into `runner.emitError`), so a pre-run rejection (compaction
  guard, missing model, an `input` handler answering `handled`) reaches
  neither event, and without the timeout the flag would latch and every
  later push would append without ever waking the lead.
- **Runaway backstop.** Unchanged: `decideOnSettle`'s streak logic moves to
  the timer callback (a fired reminder counts, a yield does not; a
  cancelled cycle leaves `sawToolCallThisCycle` as it was, which is the
  conservative direction).
- **Phase 2, ws block on push-woken runs.** Runs started by a custom push
  while the lead is idle bypass `prompt()` and lose the ws block from their
  second turn on (Background). They are started instead by delivering the
  custom message as `deliverAs: "nextTurn"` and issuing a one-line
  `sendUserMessage` wake, so the run goes through `prompt()` and
  `before_agent_start`. Push rendering (`customType`, `display`,
  `details`) is unchanged; the wake line is the only visible addition.
  Because idle-time pushes then share `prompt()`'s await window, two
  children settling in the same tick would race each other, so the hold
  machinery is extended rather than duplicated: `heldPushQueue` gains a
  second hold reason, "a wake start is pending", set by whichever caller
  starts a turn (the reminder, an idle-time push, the summary injection)
  and cleared on `agent_start`, at which point the held items are flushed
  as `followUp` with their recorded `deliverAs` (the existing settle-time
  flush and its status-line rebuild are untouched). Phase 1's
  `reminderStartPending` becomes that general flag; the `triggerTurn:
  false` append is replaced by the hold, since a held push flushed at
  `agent_start` reaches the same run as a queued follow-up.
- Rejected: an in-flight settle counter feeding the yield gate as Phase 1.
  It closes only the windows the adapter already knows about and needs a
  marker on every push site; the delay covers them all. Rejected: moving
  the reminder to the custom-message path for its atomic check-and-start;
  it would drop `before_agent_start` for the goal loop's main turn.
  Rejected: a separate `lead-wake.ts` serializer; `heldPushQueue` already
  is the adapter's idle-gated FIFO with flush-time status lines.
- Upstream: the `_runAgentPrompt` `finally` desync (clearing a flag it
  never set, emitting a settle for a run that never began) is Pi's; file
  it separately at the owner's call. The adapter fix stands without it.

## Spec Impact

`pi-adapter-runtime`, on top of the compaction ticket's amendments:
`{#260904-pi-goal-loop-arming-settled-levers}` states the lowest-priority
rule, the settle timer (config key, default, cancel points, status line,
the release routine arming it), the fire condition (idle, not compacting,
no running child; async explore excluded as documentation), the
`followUp` send, and the `reminderStartPending` guard with its clear
points. `{#260904-pi-report-to-lead-channel}` gains the `triggerTurn:
false` append rule while a reminder start is pending. Phase 2 replaces
that rule with the second hold reason and its `agent_start` flush, and
adds one sentence under `{#260905-pi-lead-bootstrap-system-prompt}`:
push-woken idle runs go through `prompt()` so the block is present and
fresh on them too.

## Constraints

- `agents-plugin-pi/` only; no ws-mcp change; no Pi patch.
- Lands after `260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction`
  Phase 1; that ticket's release routine, hold predicate, and
  `injectDiscussionSummary` routing are consumed, not re-specified.
- Push rendering and the model-facing content of pushes and reminders are
  unchanged; Phase 2 adds only the one-line wake text.
- Runaway backstop semantics stay as they are.
- Timer and flag logic is pure and unit-tested with an injectable clock;
  IO listeners are thin glue at factory scope, matching the goal-loop's
  own listeners. No timer runs in a spawned child process.

## Phases

### Phase 1: Delay the reminder past settle and guard the start

Add `settle_delay_ms` to the goal-loop config reader, the timer with its
cancel points and status line, the fire condition, the `followUp` send,
the `reminderStartPending` flag with its three clear points, and the
`triggerTurn: false` branch in `pushToLead`; point the compaction
ticket's release routine at the timer. Tests (fake clock): a settle
followed by a push before the delay yields and re-arms; a settle with
nothing else fires exactly once at the delay; `agent_start`, each disarm
lever, `/goal` re-arm, and `session_shutdown` cancel a pending timer; a
running child or a set compacting flag at fire time yields; the release
routine's idle branch arms the timer and the fired reminder carries
`pendingRearm`; a push arriving while `reminderStartPending` is set is
sent with `triggerTurn: false` and appears before the reminder in the
session; the flag clears on `agent_start`, on `agent_settled`, and by
timeout with no event; the streak advances on fired reminders only.
Amend the two Phase 1 spec passages. Owner-run live check: a `/goal` drain
where a child settles at the same moment as the lead; no `already
processing` error, Esc still interrupts, the settling status is visible.

### Phase 2: Carry the ws block on push-woken runs

Add the "wake start pending" hold reason to `heldPushQueue` with its
`agent_start` flush, generalize `reminderStartPending` into it, and start
idle-time custom pushes (and the summary injection) as `nextTurn` plus a
one-line `sendUserMessage` wake so the run goes through `prompt()` and
`before_agent_start`. Tests: an idle-time push results in one `nextTurn`
custom message and one user wake; the fake `before_agent_start` handler
runs for that run; two pushes in the same tick produce one start and one
held-then-flushed follow-up in either order; the reminder and a push in
the same tick likewise; a streaming-time push is unaffected. Amend both
Phase 2 spec passages. Owner-run live check: after a child push wakes an
idle lead, a second model call in that run still sees the manual block
and a fresh skill list.
