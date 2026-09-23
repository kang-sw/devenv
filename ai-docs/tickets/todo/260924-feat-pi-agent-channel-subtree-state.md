---
title: Move Pi subtree lifecycle state onto the parent-child control channel
blocked-by: 260924-feat-pi-agent-channel-transport
related:
  260923-research-pi-parent-child-loopback-control-channel: source research; its Outcome Ledger is this ticket's authority
  260924-feat-pi-agent-channel-transport: prerequisite transport
  260921-bug-pi-subtree-publication-lifecycle-isolation: prior Windows rename-contention fix on the snapshot path this retires
  260920-bug-pi-nested-subagent-terminal-delivery-stall: cause unknown; this ticket is not claimed to fix it
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
- The busy-before-dispatch fence waits for the parent's acknowledgment of the busy revision before a grandchild is dispatched.
- A disconnected or not-yet-connected channel maps to "waiting", preserving today's fail-closed semantics.
- The existing bounds on `descendants[]` (count and depth) and the durable `waitingOnChildren` mirror into ownership and sidecar records for restart recovery are preserved.
- `subtree.json`, the `fs.watch` watcher, the per-event synchronous read, and the snapshot's Windows rename-retry path are retired.
- Measure the latency of the acknowledgment round trip before grandchild dispatch and record it in the Result. If keeping dispatch acceptable would require relaxing the fence, the worker stops and escalates.

## Phases

### Phase 1: Channel-delivered subtree snapshots with an acknowledged busy fence

Verification:

- A grandchild is never dispatched before the parent acknowledges the busy revision. Test this by delaying or dropping the acknowledgment.
- A disconnect during outstanding work leaves the parent in waiting-on-children. Settlement proceeds only after a reconnected, quiescent snapshot or process exit.
- Nested identity propagates to the root within the existing bounds.
- No `subtree.json` is written and no watcher is installed.
- Out-of-order or duplicate snapshots cannot regress the parent's view, because last revision wins.
- The acknowledgment latency before grandchild dispatch is recorded.
