---
title: Move Pi subtree lifecycle state onto the parent-child control channel
blocked-by: 260924-feat-pi-agent-channel-transport
related:
  260923-research-pi-parent-child-loopback-control-channel: source research; its Outcome Ledger is this ticket's authority
  260924-feat-pi-agent-channel-transport: prerequisite transport
  260921-bug-pi-subtree-publication-lifecycle-isolation: prior Windows rename-contention fix on the snapshot path this retires
  260920-bug-pi-nested-subagent-terminal-delivery-stall: cause unknown; this ticket is not claimed to fix it
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1702acbc26b21803
sage-review-completeness-reviewed: 1702acbc26b21803
---

# Move Pi subtree lifecycle state onto the parent-child control channel

## Background

Each child publishes `<childHome>/subtree.json` through an atomic temp-and-rename write. The write retries on Windows EPERM/EBUSY with a synchronous `Atomics.wait` sleep (`agents-plugin-pi/src/subtree-lifecycle.ts`, `src/fork-context.ts`). The parent reads the snapshot through `fs.watch` on the child home, and also reads it synchronously on every non-delta RPC event (`agents-plugin-pi/src/spawner.ts`). The snapshot carries these fields:

- counts: `outstanding`, `active`, `deliveries`, `delegated`;
- a `revision`;
- bounded `descendants[]` identity, re-published hop by hop.

`beginSubtreeDispatch` enforces a busy-before-dispatch fence: a grandchild cannot start until the busy edge is published. A missing or mismatched snapshot means "waiting", so the check fails closed.

## Decisions

- The whole snapshot moves to the channel: counts, the busy fence, and `descendants[]` together. Splitting identity from counts would carry one structure over two transports.
- The child sends full revisioned snapshots, and the parent keeps the last revision it has seen.
  - Revisions are scoped to one launch generation: the record's `launchGeneration`, which `260924-feat-pi-agent-channel-transport` carries in the hello and in every message.
  - Within a generation, the child's revision counter survives reconnects, and so does the parent's last-seen value. A lower revision is ignored.
  - When the parent accepts a hello with a new generation, it resets its last-seen revision, so a relaunched child's counter can restart low.
  - After a reconnect, the child sends its latest snapshot in the resume section of the hello.
  - A snapshot equal to the last one sent is not sent again. Streamed deltas and duplicate events produce no sends.
- The busy-before-dispatch fence waits for the parent's acknowledgment of the busy revision before a grandchild is dispatched. The wait is bounded. If no acknowledgment arrives in time, or the channel is down, `beginSubtreeDispatch` refuses the dispatch. That is today's fail-closed outcome for a failed busy publication. The worker chooses the bound and records it in the Result.
- When a direct child's live state is cleared, its cached descendants are removed and republished, as today.
- A disconnected or not-yet-connected channel maps to "waiting", preserving today's fail-closed semantics.
- The existing bounds on `descendants[]` (count and depth) and the durable `waitingOnChildren` mirror into ownership and sidecar records for restart recovery are preserved.
- `subtree.json`, the `fs.watch` watcher, and the per-event synchronous read are retired. So is the subtree snapshot's own call into `writePrivateJson`. The shared helper keeps its Windows rename retry, because the fork context envelope still uses it.
- `SubtreeChannel` (`{path, nonce}`) is retired along with `subtree.json`.
  - `260924-feat-pi-agent-channel-transport` moves web readiness into the hello and removes its dependency on this nonce, including `WEB_NONCE_ENV` and `verifyWebReadiness`.
  - Here, the legacy-Explore check in `spawner.ts` drops its `!record.subtreeChannel` clause and decides legacy status from the delegation's network-authority fields alone. Otherwise every new Explore record would be rejected as legacy.
  - A persisted `subtreeChannel` field in an older `PersistedForkResume` record is ignored on read, with no migration. The resumed child gets a fresh channel at relaunch.
- Measure the latency of the acknowledgment round trip before grandchild dispatch and record it in the Result. If keeping dispatch acceptable would require relaxing the fence, the worker stops and escalates.

## Prior Decisions

