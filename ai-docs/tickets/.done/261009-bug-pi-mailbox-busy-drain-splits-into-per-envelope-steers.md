---
title: Pi mailbox waiter splits one busy-lead drain into per-envelope steers
related:
  260914-feat-ws-pi-mailbox-native-steer-push: introduced the waiter's per-envelope admit
---

# Pi mailbox waiter splits one busy-lead drain into per-envelope steers

## Background

`startMailboxWaiter` (`agents-plugin-pi/src/mailbox-waiter.ts`) drains every
queued envelope with one `mailbox.recv`. It then calls `admit` once per
envelope. `index.ts` wires `admit` to
`sendToLead(pi, ..., "steer", "always")`.

When the lead is streaming and `heldPushQueue` is empty, `admitPush`
(`agents-plugin-pi/src/spawner.ts`) takes the `batchMode: "always"` branch.
That branch pushes the one item and calls `submitHeldPushBatch(pi, "steer")`
right away, which empties the queue. The next envelope finds the queue empty
again, so N drained envelopes become N separate one-item `ws-push-batch`
steers.

Pi's `PendingMessageQueue` defaults to `steeringMode: "one-at-a-time"`
(`settings-manager.js`: `getSteeringMode() || "one-at-a-time"`). It dequeues
one steer per model-call boundary, so the lead sees one mail per turn instead
of the whole backlog at once. A user-reported symptom matched this on
2026-10-09.

A scratch probe reproduced it: a busy lead plus a fake waiter draining 3
envelopes produced 3 `sendMessage` calls, each with `details.items.length === 1`
and `deliverAs: "steer"`. The idle path is unaffected: envelopes are held and
released as one batch at the confirmed start.

`spawner.ts`'s header already states the invariant this breaks: a batch
exists "so Pi steering mode cannot stretch one boundary snapshot across
multiple assistant turns".

## Candidate direction (unconfirmed)

- Admit one drain as one unit: either an `admit(envelopes[])` seam or a
  `sendToLead` variant that queues every item before a single
  `submitHeldPushBatch`. The result is one steer carrying N items.
- Workaround meanwhile: `"steeringMode": "all"` in `~/.pi/agent/settings.json`.
