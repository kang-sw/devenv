---
title: wsledger moved and re-issued events lose references to other events of the same batch
related:
  261107-feat-ws-tracker-ledger-protocol: the re-issue decision ((211) a) and MoveToFreshClone recovery this breaks
  261108-feat-ws-tracker-data-structure: the domain names items and comments by event id
---

# wsledger moved and re-issued events lose references to other events of the same batch

## Background

Review sweep S2 (`fbb6d0a90..8e21ac7a5`), correctness finding, overridden to
an important follow-up on the board ((453) ok) because the fix needs a
protocol-level design.

- `internal/wsledger/remote.go` ~L603-651 (`MoveToFreshClone`) and
  `store.go` ~L937-961 (re-issue) carry payloads byte-for-byte to a new event
  id.
- The tracker domain names items and comments by event id
  (`<member>.<clone>#<seq>`). An unpushed batch is commonly an `item.create`
  plus follow-ups naming it; `Dispatch` supports naming an earlier event of
  the same call through `NextID`.
- After a move, the follow-ups name an id that was never published (the old
  ref was reset); after a re-issue they name the Ignored original. Every
  client flags them "item ... is not in the state its author had seen".
- The payload-agnostic protocol cannot rewrite these ids, and `wstracker` has
  no hook to remap them. No test covers a batch that references itself across
  a move or a re-issue.
- Related minor from the same review: moved events keep `Of` (an old-stream
  seq), so the new stream's journal can mark unrelated new-stream seqs as
  already re-issued (`remote.go` ~L604-608, 648).

## Open Questions

- Remap at the ledger (an id-alias record emitted with the moved batch, which
  the domain resolves) versus at the domain (wstracker rewrites intra-batch
  ids through a hook the ledger calls on move / re-issue) versus forbidding
  intra-batch references (one event per dependent write).
- `MoveToFreshClone` has no caller yet; re-issue is live whenever a key is
  revoked with pending writes.
