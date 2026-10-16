---
title: "Pi lead: pushes steered into the ws-compact lever turn are lost by compaction"
related:
  261002-feat-pi-lead-ws-owned-compaction: introduced the no-kept-tail summary and the custom_message-free dialog that make the drained steer invisible after compaction
  260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction: introduced the compaction push hold (`leadCompactingRef`) whose release path the recovered items reuse
  261009-bug-pi-mailbox-busy-drain-splits-into-per-envelope-steers: shaped the mailbox drain as one `ws-push-batch` steer, the most common recovered shape
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 75fd6d125a618f73
sage-review-completeness-reviewed: 75fd6d125a618f73
completed: 2026-10-15
---

# Pi lead: pushes steered into the ws-compact lever turn are lost by compaction

## Background

Observed while dogfooding the Pi lead: right after a `ws-compact` lever call,
a pushed message (child report or mailbox drain) appears between the lever
turn's `turn_end` and the compaction, then never reaches the post-compaction
context.

Mechanism, traced through the adapter and the pinned Pi host
(`@earendil-works/pi-coding-agent` 0.84.4):

1. While the lead streams the assistant turn that calls `ws-compact` (its
   prose arguments can take a long time to generate), the compaction hold is
   not yet set, so `admitPush` in `agents-plugin-pi/src/spawner.ts` sends an
   arriving push straight into Pi's steering queue (direct `sendPush`, a raw
   send, or `submitHeldPushBatch(..., "steer")`).
2. The lever's `execute` sets `leadCompactingRef` and calls `ctx.compact()`.
   Pi's `AgentSession.compact()` starts with `await this.abort()`, which
   aborts the run but does not clear the steering queue.
3. After the tool batch, Pi's agent loop emits `turn_end` and drains the
   steering queue (`getSteeringMessages()`; under Pi's default
   `one-at-a-time` steering mode this takes one message), injecting the
   steer into the run and recording it in the session. The next assistant turn is already
   aborted, so the run ends.
4. The ws-authored compaction keeps no raw tail (`NO_KEPT_ENTRY_ID`), and its
   `## Dialog` section takes only user/assistant `message` entries, never
   `custom_message` entries such as `ws-push-batch`
   (`agents-plugin-pi/src/lead-compaction.ts`). The drained push therefore
   stays in the session file but is absent from the model context after
   compaction.

Pushes that arrive after the hold is set are held in `heldPushQueue` and
released after compaction, so only the steers admitted before the lever ran
are lost. Delaying the post-compaction start does not help: the steer is
already in Pi's queue before compaction begins and is drained by Pi's loop.

## Decisions

- **Recover only on lever-initiated compactions.** Recovery applies to a
  compaction operation started by `ws-compact`, scanning entries after the
  latest `ws-compact` toolResult. Rejected: all compactions -
  threshold/overflow compaction picks up queued steers after its
  preparation, and a user `/compact` or another path has no lever toolResult
  to bound the scan.
- **Stage, then commit on success.** Candidates are collected in
  `session_before_compact` and enqueued into `heldPushQueue` only when that
  lever compaction lands (the `session_compact` path, before its deferred
  release); a failed or cancelled compaction discards them. Rejected:
  enqueueing at `session_before_compact` - on failure the steer is still in
  context, so it would be delivered twice.
- **Match push-path entries only.** Candidates are `custom_message` entries
  after the bound whose `customType` is one the push path emits
  (`ws-push-batch` and the push families/raw types admitted through
  `admitPush`), excluding the adapter's compaction preparation and milestone
  messages. Mailbox drains are included: the waiter admits each drain through
  `sendBatchToLead`, so it arrives as a `ws-push-batch` of `ws-mailbox`
  items. Rejected: every custom entry - preparation and milestone
  messages are compaction scaffolding, not lost deliveries.

- **Re-deliver the lost pushes after compaction.** Lost push items are put
  back into `heldPushQueue` so the existing post-compaction release (push
  wake, then any owed resume) delivers them. Rejected: carrying them as a
  summary section instead, which would leave them outside push delivery and
  its actionable/superseded handling.
- **Insert at the front of `heldPushQueue`, in original order.** `admitPush`
  sends a push directly as a steer only when `heldPushQueue` is empty, so
  every item held at compaction time was admitted after the lost steer;
  prepending preserves admission order. No flush can interleave, because
  `flushHeldPushes` returns early while `leadCompactingRef` is set.
- **Replay the delivered content with its original item state.** A recovered
  item keeps the content and `state` (`actionable`, `informational`,
  `superseded`) it had when it was steered; actionable approvals and
  questions must stay actionable. Rejected: re-materializing each item from
  the live registry as a held push, which needs an in-memory ledger of
  steered `HeldInput`s and buys nothing: between the lost steer's admission and its
  re-delivery the lead runs nothing (aborted, then compacting), and a steer is
  already, by contract, a send-time snapshot that can go stale while it waits
  in Pi's queue during an ordinary busy turn.
  260911-feat-ws-pi-held-push-batch-delivery resolves actionability at
  snapshot time for held items; a recovered item was already snapshotted when
  it was steered, so it keeps that snapshot rather than being re-resolved.
