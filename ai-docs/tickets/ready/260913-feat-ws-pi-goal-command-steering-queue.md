---
title: "Pi goal command: queue updates during active turns"
related:
  260909-feat-ws-pi-goal-stop-controls: owns race-safe goal disarming and terminal goal state
  260911-feat-ws-pi-held-push-batch-delivery: owns FIFO delivery of accumulated turn-boundary input
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d5a72d8e5e3a919f
sage-review-completeness-reviewed: d5a72d8e5e3a919f
---

# Pi goal command: queue updates during active turns

## Background

Pi currently cannot apply `/goal <new goal>` while the model is actively working. The owner must wait for the turn to settle and repeat the command, although the adapter already holds eligible custom-message pushes and raw summaries in a FIFO for safe-boundary delivery (`agents-plugin-pi/src/spawner.ts#L1048-L1071`, `agents-plugin-pi/src/spawner.ts#L1209-L1210`, `agents-plugin-pi/src/spawner.ts#L1387-L1397`). This makes goal correction needlessly timing-sensitive during long-running or delegated work.

Accept a valid goal update while a turn is active and place it into a typed control entry in the existing held-push FIFO. The command remains control state rather than an immediate model utterance: it is applied at the next safe delivery boundary and reflected in the next goal reminder.

## Decisions

- **Accept during active work.** A syntactically valid `/goal <new goal>` entered while the model turn is running receives immediate owner-visible acknowledgement that the update was queued rather than a busy/retry rejection.
- **Use one ordered queue and one delivery batch.** Represent the update as a typed goal-control entry in the existing held-push queue (`agents-plugin-pi/src/spawner.ts#L1209-L1210`, `agents-plugin-pi/src/spawner.ts#L1387-L1397`). Preserve FIFO order for owner-visible rendering and control application; do not create a second goal queue, segment one safe-boundary delivery into multiple model turns, or silently coalesce multiple updates. Applying several valid queued replacements in order naturally leaves the last one active.
- **Do not inject command prose.** At delivery, apply goal-control entries in FIFO order but omit them from model input. Held prose remains one model delivery and does not acquire different goal-state semantics based on positions around a control entry. Goal replacement affects the resulting adapter state and the next goal reminder, not the interpretation context of prose segments inside the current batch.
- **Apply before reinjection.** Drain applicable queued control entries at the safe turn boundary before deciding whether to enqueue the next goal reminder, so the reminder names the resulting active goal and the old goal cannot re-arm after replacement.
- **Validate before enqueue.** Reject an empty or otherwise invalid goal command immediately using the existing command validation. A delivery-time state or generation error leaves the then-current goal unchanged, reports a precise owner-visible failure, and does not reinterpret the entry as prose.
- **Keep terminal controls immediate and authoritative.** `/goal stop`, `/goal clear`, and `/goal reset` retain their existing immediate state transition and are not represented as FIFO entries. Each terminal transition advances the goal generation and invalidates queued replacements from an older generation. A new `/goal <goal>` entered after that terminal transition may queue under the new generation and arm at the safe boundary. Stale replacements report failed application rather than re-arming the terminated goal.

## Constraints

- Pi-local only; do not change shared ws-mcp or shared playbooks.
- Reuse the existing held-push queue and its FIFO wake batching (`agents-plugin-pi/src/spawner.ts#L1209-L1210`, `agents-plugin-pi/src/spawner.ts#L1387-L1397`).
- Do not interrupt the current model turn solely to apply the goal change.
- Preserve existing immediate `/goal` behavior while the host is idle.
- Keep queue acknowledgement and final applied/failed feedback distinct so the owner can tell acceptance from execution.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/test/goal-loop.test.ts, agents-plugin-pi/test/push-wake.test.ts |
| scope.surface | public-interface | /goal accepts a queued replacement during an active lead turn |
| scope.new_public_symbol | no | no exported symbol is named; /goal behavior changes |
| scope.new_type_contract | yes | typed held goal-control entry in the held-push FIFO |
| scope.test_surface | existing | agents-plugin-pi/test/goal-loop.test.ts and agents-plugin-pi/test/push-wake.test.ts |
| complexity.reuse_points | confirmed | goal-loop state transitions and heldPushQueue/flushHeldPushes |
| complexity.side_effect_risk | high | shared FIFO delivery, wake, compaction, and reminder scheduling interact |
| risk.correctness | high | FIFO control ordering and terminal-state precedence must preserve the active goal |
| risk.fit | high | the new control must compose with the settled held-push batch contract |
| risk.test | high | active/idle paths, ordering, failures, compaction, and restart boundaries need coverage |
| risk.security_or_contract | high | /goal's owner-visible asynchronous control contract changes |

## Phases

### Phase 1: Queue and apply goal replacement at a safe boundary

Add a typed held-input entry for validated goal replacement, integrate it with the existing FIFO held-push drain (`agents-plugin-pi/src/spawner.ts#L1387-L1397`), and apply it before goal reminder reinjection. Keep direct idle command handling unchanged and reuse existing goal state transitions.

Verify active-turn acceptance and acknowledgement, idle behavior, one-batch FIFO owner rendering with prose on both sides, multiple queued replacements, no per-prose-segment goal-state semantics, no synthetic model message for the command, reminder text using only the resulting goal, immediate terminal generation invalidating older queued replacements, a post-terminal replacement using the new generation, delivery-time failure preserving the then-current goal, wake batching, queue cleanup, and restart/compaction behavior with the held-push queue explicitly non-persistent today (`agents-plugin-pi/src/spawner.ts#L1200-L1201`, `agents-plugin-pi/src/spawner.ts#L1411-L1414`).

## Sage Review Round 1 (2026-09-13)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | State-sensitive prose cannot be represented by one held-push batch | critical | missing |
| 2 | Queued terminal entries do not exist in the landed control contract | critical | missing |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
