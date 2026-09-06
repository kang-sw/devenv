# Plan: goal-compact-and-continue re-injects the goal reminder before compaction finishes, and the late compaction overwrites the turn it raced — Phase 1: Hold every push while compacting and re-arm the goal on completion

## Relevant Ticket Contract

- Track an in-flight compaction as adapter state (`leadCompactingRef` beside
  `leadIdleRef`), set by the lever before `ctx.compact` and, defensively, by
  `session_before_compact` for any reason; cleared only by the release
  routine.
- `isOwningAgentIdle()` returns `false` while compacting, so `followUp`
  pushes fall into the existing `heldPushQueue`; `steer` pushes gain the same
  hold *only while compacting* (they must keep interrupting an ordinary
  mid-turn run). `HeldPush` records `deliverAs`; `flushHeldPushes` re-sends
  each item with its recorded mode.
- `registerPushFlush`'s `agent_settled` flush is gated on the compacting
  flag (it must not flush into the doomed turn the abort just settled).
- `ask.ts`'s `injectDiscussionSummary` (`ws-thread-summary`, always
  `followUp`+`triggerTurn:true`) is routed through the same hold.
- One idempotent `releaseAfterCompaction(outcome)`, deferred past Pi's own
  compaction flag, with exactly three callers: deferred `session_compact` /
  `session_compact_failed` (via `setImmediate`), the lever's
  `onComplete`/`onError` (backstop), and `agent_start` (clears the flag with
  no reminder, no queue touch). Idle branch: flush held queue, then send a
  pending lever-originated reminder (`deliverAs: "followUp"`) with the
  failure reason folded in when present. Not-idle branch: send nothing —
  leave the held queue and reminder to that turn's own `agent_settled`.
- Reminder is "pending" only for lever-originated compactions (a
  lever-set `pendingRearm` marker, consumed by whichever call sends it).
  Non-lever compactions (owner `/compact`, Pi auto-compaction) clear the
  flag / flush pushes but never synthesize a reminder — the goal loop stays
  observe-only there and its own eventual `agent_settled` produces the next
  ordinary reminder.
- `goal-loop.ts`'s settle reducer (`decideOnSettle`) gains a "waiting for
  compaction" outcome: while compacting, a settle neither re-injects nor
  advances the runaway streak, and sets a status line mirroring the yield
  branch.
- Below-advisory reminder wording changes to explicitly say "do not call
  goal-compact-and-continue".
- Amend three spec anchors in `pi-adapter-runtime` and one guide line in
  `pi-lead-guide.md`.
- Constraints: adapter-only change in `agents-plugin-pi/`, no ws-mcp
  change; the lever stays non-terminal and never disarms the goal; no
  handler sends a prompt from inside a `session_*compact*` event (must
  defer); the pure settle-reducer shape stays unit-testable without a live
  Pi session.

## Out of Scope

- Phase 2 (verbatim carry-forward folded into the reminder heading) — only
  the failure-reason text is Phase 1's concern; do not add a
  `carry_forward`-preserving field beyond what's needed to fold in the
  failure reason.
- The accepted owner-typed `/compact` race window — ticket explicitly
  accepts it as-is; document it in spec/guide prose only, no code change to
  intercept Pi's built-in `/compact`.
- Pi's threshold/overflow auto-compaction keeps its observe-only posture for
  the goal loop itself (only the push-hold applies to it).
- The live dogfood check (owner-run) at the end of Phase 1 — not something
  this plan can execute; note it as a manual follow-up.

## Codebase Findings

