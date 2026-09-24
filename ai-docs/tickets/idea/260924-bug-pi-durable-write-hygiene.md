---
title: Pi adapter durable-file write hygiene (ownership rewrites, Windows rename retry, thread registry, liveness pid)
related:
  260923-research-pi-parent-child-loopback-control-channel: inventory source; ownership.json intentionally stays on the filesystem there
  260921-refactor-pi-shared-atomic-write-util: shared atomic-write helper this work may reuse
  260921-bug-pi-subagent-registry-checkpoint: owns the shutdown sidecar's non-atomic write; excluded here
  260913-bug-ws-pi-spawn-delegation-metadata-second-write-rejection: adjacent ownership write-path defect
---

# Pi adapter durable-file write hygiene (ownership rewrites, Windows rename retry, thread registry, liveness pid)

## Background

Found during the side-channel inventory of `260923-research-pi-parent-child-loopback-control-channel`. That research keeps `ownership.json` on the filesystem, so these write-mechanics defects need fixing on their own. None of them depends on a parent-child transport.

1. **Unconditional `ownership.json` rewrites.**
   - `refreshAgentTelemetry` (`agents-plugin-pi/src/spawner.ts`) computes whether telemetry changed, then calls `updateOwnership` regardless of the result. It runs on agent start and settle, message end, compaction, and other events.
   - `observeSessionWrite` (`agents-plugin-pi/src/agent-storage.ts`) runs every 5 s per record, live or dormant. It rewrites `ownership.json` on every sample, even when the session signature is unchanged; it bumps only `updatedAt`.
   - Each write takes the ownership lock directory. That adds lock contention (see `.done` tickets `260913-bug-ws-pi-ownership-observer-lock-contention-spams-tui` and `260913-bug-ws-pi-ownership-maintenance-errors-flood-tui`) and needless I/O.
2. **No Windows retry on ownership rename.** `writeOwnershipUnlocked` (`agents-plugin-pi/src/agent-storage.ts`) does a temp write followed by `renameSync` with no EPERM/EBUSY retry. `writePrivateJson` (`agents-plugin-pi/src/fork-context.ts`) already retries for the same Windows sharing-violation class.
3. **Non-atomic thread registry write.** `saveThreadRegistryFile` (`agents-plugin-pi/src/ask.ts`) writes `<sessionFile>.ws-threads.json` with a plain `writeFileSync`. A crash mid-write can leave a truncated registry. This file carries the fork resume capture. The shutdown sidecar has the same defect but is owned by `260921-bug-pi-subagent-registry-checkpoint`.
4. **Misleading `liveness.pid`.** `allocateAgentHome` records `process.pid`, the allocating parent's pid, before the child exists, and no non-test source reads the field beyond the shape check in `validOwnership` (`agents-plugin-pi/src/agent-storage.ts#L210`), which rejects a record whose `pid` is present but not a positive integer. It looks like a child pid but is not one.

Items owned elsewhere are out of scope here: the non-atomic approval decision and web-readiness writes, and the missing grandchild usage roll-up, are absorbed by the channel research.

## Decisions

- `ownership.json` is written only when its content changes.
  - The 5-s observer writes only when the session signature changes, and dormant records stay observed.
  - `refreshAgentTelemetry` writes only when telemetry changed.
  - A timestamp by itself is not a change.
- The ownership rename retries on Windows EPERM/EBUSY by reusing the retry that `writePrivateJson` (`agents-plugin-pi/src/fork-context.ts`) already has.
  - Only the rename-retry loop and its test-hook seam are extracted into a shared helper. Do not route these writes through `writePrivateJson` itself.
  - Each file keeps its current file mode, JSON formatting, and temp-file naming.
  - This work does not wait for `260921-refactor-pi-shared-atomic-write-util`. If that refactor lands first, use its helper instead.
- `saveThreadRegistryFile` writes atomically: a temp write followed by a rename, with the same Windows retry. Its file mode is unchanged.
- `liveness.pid` is removed from new records. Nothing reads it except the shape check in `validOwnership`, which already accepts an absent `pid`. `RpcClient` does not expose the real child pid, so it cannot be replaced. Existing records keep the field and remain valid.

## Prior Decisions

