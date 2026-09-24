---
title: Roll up cumulative descendant usage over the Pi parent-child control channel
blocked-by: 260924-feat-pi-agent-channel-transport
related:
  260923-research-pi-parent-child-loopback-control-channel: source research; its Outcome Ledger is this ticket's authority
  260924-feat-pi-agent-channel-transport: prerequisite transport
  260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint: its checkpoint part waits for this ticket's evicted-baseline design
  260913-bug-ws-pi-cost-footer-cpu-saturation: prior recursive 250-ms polling regression this must not reintroduce
  260912-feat-ws-pi-custom-footer-cost-telemetry: introduced footer cost and the checkpoint
---

# Roll up cumulative descendant usage over the Pi parent-child control channel

## Background

The parent's `refreshAgentTelemetry` re-reads and parses its direct child's whole `session.jsonl` on each refresh (`agents-plugin-pi/src/spawner.ts`, `src/agent-telemetry.ts`). The footer counts only direct child records, and it arms only in a TUI lead or fork (`shouldArmAgentFooter`, `agents-plugin-pi/src/agent-footer.ts`). Grandchild and deeper usage therefore never reaches the root footer. An earlier recursive design with 250-ms polling saturated the CPU.

## Decisions

- Each hop reports its cumulative usage over the channel: itself plus all of its descendants. Last value wins, and deltas are not used, because they break replay safety.
- The parent stores the reported cumulative value in the child's ownership telemetry. The parent is already the single writer of that record.
- After a restart, each hop recomputes its cumulative value from its own session and its children's ownership records. Restarts must not double count.
- The root footer shows the whole subtree's usage. Intermediate hops run in RPC mode without a footer, but they still compute and forward the cumulative value.
- No recursive JSONL scanning or polling is reintroduced.
- Eviction fold rule:
  - The root counts only its direct children's cumulative values.
  - Evicting a direct child adds that child's last stored cumulative value to the evicted baseline. The value already includes the child's descendants.
  - The fold is idempotent per child agent ID: repeating it for the same child is a no-op.
  - The worker chooses the checkpoint schema that realizes this rule and records it in the Result.
- `260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint` consumes this rule and its schema. If the fold requires a checkpoint schema change that affects that ticket's cross-owner writer, record the change in the Result.

## Prior Decisions

- 260913-bug-ws-pi-cost-footer-cpu-saturation (2026-09-13, commit 2f833ec8): "Exact lifetime accounting and recursive nested-child traversal were rejected in favor of the ticket's bounded, monotonic cosmetic estimate." — bearing: constrains
- 260912-feat-ws-pi-custom-footer-cost-telemetry (2026-09-13, commit a075db31): "Child telemetry is persisted in ownership metadata and identity-keyed owner roll-ups so eviction, retention, reload, and retry preserve totals without double counting." — bearing: supports
- 260923-research-pi-parent-child-loopback-control-channel (2026-09-23, Confirmed Decisions): "Scope. The following move to the channel: - execute-approval decisions (A5); - the whole subtree snapshot (A6) ...; - descendant usage (A9), both live and durable cumulative" — bearing: supports
- 260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint (2026-09-24, Decisions): "The cross-owner checkpoint fix waits until the cumulative-usage design ... settles. That design may change the checkpoint schema, including how evicted cost is folded when usage becomes per-hop cumulative." — bearing: constrains
- 260924-feat-pi-agent-channel-subtree-state (2026-09-24, Decisions): "The child sends full revisioned snapshots, and the parent keeps the last revision it has seen." — bearing: supports
- 260924-bug-pi-durable-write-hygiene (2026-09-24, Open Questions): "Should the 5-s observer write only when the session signature changes, or should dormant records stop being observed?" — bearing: constrains
- 260924-research-pi-root-single-authority-durable-state (2026-09-24, Confirmed Decisions): "This question is investigated separately from the parent-child channel research, not inside its scope." — bearing: constrains
- 17dea8a3 (2026-09-10, commit): "Use durable SDK session-entry IDs and an immutable pre-first-prompt prefix anchor because responseId is optional and streaming usage is cumulative." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/agent-telemetry.ts, agents-plugin-pi/src/agent-footer.ts, agents-plugin-pi/src/agent-storage.ts, plus the channel protocol module pending 260924-feat-pi-agent-channel-transport |
| scope.surface | cross-module | spawner refreshAgentTelemetry, footer CostCheckpoint and persistOwnedTelemetryRollup, ownership telemetry in agent-storage, and the channel protocol all change together |
| scope.new_public_symbol | yes | a cumulative-usage snapshot channel message type, kept separate from approval and subtree messages per the research Non-authoritative notes |
| scope.new_type_contract | yes | AgentTelemetry gains a cumulative subtree value stored in ownership.json, and the checkpoint evictedBaseline fold may change the CostCheckpoint schema |
| scope.test_surface | existing | agents-plugin-pi/test/agent-footer.test.ts, agent-telemetry.test.ts, agent-telemetry-contract.test.ts, agent-telemetry-lifecycle.test.ts; a three-level tree case likely adds new cases |
| complexity.reuse_points | unconfirmed | existing CostCheckpoint evictedBaseline and updateOwnership telemetry path read in the tree; the channel transport is not landed yet |
| complexity.side_effect_risk | high | changes durable ownership telemetry and checkpoint state and touches the footer path that previously saturated the CPU |
| risk.correctness | high | last-wins cumulative values must stay replay safe across parent and child restarts, reordering, and a subtree eviction folded exactly once |
| risk.fit | moderate | depends on the unlanded transport and must leave a fold design that 260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint consumes |
| risk.test | high | needs multi-process three-level tree, restart, and reorder coverage; prior round-one review found tests that did not fail on evicted-baseline restore |
| risk.security_or_contract | moderate | durable ownership telemetry and checkpoint schema are read by retention and by another ticket's cross-owner writer |

## Phases

### Phase 1: Cumulative per-hop usage roll-up

Verification:

- A three-level tree (root → child → grandchild) shows grandchild usage in the root footer.
- Parent restart, child restart, and resume each preserve totals without double counting.
- Duplicate or reordered usage messages do not inflate totals.
- Evicting a child that has descendants folds its whole subtree cost into the evicted baseline exactly once.
- No per-event full-file re-read of descendant sessions is performed for the roll-up.
