---
title: "Pi adapter: keep a durable same-session subagent registry checkpoint"
related:
  260905-feat-ws-pi-push-only-child-reports: predecessor — introduced shutdown sidecar recovery for persistent Pi children
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f109b1ef0115c8d8
sage-review-completeness-reviewed: f109b1ef0115c8d8
---

# Pi adapter: keep a durable same-session subagent registry checkpoint

## Background

The Pi adapter keeps its live subagent registry in memory. Its current recovery sidecar is written during shutdown and consumed and deleted during the next startup. If the lead process exits without completing a later shutdown snapshot, durable child homes and transcripts can remain while the registry index is lost. Knowing the former agent ID or alias is then insufficient because `ws-agent-list` and `ws-agent-send` consult only the reconstructed in-memory registry and do not discover child homes.

The recovery contract is intentionally limited to resuming the same parent Pi session. A new Pi session must not discover or adopt another session's children.

Observed trigger (2026-10-05): repeated lead-process deaths right after compaction left `ws-agent-list` empty on every restart, because no shutdown snapshot was written while the previous startup had already deleted the old sidecar. The exit path of those deaths is not yet identified: host Pi 1.0.0 records an uncaught exception or fatal error in `~/.pi/agent/crashes.json` before `process.exit(1)` (`uncaughtCrash` in the host's interactive mode), and no such record existed, so a signal or native termination is also possible. Either way `session_shutdown` does not run. Reload, new, resume, and fork await `session_shutdown` before `session_start` and are not affected.

## Decisions

- Replace the shutdown-only, one-shot sidecar behavior with a persistent registry checkpoint owned by the parent Pi session identity.
- Keep the checkpoint after startup hydration. Reading it must not create a new loss window by deleting it.
- Persist semantically significant registry transactions, including membership, identity and alias changes, resumability metadata, and lifecycle transitions that affect whether a child can be resumed. High-frequency token, telemetry, subtree-watcher, and gutter-render events are not persistence boundaries.
- Restore checkpoint entries conservatively as dormant or interrupted. `ws-agent-send <alias-or-id>` is the explicit action that resumes a restored child.
- Do not automatically restart a child, resend a prompt, or replay pending questions, approvals, deliveries, or other in-flight work.
- Do not scan or adopt child homes belonging to a different Pi session. Separate sessions remain separate registry owners.
- Use a bounded full-registry snapshot rather than an append-only event log. Serialize writes per owner and atomically replace the checkpoint so an older completion cannot overwrite a newer revision.
- Prevent concurrent processes from acting as independent writers for the same parent-session checkpoint or resuming the same child generation. Recovery must not create a second writer for an existing subtree channel.
- Migrate an existing one-shot sidecar only after the persistent checkpoint has been committed successfully.
- Locator: a new sibling file `<sessionFile>.ws-registry.json` (the no-session case uses an owner bucket under the session's agent storage), embedding `parentSessionId`, which is validated on read. A new name keeps an older adapter, which deletes the legacy sidecar on version mismatch, from destroying the checkpoint. Rejected: reusing `<sessionFile>.ws-agents.json`.
- Durability points: `spawnAgent` commits after registration and before the child starts, and again after launch, before returning; a commit failure at spawn rolls back the registration and fails the spawn. Dormant resume through `ws-agent-send` and an explicit `ws-agent-stop` commit before returning. Settle, exit, thread-binding changes, writer-attribution stamps, and prune removals are serialized and coalesced but not awaited by their callers.
- Corrupt or unsupported checkpoint: a malformed or unreadable file is renamed aside to `<path>.corrupt-<timestamp>`, the owner is notified, and the session continues with a fresh checkpoint. A file with a newer unknown schema version puts the session in read-only mode (no checkpoint writes, notify) so a newer adapter's file is never clobbered.
- Writer fence: a session-lifetime lease next to the checkpoint, acquired with mkdir before hydration and holding `{pid, nonce, acquiredAt}`; it is reclaimed as stale only when its pid no longer exists, reusing the existing child-home ownership-lock logic, and released after the final shutdown commit. When a live foreign holder owns the lease, hydrate read-only: list entries, but refuse dormant resume and checkpoint writes, and notify. Pid reuse that makes a dead holder look alive is accepted for this ticket; the notice names the lease path for manual removal. Rejected: a heartbeat lease, deferred until the pid-reuse case is observed in practice.
- Restored records keep the existing `dormant` status in `ws-agent-list`; no new `interrupted` status is added. The interrupted-versus-idle signal rides the existing `ws-agent-orphaned` push.
- Add a last-chance synchronous checkpoint write on `process.on("exit")`, since the host crash path exits through `process.exit`. It complements, and does not replace, the transaction boundaries above, because SIGKILL and out-of-memory termination skip it.
- During shutdown's `stopAll`, per-stop checkpoint writes are suppressed; the final shutdown commit carries the pre-stop `running` evidence, matching the current shutdown snapshot.
- Thread-bound records remain owned by `<sessionFile>.ws-threads.json` and are excluded from the checkpoint, but binding and unbinding are checkpoint boundaries because they move a record into or out of its scope.
- Persist `cwdOverride` with the other resume metadata, so a restored child resumes in the cwd it was spawned with, as the `ws-agent-spawn` schema promises. The current sidecar omits it.
- Restore `waitingOnChildren` as persisted, keeping the 260912-feat-ws-pi-recursive-worker-subtree-lifecycle decision and the current shutdown-sidecar behavior; the checkpoint only adds mid-run writes of the same value. A transition of a record's `waitingOnChildren` value is a serialized, coalesced checkpoint boundary like settle, so the persisted value tracks the live one; subtree publications that leave the value unchanged are not boundaries. Rejected: never restoring `true`, which reverses 260912 and leaves eviction protection to ownership liveness alone.

## Constraints

- Preserve the identity, alias, model and effort, tool/delegation configuration, session and prompt paths, fork context, and other resume metadata currently required to reconstitute a registry record.
- A registry mutation must not be reported as durably successful before its checkpoint transaction completes. Spawn and deletion paths may use explicit intermediate states so a crash cannot silently produce an unindexed process or resurrect a deleted record.
- A stored `running` value is historical evidence, not proof that a process survived restart.
- Checkpoint corruption or schema incompatibility must remain diagnosable; startup must not silently replace an unreadable checkpoint with an empty registry.
- Windows transient replacement contention must use bounded non-blocking retry rather than blocking the Pi event loop. The existing `renameWithWindowsRetry` sleeps with a synchronous `Atomics.wait`, so the checkpoint path needs an asynchronous equivalent; the synchronous exit-time safety-net write is the one exception.
- Existing retention and capacity policies may still remove eligible dormant records and owned homes, but their registry removals must be reflected durably.
- "Durably successful" above binds the awaited durability points in `## Decisions`; coalesced boundaries become durable later, and revision ordering keeps an older completion from winning.
- Overlapping bootstrap generations inside one process (rapid reload: `BridgeGeneration` owners `active`/`shutdown`/`stale`, `sessionStartEpoch`, `disposeStaleBootstrap`) are not concurrent writers. The writer lease and commit serialization are process-scoped and follow the published generation, so a newer generation does not fence itself read-only, and a superseded generation never commits a registry view that replaces the published generation's checkpoint. `disposeStaleBootstrap` persists before `stopAll` only because hydration used to consume the sidecar; revisit that persist under the non-consuming checkpoint.
- The exit-time safety net writes only for the published generation, is disarmed once the final shutdown commit and lease release complete (so a normal quit's post-`stopAll` registry never replaces the pre-stop evidence), and writes nothing in read-only mode.
- Read-only mode (live foreign lease or newer schema) refuses every operation that needs a checkpoint commit, including `ws-agent-spawn` and `ws-agent-stop`, not only dormant resume; listing stays available.
- A failed spawn commit rolls back the in-memory changes the spawn made, including an alias the spawn guards moved off its previous holder. A capacity eviction already performed is not undone; the previous checkpoint's entry for the evicted record is dropped on read because its owned home is gone.
- With mixed adapter versions on one session, a legacy sidecar written after the checkpoint never overrides newer checkpoint state; define and test the precedence rule. Whether the legacy file is deleted or ignored after migration is an implementation choice under that rule.
- Resuming a restored child first confirms the previous child process for that home is gone, using the durable ownership liveness (`pid`, `instanceNonce`), because a child whose lead died keeps running its own shutdown after its stdin ends.
- The no-session checkpoint uses a filename distinct from `registry.ws-agents.json` in its owner bucket, for the same reason as the file-backed locator.
- The snapshot stays bounded by the existing registry capacity and retention policies; no separate size limit is introduced.

## Prior Decisions

- 260912-feat-ws-pi-recursive-worker-subtree-lifecycle (2026-09-12, Decisions): "Persist policy and waiting state. Depth, capability ceiling, child profile, descendant obligations, and waiting-on-children state survive automatic parking, sidecar recovery, and same-lead restart." — bearing: constrains
- 260905-feat-ws-pi-push-only-child-reports (2026-09-05, commit 654f2fe4): "Sidecar ordering is load-bearing: the snapshot is taken in session_shutdown BEFORE stopAll(), since after it every record is dormant and captureOrphans would find nothing." — bearing: constrains
- 260927-bug-pi-mailbox-waiter-bypasses-local-devenv-runtime (2026-09-27, Result a20f5c70): "...stale cleanup then stopped the recovered children and cleared the globals, losing dormant records that startup had already read and deleted from the sidecar." — bearing: constrains
- 260924-bug-pi-durable-write-hygiene (2026-09-24, Background): "The shutdown sidecar has the same defect but is owned by `260921-bug-pi-subagent-registry-checkpoint`." — bearing: supports
- 260923-research-pi-parent-child-loopback-control-channel (2026-09-24, commit dc857cf3): "...the sidecar non-atomic write is left to 260921-bug-pi-subagent-registry-checkpoint." — bearing: supports
- 260924-bug-pi-retention-cross-owner-checkpoint-fold (2026-09-24, Decisions): "`reviveOrphans` also discards a stale sidecar duplicate whose agentId is still registered from a different home, and that discard writes none." — bearing: constrains
- 260908-feat-ws-pi-agent-session-disk-retention (2026-09-09, commit 02834f2c): "Kept legacy file-backed sidecars unchanged while adding the supported same-identity no-session locator." — bearing: constrains
- 260907-bug-ws-pi-fork-first-call-prompt-cache-miss (2026-09-07, Decisions): "Persistent fork metadata. Retain the original full rendered block, effective prompt inputs, parent ws key and cache-affinity id through dormant resume, task sidecar serialization/parsing/rehydration" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-sidecar.ts, src/index.ts (session_start/session_shutdown, disposeStaleBootstrap), src/spawner.ts (spawnAgent, sendToAgent, stopAgent, evictForCapacity; locate by symbol, line anchors drift), src/atomic-write.ts |
| scope.surface | cross-module | registry mutations in spawner.ts, lifecycle wiring in index.ts, locator and lease in agent-sidecar.ts and agent-storage.ts, thread-bound boundary with ask.ts |
| scope.new_public_symbol | yes | new checkpoint locator, read, commit and lease helpers beside sidecarPath and writeSidecarAt; the ticket names no symbols |
| scope.new_type_contract | yes | versioned revisioned checkpoint file embedding parentSessionId, plus cwdOverride added to the persisted record shape (PersistedOrphan has none) |
| scope.test_surface | existing | agents-plugin-pi/test/agent-sidecar.test.ts (waitingOnChildren revival pinned at #L789-L805), test/fork-lifecycle.integration.test.ts; new test files likely for lease, crash and exit paths |
| complexity.reuse_points | confirmed | captureOrphans, parseOrphans, rehydrateOrphanRecord, reviveOrphans (agent-sidecar.ts), acquireOwnershipLock pid-liveness (agent-storage.ts#L352-L392), renameWithWindowsRetry (atomic-write.ts, sync sleep needs an async variant) |
| complexity.side_effect_risk | high | commits inside spawnAgent, sendToAgent, stopAgent and capacity eviction, plus a process exit hook and a session-lifetime lease, all touch the live registry lifecycle |
| risk.correctness | high | revision ordering, spawn rollback, lease reclaim and shutdown-versus-stale-epoch ordering are race-prone and a wrong snapshot can resurrect or lose children |
| risk.fit | moderate | extends the existing sidecar module and ownership-lock logic, but reverses the one-shot read-and-delete sidecar contract |
| risk.test | high | crash, process exit, concurrent writer and Windows rename paths need fault injection that existing sidecar tests do not cover |
| risk.security_or_contract | moderate | new on-disk file contract with mixed old and new adapters on one session file, lease file permissions and pid-reuse acceptance recorded in Decisions |

## Phases

### Phase 1: Durable same-session registry recovery

Introduce a versioned, revisioned checkpoint at a stable locator derived from the parent Pi session. Route recovery-significant registry mutations through one serialized asynchronous atomic-write path, and make startup hydrate the same-session registry without consuming the checkpoint. Retain compatibility with the current shutdown sidecar long enough to migrate an existing recoverable session safely.

Verify at least:

- A child spawned and checkpointed before an abrupt lead-process exit is listed after the exact parent Pi session is resumed.
- Both its raw ID and alias can address `ws-agent-send`, which resumes it from a dormant or interrupted state.
- Starting a different Pi session does not list or adopt the previous session's children.
- Startup hydration does not delete the authoritative checkpoint, and repeated same-session restarts preserve the registry.
- Spawn failure, alias reassignment, stop/park, resume, eviction, and deletion cannot leave an older snapshot authoritative.
- Token streaming, subtree publication, watcher duplication, and gutter repaint do not trigger checkpoint writes.
- A second writer for the same parent session is fenced before it can resume or publish for the same child generation.
- Corrupt or unsupported checkpoints are preserved and diagnosed rather than converted to an empty registry.
- Existing one-shot sidecars migrate only after the new checkpoint is durable.
- A malformed checkpoint is renamed aside and diagnosed; a newer-version checkpoint leaves the session read-only and unmodified.
- A lease held by a live foreign process yields a read-only registry that refuses dormant resume and checkpoint writes.
- The `process.on("exit")` safety net writes the current registry when the process exits without `session_shutdown`.
- Shutdown's `stopAll` does not overwrite the pre-stop `running` evidence.
- A restored child spawned with a cwd override resumes in that cwd.
- A rapid same-process reload keeps one writer: the published generation hydrates writable, and the superseded generation's late commit cannot replace its checkpoint.
- A normal quit leaves the final shutdown commit's pre-stop evidence intact after the exit hook fires.
- Read-only mode refuses `ws-agent-spawn` and `ws-agent-stop` as well as dormant resume.
- Resume of a restored child waits for, or refuses while, the previous child process for that home is still alive.
- The lease is released after the final shutdown commit, and the next same-session start acquires it without a stale-reclaim notice.
- The no-session locator round-trips through its owner bucket without colliding with the legacy no-session sidecar.
- A transient Windows replacement failure on the checkpoint path retries asynchronously with a bound and does not block the event loop.
- A `waitingOnChildren` transition is checkpointed (coalesced), a restored record keeps the persisted value, and a subtree publication that leaves it unchanged writes nothing.
