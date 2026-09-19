---
title: Remote-control step should read the target's arm condition, not the reader's
related:
  260919-feat-mailbox-arm-wait-harness-include: predecessor
---

# Remote-control step should read the target's arm condition, not the reader's

## Background

Captured as a non-blocking observation from
`260919-feat-mailbox-arm-wait-harness-include` (worker report, merged to
develop). That ticket split `lead-use-mailbox`'s `## On: arm the wait` into a
harness-aware include and rewrote `## On: remote-control another session`
step 3 to defer to "whatever condition the reader's own rendered arm-the-wait
states" instead of a blanket registered-address rule. That removed a genuine
self-contradiction and is strictly better than before.

The residual gap: in remote-control, the sender (reader) and the target may run
different harnesses. Step 3 reasons about whether the *target* will notice the
mail, but now reads the *reader's own* harness arm condition. Example: a Codex
lead mailing a pi target — Codex's condition is "auto-wake only with a
registered named inbox", but the pi target auto-pushes off its session key
without a named inbox, so the sender's own condition mis-describes the target.

## Outcome Ledger

### Verified Findings

- pi auto-arms/pushes for the lead/owner role off the session key alone
  (`agents-plugin-pi/src/mailbox-waiter.ts`, `shouldArmMailboxWaiter`), whereas
  Codex's turn-boundary wake needs a registered address, and Claude needs a
  manually armed wait — so the "will the target notice?" condition is genuinely
  harness-specific to the target.
- Existing fallback bounds the impact: a target with neither armed wait nor
  auto-wake still notices at its own next active turn, so mail is not lost even
  when the sender misjudges.

### Open Questions

- Does the sender even have the target's harness identity available at
  remote-control time (e.g. via `mailbox.lookup_peers` peer metadata, which
  already reports harness/working-dir/start-time)? If so, step 3 could point at
  the target's harness rather than the reader's.
- Is a precise per-target condition worth the added prose, given the
  next-active-turn fallback already prevents loss? The refinement may be
  documentation-only (tell the sender to consult the target's harness) rather
  than a mechanism change.

### Rejected Alternatives

- Restating a blanket registered-address rule in step 3 — this is the
  self-contradiction the predecessor ticket removed; do not reintroduce it.
