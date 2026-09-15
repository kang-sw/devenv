---
title: "Keep Pi mailbox arrivals on the informational push-batch path while the lead is busy"
related:
  260914-feat-ws-pi-mailbox-native-steer-push: source contract whose busy/no-held case is not implemented
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 86fedc1e6333a3df
sage-review-design-reviewed: 86fedc1e6333a3df
---

# Keep Pi mailbox arrivals on the informational push-batch path while the lead is busy

## Background

Read-only verification of the landed Pi mailbox waiter found one uncovered delivery state. When the lead is active and the shared held queue is empty, a mailbox arrival enters the generic raw busy-steer path directly. It therefore reaches the conversation as a direct `ws-mailbox` message rather than as an informational `ws-push-batch` item.

The source ticket requires every drained mailbox envelope to use the shared push FIFO, retain FIFO ordering, carry `state: informational`, and receive the compact batch-card rendering. Existing tests cover dormant wake and mailbox arrival behind an older held push, but not the busy/no-held case; the complete focused suite remains green while this contract is violated.

## Decisions

- Every Pi mailbox arrival must be admitted as an informational `ws-push-batch` item, including when the lead is actively streaming and no item is already held.
- Preserve the existing active steer versus idle wake behavior of the shared push channel; this correction changes the mailbox payload's admission shape, not when the lead is woken or steered.
- Preserve one drain-to-one-delivery semantics and FIFO ordering for multiple envelopes.

## Constraints

- Do not change the host-neutral mailbox store, `mailbox wait` peek semantics, authoritative `mailbox.recv` drain, waiter lifecycle, retry/backoff policy, or owner-lead-only gate.
- Do not change direct-delivery behavior for unrelated raw push families unless required to keep the shared abstraction coherent.
- The regression test must begin from busy lead state with an empty held queue and prove that the delivered message is a `ws-push-batch` containing a `ws-mailbox` item with `state: informational`; it must also exercise the compact mailbox batch renderer rather than accepting the direct-message path.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/test/push-wake.test.ts, agents-plugin-pi/test/push-render.test.ts |
| scope.surface | internal | no package-export or external ws-mcp API change; `sendToLead` is adapter-internal |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | `ws-mailbox` and `ws-push-batch` are existing adapter wire values |
| scope.test_surface | existing | agents-plugin-pi/test/push-wake.test.ts and agents-plugin-pi/test/push-render.test.ts |
| complexity.reuse_points | confirmed | `sendToLead` and `admitPush` in agents-plugin-pi/src/spawner.ts#L988-L1012; existing batch materialization in agents-plugin-pi/src/spawner.ts#L1263-L1374 |
| complexity.side_effect_risk | moderate | changes when a live mailbox arrival is held versus immediately steered |
| risk.correctness | moderate | must retain FIFO while changing the busy/no-held admission branch |
| risk.fit | moderate | the mailbox must use the shared batch without changing unrelated raw-push families |
| risk.test | moderate | existing cases omit mailbox admission from the busy/no-held state |
| risk.security_or_contract | moderate | changes the model-visible mailbox delivery envelope and compact batch rendering |

## Phases

### Phase 1: Close the busy/no-held mailbox delivery gap

Route the uncovered mailbox state through the same informational batch representation used by held and idle delivery. Add a discriminating regression test for busy/no-held admission and retain the existing timeout, drain, backoff, FIFO, rendering, and wake tests. Run the focused mailbox/push suites and the full Pi adapter test suite.
