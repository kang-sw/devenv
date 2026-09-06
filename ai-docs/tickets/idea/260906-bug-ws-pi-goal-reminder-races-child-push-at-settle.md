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

Adapter-only change under `agents-plugin-pi/` (golden rule: no ws-mcp
change). Pi is not patched; the adapter serializes its own wakes.

- **One lead wake path.** New module `src/lead-wake.ts` exporting
  `createLeadWake(pi)` and a `wakeLead(request)` function. Every
  adapter-initiated lead turn start goes through it: the goal-loop reminder
  (`goal-loop.ts`), `pushToLead` (`spawner.ts`), the `ws-approve` decision
  push in `ask.ts`, and the approval relay's `sendUserMessage` in
  `execute-gateway.ts`. Owner-typed input and the user-invoked
  `/goal` / `/ws-discuss` kickoffs (already `isIdle`-checked) stay as they
  are.
- **Rules.** With a `pending` flag owned by the module:
  - Pi streaming (`ctx.isIdle()` false): deliver as `followUp` (or the
    caller's `steer`) through the existing API; Pi queues it, nothing can
    throw.
  - Pi idle and no wake pending: set `pending`, start the turn through the
    caller's existing API (`sendUserMessage` for the reminder,
    `sendMessage` + `triggerTurn` for pushes).
  - Pi idle and a wake pending: hold the request in an adapter-local FIFO.
    The `agent_start` event clears `pending` and flushes the FIFO as
    `followUp` messages (Pi is streaming by then, so they queue).
  - Fallback: `agent_settled` with `pending` still set means the start
    failed before `agent_start`; clear `pending` and, if the FIFO is
    non-empty, start the next request. This is the only place a second
    start can be issued, and it runs strictly after the first attempt has
    settled.
  - Order within one settle: the goal-loop reminder is issued from
    `agent_settled` and a child push can arrive in the same tick; whichever
    reaches `wakeLead` first starts the turn, the other rides as a
    `followUp` on it. Both reach the model in the same turn, which is the
    behavior the goal loop already assumes ("a persistent child pushing its
    own settle/report wakes the lead next").
- **Pure core.** The decision (`start` / `queue-followup` / `hold`) is a
  pure function of `{idle, pending, fifoLength}`, unit-tested; the
  `agent_start`/`agent_settled` listeners and the API calls are thin glue,
  registered at factory scope like the goal-loop's own listeners.
- **Phase 2, ws block on push-woken turns.** When the wake path starts a
  turn for a custom push while idle, it delivers the custom message as
  `deliverAs: "nextTurn"` (Pi injects `_pendingNextTurnMessages` into the
  next `prompt()` alongside the user content, before `before_agent_start`)
  and starts the turn with a one-line `sendUserMessage` wake text. The
  turn then runs through `prompt()`, `before_agent_start` fires, and the
  ws block is present. The push keeps its custom rendering (`customType`,
  `display`, `details`), so `push-render` output is unchanged; the extra
  wake line is the only visible addition. Streaming-time pushes keep the
  plain `followUp` path since the running turn already carries the block.
- Rejected: an adapter-side mutex around `sendUserMessage` alone. It
  cannot see when Pi flips `_isAgentRunActive` inside `prompt()`'s awaits,
  so it would still let a custom push start a run in that window. Rejected:
  moving the reminder to the custom-message path to get the atomic
  check-and-start; that would drop `before_agent_start` for the reminder
  turn, which is the goal loop's main turn.
- Upstream: file the `_runAgentPrompt` `finally` desync with Pi separately
  (owner's call); the adapter fix stands on its own either way.

## Spec Impact

`pi-adapter-runtime` `{#260904-pi-goal-loop-arming-settled-levers}`: add a
passage stating that adapter-initiated lead wakes are serialized through one
path so that no two turn starts overlap, naming the four callers, the
pending/FIFO rule, and the `agent_start`/`agent_settled` clearing. The
`{#260905-pi-lead-bootstrap-system-prompt}` passage gains one sentence
(Phase 2): push-woken idle turns go through `prompt()` so the block is
present on them too.

## Constraints

- `agents-plugin-pi/` only; no ws-mcp change; no Pi patch.
- Push rendering and the model-facing content of pushes and reminders are
  unchanged; Phase 2 adds only the one-line wake text.
- The runaway backstop semantics stay as they are; a settle emitted by a
  failed start is still counted, but with serialized wakes such a settle no
  longer occurs.
- Existing tests for `goal-loop`, `spawner` push paths, `ask`, and
  `execute-gateway` keep passing with the wake path substituted.

## Phases

### Phase 1: Serialize adapter-initiated lead wakes

Add `src/lead-wake.ts` with the pure decision function and the
`agent_start`/`agent_settled` glue; route the goal-loop reminder,
`pushToLead`, the `ws-approve` decision push, and the approval relay through
it. Tests: pure decision matrix (idle/pending/fifo); reminder and push
issued in the same settle produce exactly one start and one followUp in
either arrival order; a failed start (settle before `agent_start`) releases
the next held request; a streaming-time push goes straight to followUp.
Amend the goal-loop spec passage. Owner-run live check: a `/goal` drain
where a child settles at the same moment as the lead; no
`already processing` error, Esc still interrupts.

### Phase 2: Carry the ws block on push-woken turns

Deliver an idle-time custom push as `nextTurn` plus a one-line
`sendUserMessage` wake so the turn runs through `prompt()` and
`before_agent_start`. Tests: an idle-time push results in one `nextTurn`
custom message and one user wake; the fake `before_agent_start` handler
runs for that turn; a streaming-time push is unaffected. Amend the bootstrap
spec passage. Owner-run live check: after a child push wakes an idle lead,
the lead can still call `ws-skill` and sees the manual block.
