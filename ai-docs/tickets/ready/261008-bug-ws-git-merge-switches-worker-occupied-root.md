---
title: "ws git.merge switches a root that a worker occupies on an impl/* branch"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7c7c8708a8616e3f
sage-review-completeness-reviewed: 7c7c8708a8616e3f
---

# ws git.merge switches a root that a worker occupies on an impl/* branch

## Background (dogfood, 2026-10-08)

- A Pi lead session ran a worker directly in the main checkout
  (`/home/swkang/devenv`), which switched it to `impl/develop/catty-hertz-trout`
  at 13:34:15 with uncommitted edits in progress.
- A separate Claude lead session, whose last knowledge of that checkout was
  "on develop", called `ws/git.merge(branch: impl/develop/pi-summary-title,
  target: develop)`. The tool switched the shared checkout from the worker's
  impl branch to `develop` (reflog 13:36:29), then the merge itself failed on
  the overlapping dirty files. The checkout was left on `develop` with the
  worker's uncommitted edits carried along until someone switched back at
  13:39:27. No commit landed in the window, so no history damage occurred.
- The workflow manual already says a root on `impl/*` is the worker's until
  the lead restores it, but nothing enforced it at the merge call.

## Original expectation and questions

The original capture requested pre-switch refusal on an `impl/*` branch other
than the merge source, or at least when another session occupied it, with a
pooled-worktree diagnostic. It also asked for failed-merge switch rollback and
asked whether to use branch naming, a lease/occupancy record, or a new
`expected_branch` argument. These were questions, not confirmed mechanisms.
The bounded approval below does not authorize general rollback or a new API.

## Decisions

- Owner-approved FIX 6 scope (triage 1060, relayed 2026-11-15): refuse merge
  checkout switches against a worker-occupied root, before changing HEAD,
  index, refs or working files. Name the observed occupying branch and the
  housekeeping-worktree route; name an occupant only if evidence identifies
  it.
- (1062) Technical settlement from fact population: use symbolic HEAD's
  `impl/` prefix, following the recorded branch-pattern choice in
  `260918-feat-ws-sparse-housekeeping-worktree-occupied-root`. For merge,
  refuse when that branch differs from both resolved source and target;
  session parenthood does not exempt a merge caller. Check before status can
  refresh the index, and recheck immediately before checkout. For the ticket
  consumer, refuse parent-less callers on an impl checkout while retaining
  parented callers in their session-bound checkout. This preserves current
  worker flows; parenthood is not proof of branch ownership. Missing leases
  remain supported. Rejected: mandatory leases or new branch-authorization
  arguments, because current leases do not identify an occupying branch and
  those alternatives would expand the approved call contract. External Git
  processes are not serialized by these checks; do not promise exclusion
  against arbitrary concurrent external checkout changes.
- Supply the reusable occupied-root guard consumed by
  `261030-bug-tickets-move-stages-on-occupied-root`. Land this ticket first and
  serialize the two implementations; the consumer must reuse the landed guard
  rather than invent a second occupancy policy.
- Compatibility: **T0 / bug fix**, under
  `ai-docs/manuals/release-compatibility.md`. Old plugin calls keep their
  existing arguments; the new handler rejects the unsafe checkout operation,
  not an old client version. Current code reads worktree/index state, switches
  to the target and writes an ordinary Git merge
  (`agents-plugin-tool/internal/mcp/git_merge.go:180-250,305-355`); the new
  guard must leave refused state byte-for-byte unchanged and allowed merges
  in the same Git format. Old Git/current ws readers and a new reopen can
  still read those refs/index/commits; no reconstruction is needed. No hello,
  tool-name, required-argument or lease-schema change is authorized. Any need
  to refuse an old version or break valid legacy merge calls is a blocker,
  not an approved T3 boundary.

## Current source

- VERIFIED: the occupied-root banner detects `impl/*` by branch pattern, not
  a live-worker registry, and is advisory; top-level-lead parent gating avoids
  warning a worker off its own branch
  (`agents-plugin-tool/internal/mcp/occupied_root_announcement.go:10-39`).
- VERIFIED: `mergeImplBranch` checks a target held in another worktree, but
  does not protect a different worker branch in the caller's checkout before
  `switch` (`git_merge.go:211-250,305-337`). The existing merge fixture starts
  on its own source impl branch (`git_merge_test.go:27-43,65-100`); a blanket
  ban on all impl branches would break that legitimate flow.
