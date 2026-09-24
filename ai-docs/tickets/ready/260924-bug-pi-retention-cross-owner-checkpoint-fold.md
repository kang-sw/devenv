---
title: Pi retention folds cost into other owners' checkpoints without a lock
related:
  260924-feat-pi-agent-channel-usage-rollup: prerequisite (done); its Result records the checkpoint schema and eviction fold rule this ticket replaces for removals
  260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint: split from it; that ticket keeps the fork-child prune gate
  260924-research-pi-root-single-authority-durable-state: may later reassign who authors durable state; this ticket fixes today's concrete races
  260908-feat-ws-pi-agent-session-disk-retention: introduced session-start retention
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 94d733c9f1e322dc
sage-review-completeness-reviewed: 94d733c9f1e322dc
---

# Pi retention folds cost into other owners' checkpoints without a lock

## Background

Split from `260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint`, which keeps the fork-child prune gate. This defect is independent of any parent-child transport.

The root lead runs `pruneStaleAgentHomes` at session start (`agents-plugin-pi/src/index.ts`, `agents-plugin-pi/src/agent-storage.ts`). The prune scans every owner under `ws-agents/`. Before each removal it calls `beforeRemove = persistOwnedTelemetryRollup` (`agents-plugin-pi/src/agent-footer.ts`). That call loads the owning session's `.cost-estimate/checkpoint.json`, adds the child's subtree cost to `evictedBaseline`, and writes the file back. The write is an atomic rename, but the read-modify-write takes no lock. Four defects follow:

- **Fold without removal.**
  - The fold runs before `removeOwnedAgentHome` takes the ownership lock and re-checks eligibility. A home that is then retained (busy lock, liveness changed) or whose detach fails is already folded.
  - A later prune folds it again. This needs no race.
- **Concurrent root leads.** Two root leads from different sessions can fold the same child twice.
- **Live owner overwrites.**
  - A live owner process persists its in-memory checkpoint and overwrites the retention fold. This is deterministic, not only a race.
  - If the owner's registry still holds the removed child, the child is counted through `agents[]` until it leaves the registry. It is then dropped, and its cost is lost.
- **Lost update.** A retention fold racing the owner's own footer `persist()` loses one of the two writes.

## Decisions

- **Per-child eviction record.** Every removal of a direct child home records that child's subtree cost in its own file, `ws-agents/<owner>/.cost-estimate/evicted/<agentId>.json`. It holds the child's own usage plus its stored descendant usage, in the same `CumulativeCost` unit as the checkpoint. Retention never writes `checkpoint.json`.
- **Written under the removal lock, only for a real removal.**
  - `removeOwnedAgentHome` writes the record (atomic write) while it holds the ownership lock, after the final eligibility check passes and before the home is detached.
  - If the removal ends with the home back at its path, it deletes the record, so a record exists only for a home that was, or is being, removed. See the stale-record rule below.
  - Folding moves out of `beforeRemove` into the opt-in record step described under "One record writer per removal".
  - Only retention and capacity eviction write records. `reviveOrphans` also discards a stale sidecar duplicate whose agentId is still registered from a different home (`agents-plugin-pi/src/agent-sidecar.ts`), and that discard writes none. The live registration carries that agentId's cost, and a record would wrongly supersede it. Today that discard folds nothing either.
- **Idempotent by construction.** The record is keyed by agentId, so writing it twice leaves one file. Two root leads racing on the same child cannot double count.
- **Records are summed at read time, never folded.**
  - A hop's removed-children total is `checkpoint.evictedBaseline + Σ(eviction records)`.
  - No step moves a record's value into `evictedBaseline` and then deletes the record. That removes every fold-then-delete crash window.
  - Every reader of the evicted total uses this sum: the footer, `descendantUsageValue`, and the checkpoint load.
