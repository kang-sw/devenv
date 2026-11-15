---
title: Concurrent pooled worktree acquires can select the same checkout
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e11e43b2e45965a2
sage-review-completeness-reviewed: e11e43b2e45965a2
completed: 2026-11-15
---

# Concurrent pooled worktree acquires can select the same checkout

## Background

Observed during dogfooding on 2026-11-15: two independent `worktree.acquire`
requests ran in one parallel tool batch, both using profile `full` and base
`81382b093`, with different existing target branches. One returned a successful
lease for `decree-thinness-strum`; the other failed while switching branches in
that same checkout because its Git `index.lock` already existed. Independent
acquires should not provision or switch the same idle checkout concurrently.

The successful checkout was subsequently verified clean on its intended branch
and original head. A sequential retry of the second request selected another
checkout successfully. No lock was removed, no worker was dispatched into the
contended checkout, and no dirty work was discarded.

## Observed evidence

- First target: `impl/feat/unified-history-task3/flask-churn-crib`; successful
  lease returned `decree-thinness-strum`.
- Second target: `impl/develop/model813-label`; failure named
  `.git/worktrees/decree-thinness-strum/index.lock` and refused `git switch`.
- Read-only status after both calls: the first checkout remained clean at
  `45ed47acccf3d46bce68e66c6adebc611e5723c6` on its intended branch.
- Sequential retry returned `dynasty-gawk-emphatic` for the second branch.

These observations establish overlapping selection, not its locking root cause.

## Decisions

- Owner-approved FIX 6 scope (triage 1060, relayed 2026-11-15): prevent
  simultaneous acquire calls from selecting or using one pooled checkout.
  The source-grounded synchronization decision below implements that outcome.
- (1061) Technical settlement from fact population: serialize participating
  acquisitions with a cross-process lock keyed by the canonical Git common
  directory. Reuse `github.com/gofrs/flock` and the bounded acquire/deferred
  release pattern in `internal/mcp/session_layout.go`. Take the lock before
  the worktree-list snapshot and hold it through provisioning, key mint,
  lease publication and every failure-path checkout mutation. Use a bounded,
  context-aware wait following the existing lock-timeout convention; on
  timeout return a contention error before checkout mutation, never fall back
  to unlocked acquisition or remove a holder's lock. Hook execution may outlast
  a waiter's budget without releasing the holder's protection. Rejected:
  per-entry reservations, which also require creation coordination, locked
  eligibility revalidation and more failure cleanup. The coarser lock trades
  acquisition parallelism for the smallest complete safety fix. Unmodified
  old acquirers do not participate; no mixed-version race guarantee is made.
- Compatibility: **T0 / bug fix**, under `ai-docs/manuals/release-compatibility.md`. Existing callers keep
  the same `worktree.acquire` arguments and results. Current provisioning reads
  the Git worktree list and selects a clean detached candidate
  (`agents-plugin-tool/internal/mcp/worktree_tools.go:275-282,321-360`); the
  handler then mints the key and writes the existing lease
  (`agents-plugin-tool/internal/mcp/server.go:1450-1475`). Its schema-1 writer
  and reader are `worktree_lease.go:25-34,50-76,95-112` in that package.
  The planned new writer must preserve that lease shape and Git state so the
  current/old reader can still read it and a new reopen needs no recovery.
  No version refusal or shared-schema change is authorized. A mechanism that
  requires older binaries to be excluded is a compatibility blocker, not an
  implicit T3 approval; an unfixed old acquirer is not made race-safe by a new
  binary alone.

- Verification gate amendment (MAIN09:30:24, 2026-11-15): the whole offline Go
  suite is not a gate and must not be rerun. Focused touched-path checks are the
  gate (the 98.321 s affected-suite pass counts), plus targeted `-run` comparison
  of observed failures against the exact current develop HEAD after MAIN's
  synchronization. Only failures on the implementation head and not develop
  block. Known develop reds are exempt: `TestBoardStateFilterBrowser`,
  `TestWhiteboardEffectsBrowser`, `TestTrackerMockupI18nIsCurrent`,
  `TestRenderPageEmbedSizeBudget`, `TestBoardPanelFrameReuseBrowser`;
  `TestGitWakeReachesThePagesUnderItsRoot` is known flaky. Never run the entire
  `internal/wsdashboard` suite. Every future MCP test invocation must use
  `-timeout=30m`; every Go invocation keeps `GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local`. MAIN coordinates bounded comparisons and independent Lite;
  the worker pauses at the clean pre-review checkpoint without moving HEAD.

