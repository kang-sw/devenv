---
title: "Pi goal command: queue updates during active turns"
related:
  260909-feat-ws-pi-goal-stop-controls: owns race-safe goal disarming and terminal goal state
  260911-feat-ws-pi-held-push-batch-delivery: owns FIFO delivery of accumulated turn-boundary input
---

# Pi goal command: queue updates during active turns

## Background

Pi currently cannot apply `/goal <new goal>` while the model is actively working. The owner must wait for the turn to settle and repeat the command, even though ordinary steering can already be held for a safe delivery boundary. This makes goal correction needlessly timing-sensitive during long-running or delegated work.

Accept a valid goal update while a turn is active and place it into the existing held steering path. The command remains control state rather than an immediate model utterance: it is applied at the next safe delivery boundary and reflected in the next goal reminder.

## Decisions

- **Accept during active work.** A syntactically valid `/goal <new goal>` entered while the model turn is running receives immediate owner-visible acknowledgement that the update was queued rather than a busy/retry rejection.
- **Use one ordered queue.** Represent the update as a typed goal-control entry in the existing held steering queue. Preserve FIFO order relative to held owner prose and other control entries; do not create a second goal queue or silently coalesce multiple updates. Applying several queued goal updates in order naturally leaves the last one active.
- **Do not inject command prose.** At delivery, mutate adapter goal state at the control entry's FIFO position. Do not turn the slash command into a synthetic user message or make the model interpret it as an instruction.
- **Apply before reinjection.** Drain applicable queued control entries at the safe turn boundary before deciding whether to enqueue the next goal reminder, so the reminder names the newly active goal and the old goal cannot re-arm after replacement.
- **Preserve surrounding steering order.** Held prose before the goal-control entry is delivered under the preceding goal state; held prose after it observes the replacement goal. Owner-visible rendering must make this ordering understandable without exposing internal envelopes to the model.
- **Validate before enqueue.** Reject an empty or otherwise invalid goal command immediately using the existing command validation. A delivery-time state error leaves the current goal unchanged, reports a precise owner-visible failure, and does not reinterpret the entry as prose.
- **Keep terminal controls authoritative.** A queued goal replacement ordered after a terminal goal-control entry may arm a new goal; one ordered before a later terminal entry is stopped by that later entry. Reuse the race-safe state transition rules owned by the goal-stop ticket rather than adding selective host-queue deletion.

## Constraints

- Pi-local only; do not change shared ws-mcp or shared playbooks.
- Reuse the existing steering/held-input queue and its FIFO wake batching.
- Do not interrupt the current model turn solely to apply the goal change.
- Preserve existing immediate `/goal` behavior while the host is idle.
- Keep queue acknowledgement and final applied/failed feedback distinct so the owner can tell acceptance from execution.

## Phases

### Phase 1: Queue and apply goal replacement at a safe boundary

Add a typed held-input entry for validated goal replacement, integrate it with the existing FIFO steering drain, and apply it before goal reminder reinjection. Keep direct idle command handling unchanged and reuse existing goal state transitions.

Verify active-turn acceptance and acknowledgement, idle behavior, FIFO ordering with prose on both sides, multiple queued replacements, replacement before/after terminal goal controls, no synthetic model message for the command, reminder text using only the resulting goal, delivery-time failure preserving the old goal, wake batching, queue cleanup, and restart/compaction behavior where held steering state is persisted or explicitly non-persistent today.
