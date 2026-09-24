---
title: Ticket index - read timeout, absence caching, and review-sweep fixes
related:
  260924-feat-origin-ticket-ownership-index: introduced the index, its read path, and the scenario contracts (A*, B*, C*, I*) the findings cite
  260924-chore-pin-ticket-index-playbook-contracts: sibling fix ticket from the same review sweep (playbook pins and doc wording); both edit mcp/ticket_index.go, so this one runs first
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b20432e837bbe60e
sage-review-completeness-reviewed: b20432e837bbe60e
---

# Ticket index - read timeout, absence caching, and review-sweep fixes

## Background

Dogfood on this repository (2026-09-24, develop 41803afb), right after a
successful `tickets.acquire` / `tickets.release` push to a GitHub SSH origin:
`tickets.query` reported `ticket-index: origin unreachable; ownership is from
the cached index (age 4m57s)`, although origin was reachable.

- `DefaultReadTimeout` is 1500 ms (`agents-plugin-tool/internal/wsindex/client.go`),
  chosen for the ticket's "query must not block beyond roughly 2 seconds".
- A plain `git ls-remote origin 'refs/ticket-index-larkspur/*'` from the same
  machine took 2.6 s wall time (SSH handshake dominates).
- So on an ordinary SSH remote every read past the 60 s TTL times out, is
  reported as unreachable, and serves the stale cache; the "unreachable"
  wording is misleading, and the view never refreshes from the read path.

The develop review sweep 0a361491..4a2f4721 (stamped `block` against this
ticket) widened the scope. Its release-blocking finding shares the root cause:

- **Never-opted-in projects pay the timeout on every call.** On a clone that
  has never seen an index, a discovery `ls-remote` that times out returns
  Unreachable with `timedOut=true`, and `Read` and `Write` deliberately skip
  `markAbsent` (`client.go` `sync`, `Read`, `Write`). A downstream project that
  never ran `tickets.index_init`, on an SSH origin slower than 1.5 s, runs a
  network `ls-remote` and waits up to 1.5 s on every `tickets.query` and every
  move/close guard read; on a blackholing origin, every
  move/close/create_empty/sage_stamp piggyback also waits the 10 s write
  timeout. This breaks "no ref, no change" and A2 (at most one discovery per
  absence TTL). A reviewer probe measured 3 `ls-remote` calls for 3 `Read`s.

The sweep also found correctness minors in the index, index test gaps, and
fit minors; all are folded into this ticket (Phases 1 and 2). Labels follow
the sweep's findings file: the release-blocking finding above is its M1;
C2-C6 are index correctness minors; F1, F2, F4 are fit minors; T1-T8 are
test gaps. The sweep's F3 (a Pi constant name) is in the idea ticket
`260924-chore-review-sweep-test-and-naming-minors`; its T9, F5, and F6
(playbook pins and doc wording) are in the sibling chore ticket.

## Decisions

- **Keep the ~2 s query bound.** `DefaultReadTimeout` stays 1500 ms.
  Rejected: raising it, since that makes every stale-TTL query slower on every
  project to fix a wording problem, and the view still refreshes from writes.
- **A read timeout on a seen clone is "not refreshed", not "unreachable".**
  The read-path report for a timeout says the view was not refreshed because
  origin timed out, with the cache age. Agreed text for the query trailer:
  `ticket-index: origin did not answer within <read timeout>; ownership is
  from the cached index (age <age>), not refreshed`. "Unreachable" is kept for
  real transport failures (refused, DNS, auth, non-timeout errors). The
  per-ticket ViewUnknown text ("unknown (origin unreachable and no cached
  index)") becomes "unknown (origin not reached and no cached index)", since
  a timeout can produce it. Unchanged: the JSON `index_state` values, and the
  offline pending warning on writes (a 10 s write timeout is reported as
  unreachable, as today). A seen-clone timeout is never absence: the
  discontinuity/discard logic is unchanged.
