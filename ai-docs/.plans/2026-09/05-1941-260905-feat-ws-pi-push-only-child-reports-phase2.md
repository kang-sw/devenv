# Plan: 260905-feat-ws-pi-push-only-child-reports — Phase 2: Goal loop yields to live children

## Relevant Ticket Contract

- Phase 2 text (ticket lines 530-548): in `goal-loop.ts`, the armed `agent_settled`
  handler consults the RPC registry (through the Phase 1 `session_start`-filled
  ref, since `registerGoalLoop` runs at factory scope) with the Phase 1 fan-in
  predicate: when N > 0 (some persistent child is mid-turn) it neither
  re-injects the reminder nor advances the runaway streak.
- Record a "yielding to running agents" status via `ctx.ui.setStatus` under its
  own key, cleared on the next lead turn. The footer's agent count belongs to
  `260905-feat-ws-pi-live-agent-widget`'s segment — this phase carries the
  yield wording only, not a running-count footer segment.
- Test coverage required by the ticket text: running child → no re-fire, no
  streak change; children idle, dormant, stopped, or final-reported-this-turn
  → normal re-fire; a `threadBound` respondent alone → normal re-fire; a
  pushed `ws-agent-settled`/`ws-agent-report` arriving while yielding starts
  the turn that continues the goal; a child found dead by the probe while
  yielding ends the yield through its `exited` push.
- Live check: `/goal` a task that spawns a worker and confirm the lead does
  not re-fire until the worker's message lands.
- Depends on Phase 1 (landed: `654f2fe4`, `01dd2824`, `ab9832a4`, `c37f920e`,
  `68150a2c`, plus Editions `9f740c46`, `5b9fa21c`). Phase 1 already exports
  the exact fan-in predicate and the registry ref this phase must reuse — see
  Codebase Findings.

## Out of Scope

- `agents-plugin-tool/`, `agents-plugin/skills/` — untouched per the golden
  rule and this task's explicit instruction.
- The always-visible running-agent footer segment
  (`260905-feat-ws-pi-live-agent-widget`) — this phase's status line is the
  yield wording only, not a running count.
- `ai-docs/spec/pi-adapter-runtime.md`'s goal-loop anchor
  (`{#260904-pi-goal-loop-arming-settled-levers}`). The ticket's own
  `## Spec Impact` section names this anchor for the yield rule, but Phase
  2's own phase text (unlike Phase 1's explicit "Rewrite `pi-lead-guide.md`
  ...") does not list a doc file, and this task's scope is limited to
  `agents-plugin-pi/src/{goal-loop,spawner,index}.ts` and `test/*.test.ts`.
  Flagged under Escalations as a lead-visible gap, not solved here.
- Phase 1's own remaining owner-run live checks (three-worker fan-out, orphan
  gating, etc.) — unrelated to this phase's code.

## Codebase Findings

- `agents-plugin-pi/src/goal-loop.ts#L293-L308` — `decideOnSettle(state,
  threshold)` is the pure reducer the ticket's yield rule must extend. It
  currently returns `{ action: "ignore" }` only when `!state.active`. The
  yield case belongs here (a new branch checked right after the
  `!state.active` early return), not as an ad hoc early return inside the IO
  glue — matches this file's own established convention of extracting every
  settle decision into the pure reducer (see the file's `isChildProcess`
  extraction note at L253-L260, done for the identical reason: automated
  positive/negative coverage instead of a manual spot-check).
- `agents-plugin-pi/src/goal-loop.ts#L276-L279` — `SettleDecision` union to
  extend with a new `{ action: "yield" }` member.
- `agents-plugin-pi/src/goal-loop.ts#L338-L398` — `registerGoalLoop`'s IO
  glue: `pi.on("tool_call", ...)` (L361-L363) and `pi.on("agent_settled",
  ...)` (L365-L387) are the two listeners registered at factory scope. The
  `agent_settled` handler (L365-L387) is where `readGoalLoopConfig` /
  `decideOnSettle` / `ctx.getContextUsage()` are already wired together — the
  yield check and the new `ctx.ui.setStatus` call are added here, before the
  existing `if (decision.action === "ignore") return;` line.
- `agents-plugin-pi/src/goal-loop.ts#L325-L328` — `RegisterGoalLoopOptions`
  currently carries only `goalLoopConfigPath`. Needs a new field for the
  registry ref (see decision below).
