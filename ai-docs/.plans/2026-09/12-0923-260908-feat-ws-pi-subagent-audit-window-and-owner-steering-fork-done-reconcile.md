# Plan: 260908-feat-ws-pi-subagent-audit-window-and-owner-steering — Phase 2 same-process fork-raised `/done` reconciliation sub-slice

## Relevant Ticket Contract

**Active authority:** the owner's final direction in this research session supersedes the ticket's crash-safe clauses for this selected sub-slice: require **same-process lifecycle correctness only**. Abrupt death and plugin `/reload` exact reconciliation are best-effort non-goals. Preserve recovery supplied naturally by existing thread persistence; add no protocol or checkpoint machinery solely for either case. This researcher does not edit the ticket; broader Phase 2 remains excluded.

Implement this smallest complete behavior:

- `/done` on an open/bound fork-raised thread starts one in-memory finish operation and closes the owner view. Repeated `/done`, report/settle races, and duplicate callbacks join that operation or do nothing; they do not dispatch another closeout or terminal message.
- If the fork is running, wait for its actual next `agent_settled`; do not interrupt it or immediately append a closeout. If it has already settled, evaluate immediately instead of waiting for an event that will never arrive—the stranded-idle bug.
- If a valid final is already accepted for this work, deliver it through the existing push path if needed, then park silently. If its terminal final or settlement is already held/enqueued for the lead, do not duplicate it or request another report; park silently. “Held” is an in-process delivery obligation, not proof of model consumption.
- Otherwise send exactly one lead-attributed closeout through the existing `sendToAgent`/`promptAgent` branches. The closeout asks for the normal final report. On its settle, route a valid final once; no valid final or another question yields the existing missing-final advisory and silent park, never synthetic success or another closeout/nudge.
- Use positive same-process observations of report, settle, and push admission/enqueue. Do not infer delivered final from timestamps or absence of `pendingFinal`. `running`/`streaming` may select wait versus immediate evaluation, but are not delivery evidence.
- Continue ordinary `.ws-threads.json` persistence of thread status/resume data and existing shutdown snapshots. Persistence is not a transaction or an exactly-once promise.
- No changes to lead-raised questions, ws-mcp, or `session_children`. No Phase 2 completion claim. The lead owns ticket/spec reconciliation with this owner decision; this research changes only the plan.

### Explicitly removed requirements

The following are **retired from the active implementation plan**, not deferred acceptance gates:

- Persisted `reconciliationId`, durable multi-step reconciliation state machine, persist-before-every-side-effect ordering, and special canonical-store ownership through a crash/reload reconciliation.
- Child receipt command/control envelope, child dedupe journal, per-ID acknowledgements, durable admission, and automatic dispatch retry.
- Durable terminal outbox, at-least-once crash replay, lead ingress dedupe ledger/tombstones, durable delivery acknowledgement, and transactional clearing after acknowledgement plus park.
- Strict/fsync-backed thread writes, write-ahead journal, restart fencing, or new storage schema solely for finish recovery.
- Graceful-reload drain/checkpoint/barrier, preservation of in-flight closure state across extension replacement, and exact reload reconciliation.
- Crash-at-every-boundary tests and proof of exactly-once provider invocation. Ordinary same-process error/race tests remain required.

## Out of Scope

- All other Phase 2 owner steering: `lastWriter`/`ownerSends`, audit interaction, modal/interrupt controls, owner-held exemptions/flags, widget changes, and transcript retirement.
- Phase 1 owner-run live acceptance, ticket/spec/source edits during research, and upstream Pi or custom SDK-host changes.
- Broad reliability redesign of `heldPushQueue`, generic child report validation, or all child stop/resume behavior. Extend shared mechanisms only enough to make this finish operation correct.
- Automatic recovery/retry of ambiguous sends after RPC timeout, abrupt process death, or `/reload`. Do not add infrastructure to hide these expressly accepted windows.

## Codebase Findings

### Existing mechanisms to reuse

