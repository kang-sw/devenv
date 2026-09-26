---
title: Review-sweep test and naming minors (Pi usage rollup, worktree.list, config isolation)
related:
  260924-bug-ticket-index-read-timeout-below-ssh-roundtrip: the same sweep's release-blocking index fixes; these minors were left out as non-blocking
  260925-bug-ticket-index-move-guard-silent-during-absence-ttl: the move-guard finding, split out because it changes caller-visible behavior
  260924-chore-pin-ticket-index-playbook-contracts: source of part of the re-review additions
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 55132ddf9f774b4e
sage-review-completeness-reviewed: 55132ddf9f774b4e
completed: 2026-09-25
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

- **Config isolation, observed scope (2026-09-25 v0.46.20 ship pre-flight).**
  With a host `~/.ws/config.json` that overrides the `codex` and `pi` agent
  tiers (`gpt-6-sol`), `go test ./...` fails five tests, not two:
  `internal/mcp` `TestPlaybookPrintModelAliasPiHarnessFallsBackToDefault`
  and `TestServeStdioConfigResolveAgentFallsBackToDefault`; `internal/wsconfig`
  `TestSetAgentsTierDoesNotOverwriteOtherBackendAliasMappings`,
  `TestResolveAgentExplicitBackendDoesNotBorrowCrossBackendModel`, and
  `TestResolveAgentTierForHarnessFallsBackToDefault`. The same suite passes
  with `HOME` pointed at an empty temp directory.

## Re-review additions (0a361491..4dcae29a, after the index repair)

Non-blocking minors from the re-review of the index repair
(`260924-bug-ticket-index-read-timeout-below-ssh-roundtrip`,
`260924-chore-pin-ticket-index-playbook-contracts`):

- **Move guard silent for the absence TTL.** Split out to
  `260925-bug-ticket-index-move-guard-silent-during-absence-ttl`; out of
  scope here.
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

## Decisions

- **Config isolation is a per-package `TestMain` that points
  `WS_CONFIG_HOME` at an empty temp directory.** `internal/mcp` extends its
  existing `runTestMain`; `internal/wsconfig` gains a new `TestMain`. This
  covers the five agent-config tests listed under "Config isolation, observed
  scope", `TestWorktreeListIncludesLegacyInTreePoolUnderDefaultConfig`, and the
  `git_merge_test.go` `legacy-pool-holder` dispatch subtest.
  - Rejected: fixing each test on its own, which leaves the next
    default-dependent test exposed.
  - Rejected: a shared test-support package. Go cannot share `_test.go`
    helpers across packages, and ca734c1d already declined a new support
    package for the hang helper.
- **The replay report takes the `ticket-index:` prefix.** The
  replayed-takeover report in `wsindex/apply.go` carries the same
  `ticket-index:` prefix as the conflict reports in the same slot.
  - Rejected: dropping the prefix from the conflict reports.
- **Rollup "telemetry reset on a revived record" test seeds an older
  in-memory value** so it exercises the `descendantUsageOrder` gate and fails
  when the gate is removed.
  - Rejected: retitling it to what it checks today.
- **The "parent restart" integration test is retitled and commented as a
  persistence round-trip.** Unit tests already cover `restoreDescendantUsage`
  on the revival path.
  - Rejected: routing it through `reviveOrphans`/`rehydrateForkRecord`.
- **The mcp hang transport quotes its paths with a local copy of
  `shellQuote`**, keeping the per-package copies ca734c1d chose.
  - Rejected: exporting `shellQuote` from `wsindex` for test use.
- **The POSIX stop-file finding gets a code comment only**: process-group
  SIGKILL in `proc_unix.go` makes a POSIX wait unnecessary, as ca734c1d
  recorded. No behavior change.
  - Rejected: adding a POSIX wait.
- **`wsindex/origin.go`'s `ticketsDir` is removed in favor of wsdoc's
  `ticketIndexPrefix`**, exported for that use. `ticketsDirPrefix` (with the
  trailing slash) is left alone.
  - Rejected: keeping the duplicate.
- **`TestIndexCommitIgnoresSigningConfig` also asserts the recorded
  `commit-tree` arguments include `--no-gpg-sign`.** This overrides
  7c16252a's earlier acceptance of outcome-only pinning: current git ignores
  `commit.gpgSign` for `commit-tree`, so only the argument assertion guards
  against a git that starts honoring it.
  - Rejected: dropping the item and keeping outcome-only pinning.
- **The move-guard finding is out of scope** (split out, see `related:`).

## Constraints

- `go test ./...` in `agents-plugin-tool/` passes with the host's real
  `HOME`, including a host `~/.ws/config.json` that overrides agent tiers or
  `worktree_pool`.
