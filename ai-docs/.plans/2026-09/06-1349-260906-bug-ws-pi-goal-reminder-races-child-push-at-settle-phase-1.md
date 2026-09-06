# Plan: Pi goal-loop reminder races a child push at agent_settled and spins the runaway backstop — Phase 1: Delay the reminder past settle and guard the start

## Relevant Ticket Contract

- One reminder emission path: the settle timer. `agent_settled` (goal active, lead
  process, not compacting) arms a single timer using `settle_delay_ms` from
  `goal-loop-config.json` (built-in default 5000, read fresh per arm). The timer
  closes over the settle `ctx`, is `unref`'d, and is cancelled by re-arming,
  `agent_start`, `goal-achieved`, `goal-blocked`, force-stop, `/goal` re-arm, and
  `session_shutdown`. Status line reads `Goal loop: settling` while pending.
- Fire condition (evaluated fresh at fire time, not at settle time): reminder
  fires only when `ctx.isIdle()`, `!leadCompactingRef.current`, and
  `!hasRunningAgents(registry)`. Otherwise: yield — no reminder, no streak
  advance, status line stays, the next `agent_settled` re-arms.
- Reminder send: `pi.sendUserMessage(reminder, { deliverAs: "followUp" })`.
- Boundary guard: a `reminderStartPending`-style flag is set immediately before
  that `sendUserMessage` call. `pushToLead` reads it and, when set, sends with
  `triggerTurn: false` instead of the current hardcoded `true`, so a push
  racing the reminder's own `prompt()` pre-run awaits lands in
  `agent.state.messages` via Pi's `_appendCustomMessage` instead of colliding.
  Cleared on `agent_start`, on `agent_settled`, and by a fallback timeout of
  `settle_delay_ms` (the timeout also re-arms the settle timer and sets status
  `Goal loop: reminder did not start a turn, retrying`).
- Runaway backstop unchanged: `decideOnSettle`'s pure reducer (streak
  increment/reset, force-stop at threshold) itself does not change — only its
  call site moves from the live `agent_settled` handler to the timer's fire
  callback (a fired reminder counts, a yield does not; a cancelled cycle
  leaves `sawToolCallThisCycle` as it was).
- Lands after `260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction`
  Phase 1 (landed `81463a7d`/`45f7c2ef`, review fixes `2c8db8c9`/`60216eaa`).
  Its `releaseAfterCompaction` idle branch must now arm the same settle timer
  instead of sending the reminder directly; its `pendingRearm` payload
  (failure reason) is consumed by the timer's reminder build. Its
  `leadCompactingRef`, `HeldPush.deliverAs`, `settleSwallowedWhileCompacting`,
  and `GoalLoopShutdownHandle` are consumed as shipped, not re-specified.
