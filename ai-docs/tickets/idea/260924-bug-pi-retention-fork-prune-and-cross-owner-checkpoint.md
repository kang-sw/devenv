---
title: Pi retention runs in fork children and folds cost into other owners' checkpoints without a lock
related:
  260923-research-pi-parent-child-loopback-control-channel: the checkpoint part waits for this research's cumulative-usage design
  260924-research-pi-root-single-authority-durable-state: may later reassign who authors durable state; this ticket fixes today's concrete defects
  260908-feat-ws-pi-agent-session-disk-retention: introduced session-start retention
---

# Pi retention runs in fork children and folds cost into other owners' checkpoints without a lock

## Background

Found during the side-channel inventory of `260923-research-pi-parent-child-loopback-control-channel`. Both defects are independent of any parent-child transport.

1. **Fork children run a machine-wide prune.** `applySessionStartAgentRetention` (`agents-plugin-pi/src/index.ts`) gates on `isLeadOrFork(role)`, so every fork child runs `pruneStaleAgentHomes` at session start, not only the lead. The prune scans every owner namespace under `<agentDir>/ws-agents/*/*`, including other Pi sessions' namespaces (`agents-plugin-pi/src/agent-storage.ts`, `pruneStaleAgentHomes`), and deletes stale homes.
2. **Unlocked cross-owner checkpoint update.** Before deleting a home, retention calls `persistOwnedTelemetryRollup` (`agents-plugin-pi/src/agent-footer.ts`). It loads the owning session's `.cost-estimate/checkpoint.json`, folds the evicted agent's cost into `evictedBaseline`, and writes it back. The write is an atomic rename, but the read-modify-write takes no lock. It can therefore race with that owner's own footer `persist()` and lose an update. The writer may be a different process, or a different Pi session, from the checkpoint's owner.

## Decisions

- The fork-child prune gate has no ordering dependency and may be fixed at any time.
- The cross-owner checkpoint fix waits until the cumulative-usage design in `260923-research-pi-parent-child-loopback-control-channel` settles. That design may change the checkpoint schema, including how evicted cost is folded when usage becomes per-hop cumulative. Fixing it earlier risks rework.

## Open Questions

- Which role should run retention: only the tree root lead, or one elected process per machine? Retention is cross-session by construction, so restricting it to the tree root still leaves two leads from different sessions pruning concurrently.
- Should the checkpoint fold be moved to the owner, for example by leaving an eviction record that the owner folds on its next persist, or should it be made safe for a foreign writer?
