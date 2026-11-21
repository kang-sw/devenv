---
title: "agy 재연결 시 감시 복원과 실패한 턴 입력 보존"
related:
  261120-refactor-ws-agent-workers-under-holders: "held workers; watchdog rearming across replacement was excluded"
  261119-bug-ws-agent-v2-dogfood-surprises: "daemon replacement and agy lifecycle observations"
  261120-bug-ws-sweep-1120-agy-fixes: "earlier watchdog corrections"
  261120-feat-ws-agy-timeout-process-respawn: "separate timeout exhaustion process policy"
---

# Restore agy watchdog on reattach and account for sent input

## Background

Owner decision (2071)=b directs the two remaining gaps here. Context supplied by the owner: agy 1.5.13 Cloud Code Assist streams have a 600s client idle timeout; native retry recovers most hits, but some stop turns. Gaps 3 and 4 (terminal failure settles as `TurnFailed` with class, and 15s drain before cancel to allow native recovery) landed in `3dd5f02fb`, merged at `c84c94be7`; retain those behaviors.

- **[VERIFIED] Reattach:** `agents-plugin-tool/internal/wsagent/runner_conv.go:96-117` makes watchdog state loop-owned and in-memory. `runner_process.go:532-549` rejoins the holder and `:647-667` / `:678-698` finishes replay without arming it. Replayed turn starts call `beginTurn` (`runner_events.go:108-115,288-317`), which does not arm it. `startModelWait` runs after an ordinary send (`runner_conv.go:1250-1257,1286-1290`) and explicitly excludes replay (`agy_watchdog.go:137-145`); retry interception requires the armed flag (`agy_watchdog.go:342-347`). **[INFERRED]** A daemon replacement mid-turn can therefore leave the rejoined turn without watchdog failure interception/retry. Silence timing has a separate current limitation: `agySilenceWatchdogEnabled` is false (`agy_watchdog.go:48,148-151`), so do not claim silence detection is enabled in production merely by rearming.
- **[VERIFIED] Sent input:** after successful send, `runner_conv.go:1250-1259,1305-1318` removes the sent queue prefix and clears delivered mail/reports; a send error instead retains the queued input (`:1269-1283`). `record.go:98-105,130-138` describes these as undelivered inputs. On a later error `runner_events.go:363-368` settles failure; `spawn.go:274-286` publishes the result, with no sent input in that result. **[INFERRED]** If the turn fails or aborts after accepting input, the parent's message may be unavailable for retry and its loss may be silent; validate the exact abort, report, and receipt paths, including partial processing, before choosing behavior. A result reporting failure alone does not establish which input was handled.

Reproduction shape: deliver a parent message to a held agy worker, replace the daemon while the turn remains open, finish holder replay, then inject a retryable terminal model failure and observe watchdog recovery. Separately, deliver identifiable input successfully, then fail or abort the turn before useful completion; inspect the next turn and parent-visible result/receipt. Include replay and process-exit variants rather than depending on a real 600s wait.

## Compatibility and constraints

**Expected T0, conditional on design:** an in-memory-only rearm with unchanged wire/store surfaces lets old binaries continue their existing behavior; existing state and normal writes are `Record` fields (`record.go:98-116,130-144`), and the watchdog fields are process-local (`runner_conv.go:96-117`). The input solution is undecided: if it adds persisted state or a receipt/protocol surface, this T0 expectation is not established. `readRecord` decodes JSON (`record.go:309-316`) and `writeRecord` marshals the record (`record.go:328`); inspect unknown-field preservation and concrete old/new read/write and recovery paths before claiming a tier. Whole-file overwrite without recovery is not T2. Settle the tier before implementation/release; T3 requires explicit owner approval. Follow the matching implementation manuals, especially performance/storage and release compatibility, when promoting and implementing.

## Open Decision Queue
Last ID: none

- [open] [technical] How should a reattached active ordinary agy turn reconstruct its watchdog and related pending tool/approval, failure, retry-budget, drain, cancel and backoff state, including replay gaps and boundaries, without a duplicate retry or disrupting an in-flight tool? Decide what is recoverable and the safe behavior when history is insufficient; respect the separate disabled silence watchdog and existing 15s drain.
- [open] [policy] On failed or aborted delivered input, what parent-visible guarantee is required: requeue, an actionable receipt identifying the unconsumed/uncertain input, or another explicitly settled policy? Decide treatment of partial processing, intentional interruption, duplicate side effects, ordering, and retries.
- [open] [technical] How should the chosen input guarantee track ownership and acknowledgement of queued text, mail and reports through successful send, settle, process exit and daemon replacement, including atomicity, deduplication, storage cost, and mixed-version recovery? Establish the actual compatibility tier from the chosen design.

## Phases

### Phase 1: Restore recovery and account for delivered input

After the queue is resolved, implement the chosen recovery and delivery guarantees for held agy turns, maintaining existing terminal classification and native-retry drain. Coordinate shared input handling with other backends if the selected mechanism touches them. Keep unrelated process-respawn policy in its related ticket.

Verify with regression tests named by behavior: reattached active agy turn retries a retryable failure; reattached successful/completed or idle turn does not retry; replay gaps and pending tool/approval activity do not cause unsafe cancellation or duplicate work; drain allows native recovery before cancellation; delivered parent input survives failure or gets the settled actionable receipt; intentional abort and process exit follow the chosen policy; multiple queued inputs retain order without duplication across daemon replacement; normal success acknowledges exactly once. Exercise compatibility of any changed persisted or protocol surface against old and new readers/writers, and verify any configured silence timer separately from the currently disabled production switch.