- `agents-plugin-pi/src/spawner.ts#L936-L974` — `computeRunningStatusLine`
  IS the Phase 1 fan-in predicate the ticket says to reuse: it loops the
  registry, skipping `record.threadBound || !record.client`, and counts
  `record.running && !record.terminalThisTurn` as N. It returns a formatted
  *string* (or `undefined`), not a boolean/number, so it cannot be consumed
  directly as an "N > 0" gate without string-parsing. A small new export
  sharing the same loop is the correct reuse (see Implementation Plan step 1)
  — parsing the returned string would be a duplicated-logic smell risk and
  is rejected.
- `agents-plugin-pi/src/spawner.ts#L872` — `export type RpcAgentRegistry =
  Map<string, RpcAgentRecord>;` — the type to import into `goal-loop.ts`.
- `agents-plugin-pi/src/spawner.ts#L699-L710` (`running`), `#L711-L720`
  (`terminalThisTurn`), `#L745-L758` (`threadBound`) — the three
  `RpcAgentRecord` fields the fan-in predicate reads; already documented
  in-file with the exact semantics the ticket's yield rule depends on
  (`running` set at `promptAgent` issue time, `terminalThisTurn` cleared on
  the next prompt, `threadBound` excludes an owner-thread record from the
  fan-in entirely).
- `agents-plugin-pi/src/index.ts#L174-L179` — `rpcRegistryRef: { current:
  RpcAgentRegistry | undefined }` already exists at factory scope, created
  specifically because `createApprovalRelay` (also factory-scope-adjacent)
  needs the registry before `registerAgentTools` creates it. This is
  verbatim the "Phase 1 `session_start`-filled ref" the ticket names.
- `agents-plugin-pi/src/index.ts#L226` — `registerGoalLoop(pi, {
  goalLoopConfigPath });` — the call site to extend with the ref. `L279`:
  `rpcRegistryRef.current = agentTools.rpcRegistry;` is where the ref is
  filled, inside `session_start`, confirming `registerGoalLoop`'s handlers
  only see a live registry after the first `session_start` — before that
  (or in a headless/test harness that never ran it), `.current` is
  `undefined` and `hasRunningAgents(undefined)` must resolve to `false`
  (never yield) so goal mode still functions.
- `agents-plugin-pi/src/execute-gateway.ts#L450-L454` — `createApprovalRelay(pi,
  sessionCtx, registryRef?: { current: RpcAgentRegistry | undefined })` is
  the established pattern for a function that needs the registry ref: an
  optional ref parameter that degrades gracefully (comment at L439-L443:
  "a still-empty ref degrades to no status line, never to a crash"). Same
  degrade-gracefully contract applies here: an empty/undefined ref means
  "nothing known to be running", i.e. never yield.
- `agents-plugin-pi/src/lead-bootstrap.ts#L78-L79` — `registerLeadBootstrap(pi,
  wsBlockRef: { current: string | undefined })` — the OTHER established ref
  convention, a bare positional parameter (not inside an options object).
  `registerGoalLoop` already takes a single options object
  (`RegisterGoalLoopOptions`), so adding the new ref as a field on that
  object (rather than a third positional parameter) fits this file's own
  existing shape; this is a legitimate style choice already reflected in the
  Implementation Plan, not a fork the executor should agonize over.
- Pi SDK type defs
  (`@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts#L80`) —
  `setStatus(key: string, text: string | undefined): void` — `undefined`
  clears. `#L550-L553` — `AgentStartEvent { type: "agent_start" }`, "Fired
  when an agent loop starts" — this is the literal "next lead turn" the
  ticket names for clearing the status, and it fires regardless of what
  triggered the turn (owner-typed prompt or a pushed `steer`/`followUp`
  message), so one `pi.on("agent_start", ...)` listener clearing the key
  unconditionally is sufficient — no separate handling per push family is
  needed.