- **A discovery timeout on a never-seen clone is cached as absence, scoped
  by the path that timed out.** The cached absence records its source:
  confirmed (no ref, or a fast non-timeout transport failure such as offline,
  refused, or DNS, which is cached as absence today), read-timeout, or
  write-timeout. All three last the
  existing `DefaultAbsenceTTL` (10 minutes, unchanged).
  - Reads (`tickets.query`, the move/close guard read) skip discovery on any
    cached absence.
  - Writes (the piggybacks of move, close, create_empty, sage_stamp) skip
    discovery only on confirmed or write-timeout absence. A read-timeout
    absence does not silence them: they still run their 10 s discovery, at
    most once per TTL (a write-path timeout then records write-timeout
    absence). This keeps today's behavior where a slow origin's index is
    found by the first move/close, and still removes the per-call cost on a
    blackholing origin.
  - A successful discovery on either path clears the cached absence.
  A never-seen clone has no cache to discard, so treating a timed-out
  discovery as "absent for now" behaves exactly like a project without the
  index. Supersedes df16de9e ("A timeout is not cached (rctx.Err), so a slow
  SSH ls-remote is retried and later discovered"): the sweep showed that rule
  makes never-opted-in projects pay the timeout per call. Rejected: a longer
  TTL (for example 30 minutes), since it widens the window in which a clone
  ignores an index a collaborator just created. Rejected: one unscoped
  absence for both paths, since a read timeout would then stop move/close
  from ever finding the index on an origin slower than the read timeout.
- **A lease acquire bypasses the absence cache.** `tickets.acquire` called
  without an impl branch (the lead's lease acquire) always runs discovery, so
  a clone learns of an index a collaborator just created at the one
  ownership-bearing call instead of up to 10 minutes later. The worker's
  impl-branch record (acquire from an `impl/*` branch) keeps using the cache:
  its lead's acquire on the same clone just probed. A successful discovery
  clears the cached absence; a discovery that times out or finds no ref
  re-caches absence (write-timeout or confirmed) and acquire returns its
  existing silent index-absent result. Accepted cost: an index-absent project
  on a blackholing origin waits up to the 10 s write timeout once per lease
  acquire (once per lead-run). Supersedes the predecessor's read-path rule
  ("Within the absence TTL, acquire returns the legacy mock without contacting
  the remote; a lease missed in the window after another clone's init is the
  same accepted window as A9") and narrows its A2 ("at most one remote
  discovery call") to exclude lease acquires. Rejected: bypassing the cache
  for every write, since that restores the per-call cost this ticket removes;
  bypassing it for the impl record too, since that doubles the blackholing
  cost per run for no new information.
- **Live close never changes a lease held by a different email without a
  matching override (C2).** The move/close guard reads the TTL-cached view,
  and the piggyback then applies against the fresh tip. Live `close` runs the
  same owner-conflict check that replay runs: when the fresh tip shows a
  holder that needs an override (different email) and the call carried no
  override, or an override naming a different holder, the lease is left
  unchanged and the piggyback prints one `ticket-index:` line naming the
  holder. This is a non-refusal outcome: the same write still registers,
  flushes the pending log, and prunes. The already-closed no-op (C3) is
  checked first, so closing a lease another email already closed prints
  nothing. The host operation still succeeds (piggyback never fails the host
  operation). This narrows a risk df16de9e accepted ("the guard evaluates a
  cache up to the read TTL old"): the stale guard read stays, but its
  consequence no longer reaches another person's lease. Rejected: a fresh
  (TTL-bypassing) guard read before the folder
  move, which adds a network round trip to every move/close and still leaves
  a window between read and write.
- **Override replay is idempotent where state shows it (C3).** A close whose
  lease is already `closed` is a no-op with no audit line, with or without an
  override. An override move's audit line includes the pending entry ID, so a
  replayed duplicate (crash between push and pending clear) is recognizable as
  the same event; it is not deduplicated. Rejected: persisting applied entry
  IDs in the index (schema change for a crash-window duplicate). df16de9e
  accepted the duplicate audit line as a recorded risk; this fixes only the
  close case, where the index state already shows the replay is a no-op.
- **Replay keeps the takeover warning (C4).** A replayed entry's
  `Outcome.Warning` (same email, different clone or track) is carried out of
  `Replay` and printed as a `ticket-index:` line by the flushing tool, like
  the other replay reports.
- **Init and check report discards (C5).** `Create`, adopt, and `Check`
  return their discontinuity discard reports, and `tickets.index_init` prints
  each once as a `ticket-index:` line (the same "N offline entries were
  discarded" text the other paths print).
- **One review-track resolution (C6).** When the local
  `refs/remotes/origin/HEAD` is missing, paths that already go online (every
  index write, acquire, init) resolve origin's default branch with the same
  `ls-remote --symref` probe `InitSource` uses, then read the declared
  review-track from that branch's AGENTS.md (fetching that branch's tracking
  ref first when it is missing, as `InitSource` does), so init, acquire,
  pruning, and
  GC agree. Read-only paths with no network (the query hint) keep the local
  fallback. No caching of the probed branch: the probe only runs on clones
  lacking `origin/HEAD`. Rejected: writing `refs/remotes/origin/HEAD` into the
  user's clone (mutates user refs); storing the track in the index (schema
  change, and a later AGENTS.md change would not propagate).
- **One ticket stem and path parser (F1).** `wsindex` and `mcp` use exported
  `wsdoc` helpers for the stem regex, the `ai-docs/tickets/<status>/<stem>.md`
  path shape, and ticket-file lookup, instead of their own copies
  (`wsindex/origin.go` `ticketStemRE` and `ticketPath`,
  `mcp/ticket_index.go` `ticketFileExists`). The lookup keeps
  `wsdoc.findTicketPath`'s handling of tickets hidden by sparse checkout.
  `wsdoc` imports neither `wsindex` nor `mcp`, so no cycle arises.
- **Naming (F2, F4).** `indexMockText` / `indexMockResponse` and the "silent
  mock" test names become names that say "index absent"; `holderText` is
  inlined. Scenario-ID Go test names (`TestA2...`, `TestB3...`,
  `TestI18I22...`) keep their IDs, per df16de9e (the IDs are the
  traceability key the Results map to tests); each test file using them gains
  one file-level comment naming `260924-feat-origin-ticket-ownership-index`
  as the source of the scenario IDs. Rejected: renaming to behavior names
  (reverses df16de9e for readability alone).

## Constraints

- Behavior outside the decisions above is unchanged. In particular the index
  schema, the lease matrix, and the no-ref byte-identical output are
  unchanged; a project without the index still sees no output difference.
- Every behavior change in Phase 1 lands with a test that fails without it.
- Tests that touch config use an isolated `HOME` / `WS_CONFIG_HOME` like the
  existing index harness; wall-clock assertions stay bounded (no new tight
  timing bounds).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- df16de9e (2026-09-24, commit): "A never-seen clone keeps caching a fast unreachable failure as absence ... A timeout is not cached (rctx.Err), so a slow SSH ls-remote is retried" — bearing: superseded (never-seen timeout-caching Decision)
- df16de9e (2026-09-24, commit): "Test scenario IDs stay in test names. They are the ticket's traceability key, and the Results map IDs to tests." — bearing: supports (F4 keeps the IDs)
- df16de9e (2026-09-24, commit): "Risk accepted, recorded rather than fixed: re-flushing an override close after a crash (I6) can write a duplicate audit line; the guard evaluates a cache up to the read TTL old" — bearing: superseded in part (C2 bounds the stale-guard consequence; C3 fixes the close case)
- 260924-feat-origin-ticket-ownership-index (2026-09-24, Decisions, read path): "Within the absence TTL, acquire returns the legacy mock without contacting the remote; a lease missed in the window after another clone's init is the same accepted window as A9" — bearing: superseded for lease acquire (acquire-bypass Decision)
- 260924-feat-origin-ticket-ownership-index (2026-09-24, Decisions): "An unreachable remote never counts as absence; offline alone never discards." — bearing: constrains
- 260924-feat-origin-ticket-ownership-index (2026-09-24, Decisions): "Read path. tickets.query fetches the index ref with a short timeout and caches it with a TTL. On fetch failure it serves the cache marked stale, with its age. Query must not block beyond roughly 2 seconds" — bearing: constrains
- 24898ba4 (2026-09-24, commit): "Discovery is ls-remote only while the clone has never seen an index (one call per absence TTL); a seen clone ... classifies a failed fetch through ls-remote so a deleted ref reads as absence" — bearing: supports
- 57c930bb (2026-09-24, commit): "The review-track resolves origin-first: the local refs/remotes/origin/HEAD tree's AGENTS.md, then wsreview.ResolveTrackFallback. The local checkout's declaration is never read (A8)." — bearing: constrains
- 57c930bb (2026-09-24, commit): "All validation (stem, track, email, file existence, override reason) runs inside a Prepare hook that fires only once the index is known to be in use. The legacy mock therefore stays silent" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsindex/client.go, origin.go, apply.go, agents-plugin-tool/internal/mcp/ticket_index.go, ticket_index_view.go, agents-plugin-tool/internal/wsdoc/tickets.go and tickets_mutate.go, plus wsindex and mcp ticket_index tests |
| scope.surface | cross-module | wsindex, mcp, and wsdoc change together; tool output wording for a read timeout changes at agents-plugin-tool/internal/mcp/ticket_index_view.go#L264 |
| scope.new_public_symbol | yes | exported wsdoc stem regex, ticket path parser, and ticket lookup helpers per F1 |
| scope.new_type_contract | yes | wsindex Create, adopt, Check return discard reports per C5; Replay carries Outcome.Warning out per C4 |
| scope.test_surface | existing | agents-plugin-tool/internal/wsindex/storage_test.go, continuity_test.go, apply_test.go, agents-plugin-tool/internal/mcp/ticket_index_test.go, ticket_index_view_test.go |
| complexity.reuse_points | confirmed | markAbsent and absenceCached in wsindex/client.go, InitSource ls-remote --symref probe wsindex/origin.go#L153-L156, NeedsOverride, wsdoc findTicketPath tickets_mutate.go#L710, hangingSSH test helper |
| complexity.side_effect_risk | moderate | changes absence-cache state and live-close lease writes pushed to origin, plus a new network probe on write paths lacking origin/HEAD |
| risk.correctness | high | absence and continuity state machine, CAS close owner-conflict check, and replay idempotence; the sweep stamped the range block on this finding |
| risk.fit | moderate | F1 moves stem and path parsing into wsdoc across packages and C6 splits online versus offline track resolution |
| risk.test | moderate | timeout-driven tests with a hanging remote, broad test renames, and wall-clock bound changes in storage_test.go |
| risk.security_or_contract | moderate | no-ref byte-identical output contract and ticket-index tool wording must hold; index schema must stay unchanged |

## Phases

### Phase 1: Absence caching, timeout wording, and index correctness fixes

Implement every Decision except the F2/F4 naming, each with its own test:

- Never-seen discovery timeout caches absence scoped by path: after a
  read-timeout absence, repeated `Read`s make no `ls-remote` while the next
  move/close piggyback still discovers (and registers into) an index present
  on origin; after a write-timeout absence, piggybacks skip too. A hanging-
  origin probe (the harness's hanging-remote helper is `hangingSSH`,
  `agents-plugin-tool/internal/wsindex/storage_test.go#L80-L92`) counts calls
  per path: across repeated `Read`s and piggyback writes, at most one
  read-path and one write-path discovery per absence TTL.
- Lease acquire bypasses the absence cache: with absence cached and an index
  created meanwhile on origin, acquire discovers and leases it; an impl-record
  acquire with absence cached makes no remote call.
- Code comments stating the superseded rules are updated (the timeout-not-
  cached comments in `wsindex/client.go` and the absence-mock comment in
  `mcp/ticket_index.go`).
- Seen-clone read timeout reports "not refreshed" (with age); a real
  transport failure still reports unreachable.
- C2: a different-email lease acquired after the guard's cached read is left
  unchanged by a live close without override, with one `ticket-index:` line;
  the file still moves.
- C3: replaying an override close of an already-closed lease produces no
  commit; an override move's audit line carries the entry ID.
- C4: a replayed takeover prints its warning line.
- C5: `index_init` on a clone with pending entries against a deleted index
  prints the discard report.
- C6: a clone without local `origin/HEAD`, whose origin default branch
  declares `review-track: develop`, prunes against develop and gets
  acquire's origin-closed refusal for a stem closed on develop, the same track
  init registered from.
- F1: a ticket hidden by sparse checkout is found by the index paths that
  look tickets up.

### Phase 2: Index test backfill and naming

Depends on Phase 1 (tests target the Phase 1 behavior). Test-only plus the
F2 renames and F4 comments:

- T1: `tickets.index_init` with a delegate or leaf session key is rejected
  (precedent `TestServeStdioSageStampDelegateKeyBlocked`).
- T2: piggyback write failure (`failFetch` on a never-seen clone whose origin
  holds the index): the host `tickets.move` succeeds, the file moves, and
  exactly one `ticket-index:` line reports the failure.
- T3: `tickets.sage_stamp` registers the stem in index mode, and is added to
  `TestNoRefPathIsByteIdentical`.
- T4: landed-closure pruning on a non-acquire write that reads the local
  tracking ref without fetching; a `LoadContext`-level test that `Closed` is
  filled without fetch.
- T5: the GC race test asserts lease protection with an entry kept only by
  its lease (not open anywhere), and GC-once via `meta.last_gc` and the GC
  commit count.
- T6: push `--no-verify` (a failing pre-push hook on the clone), commit
  `--no-gpg-sign` (signing on in the clone config), and permanent refusal
  without retry (a pre-receive hook rejecting the index ref).
- T7: init against an empty or unpushed origin creates the ref with zero
  registrations.
- T8: a flushed entry's worktree path appears in neither `index.json` nor
  commit history; the I3 report assertion checks the holder's own track,
  acquire time, and local worktree path.
- Index test minors: assert the A5 env loop's `calls` is non-empty; add
  `OpRegister` rows to `TestNeedsOverride`; refused move/close leave the file
  in place and write nothing to the index; `unleased_or_mine` combined with
  `limit`, and a wsdoc-level `TicketFindOptions.Exclude` test; move `t.Fatalf`
  out of goroutines; retitle the tool-level I5-I7 tests to say they record
  entries sequentially (the library-level tests own concurrency); loosen, not
  remove, the 2.5-4 s wall-clock bounds in `storage_test.go` (they assert the
  timeout bound itself) to a generous multiple that survives `-race`.
- F2 renames and the F4 file-level scenario-ID comments.