- 260924-feat-pi-agent-channel-subtree-state (2026-09-24, Decisions): "`subtree.json`, the `fs.watch` watcher, and the per-event synchronous read are retired. So is the subtree snapshot's own call into `writePrivateJson`. The shared helper keeps its Windows rename retry" — bearing: supports
- 260924-feat-pi-agent-channel-usage-rollup (2026-09-24, Decisions): "The parent stores the reported cumulative value in the child's ownership telemetry. The parent is already the single writer of that record." — bearing: constrains
- 260921-bug-pi-subtree-publication-lifecycle-isolation (2026-09-21, Decisions): "Keep the existing bounded Windows retry behavior. Async writer queues, writer leases, alternate transports, and broader registry recovery are outside this hotfix." — bearing: supports
- b318a127 (2026-09-21, commit): "Used deterministic hooks for retry tests instead of timing-dependent Windows process contention." — bearing: supports
- 260913-bug-ws-pi-ownership-observer-lock-contention-spams-tui (2026-09-13, commit a8a4abe6): "The observer retries through its existing lifecycle and five-second sampling points, so synchronous waiting or a new retry timer would block the Pi event loop without improving safety." — bearing: constrains
- 260913-bug-ws-pi-ownership-maintenance-errors-flood-tui (2026-09-13, commit ecaff2c7): "Keep observer failures best-effort and silent while preserving unknown-liveness downgrades; authoritative metadata writes, removal, retention, and capacity admission still reject or retain on uncertainty." — bearing: constrains
- 260908-feat-ws-pi-agent-session-disk-retention (2026-09-09, commit 02834f2c): "Session-file activity comes only from actual stat changes, not polling or directory timestamps." — bearing: supports
- ad825481 (2026-09-09, commit): "[fixed] I2 validates persisted liveness/protection facts conservatively." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/agent-storage.ts, agents-plugin-pi/src/ask.ts, agents-plugin-pi/src/fork-context.ts |
| scope.surface | cross-module | the retry lives module-private in fork-context.ts#L152-L154 and must reach agent-storage.ts and ask.ts; exported updateOwnership, observeSessionWrite, saveThreadRegistryFile keep their signatures |
| scope.new_public_symbol | yes | Decisions extract the rename-retry loop and hook seam from fork-context.ts#L177-L198 into a shared exported helper; writePrivateJson is not reused directly because its mode, compact JSON, and temp naming differ from writeOwnershipUnlocked at agent-storage.ts#L179-L185 |
| scope.new_type_contract | no | liveness.pid is already optional in OwnershipMetadata at agent-storage.ts#L46; new records omit it |
| scope.test_surface | existing | agents-plugin-pi/test/agent-storage.test.ts, test/ownership-contention.test.ts, test/ask.test.ts, test/fork-context.test.ts, test/agent-telemetry*.test.ts |
| complexity.reuse_points | confirmed | writePrivateJson retry loop and hooks seam at fork-context.ts#L177-L198 |
| complexity.side_effect_risk | moderate | ownership.json writes share the lock with retention and deletion gating, and the observer error path downgrades liveness to unknown at agent-storage.ts#L434-L439 |
| risk.correctness | moderate | the telemetry change snapshot at spawner.ts#L655-L660 compares more fields than the telemetry it writes, and a missed real change would leave durable telemetry or signature stale; updatedAt has no reader outside validOwnership |
| risk.fit | low | follows the existing writePrivateJson temp-then-rename-with-retry pattern; the overlapping refactor 260921-refactor-pi-shared-atomic-write-util is addressed by a Decision |
| risk.test | moderate | needs injected EPERM/EBUSY on a non-win32 host, a no-lock assertion for unchanged samples, and a mid-write crash simulation; only writePrivateJson has an injection seam today |
| risk.security_or_contract | moderate | durable ownership.json schema read by retention across processes must keep accepting legacy pid at agent-storage.ts#L210; an atomic registry write via writePrivateJson would also change the ws-threads.json file mode to 0600 |

## Phases

### Phase 1: Durable-write hygiene

Verification:

- Repeated observer samples with an unchanged session signature, and telemetry refreshes with unchanged telemetry, perform no `ownership.json` write and take no lock.
- An injected EPERM/EBUSY on the ownership rename is retried and then succeeds. After the retry budget is exhausted, it fails as today.
- A simulated crash during the thread registry write never leaves a truncated registry.
- Newly allocated records carry no `liveness.pid`, and records that still have the field load unchanged.
