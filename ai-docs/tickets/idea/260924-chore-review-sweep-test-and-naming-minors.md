---
title: Review-sweep test and naming minors (Pi usage rollup, worktree.list, config isolation)
related:
  260924-bug-ticket-index-read-timeout-below-ssh-roundtrip: the same sweep's release-blocking index fixes; these minors were left out as non-blocking
---

# Review-sweep test and naming minors (Pi usage rollup, worktree.list, config isolation)

## Background

Non-blocking findings from the develop review sweep 0a361491..4a2f4721 that
fall outside the index fix ticket:

- **Pi usage rollup tests.**
  - `agents-plugin-pi/test/agent-usage-rollup.test.ts`, the "telemetry reset
    on a revived record" test, deletes `revived.descendantUsage` before
    refreshing. It never exercises the `descendantUsageOrder` gate its title
    names and passes with the gate removed (mutation-confirmed). The gate is
    covered by later tests in the same file. Either seed an older in-memory
    value or retitle.
  - `agent-usage-rollup.integration.test.ts`, the "parent restart" test, now
    calls `restoreDescendantUsage` itself, so it no longer shows the
    production revival path restores the value (unit tests cover it).
- **worktree.list.** The "skip prunable entries" rule, the empty-pool
  "worktrees: none" output, and the lease-read-failure warning have no test.
- **Config isolation.** `TestWorktreeListIncludesLegacyInTreePoolUnderDefaultConfig`
  and the `git_merge_test.go` `legacy-pool-holder` dispatch depend on the
  default `worktree_pool` without isolating `WS_CONFIG_HOME`; a host
  `~/.ws/config.json` pool override would break them (same class as the two
  known agent-config tests that need a clean `HOME`).
- **Effort text test.** `server_test.go`'s `config.resolve_agent` case builds
  its expected text with `formatEffortForText`, the helper under test, with
  no independent "no injected line" assertion.
- **F3 naming.** `ChildApprovalGate.EARLY_DECISION_CAP`
  (`agents-plugin-pi/src/approval-protocol.ts`) now also caps the `consumed`
  list; the name says early decisions only.

## Open questions

- Whether the two known HOME-dependent agent-config tests and the
  `WS_CONFIG_HOME` leak above share one fix (a test-wide config isolation
  helper) worth doing together.

## Re-review additions (0a361491..4dcae29a, after the index repair)

Non-blocking minors from the re-review of the index repair
(`260924-bug-ticket-index-read-timeout-below-ssh-roundtrip`,
`260924-chore-pin-ticket-index-playbook-contracts`):

- **Move guard silent for the absence TTL.** On a never-seen clone with a
  read-timeout absence cached, `guardMoveClose` skips the owner check for up
  to the 10-minute TTL, so `tickets.move` of a stem a collaborator leased
  proceeds with no warning (the lease itself is untouched; the piggyback then
  discovers and registers). Close is protected by C2; move is not, by that
  ticket's scope. Before the repair the same skip lasted one call. Decide
  whether the piggyback should print the holder for a move too.
- **`clearAbsent` unpinned.** Removing the explicit clear in
  `wsindex/client.go` `sync` passes every test; the `storage_test.go`
  assertion passes through `markSynced` rewriting state. It only matters when
  discovery succeeds and the fetch then fails. Pin it with `failFetch` on a
  clone with absence cached, then assert the next Read discovers.
- **`index_init` adopt-error report print untested** (`mcp/ticket_index.go`
  and `wsindex/client.go` Create's adopt error path). Reachable only when the
  ref is deleted between Create's `ls-remote` and the adopt fetch; an
  `ixRunner` hook deleting the ref on first fetch would make it testable.
- **`--no-gpg-sign` pinned by outcome only.** Current git's `commit-tree`
  ignores `commit.gpgSign`, so `TestIndexCommitIgnoresSigningConfig` passes
  without the flag; assert the recorded `commit-tree` args instead.
- **GC-once claim inside the race test** (`mcp/ticket_index_test.go`,
  `TestB5GCRacesAcquire`) holds even if both racers ran GC; the within-period
  follow-up carries the rule. Trim the comment's claim.
- **Replay report prefix mix.** The replayed-takeover report in
  `wsindex/apply.go` is returned without the `ticket-index:` prefix while the
  conflict reports in the same slot carry it; `indexReportLine` normalizes
  both. Pick one convention.
- **`ticketsDir` copy.** `wsindex/origin.go` keeps `ticketsDir =
  "ai-docs/tickets"` beside wsdoc's `ticketIndexPrefix`; leftover from the F1
  consolidation.

## Release-gate additions (v0.46.19 fix-forward, 2d62d659..30b41a3f)

- **Permanent CAS reasons unpinned.** `TestPushStatusClassification`
  (`internal/wsindex/remote_test.go`) pins only `refname conflict` as
  `lost: false`. Three other permanent `ref_transaction_error_msg` reasons are
  not pinned:
  - `invalid new value provided`
  - `expected symref but found regular ref`
  - `reference conflict due to case-insensitive filesystem`

  A future `casLossPhrases` addition could start matching one of them with no
  failing test. Add all three as `lost: false` rows.
- **Hang transport copies drift in quoting.** The mcp copy
  (`internal/mcp/ticket_index_test.go` `ixCheckout.hang`) puts the live and
  stop paths inside `'%s'` as-is. The wsindex copy (`hangTransport`) uses
  `shellQuote`. The two are meant to stay in step. Use `shellQuote` in both,
  or share one helper.
- **POSIX stop file is not waited on.** On POSIX, `stopHangTransports` raises
  the stop file and returns at once, and the `TempDir` removal deletes it
  right after. A transport that somehow escaped the process-group kill would
  miss the stop file and run until the 120 s backstop. This is harmless while
  every remote call is deadline-bound through `ExecRunner`.
