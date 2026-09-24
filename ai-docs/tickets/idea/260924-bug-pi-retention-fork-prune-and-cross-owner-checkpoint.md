---
title: Pi retention runs in fork children and folds cost into other owners' checkpoints without a lock
related:
  260923-research-pi-parent-child-loopback-control-channel: source of the cumulative-usage decision
  260924-feat-pi-agent-channel-usage-rollup: the checkpoint part waits for its evicted-baseline design; the fork-prune part does not
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
- Only the tree root lead runs session-start retention. Fork children never run it. When leads from different sessions prune at the same time, the existing ownership lock directory handles the contention. There is no per-machine election.
- The cross-owner checkpoint fix waits for the eviction fold rule and schema from `260924-feat-pi-agent-channel-usage-rollup`. That work may change the checkpoint schema, so fixing it earlier risks rework. Whether the owner performs the fold or a foreign writer is made safe is decided in Phase 2 against that schema.

## Prior Decisions

- 32b01532 (2026-09-13, commit): "Controller-only startup integration avoids redundant global scans in worker/explore children and keeps scan failure diagnostic-only." — bearing: supports
- 32b01532 (2026-09-13, commit): "Age eligibility is rechecked under the claim because the scanner's earlier cutoff observation is insufficient when another process can refresh activity before deletion." — bearing: constrains
- 3cfa8c45 (2026-09-13, commit): "Session files are sampled before the age decision so a final unobserved write refreshes activity; deletion failures remain diagnostic-only and scanning continues across other lead namespaces." — bearing: constrains
- 260924-feat-pi-agent-channel-usage-rollup (2026-09-24, Decisions): "Eviction fold rule: The root counts only its direct children's cumulative values. Evicting a direct child adds that child's last stored cumulative value to the evicted baseline. The fold is idempotent per child agent ID" — bearing: constrains
- 260924-feat-pi-agent-channel-usage-rollup (2026-09-24, Decisions): "`260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint` consumes this rule and its schema. If the fold requires a checkpoint schema change that affects that ticket's cross-owner writer, record the change in the Result." — bearing: supports
- 2f833ec8 (2026-09-13, commit): "One .cost-estimate/checkpoint.json stores lead totals, an evicted scalar baseline, and at most the live registry identities; shutdown, compaction, ordinary stop, and eviction/retention boundaries persist it." — bearing: constrains
- 2f833ec8 (2026-09-13, commit): "Exact lifetime accounting and recursive nested-child traversal were rejected in favor of the ticket's bounded, monotonic cosmetic estimate." — bearing: constrains
- dc857cf3 (2026-09-24, commit): "ownership.json stays on disk because its lock guards cross-session retention an intra-tree channel cannot reach." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1 edits agents-plugin-pi/src/index.ts and agents-plugin-pi/test/session-retention.test.ts; ticket also names process-role.ts, agent-storage.ts, agent-footer.ts |
| scope.surface | internal | applySessionStartAgentRetention is exported from index.ts only as a test seam; no adapter-facing contract changes |
| scope.new_public_symbol | no | none; Phase 1 swaps isLeadOrFork for role === undefined |
| scope.new_type_contract | no | none for Phase 1; Phase 2 checkpoint schema is unsettled pending 260924-feat-pi-agent-channel-usage-rollup and must be re-populated before promotion |
| scope.test_surface | existing | agents-plugin-pi/test/session-retention.test.ts asserts worker/explore skip but has no fork case; agents-plugin-pi/test/ownership-contention.test.ts for Phase 2 |
| complexity.reuse_points | confirmed | readSpawnRole returns undefined for the host lead, agents-plugin-pi/src/process-role.ts#L95-L108; same owner-lead gate precedent in 7f0a0d35 |
| complexity.side_effect_risk | moderate | Phase 1 removes machine-wide prune from fork children; Phase 2 changes cross-process writes to another owner's checkpoint |
| risk.correctness | moderate | the beforeRemove fold at agents-plugin-pi/src/agent-storage.ts#L392 runs before removeOwnedAgentHome takes the ownership lock at agents-plugin-pi/src/agent-storage.ts#L296-L303 |
| risk.fit | moderate | Phase 2 owner-fold versus safe-foreign-writer choice is unsettled and interacts with usage-rollup and root-single-authority research |
| risk.test | moderate | fork-role gate case is missing from session-retention.test.ts and Phase 2 race-freedom needs cross-process contention coverage |
| risk.security_or_contract | low | deletion eligibility and ownership-lock semantics are unchanged by Phase 1; only the role gate narrows |

## Phases

### Phase 1: Restrict retention to the root lead

Change the retention gate in `applySessionStartAgentRetention` (`agents-plugin-pi/src/index.ts`) from `isLeadOrFork(role)` to the lead only. A lead has no spawn role: `role === undefined` (`agents-plugin-pi/src/process-role.ts`).

Verification:

- A fork child's session start runs no prune and no checkpoint fold.
- A lead's session start still prunes stale homes as before.

### Phase 2: Race-free cross-owner checkpoint fold

Blocked until `260924-feat-pi-agent-channel-usage-rollup` records its eviction fold rule and checkpoint schema in its Result. Settle the design against that schema before this phase is promoted. The design covers two races:
- a lost update between the owner's own persist and a foreign writer's fold;
- a double fold. Today the `beforeRemove` fold runs before `removeOwnedAgentHome` takes the ownership lock (`agents-plugin-pi/src/agent-storage.ts`). Two concurrent root leads can therefore fold the same evicted agent twice. A fold whose removal is then retained has the same effect.

The fold must be idempotent per child agent ID, following the usage-rollup rule, and it must take effect only for a removal that actually happens.