## Current source and historical evidence

- VERIFIED by source inspection: `provisionWorktree` reads eligibility and
  assigns the local `claim` before switching and hygiene; no exclusive claim
  is taken in that path (`worktree_tools.go:321-454`). Lease publication is
  later in dispatch (`server.go:1450-1475`). This supports the race hypothesis;
  it is not a new live reproduction.
- Historical duplicates corroborate distinct targets colliding on one idle
  checkout: `260923-bug-worktree-acquire-concurrent-pool-claim` (three full
  acquisitions), `261114-bug-ws-concurrent-worktree-acquire-pool-collision`
  (two housekeeping branches), and
  `261114-bug-ws-parallel-worktree-acquire-reuses-same-pool-entry` (two full
  acquisitions). Their terminal records remain unchanged. The housekeeping
  report also records a misleading held-branch hint after an index-lock
  failure; an index lock alone does not prove branch occupancy.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Preserve existing leases, dirty work, profile matching, quarantine, preserved
  artifacts, ticket-board visibility and checkout safety.
- Do not remove an unknown Git lock or steal an occupied branch as recovery.
- Cover selection through provisioning and key/lease publication, including
  failed provisioning, failed mint and failed lease-write cleanup. Do not
  release protection while another call can still mutate that checkout.
- Applicable implementation conventions: read
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/release-compatibility.md` before their covered source edits.
  Do not exercise the shared pool or replace the running daemon as part of
  implementation verification.

## Prior Decisions

- 261020-feat-worktree-profiles (2026-10-20, Decisions): "Profile = pool-partition key + lifecycle definition. A pooled worktree is reused only by an acquire naming the same profile." — bearing: constrains
- b76af960 (2026-11-10, commit): "Server.stateOptions carries Env so every wsstate.NewManager site in the server (notes, mailbox paths, generated paths, session layout) follows without a per-site change." — bearing: constrains
- 261114-bug-ws-concurrent-worktree-acquire-pool-collision (2026-11-14, Resolution): "Triage 2026-11-15 (owner (1057)): DROP - Duplicate ticket about concurrent worktree acquire conflicts; 261115-bug-ws-worktree-acquire-concurrent-pool-claim." — bearing: supports
- 261114-bug-ws-parallel-worktree-acquire-reuses-same-pool-entry (2026-11-14, Resolution): "Triage 2026-11-15 (owner (1057)): DROP - Duplicate ticket about concurrent worktree acquire conflicts; 261115-bug-ws-worktree-acquire-concurrent-pool-claim." — bearing: supports
- 260923-bug-worktree-acquire-concurrent-pool-claim (2026-09-23, Resolution): "Triage 2026-11-15 (owner (1057)): DROP - Duplicate ticket about concurrent worktree acquire conflicts; 261115-bug-ws-worktree-acquire-concurrent-pool-claim." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/worktree_tools.go and agents-plugin-tool/internal/mcp/server.go span selection and key/lease publication. |
| scope.surface | public-interface | worktree.acquire concurrency behavior changes; existing arguments and results must remain compatible (agents-plugin-tool/internal/mcp/server.go#L1418-L1489). |
| scope.new_public_symbol | no | No new tool or argument is authorized. |
| scope.new_type_contract | no | existing flock coordination is internal; no lease-schema, tool argument or shared-data contract changes. |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/worktree_tools_test.go#L190-L253; agents-plugin-tool/internal/mcp/worktree_lease_test.go#L14-L108; worktree_profile_acquire_test.go and worktree_hooks_test.go in that package. |
| complexity.reuse_points | confirmed | provisionWorktree and writeWorktreeLease were read; existing github.com/gofrs/flock dependency (agents-plugin-tool/go.mod#L7) and bounded lock/release pattern (agents-plugin-tool/internal/mcp/session_layout.go#L429-L450) are reused by decision 1061. |
| complexity.side_effect_risk | high | Selection leads to branch switch, reset/clean, hooks and rollback detach; protection must outlive all checkout mutations (agents-plugin-tool/internal/mcp/worktree_tools.go#L399-L493; agents-plugin-tool/internal/mcp/server.go#L1450-L1477). |
| risk.correctness | high | Releasing before failed-mint or failed-lease rollback can detach another successful caller's checkout (agents-plugin-tool/internal/mcp/server.go#L1459-L1477). |
| risk.fit | moderate | Cross-process coordination must span helper and handler without changing profile partitioning or failure semantics (agents-plugin-tool/internal/mcp/worktree_tools.go#L321-L360; agents-plugin-tool/internal/mcp/server.go#L1450-L1477). |
| risk.test | high | Phase 1 requires deterministic overlap across processes, two profiles and failure paths; existing idle-reuse tests are sequential (agents-plugin-tool/internal/mcp/worktree_tools_test.go#L190-L229). |
| risk.security_or_contract | moderate | Exclusive ownership and mixed-version compatibility are contractual; schema-1 lease replacement is not itself an exclusive claim (agents-plugin-tool/internal/mcp/worktree_lease.go#L22-L34, agents-plugin-tool/internal/mcp/worktree_lease.go#L50-L78; ai-docs/manuals/release-compatibility.md). |

## Phases

### Phase 1: Diagnose and prevent overlapping checkout selection

Using decision (1061), prevent overlapping checkout use across
idle selection, provisioning and lease publication. Independent same-profile
requests for distinct branches must complete with distinct exclusively held
checkouts, without transient index-lock collisions from sharing one candidate.

Regression verification: deterministically overlap acquisitions against one
eligible idle checkout for both `full` and `housekeeping`; assert distinct paths,
requested branches, worker keys and matching published leases. Cover separate
server/process callers, creation when no idle entry exists, dirty/branch-held/
leased candidates, profile mismatch, hook/quarantine failure and mint/lease-write
failure cleanup. Assert failure never detaches, cleans or overwrites a checkout
another successful acquire owns; a later safe acquire can progress. Retain the
existing worktree/profile/hook suites. Use bounded local temporary repositories,
not the shared pool. Run the affected `internal/mcp` tests and the project's
Level 1 Go suite (`cd agents-plugin-tool && go test ./...`); any runtime smoke
requiring a shared daemon is a separate lead-controlled step. No implementation
or test pass is claimed by this draft.

#### Pre-review checkpoint (47401fef9) - 2026-11-15

- Implemented the common-Git-directory flock with a bounded context-aware wait.
  `acquireWorktree` now owns the lock across the eligibility snapshot,
  provisioning, injected worker-key mint, schema-1 lease publication and all
  rollback mutations. Profile selection, hygiene, lease format and existing
  cleanup semantics remain unchanged. Older acquirers do not participate.
- Added temporary-repository process overlap tests for full/housekeeping, idle
  reuse/new creation and linked caller roots; regressions cover wait timeout and
  cancellation, slow hooks, quarantine, mint/lease-write/hidden-board failure
  cleanup, and preservation of dirty, branch-held/leased and other-profile
  candidates. A first fixture-only failure used macOS `/var` pool aliases;
  canonicalizing the fixture paths fixed it without changing production guards.
- VERIFIED commands from `agents-plugin-tool/`, all with
  `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`:
  - `go test ./internal/mcp -run '^TestWorktreeAcquire(ReleaseDispatch|WritesLeaseAndReleaseRemovesIt|FailedLeaseWriteDetaches|HousekeepingDispatch)$' -count=1`:
    exit 0, 10.339 s.
  - `go test ./internal/mcp -run '^TestWorktreeAcquire(ConcurrentProcesses|LockTimeoutAndCancellation|RollbackRetainsLock)$' -count=1 -timeout=150s`:
    first exit 1 on fixture path mismatch; after canonicalizing, exit 0, 46.455 s.
  - `go test ./internal/mcp -run '^(TestWorktreeAcquire|TestProvisionWorktree|TestReleaseWorktree|TestWorktreeHook|TestWorktreeHygiene)' -count=1 -timeout=300s`:
    exit 0, 98.321 s on the final implementation source.
  - `go test ./...`: exit 1; full output read. Four packages exhausted the
    default ten-minute package timer, and unrelated-looking tool-list,
    dashboard, holder, job-wait, agent-envelope and embed-budget checks failed.
    Attribution to baseline drift or concurrent load is INFERRED, not a
    reproduced baseline result. This broad command will not be rerun.
  - `gofmt` and `git diff --check`: exit 0.
- Complete failure names, source-ground attribution, command output and pending
  comparison boundary:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@c1381248/review-paths/b5e36ec5-01-pool-acquire-checkpoint-verification.md`.
  Full combined stdout/stderr:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@c1381248/review-paths/b5e36ec5-01-pool-acquire-full-suite.log`
  (SHA-256 `4642b4c93b78e3b06c0b8601ab2e45e0d41fb4f78cc7dc9c3a9cd88e2e95bda2`).
- Tooling follow-up, not a ticket-scope permission change: the sole
  `playbook.render(name: "delegate-implementer", tier_override: "medium",
  context: {plan_stem: "pool-acquire-eligibility"})` call failed with
  `ws-pi-agent: child capability exceeds parent ceiling
  (ws__session_request_hire, ws__session_roster)`. No child was spawned, no
  permission was altered, and the render was not retried.
- Not a phase Result or completion claim. MAIN's latest-develop sync and bounded
  targeted failure comparisons remain pending. One fresh independent
  `code-review-lite` review of the Git diff (correctness and test integrity)
  follows those steps, with no self-review substitute or second sweep. No
  shared-pool/runtime smoke, Windows runtime check or race-detector check ran.

### Result (47401fef9) - 2026-11-15

Implemented decision (1061): a symlink-resolved common-Git-directory flock
serializes participating acquisitions before their eligibility snapshots and
through provisioning, canonical worker-key binding, lease publication and
failure rollback. Its context-aware two-second wait fails before checkout
mutation; a timeout never unlinks the lock or cancels the holder's hooks. Tool
arguments/results, profile partitioning and schema-1 leases remain compatible
(T0); old acquirers remain outside the synchronization guarantee.

VERIFIED: The affected acquisition/profile/hook suite passed in 98.321 s with
exit 0, using the exact focused command recorded above. MAIN explicitly retained
that evidence under the amended gate. MAIN synchronized develop
`dedb8a1e5c28b9efd78460c52b0f00a78458d646` into this branch without conflicts at
`014a7d701a4acda78dac15c39a7ebbd63f69320d`. Subsequent work is ticket-only;
no additional source/test changes or broad test runs were made.

Independent one-pass Lite reviewed the exact synchronized range
`dedb8a1e5c28b9efd78460c52b0f00a78458d646..014a7d701a4acda78dac15c39a7ebbd63f69320d`.
No Critical or Important findings; remaining none. The complete report was read:
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/f7de7afb-01-fix6-pool-lite.md`.
No fixes or second review were required.

