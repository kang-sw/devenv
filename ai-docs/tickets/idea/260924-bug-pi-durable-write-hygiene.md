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
4. **Misleading `liveness.pid`.** `allocateAgentHome` records `process.pid`, the allocating parent's pid, before the child exists, and no non-test source reads the field. It looks like a child pid but is not one.

Items owned elsewhere are out of scope here: the non-atomic approval decision and web-readiness writes, and the missing grandchild usage roll-up, are absorbed by the channel research.

## Open Questions

- Should `liveness.pid` be removed, renamed to state its meaning (for example `ownerPid`), or replaced by the real child pid? `RpcClient` does not expose its child process publicly.
- Should the 5-s observer write only when the session signature changes, or should dormant records stop being observed?
