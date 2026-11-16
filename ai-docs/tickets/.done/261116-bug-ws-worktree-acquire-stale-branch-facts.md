---
title: "worktree.acquire hides a stale reused branch; the route resolver blocks a new branch while a merged canonical ref remains"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 76e3925e5b9cf75f
sage-review-completeness-reviewed: 76e3925e5b9cf75f
completed: 2026-11-16
---

# worktree.acquire hides a stale reused branch; the route resolver blocks a new branch while a merged canonical ref remains

## Background

Two dogfood surprises reported by a peer lead (EXEC2) on 2026-11-16 and
relayed by MAIN as an assignment:

1. **Stale reused branch was invisible before Phase 1.** `worktree.acquire`
   with a `target_branch` that already exists switches to that branch and never
   resets it to `base` (by design, so committed work on it survives; see the
   current branch step at
   `agents-plugin-tool/internal/mcp/worktree_tools.go#L523-L551`). The pre-Phase-1
   result (`worktreeAcquireResult`,
   `69096ff0d:agents-plugin-tool/internal/mcp/worktree_tools.go#L21-L38`) carried
   path, worker key, reused, pool, profile, provisioned, hooks, nudge and warnings,
   but no HEAD commit, branch-preexistence fact or ahead/behind relative to
   `base`. A caller therefore could not tell a fresh branch from a stale
   retained one. Phase 1 added those optional facts
   (`agents-plugin-tool/internal/mcp/worktree_tools.go#L22-L43`). EXEC2 reported
   starting work on a stale `b9f141522` this way.
2. **Resolver blocks a fresh branch after an earlier phase merged.** For a
   multi-phase ticket the route resolver keeps deriving the same canonical
   impl branch name. When that old ref still exists (fully merged) and the
   lead vouches identity for a new branch, the vouch path only continues when
   the canonical target does not exist
   (`agents-plugin-tool/internal/mcp/implement_resolver.go#L947-L951`), and
   otherwise stops: with rename allowed it reaches the `target implementation
   branch already exists` stop, while disabled rename reaches the earlier
   rename-not-allowed stop
   (`agents-plugin-tool/internal/mcp/implement_resolver.go#L953-L961`). The
   non-implementation-branch create path already selects deletion and recreation
   of a fully landed leftover target
   (`agents-plugin-tool/internal/mcp/implement_resolver.go#L835-L884`). EXEC2 reported
   having to delete a fully merged local branch by hand before Phase 4 could
   start; the local deletion itself is not independently verifiable from source.

## Decisions

- **Phase 1 scope (MAIN assignment 2026-11-16, under the owner's autonomy
  grant):** add response facts only; no argument change, no change to which
  branch is checked out, no reset of an existing branch. Compatibility T0
  (additive response fields).
- **Technical: field set.** `head` (full OID of the worktree HEAD after the
  branch step and hygiene), `target_branch_existed` (true when
  `target_branch` named a branch that already existed and was checked out as
  is, without reset; false when it was created on `base`; omitted when no
  `target_branch` was passed), `ahead_of_base` and `behind_base` (commit counts
  of HEAD relative to the resolved `base` commit). Rejected: a single `stale`
  boolean (hides the magnitude and needs a threshold policy); reusing
  `reused` (it already means "an idle pooled worktree was reused", a different
  fact).
- **Technical: text output.** The default compact text gains one line per fact
  in the existing `key: value` style (`head:`, `target_branch_existed:`,
  `ahead_of_base:`, `behind_base:`), and format=json mirrors the same fields.
  When `target_branch_existed` is true and `behind_base > 0`, append one entry
  to the existing `warnings` list naming the branch, its behind count and
  that it was not reset, so a caller that only reads warnings still notices.
  Rejected: refusing or auto-resetting a stale branch (a semantic change that
  could discard work; out of scope).
