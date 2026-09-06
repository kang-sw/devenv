---
title: Pi goal-loop reminder races a child push at agent_settled and spins the runaway backstop
spec:
  - pi-adapter-runtime
related:
  - 260906-bug-ws-pi-lead-cannot-see-or-load-skills
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
must not fire. The existing yield gate is one instance of that rule; this
ticket makes the rule complete and closes the remaining overlap.

Adapter-only change under `agents-plugin-pi/` (golden rule: no ws-mcp
change). Pi is not patched.

- **Waker inventory.** A pure function `pendingWakers(registry, wake)`
  returns the set of things that can still wake the lead:
  - an RPC child that is `running` and not `terminalThisTurn` (today's
    gate);
  - an RPC child whose settle push is still in flight: the settle IIFE
    increments a `settleInFlight` counter on the record (or on the
    registry) synchronously in the same tick that clears `running`, and
    decrements it after `pushToLead` (or after deciding to suppress the
    push). This closes the `harvestLastMessage` window;
  - a gated command awaiting `ws-approve` (`record.pendingApproval`): the
    child is blocked and still `running`, so it is already counted, but the
    inventory names it explicitly so the rule survives a future change to
    how approvals are tracked;
  - a wake already queued in the adapter wake path (below).
  Async explore is not a waker: its completion is only ever harvested on a
  later call, never pushed, so the reminder is the only thing that brings
  the lead back to poll it. It stays outside the inventory, with a note in
  the spec that making it a push would move it inside.
- **Goal loop yields to any waker.** `decideOnSettle`'s `yielding` input
  becomes `pendingWakers(...).size > 0`. The yield path is unchanged
  otherwise (no reminder, no streak advance, status line set, cleared on
  the next `agent_start`).
- **Wake path for everything else.** `src/lead-wake.ts` serializes the
  adapter's non-goal wakes so they cannot overlap each other or the
  reminder: `pushToLead`, the `ws-approve` decision push (`ask.ts`), the
  approval relay (`execute-gateway.ts`), and the reminder itself. Rules,
  with a `pending` flag and a FIFO owned by the module:
  - Pi streaming: deliver as `followUp` (or the caller's `steer`); Pi
    queues, nothing throws.
  - Pi idle, nothing pending: set `pending`, start the turn through the
    caller's API.
  - Pi idle, a start pending: hold in the FIFO; `agent_start` clears
    `pending` and flushes the FIFO as `followUp`.
  - `agent_settled` with `pending` still set means the start failed before
    `agent_start`; clear it and start the next held request.
  A queued or pending wake is itself a waker for the inventory above, so
  the reminder never fires while one is outstanding.
- **Phase 2, ws block on push-woken turns.** A turn started by a custom
  push while the lead is idle bypasses `prompt()` and therefore
  `before_agent_start`; `registerLeadBootstrap`'s `<ws>` block is missing
  from it. The wake path starts such turns by delivering the custom message
  as `deliverAs: "nextTurn"` and issuing a one-line `sendUserMessage` wake,
  so the turn runs through `prompt()`. Push rendering (`customType`,
  `display`, `details`) is unchanged; the wake line is the only visible
  addition. This is why the serializer is needed even with a complete
  inventory: once pushes go through `prompt()`, two children settling in
  the same tick would race each other in `prompt()`'s await window.
- Rejected: fixing only the inventory. It closes the observed race but
  leaves two non-goal wakes free to overlap (approval relay against a
  push today; push against push after Phase 2). Rejected: moving the
  reminder to the custom-message path for its atomic check-and-start; it
  would drop `before_agent_start` for the goal loop's main turn.
- Upstream: the `_runAgentPrompt` `finally` desync (clearing a flag it
  never set, emitting a settle for a run that never began) is Pi's; file
  it separately at the owner's call. The adapter fix stands without it.

## Spec Impact

`pi-adapter-runtime` `{#260904-pi-goal-loop-arming-settled-levers}`: state
the lowest-priority rule, the waker inventory (running child, settle push
in flight, pending approval, queued wake; async explore excluded and why),
and the wake path's pending/FIFO rule with its `agent_start`/`agent_settled`
clearing. `{#260905-pi-lead-bootstrap-system-prompt}` gains one sentence
(Phase 2): push-woken idle turns go through `prompt()` so the block is
present on them too.

## Constraints

- `agents-plugin-pi/` only; no ws-mcp change; no Pi patch.
- Push rendering and the model-facing content of pushes and reminders are
  unchanged; Phase 2 adds only the one-line wake text.
- Runaway backstop semantics stay as they are.
- The inventory is pure and unit-tested; IO listeners are thin glue at
  factory scope, matching the goal-loop's own listeners.

## Phases

### Phase 1: Goal loop yields to every waker; serialize adapter wakes

Add the `settleInFlight` marker to the settle IIFE, the pure
`pendingWakers` inventory, and `src/lead-wake.ts`; feed the inventory into
`decideOnSettle`'s `yielding`; route the reminder, `pushToLead`, the
`ws-approve` decision push, and the approval relay through the wake path.
Tests: inventory matrix (running child, settle in flight, pending approval,
queued wake, async explore only, nothing); a lead settle landing between a
child's `agent_settled` event and its push yields instead of re-injecting;
reminder and push in the same settle produce exactly one start and one
followUp in either arrival order; a failed start releases the next held
request; a streaming-time push goes straight to followUp. Amend the
goal-loop spec passage. Owner-run live check: a `/goal` drain where a child
settles at the same moment as the lead; no `already processing` error, Esc
still interrupts.

### Phase 2: Carry the ws block on push-woken turns

Deliver an idle-time custom push as `nextTurn` plus a one-line
`sendUserMessage` wake so the turn runs through `prompt()` and
`before_agent_start`. Tests: an idle-time push results in one `nextTurn`
custom message and one user wake; the fake `before_agent_start` handler
runs for that turn; two pushes in the same tick still produce one start and
one followUp; a streaming-time push is unaffected. Amend the bootstrap spec
passage. Owner-run live check: after a child push wakes an idle lead, the
lead can still call `ws-skill` and sees the manual block.