- **Single-message steers take their state from their family.** A recovered
  non-batch steer of family `ws-agent-approval` or `ws-agent-question` is
  replayed as `actionable`; every other non-batch steer is `informational`.
  A direct `sendPush` happens only for a current request, so it was
  actionable by construction, and its entry stores no `state` field. Batch
  items take `state` from `details.items`.
- **Commit only under the ws-authored summary.** Staged candidates are
  committed only when the landed compaction entry is this adapter's
  (`isLeadCompactionDetails(event.compactionEntry.details)`). Rejected:
  committing on any landed compaction - when the ws result fails to build or
  another extension's result wins, Pi's native summary keeps a raw tail that
  likely still holds the drained steer, so recovery would deliver it twice.
- **Unpack recovered batches.** A recovered `ws-push-batch` contributes its
  `details.items` individually, so the post-compaction batch never nests a
  batch.
- **Leave terminal delivery bookkeeping untouched.** The lost steer already
  moved its `TerminalDelivery` to `enqueued` and ran `afterEnqueue`;
  re-delivery continues that delivery and must not reset it to `held` or run
  `afterEnqueue` again.

## Constraints

- User-typed steers drained into the aborted lever run are out of scope. They
  are user-role messages that remain visible in `## Dialog` history; they are
  not re-delivered by this ticket.
- Raw held items currently materialize as `"informational"`
  (`materializeHeldInput`); a recovered item needs a way to carry its
  original `state` through materialization.
- A batch carries the adapter label once and its items carry unlabeled
  content; a recovered single-message steer whose stored `content` is
  labeled must not end up labeled twice inside the new batch. Reuse
  `adapterLabeledBody` (`adapter-label.ts`) to strip it.
- Accepted ordering limit: Pi drains one steer per poll, so when two
  separate steers (each an approval, a question, or one mailbox drain) land
  during the lever turn, only the first is drained and lost; the second stays
  in Pi's queue, survives compaction, and is delivered at the next run's
  start, before the recovered first one. Nothing is lost, and the two are
  normally independent events (an approval blocks its command until decided,
  so one child rarely steers twice within one lever turn). Rejected: making `admitPush`
  hold while Pi already has a pending steer, which changes ordinary
  busy-turn steer delivery outside this ticket.
- Worker/explore/fork sessions keep Pi's native compaction and are untouched.

## Prior Decisions

- 260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction (2026-09-06, commit b5c5599a): "Pi emits session_compact while its compaction flag is still set, so a prompt from inside that handler fails silently ... Fixed with a setImmediate-deferred release" — bearing: constrains
- 260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction (2026-09-06, Result): "`leadCompactingRef` ... is set by the lever before `ctx.compact` and by `session_before_compact` for every compaction, and `isOwningAgentIdle()` returns false while it is set." — bearing: supports
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, commit f065de0d): "The reroute keeps the unconditional push hold at the start of session_before_compact; the preparation is sent from session_compact_failed after the deferred release flushes held pushes" — bearing: constrains
- 260911-feat-ws-pi-held-push-batch-delivery (2026-09-13, commit 4eece5b2): "Approval and question actionability is resolved against current record identity at snapshot time; superseded controls remain visible but are omitted from the actionable summary." — bearing: constrains
- 260914-feat-ws-pi-mailbox-native-steer-push (2026-09-15, Result): "`buildMailboxPushMessage` produces a `PushBatchItem` with `customType:ws-mailbox`, `state:informational` (forced by the FIFO's raw held-send materialization)" — bearing: supports
- 261009-bug-pi-mailbox-busy-drain-splits-into-per-envelope-steers (2026-10-09, commit e5202ac3): "`admitPush` gains a batch tail rather than a new held shape, so materialization and terminal bookkeeping are unchanged." — bearing: supports
- 261007-bug-pi-adapter-messages-read-as-user-text (2026-10-07, commit 030f07d5): "The label is applied once at each send ... never inside content builders, so a batch carries it once and its inner <message> items and details stay unlabeled." — bearing: constrains
- 261004-feat-pi-explicit-post-compaction-continuation (2026-10-04, Decisions D4): "It does not promise receipt or processing of every message arriving during preparation/compaction. Leave held child pushes, wake accounting, host follow-up queues and their existing ordering unchanged." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/src/lead-compaction.ts, plus tests under agents-plugin-pi/test/ |
| scope.surface | cross-module | goal-loop.ts session_before_compact/session_compact handlers must stage and enqueue into spawner.ts heldPushQueue; no package exports, ws-pi-bridge is private |
| scope.new_public_symbol | unknown | ticket names no symbol; a new spawner.ts export for front-inserting recovered items is likely but not specified |
| scope.new_type_contract | yes | HeldInput needs a recovered-item shape or state carrier so materializeHeldInput stops forcing informational, agents-plugin-pi/src/spawner.ts#L1633-L1641 |
| scope.test_surface | existing | agents-plugin-pi/test/goal-loop.test.ts, agents-plugin-pi/test/spawner.test.ts, agents-plugin-pi/test/push-flush-order.integration.test.ts, agents-plugin-pi/test/lead-compaction.test.ts |
| complexity.reuse_points | confirmed | heldPushQueue, flushHeldPushes early return on leadCompactingRef, submitHeldPushBatch, ws-push-batch details.items read in spawner.ts and push-protocol.ts |
| complexity.side_effect_risk | moderate | changes held-queue ordering and post-compaction delivery content on every lever compaction that drained a steer |
| risk.correctness | high | ordering, double delivery on failed compaction, actionable state replay, terminal bookkeeping, and label de-duplication all must hold |
| risk.fit | moderate | adds a staging path spanning goal-loop compaction handlers and the spawner push FIFO |
| risk.test | moderate | tests must simulate Pi draining a steer into a custom_message entry around an aborted lever run |
| risk.security_or_contract | moderate | replaying actionable approval or question items with stale state could re-expose already-handled actions to the lead |

