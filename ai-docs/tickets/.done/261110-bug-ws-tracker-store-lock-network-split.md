---
title: "Tracker host: network fetch and sync stop blocking local reads of the store"
related:
  261110-bug-ws-dashboard-daemon-lock-scope: "its Decision 6 bounds wiki and attachment reads at 500 ms until this split lands"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 17436bf2ea19af7b
sage-review-completeness-reviewed: 17436bf2ea19af7b
completed: 2026-11-11
---

# Tracker store lock: split network work from local reads

## Background

A tracker store's `mu` (`agents-plugin-tool/internal/wstrackerhost/host.go`,
`with`) is held across network calls through `attempt`, with
`networkTimeout` at 60 s: `Check` and `Sync`, `PollOnce` (`poll.go`), and
the remote attachment fetch (`attachments.go`). Local reads take the same
lock: the wiki routes (`wiki.go`), local attachment reads, and the view
reload (which also spawns git in `countCommits`). During any fetch, a view
GET costs its full `ViewWithin` wait and wiki or attachment reads from the
dashboard wait up to `dashboardLockWait` (500 ms) and then answer busy,
while the MCP wiki tools wait on `st.mu` without a bound
(`agents-plugin-tool/cmd/ws-mcp/tracker_host.go#L31-L40`,
`agents-plugin-tool/internal/wstrackerhost/host.go#L313-L323`;
`writeLockWait` bounds only the `write.lock` file and the background
reload, `host.go#L855-L860`, `pageview.go#L167-L169`). Found by the dashboard stall investigation (whiteboard
topic "데몬 응답 지연 / 서포트 채팅 무응답"); the user chose to do this
after the daemon lock-scope fix, as its own ticket ((633) a).

## Decisions

1. **Network work no longer holds the lock local reads need (user,
   (633) a).** Fetch and sync run without blocking the view reload, wiki
   reads, and reads of attachments already present locally.
2. **The split reaches wsledger (technical, (270)).** Fetch and Sync run
   their network phases outside `wsledger.(*Ledger).withLock`, fetching only
   objects and remote-tracking refs, and take the lock briefly to apply the
   result. `wstrackerhost` gets a separate network-operation mutex that
   serializes network work, and `st.mu` guards local state only.
   Rejected: splitting `st.mu` alone, which cannot help because `withLock`
   holds the in-process mutex and the `<store>/lock` flock across the network
   calls and `Reload` takes the same lock
   (`agents-plugin-tool/internal/wsledger/store.go#L350-L356`, `#L1118`,
   `#L1175`, `#L1204`); a separate lock-free read path, a larger change.
3. **Invariants kept (technical, (271)).** The apply step is atomic under the
   lock, so a reload never observes a half-applied fetch. `DispatchWith`'s
   write-lock and its `$N` reference rule are unchanged.
4. **The 500 ms dashboard bound stays (technical, (272)).**
   `261110-bug-ws-dashboard-daemon-lock-scope` Decision 6's bound remains a
   safety net. Rejected: removing it, which would turn a later regression
   back into an unbounded wait.
5. **Network work stays serialized across processes, and the apply step
   re-checks its premises (technical, review-raised).** A second flock,
   `<store>/net.lock`, held for the network phase, serializes network work
   across processes the way the `<store>/lock` flock does today (prior
   decision 9b3721bd: "another process syncs"), so two processes never run
   `git fetch` or push on one store directory at once. The network mutex and
   `net.lock` cover every network path: `Check` and `Sync` (fetch and push),
   `PollOnce`, the idle push, and the remote attachment fetch
   (`ReadAttachment` in `attachments.go`); on the dashboard attachment
   path, the wait for the network mutex and `net.lock` carries the same
   context bound as Decision 4's, so a not-yet-local attachment answers
   busy instead of waiting out a sync. A push runs after the apply step,
   from the applied state. A local append that lands between the network
   phase and the apply step merges at apply, with no retry of the network
   phase. The apply step re-checks the registry entry under `st.mu`; a store
   unregistered meanwhile discards the fetched result.
   Rejected: relying on a single opener process now that the daemon hosts
   the MCP servers, which nothing enforces; an in-process mutex alone, which
   lets two processes race on git refs ("cannot lock ref").

## Constraints