#### Targeted comparison matrix

MAIN's independent diagnostic worker compared the exact pinned develop head
`dedb8a1e5c28b9efd78460c52b0f00a78458d646` in
`/Users/kang-sw/.ws-worktrees/devenv/bountiful-dress-energy` with synchronized
implementation head `014a7d701a4acda78dac15c39a7ebbd63f69320d` in this worktree.
Both heads and clean worktrees were checked before and after. The complete
report `/tmp/fix6-targeted-comparison.md` and all 26 raw logs were read. Commands
ran serially, once per row per head, from each checkout's `agents-plugin-tool/`:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test <package> -run '<selector>' -count=1 -timeout=<budget> > /tmp/fix6-<baseline|impl>-<stem>.log 2>&1
```

| Stem | Package | Exact selector | Budget | Develop result | Implementation result |
|---|---|---|---|---|---|
| capabilities | `./cmd/ws-mcp` | `^TestRuntimeCapabilitiesCommandReports(Launcher\|Wsflow)ContractSurface$` | `20s` | PASS, 5.570 s | PASS, 6.234 s |
| execjob | `./internal/execjob` | `^TestResultWithContextEndsAWaitWhenTheContextEnds$` | `20s` | PASS, 8.790 s | PASS, 9.675 s |
| dashboard | `./internal/wsdashboard` | `^Test(DevAssetsIDFollowsTheServedFiles\|ToolbarPageActionHooks\|DevAssetsOffServesTheEmbeddedPages\|DevAssetsShowEditsWithoutARestart\|DevAssetsReportABrokenTemplateAndRecover\|DevAssetsRequireSourceFragments\|LabelRouteSavesShowsAndResets\|SelectionTextTransportLifecycle\|GearHoldsTheThemeSection)$` | `20s` | FAIL, 7.386 s | FAIL, 9.797 s |
| mcp | `./internal/mcp` | `^TestImplIdentityResolveProvisionRouteIntegration$` | `30m` | PASS, 6.142 s | PASS, 3.349 s |
| wsagent-assertions | `./internal/wsagent` | `^Test(TokenSpawnRecordsTheHolderAsParent\|CommonContractLifecycle\|RESTRoutesEndToEnd\|PiLeadCommands)$` | `20s` | FAIL, 1.267 s | FAIL, 1.277 s |
| wsagent-launch | `./internal/wsagent` | `^Test(ClaudeLeadRejoinIgnoresChangedPluginLaunchInputs\|ClaudeLeadLaunchAsksForCommands)$` | `20s` | FAIL, 15.787 s | FAIL, 15.825 s |
| wsagent-drain | `./internal/wsagent` | `^Test(ClaudeLeadCommandSplitsTheDrain\|LeadQueuedInputDropsWithTheConversation\|LeadStopKeepsTheQueueWithoutARestart)$` | `20s` | FAIL, 15.776 s | FAIL, 15.888 s |
| wsagent-handoff | `./internal/wsagent` | `^Test(LeadRowTakenWhileNoDaemonRan\|LeadRowOfAnEarlierDaemonIsNotATerminals\|LeadControlRoutesTakeTheTrustedGate)$` | `20s` | FAIL, 15.730 s | FAIL, 16.033 s |
| wsagent-daemon-control | `./internal/wsagent` | `^TestLeadControlRoutesThroughTheDaemonsGate$` | `20s` | FAIL, 5.740 s | FAIL, 5.972 s |
| wsagent-isolation | `./internal/wsagent` | `^TestLeadIsolationRequestAndRelaunch$/^pi$/^default$` | `20s` | PASS, 0.664 s | PASS, 0.895 s |
| holder | `./internal/wsholder` | `^TestAConsumersReplayIsNoLoss$` | `20s` | FAIL, 15.715 s | FAIL, 15.568 s |
| ledger | `./internal/wsledger` | `^TestRemoteSmokeRegisterSyncFetch$` | `20s` | Diagnostic timeout, 20.490 s | PASS, 18.718 s |
| trackerhost | `./internal/wstrackerhost` | `^TestIdlePushWithoutDaemon$` | `20s` | PASS, 11.796 s | PASS, 13.144 s |

VERIFIED: Among 30 selected top-level tests (including the selected isolation
subtest), 17 failed on both heads, 12 passed on both, and the ledger control
reached only its baseline's diagnostic budget while implementation passed. No
implementation-only failure was observed. The shared failures are:

- Dashboard: `TestToolbarPageActionHooks`,
  `TestLabelRouteSavesShowsAndResets`, `TestGearHoldsTheThemeSection`.
- wsagent: all 13 top-level names selected by the five failing wsagent rows;
  the rejoin test fails in both `malformed_settings` and `missing_bundle`, and
  the common-contract test fails for both `claude` and `pi`.
- Holder: `TestAConsumersReplayIsNoLoss`.

The six other selected dashboard names pass on both synchronized heads. MCP
integration, tracker-host idle push and isolated `pi/default` also pass on both;
the original package-wide ten-minute timers were not standalone assertion
failures for those tests. The baseline ledger's 20 s timeout occurs during
`Register` at `remote_smoke_test.go:68`, not the original full-run `syncOK`
point at line 47. It does not establish failure at the normal ten-minute budget.
Shared reproduction establishes baseline failures, not their timing root cause.

#### Gate settlement and limits

MAIN confirmed the revised focused-plus-comparison gate satisfied and authorized
Result/closure after reading the full comparison and clean independent Lite.
The historical whole-suite command remains **FAIL**, never PASS, and was not
rerun. MAIN-exempt `TestTrackerMockupI18nIsCurrent` and
`TestRenderPageEmbedSizeBudget` were deliberately not repeated. All other
pool-lane failure names and original timer cases were covered; the diagnostic
worker received no occupied-lane inventory, so no cross-lane coverage is claimed.

Omitted: shared-runtime/pool smoke, Windows runtime verification, race detector,
separate in-flight cancellation and symlink-alias regression cases, historical
`d17d335cb` reproduction, and a normal-budget baseline ledger retry. These are
explicit evidence limits, not implementation findings. No resource-exhaustion
or full-suite-pass claim is made. The earlier delegation capability-ceiling
refusal remains a reported tooling follow-up for MAIN; no permission changes,
unrelated repairs, additional review or branch integration were performed.
