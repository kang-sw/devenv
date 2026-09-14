---
title: "Pi ownership observer emits transient lock contention into the TUI"
related:
  260908-feat-ws-pi-agent-session-disk-retention: introduced durable owned-home locking and periodic session observation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b1439cdfaeca9a15
sage-review-completeness-reviewed: b1439cdfaeca9a15
completed: 2026-09-13
---

# Pi ownership observer emits transient lock contention into the TUI

## Background

During live Pi dogfood with workers active, the lead TUI repeatedly received raw diagnostics shaped as:

```text
ws-pi-agent: could not observe owned session write: Error: EEXIST: file already exists, mkdir '<lead-session-dir>/.<agent-id>.ownership-lock'
```

The lock directory is a sibling of the agent home, not a child of it (agents-plugin-pi/src/agent-storage.ts#L113-L118). The two observed messages named different agent IDs, and each lock directory had already disappeared when inspected. Source inspection shows that `startOwnedSessionObserver` performs a best-effort sample every five seconds, `acquireOwnershipLock` intentionally refuses to reclaim a claim held by a live process, and `observeSessionWrite` catches the resulting `EEXIST` but writes it through `console.error`. Pi surfaces that stderr directly inside the owner TUI, interrupting normal interaction even though a later observer sample can retry.

This is distinct from authoritative ownership mutation and deletion. Those operations must continue to fail closed under contention so that local state, durable protection, retention, and deletion cannot diverge.

## Decisions

- Treat `EEXIST` caused by a live holder during `observeSessionWrite` as an expected transient busy result, not an owner-visible error.
- Keep the observer best-effort: skip the contended sample and rely on the next lifecycle or periodic sample rather than waiting synchronously on the Pi event loop.
- Limit suppression to the observation path. Do not weaken or silence contention failures for ownership updates, protection changes, owner-thread binding, transcript references, retention eligibility, or deletion.
- Preserve diagnostics for malformed or unreadable lock-owner facts, non-`EEXIST` I/O failures, and stale-lock recovery failures.

## Constraints

- Do not delete or reclaim a lock held by a live PID.
- Do not introduce blocking sleeps, busy waiting, unbounded retries, or a new high-frequency timer.
- The fix must not convert an authoritative failed write into accepted in-memory state.
- A later successful observation must still update the session signature and activity facts after a transient busy sample.
- Tests must distinguish the expected observer-only busy case from failures that remain diagnostic and fail closed.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-storage.ts plus agents-plugin-pi/test/agent-storage.test.ts or agents-plugin-pi/test/ownership-contention.test.ts |
| scope.surface | internal | no public tool or exported plugin interface change is named |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | no type or signature is named |
| scope.test_surface | existing | existing agents-plugin-pi/test/agent-storage.test.ts covers observer diagnostics; agents-plugin-pi/test/ownership-contention.test.ts covers cross-process claims |
| complexity.reuse_points | confirmed | acquireOwnershipLock and observeSessionWrite in agents-plugin-pi/src/agent-storage.ts already provide the claim and sample paths |
| complexity.side_effect_risk | high | misclassification could hide a genuine observation failure or change liveness under contention |
| risk.correctness | high | the busy path must not weaken durable ownership, retention, or deletion safety |
| risk.fit | high | only observer sampling may be silent while authoritative callers remain fail closed and diagnostic |
| risk.test | high | a deterministic live-lock collision, later release, and diagnostic failures need coverage |
| risk.security_or_contract | high | the durable lock is the ownership boundary protecting retention and deletion |

## Phases

### Phase 1: Keep expected observer contention out of the TUI

Introduce an explicit transient-busy classification at the narrowest ownership-lock boundary and consume it silently only from `observeSessionWrite`. Preserve every existing fail-closed caller contract and stale dead-owner recovery behavior. Add cross-process or deterministic lock-fixture coverage proving that a live-holder observer collision emits no `console.error`, leaves ownership metadata unchanged for that sample, succeeds on a later sample after release, and does not suppress malformed-lock or non-observation diagnostics.

### Result (a8a4abe) - 2026-09-13

A confirmed live lock holder now produces an internal transient-busy error that only `observeSessionWrite` consumes silently; authoritative updates still report contention and fail without accepting state. Malformed owner facts, unrelated I/O failures, and stale-lock recovery failures retain their diagnostics and conservative behavior.

A real subprocess lock barrier verifies the silent skipped sample, unchanged metadata, later signature/activity recovery, authoritative write failure, and malformed-lock diagnostics. Focused storage/contention tests passed 26/26; the full Pi suite passed 1,778 tests with 2 expected skips and no failures. Independent correctness, fit, and test reviews were clean with no findings.


## Resolution (2026-09-13)

Implemented observer-only transient live-lock suppression while preserving authoritative fail-closed ownership semantics and diagnostics. Cross-process regression coverage and the full Pi suite passed; partitioned correctness, fit, and test reviews were clean.