- VERIFIED: `git.commit` has a remembered-branch assertion, not an unconditional
  impl-branch ban (`agents-plugin-tool/internal/wsgit/git.go:634-657`). It is
  evidence for no-mutation refusal and recovery wording, not a selected API
  addition to merge.

## Constraints

- Preserve allowed merges from their own source branch, existing release-target
  acknowledgement, target-held checks, dirty-index safeguards and intentional
  conflict state on the target. Do not force, detach or restore a worker's
  checkout as an automatic recovery.
- General failed-merge rollback, new required arguments, new occupancy policy
  and broader session-liveness machinery are not approved by this contract.
- Applicable implementation conventions: read
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/release-compatibility.md` before their covered source edits.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 261030-bug-tickets-move-stages-on-occupied-root (2026-10-30, Decisions): "Serialize implementation and reuse its guard; do not ship a separate occupancy predicate." — bearing: constrains
- 0520bc61 (2026-10-30, commit): "Committed from a housekeeping worktree because the root checkout is occupied by another worker's impl/develop/cacti-frill-lance branch." — bearing: supports
- 260918-feat-ws-sparse-housekeeping-worktree-occupied-root (2026-09-18, Decisions): "Branch-pattern detection was chosen over child-session liveness because it needs no liveness model" — bearing: constrains
- 260916-feat-ws-git-merge-relax-worktree-gate (2026-09-17, commit): "Narrowed checkWorktree to block only a dirty index or unmerged paths" — bearing: constrains
- c7c1de4d (2026-09-12, commit): "Collect all checkable safety findings, retain them as non-waivable diagnostics, and recheck candidate tips before checkout and merge to detect intervening changes." — bearing: constrains
- 260911-feat-ws-git-merge-lead-owned-merge-authority (2026-09-11, commit): "Accept an explicit impl branch for lead calls from the base checkout; preserve conflicts for lead-delegate and use non-forced branch deletion only after success." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/git_merge.go, occupied_root_announcement.go, git_merge_test.go; shared guard and MCP dispatch integration |
| scope.surface | public-interface | git.merge refusal behavior changes; existing arguments remain (agents-plugin-tool/internal/mcp/server.go#L1377-L1417) |
| scope.new_public_symbol | no | no new tool or exported symbol required by Phase 1; reusable guard stays internal |
| scope.new_type_contract | no | existing symbolic HEAD and caller-parent context suffice; an internal reusable guard adds no wire or shared-data contract |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/git_merge_test.go#L27-L100, #L778-L861, #L1020-L1170; reusable-guard test file placement unspecified |
| complexity.reuse_points | confirmed | occupiedRootAnnouncement branch detection, parseImplBranchRoot, merge diagnostics and pre-switch checkTips read in agents-plugin-tool/internal/mcp/occupied_root_announcement.go#L10-L39 and git_merge.go#L305-L337 |
| complexity.side_effect_risk | high | guard must precede checkout mutation; allowed path switches, merges and deletes source refs (agents-plugin-tool/internal/mcp/git_merge.go#L319-L399) |
| risk.correctness | high | the pre-status and pre-switch guards must preserve another worker's checkout; arbitrary external Git races are outside the exclusion guarantee |
| risk.fit | moderate | advisory parent gating is not branch ownership proof; shared consumer must retain worker ticket moves (agents-plugin-tool/internal/mcp/session_auth.go#L17-L32) |
| risk.test | high | clean/dirty refusal byte preservation, stale records and interleaving require new regressions beyond own-source fixtures |
| risk.security_or_contract | high | the settled branch/caller predicate must preserve legacy permitted merges and worker own-checkout moves without a new required argument |

## Phases

### Phase 1: Refuse occupied-root merge switches before mutation

Implement the settled reusable guard and apply it before a merge
can switch a checkout another worker occupies. Return an actionable refusal
without changing the worker's branch, index, refs or dirty/untracked files.
Do not generalize this into rollback after an allowed merge has begun.

Regression verification: reproduce a caller intending to merge source A while
HEAD is on worker branch B, with both clean and dirty B checkouts; assert no
switch/merge/delete command, identical HEAD/index/file bytes, retained source
and diagnostic recovery guidance. Cover the chosen predicate's missing/stale
record cases, allowed own-source and already-on-target merges, a target held
elsewhere, release-target acknowledgement and conflicts. Test through MCP as
well as the reusable guard. Run affected `internal/mcp` tests and
`cd agents-plugin-tool && go test ./...`; use local temporary repositories,
not the shared root/pool. This draft records no implementation or test result.