- `agents-plugin-pi/src/ask.ts`: `closeThreadOnDone` routes fork-raised close to `detachForkRaisedThread`, which currently clears `threadBound` and marks dormant immediately. Nothing evaluates an already-idle fork afterward. This is the primary entry point to fix. `handleRespondentFinalReport`/`armFinalReportHook` also close fork-raised threads; they must cooperate with an active finish, not independently unbind it.
- `ask.ts`: `ThreadRecord`, `persistThreads`, `captureForkResume`, `hydrateThreadRegistry` already retain ordinary thread/resume state. `saveThreadRegistryFile` is best-effort and `loadThreadRegistryFile` tolerates missing/corrupt files. Keep those contracts. No durable finish fields are necessary under the owner decision.
- `agents-plugin-pi/src/spawner.ts`: `RpcAgentRecord`, `applyRpcEvent`, `flushPendingFinal`, `attachEventListener`, `promptAgent`, `sendToAgent`, `stopAgent` own child lifecycle. Reports are observed at `tool_execution_start`; finals are held in `pendingFinal` until settle. `flushPendingFinal` clears that slot before a void push, so its absence is not positive delivery evidence.
- `attachEventListener` normally flushes final, optionally pushes settlement, probes liveness, then parks if not thread-bound/running. `agents-plugin-pi/src/fork.ts#wireAntiBleedLoop` is a separate event listener that may independently nudge or advise. Active finish must be handled by one coordinator before both ordinary settle/park and anti-bleed behavior, not by a second competing settle listener.
- `fork.ts`: `validateFinalReportShape`, `checkExpectsCommitCompletion`, and existing advisory construction supply final validation and missing-final language. `wireAntiBleedLoop` knows `expectsCommit`; pass its validation policy to the lower-level lifecycle coordinator through a record callback rather than introducing a `spawner.ts -> fork.ts` import cycle.
- `spawner.ts#sendToAgent`: idle/dormant sends go through `promptAgent`; busy sends use followUp/steer. It clears pending final when new work is sent. Reuse it for the one closeout and retain normal attribution, model, tool, and resume behavior; do not call a second raw `RpcClient` transport. Finish should normally send only at settle/idle, not queue another turn into a known running fork.
- `spawner.ts`: `admitPush`, `heldPushQueue`, `sendPush`, `flushHeldPushes`, `registerPushFlush`, `buildPushContent` implement the shared FIFO, compaction holds, counted user wake, and fresh fan-in formatting. A push held there is not yet a Pi enqueue. Add only optional in-memory admission/enqueue notification for finish-related terminal events; no second queue and no durable delivery layer.
- `spawner.ts#stopAgent` clears live state synchronously before abort/stop and returns even when these fail. It also clears `threadBound` and, for silent stop, pending final. Decide/capture the terminal outcome before calling it. Do not use it to prematurely discard an unprocessed final. Same-process finish calls silent stop once and reports a detectable stop failure honestly; do not retrofit durable park certification.
- `agents-plugin-pi/src/agent-sidecar.ts#captureOrphans` skips thread-bound records. `agents-plugin-pi/src/index.ts#persistShutdownAgentSnapshots` snapshots sidecar candidates, stops children, then refreshes thread resume data. Keep this existing recovery behavior; no special reconciliation ownership transfer rule is required now.
- `agents-plugin-pi/test/fork-lifecycle.integration.test.ts` uses real SDK/resource/session components but substitutes RPC methods and manually emits some events. It is useful regression coverage, not a real-crash proof; real-crash proof is no longer required.

### API research retained as guardrails, not blockers

Inspected local Pi **0.84.4** and host **0.85.1**, separate installations at `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent` and `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent`. `spawner.ts#RPC_CLI_PATH`/`buildRpcClientOptions` use the invoking host CLI and exact adapter entry.

Read the complete host README, `docs/rpc.md`, `sdk.md`, `extensions.md`, `session-format.md`, `sessions.md`, `compaction.md`, `tui.md`; compared both installations' six non-TUI documents and relevant runtime source diffs. Followed the applicable input-transform, streaming-input-transform, send-user-message, file-trigger, event-bus, RPC UI/demo, and SDK extensions/sessions examples completely. Consulted `ai-docs/mental-model/plugin-runtime.md`; it supplies no Pi durable delivery mechanism.

