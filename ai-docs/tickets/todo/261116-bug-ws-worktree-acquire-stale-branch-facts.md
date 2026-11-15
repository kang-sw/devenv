---
title: "worktree.acquire hides a stale reused branch; the route resolver blocks a new branch while a merged canonical ref remains"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b594b03d5ece0a8f
sage-review-completeness-reviewed: b594b03d5ece0a8f
---

# worktree.acquire hides a stale reused branch; the route resolver blocks a new branch while a merged canonical ref remains

## Background

Two dogfood surprises reported by a peer lead (EXEC2) on 2026-11-16 and
relayed by MAIN as an assignment:

1. **Stale reused branch is invisible.** `worktree.acquire` with a
   `target_branch` that already exists switches to that branch and never
   resets it to `base` (by design, so committed work on it survives; see the
   branch step in the acquire path, VERIFIED at
   `agents-plugin-tool/internal/mcp/worktree_tools.go:467-493`). The result
   (`worktreeAcquireResult`, `agents-plugin-tool/internal/mcp/worktree_tools.go#L21-L38`) carries path, worker
   key, reused, pool, profile, provisioned, hooks, nudge and warnings, but no
   HEAD commit, no statement that the target branch pre-existed, and no
   ahead/behind relative to `base`. A caller therefore cannot tell a fresh
   branch from a stale retained one. EXEC2 started work on a stale
   `b9f141522` this way.
2. **Resolver blocks a fresh branch after an earlier phase merged.** For a
   multi-phase ticket the route resolver keeps deriving the same canonical
   impl branch name. When that old ref still exists (fully merged) and the
   lead vouches identity for a new branch, the vouch path only continues when
   the canonical target does not exist
   (`agents-plugin-tool/internal/mcp/implement_resolver.go:944-949`), and
   otherwise stops: with rename allowed it reaches the `target implementation
   branch already exists` stop, while disabled rename reaches the earlier
   rename-not-allowed stop
   (`agents-plugin-tool/internal/mcp/implement_resolver.go#L952-L960`). The
   non-implementation-branch create path already selects deletion and recreation
   of a fully landed leftover target
   (`agents-plugin-tool/internal/mcp/implement_resolver.go#L835-L884`). EXEC2 had to delete a fully merged local
   branch by hand before Phase 4 could start.

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
- **Deferred (owner decision pending): Phase 2 resolver behavior.** Changing
  how the resolver treats an old canonical impl ref changes observable routing
  behavior and needs owner approval before any phase plan is written or
  implemented. Candidate options recorded for the owner, none chosen:
  (a) in the vouched path, treat a canonical ref with no commits ahead of its
  merge root (fully landed, an ancestor of the merge root or goal branch) as
  finished, and delete/recreate it like the non-vouched leftover path;
  (b) keep the refusal but make its reason name the fully landed state and the
  exact remedy (delete the merged branch), a message-only change;
  (c) leave the behavior as is.

## Prior Decisions

- 47401fef (2026-11-15, commit): "The lock is acquired before the eligibility snapshot and released only after all checkout mutations finish" — bearing: constrains
- 261020-feat-worktree-profiles (2026-10-20, commit 6ad303b0): "The nudge runs after the lease is written, so the worktree just acquired is never listed; a failed nudge is a result warning, never an acquire failure." — bearing: supports
- 260921-feat-ws-impl-branch-identity-resolver (2026-09-21, commit 065d86c2): "A vouched shared/tracking branch or rename-disabled branch continues in place; target-name collisions remain blocking." — bearing: constrains
- 260918-feat-ws-sparse-housekeeping-worktree-occupied-root (2026-09-18, commit 76a45c38): "Direct-base checkout uses `git switch --no-guess -- <base>`; Git's one-checkout-per-branch rule is the safety net" — bearing: constrains
- 260910-feat-lead-run-worktree-parallel-route (2026-09-15, commit 95d18ff6): "Branch setup uses `git switch` (branch-only verb) not `checkout`, and never resets an existing branch to base, so committed work on a re-entered target_branch is preserved" — bearing: constrains
- 260825-feat-impl-branch-single-ticket-scope-merge-timing (2026-08-25, commit 3db94261): "Per plan, commit-content parsing was explicitly rejected as a resolver input" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1 names agents-plugin-tool/internal/mcp/worktree_tools.go and existing acquire tests in agents-plugin-tool/internal/mcp/worktree_tools_test.go; implement_resolver.go is Phase 2 only, not yet planned. |
| scope.surface | public-interface | worktree.acquire adds JSON response members and compact text lines; agents-plugin-tool/internal/mcp/server.go#L1488-L1491 selects the existing response format. |
| scope.new_public_symbol | yes | New wire response fields head, target_branch_existed, ahead_of_base and behind_base; no new MCP tool or exported Go type is planned. |
| scope.new_type_contract | yes | Decisions define optional facts on computation failure, a tri-state target_branch_existed (true/false/absent), and integer ahead/behind counts; current shape is agents-plugin-tool/internal/mcp/worktree_tools.go#L21-L38. |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/worktree_tools_test.go#L494-L551 covers dispatch/JSON, #L697-L735 covers direct-base checkout; worktree_acquire_concurrent_test.go#L21-L25 supplies an injectable Runner. |
| complexity.reuse_points | confirmed | agents-plugin-tool/internal/mcp/worktree_tools.go: wtRun, worktreeAcquireResult.text, acquireWorktree and provisionWorktree provide the runner, result, rendering and locked lifecycle; server.go#L1488-L1491 already serializes the same result for JSON/text. |
| complexity.side_effect_risk | moderate | Facts are read-only additions, but must run inside the acquisition lock without changing switch/reset/clean, hooks or lease publication (agents-plugin-tool/internal/mcp/worktree_tools.go#L283-L317, #L467-L590). |
| risk.correctness | moderate | Must distinguish pooled-worktree reuse from branch preexistence, orient divergence counts correctly, retain zero/false values, and omit failed facts without failing acquire. |
| risk.fit | moderate | Phase 1 must preserve generic ticket-unaware acquire and the locked lifecycle; Phase 2 has no approved behavior yet and must preserve identity/collision boundaries until approved. |
| risk.test | moderate | Existing fixtures cover creation, reuse, dispatch and direct-base checkout, but new assertions must discriminate retained divergent branches, omission versus false/zero, text/JSON parity and injected fact failures. |
| risk.security_or_contract | moderate | Public MCP output changes target T0, with no new arguments, persistence, reset or authority; compatibility still requires the source-backed consumer audit specified in Constraints. |

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

Not yet planned. Blocked on the deferred owner decision in `## Constraints`;
a later ticket edit replaces this paragraph with the approved plan before any
implementation.