- **Technical: failure handling.** If computing the facts fails after a
  successful branch step, the acquire still succeeds (the worktree is usable);
  the fact is omitted and a warning names the failure. Rejected: failing the
  acquire (would turn an informational addition into a new refusal).
- **Phase 2 scope (owner (1184)=a, 2026-11-16):** a matching lead identity
  vouch may select the existing `recreate` action when the different canonical
  name-rooted target has zero commits ahead of its encoded merge root. Reuse
  the non-implementation path's delete/recreate semantics: create the canonical
  branch from that merge root, leaving the differently named current branch
  and its commits/tracking untouched. This applies before rename-policy or
  current-branch tracking refusals because recreation does not rename the
  current branch. Rejected: manual deletion as the normal remedy and retaining
  the fully-landed collision refusal (the approved dogfood fix removes it).
- **Technical: Phase 2 safety boundary.** Only the exact matching branch/ticket
  vouch lifts the identity stop; targets with unique commits still stop and
  failed ahead-state observation still fails closed. An already canonical
  current branch continues unchanged. Rootless legacy targets retain their
  current behavior because their observation does not establish an encoded
  merge root. Rejected: treating an unmeasured zero as landed, broadening
  unvouched cleanup, or deleting/resetting the current branch.
- **Compatibility: Phase 2 T0**, relative to `deb8289d2`. No tool argument,
  response member, action enum, or persistent schema is added. The existing old
  writer already emits `recreate` for fully-landed targets
  (`agents-plugin-tool/internal/mcp/implement_resolver.go:868-884`), and the old
  instruction reader already handles it
  (`agents-plugin-tool/internal/mcp/implement_resolver.go#L1004-L1005`).
  The unchanged handler marshals the same agenda/branch-plan shape and writes
  it through `enterMode` (`agents-plugin-tool/internal/mcp/session_state.go:1137-1152`);
  the shape is `implement_resolver.go:101-128`. New routing reuses that shape
  for the owner-approved additional case. Old callers send the same calls;
  old readers see a known action, and old writers can still write their prior
  refusal without losing new data. A new invocation after reopening reobserves
  Git (`session_state.go:1123-1131`), so there is no lost field to recover.
  Verify these references against the completed implementation in Phase 2's
  Result; this changes routing behavior, not the cross-version contract.

## Constraints

- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
  default results stay compact text in the existing envelope; JSON mirrors text.
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
  state the tier with source lines for which version writes/reads what. Expected T0: an old caller
  ignores unknown JSON fields and extra text lines; verify that no in-repo
  consumer (Pi bridge, launcher, dashboard, playbooks, tests) parses the
  acquire text strictly enough to break on added lines.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
  emitted text is downstream-neutral.
- No change to `worktree.acquire` arguments, branch selection, hygiene, hooks,
  lease records or the worktree pool. No change to the route resolver in
  Phase 1.
- Phase 2 changes only resolver handling and its regression tests. Existing
  acquire behavior and Phase 1 are untouched; no Git mutation is performed by
  the resolver itself. Branch deletion remains the existing non-forced
  `git branch -d` worker action; another worktree's checkout or a changed Git
  state may refuse it rather than authorizing forced cleanup.
- Phase 2 implements owner (1184)=a. Options (b), improving the manual-cleanup
  refusal only, and (c), leaving behavior unchanged, were not selected.
- Verification is one scoped MCP test family on the configured remote worker,
  not a whole-package run; record any proven pre-existing slow failure as one
  Forward instead of expanding this ticket.

## Prior Decisions

