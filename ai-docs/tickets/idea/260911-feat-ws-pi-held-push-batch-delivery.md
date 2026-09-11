---
title: Batch accumulated Pi adapter push messages per wake
related:
  260911-bug-ws-pi-question-queue-dogfood: dogfood session that exposed one-at-a-time model delivery after a counted wake
---

# Batch accumulated Pi adapter push messages per wake

## Background

The Pi adapter currently drains every record from its held push queue when a counted wake starts, but submits each record as a separate steering message. With Pi's default `one-at-a-time` steering mode, the model therefore receives one accumulated ws message per assistant turn even though the adapter flushed the queue together.

The owner has changed the local Pi steering mode to `all`, which is an adequate current workaround and makes this follow-up non-urgent. That setting is global, however: it also changes non-ws steering behavior and should not become an adapter-owned default merely to batch ws traffic.

## Decisions

- Long term, one counted wake should deliver the ws messages accumulated behind that wake as one ws-owned batch rather than relying on Pi's global steering mode.
- Keep this in `idea/` as a non-urgent redesign while the owner's `all` setting provides acceptable behavior.
- Do not have the adapter silently change or persist the global Pi steering mode.
- The concrete batch envelope, transcript representation, rendering, size bound, and mixed approval/question/final semantics remain intentionally unsettled at this stage.

## Phases

### Phase 1: Define and implement ws-owned held-push batching

Specify and implement an adapter-local batch delivery contract so all ws pushes held behind one confirmed wake become model-visible together without changing global steering behavior. Preserve deterministic FIFO interpretation and explicitly settle how per-item identity, details, status snapshots, transcript replay, rendering, oversized batches, and send failures behave before promoting this ticket to implementation-ready status.

Verify the behavior under both Pi steering modes, with mixed report, approval, question, advisory, final, and raw-summary payloads. Retain the existing confirmed-start and wake-recovery guarantees, and prove that one wake does not require one model response per held item.