- `agents-plugin-pi/src/spawner.ts#L1143-L1178` (`pushToLead`) and the
  liveness probe (`#L1280-L1305`, `markAgentExited` around `#L1257-L1275`)
  already implement "a pushed message wakes the lead" and "a dead child is
  pushed as `exited`" — both already landed in Phase 1 with `triggerTurn:
  true`. The ticket's "starts the turn that continues the goal" / "ends the
  yield through its exited push" bullets are the STRUCTURAL CONSEQUENCE of
  composing the already-tested push mechanism with this phase's new
  fan-in-gated `decideOnSettle`, not new production code — the ticket's own
  closing sentence confirms this is a **live check**
  (`/goal` + spawn a worker + confirm no re-fire until the worker's message
  lands), not a new unit-testable code path.
- `agents-plugin-pi/test/spawner.test.ts#L480-L501` — `freshRpcRecord` /
  `liveRpcRecord` test-fixture helpers already build the exact
  `RpcAgentRecord` shapes (running/idle/dormant/threadBound/terminalThisTurn)
  needed to test the new predicate; `#L806-L907` is the existing
  `computeRunningStatusLine` describe block these new tests sit beside.
- `agents-plugin-pi/test/goal-loop.test.ts#L322-L374` — the existing
  `decideOnSettle` describe block to extend; its header comment (`#L1-L23`)
  already documents that `registerGoalLoop`'s IO glue is covered only by the
  live gate, not this unit suite — the new `agent_settled`/`agent_start`
  wiring therefore needs no new IO-glue unit test, only pure-function tests.
- `ai-docs/spec/pi-adapter-runtime.md#L867-L899` (goal-loop anchor) — already
  stale independent of this phase (still says `WS_PI_AGENT_CHILD=1`, a marker
  renamed by 260904 Phase 1 per `goal-loop.ts`'s own header comment at
  `#L48-L52`). Confirms the spec-update gap flagged under Out of Scope /
  Escalations is pre-existing drift, not something this phase would newly
  introduce.

## Implementation Plan

1. `agents-plugin-pi/src/spawner.ts` (near `#L964-L974`): extract the shared
   registry walk out of `computeRunningStatusLine` into a private helper,
   e.g. `computeFanIn(registry: RpcAgentRegistry | undefined): { present:
   boolean; running: number }` (same body: skip
   `record.threadBound || !record.client`, count `record.running &&
   !record.terminalThisTurn`). Rewrite `computeRunningStatusLine` to call it
   and keep its exact current return values (no behavior change — existing
   tests at `test/spawner.test.ts#L806-L907` must keep passing unmodified).
   Add a new export, e.g.:
   ```ts
   /** Phase 2 (260905) goal-loop yield predicate: true when N > 0 under the
    * same fan-in walk computeRunningStatusLine uses, so the yield decision
    * can never drift from the pushed status line's own arithmetic. */
   export function hasRunningAgents(registry: RpcAgentRegistry | undefined): boolean {
     return computeFanIn(registry).running > 0;
   }
   ```
