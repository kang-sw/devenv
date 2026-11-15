---
title: "tickets.move stages a ticket move into a worker's impl/* branch on an occupied root"
blocked-by: 261008-bug-ws-git-merge-switches-worker-occupied-root
related:
  261008-bug-ws-git-merge-switches-worker-occupied-root: "prerequisite; reuse its occupied-root guard"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 138a59b788b2e322
sage-review-completeness-reviewed: 138a59b788b2e322
completed: 2026-11-16
---

# tickets.move stages a ticket move into a worker's impl/* branch on an occupied root

## Background

During lead-ticket promotion (2026-10-30), a lead session authored an
untracked ticket in the root checkout while it was on `develop`. A parallel
worker then switched the root to `impl/develop/cacti-frill-lance`. The lead's
`ws/tickets.move(stem, to: "ready")` succeeded and staged
`ai-docs/tickets/ready/<stem>.md` into that worker branch's index, with no
refusal or warning. Only the follow-up `ws/git.commit(expected_branch:
"develop")` caught the mismatch and pointed at the housekeeping worktree. The
lead had to unstage the file by hand before the worker could commit it.

## Original expectation

The original capture suggested occupied-root detection for `tickets.move` and
possibly `tickets.create_empty` and other ticket writers, by refusal or warning.
A staged write landing in a worker's index is the costly failure, because the
worker's next commit can absorb it silently. The approval below settles refusal
for `tickets.move` only; the broader writer suggestion is not implementation
authority.

## Decisions

- Owner-approved FIX 6 scope (triage 1060, relayed 2026-11-15): prevent
  `tickets.move` from staging into a worker-owned index. Refuse before
  frontmatter, filesystem or index mutation; a warning after staging is not
  sufficient. Keep a diagnostic naming the observed worker branch and the
  housekeeping-worktree route, without inventing a holder identity.
- (1062) Technical settlement: consume the prerequisite's symbolic-HEAD and
  existing caller-parent predicate, before any possible posture/frontmatter or
  index write (pending 261008-bug-ws-git-merge-switches-worker-occupied-root). Reject mandatory lease proof and a new branch-assertion API:
  neither is needed to preserve the existing worker/lead distinction, and
  current leases do not identify an occupying branch.
- Hard prerequisite: `261008-bug-ws-git-merge-switches-worker-occupied-root`
  must land first. Serialize implementation and reuse its guard; do not ship
  a separate occupancy predicate.
- Compatibility: **T0 / bug fix**, under
  `ai-docs/manuals/release-compatibility.md`. Old calls retain the same
  `tickets.move` arguments and permitted moves. Current dispatch invokes
  `TicketsMove` after its ticket-ownership guard
  (`agents-plugin-tool/internal/mcp/server.go:1986-2010`); the existing writer
  may change frontmatter before `git add` and `git mv`
  (`agents-plugin-tool/internal/wsdoc/tickets_mutate.go:249-298,929-946`).
  The new guard must refuse only the unsafe caller/root case before those
  writes, leaving the same ticket Markdown and Git index formats for allowed
  moves. Current/old ticket readers and a new reopen still see the original
  or normally moved ticket; no recovery path is needed. No new shared schema,
  version gate or required argument is authorized; any contrary requirement
  is a blocker rather than implicit T3 approval.

## Current source

- VERIFIED: `TicketsMove` has category, dependency, scope and review checks but
  no occupied-root check before its first possible frontmatter write and
  `scopedGitMove` (`tickets_mutate.go:171-298`). `atomicGitMove` stages before
  moving (`tickets_mutate.go:929-946`).
- VERIFIED: `git.commit` catches a remembered/actual branch mismatch before
  staging (`agents-plugin-tool/internal/wsgit/git.go:634-657`), too late to
  protect an earlier `tickets.move`. It is not a blanket impl-branch guard.
- VERIFIED: the occupied-root announcement deliberately distinguishes a
  top-level lead from a worker's own context by session parent
  (`agents-plugin-tool/internal/mcp/occupied_root_announcement.go:10-24`).
  The shared predicate is settled in the prerequisite's decision (1062):
  parent-less callers on an impl checkout are refused, while parented callers
  retain their session-bound worker flow. This is not stronger branch-ownership
  proof; missing leases remain supported, and no new required argument or
  arbitrary external-Git exclusion guarantee is introduced (pending
  261008-bug-ws-git-merge-switches-worker-occupied-root).
