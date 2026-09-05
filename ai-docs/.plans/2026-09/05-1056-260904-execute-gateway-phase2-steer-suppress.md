# Plan: Pi lead-execute approval gateway — Phase 2 (re-scoped 2026-09-05): suppress the redundant approval steer when a waiter was woken

## Relevant Ticket Contract

- From `ai-docs/tickets/ready/260904-feat-ws-pi-execute-approval-gateway.md`, `### Phase 2` `#### Re-scope (2026-09-05)`: the headline pause/resume deliverable is already achieved (260905). The sole remaining item: `createApprovalRelay` (`execute-gateway.ts`) still injects the approval request via `pi.sendUserMessage(..., {deliverAs:"steer"})` **unconditionally**, in parallel with the waiter wake. When a waiter was woken (the lead already learned of the request through `ws-agent-wait`'s `pending_approval` return), the steer arrives at the next turn boundary as a stale duplicate ("steering: queued message"). Harmless but noisy.
- Required fix: emit the steer only when **no** waiter was woken by the request, preserving the relay as the fallback path for a non-waiting lead.
- Required test coverage: unit tests for both branches (waiter-woken → no steer; no waiter → steer).
- Re-scoped verification also includes a live-run check (two-branch), which is a manual gate — out of scope for this survey to execute, but must be named in the plan's Verification Plan.
- Non-goal (ticket-level, still applies): no changes to `agents-plugin-tool/` (ws-mcp Go) or `agents-plugin/skills/`.

## Out of Scope

- Mid-task `complex` escalation and on-demand approval-context expand — explicitly dropped from Phase 2 by the re-scope; do not implement.
- The Pi in-flight pause/resume capability probe — dropped; the headline is already satisfied by 260905's wait-return path.
- Any change to `ws-agent-wait`'s `pending_approval` reason/payload shape (`firstPendingApprovalAgentId`, `harvestWinner` at `src/spawner.ts#L1181-1330` area) — already landed under 260905, not touched here.
- Any change to `waitForDecisionFile`, the decision-file protocol, or the gated-exec tool's `execute()` body in `execute-gateway.ts` — unaffected by this refinement.

## Codebase Findings

- `agents-plugin-pi/src/spawner.ts#L428-432` — `settleWaiters<T extends {waiters: Array<()=>void>}>(record): void` drains `record.waiters` and resolves each. Currently returns nothing; the array's length *before* draining is exactly "how many waiters were woken." This is the natural signal source — no new bookkeeping needed, just capture the pre-drain length and return it.
- `agents-plugin-pi/src/spawner.ts#L918-950` — `applyRpcEvent(record, evt): void`. Four branches; only the `tool_execution_start`/`GATED_EXEC_TOOL_NAME` branch (L932-948) is relevant — it sets `record.pendingApproval` then calls `settleWaiters(record)` (L947) as the 260905 deadlock fix. This is the one call whose "did it wake anyone" result must reach `attachEventListener`. The other three branches (`agent_start`, `agent_settled`, `ws-report-to-lead`) also call/don't call `settleWaiters`, but nothing downstream currently needs their wake result — safe to give them a uniform `{waiterWoken: false}` or the same computed value; simplest is to compute uniformly from whatever `settleWaiters` call happens (if any) in that invocation.
- `agents-plugin-pi/src/spawner.ts#L963-971` — `attachEventListener(record, client, onApprovalPending?)`: calls `applyRpcEvent(record, e)` (return currently discarded) then, only for the gated-exec `tool_execution_start`, calls `onApprovalPending(record)`. This is the exact seam to thread the new signal through: capture `applyRpcEvent`'s return value and pass it as a second arg to `onApprovalPending`.
- `agents-plugin-pi/src/spawner.ts#L773`, `#L779`, `#L1482` — three type declarations of `onApprovalPending?: (record: RpcAgentRecord) => void` (`RpcSpawnCtx.onApprovalPending`, `RpcResumeCtx.onApprovalPending`, `registerAgentTools`'s parameter). All three must widen to `(record: RpcAgentRecord, info: { waiterWoken: boolean }) => void` for TS consistency — they're all the same callback threaded through `spawnAgent`/`sendToAgent`/`registerAgentTools` to the one real implementation.
- `agents-plugin-pi/src/spawner.ts#L1068`, `#L1140` — the two `attachEventListener(record, client, ctx.onApprovalPending)` call sites (`spawnAgent`'s initial spawn, `sendToAgent`'s dormant-resume branch). No change needed here — they just pass the callback through unchanged; the signature widening is transparent to these call sites.
- `agents-plugin-pi/src/execute-gateway.ts#L422` — `ExecuteGatewaySessionCtx.onApprovalPending?: (record: RpcAgentRecord) => void` — same widening needed.
- `agents-plugin-pi/src/execute-gateway.ts#L436-450` — `createApprovalRelay(pi, sessionCtx): (record) => void`. Currently: reads `record.pendingApproval`, scrapes context, builds the prompt text, then **unconditionally** `pi.sendUserMessage(text, {deliverAs:"steer"})`. This is the sole call site that must gain the `info.waiterWoken` early-return.
- `agents-plugin-pi/src/execute-gateway.ts#L1-87` (header doc comment) — the "approval-request relay (child -> lead)" bullet (L37-47) states `deliverAs: "steer"` is "the safe unconditional choice here" — this specific claim is now false once the skip is added and must be corrected to describe the new conditional behavior.
- `agents-plugin-pi/src/index.ts#L188-195` — sole call site of `createApprovalRelay` (`onApprovalPending = createApprovalRelay(pi, {cwd: ctx.cwd})`), then threaded into `registerAgentTools` and the execute-gateway session ctx. No signature mismatch after the widening (TS infers the wider callback type automatically); no edit needed here.
- `agents-plugin-pi/test/spawner.test.ts#L440-468` (`freshRpcRecord` helper) and `#L581-670` (`describe("applyRpcEvent: ws-worker-exec …")`) — existing tests call `applyRpcEvent(record, evt)` without inspecting its return value (e.g. L647, L658). Widening the return type from `void` to an object is backward-compatible with all of these; no existing test needs edits, only additions.
- `agents-plugin-pi/test/execute-gateway.test.ts#L1-51` — header comment explicitly places `createApprovalRelay`'s `pi.sendUserMessage` call in the "NOT covered here — genuinely live-gate only" bucket (needs a live `pi --mode rpc` session or real `RpcClient`, per the comment). That reasoning was really about `scrapeWorkingContext`'s live `git` subprocess call, not about `pi.sendUserMessage` itself — `createApprovalRelay` takes `pi: ExtensionAPI` as a plain parameter, so a minimal fake `{ sendUserMessage: (text, opts) => {...} }` object (cast as needed) is sufficient to unit-test the skip/no-skip branching without a live session; `scrapeWorkingContext` against a non-git tmpdir degrades gracefully (returns `undefined` fields, never throws) as already documented at `execute-gateway.ts#L336-353`.

## Implementation Plan

1. `agents-plugin-pi/src/spawner.ts#L428-432`: change `settleWaiters` to return the number of waiters it resolved (capture `waiters.length` before clearing/draining), e.g. `function settleWaiters<T extends {waiters: Array<()=>void>}>(record: T): number { const waiters = record.waiters; record.waiters = []; for (const resolve of waiters) resolve(); return waiters.length; }`. Existing call sites (L516, L524, L867) keep working unchanged (return value simply unused there).
2. `agents-plugin-pi/src/spawner.ts#L918-950`: change `applyRpcEvent`'s return type from `void` to `{ waiterWoken: boolean }`. Capture the `settleWaiters(record)` result at each call site (`agent_settled` branch L924, gated-exec branch L947) and return `{ waiterWoken: count > 0 }`; the `agent_start` and `ws-report-to-lead`-args-invalid / no-op paths fall through to a default `return { waiterWoken: false };` at the end (since they never call `settleWaiters` on the "did nothing" sub-paths — the `ws-report-to-lead` branch itself also calls `settleWaiters` via `enqueueReport`, so mirror the same capture-and-return there for consistency, even though nothing downstream currently reads it for that branch).
3. `agents-plugin-pi/src/spawner.ts#L963-971`: change `attachEventListener` to capture `applyRpcEvent`'s return value and pass it through: `const { waiterWoken } = applyRpcEvent(record, e); if (onApprovalPending && e.type === "tool_execution_start" && e.toolName === GATED_EXEC_TOOL_NAME) { onApprovalPending(record, { waiterWoken }); }`.
4. `agents-plugin-pi/src/spawner.ts#L773`, `#L779`, `#L1482`: widen all three `onApprovalPending?: (record: RpcAgentRecord) => void` declarations to `onApprovalPending?: (record: RpcAgentRecord, info: { waiterWoken: boolean }) => void`. Update each declaration's doc comment one line to mention the new `waiterWoken` param (brief, e.g. "the approval-request-relay injection hook, now told whether a lead-side waiter was already woken by this event").
5. `agents-plugin-pi/src/execute-gateway.ts#L422`: widen `ExecuteGatewaySessionCtx.onApprovalPending`'s type to match (same signature as step 4).
6. `agents-plugin-pi/src/execute-gateway.ts#L436-450`: change `createApprovalRelay`'s returned callback to accept the second param and skip the steer when a waiter was woken:
   ```ts
   export function createApprovalRelay(pi: ExtensionAPI, sessionCtx: { cwd: string }): (record: RpcAgentRecord, info: { waiterWoken: boolean }) => void {
     return (record, info) => {
       const pending = record.pendingApproval;
       if (!pending) return;
       if (info.waiterWoken) return; // lead already learned via a woken ws-agent-wait; steering here would be a stale duplicate notice
       const context = scrapeWorkingContext(resolveApprovalContextCwd(pending, sessionCtx.cwd));
       const text = buildApprovalPromptText({ agent_id: record.agentId, cmd_id: pending.cmdId, command: pending.command, rationale: pending.rationale, context });
       pi.sendUserMessage(text, { deliverAs: "steer" });
     };
   }
   ```
7. `agents-plugin-pi/src/execute-gateway.ts#L34-47` (header doc comment, "The approval-request relay (child -> lead)" bullet): update the prose that currently says `deliverAs: "steer"` is "the safe unconditional choice here" — replace with a short note that the steer is now skipped when `attachEventListener`/`applyRpcEvent` report a waiter was already woken (the lead learns via `ws-agent-wait`'s `pending_approval` return instead), citing the 260905 wait-return path and this re-scoped Phase 2 item; keep the existing rationale for *why* `steer` (not `followUp`) is used in the fallback case.
8. `agents-plugin-pi/src/index.ts#L188-195`: no code change expected (TS should accept the widened callback type transparently); read after step 6 to confirm no cast/annotation there needs updating.

## Verification Plan

- `cd agents-plugin-pi && npm test` (node --test, no tsc) — must stay green, extended with:
  - `test/spawner.test.ts`, in/near `describe("applyRpcEvent: ws-worker-exec …")` (`#L581-670`): assert `applyRpcEvent(record, {type:"tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId, args:{command}})` returns `{waiterWoken: true}` when `record.waiters` was non-empty beforehand, and `{waiterWoken: false}` when it was empty (mirrors the existing "260905 (deadlock fix)" tests at L641-661, which already set up both waiter/no-waiter states — extend rather than duplicate).
  - `test/execute-gateway.test.ts`: new `describe("createApprovalRelay")` block with a minimal fake `pi` (`{ sendUserMessage: (text, opts) => { calls.push({text, opts}); } } as unknown as ExtensionAPI`, or the narrowest type that satisfies the parameter) and a `freshRpcRecord`-equivalent fake `RpcAgentRecord` with `pendingApproval` set (reuse the shape from `execute-gateway.test.ts`'s existing `PendingApproval`/`WorkingContext` imports, or a local minimal record literal since this file doesn't currently import `RpcAgentRecord` — add that type import from `../src/spawner.ts`):
    - waiter-woken branch: call the relay with `info: {waiterWoken: true}` → assert `sendUserMessage` was NOT called.
    - no-waiter branch: call the relay with `info: {waiterWoken: false}` → assert `sendUserMessage` WAS called exactly once (and, optionally, that the text contains the `cmd_id`/`command`, reusing `buildApprovalPromptText`'s already-tested format).
    - Use a non-git tmpdir (e.g. `mkdtempSync`, matching the existing `waitForDecisionFile` tests' tmpdir pattern at the top of the file) as `sessionCtx.cwd` so `scrapeWorkingContext`'s real `git` calls degrade to `undefined` fields rather than needing a live session — no live `pi` process required.
- Manual/live gate (out of survey scope, name only): a live run where a lead parked in `ws-agent-wait` handles an approval via the wait return and receives no trailing stale steer, and a second run where a non-waiting lead still receives the steer relay — per the ticket's re-scoped Verification note. Report this as still-outstanding after implementation, consistent with Phase 1's precedent of deferring live `pi --mode rpc` verification.

## Escalations

- None.
