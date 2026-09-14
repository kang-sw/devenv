---
title: ws-agent-list shows each agent's model, and a revived orphan keeps its last-report time
related:
  260905-feat-ws-pi-agent-alias-park-and-registry-cap: owns the ws-agent-list shape and the dormant sidecar
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 63df6d2f98b0dc3b
sage-review-completeness-reviewed: 63df6d2f98b0dc3b
completed: 2026-09-06
---

# ws-agent-list shows each agent's model, and a revived orphan keeps its last-report time

## Background

Two fidelity gaps surfaced in the 2026-09-05 acceptance run of the Pi
adapter:

- **D4.** The run asked the lead to compare the model of a default
  `ws-execute` worker against one spawned with `complex:true`. `ws-agent-list`
  exposes no model field, so the lead could not tell them apart even though
  every record already carries `modelBase` and `modelEffort` (the sidecar
  persists both). The only signal the lead saw was the unset-catalog advisory
  mentioning parent-model inheritance.
- **H.** After `/reload`, the restored registry entries had lost the
  `last_report_at` value they showed before. The sidecar writes
  `lastReportAt` for the orphan roll-call, but `rehydrateOrphanRecord`
  rebuilds the record with an empty `reportLog`, so the listing derives no
  last-report time for a revived agent.

## Decisions

- **`model` is a listing field, derived, never stored twice.** Each
  `ws-agent-list` entry gains `model`: `modelBase` plus `/<effort>` when
  `modelEffort` is set. It reports the effective resolved model the child
  was launched with: `resolveModelForAlias` falls back to the parent's
  concrete model string when the catalog has no entry for the alias, so an
  inheriting child shows the parent's model by name rather than an absent
  field. The tool description says so ("the model the agent runs on; an
  inheriting child shows its parent's model"). The field is omitted only
  when the record carries no `modelBase` at all (a sidecar written before
  the field existed). Rejected: an inherited-vs-explicit provenance flag,
  because the lead already knows which spawns it asked for by tier and the
  value it needs for the D4 comparison is the concrete model.
- **`lastReportAt` rides on the record, not on a synthetic report.**
  `RpcAgentRecord` gains an optional `lastReportAtOverride` (ISO string) that
  `rehydrateOrphanRecord` fills from the sidecar; the listing's
  `last_report_at` derivation prefers the newest `reportLog` entry and falls
  back to the override. The registry-cap eviction score in `spawner.ts`
  (`max(lastLeadPromptAt, newest reportLog.at)`) consumes the same fallback,
  so a revived orphan is scored by its real last activity instead of as
  never-active. Rejected: injecting a fake `reportLog` entry, since
  `reportLog` entries carry a kind and text that a revived record does not
  have, and downstream code reads them as real reports.
- **No sidecar shape change.** The sidecar already persists `modelBase`,
  `modelEffort`, and `lastReportAt`; only the read side changes.

## Constraints

- The listing's existing fields and ordering are unchanged; the two fields
  are additive.
- A record that has reported since revival shows the real newest report
  time, never the stale override.

## Spec Impact

`pi-adapter-runtime`: amend the alias/park/cap listing entry in the
delegation-spawner anchor with the `model` field (effective resolved model;
inheriting child shows the parent's), and the shutdown-sidecar text so a
revived record keeps its last-report time in both the listing and the
eviction score.

## Phases

### Phase 1: List model and keep last-report time across revival

In `spawner.ts`: add `model` to the `ws-agent-list` entry builder, and the
`lastReportAtOverride` fallback to both the `last_report_at` derivation and
the eviction activity score; in `agent-sidecar.ts`: fill the override in
`rehydrateOrphanRecord`. Tests: a record with `modelBase` and `modelEffort`
lists `model: "<base>/<effort>"`; a record with `modelBase` only lists the
bare base; a record with neither has no `model` key; a rehydrated orphan
lists the sidecar's `last_report_at`; a rehydrated orphan that reports
afterwards lists the new time; eviction prefers to drop a never-active
record over a revived orphan whose override is newer. Amend the two spec
passages. Live check (owner-run): with no model catalog configured, spawn a
`complex:true` execute worker and a default one and confirm both list the
lead's own concrete model (they resolve identically without a catalog);
optionally configure a catalog entry for the complex alias and confirm the
two values then differ; finally `/reload` and confirm `last_report_at`
survives.

### Result (94c028ca) - 2026-09-06

Landed as `69d13167` (survey plan), `94c028ca` (code and the six ticket
tests), `874379db` (spec), `c3dc2766` (review relay #1) on the
implementation branch under the goal branch. Adapter-only change.

- `listAgents` adds `model` as `modelBase` plus `/<effort>` when an effort
  is set, omitted when the record carries no `modelBase` (pre-field sidecar
  or a fresh spawn whose alias resolution returned no model).
  `last_report_at` prefers the newest `reportLog` entry and falls back to
  the new `lastReportAtOverride` record field; `evictForCapacity` scores
  activity with the same fallback. `rehydrateOrphanRecord` fills the
  override from the sidecar's `lastReportAt`; no sidecar shape change.
- Relay #1 additionally made `captureOrphans` persist the override when
  `reportLog` is empty (a revived orphan that never reports again now
  survives a second shutdown) and made `parseOrphans` drop a
  `lastReportAt` that does not parse as a date.
- Tests: the six ticket cases plus three from relay (capture round trip,
  non-date sidecar value, a real report beating a numerically newer
  override). Adapter suite 745 pass, 0 fail.
- Spec: `ws-agent-list` bullet and the shutdown-sidecar paragraph under the
  delegation-tools anchor amended; ids unchanged.

Review (partitioned correctness/test): correctness one Important (capture
side dropped the override) and two Minor; test one Critical (the eviction
test tied at activity 0 and passed under the pre-fix formula) and one
Minor. All fixed in relay #1; the Critical-scoped re-review confirmed the
rewritten test fails with the fallback removed.

Owner-run live check outstanding: the catalog/no-catalog model listing and
the `/reload` survival check against the merged goal branch.

## Blocked (2026-09-06) — owner sign-off pending, not a work item

Phase 1 carries a Result; no autonomous work remains. Closing waits on the
owner-run live check above. Once confirmed, close the ticket to `.done/`.


## Resolution (2026-09-06)

Owner-run live check on 2026-09-06 passed: both agents kept their `provider/id` model strings across the list calls and their last-report timestamps stayed unchanged between reads.
