---
title: Pi session-start retention runs in fork children
related:
  260924-bug-pi-retention-cross-owner-checkpoint-fold: split out; owns the unlocked cross-owner checkpoint fold, which waits for the usage-rollup schema
  260923-research-pi-parent-child-loopback-control-channel: inventory source
  260924-research-pi-root-single-authority-durable-state: may later reassign who authors durable state; this ticket fixes today's concrete defect
  260908-feat-ws-pi-agent-session-disk-retention: introduced session-start retention
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ab46fc75e480a66a
sage-review-completeness-reviewed: ab46fc75e480a66a
---

# Pi session-start retention runs in fork children

## Background

Found during the side-channel inventory of `260923-research-pi-parent-child-loopback-control-channel`. The defect is independent of any parent-child transport.

`applySessionStartAgentRetention` (`agents-plugin-pi/src/index.ts`) gates on `isLeadOrFork(role)`, so every fork child runs `pruneStaleAgentHomes` at session start, not only the lead. The prune scans every owner namespace under `<agentDir>/ws-agents/*/*`, including other Pi sessions' namespaces (`agents-plugin-pi/src/agent-storage.ts`, `pruneStaleAgentHomes`), and deletes stale homes. The original intent was retention run only by the controlling lead (32b01532).

The unlocked cross-owner checkpoint fold that retention performs before deletion was split into `260924-bug-pi-retention-cross-owner-checkpoint-fold`, because its fix waits for the usage-rollup checkpoint schema.

## Decisions

- Only the tree root lead runs session-start retention. Fork children never run it. When leads from different sessions prune at the same time, the existing ownership lock directory serializes the deletions. There is no per-machine election.
- This fix has no ordering dependency.

## Prior Decisions

- 32b01532 (2026-09-13, commit): "Controller-only startup integration avoids redundant global scans in worker/explore children and keeps scan failure diagnostic-only." — bearing: supports
- 32b01532 (2026-09-13, commit): "Age eligibility is rechecked under the claim because the scanner's earlier cutoff observation is insufficient when another process can refresh activity before deletion." — bearing: constrains
- 3cfa8c45 (2026-09-13, commit): "Session files are sampled before the age decision so a final unobserved write refreshes activity; deletion failures remain diagnostic-only and scanning continues across other lead namespaces." — bearing: constrains
- dc857cf3 (2026-09-24, commit): "ownership.json stays on disk because its lock guards cross-session retention an intra-tree channel cannot reach." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | single-file | agents-plugin-pi/src/index.ts applySessionStartAgentRetention, plus agents-plugin-pi/test/session-retention.test.ts |
| scope.surface | internal | applySessionStartAgentRetention is exported from index.ts only as a test seam; no adapter-facing contract changes |
| scope.new_public_symbol | no | none; the gate swaps isLeadOrFork for role === undefined |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-pi/test/session-retention.test.ts asserts worker/explore skip but has no fork case |
| complexity.reuse_points | confirmed | readSpawnRole returns undefined for the host lead, agents-plugin-pi/src/process-role.ts#L95-L108; same owner-lead gate precedent in 7f0a0d35 |
| complexity.side_effect_risk | low | removes the machine-wide prune from fork children only; deletion eligibility and locking are unchanged |
| risk.correctness | low | a one-predicate gate change with existing role tests |
| risk.fit | low | restores the controller-only intent of 32b01532 |
| risk.test | low | add a fork case beside the existing worker/explore cases |
| risk.security_or_contract | low | deletion eligibility and ownership-lock semantics are unchanged; only the role gate narrows |

## Phases

### Phase 1: Restrict retention to the root lead

Change the retention gate in `applySessionStartAgentRetention` (`agents-plugin-pi/src/index.ts`) from `isLeadOrFork(role)` to the lead only. A lead has no spawn role: `role === undefined` (`agents-plugin-pi/src/process-role.ts`).

Verification, in `agents-plugin-pi/test/session-retention.test.ts`:

- A fork child's session start runs no prune and no checkpoint fold.
- A lead's session start still prunes stale homes as before.
- The existing worker and Explore skip cases still pass.