- **A record supersedes every live count of the same agentId.**
  - An agentId with an eviction record is excluded from `agents[]`, from the registry-resident direct-child sum, and from sidecar revival (`parseOrphans`/`reviveOrphans`, alongside `260924-feat-pi-agent-channel-usage-rollup`'s removed-home skip).
  - This covers two cases: a live owner whose registry still holds the removed child, and a crash between writing the record and detaching the home.
- **Owner capacity eviction uses the same records.**
  - Capacity eviction already removes the child's home through `removeOwnedAgentHome` (`spawner.ts`) before `persistEvictedAgentCost`. That removal writes the eviction record (see "One record writer per removal"), and `foldAndPersist` stops adding to `evictedBaseline`.
  - `evictedBaseline` becomes legacy, read-only: loaded and summed, never increased. No new writer folds into it. This deliberately supersedes `260913-bug-ws-pi-cost-footer-cpu-saturation`'s "fold into the baseline exactly once". Its exactly-once intent is kept by the agentId-keyed record.
  - `checkpoint.json` is then written only by its owner process (`persist`), and it holds only `lead` and the live `agents[]`.
- **A removed agentId never runs again.**
  - An agentId that has a record is never rehydrated or relaunched, whether its home is gone or still present (removal-pending). Two paths could otherwise bring it back:
    - a fork rehydrated from `thread.forkResume` (`ask.ts` `ensureRespondent` → `rehydrateForkRecord`);
    - a live owner's registry record of the removed child, reached through send or relaunch.
  - Both paths refuse with an explicit error saying the agent was removed by retention or eviction.
  - The worker first confirms which of these paths can reach a relaunch today, then gates each one it finds.
  - A live owner that sees a record for a registry-resident agentId whose home is gone drops that entry from its registry. A home-present one is removal-pending; see the stale-record rule.
  - Rejected alternative: counting live cost above the recorded value. The relaunched session starts empty, so its cost would be lost or its identity conflated.
- **A stale record is repaired under the lock.**
  - A record is stale when its agentId's home is present at the original path.
  - `removeOwnedAgentHome` deletes the record whenever it ends with the home at its original path. That covers retained at the final check, a detach failure, and a successful rename-back.
  - If the home is gone from its path, for example because the rename-back failed and only the staged directory remains, the record is kept.
  - A recorded agentId whose home is still present is removal-pending. No other path deletes its record, and no path resurrects it. The only repair is a `removeOwnedAgentHome` call that ends with the home in place; otherwise the next prune completes the removal.
  - While the record exists, the child counts once, through the record. A live owner keeps a home-present recorded agentId registered but excludes it from live counts. It drops a registry entry only when the home is gone. After a repair deletes the record, the registry entry or the next sidecar revival counts the child again, exactly once.
  - A busy ownership lock returns retained without holding the lock, so it attempts no repair.
  - If the atomic record write fails, the removal aborts before the detach and returns retained or failed. No home is removed without a record.
- **One record writer per removal.**
  - The record is written inside `removeOwnedAgentHome` as an opt-in step. The caller supplies the cost through an option or callback, in the style of the existing `remove`/`stillEligible` pattern.
  - The stale-duplicate discard does not opt in.
  - `persistEvictedAgentCost` writes no second record for an owned candidate. An unowned candidate has no home, so its owner writes the record directly.
  - **Value.** The record value is the merged floor that today's folds use: `mergeAgentCost` of the checkpoint's `agents[]` entry for retention, or of the in-memory entry for capacity eviction, with the child's subtree cost. A rewrite merges with the existing record instead of letting the last write win.
  - **Import cycle.** `agent-storage.ts` must not import `agent-footer.ts`. The cost arrives through the callback, or the cost helpers move to `agent-telemetry.ts`.
- **Freshness.**
  - A live owner re-reads `evicted/` at `reconcile` time, which runs on refresh and on `descendantUsageValue`. The read is cached on a cheap change signal, such as the directory's mtime, so a footer render never scans the directory. This keeps the fix from `260913-bug-ws-pi-cost-footer-cpu-saturation`.
  - Records are always read from disk, never through the retained failed-checkpoint cache.
  - The exclusion of recorded agentIds is applied in memory at `reconcile` and to the `agents[]` that `persist` writes.
- **Storage.**
  - `ownerArtifactDirectory` gains nested-bucket support for `.cost-estimate/evicted/`, with the same canonical containment and symlink refusal.
  - The record rollback needs an owner-artifact removal helper.
- **No compaction.** Eviction records accumulate, one small file per removed child, for as long as the owner's storage exists. Compaction would reintroduce a read-modify-write across writers.
- **Rejected alternative.** An owner-level checkpoint lock, with `persist()` re-reading and merging from disk and a folded-agentId set recorded in the checkpoint. It puts a cross-process lock, with stale-lock handling, on the per-turn persist path, and it still needs an idempotency set. It is more complex and riskier than per-child records.

## Prior Decisions

- 260924-feat-pi-agent-channel-usage-rollup (2026-09-24, Result 19e57c98): "Fold rule. A fold happens once per actual removal: capacity eviction (`foldAndPersist`); retention (`persistOwnedTelemetryRollup`). Each folds the whole subtree: own plus the stored descendant value." — bearing: constrains
- 260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint (2026-09-24, Result f3b93840): "`applySessionStartAgentRetention` now gates on `role !== undefined`. Only the tree-root lead runs `pruneStaleAgentHomes` and its checkpoint fold at session start." — bearing: supports
- 32b01532 (2026-09-13, commit): "Retention must never delete a home while activity, protection, or dormant-resume state is being established; one sibling filesystem claim now serializes metadata writers with final eligibility and atomic detachment." — bearing: constrains
- 260924-bug-pi-durable-write-hygiene (2026-09-24, Result 9310cda9): "New leaf module `agents-plugin-pi/src/atomic-write.ts` exports `renameWithWindowsRetry` and `RenameRetryHooks`, holding the EPERM/EBUSY retry loop and hook seam extracted from `writePrivateJson`." — bearing: supports
- 270ac98f (2026-09-13, commit): "Owner-scoped roll-up path creation/read/write belongs to agent-storage so it shares canonical containment and symlink refusal instead of duplicating partial safety in the presentation module." — bearing: constrains
- 260913-bug-ws-pi-cost-footer-cpu-saturation (2026-09-13, Decisions): "Cache one `evictedBaseline` plus cumulative telemetry snapshots for the current in-memory agent registry... when a record is evicted, fold its last cumulative value into the baseline exactly once." — bearing: contradiction-candidate
- 260913-bug-ws-pi-ownership-observer-lock-contention-spams-tui (2026-09-13, Decisions): "Do not weaken or silence contention failures for ownership updates, protection changes, owner-thread binding, transcript references, retention eligibility, or deletion." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-storage.ts, agents-plugin-pi/src/agent-footer.ts, agents-plugin-pi/src/agent-sidecar.ts, agents-plugin-pi/src/index.ts, agents-plugin-pi/src/spawner.ts (capacity eviction, sendToAgent relaunch), agents-plugin-pi/src/ask.ts (ensureRespondent -> rehydrateForkRecord gate), agents-plugin-pi/test/ownership-contention.test.ts |
| scope.surface | cross-module | exported removeOwnedAgentHome and pruneStaleAgentHomes in agent-storage.ts, persistOwnedTelemetryRollup, persistEvictedAgentCost and descendantUsageValue in agent-footer.ts, parseOrphans and reviveOrphans in agent-sidecar.ts, callers in index.ts#L290 and spawner.ts#L2927-L2937; relaunch/rehydrate gates in spawner.ts sendToAgent and ask.ts ensureRespondent |
| scope.new_public_symbol | yes | an opt-in cost option or callback on removeOwnedAgentHome, a nested-bucket owner-artifact path and an owner-artifact removal helper in agent-storage.ts; exact names left to the worker |
| scope.new_type_contract | yes | new on-disk eviction record ws-agents/<owner>/.cost-estimate/evicted/<agentId>.json holding a CumulativeCost; checkpoint evictedBaseline becomes read-only legacy |
| scope.test_surface | existing | test/ownership-contention.test.ts, test/session-retention.test.ts, test/agent-footer.test.ts, test/agent-usage-rollup.test.ts, test/agent-usage-rollup.integration.test.ts, test/agent-sidecar.test.ts; a sibling test file is allowed |
| complexity.reuse_points | confirmed | ownership lock and final eligibility check in removeOwnedAgentHome agent-storage.ts#L325-L368; atomic temp-rename writeOwnerArtifact agent-storage.ts#L108-L121; ownerArtifactDirectory accepts only a single dot-prefixed bucket agent-storage.ts#L63-L75, so the nested evicted directory needs a helper extension |
| complexity.side_effect_risk | high | changes durable cost accounting on every removal path, including the reviveOrphans stale-duplicate discard at agent-sidecar.ts#L395-L401 |
| risk.correctness | high | cross-process races, crash windows, and live-count exclusion must each count a child exactly once |
| risk.fit | moderate | adds a durable artifact kind while 260924-research-pi-root-single-authority-durable-state may later reassign durable-state authorship |
| risk.test | high | verification requires multi-process race, lost-update, and crash-window tests |
| risk.security_or_contract | moderate | new files under owner storage must keep canonical containment and symlink refusal, and the checkpoint evictedBaseline contract changes to read-only |

## Phases

### Phase 1: Race-free per-child eviction records

Add the eviction record, write it inside `removeOwnedAgentHome`'s lock, and convert retention and owner capacity eviction to it. Make every evicted-total reader sum the records, and exclude recorded agentIds from live counts and revival.

Verification:

- **Retained after the final check:** a stale home that fails the final eligibility check leaves no record, and its cost is unchanged. The same holds when the ownership lock is busy, and when the detach fails, in which case the record is rolled back.
- **Two root-lead prunes, cross-process:** two prunes race on the same owner's stale child. The owner's total includes that child exactly once. Put this in `agents-plugin-pi/test/ownership-contention.test.ts` or a sibling file.
- **Live owner:** the owner process holds the removed child in its registry and persists afterwards. Its total counts the child exactly once, both while the record exists and after the child leaves the registry.
- **Lost-update race:** the owner's footer `persist()` runs concurrently with retention writing a record. No cost is lost, cross-process.
- **Crash window:** a record exists but the home is still present. The child is counted once, and a later prune removes it without a second count.
- **Capacity eviction:** a capacity eviction writes a record. `evictedBaseline` does not change, and the hop's reported descendant usage is unchanged across eviction and restart.
- **Legacy checkpoint:** a checkpoint with a nonzero `evictedBaseline` from before this change still counts, plus any records.
- **Revival:** sidecar revival skips an agentId that has a record.
- **Returning agentId:** a fork rehydrate, or a live owner's send or relaunch, of an agentId whose home was removed and recorded is refused with the explicit error. No cost is lost or double counted.
- **Stale-record repair:**
  - a record left while the home is still present is removed when the next removal attempt ends with the home in place;
  - a failed rename-back that leaves the home off its path keeps the record;
  - the child's cost is counted exactly once before, during and after the repair;
  - a failed record write removes no home.
- **Freshness:**
  - a record written by another process is reflected in a live owner's total at its next `reconcile`;
  - repeated footer renders with no change do not rescan `evicted/`.
- **Containment:** a symlinked `evicted/` directory or record path is refused, as other owner artifacts are.
- **Fault seams:** the crash-window, rename-back and detach-failure cases are driven through `removeOwnedAgentHome`'s injectable `remove` or an equivalent test hook placed between the record write and the detach. They do not rely on timing.
- **Full suite:** `npm test` in `agents-plugin-pi/` passes.