- `agents-plugin-pi/src/spawner.ts#L1216` — `leadIdleRef: { current: (() => boolean) | undefined }`, filled by `index.ts`'s `session_start` (`ctx.isIdle`). Add `leadCompactingRef: { current: boolean }` next to it, default `false`, exported the same way.
- `agents-plugin-pi/src/spawner.ts#L1247-1256` — `isOwningAgentIdle()` is a private (non-exported) function reading only `leadIdleRef`. Needs: (a) fold in the compacting flag (`if (leadCompactingRef.current) return false;` before the existing check) and (b) an `export` keyword, since `goal-loop.ts`'s `releaseAfterCompaction` also needs an idle read (or it can use its own `ctx.isIdle()` from the event it's called with — see Implementation step 5).
- `agents-plugin-pi/src/spawner.ts#L1108` — `PushDeliverAs = "steer" | "followUp" | "nextTurn"`; only `"steer"`/`"followUp"` are ever passed to `pushToLead` (confirmed via `grep pushToLead(` across `src/*.ts`).
- `agents-plugin-pi/src/spawner.ts#L1259-1264` — `HeldPush` interface has no `deliverAs` field; `flushHeldPushes` (L1335-1342) hardcodes `"followUp"` on resend. Add `deliverAs: PushDeliverAs` to the interface and thread it through both the push site and the resend site.
- `agents-plugin-pi/src/spawner.ts#L1409-1423` — `pushToLead`'s hold predicate is `deliverAs === "followUp" && !isOwningAgentIdle()`. After folding compacting into `isOwningAgentIdle()`, `followUp` is already covered; add a second branch `deliverAs === "steer" && leadCompactingRef.current` so steer pushes (`ws-agent-approval`, `ws-agent-question`) hold only while compacting, never merely mid-turn.
- `agents-plugin-pi/src/spawner.ts#L1351-1356` — `registerPushFlush`'s `agent_settled` handler calls `flushHeldPushes(pi)` unconditionally once `shouldPushToLead()` passes. Add `if (leadCompactingRef.current) return;` before the flush — this is the "gate the agent_settled flush on the flag" requirement (the abort-triggered settle inside `compact()` fires before Pi's own flag is set, so this is the only thing stopping a premature flush into the doomed turn).
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/core/agent-session.js#L1468-1593` — confirms the race mechanics verbatim: `compact()` calls `await this.abort()` (line 1469, which resolves `waitForIdle()` and fires `agent_settled` at L339-354) **before** setting `_compactionAbortController` (L1470); `prompt()`'s guard (`Cannot submit a prompt while compaction is in progress`, L836-838) only exists on the `prompt()` path, not on `sendCustomMessage`'s non-streaming `_runAgentPrompt` branch (L1099-1132, specifically L1120-1122) — so any `pi.sendMessage(..., {triggerTurn:true})` push during compaction bypasses the guard entirely, exactly as the ticket describes. `session_compact` is emitted at L1540-1548, still with `_compactionAbortController` non-`undefined` until L1558 clears it right after — confirms `setImmediate` deferral is required for a `session_compact` handler to safely send anything.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/core/agent-session.js#L2069-2080` — `ctx.compact()`'s extension wrapper: `onComplete`/`onError` fire only after the awaited `this.compact(...)` fully returns (including its `finally` clearing `_compactionAbortController`), confirming they are safe backstop call sites for `releaseAfterCompaction`.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts#L200-249,#L902,#L914-915` — `SessionCompactEvent`/`SessionCompactFailedEvent` handlers both receive `(event, ctx: ExtensionContext)`; `ExtensionContext` (not just the narrower command context) already exposes `isIdle()` (L232) and `getContextUsage()`/`compact()`, so `releaseAfterCompaction` can use the `ctx` handed to it at each call site directly instead of reaching through `spawner.ts`'s module-level ref.
- `agents-plugin-pi/src/goal-loop.ts#L376-472` — `registerGoalLoop`'s existing IO glue: `agent_settled` (L403-436), `agent_start` (L457-461), `session_before_compact` (L468-472, currently gated entirely on `!state.active`, must instead set the compacting flag unconditionally and keep the advisory-notify gated on `state.active`), and the `goal-compact-and-continue` tool (L510-538, currently fire-and-forget with only `onComplete`/`onError` notifying — needs to set the flag+`pendingRearm` before `ctx.compact` and call `releaseAfterCompaction` from both callbacks).
- `agents-plugin-pi/src/goal-loop.ts#L285-333` — `decideOnSettle(state, threshold, yielding = false)` pure reducer; add a 4th optional `compacting = false` parameter, checked before the `yielding` check (compaction dominates: if compacting, nothing else about running children matters), returning `{ next: state, decision: { action: "waiting" } }` — same no-mutation shape as the existing `yield` branch (L319-321).
- `agents-plugin-pi/src/goal-loop.ts#L212-229` — `buildGoalReminder`'s below-advisory branch (L219, `Context usage: ${Math.round(percent)}% of window.`) needs to become an explicit "do not call goal-compact-and-continue" sentence per the ticket's secondary observation (the trivial-goal dogfood case). The `percent >= advisoryPercent` branch (L217-218) is unchanged.
- `agents-plugin-pi/src/ask.ts#L921-964` — `injectDiscussionSummary` currently calls `pi.sendMessage(...,{deliverAs:"followUp", triggerTurn:true})` unconditionally, with no idle/mid-turn check at all (unlike `pushToLead`) — it has never needed one because it never carries a computed status line, so mid-turn staleness never applied. The new requirement is narrower than a full `pushToLead` reuse: hold it only while compacting (folding it into the general `isOwningAgentIdle()` check is also acceptable and is what the ticket's "Background... holds after this change" line implies, but changes ordinary mid-turn timing for this call site too — see Implementation step 6 for the exact call).
- `agents-plugin-pi/src/index.ts#L317-322` — `session_start` sets `leadIdleRef.current = () => ctx.isIdle()`; no analogous compacting wiring needed here since `leadCompactingRef` is set directly by `goal-loop.ts` (lever + `session_before_compact`), not derived from a per-session ctx closure.
- `agents-plugin-pi/test/spawner.test.ts#L1242-1382` — `describe("pushToLead: holding a mid-turn push...")` is the existing test-harness pattern (`fakePi()`, `leadIdleRef.current`, `heldPushQueue.length = 0` reset in `beforeEach`/`afterEach`) to extend with `leadCompactingRef` reset and new cases.
- `agents-plugin-pi/test/goal-loop.test.ts#L1-56` — pure-only unit suite today; its own doc comment states IO glue (including the goal-compact tool and `session_before_compact`) is covered only by the live `pi --mode json` gate, not this file. The ticket's required Phase 1 tests (release-runs-once, deferred `session_compact`, `onError`-alone release, `agent_start` backstop, held pushes/`ws-thread-summary` while compacting) are IO-glue-level and need a fake-`pi` harness added to this file (mirroring `test/ask.test.ts#L870-922`'s `describe("closeThreadOnDone / injectDiscussionSummary (fake pi)")` shape) — **this is a doc-comment-drift risk worth a one-line update once the harness lands.**
- `ai-docs/spec/pi-adapter-runtime.md#L626-636` — the exact "Delivery is held until the lead's turn settles... `steer` families (approval, question) are never held" paragraph to amend for `{#260904-pi-report-to-lead-channel}`.
- `ai-docs/spec/pi-adapter-runtime.md#L1108-1157` (arming/settled anchor) and `#L1158-1195` (model-driven-compaction anchor, esp. L1167-1169's "the goal then reaches a fresh settle and the existing armed `agent_settled` reminder re-enters" sentence) — the two goal-loop anchors to amend.
- `agents-plugin-pi/pi-lead-guide.md#L41-44` — the verb-routing table rows for `/goal`/`goal-achieved`/`goal-blocked`/`goal-compact-and-continue`; add the one line naming the lever as the documented way to compact under an armed goal (and, per the ticket's accepted-window note, that owner-typed `/compact` still races the reminder).

## Implementation Plan

1. `agents-plugin-pi/src/spawner.ts`: add `export const leadCompactingRef: { current: boolean } = { current: false };` next to `leadIdleRef` (~L1216). Change `isOwningAgentIdle` (~L1248) to `export function isOwningAgentIdle(): boolean { if (leadCompactingRef.current) return false; ... }` (keep its existing try/catch body unchanged below the new guard).
2. `agents-plugin-pi/src/spawner.ts`: add `deliverAs: PushDeliverAs;` to the `HeldPush` interface (~L1259-1264). In `flushHeldPushes` (~L1335-1342) change the hardcoded `"followUp"` to `held.deliverAs`. In `pushToLead` (~L1409-1423), push `deliverAs` onto the held record, and broaden the hold condition to also hold `steer` while `leadCompactingRef.current` is true:
   ```ts
   const holdFollowUp = deliverAs === "followUp" && !isOwningAgentIdle();
   const holdSteerWhileCompacting = deliverAs === "steer" && leadCompactingRef.current;
   if (holdFollowUp || holdSteerWhileCompacting) {
     heldPushQueue.push({ registry, record, family, payload, deliverAs });
     return;
   }
   ```
3. `agents-plugin-pi/src/spawner.ts`: in `registerPushFlush` (~L1351-1356), add `if (leadCompactingRef.current) return;` immediately after the `shouldPushToLead()` guard, before `flushHeldPushes(pi)`.
4. `agents-plugin-pi/src/goal-loop.ts`: `decideOnSettle` (~L311-333) — add a 4th parameter `compacting = false`, checked right after the `!state.active` early return and before the `yielding` check:
   ```ts
   if (compacting) {
     return { next: state, decision: { action: "waiting" } };
   }
   ```
   Add `"waiting"` to the `SettleDecision` union (~L285-289).
5. `agents-plugin-pi/src/goal-loop.ts`: add module-level `let pendingRearm = false;` beside the existing `let state: GoalLoopState = initialGoalLoopState();` inside `registerGoalLoop` (~L377). Implement `releaseAfterCompaction(pi: ExtensionAPI, ctx: ExtensionContext, failureReason?: string): void` as a local function inside `registerGoalLoop` (it needs the closed-over `state`/`pendingRearm`, plus `opts.goalLoopConfigPath`):
   ```ts
   function releaseAfterCompaction(ctx: ExtensionContext, failureReason?: string): void {
     if (!leadCompactingRef.current) return; // idempotent: already released
     leadCompactingRef.current = false;
     if (!ctx.isIdle()) return; // leave the held queue + reminder to that turn's own settle
     flushHeldPushes(pi);
     if (!pendingRearm) return;
     pendingRearm = false;
     if (!state.active || !state.goal) return; // nothing lever-originated to say
     const config = readGoalLoopConfig(opts.goalLoopConfigPath);
     const advisoryPercent = resolveCompactionAdvisoryPercent(config);
     const contextWindowOverride = resolveContextWindowOverride(config);
     const percent = computeContextPercent(ctx.getContextUsage(), contextWindowOverride);
     let reminder = buildGoalReminder(state.goal, { percent, advisoryPercent });
     if (failureReason) {
       reminder = `Compaction failed: ${failureReason}. Do not retry goal-compact-and-continue — call goal-achieved or goal-blocked instead.\n${reminder}`;
     }
     pi.sendUserMessage(reminder, { deliverAs: "followUp" });
   }
   ```
   Use `ctx.isIdle()` from whichever event supplied it (each of the three callers below receives its own `ctx`), so no new export off `spawner.ts` is needed for the idle read itself — only `leadCompactingRef` and `flushHeldPushes` are imported from `spawner.ts` (already importing `hasRunningAgents`/`RpcAgentRegistry` from there — extend that import list).
6. `agents-plugin-pi/src/goal-loop.ts`: wire the three callers:
   - `session_before_compact` listener (~L468-472): set `leadCompactingRef.current = true;` unconditionally as the first line (before the `isChildProcess`/`!state.active` checks), keeping the existing advisory `ctx.ui.notify` gated on `state.active && state.goal` as today.
   - New listeners registered alongside it (still factory scope, not inside `session_start`):
     ```ts
     pi.on("session_compact", (_event, ctx) => {
       setImmediate(() => releaseAfterCompaction(ctx));
     });
     pi.on("session_compact_failed", (event, ctx) => {
       setImmediate(() => releaseAfterCompaction(ctx, event.errorMessage));
     });
     ```
   - `goal-compact-and-continue` tool's `execute` (~L522-537): before calling `ctx.compact(...)`, set `leadCompactingRef.current = true; pendingRearm = true;`. Change `onComplete`/`onError` to also call the release routine:
     ```ts
     onComplete: () => { ctx.ui.notify("Compaction completed", "info"); releaseAfterCompaction(ctx); },
     onError: (error) => { ctx.ui.notify(`Compaction failed: ${error.message}`, "error"); releaseAfterCompaction(ctx, error.message); },
     ```
   - `agent_start` listener (~L457-461): add, before the existing `isChildProcess`/`!state.active` early returns (this backstop must fire even when goal mode is inactive): `if (leadCompactingRef.current) { leadCompactingRef.current = false; return; }`.
7. `agents-plugin-pi/src/goal-loop.ts`: `agent_settled` listener (~L403-436) — read `const compacting = leadCompactingRef.current;` and pass it into `decideOnSettle(state, threshold, yielding, compacting)`. Add a branch for `decision.action === "waiting"`: `ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: waiting for compaction");` — reuse the existing yield status key (it's already cleared unconditionally by the `agent_start` listener on the next real turn, so no new key/cleanup path is needed).
8. `agents-plugin-pi/src/goal-loop.ts`: `buildGoalReminder` (~L212-229) — change the below-advisory branch to:
   ```ts
   : `Context usage: ${Math.round(percent)}% of window — below the compaction advisory point (${advisoryPercent}%); do not call goal-compact-and-continue.`;
   ```
9. `agents-plugin-pi/src/ask.ts`: `injectDiscussionSummary` (~L921-940) — import `leadCompactingRef`, `heldPushQueue`, `isOwningAgentIdle` from `./spawner.ts` (extend the existing import block ~L67-76). Before the `pi.sendMessage(...)` call, hold while the lead is not idle (mid-turn OR compacting, matching `pushToLead`'s `followUp` predicate) by generalizing `heldPushQueue`'s element type to a small discriminated union so a raw pre-built message can share the queue with family-shaped pushes:
   - In `spawner.ts`, change `heldPushQueue: HeldPush[]` to accept `HeldPush | HeldRawSend`, where `HeldRawSend = { kind: "raw"; send: (pi: ExtensionAPI) => void }` and the existing `HeldPush` gets `kind: "push"` added as its discriminant; `flushHeldPushes` dispatches on `item.kind`.
   - In `ask.ts`, when `!isOwningAgentIdle()`, push `{ kind: "raw", send: (pi) => pi.sendMessage(message, { deliverAs: "followUp", triggerTurn: true }) }` and return before doing the rest of `injectDiscussionSummary`'s side effects (thread status, `stopAgent`, `persistThreads`) — **or**, more simply and with less duplicated bookkeeping risk, keep the message-building at the top of the function unchanged and only gate the final `pi.sendMessage(...)` call itself, deferring the *entire rest of the function* (thread-dormant transition, `stopAgent`, persistence) to run only once the message is actually sent — check whether the ticket's "released like any other push" implies the thread-state side effects should also wait for release, or whether they may proceed immediately regardless of hold. **This ordering question (does closing the thread wait for the hold, or run immediately) is not settled by the ticket text and needs an explicit implementation decision — recommend: run the thread-close/stop/persist side effects immediately (unchanged), and hold only the outbound message itself, since those side effects are not part of the race being fixed and delaying them adds no safety.**
10. Spec amendments:
    - `ai-docs/spec/pi-adapter-runtime.md#L626-636`: change "A `followUp` push is therefore sent at once only when the owning process's agent is idle" to "...idle and no compaction is in flight", and add a sentence to the `steer families... never held` line noting the compacting exception.
    - `ai-docs/spec/pi-adapter-runtime.md#L1108-1157` (goal-loop arming anchor): add the "waiting for compaction" settle outcome (mirroring the existing yield-to-live-children bullet) and the below-advisory wording change.
    - `ai-docs/spec/pi-adapter-runtime.md#L1158-1195` (model-driven-compaction anchor): replace the L1167-1169 "the goal then reaches a fresh settle and the existing armed `agent_settled` reminder re-enters" sentence with the release-routine re-arm description, and add the accepted owner-typed `/compact` window as a stated exception.
    - `agents-plugin-pi/pi-lead-guide.md#L41-44`: add one line naming `goal-compact-and-continue` as the documented way to compact under an armed goal (and that owner `/compact` still races the reminder).

## Verification Plan

- `cd agents-plugin-pi && npm test` — must cover, per the ticket's Phase 1 test list, at minimum:
  - `decideOnSettle`: settle-while-compacting → `{action:"waiting"}`, streak unchanged, `sawToolCallThisCycle` unchanged (extend `test/goal-loop.test.ts`'s existing `describe("decideOnSettle")` block, ~L331-421, mirroring the existing yield tests at L384-418).
  - `buildGoalReminder`: below-advisory wording assertion updated (`test/goal-loop.test.ts#L257-260`) to match the new "do not call goal-compact-and-continue" text.
  - `spawner.ts`: `pushToLead`/`flushHeldPushes` — `followUp` and `steer` held while `leadCompactingRef.current` is true and re-sent with their recorded `deliverAs` on flush (extend `test/spawner.test.ts`'s `describe("pushToLead: holding a mid-turn push...")`, ~L1242-1382, with a `leadCompactingRef` reset in `beforeEach`/`afterEach` alongside the existing `heldPushQueue`/`leadIdleRef` resets); `registerPushFlush`'s `agent_settled` handler does not flush while compacting.
  - `goal-loop.ts` IO glue (new fake-`pi` harness, mirroring `test/ask.test.ts#L870-922`): release runs once when both `session_compact` and the lever's `onComplete` arrive; release triggered by `session_compact` is deferred (nothing sent synchronously inside that handler — assert via a synchronous check right after emitting the event, before draining the macrotask queue); `onError` alone releases with the failure reason folded into the reminder; `agent_start` clears the flag without sending a reminder; a non-lever `session_compact` (no `pendingRearm` set) releases held pushes but sends no reminder; release while the agent is not idle sends nothing, and a subsequent settle flushes + re-arms normally.
  - `ask.ts`: `injectDiscussionSummary` held while `leadCompactingRef.current` is true, delivered once released (extend `test/ask.test.ts`'s `describe("closeThreadOnDone / injectDiscussionSummary (fake pi)")`, ~L870-922).
  - Trivial-goal case: a reminder built with `percent` below `advisoryPercent` explicitly tells the model not to compact (covered by the updated `buildGoalReminder` test above).
- Live check (owner-run, not executable by this plan): repeat both dogfood runs from the ticket's Background and confirm (a) the post-lever conversation is not replaced by a late compaction summary, and (b) a child report arriving mid-compaction is delivered afterward rather than lost or racing a fresh turn.

## Escalations

- None.