- Prior decision 55e7a3c5: `MarkSeen` already avoids the store lock for
  this reason.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 261109-bug-ws-tracker-external-write-refresh (2026-11-09, Decisions): "The real wait is not `st.mu` alone: `Ledger.Fetch` / `Sync` run inside `wsledger.(*Ledger).withLock`, which holds the in-process `l.mu` and the cross-process `<store>/lock` flock across the network call" — bearing: constrains
- 261110-bug-ws-dashboard-daemon-lock-scope (2026-11-10, Decisions): "Tracker wiki and attachment reads wait a bounded time for the store lock (user, whiteboard (633) a: split the lock later in 261110-bug-ws-tracker-store-lock-network-split, bound these reads meanwhile)" — bearing: supports
- b10d2da6 (2026-11-10, commit): "The bound travels in the context rather than in new Host methods or parameters so the MCP wiki tools, which call the same Host methods, keep today's unbounded wait without any change" — bearing: constrains
- 922d5931 (2026-11-11, commit): "Rejected: holding the root lock across open to close the open/unregister race; a sync can hold the store lock for a minute and would starve every registry write." — bearing: constrains
- 261111-feat-ws-tracker-store-unregister-relabel (2026-11-11, Result): "every host call re-checks the registry entry under the store mutex and writes re-check after taking `write.lock`" — bearing: constrains
- 9b3721bd (2026-11-08, commit): "The lock wait is two network timeouts plus the lock wait, because the holder may wait inside Append for wsledger's lock while another process syncs (a push and a fetch)." — bearing: constrains
- 3d4630e8 (2026-11-09, commit): "The bound sits in the host, which owns the timeout policy (wsledger has no network timeout of its own); it covers only the ledger read, not the Reload before it." — bearing: constrains
- 261107-feat-ws-tracker-ledger-protocol (2026-11-08, Result): "`Events`, `Identity`, and `Pending` wait on the store mutex while `Sync` holds it across network calls." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstrackerhost/host.go, poll.go, attachments.go, wiki.go, view.go, pageview.go; agents-plugin-tool/internal/wsledger/store.go (withLock holds l.mu and the flock across Fetch and Sync, store.go#L350-L356) |
| scope.surface | cross-module | wstrackerhost store locking and wsledger Ledger.withLock (Decisions 2 and 5) |
| scope.new_public_symbol | yes | a wsledger split of Fetch and Sync into a network phase and an apply step; names chosen in planning |
| scope.new_type_contract | no | the ledger's event and cursor contracts are unchanged |
| scope.test_surface | existing | agents-plugin-tool/internal/wstrackerhost/lockwait_test.go, pageview_test.go, wiki_test.go, host_test.go; wsledger store tests |
| complexity.reuse_points | confirmed | storeLock lockWithin in host.go; the existing `<store>/lock` flock pattern in wsledger for `<store>/net.lock` |
| complexity.side_effect_risk | high | lock split touches every Host entry point and the cross-process ledger flock |
| risk.correctness | high | concurrency change; a reload must never observe a half-applied fetch and DispatchWith write-lock plus $N rule must hold |
| risk.fit | moderate | must coexist with the 500 ms dashboard bound, MarkSeen seenMu, and unregister's re-check under the store mutex |
| risk.test | high | race behavior needs deterministic lock-holding tests like lockwait_test.go and -race runs |
| risk.security_or_contract | moderate | ledger invariants and DispatchWith semantics are an internal contract other callers rely on |

## Phases

### Phase 1: split

Scope:
- Decisions 1-5: the wsledger network phase outside `withLock` with a short
  apply step, the network-operation mutex and `<store>/net.lock` over every
  network path, the apply-time registry re-check, and `st.mu` narrowed to
  local state.

Verification ((273)):
- With a fetch held open by a slow fake remote, and separately with a slow
  push held open inside `Sync`, a view reload, a wiki read, and a read of an
  attachment already present locally each finish promptly: within the
  500 ms dashboard bound and without a busy answer.
- No reader observes a half-applied fetch; a local append made during a
  held fetch survives the apply and is pushed.
- A store unregistered during a held fetch discards the result.
- Two ledgers on one store directory (two processes' view) never run
  network work at once.
- The tests hold locks deterministically, in the style of
  `lockwait_test.go`, and run under `-race`.
- Existing wsledger and wstrackerhost tests pass, including `DispatchWith`
  `$N` references.

### Result (847121a16) - 2026-11-11

Implemented in one commit (847121a16).

- wsledger: a network lock (an in-process one-slot channel plus the
  `<store>/net.lock` flock, `NetLockFile`) is held for the whole of `Fetch`,
  `Sync` (the idle push included), the on-demand attachment fetch,
  `WikiSave`, `Register`, `Restore`, and `MoveToFreshClone`. `Fetch` and
  `Sync` take the store lock only for their local steps: deciding what to
  push, recording what landed, and applying a fetch in one step. The fetch
  itself writes only objects and remote-tracking refs.
- wsledger: `Options.ApplyGate` wraps every locked step of `Fetch` and
  `Sync`; a gate that does not run the step discards the fetched result.
  `WithNetWait` bounds the network-lock wait (`ErrBusy`).
  `ReadLocalAttachment` never fetches (`ErrAttachmentNotLocal`);
  `ReadAttachment` reads locally first and fetches under the network lock.
- wstrackerhost: `store.net` is the per-store network mutex, taken before
  `st.mu` and never while holding it; `st.mu` guards local state only.
  `Check`, `Sync`, `PreRead`, `CheckAll`, `PollOnce`, `WikiSave`, and the
  remote attachment read run through `withNet`. `applyGate` re-reads the
  registry entry under `st.mu` and refuses the step of a store unregistered
  (or registered again) meanwhile, retiring the handle.
- The dashboard attachment path reads a local copy under the store lock and
  goes to the network lock only for a copy this clone lacks, with the page's
  lock-wait bound carried onto both the host mutex and `net.lock`.
- `DispatchWith`, its write lock, and the `$N` rule are unchanged.

Decisions made while implementing:

- This clone's own ref and attachment refs are pushed by commit id, so the
  push is exactly what the deciding step read even if another process closes
  a batch meanwhile; genesis, claim, and wiki refs still go by name.
- The apply-time registry re-check is a hook the ledger calls
  (`ApplyGate`), because the idle push runs inside the ledger with no host
  call around it.
- `WikiSave`, `Register`, `Restore`, and `MoveToFreshClone` take the network
  lock but keep the store lock across their own network calls; Decision 5
  lists none of them among the paths to split, so a wiki save still holds
  local reads for its duration.
- `Unregister` does not wait for a fetch in flight (its result is
  discarded); a push in flight still refuses it as unpushed.
- `TestViewWithinServesLastStateWhileFetchBlocked` was removed: it asserted
  that a blocked fetch makes the page view stale, which is the bug. The
  stale-view path stays covered by
  `TestViewWithinReturnsWhileAnotherProcessHoldsTheStoreLock`.

Verification (via the remote builder, `go` in `agents-plugin-tool`):

- `go vet ./internal/wsledger/ ./internal/wstrackerhost/ ./internal/wsdashboard/ ./cmd/...`: clean.
- `go test ./internal/wsledger/ ./internal/wstrackerhost/ -race -count=2`:
  ok (174.9s, 140.2s).
- `go test ./internal/wstracker/ ./internal/mcp/ ./cmd/... ./internal/wsdashboard/... -race -count=1`:
  ok.
- New tests, one or more per verification bullet:
  `TestFetchInFlightHoldsNoLocalCall`, `TestPushInFlightHoldsNoLocalCall`,
  `TestFetchAppliesWhole`, `TestNetworkLockSerializesAcrossLedgers`,
  `TestApplyGateRefusalDiscardsTheFetch` (wsledger);
  `TestLocalReadsDoNotWaitForNetwork`,
  `TestRemoteAttachmentReadAnswersBusyWhileNetworkHeld`,
  `TestAppendDuringHeldFetchSurvivesAndIsPushed`,
  `TestUnregisterDuringHeldFetchDiscardsTheResult`,
  `TestNetworkCallsOfOneStoreRunOneAtATime` (wstrackerhost).

Lite review: clean, three Minor findings left open.

- On Windows an unregister during another process's fetch now fails at the
  rename (open `net.lock` and git files) with a rename error instead of a
  busy refusal; the store stays registered.
- A store unregistered during a fetch and registered again at the same path
  within one network timeout is not excluded from the old call's remaining
  git commands; the gate still discards the old result.
- A bounded call waits its bound separately at the host network mutex, the
  store lock, and `net.lock`, so a dashboard read of an attachment this
  clone lacks can wait up to about three times 500 ms before answering
  busy.
