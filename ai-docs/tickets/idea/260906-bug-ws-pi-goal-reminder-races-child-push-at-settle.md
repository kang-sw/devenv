---
title: Pi goal-loop reminder races a child push at agent_settled and spins the runaway backstop
spec:
  - pi-adapter-runtime
related:
  - 260906-bug-ws-pi-lead-cannot-see-or-load-skills
  - 260906-feat-ws-pi-lead-explore-as-async-rpc-child
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
`<available_skills>`) from that hook, and Pi resets `agent.state.systemPrompt`
to the base prompt at the end of every run. A push-woken turn therefore runs
on the bare base system prompt without the ws block. This is the same wake
path and belongs to the same fix, so it is Phase 2 rather than a separate
ticket.

## Proposed direction

Principle, set by the owner on 2026-09-06: the goal-loop reminder is the
lowest-priority wake. It exists only to keep the lead moving when nothing
else will; whenever any other item could still wake the lead, the reminder
must not fire. Owner's chosen shape (same day): a deliberate settle delay.
The reminder is not issued from `agent_settled` itself but from a timer
armed there; when the timer fires, the reminder goes out only if the lead
is still idle and every other wake condition is exhausted.

Adapter-only change under `agents-plugin-pi/` (golden rule: no ws-mcp
change). Pi is not patched.

- **Settle delay.** `agent_settled` (goal mode active, lead process) arms a
  single timer, `settle_delay_ms` from `goal-loop-config.json` (built-in
  default 5000, read fresh per settle like the threshold). Re-arming
  cancels the previous timer; `agent_start`, `goal-achieved`,
  `goal-blocked`, and force-stop cancel it. While the timer is pending the
  status line reads `Goal loop: settling`. The delay absorbs every
  short-lived post-settle wake the adapter issues (the `harvestLastMessage`
  window, the liveness probe, auto-park, a child's own final report) and
  any it does not yet know about, without an in-flight counter.
- **Fire condition at the timer.** The reminder is issued only when
  `ctx.isIdle()` is true AND no other wake condition remains: no RPC child
  `running` and not `terminalThisTurn` (today's gate), and no
  `record.pendingApproval` awaiting `ws-approve`. Otherwise the tick is a
  yield: no reminder, no streak advance, the status line stays, and the
  next `agent_settled` (from whichever wake did fire) re-arms the timer.
  Async explore is not a wake condition: its completion is never pushed,
  only harvested on a later call, so the reminder is the only thing that
  brings the lead back to poll it. This carve-out disappears once
  `260906-feat-ws-pi-lead-explore-as-async-rpc-child` turns the lead-side
  explore into an RPC child.
- **Boundary guard.** The delay shrinks the race to the reminder's own
  `prompt()` await window (auth check, compaction check,
  `before_agent_start`). A push landing there would still collide, and the
  failure is severe, so it is closed outright: the goal loop sets a
  `reminderStartPending` flag immediately before `pi.sendUserMessage` and
  clears it on `agent_start` (and on `agent_settled`, the failed-start
  fallback). `pushToLead` reads that flag through the same
  `leadIdleRef`-style seam and, when set, sends with `triggerTurn: false`:
  Pi appends the custom message to the session at once (the lead is not
  streaming yet), and the reminder turn that starts moments later sees it
  in context. One turn start, no collision.
- **Runaway backstop.** Unchanged: `decideOnSettle`'s streak logic moves to
  the timer callback (a fired reminder counts, a yield does not).
- **Phase 2, ws block on push-woken turns.** A turn started by a custom
  push while the lead is idle bypasses `prompt()` and therefore
  `before_agent_start`; `registerLeadBootstrap`'s `<ws>` block is missing
  from it. Those turns are started by delivering the custom message as
  `deliverAs: "nextTurn"` and issuing a one-line `sendUserMessage` wake, so
  the turn runs through `prompt()`. Push rendering (`customType`,
  `display`, `details`) is unchanged; the wake line is the only visible
  addition. Because pushes then share `prompt()`'s await window, two
  children settling in the same tick would race each other; Phase 2
  therefore also adds the small adapter wake serializer (`src/lead-wake.ts`:
  a `pending` flag plus FIFO; idle and nothing pending starts the turn,
  idle with a start pending holds until `agent_start` and flushes as
  `followUp`, streaming delivers as `followUp` directly, a settle with
  `pending` still set releases the next held request). The reminder and
  the approval relay route through it too.
- Rejected: an in-flight settle counter feeding the yield gate as Phase 1.
  It closes only the windows the adapter already knows about and needs a
  marker on every push site; the delay covers them all. Rejected: moving
  the reminder to the custom-message path for its atomic check-and-start;
  it would drop `before_agent_start` for the goal loop's main turn.
- Upstream: the `_runAgentPrompt` `finally` desync (clearing a flag it
  never set, emitting a settle for a run that never began) is Pi's; file
  it separately at the owner's call. The adapter fix stands without it.

## Spec Impact

`pi-adapter-runtime` `{#260904-pi-goal-loop-arming-settled-levers}`: state
the lowest-priority rule, the settle delay (config key, default, cancel
points, status line), the fire condition (idle, no running child, no
pending approval; async explore excluded and why), and the
`reminderStartPending` guard on `pushToLead`. Phase 2 adds the wake
serializer's pending/FIFO rule there and one sentence under
`{#260905-pi-lead-bootstrap-system-prompt}`: push-woken idle turns go
through `prompt()` so the block is present on them too.

## Constraints

- `agents-plugin-pi/` only; no ws-mcp change; no Pi patch.
- Push rendering and the model-facing content of pushes and reminders are
  unchanged; Phase 2 adds only the one-line wake text.
- Runaway backstop semantics stay as they are.
- Timer and flag logic is pure and unit-tested with an injectable clock;
  IO listeners are thin glue at factory scope, matching the goal-loop's
  own listeners. No timer runs in a spawned child process.

## Phases

### Phase 1: Delay the reminder past settle and guard the start

Add `settle_delay_ms` to the goal-loop config reader, the timer with its
cancel points and status line, the fire condition, the
`reminderStartPending` flag, and the `triggerTurn: false` branch in
`pushToLead`. Tests (fake clock): a settle followed by a push before the
delay yields and re-arms; a settle with nothing else fires exactly once at
the delay; `agent_start` and each disarm lever cancel a pending timer; a
running child or a pending approval at fire time yields; async explore
alone does not block firing; a push arriving while `reminderStartPending`
is set is sent with `triggerTurn: false` and appears before the reminder
in the session; the streak advances on fired reminders only. Amend the
goal-loop spec passage. Owner-run live check: a `/goal` drain where a
child settles at the same moment as the lead; no `already processing`
error, Esc still interrupts, the settling status is visible.

### Phase 2: Carry the ws block on push-woken turns

Add `src/lead-wake.ts` and route the reminder, `pushToLead`, the
`ws-approve` decision push, and the approval relay through it; deliver an
idle-time custom push as `nextTurn` plus a one-line `sendUserMessage` wake
so the turn runs through `prompt()` and `before_agent_start`. Tests: an
idle-time push results in one `nextTurn` custom message and one user wake;
the fake `before_agent_start` handler runs for that turn; two pushes in the
same tick produce one start and one followUp in either order; a failed
start releases the next held request; a streaming-time push is unaffected.
Amend both spec passages. Owner-run live check: after a child push wakes an
idle lead, the lead can still call `ws-skill` and sees the manual block.