- dc857cf3 (2026-09-24, commit): "Subtree state moves as a whole (splitting identity from counts would put one struct on two transports); usage is per-hop cumulative last-wins because deltas break replay safety" — bearing: supports
- 260923-research-pi-parent-child-loopback-control-channel (2026-09-23, Confirmed Decisions): "Subtree state over the channel. The child sends full revisioned snapshots, and the last revision wins. The busy-before-dispatch fence waits for the parent's acknowledgment of the busy revision" — bearing: supports
- 226b4a56 (2026-09-21, commit): "Only beginSubtreeDispatch's initial busy edge remains fail-closed; later publication failures are diagnosed without aborting RPC event application, settlement, watcher cleanup, or spawn-failure reporting." — bearing: constrains
- 260921-bug-pi-subtree-publication-lifecycle-isolation (2026-09-21, Decisions): "Skip a `subtree.json` replacement when the effective snapshot is unchanged. Token streaming, duplicate filesystem notifications, and render refreshes must not create equivalent writes." — bearing: constrains
- 08ac495b (2026-09-21, commit): "The streamed-delta path now skips subtree observation, telemetry refresh, and subtree publication" — bearing: constrains
- 2f4771f3 (2026-09-20, commit): "opportunistic channel reads on direct-child RPC events could leave nested identities stale while the parent was idle; directory watches preserve push semantics without making advisory identity affect wait accounting." — bearing: constrains
- 2f4771f3 (2026-09-20, commit): "Clearing a direct child's live state now also removes and republishes its cached descendants so finished nested agents cannot remain visibly live." — bearing: constrains
- 260913-bug-ws-pi-settled-agent-falsely-remains-running (2026-09-13, Decisions): "Preserve lifecycle-only fences around settlement: work generation, descendant waiting and subtree revision, direct-parent routing, terminal queue admission and retry" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/subtree-lifecycle.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/fork-context.ts, plus SubtreeChannel consumers agents-plugin-pi/src/ask.ts#L386, agents-plugin-pi/src/delegation-policy.ts#L10 |
| scope.surface | cross-module | subtree-lifecycle.ts exports SubtreeChannel, readSubtreeSnapshot, beginSubtreeDispatch consumed by spawner.ts#L113; subtreeChannel persisted in PersistedForkResume ask.ts#L386 and its nonce feeds WEB_NONCE_ENV spawner.ts#L2205 |
| scope.new_public_symbol | unknown | ticket names no symbol and the transport message API it builds on is unlanded, pending 260924-feat-pi-agent-channel-transport |
| scope.new_type_contract | yes | new subtree snapshot and busy-revision acknowledgment channel messages replace the SubtreeChannel path-and-nonce contract subtree-lifecycle.ts#L7 |
| scope.test_surface | existing | agents-plugin-pi/test/recursive-worker.test.ts, test/spawner.test.ts, test/persistent-explore.test.ts, test/fork-lifecycle.integration.test.ts build subtree.json fixtures |
| complexity.reuse_points | unconfirmed | channel transport from 260924-feat-pi-agent-channel-transport is not in the tree, ls agents-plugin-pi/src found no channel module |
| complexity.side_effect_risk | high | retires the watcher and per-event read on the settlement path spawner.ts#L2397-L2430 and #L2495, and the SubtreeChannel nonce also drives web readiness and the legacy-Explore check spawner.ts#L3130-L3134 |
| risk.correctness | high | the synchronous fail-closed busy edge becomes an asynchronous acknowledgment round trip, and waitingOnChildren gates parking and settlement spawner.ts#L1455-L1563 |
| risk.fit | moderate | must fit an unlanded transport API and share the channel with the approval and usage migrations |
| risk.test | high | verification needs delayed or dropped acknowledgments, disconnect and reconnect, out-of-order snapshots, and a latency measurement |
| risk.security_or_contract | moderate | changes the parent-child protocol and the persisted subtreeChannel shape in fork resume and sidecar records ask.ts#L733-L776 |

## Phases

### Phase 1: Channel-delivered subtree snapshots with an acknowledged busy fence

Replace the snapshot file, the watcher, and the per-event read with channel snapshots. Add the busy-revision acknowledgment and its bounded fence, and report the latest snapshot in the hello's resume section. Retire `SubtreeChannel` and adjust the legacy-Explore check and the fork-resume read.

Verification:

- A grandchild is never dispatched before the parent acknowledges the busy revision. Test this by delaying the acknowledgment. When the acknowledgment is dropped, or the channel is down, the bounded wait refuses the dispatch.
- A disconnect during outstanding work leaves the parent in waiting-on-children. Settlement proceeds only after a reconnected, quiescent snapshot or process exit.
- A relaunched child whose revision counter restarts is accepted under its new generation.
- Within one generation, out-of-order or duplicate snapshots cannot regress the parent's view.
- Nested identity propagates to the root within the existing bounds. Clearing a direct child removes its cached descendants.
- No `subtree.json` is written, no watcher is installed, and unchanged snapshots and streamed deltas cause no sends.
- A new Explore record is not rejected as legacy. An older `PersistedForkResume` record that still has `subtreeChannel` loads and relaunches.
- After a restart, the durable `waitingOnChildren` mirror in the ownership and sidecar records still restores waiting-on-children.
- The acknowledgment latency before grandchild dispatch is recorded.
