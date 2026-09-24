---
title: Pi removal gate - abandoned claims and the claim-cycle read window
related:
  260924-bug-pi-review-sweep-correctness-fixes: made the sidecar removal gate claim-aware and left these two windows open
  260924-refactor-pi-channel-and-cost-module-boundaries: the no-behavior-change refactor these were first forwarded to; they moved here because they change behavior
---

# Pi removal gate - abandoned claims and the claim-cycle read window

## Background

Two accepted minors from `260924-bug-pi-review-sweep-correctness-fixes`
(merged 99f8ac90), originally forwarded to the predicate-unification
refactor, which settled on keeping three named predicates with no behavior
change:

- **Abandoned claim (4128ae93).** A removal claim abandoned on a detached home
  keeps the entry "claimed" forever: the sidecar revives it every session, and
  reconcile's `isOwnedHomeGone` has the same claim-held semantics
  (`agents-plugin-pi/src/agent-storage.ts`, `ownedHomeRemovalState` and
  `isOwnedHomeGone`).
- **Claim-cycle window (5c5ae07f).** A full claim cycle that writes and rolls
  back an eviction record entirely between the gate's record read and claim
  read is still read as a removal. Closing it needs a claim-generation marker.

## Open questions

- How an abandoned claim is recognized (lock liveness, claim age, owner pid)
  without reading lock liveness in the hot reconcile path, which 03305a62
  rejected.
- Shape of a claim-generation marker and where the gate compares it.
- Practical reachability of each window, to decide whether this is worth a
  ready ticket.