- Constraints: `agents-plugin-pi/` only; no ws-mcp change; no Pi patch. Timer
  and flag logic pure and unit-tested with an injectable clock; IO listeners
  thin glue at factory scope (matching the goal-loop's existing listeners). No
  timer runs in a spawned child process.
- Spec impact (Phase 1 only): amend `{#260904-pi-goal-loop-arming-settled-levers}`
  (settle timer, fire condition, `followUp` send, `reminderStartPending` guard)
  and `{#260904-pi-report-to-lead-channel}` (the `triggerTurn: false` append
  rule while a reminder start is pending).

## Out of Scope

- Phase 2 (idle-time push hold, one-line wake, compose-all-reasons release,
  `agent_start` flush, ws-block-on-push-woken-runs fix) — separate phase in
  the same ticket, not touched here.
- The "async explore needs no code" note — documentation-only, no source
  change.
- The upstream Pi `_runAgentPrompt` `finally` desync — filed separately per
  the ticket; the adapter fix stands without it.
- The accepted `/compact`-while-armed race window from the prerequisite
  compaction ticket — unchanged, not re-litigated here.
- Any change to `goal-compact-and-continue`'s carry-forward wording (that is
  the compaction ticket's own Phase 2, not this ticket).

## Codebase Findings

- `agents-plugin-pi/src/goal-loop.ts#L322-348` — `decideOnSettle` pure reducer.
  Reuse unchanged; only its call site moves (settle handler -> timer fire
  callback). Its `yielding`/`compacting` params can both stay `false` (their
  defaults) at the new call site, since the fire-time gate already excludes
  those cases before `decideOnSettle` is ever reached.
- `agents-plugin-pi/src/goal-loop.ts#L86-153` — `GoalLoopConfig`,
  `readGoalLoopConfig`, `resolveRunawayThreshold`/`resolveCompactionAdvisoryPercent`
  never-hard-fail resolver shape to mirror for a new `settle_delay_ms` /
  `resolveSettleDelayMs` / `DEFAULT_SETTLE_DELAY_MS = 5000`.
- `agents-plugin-pi/src/goal-loop.ts#L411-636` — `registerGoalLoop`'s full IO
  glue: `pendingRearm`, `settleSwallowedWhileCompacting`,
  `dispatchSettleDecision`, `releaseAfterCompaction`, the `/goal` command, the
  `agent_settled`/`agent_start`/`session_before_compact`/`session_compact`/
  `session_compact_failed` listeners, and the three tools. This whole block is
  the restructuring target.
- `agents-plugin-pi/src/goal-loop.ts#L820-827` — `GoalLoopShutdownHandle`
  return value; `resetCompactionStateForShutdown` is the existing extension
  point `index.ts`'s `session_shutdown` already calls — extend it, do not add
  a second handle method, so the call site in index.ts needs no signature
  change.
- `agents-plugin-pi/src/spawner.ts#L1216-1282` — `leadIdleRef`,
  `leadCompactingRef`, `isOwningAgentIdle()`: the exact `lead*Ref`-in-spawner.ts
  convention (owned/read here, mutated by goal-loop.ts) to mirror for the new
  boundary-guard ref.
- `agents-plugin-pi/src/spawner.ts#L1336-1368` (`sendPush`) and `#L1476-1492`
  (`pushToLead`) — `sendPush` currently hardcodes `{ deliverAs, triggerTurn:
  true }`. This is the single place to read the new ref and override
  `triggerTurn` to `false` when set, so neither `pushToLead`'s nor
  `flushHeldPushes`'s call sites need a new parameter threaded through.
- `agents-plugin-pi/src/spawner.ts#L1132-1141` (`computeFanIn`) and
  `#L1182-1184` (`hasRunningAgents`) — reuse as-is; call fresh at fire time
  (not at settle time) via `hasRunningAgents(opts.rpcRegistryRef?.current)`.
- `agents-plugin-pi/src/spawner.ts#L1601-1615` (`startLivenessProbe`) — the
  only existing timer in this codebase; uses a bare `setInterval`/`unref`
  with a real (small, e.g. `1`ms in tests) interval — **no existing
  injectable-clock/fake-timer seam anywhere in the repo**. The new settle
  timer needs its own seam (see Implementation step 4b).
- `agents-plugin-pi/src/index.ts#L297` (`registerGoalLoop(pi, {
  goalLoopConfigPath, rpcRegistryRef })`) and `#L507-548` (`session_shutdown`,
  calls `goalLoopHandle.resetCompactionStateForShutdown()` at L539) — no
  signature change needed here; the injectable timer option is test-only and
  must default silently in production.
- `agents-plugin-pi/test/goal-loop.test.ts#L497-1010` — the
  `"registerGoalLoop IO glue (fake pi): compaction release (260906 Phase 1)"`
  describe block. Its `fakePi()`/`fakeCtx()` harness (L516-577) has no timer
  concept; nearly every test in this block asserts an *immediate*
  `sentUserMessages` push right after `agent_settled` / lever `onComplete` /
  `onError` / `session_compact`. Introducing the timer breaks essentially all
  of these assertions — they need rewriting to "trigger event -> assert
  nothing sent yet, status is `Goal loop: settling` -> fire the fake timer ->
  assert the send", not just new tests appended alongside.
- `agents-plugin-pi/test/spawner.test.ts#L1149-1226` (`describe("pushToLead"...)`)
  and `#L1243+` (hold tests) — pattern to extend with a `triggerTurn: false`
  assertion gated on the new ref, and a `beforeEach`/`afterEach` reset for it
  (mirroring the existing `leadCompactingRef`/`heldPushQueue` resets).
- `agents-plugin-pi/goal-loop-config.json` — currently `{}`; no edit needed,
  a missing `settle_delay_ms` key already falls back to the default via the
  never-hard-fail resolver pattern.
- `ai-docs/spec/pi-adapter-runtime.md#L1115-1176` (`{#260904-pi-goal-loop-arming-settled-levers}`)
  and `#L559-620` (`{#260904-pi-report-to-lead-channel}`) — the two passages
  Phase 1 must amend, per the ticket's Spec Impact section.
- Pi runtime verification (mechanism confirmed against installed source, all
  claims in the ticket's Background hold):
  - `node_modules/@earendil-works/pi-coding-agent/dist/core/agent-session.js#L347-356`
    (`_emitAgentSettled` sets `_isAgentRunActive = false` before awaiting
    extension `agent_settled` handlers).
  - `#L772-786` (`_runAgentPrompt`'s `finally` always calls `_emitAgentSettled`,
    even on a throw).
  - `#L821-872` (`prompt()`: compaction-abort throw first, then an awaited
    `emitInput`, THEN the `isStreaming` check at L860, then further awaits —
    a streaming session without `streamingBehavior` throws `"Agent is already
    processing..."`).
  - `#L1099-1139` (`sendCustomMessage`: with `options.triggerTurn === false`
    and `!isStreaming`, falls through to `_appendCustomMessage`, which
    synchronously pushes onto `agent.state.messages` and emits
    `message_start`/`message_end` — no turn started).
  - `#L1153-1189` (`sendUserMessage` always calls `this.prompt(text, {
    streamingBehavior: options?.deliverAs, ... })` — confirmed no
    `triggerTurn` option exists on `sendUserMessage` at all, only on
    `sendMessage`/`sendCustomMessage`).
  - `dist/core/extensions/types.d.ts#L971-983` — confirms the same shape in
    the public extension-facing API: `sendMessage` takes `{ triggerTurn?,
    deliverAs? }`; `sendUserMessage` takes only `{ deliverAs?,
    expandPromptTemplates? }`.
  - Net effect: a push sent with `triggerTurn: false` while the reminder's own
    `sendUserMessage` is mid-await (not yet streaming) appends synchronously
    and ahead of the reminder's own user-message append (which happens later,
    after several awaits inside `prompt()`), matching the ticket's ordering
    claim ("appears before the reminder in the session") without further
    verification needed.

## Implementation Plan

1. `agents-plugin-pi/src/goal-loop.ts` (~L86-153): add `settle_delay_ms?:
   number` to `GoalLoopConfig`, `DEFAULT_SETTLE_DELAY_MS = 5000`, and
   `resolveSettleDelayMs(config)` mirroring `resolveRunawayThreshold`'s
   never-hard-fail shape (positive finite number, else default).
2. `agents-plugin-pi/src/spawner.ts` (~L1232, beside `leadCompactingRef`):
   add `export const leadReminderStartPendingRef: { current: boolean } = {
   current: false };` with a doc comment stating it is owned/mutated by
   goal-loop.ts's settle timer and read here by `pushToLead`/`sendPush`, plus
   its three clear points (for context, not enforced here).
3. `agents-plugin-pi/src/spawner.ts` (~L1336-1368, `sendPush`): read
   `leadReminderStartPendingRef.current` and send with `triggerTurn: false`
   when it is `true`, `true` otherwise — no new parameter needed on
   `sendPush`/`pushToLead`/`flushHeldPushes`, since this is a module-level
   read at send time.
4. `agents-plugin-pi/src/goal-loop.ts`, `registerGoalLoop` (~L411-636):
   restructure so the settle timer is the single reminder-dispatch path.
   a. Closure state: `let settleTimer: NodeJS.Timeout | undefined;` and
      `let reminderTimeoutHandle: NodeJS.Timeout | undefined;` (the
      boundary-guard fallback timer). `cancelSettleTimer()` clears/undefines
      `settleTimer`. Keep `pendingRearm` and `settleSwallowedWhileCompacting`
      as closure state (unchanged), but stop clearing them inside
      `releaseAfterCompaction` — they must survive until the timer's fire
      callback consumes them (see 4f).
   b. Add an injectable timer seam to `RegisterGoalLoopOptions` (test-only —
      production callers omit it), e.g. `scheduleTimer?: (cb: () => void, ms:
      number) => NodeJS.Timeout; clearTimer?: (h: NodeJS.Timeout) => void`,
      defaulting to real `setTimeout`/`clearTimeout` with `.unref?.()` called
      on the real handle (mirrors `startLivenessProbe`'s `timer.unref?.()`).
      Tests pass a fake pair that records `{ cb, ms }` and lets the test
      invoke `cb()` directly instead of waiting — this is the "fake clock"
      seam the ticket requires, with no new dependency.
   c. `armSettleTimer(ctx)` helper: `cancelSettleTimer()`; read
      `resolveSettleDelayMs(readGoalLoopConfig(opts.goalLoopConfigPath))`;
      `settleTimer = scheduleTimer(() => onSettleTimerFire(ctx), delayMs)`;
      `ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: settling")`.
   d. `onSettleTimerFire(ctx)`: `settleTimer = undefined`; recompute
      `notIdle = !ctx.isIdle()`, `compacting = leadCompactingRef.current`,
      `yielding = hasRunningAgents(opts.rpcRegistryRef?.current)`; if any of
      the three is true, return without sending, without touching `state`,
      and without changing the status line (ticket: "the status line
      stays") — the next live `agent_settled` re-arms. Otherwise: if
      `pendingRearm` is true, build the lever reminder (existing wording +
      failure-reason fold-in logic from the current `releaseAfterCompaction`
      pendingRearm branch), clear both `pendingRearm` and
      `settleSwallowedWhileCompacting`; else if `settleSwallowedWhileCompacting`
      is true, run `decideOnSettle(state, threshold)` (defaults `false,
      false`), update `state`, clear the marker, and dispatch its decision
      (reinject/force-stop) exactly as `dispatchSettleDecision` does today for
      those two outcomes; else (ordinary live-settle arm) run the same
      `decideOnSettle(state, threshold)` call. On a `reinject` outcome: set
      `leadReminderStartPendingRef.current = true`, call
      `pi.sendUserMessage(reminder, { deliverAs: "followUp" })`, then arm
      `reminderTimeoutHandle = scheduleTimer(() => { if
      (leadReminderStartPendingRef.current) {
      leadReminderStartPendingRef.current = false; ctx.ui.setStatus(KEY,
      "Goal loop: reminder did not start a turn, retrying");
      armSettleTimer(ctx); } }, delayMs)`. On `force-stop`: notify + clear
      status as `dispatchSettleDecision` does today.
   e. `agent_settled` handler: at the very top (before the child-process
      guard is fine either order, but before any other logic), clear
      `leadReminderStartPendingRef.current = false` (clear point "on
      agent_settled" — also cancel `reminderTimeoutHandle` if set, since a
      real settle proves the reminder's run at least started). Then:
      unchanged child-process guard; `if (!state.active) return;`; `if
      (leadCompactingRef.current) { settleSwallowedWhileCompacting = true;
      ctx.ui.setStatus(KEY, "Goal loop: waiting for compaction"); return; }`;
      else `armSettleTimer(ctx)`. Remove the old `decideOnSettle`/
      `dispatchSettleDecision(ctx, config, decision)` call from this handler
      entirely — it moved to `onSettleTimerFire`.
   f. `releaseAfterCompaction`'s idle branch: after `flushHeldPushes(pi)`,
      replace the two direct-send branches (`pendingRearm` block and
      `settleSwallowedWhileCompacting` block) with a single `if (pendingRearm
      || settleSwallowedWhileCompacting) armSettleTimer(ctx);` — do NOT clear
      either marker here (moved to `onSettleTimerFire`, step 4d). The
      not-idle branch keeps clearing both markers exactly as today (nothing
      to arm; that turn's own settle re-evaluates via 4e).
   g. `agent_start` handler: add, alongside the existing compacting backstop
      (unconditional, before the `isChildProcess`/`state.active` checks),
      `cancelSettleTimer()` and clear `leadReminderStartPendingRef.current =
      false` (also cancel `reminderTimeoutHandle` if set) — clear point "on
      agent_start".
   h. `/goal` command handler: call `cancelSettleTimer()` before `state =
      armGoal(goal)` (re-arm cancel point).
   i. `goal-achieved`/`goal-blocked` tool `execute()`: call
      `cancelSettleTimer()` alongside `state = disarmGoal()`.
   j. `resetCompactionStateForShutdown()` (rename optional, keep the name if
      simplest since `index.ts` calls it by that name): also call
      `cancelSettleTimer()`, clear `reminderTimeoutHandle` (clearTimer +
      undefine), and clear `leadReminderStartPendingRef.current = false`.
5. `agents-plugin-pi/src/index.ts`: no code change expected — verify after
   step 4 that `goalLoopHandle.resetCompactionStateForShutdown()` at L539
   still compiles against the extended handle (same method name, extended
   body only).
6. Tests, `agents-plugin-pi/test/goal-loop.test.ts`:
   a. Add a fake timer harness beside `fakePi()`/`fakeCtx()` (~L516-577):
      records scheduled `{ cb, ms }` pairs keyed by insertion order (or a
      small map if the test needs to fire a specific one), exposes a `fire()`
      (or `fireLatest()`) helper the test calls to invoke a pending callback
      synchronously, and a `pendingCount()`/`cancelled` list for the
      cancel-point tests. Pass it into every `registerGoalLoop(pi.api, {
      goalLoopConfigPath, scheduleTimer, clearTimer })` call in this
      describe block.
   b. Rewrite every existing test in the `"registerGoalLoop IO glue"` block
      (~L497-1010) whose assertion is "N sent messages immediately after
      event X" to the two-step shape: trigger X, assert the count is
      unchanged (and status is `Goal loop: settling` where relevant), fire
      the fake timer, then assert the count/content.
   c. Add the ticket's Phase 1 test list: settle + a push before the delay
      yields and re-arms (no send at fire time when a running child appears
      in the interim); settle alone fires exactly once at the delay; each of
      `agent_start` / `goal-achieved` / `goal-blocked` / force-stop / `/goal`
      re-arm / `resetCompactionStateForShutdown` cancels a pending timer
      (assert `clearTimer` was called / `fire()` on the cancelled handle is a
      no-op); a running child or `leadCompactingRef.current` true AT FIRE
      TIME yields (status/streak untouched); `releaseAfterCompaction`'s idle
      branch arms the timer and the eventually-fired reminder carries the
      `pendingRearm` wording; a push sent (via a fake `pushToLead`-shaped call
      into the harness, or by asserting `leadReminderStartPendingRef.current`
      directly around a `sendMessage` call) while the flag is set carries
      `triggerTurn: false` and its message precedes the reminder in
      `sentMessages`/`sentUserMessages` ordering; the flag clears on
      `agent_start`, on `agent_settled`, and by timeout-with-no-event (assert
      the retry status text and that the settle timer re-arms); the streak
      advances only on fired reminders (yielded/cancelled ticks leave
      `noToolCallStreak` untouched — reuse the existing streak-advance
      assertion pattern already in this file, e.g. L811-818).
7. Tests, `agents-plugin-pi/test/spawner.test.ts` (~L1149-1226): add a
   `beforeEach`/`afterEach` reset of `leadReminderStartPendingRef.current`
   (mirroring the existing `leadCompactingRef`/`heldPushQueue` resets nearby),
   and a test asserting `pushToLead(..., "followUp")` sends with `{
   triggerTurn: false }` when the ref is `true`, and the existing `{
   triggerTurn: true }` behavior (L1162) is unchanged when it is `false`.
8. Spec: amend `ai-docs/spec/pi-adapter-runtime.md`
   `{#260904-pi-goal-loop-arming-settled-levers}` (~L1115-1176) with the
   settle-timer mechanism (config key + default, the five/six cancel points,
   the `Goal loop: settling` status, the fire condition re-evaluated at fire
   time, the `followUp` send, and the `reminderStartPending` guard with its
   three clear points and retry-timeout behavior), and
   `{#260904-pi-report-to-lead-channel}` (~L559-620) with the `triggerTurn:
   false` append rule while a reminder start is pending.

## Verification Plan

- `cd agents-plugin-pi && node --test test/goal-loop.test.ts test/spawner.test.ts`
  first (fast, targeted), then `npm test` (full suite, currently 836 passing
  per the prerequisite ticket's Result) to confirm no regressions elsewhere.
- Manual-only, ticket-specified, not automatable in this survey: owner-run
  live check — a `/goal` drain where a child settles at the same moment as
  the lead; confirm no `Agent is already processing a prompt` error, Esc
  still interrupts, and the `Goal loop: settling` status is visible during
  the delay window.

## Escalations

- None.
