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
- Folding a child's cumulative subtree usage into the owner checkpoint's evicted baseline without double counting is designed here, and its design is recorded in the Result. `260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint` consumes that design. If the fold requires a checkpoint schema change that affects that ticket's cross-owner writer, record the change in the Result.

## Phases

### Phase 1: Cumulative per-hop usage roll-up

Verification:

- A three-level tree (root → child → grandchild) shows grandchild usage in the root footer.
- Parent restart, child restart, and resume each preserve totals without double counting.
- Duplicate or reordered usage messages do not inflate totals.
- Evicting a child that has descendants folds its whole subtree cost into the evicted baseline exactly once.
- No per-event full-file re-read of descendant sessions is performed for the roll-up.
