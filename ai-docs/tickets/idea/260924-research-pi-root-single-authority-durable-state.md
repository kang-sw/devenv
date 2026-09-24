---
title: Investigate root single-authority authoring of durable Pi tree state
related:
  260923-research-pi-parent-child-loopback-control-channel: provides the parent-child transport that descendants would use to send updates upward
  260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint: concrete retention defect this research may reframe
  260924-bug-pi-retention-cross-owner-checkpoint-fold: concrete multi-writer checkpoint defect this research may reframe
  260924-bug-pi-durable-write-hygiene: write-mechanics fixes that stand regardless of this research
---

# Investigate root single-authority authoring of durable Pi tree state

## Background

Several processes write durable Pi adapter state:

- Each direct parent writes its children's `ownership.json` under a lock directory.
- A 5-s observer, and retention running in any lead or fork process, also write `ownership.json`.
- The cost checkpoint is written by its owner's footer, by eviction, and by foreign retention processes.

The side-channel inventory in `260923-research-pi-parent-child-loopback-control-channel` raised an alternative. Descendants would send durable updates upward over the parent-child channel, and only the tree's root Pi process would write them to disk. This would remove multi-writer filesystem contention. That research deliberately kept this question out of its scope.

## Questions

- Which durable records are candidates? Candidates include ownership records, cost checkpoints, retention decisions, and registries. Which must stay with the process that recovers them? Today the direct parent resumes its own children from their records.
- Retention scans every owner namespace on the machine, including other Pi sessions' namespaces. Authority at the tree root therefore does not remove cross-session contention. Does a per-machine authority, a per-namespace owner rule, or a lock still have to cover that?
- What happens to durable writes when the root is absent, restarting, or crashed while descendants keep running or resume?
- Does routing writes through the root create a single point of failure or a latency path that outweighs the removed lock contention?

## Outcome Ledger

### Verified Findings

- `pruneStaleAgentHomes` iterates every owner directory under `<agentDir>/ws-agents/` and every home under each, not only the caller's own namespace (`agents-plugin-pi/src/agent-storage.ts`).
- `ownership.json` for a child is written by that child's direct parent, not by the tree root (`agents-plugin-pi/src/spawner.ts`, `src/agent-storage.ts`).

### Confirmed Decisions

- This question is investigated separately from the parent-child channel research, not inside its scope.

### Proposals

### Open Questions

- All questions under `## Questions` above remain open.

### Rejected Alternatives