## Phases

### Phase 1: Recover boundary steers across lever compaction

Implement the decisions above in the Pi lead compaction path.

Verification:

- A test reproduces the loss shape: a push admitted as a steer while the lever
  turn streams, then the lever compaction; after release, the post-compaction
  `ws-push-batch` carries the recovered items first, in original order, with
  their original `state`, followed by pushes held during compaction.
- A recovered `ws-push-batch` is unpacked into its items (no nested batch),
  and a recovered single-message steer is not double-labeled.
- A failed or cancelled lever compaction recovers nothing (the steer is still
  in context), and a non-lever compaction recovers nothing.
- The landed compaction must be the ws-authored summary for recovery to
  commit; a native-summary fallback recovers nothing.
- A recovered single `ws-agent-approval`/`ws-agent-question` steer is replayed
  as `actionable`.
- Terminal deliveries of recovered items are not reset to `held`, and
  `afterEnqueue` does not run a second time.
- The premise that Pi records a drained custom steer as a `custom_message`
  session entry is pinned by a test or a host-source check recorded in the
  Result.
- Existing `agents-plugin-pi` test suites pass.

### Result (1e5fb107d) - 2026-10-15

Landed in `1e5fb107d` (implementation) and `0b436c621` (review test fix).

- `lead-compaction.ts`: `collectLeverBoundarySteers(entries)` walks back from
  the branch end to the latest `ws-compact` toolResult (a `compaction` entry
  or the branch start reached first recovers nothing) and collects
  `custom_message` entries whose `customType` is `ws-push-batch` or a
  `PUSH_FAMILIES` member. Batches unpack into `details.items` with their
  stored `state` (`ws-goal-control` items dropped: applied at their own
  boundary, never model content); single steers are unlabeled through
  `adapterLabeledBody` and take `actionable` for approval/question,
  otherwise `informational`.
- `spawner.ts`: `HeldRawSend` gains an optional `state` read by
  `materializeHeldInput` (default `informational`, so existing raw senders
  are unchanged); `prependRecoveredPushes(items)` unshifts them as raw held
  entries with no terminal reference, so terminal deliveries stay
  `enqueued` and `afterEnqueue` never re-runs.
  `GOAL_CONTROL_CUSTOM_TYPE` moved to `push-protocol.ts`.
- `goal-loop.ts`: the lever branch of `session_before_compact` stages the
  items; `session_compact` commits them only when the stored entry is the
  adapter's and the staged operation is the active one; staging is cleared
  at every `session_before_compact`, `session_compact_failed`, the lever's
  `onError`, and shutdown.

Premise (host-source check, Pi 0.84.4): `agent-session.js` `message_end`
for `role === "custom"` calls `sessionManager.appendCustomMessageEntry`,
which writes a `type: "custom_message"` entry (`session-manager.js`);
`pi-agent-core` `agent-loop.js` polls `getSteeringMessages()` after
`turn_end` and records the drained steer before the aborted assistant turn;
`compact()` starts with `await this.abort()`, which does not clear the
steering queue, then reads `getBranch()` for `session_before_compact`; and
`session_compact` is emitted before the lever's `onComplete`.

Verification: `npm test` in `agents-plugin-pi`: 2279 tests, 2276 pass, 0
fail, 3 skipped. New tests: `test/lead-compaction.test.ts` "lever boundary
steers (261015)" (4) and `test/goal-loop.test.ts` "pushes steered into the
lever turn survive the lever compaction (261015)" (5), covering front
insertion and order ahead of compaction-held pushes, actionable approval
replay with a single label and action summary, batch unpacking with no
nested batch, the release's push wake for a recovered-only queue, and
no recovery on failed, native-summary, or non-lever compaction. Mutation
checks: removing the commit fails both positive tests; dropping the
ws-summary gate fails the native-summary test.

Lite review: 0 Critical, 0 Important. One Minor was fixed (the release
wake was not asserted). Two Minors were left as they are: the
stale-operation guard is untested, and the terminal assertions hold by
construction.