- Production behavior does not change, except the replay report prefix and
  the `EARLY_DECISION_CAP` rename.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- ca734c1d (2026-09-25, commit): "The two packages keep separate copies (as before) with a keep-in-step note; a shared test-support package was not worth a new package. ... on POSIX proc_unix.go SIGKILLs git's process group" — bearing: contradiction-candidate
- 7c16252a (2026-09-25, commit): "Two declined Minors (callRaw framing duplication; signing test checks outcome only because current git commit-tree ignores commit.gpgSign, so the store.go comment was corrected)" — bearing: contradiction-candidate
- 547c08d4 (2026-09-25, commit): "The other ref_transaction_error_msg() reasons (refname conflict, invalid new value, expected symref, case conflict) are permanent and stay pushRefused." — bearing: supports
- 57c930bb (2026-09-24, commit): "commit-tree runs with --no-gpg-sign so a user's signing config cannot prompt on or fail index plumbing commits." — bearing: constrains
- 260924-chore-pin-ticket-index-playbook-contracts (2026-09-25, Result): "`go test ./... -count=1` in `agents-plugin-tool` with an isolated HOME: all packages ok" — bearing: supports
- 260924-bug-pi-review-sweep-correctness-fixes (2026-09-24, commit 99f8ac90): "Accepted minors left open: ... EARLY_DECISION_CAP naming, unguarded symlinkSync in other test files." — bearing: supports
- 5b410a54 (2026-09-24, commit): "Ticket Decision 3. The sibling field removes the coupling that made the usage-rollup Result keep a telemetry-less child's value in memory only." — bearing: constrains
- 24898ba4 (2026-09-24, commit): "Push rejections classified as a CAS loss (stale info, non-fast-forward, cannot lock / failed to update ref) retry against the fresh tip up to MaxAttempts; any other refusal fails immediately." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/test/agent-usage-rollup.test.ts, agents-plugin-pi/test/agent-usage-rollup.integration.test.ts, agents-plugin-pi/src/approval-protocol.ts, agents-plugin-tool/internal/mcp/worktree_list_test.go, agents-plugin-tool/internal/mcp/git_merge_test.go, agents-plugin-tool/internal/mcp/server_test.go, agents-plugin-tool/internal/mcp/playbook_tools_test.go, agents-plugin-tool/internal/mcp/ticket_index_test.go, agents-plugin-tool/internal/wsconfig/config_test.go, agents-plugin-tool/internal/wsindex/apply.go, agents-plugin-tool/internal/wsindex/origin.go, agents-plugin-tool/internal/wsindex/remote_test.go, agents-plugin-tool/internal/wsindex/storage_test.go |
| scope.surface | cross-module | spans agents-plugin-pi and agents-plugin-tool packages mcp, wsconfig, wsindex, wsdoc; production edits are the replay report text in wsindex/apply.go#L152 and the ChildApprovalGate.EARLY_DECISION_CAP rename at approval-protocol.ts#L138 |
| scope.new_public_symbol | yes | renamed ChildApprovalGate static cap replacing EARLY_DECISION_CAP; consolidating wsindex/origin.go#L11 ticketsDir needs an exported wsdoc constant since ticketIndexPrefix is unexported at wsdoc/tickets_scope.go#L43 |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | worktree_list_test.go, git_merge_test.go, server_test.go TestMain at L35-53, config_test.go, remote_test.go TestPushStatusClassification, storage_test.go, ticket_index_test.go, agent-usage-rollup tests; wsconfig has no TestMain yet |
| complexity.reuse_points | confirmed | mcp runTestMain server_test.go#L42-53, ixRunner.failFetch ticket_index_test.go#L114, wsindex countingRunner before hook harness_test.go#L57-87, wsindex shellQuote git.go#L202 unexported, indexReportLine mcp/ticket_index.go#L94-99 |
| complexity.side_effect_risk | moderate | a package-wide WS_CONFIG_HOME default in TestMain changes the environment of every test in mcp and wsconfig, including tests that set their own config home |
| risk.correctness | low | production change limited to a report prefix that indexReportLine already normalizes at mcp/ticket_index.go#L94-99 and a constant rename with three src/test call sites |
| risk.fit | low | per-package TestMain isolation and a local shellQuote copy keep ca734c1d's per-package posture; ticketsDir consolidation needs a wsdoc export (wsindex already imports wsdoc) |
| risk.test | high | many new pins must be mutation-verified across Go and TS; hang-transport and GC race tests are timing-sensitive and Windows behavior is CI-only |
| risk.security_or_contract | low | no MCP schema or protocol change; the replay report text is the only caller-visible string and mcp output already carries the prefix |

## Phases

### Phase 1: Land the review-sweep minors

Resolve every item under Background, "Re-review additions", and
"Release-gate additions" except the split-out move-guard finding, following
the Decisions above.