- VERIFIED: caller context already reaches MCP: `resolveToolRoot` resolves the
  required session key to its bound root, and session entries retain `parent`
  (`agents-plugin-tool/internal/mcp/server.go#L4135-L4148`,
  `agents-plugin-tool/internal/mcp/session_auth.go#L17-L32`). The move handler
  currently passes only Env, TicketStem, To and SageReview into `TicketsMove`,
  not parent context (`agents-plugin-tool/internal/mcp/server.go#L1987-L2011`).
  Its existing ownership guard permits absent views/leases
  (`agents-plugin-tool/internal/mcp/ticket_index_view.go#L291-L303`); it is not
  an occupied-branch guard. The new check must precede any index-refreshing
  status call as well as the posture writer, not merely `git mv`; the current
  posture helper persists frontmatter before staging
  (`agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L249-L298`,
  `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L583-L615`). Existing
  caller-bound `s.gitRunner()` and `s.environ()` are available at dispatch;
  no wire argument is needed to obtain the existing root/parent context.

## Constraints

- Limit implementation to `tickets.move` plus reuse/tests of the prerequisite
  guard. Do not expand to create, close, stamp, other ticket writers, or a new
  branch-assertion API without separate approval.
- Preserve authorized worker ticket operations in their own checkout, existing
  scope/ownership/review/dependency gates and normal move staging. A refusal
  must not write frontmatter, create a destination directory or alter the index.
- Applicable implementation conventions: read
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/release-compatibility.md`. If embedded conventions change,
  also read `ai-docs/manuals/skill-authoring.md`; this contract does not require
  changing them.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 261008-bug-ws-git-merge-switches-worker-occupied-root (2026-10-08, Decisions): "Land this ticket first and serialize the two implementations; the consumer must reuse the landed guard rather than invent a second occupancy policy." — bearing: constrains
- 260918-feat-ws-sparse-housekeeping-worktree-occupied-root (2026-09-18, Decisions): "Release-before-merge ordering is a stated rule." — bearing: constrains
- 0520bc61 (2026-10-30, commit): "Committed from a housekeeping worktree because the root checkout is occupied by another worker's impl/develop/cacti-frill-lance branch." — bearing: supports
- cf7dc36f (2026-10-30, commit): "tickets.move staged a ready move into another worker's impl/* index on the root with no refusal; only git.commit's expected_branch guard caught it." — bearing: supports
- 7c3a8c54 (2026-11-10, commit): "free functions take a minimal parameter rather than a Server" — bearing: constrains
- dc532266 (2026-09-10, commit): "Preserve the existing ready posture/freshness contract and epic design-only settlement; historical metadata remains readable and todo moves preserve it unchanged." — bearing: constrains
- 260806-feat-worktree-ticket-scope (2026-08-06, commit): "Both new write-then-reject returns are now gated on an active scope AND on the file really differing from its pre-call bytes" — bearing: constrains
- 260720-wsdoc-commit-boundary (2026-11-10, commit): "wsdoc may not import wsgit" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, occupied_root_announcement.go, internal/wsdoc/tickets_mutate.go; MCP and wsdoc regressions |
| scope.surface | public-interface | tickets.move refusal behavior; existing wire arguments retained (agents-plugin-tool/internal/mcp/server.go#L1987-L2011) |
| scope.new_public_symbol | no | no new tool, argument or exported symbol authorized; existing session context suffices |
| scope.new_type_contract | no | no new wire/shared-data contract; internal caller plumbing only |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/tickets_mutate_test.go#L233-L261; agents-plugin-tool/internal/mcp/ticket_index_view_test.go#L442-L489; tickets_scope_test.go |
| complexity.reuse_points | unconfirmed | symbolic HEAD and parent mechanism read in agents-plugin-tool/internal/mcp/occupied_root_announcement.go#L10-L39; reusable enforcement guard pending 261008-bug-ws-git-merge-switches-worker-occupied-root |
| complexity.side_effect_risk | high | existing upward moves can persist posture before staging; refusal must leave source bytes, destination and index unchanged |
| risk.correctness | high | guard ordering must protect tracked/untracked files and pre-populated worker index without blocking parented workers |
| risk.fit | moderate | shared prerequisite predicate must integrate with caller-bound MCP context, not a second wsdoc ownership policy |
| risk.test | high | existing move/lease tests do not cover occupied-root refusal with byte-identical index and parented worker acceptance |
| risk.security_or_contract | high | preserve legacy allowed calls, scope/lease/review gates and worker flows; no required argument or stronger ownership proof authorized |

## Phases

### Phase 1: Guard ticket moves before any occupied-root write

After the prerequisite lands, apply its settled guard to the move path
before any mutation. A lead whose root is now worker-owned must receive an
unchanged-state refusal and housekeeping guidance rather than staging into
that index. Keep the worker's legitimate own-root move behavior.

Regression verification: simulate the lead's root changing from develop to a
worker impl branch before `tickets.move`; cover tracked and untracked tickets,
upward moves with possible posture writes, downward moves and a pre-populated
worker index. Assert identical source bytes, HEAD and index, absent destination,
and no add/mv/frontmatter write on refusal. Cover authorized worker moves and
unoccupied-root normal moves through MCP; retain existing category, dependency,
sparse-scope, review and move-atomicity tests. Run affected `internal/mcp` and
`internal/wsdoc` tests and `cd agents-plugin-tool && go test ./...` using only
local temporary repositories. This draft records no implementation or pass.

### Result (49f1994d6) - 2026-11-15

- Reused the prerequisite's `guardOccupiedRoot` in the MCP `tickets.move`
  dispatcher before sage configuration and ownership refresh, then re-observed
  HEAD immediately before the wsdoc writer. Parent-less callers receive the
  existing branch-specific housekeeping refusal; a nonempty session parent
  retains the authorized worker flow, including absent parent records and
  leases. No occupant identity is inferred, and arbitrary external Git
  processes are not serialized.
- Regression coverage landed in `cbec661bf`, with guard integration and both
  inspection-failure boundaries in `49f1994d6`. MCP regressions snapshot all
  working-tree and Git-admin file bytes across tracked/untracked upward and
  downward refusals, pre-populated worker indexes, rootless branches and
  missing/stale leases. They check absent destination directories, normal
  staged moves, ready posture normalization, parented lead/delegate callers,
  detached/non-impl checkouts, config/ownership ordering and the caller-local
  Git environment. The leaf's initial shim failure was a test-setup error
  (omitting Git's `-C <root>` prefix); only argument matching was corrected
  before its scoped run passed. No implementation was changed to fit a test.
- VERIFIED from `agents-plugin-tool/`, with complete command outputs read:
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/mcp -run '^Test(GuardOccupiedRoot|OccupiedRootAnnouncement|TicketsMoveOccupiedRoot|MoveCloseGuard$|IndexAbsentVerbsAnswerPlainOK$|ServeStdioTicketsMove|ServeStdioOpenDecisionQueueRefusals$)' -count=1 -timeout=4m`
    passed (`ok`, 19.161s).
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdoc -run '^Test(TicketsMove|ScopeBlockedMutationIsANoOpOnSageBearingTicket$|GitMoveFailureCarriesNoNoticeWithoutAScope$|ScopedGitMoveFailureNoticeAssertsOnlyThisCallsWrites$|TicketScopeGateIsInertWithoutSparseCheckout$|BlockedByPromotion|SageGateReady|TicketVerifyReadySagePostureGuardrail$)' -count=1 -timeout=4m`
    passed (`ok`, 2.199s).
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/mcp -run '^TestRepoScope(AppliesAtTicketMutatorsAndWorktreePool|MalformedFileFailsLoud)$/(move|tickets.move)$' -count=1 -timeout=4m`
    passed (`ok`, 1.103s).
  - `git diff --check` and `gofmt -l` on both changed Go files were clean.