- 47401fef (2026-11-15, commit): "The lock is acquired before the eligibility snapshot and released only after all checkout mutations finish" — bearing: constrains
- 261020-feat-worktree-profiles (2026-10-20, Result (6ad303b09)): "The nudge runs after the lease is written, so the worktree just acquired is never listed, and a nudge failure is a result warning, never an acquire failure." — bearing: constrains
- 260921-feat-ws-impl-branch-identity-resolver (2026-09-21, Result (065d86c2)): "Without a matching vouch, the safety stop remains. Target-name collisions remain blocking." — bearing: constrains; owner (1184)=a supersedes only the verified fully-landed name-rooted target collision in Phase 2.
- 43f0bfb7 (2026-09-15, commit): "Computed only when TargetExists and fails closed on an unverifiable count, matching the existing current-branch guard." — bearing: supports
- 260918-feat-ws-sparse-housekeeping-worktree-occupied-root (2026-09-18, commit 76a45c38): "Direct-base checkout uses `git switch --no-guess -- <base>`; Git's one-checkout-per-branch rule is the safety net" — bearing: constrains
- 260910-feat-lead-run-worktree-parallel-route (2026-09-15, commit 95d18ff6): "Branch setup uses `git switch` (branch-only verb) not `checkout`, and never resets an existing branch to base, so committed work on a re-entered target_branch is preserved" — bearing: constrains
- 260825-feat-impl-branch-single-ticket-scope-merge-timing (2026-08-25, commit 3db94261): "Per plan, commit-content parsing was explicitly rejected as a resolver input" — bearing: constrains
- 523054ba (2026-08-10, commit): "Parsing rule is strip "impl/", split on the LAST "/": everything before is <merge-root> (may itself contain "/"), final segment is <stem>." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Remaining Phase 2: agents-plugin-tool/internal/mcp/implement_resolver.go, impl_identity_test.go, implement_resolver_test.go and impl_identity_integration_test.go in that directory; completed Phase 1 is excluded. |
| scope.surface | public-interface | route.resolve_implement changes its branch-plan decision for a vouched, fully-landed name-rooted collision; existing MCP handler is agents-plugin-tool/internal/mcp/session_state.go#L1100-L1161. |
| scope.new_public_symbol | no | No new tool, argument, response member or action; recreate already exists at agents-plugin-tool/internal/mcp/implement_resolver.go#L868-L884 and #L1004-L1005. |
| scope.new_type_contract | no | Reuses implementBranchPlan and implementAgenda without a new signature or schema (agents-plugin-tool/internal/mcp/implement_resolver.go#L103-L128). |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/impl_identity_test.go#L35-L76, implement_resolver_test.go#L267-L462 and impl_identity_integration_test.go#L222-L274 provide the existing vouch, safety and route fixtures. |
| complexity.reuse_points | confirmed | Read finishImplementBranchPlanTail, TargetAheadOfMergeRoot observation, create-path recreate and its instruction at agents-plugin-tool/internal/mcp/implement_resolver.go#L503-L549, #L868-L884, #L927-L970 and #L1004-L1005. |
| complexity.side_effect_risk | high | Resolver does not mutate Git refs, but its plan authorizes worker deletion/recreation of a canonical ref; the current branch and tracking must survive (agents-plugin-tool/internal/mcp/implement_resolver.go#L1004-L1005). |
| risk.correctness | high | Exact branch/ticket vouch and measured target zero must precede recreation; a rootless default zero is not landing evidence, and unique target work must never be deleted (agents-plugin-tool/internal/mcp/implement_resolver.go#L523-L539, #L933-L961). |
| risk.fit | moderate | Shared tail serves both name-rooted and legacy paths; the owner-approved exception must preserve rootless behavior, canonical-current continue, and unvouched identity stops (agents-plugin-tool/internal/mcp/implement_resolver.go#L887-L961). |
| risk.test | moderate | Existing identity collision cases expect stop; extend them and add real-Git coverage for nested roots, tracking, failed observation, unchanged refs during routing and retained current-branch commits after following the plan. |
| risk.security_or_contract | moderate | Existing recreate vocabulary and agenda shape support T0, but selecting destructive worker guidance must remain bounded by lead vouch and observed landing; handler persistence is agents-plugin-tool/internal/mcp/session_state.go#L1137-L1151. |

## Phases

### Phase 1: Report HEAD, branch reuse and base divergence from worktree.acquire

Implement the Decisions above in `worktreeAcquireResult` and the acquire
path in `agents-plugin-tool/internal/mcp/worktree_tools.go`, including the text
renderer and JSON mirror. Compute the facts after the branch step and hygiene,
inside the existing acquisition lock.

Verification (source fixtures; GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local,
`-count=1`, `./internal/mcp` only with `-run`, never the full package):

1. Fresh `target_branch` on base: `target_branch_existed=false`, ahead/behind
   0/0, `head` equals base.
2. Existing `target_branch` with its own commits and base advanced past it:
   `target_branch_existed=true`, exact ahead/behind counts, `head` equals the
   branch tip (unchanged, not reset), and one warning naming the behind count.
3. Existing `target_branch` equal to base: existed true, 0/0, no warning.
4. `target_branch` omitted: `target_branch_existed` absent; ahead/behind 0/0
   against base.
5. Text and JSON carry the same values; existing acquire tests stay green
   (`-run 'Worktree'`).
6. Fact-computation failure (injected runner error) keeps the acquire
   successful with the fact omitted and a warning.
7. Record the compatibility tier with source lines.

### Result (10aae7320) - 2026-11-16

- Added optional `head`, `target_branch_existed`, `ahead_of_base` and
  `behind_base` facts to both response formats. Known false/zero values survive
  serialization; an omitted target leaves its existence fact absent. A retained
  branch behind base produces one warning naming the branch, count and lack of
  reset. Fact-read/parse failures omit unavailable facts and warn while mint and
  lease publication still succeed.
- Facts are read immediately after hygiene inside the existing acquisition lock
  (`agents-plugin-tool/internal/mcp/worktree_tools.go:339-352,550-560`). Base is
  resolved in the caller's root so relative revisions such as `HEAD` do not
  accidentally name the target checkout; a regression covers this choice.
  Arguments, branch selection, hygiene, hooks, lease records, pool behavior and
  the resolver are unchanged.
- Verification passed with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` from
  `agents-plugin-tool/`:
  - `go test ./internal/mcp -run 'Worktree|^TestImplIdentityResolveProvisionRouteIntegration$' -count=1`
    — passed in 71.340s. Covers fresh/reused pools independently of branch
    existence, exact asymmetric divergence, unchanged retained HEAD, direct-base
    omission, actual text/JSON dispatch, read failures and invalid count output.
  - `go build ./...` — passed.
  - `go vet ./internal/mcp` — passed.
  - `git diff --check` from the repository root — passed.
- **Compatibility: T0**, comparing pre-change `69096ff0d` with `10aae7320`.
  The old result writer has no new facts
  (`69096ff0d:agents-plugin-tool/internal/mcp/worktree_tools.go:21-50`);
  the new writer adds only optional response members/text lines and warnings
  (`agents-plugin-tool/internal/mcp/worktree_tools.go:25-106`). Both versions'
  unchanged dispatcher serializes this transient result through the existing
  envelope (`agents-plugin-tool/internal/mcp/server.go:1488-1491`). No new
  argument or persistent field is written, so old calls still work and reopening
  the new version has no lost data to recover; a fresh acquire recomputes facts.
- **Consumer audit:** searched `worktree[._/]acquire`, `worktreeAcquire`,
  `worker_key`, `target_branch_existed` and existing text labels in Pi, launcher,
  dashboard, ws/wsflow playbooks and tests. No strict acquire-result parser or
  whole-text equality assertion was found. The unchanged pre-change readers are:
  - Pi: generic `tools/call` request
    (`agents-plugin-pi/src/mcp-stdio-client.ts:341-342`); bridge forwards content
    and details, with special parsing restricted to other named tools
    (`agents-plugin-pi/src/bridge.ts:1188-1224`). Its acquire test reference is
    inventory-only (`agents-plugin-pi/test/bridge.test.ts:86`).
  - Launcher: forwards response bytes, tracking IDs without parsing acquire
    text/JSON (`agents-plugin/bin/ws-mcp-launcher.py:1505-1520,1599-1610`).
  - Dashboard: reads its own pool source, not acquire output
    (`agents-plugin-tool/internal/wsdashboard/worktrees.go:27-30,173-174`;
    `agents-plugin-tool/internal/mcp/dashboard_worktrees.go:95-136`).
  - Playbooks: ws and wsflow consume the returned key and nudge, not a fixed
    response layout (`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:171-201`;
    `agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md:171-201`).
    The two files are byte-identical.
  - Go JSON consumers use permissive `json.Unmarshal`, ignoring unknown fields
    (`agents-plugin-tool/internal/mcp/worktree_list_test.go:30-42`,
    `config_repo_scope_test.go:224-236`,
    `impl_identity_integration_test.go:128-136`). Existing acquire dispatch tests
    likewise decoded permissively and are extended to assert the new facts
    (`agents-plugin-tool/internal/mcp/worktree_tools_test.go:684-758,968-979`).
- Independent lite review was omitted under the binding lead instruction (no
  subagents available); the lead will run it after the worker report. No shared
  daemon restart/replacement, Electron, push or merge was performed.
- Phase 2 remains open and unplanned pending the owner's decision. The ticket
  stays in `ready/`.

#### Edition (ad205513e) - 2026-11-16

VERIFIED: E3 moves this ticket from `ready/` to `todo/` through
`ws/tickets.move`. This queue correction supersedes the historical Result's
ready-status sentence without changing Phase 1 or its Result. The only
remaining phase is unplanned and gated on owner decision (1184); no approval,
ready promotion or resolver implementation is implied. The existing lease
stays with its same-email holder on the develop track.

### Phase 2: Resolver handling of a fully landed canonical impl ref

Depends on completed Phase 1 and owner (1184)=a, now recorded below.

In `finishImplementBranchPlanTail` in
`agents-plugin-tool/internal/mcp/implement_resolver.go`, select the existing
`recreate` plan for a different, existing canonical name-rooted target whose
observed ahead-of-merge-root count is zero, only after validating the matching
lead vouch and retaining the current-branch identity boundary. Apply this
before rename permission/tracking checks; otherwise preserve the existing
continue/rename/stop decisions. Reuse the existing recreate instruction rather
than adding an action or a branch-mutating tool. Recreating starts from the
encoded merge root; the current vouched branch remains available with its
original commits and tracking. Keep legacy rootless collision behavior intact.

Verification (`./internal/mcp` only, `-run`, `-count=1`; execute Go remotely
through the configured build lock):

1. Extend `impl_identity_test.go` / existing resolver fixtures: a valid vouch
   plus a fully-landed canonical target returns `recreate`, including when
   rename is disabled or the current branch has tracking state. Cover develop
   and a nested goal merge root. Non-landed canonical targets still stop;
   absent/wrong branch/wrong ticket vouches do not bypass identity safety.
2. Retain canonical-current `continue`, no-target vouch rename/continue,
   unvouched unique-current stop, and rootless legacy collision behavior.
3. Add a real-Git route regression in the existing identity integration
   surface: an earlier canonical branch is fully merged, a different current
   branch is vouched, and routing emits existing delete/recreate instructions
   without mutating either ref. Follow the plan using safe Git operations and
   prove the new canonical tip starts at the merge root while the prior
   current branch and its commits remain reachable. A canonical target with
   unique work is never cleanup-eligible; unresolved target ahead state fails
   closed. Do not force-delete a branch checked out elsewhere.
4. Run the single scoped family covering changed identity/resolver tests, for
   example `go test ./internal/mcp -run 'ImplIdentity|ResolveImplement|ObserveImplementBranch' -count=1`.
   Record the actual command and result; no broad package/build/vet sweep is
   required for this bounded change. Record source-backed T0 compatibility
   against `deb8289d2` and run `git diff --check`.
5. Independent R1 review of the implementation; fixes-only R2 only for
   substantive findings. Record minor/style findings as Forward. Append the
   Phase 2 Result without changing Phase 1 history.

### Result (a576fda55) - 2026-11-16

- A matching branch/ticket identity vouch now selects the existing `recreate`
  action for a different, existing name-rooted canonical target with zero
  measured commits ahead of its encoded merge root. The check precedes rename
  permission/tracking refusals; canonical-current continuation, no-target vouch
  handling, unvouched identity stops, unique-target collisions and legacy
  rootless behavior remain intact. The existing create-path policy advisory is
  restricted to a current branch that is itself the merge root, so this new
  name-rooted case does not emit a contradictory warning.
- Real-Git regressions cover develop and nested goal roots, unchanged refs and
  checkout during routing, unique-target refusal, non-forced deletion refusal
  while another worktree holds the target, recreation from the merge root, and
  retained prior-current commits/tracking. Unit matrices cover exact/absent/
  mismatched vouches, rename/tracking combinations and legacy boundaries;
  injected count-read/parse failures still fail closed.
- Independent R1 lite review (`74520463-01-stale-branch-phase2-r1.md`) found one
  Critical: short revision operands could resolve a same-named tag instead of
  the verified local branch. Fixed in `ce2b2391c` by qualifying both branch
  operands as `refs/heads/` in the shared count helper
  (`agents-plugin-tool/internal/mcp/implement_resolver.go:469-486`). Real-Git
  regressions cover target, merge-root and current-branch shadowing tags with
  ambiguity warnings disabled. No Important or Minor findings were raised.
  The route allocated one-pass `lite`, so no R2 was dispatched under the worker
  protocol; the fix was verified with the same scoped family.
- Verification: from this pooled worktree, the exact remote command was
  `bash D:/sys_lock.sh remote w5090 run exec1-phase2 15 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/groove-animosity-playroom' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/mcp/ -run "ImplIdentity|ResolveImplement|ObserveImplementBranch" -count=1'`.
  It passed before review (`0.866s`) and, after the substantive R1 fix changed
  covered source inputs, passed again (`1.030s`, exit 0). `git diff --check`
  passed. No full-package/build/vet sweep, local Go command, local Go/test
  binary, local temporary artifact, daemon replacement, push or merge was
  performed. D: free space remained above 150 GB.
- **Compatibility: T0**, relative to `deb8289d2`. The old writer already emits
  `recreate` (`deb8289d2:agents-plugin-tool/internal/mcp/implement_resolver.go:868-884`),
  and the new writer reuses it at
  `agents-plugin-tool/internal/mcp/implement_resolver.go:951-957`. The old
  instruction reader (`deb8289d2:agents-plugin-tool/internal/mcp/implement_resolver.go:1004-1005`)
  and new reader (`agents-plugin-tool/internal/mcp/implement_resolver.go:1018-1019`)
  both understand the same non-forced delete/recreate action. Branch-plan and
  agenda fields remain identical (`implement_resolver.go:103-128` in both
  versions); no argument, response member, action enum or persistent schema is
  added. The unchanged handler marshals/writes that same agenda through
  `enterMode` (`agents-plugin-tool/internal/mcp/session_state.go:1138-1151`).
  Old callers keep working, and old writers can still emit their previous
  refusal without losing a new field. Reopening the new version reobserves Git
  on the next invocation (`session_state.go:1123-1131`); there is no stored
  addition to recover. This is the approved routing correction, not a new
  protocol contract.
- Phase 1 and its historical Result/Edition are unchanged. All phases are now
  complete; the implementation branch is retained for lead-owned integration.

## Owner Decisions (2026-11-16)

- (1184)=a: when the route resolver finds a leftover canonical branch with
  zero commits not already on develop, it treats the branch as finished,
  deletes it and creates the new branch. This matches the existing behavior
  of the other path. No commits are lost, because every one is on develop.
  Phase 2 can now be planned; move this ticket back to `ready/` once it is.
