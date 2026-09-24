---
title: Pi retention folds cost into other owners' checkpoints without a lock
blocked-by: 260924-feat-pi-agent-channel-usage-rollup
related:
  260924-feat-pi-agent-channel-usage-rollup: prerequisite; its Result records the eviction fold rule and checkpoint schema this ticket builds on
  260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint: split from it; that ticket keeps the fork-child prune gate
  260924-research-pi-root-single-authority-durable-state: may later reassign who authors durable state; this ticket fixes today's concrete races
  260908-feat-ws-pi-agent-session-disk-retention: introduced session-start retention
---

# Pi retention folds cost into other owners' checkpoints without a lock

## Background

Split from `260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint`, which keeps the fork-child prune gate. This defect is independent of any parent-child transport. Its fix depends on the checkpoint schema that `260924-feat-pi-agent-channel-usage-rollup` settles.

Before deleting a home, retention calls `persistOwnedTelemetryRollup` (`agents-plugin-pi/src/agent-footer.ts`). It loads the owning session's `.cost-estimate/checkpoint.json`, folds the evicted agent's cost into `evictedBaseline`, and writes it back. The write is an atomic rename, but the read-modify-write takes no lock. The writer may be a different process, or a different Pi session, from the checkpoint's owner. Two races follow:

- **Lost update.** The fold can race with the owner's own footer `persist()`, and one of the two writes is lost.
- **Double fold.** The `beforeRemove` fold runs before `removeOwnedAgentHome` takes the ownership lock (`agents-plugin-pi/src/agent-storage.ts`). Two concurrent root leads from different sessions can therefore fold the same evicted agent twice. A fold whose removal then ends up retained has the same effect.

## Decisions

- The fix follows the usage-rollup eviction fold rule: a fold happens at most once per actual removal of a child home. It takes effect only for a removal that actually happens.
- Whether the owner performs the fold, for example from an eviction record it consumes on its next persist, or whether a foreign writer is made safe, is decided against the usage-rollup checkpoint schema before this ticket is promoted.

## Phases

### Phase 1: Race-free cross-owner checkpoint fold

Design and verification are settled after `260924-feat-pi-agent-channel-usage-rollup` records its Result, and before this ticket is promoted. Verification must include cross-process contention coverage for both races, for example in `agents-plugin-pi/test/ownership-contention.test.ts`.