- Independent one-pass Lite review of
  `76d8f7971a5c3bb3dcad7d5cad63e58ede00b0ab..49f1994d6fb81b034a5a4e89b13340cdf12937fb`
  was clean, with no findings. The fresh reviewer inspected the complete diff,
  contracts, guard and regressions; it did not execute tests. Full report:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@d3adad41/review-paths/424d92e0-01-tickets-move-occupied-lite.md`.
- Decisions: enforcement stays at the MCP session boundary, not in wsdoc,
  because it already owns root, parent and caller-bound Git context. This
  avoids a second occupancy predicate and preserves the dependency direction.
  T0 remains intact: `server.go:1999-2031` adds only the approved unsafe-case
  refusal around the unchanged options; `wsdoc/tickets_mutate.go:171-298,929-946`
  still writes ordinary ticket Markdown and Git index entries. No wire
  argument, required lease, shared schema or version gate changed; reopening
  either version reads the same original or normally moved ticket.
- Omitted by MAIN's explicit dispatch restriction, superseding the phase's
  broad-suite request: full `internal/mcp`, `internal/wsdashboard`, and
  repository/module suites, including `go test ./...`. Only offline scoped
  `-run` commands against local temporary repositories were authorized and
  run. The POSIX inspection shim is skipped on Windows; no Windows, live
  runtime, daemon, CI, install/download, push or merge verification was run.
  Unresolved: none under this verification boundary.


## Resolution (2026-11-16)

Completed the approved T0 tickets.move guard reuse, scoped offline regressions, and clean independent Lite review. Ticket verification passed with a Sage freshness warning; accepted because the diff adds only implementation/verification evidence and does not alter the approved decisions or phase plan. MAIN's explicit scoped-only verification boundary supersedes the phase's full-suite request. Implementation branch is retained for lead-owned integration.