2. `agents-plugin-pi/src/goal-loop.ts`:
   - Import `type { RpcAgentRegistry }` and `hasRunningAgents` from
     `./spawner.ts`.
   - Extend `SettleDecision` (`#L276-L279`) with `| { action: "yield" }`.
   - Extend `decideOnSettle` (`#L293-L308`) to accept a third parameter
     `yielding: boolean = false`. Immediately after the existing
     `if (!state.active) return { next: state, decision: { action: "ignore" } };`,
     add: `if (yielding) return { next: state, decision: { action: "yield" } };`
     — state passes through completely unchanged (no streak mutation, no
     `sawToolCallThisCycle` reset), matching "neither re-injects ... nor
     advances the runaway streak" verbatim. Update the function's doc
     comment to describe the new parameter and branch.
   - Add a module constant, e.g. `const GOAL_LOOP_YIELD_STATUS_KEY =
     "ws-goal-loop-yield";`.
   - Extend `RegisterGoalLoopOptions` (`#L325-L328`) with
     `rpcRegistryRef?: { current: RpcAgentRegistry | undefined };` (optional,
     degrade-gracefully — matches `createApprovalRelay`'s
     `registryRef?: { current: RpcAgentRegistry | undefined }` convention at
     `execute-gateway.ts#L450-L454`).
   - In `registerGoalLoop`'s `agent_settled` handler (`#L365-L387`): after
     the existing `isChildProcess` guard, compute
     `const yielding = hasRunningAgents(opts.rpcRegistryRef?.current);`
     and pass it as `decideOnSettle(state, threshold, yielding)`'s third
     argument. Add a branch before the existing `ignore`/`force-stop`
     checks (or alongside them):
     ```ts
     if (decision.action === "yield") {
       ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: yielding to running agents");
       return;
     }
     ```
   - Add a new factory-scope listener (beside the existing `tool_call`
     listener at `#L361-L363`, registered once — never inside
     `session_start`, matching this file's own "no duplicate handlers across
     `/reload`" convention):
     ```ts
     pi.on("agent_start", (_event, ctx) => {
       if (isChildProcess(process.env)) return;
       ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, undefined);
     });
     ```
     This clears the status on the very next turn regardless of what started
     it (owner prompt, or a pushed `steer`/`followUp` message with
     `triggerTurn: true`), satisfying "cleared on the next lead turn"
     without per-push-family special-casing.
3. `agents-plugin-pi/src/index.ts` (`#L226`): change the call site to
   `registerGoalLoop(pi, { goalLoopConfigPath, rpcRegistryRef });` —
   `rpcRegistryRef` is already declared above at `#L179` and in closure scope
   at this call site (created once per extension-factory invocation, filled
   later at `#L279` inside `session_start`), so no reordering is needed.
4. `agents-plugin-pi/test/spawner.test.ts`: add `hasRunningAgents` to the
   import list (`#L77-L110` area) and a new
   `describe("hasRunningAgents (goal-loop yield predicate)", ...)` block
   beside `computeRunningStatusLine`'s (`#L806-L907`), using the existing
   `freshRpcRecord`/`liveRpcRecord` helpers (`#L480-L501`) to cover:
   - empty/absent registry → `false`.
   - one `liveRpcRecord({ running: true })` → `true` ("running child").
   - one dormant record (`freshRpcRecord()`, no `client`) → `false`.
   - one stopped record (`freshRpcRecord({ running: false })`, no `client`)
     → `false`.
   - one `liveRpcRecord({ running: true, terminalThisTurn: true })` (final
     reported this turn) → `false`.
   - one `liveRpcRecord({ running: true, threadBound: true })` ALONE →
     `false` ("a threadBound respondent alone").
   - a mix: one `threadBound` running record plus one ordinary running
     record → `true` (the threadBound one is excluded but the other still
     counts — this is the same arithmetic `computeRunningStatusLine`'s own
     threadBound test already exercises, restated for the boolean form).
5. `agents-plugin-pi/test/goal-loop.test.ts`: extend the `decideOnSettle`
   describe block (`#L322-L374`) with new cases:
   - `yielding: true` on an active goal → `{ action: "yield" }`, and `next`
     is reference-identical (`assert.equal`, not `deepEqual`) to the input
     `state` — no streak increment, no `sawToolCallThisCycle` reset (mirrors
     the existing `recordToolCall` "same object" test style at
     `#L305-L311`).
   - `yielding: true` on an INACTIVE goal → still `{ action: "ignore" }`
     (the inactive check must run first; yielding never resurrects an
     inactive loop).
   - a yield decision followed by a non-yielding settle continues the SAME
     streak state as if the yield call had never happened (call
     `decideOnSettle` twice: once with `yielding: true`, once with
     `yielding: false`, and assert the streak/decision match calling
     `decideOnSettle(state, threshold, false)` directly without the
     intervening yield call) — proves yield is a true no-op pass-through,
     not a hidden streak reset.
   - omitting the third argument still defaults to `false` (a two-argument
     call site, matching every pre-existing call in this file, keeps
     producing the exact same results as before this change) — this can be
     satisfied by leaving the existing tests unmodified and passing (no new
     test strictly required, but note it explicitly if the executor changes
     any existing call site).

## Verification Plan

- `cd agents-plugin-pi && npm test` — must stay green (currently 634/634 per
  the ticket's Phase 1 Edition result) plus the new `hasRunningAgents` and
  `decideOnSettle` yield-branch cases above.
- No `tsc --noEmit` gate exists in this package (Phase 1 Result: "The
  suggested `tsc --noEmit` gate was not added (new dev dependency; owner
  decision)") — do not add one as part of this phase.
- Live check (owner-run, matches the ticket's own closing sentence): `/goal`
  a task that spawns one worker; confirm the lead does not re-fire the
  reminder (no new turn, no streak change) until the worker's
  `ws-agent-report`/`ws-agent-settled` push lands, and confirm the footer
  status shows the yield wording while waiting and clears on the turn the
  push starts.

## Escalations

- None.
