---
title: "Tracker host: network fetch and sync stop blocking local reads of the store"
related:
  261110-bug-ws-dashboard-daemon-lock-scope: "its Decision 6 bounds wiki and attachment reads at 500 ms until this split lands"
---

# Tracker store lock: split network work from local reads

## Background

A tracker store's `mu` (`agents-plugin-tool/internal/wstrackerhost/host.go`,
`with`) is held across network calls through `attempt`, with
`networkTimeout` at 60 s: `Check` and `Sync`, `PollOnce` (`poll.go`), and
the remote attachment fetch (`attachments.go`). Local reads take the same
lock: the wiki routes (`wiki.go`), local attachment reads, and the view
reload (which also spawns git in `countCommits`). During any fetch, a view
GET costs its full `ViewWithin` wait and wiki or attachment reads wait up
to `writeLockWait`. Found by the dashboard stall investigation (whiteboard
topic "데몬 응답 지연 / 서포트 채팅 무응답"); the user chose to do this
after the daemon lock-scope fix, as its own ticket ((633) a).

## Decisions

1. **Network work no longer holds the lock local reads need (user,
   (633) a).** Fetch and sync run without blocking the view reload, wiki
   reads, and reads of attachments already present locally.

## Constraints

- To settle at promotion: the lock split (a network-op lock separate from
  the local-state lock, relying on wsledger's own on-disk locking, or
  another shape), which ledger invariants the split must keep (a reload
  never observes a half-applied fetch; `DispatchWith`'s write-lock and
  `$N` reference rule), and whether
  `261110-bug-ws-dashboard-daemon-lock-scope` Decision 6's 500 ms bound is
  removed or kept.
- Prior decision 55e7a3c5: `MarkSeen` already avoids the store lock for
  this reason.

## Phases

### Phase 1: split

Scope and verification are settled at promotion.