- Installed `dist/core/agent-session.js#AgentSession.prompt` dispatches registered extension commands before queue admission. The prior survey's claim that raw RPC offered no pre-queue control path was incomplete. However `_tryExecuteExtensionCommand` catches command errors and still leads to successful preflight acknowledgement.
- `ExtensionAPI.sendMessage`/`sendUserMessage` return void. `sendCustomMessage` uses volatile busy/next-turn queues; ordinary `message_end` hooks precede session append. `SessionManager._persist` can defer a fresh session's file until its first assistant message. These are why no crash guarantee should be claimed—not reasons to add receipt machinery now.
- `AgentSession.reload` awaits old extension shutdown, invalidates/rebuilds the extension runner, and starts the replacement extension on the same AgentSession. Pi's own queue can survive while adapter `registerPushFlush` clears its held queue and `index.ts` stops children. Exact reload continuity therefore does not follow from “same OS process”. The owner explicitly excludes it.
- Isolated real-SDK probes against both versions confirmed: duplicate handled receipt commands can run without a model; command throws can still acknowledge success; same-ID custom sends are not natively deduped; fresh session entries can exist only in memory. Probe temporary directories were removed. No provider calls were made.

### Minimal runtime contract (new internal fields, not persistence or public API)

Use one small in-memory coordinator per fork finish. Its proposed state is explicit here so a new executor need not invent a protocol:

- Identity: thread ID, current record/client launch generation, and a unique **in-memory** operation token. A completed operation is a no-op for repeated `/done`; reopening a thread is a new lifecycle, not reuse of the old token.
- Phase: waiting for current settle; closeout issued/waiting; finishing; complete. One shared promise/guard serializes advancement; set phase before calling any async send or park.
- Inputs: a valid final candidate for the current work, a positive settled observation, and a terminal delivery reference with `held`/`enqueued` state. Retain report payload before a helper clears `pendingFinal`. Invalidate prior-work facts whenever a real new instruction is accepted, following the existing `promptAgent` and busy `sendToAgent` reset sites. An anti-bleed internal nudge must not manufacture a new task boundary.
- A terminal delivery reference identifies one in-process event object, not a durable event ID. The reference can stay with the shared FIFO until it enqueues. Once admitted there, the coordinator must not admit the same event again; after enqueue it must not recreate it. No model-consumption acknowledgement is required.
- Callbacks supplied by the owning layer: validate final using existing fork policy; complete the ordinary thread status/resume update; optionally notify owner of operational failure. Do not import `ask.ts` into `spawner.ts`.
- Internal send ownership: carry the operation token locally through the closeout send so its own `promptAgent` reset does not cancel its coordinator. A different real instruction supersedes it. Do not use `RpcResumeCtx.leadSend`'s unconditional thread-unbind behavior for the coordinator-owned closeout; that flag controls external takeover, not human-versus-lead transcript attribution. Keep protection until finish-owned park.

Record callbacks/state are internal additions. There is **no new wire format, config/env surface, persistence schema, or public tool field**. Runtime facts should be captured while the fork/thread is live, not first reconstructed from flags when `/done` arrives.

## Implementation Plan

1. **Add the small same-process lifecycle seam in `agents-plugin-pi/src/spawner.ts`.** Extend `RpcAgentRecord` with the coordinator/positive completion facts above and a fork-validation callback (new internal members; names left to normal local style). Centralize finish advancement in a helper alongside `attachEventListener`, not a durable subsystem. Integrate `applyRpcEvent` report/settle observations and the existing prompt/send reset sites. For a finish-owned final candidate, retain its `toolCallId`/payload at `tool_execution_start` and accept or reject it on the matching `tool_execution_end` result; this prevents a failed tool invocation from becoming successful completion. Intercept finish-owned question reports before ordinary question registration/push side effects. Scope collected facts to current work and launch generation so stale final or late events cannot complete a replacement task. Preserve existing behavior for records without an active finish.