Implementer's discretion, within the Decisions:

- New identifier names: the replacement for `EARLY_DECISION_CAP` (it caps
  both the early-decision map and the `consumed` list) and the exported wsdoc
  constant that replaces `ticketIndexPrefix`.
- The concrete assertions for the untested `worktree.list` behaviors and the
  independent expected text for the effort-text test.
- Which layer pins `clearAbsent` (`wsindex` storage tests or the mcp
  `ixRunner` harness with `failFetch`), provided removing the explicit clear
  in `wsindex/client.go` `sync` fails the pin.

Scope details:

- The mcp hang helper's local `shellQuote` copy also quotes its
  `core.sshCommand` value, which wraps the path in raw quotes today, so the
  mcp and wsindex copies stay in step.
- The POSIX stop-file comment goes on both hang-transport copies (wsindex
  `stopHangTransports` and mcp `ixCheckout.hang`).

Verification:

- `go test ./...` in `agents-plugin-tool/` passes in three runs: with the
  real `HOME`; with `HOME` pointed at an empty temp directory; and with `HOME`
  pointed at a temp directory whose `.ws/config.json` overrides both agent
  tiers and `worktree_pool`. The real `~/.ws/config.json` is never edited.
- `npm test` in `agents-plugin-pi/` passes.
- Each of these newly pinned behaviors fails when the pinned code is mutated
  away: the `descendantUsageOrder` gate, `clearAbsent`, the `--no-gpg-sign`
  argument, and the permanent CAS reasons.

### Result (619cb283e) - 2026-09-25

Landed every item except the split-out move-guard finding, in
340889a15..619cb283e.

- **Config isolation.** `internal/mcp` `runTestMain`, a new
  `internal/wsconfig/main_test.go` `TestMain`, and (after round-1 review)
  `cmd/ws-mcp` `TestMain` set `WS_CONFIG_HOME` to an empty temp dir
  unconditionally. `cmd/ws-mcp` `envValue` now returns the last env entry,
  matching os/exec, so the mailbox seeding helpers use the subprocess's store.
- **Pi.** The revived-record rollup test seeds an older in-memory
  `descendantUsage`; the "parent restart" integration test is retitled as a
  persistence round-trip; `EARLY_DECISION_CAP` is now `RETAINED_DECISION_CAP`.
- **wsindex/wsdoc.** New `TestDiscoveryClearsAbsenceWhenFetchFails` pins
  `clearAbsent`; `TestIndexCommitIgnoresSigningConfig` asserts the recorded
  `commit-tree` args carry `--no-gpg-sign`; `TestPushStatusClassification`
  gains the three permanent reasons as `lost: false`; the replayed-takeover
  report carries `ticket-index:`; `ticketsDir` is replaced by the exported
  `wsdoc.TicketsDir`; `stopHangTransports` documents the POSIX no-wait.
- **mcp.** New worktree.list tests (prunable skip, empty-pool text, unreadable
  lease warning); the effort-text test uses independent literal expectations
  plus a 4-line check; `TestIndexInitAdoptErrorPrintsDiscardReport` drives the
  adopt error path through an `ixRunner` `beforeRemote` hook; the
  `TestB5GCRacesAcquire` comment no longer claims GC ran once; the hang
  transport shell-quotes its paths and `core.sshCommand` with a local
  `shellQuote` copy and carries the POSIX stop-file comment.

Verification:

- `go test ./... -count=1` in `agents-plugin-tool`: all 16 packages ok with the
  real `HOME`, with an empty temp `HOME`, and with a temp `HOME` whose
  `.ws/config.json` overrides `agents.tiers`, every `model_aliases` bucket
  (codex, pi, claude, default), and `worktree_pool`.
- `npm test` in `agents-plugin-pi`: 1862 tests, 1860 pass, 0 fail, 2 skipped.
- Mutation-verified (each pin fails with its target removed, confirmed by
  both the implementer and the test reviewer): the `descendantUsageOrder`
  gate, `clearAbsent`, `--no-gpg-sign`, each permanent CAS reason, plus the new
  worktree.list, effort-text, adopt-error, and replay-prefix pins.

Decisions:

- `WS_CONFIG_HOME` isolation also covers `cmd/ws-mcp`: round-1 correctness
  review found `TestConfigCLICommandsReturnConfigView` fails under a host
  config that overrides `agents.tiers`, which the Constraint covers.
- The TestMain override is unconditional, unlike the `WS_CACHE_HOME` default,
  because an inherited `WS_CONFIG_HOME` is a real config too.
- Names: `RETAINED_DECISION_CAP` and `wsdoc.TicketsDir`; `clearAbsent` is
  pinned at the wsindex layer.