2. **Observe actual shared push admission in `spawner.ts#admitPush`, `sendPush`, `flushHeldPushes`, and `flushPendingFinal`.** Add an optional terminal-event reference/callback and carry it through `HeldPush` so the coordinator distinguishes held from enqueued. Publish `held` only when the event is actually accepted into the FIFO, and `enqueued` only when the real `pi.sendMessage` invocation occurs without synchronous rejection. A skipped push (missing Pi/context/gate) is not successful admission. Keep the current queue, compaction hold, wake path, formatting, and delivery mode behavior. This is same-process adapter bookkeeping, not a claim that void `sendMessage` durably committed or that the model read the message. Avoid emitting both final and settled for one outcome. Final payload must be captured before clearing `pendingFinal`.

3. **Wire owner finish in `agents-plugin-pi/src/ask.ts#closeThreadOnDone`/`detachForkRaisedThread`.** Keep lead-ask routing unchanged. For an open/bound fork-raised thread, create/join the coordinator synchronously before any asynchronous work; close the view and persist its ordinary dormant status/resume snapshot. Keep `threadBound` as the existing temporary park/nudge protection until coordinator-owned park; do not release it and hope for a future settle. Repeated `/done` first checks the active coordinator, then dormant/unbound no-op. `handleRespondentFinalReport` must route into an already-active finish instead of independently detaching it; normal automatic final close outside finish keeps its existing behavior. On completion release the bind, refresh `forkResume`, persist ordinary thread status, and refresh the widget.

4. **Implement the finish decision in `spawner.ts#attachEventListener` and the new local coordinator.** Before ordinary settle delivery/park logic, route an active finish exclusively through this decision:
   - Running: wait for observed settle. Idle: evaluate now; do not await another event.
   - Current-work valid final with no terminal event admitted: admit that final once through the shared push path, then silently park.
   - Current-work terminal final/settlement already held or enqueued: leave that one event alone and silently park, with no extra prompt/delivery.
   - No terminal outcome and no closeout issued: mark closeout issued **before** awaiting `sendToAgent`, then send one short lead-attributed request for the normal final report. Keep protection until its settle.
   - Closeout settles with a valid final: admit final once and silently park. No valid final, invalid final, or another question: admit one existing missing-final advisory and silently park. Do not reopen/register another owner question or issue a second closeout.
   Handle settle-before-send-promise-resolution and duplicate settle/report callbacks through the same guard. A send rejection/timeout ends that closeout attempt with the existing failure/advisory semantics; do not blindly retry an ambiguously accepted prompt. A fresh independent lead send must supersede/cancel a stale finish operation rather than let its late callback park newly assigned work.

5. **Coordinate fork validation/anti-bleed in `agents-plugin-pi/src/fork.ts#wireAntiBleedLoop`.** Install the validation callback using `validateFinalReportShape` and `checkExpectsCommitCompletion`. During an active finish, let the coordinator own missing-final/question/final outcome and suppress competing automatic nudge/advisory paths. Preserve progress reporting and ordinary behavior outside finish. A raw final candidate is not successful completion merely because its kind is final; apply existing shape/commit checks before selecting final over advisory. If the report tool fails, do not treat the failed candidate as valid final. Do not broaden this step into generic report protocol redesign.

6. **Reuse silent park in `spawner.ts#stopAgent`, without a reliability rewrite.** Ensure the selected terminal event has been captured/admitted before silent stop clears pending final; mark the coordinator finishing before awaiting stop and complete it once. Respect launch/client generation so a late stop completion cannot clear a new send's state. If necessary expose the already-computed local `stopped` result to an internal finish callback, without changing the public stop tool's best-effort contract. A failed park should produce an honest operational diagnostic, not a success claim or another closeout. Do not add retries, durable park markers, or restart cleanup jobs.

7. **Leave restart/reload mechanics alone.** `ask.ts#hydrateThreadRegistry`, `agent-sidecar.ts#captureOrphans`, and `index.ts#persistShutdownAgentSnapshots` retain their existing status/resume behavior. Dispose/invalidate coordinator callbacks with their session/record lifetime so stale closures cannot act on replacement objects; do not hydrate a live finish or replay closeout/delivery from disk. No changes to these stores are required except ordinary existing snapshot calls needed by Step 3.

## Verification Plan

### Required tests

Extend `agents-plugin-pi/test/ask.test.ts`, `test/spawner.test.ts`, and `test/fork.test.ts` using existing duck-typed clients/event emitters and explicit promise barriers:

- **Regression:** fork already idle after its owner-held/thread-bound settle; `/done` dispatches exactly one closeout without another external event. Closeout final is delivered once and the child parks silently.
- Running `/done` dispatches nothing until settle; final in that run avoids closeout. Settle without terminal outcome starts exactly one closeout.
- Final already held, final already enqueued, and settlement already held/enqueued: no additional closeout, final, settlement, or advisory; silent park.
- Valid final acceptance racing `/done`; repeat `/done` before send resolution, during closeout, during stop, and after completion; duplicate settle/report callbacks. One send, one selected terminal event, one park operation.
- Closeout with plain text only, invalid final shape/commit, report tool failure, or another question: one missing-final advisory, no synthetic success, no owner-question reopening, no anti-bleed nudge, silent park.
- Enqueue callback fires on actual shared-FIFO release, not admission; skipped/rejected send is not marked enqueued. Compaction and idle wake behavior remain unchanged. The retained event is not re-admitted while held.
- Settle arriving before closeout RPC promise resolves; timeout/rejection after an ambiguous send; no automatic resend. Park errors remain honest and do not create another closeout.
- New lead work supersedes finish; old launch events/park callbacks cannot finish or stop the replacement work. Old final facts cannot suppress a required closeout for new work.
- Lead-ask behavior, ordinary fork final auto-close, unaffected child report families, normal stop/resume, and thread serialize/hydrate regression tests stay green.

No crash-boundary or exact `/reload` tests are acceptance gates. A lightweight reload cleanup regression may ensure invalidated callbacks do not affect a new instance; it must not grow into reconciliation checkpoint/replay machinery. Keep existing `test/agent-sidecar.test.ts` and `test/fork-lifecycle.integration.test.ts` as regression coverage.

Run `cd agents-plugin-pi && npm test -- test/ask.test.ts test/spawner.test.ts test/agent-sidecar.test.ts test/fork.test.ts test/fork-lifecycle.integration.test.ts`, then `npm test`. Research ran SDK capability probes only, not this not-yet-implemented behavior. A later optional live check is simply an already-idle task fork followed by `/done`, one closeout, one report/advisory, and park; it does not certify broader Phase 2.

### Explicit residual death/reload windows

These are accepted limitations, not future tasks generated by this plan:

| Window | Possible outcome after death or `/reload` |
| --- | --- |
| Thread saved dormant before finish/send completes | Thread reopens from its resume snapshot, but no finish resumes automatically; child may be idle or orphaned. |
| Closeout sent before callback/settle | Volatile issued flag is lost. No automatic retry; report may be absent. A later explicit owner/lead send may cause another turn. |
| Final observed before lead FIFO admission | Final payload can be lost with the process or silent shutdown stop. |
| Terminal held in adapter FIFO | `registerPushFlush` clears it on shutdown/reload; terminal event can be lost. |
| Terminal enqueued into Pi, before adapter bookkeeping or shutdown | Pi may retain/process it across reload while adapter forgets it; later manual finish/work can repeat a notice. Abrupt death may lose it before session persistence. |
| Park started before stop finishes | Record can look dormant while process stop is uncertain, or snapshot may lag actual exit. Existing stop/orphan/resume behavior is the recovery, not a new reconciler. |
| Reload replaces callbacks while child activity is in flight | Existing shutdown may interrupt the run; old finish state is discarded. No guarantee of one closeout/terminal event across extension instances. |

Normal same-process held-event delivery is still required. Excluding reload does not excuse duplicates or stranded idle forks while the active adapter instance remains intact.

## Escalations

**None for API feasibility under the final owner scope.** Use ordinary raw-text `sendToAgent` and the existing push FIFO. No child receipt, durable outbox, lead dedupe, custom SDK host, upstream Pi modification, or graceful-reload checkpoint is needed.

The owner explicitly retired the earlier crash/reload blockers; do not carry them back into implementation as mandatory architecture or tests. The lead is concurrently aligning the ticket with this owner decision; any remaining ticket/spec wording is the lead's documentation responsibility, not authorization to restore crash-safe scope. No existing path/symbol cited here is `[UNVERIFIED]`; proposed coordinator members/helper are explicitly new internal implementation elements, not claims about installed APIs.
