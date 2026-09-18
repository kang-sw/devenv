---
title: "Sparse housekeeping worktree for a lead whose root is occupied by a worker's impl/ branch"
related:
  260911-research-lead-commit-guard-during-worker-run: sibling — this ticket builds the sanctioned escape hatch its Open question #2 asks for; the hard MCP-level guard stays there
  260910-feat-lead-run-worktree-parallel-route: origin of worktree.acquire/release and the pool this ticket extends with a sparse shape
  260806-feat-worktree-ticket-scope: prior art — git.commit's sparse-aware staging (`git add --sparse` via SparseScopeActive) and the workflow_manual scope-announcement banner pattern this ticket reuses
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5850e6a31ef99364
sage-review-completeness-reviewed: 5850e6a31ef99364
completed: 2026-09-18
---

# Sparse housekeeping worktree for a lead whose root is occupied by a worker's impl/ branch

## Background

In the default serial `lead-run` route the worker checks its `impl/<parent>/<slug>`
branch out in the lead's own worktree, and that checkout outlives the worker's
turn (`worker-stop-protocol.md` "## Branch": the terminal report does not
restore the checkout). While the root is in that state the lead has no
sanctioned place to write: a `git.commit` there is refused by the
`expected_branch` guard (or, before that guard, landed on the worker's branch —
the 2026-09-11 incident recorded in
`260911-research-lead-commit-guard-during-worker-run`), and the only existing
alternative, `worktree.acquire`, is a full checkout whose cost is what makes
the parallel route user-gated ("provisioning is expensive in large or
submodule-heavy repositories"). Downstream projects with large trees feel this
most: the lead wants to capture a ticket or update a document mid-run and
either waits or pays a full worktree.

The same gap hits a fresh session opened beside a running one ("a worker is
busy here, let's discuss something else"): that lead did not spawn the worker
and has no signal that the root is occupied until a commit is refused.

Evidence gathered on 2026-09-18 (all cited by search, not by line):

- `provisionWorktree` (`agents-plugin-tool/internal/mcp/worktree_tools.go`)
  runs `git worktree add --detach`, `git switch`, `git reset --hard`,
  `git clean -ffdx`, then `git submodule update --init --recursive` when
  `.gitmodules` exists; it reuses a pooled worktree only when it is under the
  pool, at detached `HEAD`, and `status --porcelain`-clean. No `--no-checkout`
  or sparse-checkout is involved.
- The `worktree.acquire` dispatch handler (`server.go`, `case
  "worktree.acquire"`) requires `base` and `target_branch`, then mints a
  `roleLead` session key bound to the canonical worktree path and returns
  `path` + `worker_key`. Every root-aware tool (`git.commit`, `tickets.move`,
  `tickets.close`, ...) resolves its root from the session key
  (`resolveToolRoot`), so a key bound to the sparse worktree makes the whole
  existing toolset operate there with no per-tool change.
- `git.commit` already stages with `git add --sparse` when
  `CommitOptions.SparseScopeActive` is set from `wsdoc.SparseCheckoutActive`
  (`server.go` `case "git.commit"`; `wsgit/git.go` `stagingCommandsForCommit`).
- `workflow_manual.go` injects bootstrap banners through
  `injectBootstrapStalenessWarning(body, scopeAnnouncement(root))` on both the
  FRESH-with-root path and the CONTINUE path; `scope_announcement.go` is the
  model for a root-state banner, and `scope_announcement_test.go` for its
  test shape (asserts both paths).
- `git.merge` (`git_merge.go`, `mergeImplBranch`) checks the target branch out
  in the session root with `git switch` before merging: it needs the target
  branch free, which is why the release-before-merge ordering below is a
  hard rule rather than a preference. When the target is checked out in
  another worktree, that `git switch` fails (`'develop' is already used by
  worktree at '<path>'`) and today's `switch_failed` diagnostic misattributes
  it: its reason says "a working-tree change likely overlaps it" and its
  resolution says "Commit, stash, or revert the overlapping working-tree
  change" — advice that cannot succeed and invites a `git worktree remove` or
  `--force` guess. `listWorktrees` (`worktree_tools.go`, same package)
  already parses `git worktree list --porcelain` with each entry's
  `branch refs/heads/<name>`.
- `parseImplBranchRoot` / `implementMergeRootFor`
  (`implement_resolver.go`) already derive the parent of an
  `impl/<parent>/<slug>` branch (split on the last `/`); a rootless
  `impl/<stem>` yields no parent and is treated as a legacy path.
- The `wsgit` commit guard (`git.go`, `Client.Commit`) emits two refusals:
  detached `HEAD`, and believed-vs-actual branch mismatch.
- Workers read the manual too: `ticket-worker.md` "Inputs" calls
  `workflow_manual(session_key: <your key>)`, and `childRoleForPlaybookRole`
  maps `worker` to `roleLead`. Session records (`session_auth.go`,
  `sessionRecord`) carry `parent`: the FRESH-with-root mint in
  `workflow_manual.go` passes `""`, while `playbook.render` and
  `worktree.acquire` pass the caller's key.
- `wsdoc.newTicketScope` (`internal/wsdoc/tickets_scope.go`) activates on
  `core.sparseCheckout` only and reads the linked worktree's
  `config.worktree`; it has no notion of cone mode. `scopeAnnouncement`
  fires on `Active` alone, including the `Hidden == 0` branch.

## Decisions

Confirmed by the user on 2026-09-18 (discussion, this session).

1. **Sparse worktree, not plumbing commits.** Committing to the parent branch
   without any checkout (`commit-tree` / `update-ref` against a scratch tree)
   was rejected: every ws tool and the lead's editor assume a real tree at the
   session root, and `git.merge` has no checkout-free path either, so plumbing
   would need a parallel primitive set. A sparse pooled worktree keeps every
   existing tool working through the returned key.
2. **Extend `worktree.acquire`; no new tool.** A docs-specific
   `worktree.acquire-docs` was rejected: a generic `sparse_paths` serves a
   scoped worker in a monorepo as well as a docs worktree, and keeps the
   surface host-neutral. The lead's housekeeping call is
   `worktree.acquire(base: <parent branch>, sparse_paths: ["ai-docs"])`.
3. **`sparse_paths` is a cone-mode directory list; omitted means the current
   full checkout.** No implicit `ai-docs` default: the parallel route's calls
   keep their exact behavior.
4. **Direct parent-branch checkout (`target_branch` omitted).** Omitting
   `target_branch` checks `base` itself out in the sparse worktree, so the
   lead's commits land on the parent branch with no extra branch and no merge
   step. A `docs/<stem>` branch + later fast-forward was rejected as an extra
   branch and merge for no safety gain. Git's one-checkout-per-branch rule is
   the safety net: `base` is free exactly while the worker holds `impl/*` in
   the root, and when it is not free (nobody is running a worker) the tool
   fails loudly and the lead does not need it anyway. `base` must then be a
   local branch name; Git's "already checked out" refusal surfaces verbatim as
   the tool error with the hint sentence from Decision 9 appended.
5. **Pool reuse matches sparse shape.** A pooled candidate is eligible only
   when its `core.sparseCheckout` state matches the request (sparse candidate
   for a sparse request, full for full); a sparse reuse re-applies the
   requested pattern set every time. `worktree.release` keeps the sparse state
   (reset, clean, detach as today). Converting on reuse was rejected: disabling
   sparse at release materializes the whole tree and defeats the purpose;
   converting a full candidate to sparse at acquire makes the two directions
   asymmetric for no benefit.
6. **Occupied-root banner in `workflow_manual`.** Rendered on both the
   FRESH-with-root path and the CONTINUE path (same injection as
   `scopeAnnouncement`) when the session root's `HEAD` symbolic ref has the
   `impl/` prefix, resolved with one git subprocess (`git symbolic-ref --quiet
   --short HEAD`; detached or non-`impl/` → no banner, resolution error → no
   banner, never a render failure). The parent is derived with
   `parseImplBranchRoot`; a rootless `impl/<stem>` renders the banner without
   a `base` suggestion. Branch-pattern detection was chosen over child-session
   liveness because it needs no liveness model (the hard problem
   `260911-research-lead-commit-guard-during-worker-run` flags) and also
   covers the "worker done, lead not yet restored" state; the banner is
   advisory, so a false positive costs a sentence. **Rendered for top-level
   lead keys only** (design review, confirmed 2026-09-18): workers also call
   `workflow_manual` (`ticket-worker.md` "Inputs"), their keys are minted
   `roleLead` (`childRoleForPlaybookRole`), and on an `impl/*` root the
   branch's own owner would read "do not commit here". The discriminator is
   the session record's `parent`: a key minted by the FRESH-with-root path
   has `parent: ""`; every `playbook.render` and `worktree.acquire` key
   carries the lead's key as `parent`. A key with a non-empty `parent` never
   gets the banner. Rewording the banner conditionally ("unless this is your
   branch") was rejected: it hands the reader an ownership judgment the
   record already answers.
7. **Not in `lead-discuss`.** Discuss writes nothing ("Edit no source and
   write no document here"), so the guidance lives in the layers every lead
   skill loads (the manual and its banner) and at the write sites
   (`lead-ticket`, `lead-run`), plus the refusal that bites (`git.commit`).
8. **Release-before-merge ordering is a stated rule.** `git.merge` switches
   the session root to the target branch, so the sparse worktree holding that
   branch must be released first; the prose says so at both write sites.
9. **`git.commit` mismatch refusal names the escape hatch** only when the
   actual branch has the `impl/` prefix; the detached-`HEAD` refusal is
   unchanged.
10. **Three phases** (acquire → banner + refusal text → prose + mirrors), each
    one commit unit; prose last so it names a landed contract.
11. **Submodules under sparse:** Phase 1 keeps the unconditional
    `submodule update --init --recursive` and records the observed behavior in
    its Result; a design change happens only if it fails.
12. The user accepted the surface cost ("a bit much, but it covers the single
    most common pattern").
13. **The ticket-scope banner is silent in a cone-mode worktree** (design
    review, confirmed 2026-09-18). `newTicketScope` keys on
    `core.sparseCheckout` alone and `scopeAnnouncement` fires on `Active`
    alone (deliberately even when nothing is hidden), so a `sparse_paths:
    ["ai-docs"]` worktree would announce "Sparse-checkout scope is active …
    restore full visibility with `git sparse-checkout disable`" — false, and
    a remedy that materializes the whole tree. Ticket-board scoping
    (`lead-scope-worktree`) requires `--no-cone` (its patterns are file-level;
    `260806-feat-worktree-ticket-scope` Decisions), and `git sparse-checkout
    set --no-cone` flips the mode, so `core.sparseCheckoutCone=true` means
    "not a ticket scope" by construction. Rule: when the worktree's
    sparse-checkout is in cone mode, `scopeAnnouncement` returns `""`;
    `SparseCheckoutActive` (git.commit's `--sparse` staging) and the
    tickets.move/close scope gate are unchanged. Rejected: silencing on
    `Hidden == 0` (reverses 260806's pinned contract,
    `TestScopeAnnouncementFiresWithNoTicketHidden`); matching the cone set
    exactly against `["ai-docs"]` (a future development cone such as
    `["ai-docs", "src/foo"]` still carries the whole ticket tree and would
    re-trigger the false banner; a cone that excludes `ai-docs` makes the
    banner's remedy wrong too — the fix there is release and re-acquire, a
    different advisory outside this ticket). Accepted side effect: a hand-made
    cone checkout that excludes `ai-docs` loses the hidden-stem notice it gets
    today; Phase 2's Result records this explicitly rather than leaving it as
    silent drift.
14. **`git.merge` refuses a target held by another worktree, as a merge
    stop** (user, 2026-09-18, after the live occurrence in this session). The
    release-before-merge rule (Decision 6's banner, Phase 3 prose) is
    advisory; its violation surfaces in the *other* lead's `git.merge`, whose
    current `switch_failed` diagnostic gives wrong advice (Background). Rule:
    before the internal `git switch`, `mergeImplBranch` inspects `git worktree
    list --porcelain` and, when the target branch is checked out in a worktree
    other than the session root, adds a `must_resolve` diagnostic
    `target_held_elsewhere` naming that path, so the merge is
    `policy_blocked` with nothing switched or merged. The text tells the
    caller to treat it as the merge stop `lead-run` already defines (report,
    leave the branch retained and `HEAD` unchanged, wait for the holder to
    release) and never to remove, switch, or force the holding worktree. The
    holder's kind is named when known: a path under the ws worktree pool is
    "a parallel lead's housekeeping checkout"; any other path is "another
    worktree". Rejected: matching git's `switch` error text (wording differs
    across git versions: "already checked out at" vs "already used by
    worktree at"); merging inside the holding worktree on the caller's behalf
    (touches another session's possibly dirty sparse tree and breaks the
    one-root-per-key binding); auto-releasing the holder (its key belongs to
    another session; the user coordinates the two).

## Constraints

- `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/skill-authoring.md` govern every prose and emitted-string
  edit below. The pre-authored texts in the phases were written against
  them; keep them verbatim unless a test or the fresh-reader audit forces a
  change, and never add a `26xxxx` stem, a commit hash, or a devenv path to a
  shipped string.
- `ai-docs/manuals/wsflow-mirroring.md`: `lead-workflow-manual`, `lead-run`,
  and `lead-ticket` are shipped wsflow skills, so after editing any of the
  three rsrc playbooks run, in order and each with `-count=1`:
  `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  then `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`,
  and run `python3 -m unittest discover agents-plugin-wsflow/tests`.
- `agents-plugin-pi/rsrc/` is a third byte-identical mirror of
  `agents-plugin/rsrc/`, guarded by `TestPiMirrorUpToDate`
  (`agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go`, run by `go test
  ./...`). Its only scripted resync is `bump-ws-version.sh`, which also bumps
  the plugin version — **do not run it for this ticket**. Resync by copying the
  tree byte-for-byte (the script's own step is `sync_tree("agents-plugin/rsrc",
  "agents-plugin-pi/rsrc")`; `rsync -a --delete agents-plugin/rsrc/
  agents-plugin-pi/rsrc/` is equivalent) and let the guard test confirm.
- `ai-docs/manuals/ws-mcp.md` for `agents-plugin-tool/internal/mcp/` edits.
- Go-emitted strings name tools bare (`worktree.acquire`, `git.merge`), matching
  the existing `implement_resolver.go` warning text; rsrc prose names them as
  `{{.McpNamespace}}/<tool>`.
- Never reset or delete a pooled worktree beyond what `releaseWorktree`
  already does; the sparse shape adds no destructive step.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Art

- `scope_announcement.go` + `scope_announcement_test.go`: banner function
  returning `""` when inactive, injected via `injectBootstrapStalenessWarning`
  on both manual paths; tests use a temp repo and assert presence on
  FRESH-with-root and CONTINUE, absence when inactive.
- `worktree_tools_test.go`: `TestProvisionWorktreeReuseIdle`,
  `TestProvisionWorktreeSkipBranchCheckedOut`,
  `TestProvisionWorktreeHygieneResetToBase`, `TestReleaseWorktreeDetaches`,
  `TestWorktreeAcquireReleaseDispatch`,
  `TestWorktreeAcquireRejectsNonLeadAndBadArgs` — the harness and the
  assertions to extend.
- `wsgit/git_test.go` `TestCommitRefusesBranchMismatch` (asserts the
  substrings `develop`, `impl/develop/other`, `believed`, `actual` and a
  single runner call) and `TestCommitRefusesDetachedHead` (asserts
  `detached`) — the refusal-text assertions to extend; appending Decision 9's
  suffix breaks neither. No rsrc playbook or mirror quotes the refusal text,
  and `internal/wsgit` does not import `internal/mcp` (the dependency runs
  `mcp` → `wsgit`), so the parent-parsing helper is local to `wsgit`.
- `wsdoc.SparseCheckoutActive` / `wsdoc.TicketScope` in
  `internal/wsdoc/tickets_scope.go` — cheap sparse-state detection already
  used by `git.commit`.
- `git_merge.go` `mergeImplBranch`: diagnostics are `implMergeDiagnostic{Code,
  Classification, Reason, Resolution}` appended through the local `add` /
  `gitFailure` closures; `checkWorktree` runs before and after the switch;
  `blocked()` sets `policy_blocked` and returns the first diagnostic's
  reason as the error for non-release targets. `git_merge_test.go`
  `mergeFixture(t, target)` builds a repo on `impl/<target>/test-merge-unit`
  with `<target>` one commit behind; `TestImplMergeRefusals` asserts the
  error substring and that `HEAD` did not move;
  `requireMergeDiagnostic(t, r, code, classification)` checks a diagnostic's
  presence and non-empty text. The `git.merge` handler (`server.go`, `case
  "git.merge"`) resolves only the root; the `worktree.acquire` handler shows
  how to resolve `wsconfig.ItemWorktreePool` from the session key and
  `resolvePoolRoot(value, mainRoot)` turns it into a path.

## Prior Decisions

- 260910-feat-lead-run-worktree-parallel-route (2026-09-15, commit): "Reuse eligibility is the conservative golden rule from the ticket: under the owned pool prefix, at detached HEAD, and clean." — bearing: supports
- 260806-feat-worktree-ticket-scope (2026-08-06, Decisions): "Cone mode cannot express this scope: it selects directories, not files (`git sparse-checkout set <file>` fails with `is not a directory`). `--no-cone` is required." — bearing: supports
- 31eedb1e (2026-09-17, commit): "git.commit now resolves the actual branch via `git symbolic-ref --quiet --short HEAD` and hard-refuses (no mutation) on believed-vs-actual mismatch or detached HEAD, before any staging." — bearing: constrains
- e02d2eb0 (2026-09-16, commit): "provisionWorktree/releaseWorktree must recognize the literal default template... chose isDefaultPoolConfig over threading a Scope/override-provenance flag since it needed no signature or contract change." — bearing: constrains
- 6bc205d2 (2026-09-15, commit): "worktree.release guard resolves primary via `worktree list` first entry and canonicalizes... without the guard a lead passing the main-repo path would hard-reset the primary working tree." — bearing: constrains
- 260824-feat-review-watermark-ledger-sweep-lazy-checkpoint (2026-08-30, commit): "workflow_manual.go's two call sites inject the raw advisory directly (mirroring scopeAnnouncement/computeManuals)... an intentional per-site formatting split." — bearing: supports
- 260909-refactor-retire-spec-mental-model-layers (2026-09-10, commit): "The alarm's entire purpose was to push a project toward populating two document directories... it cannot be reworded to survive the layer it advertises, so it is deleted rather than repointed." — bearing: supports
- d6845400 (2026-08-10, commit): "Both survey forks (git.commit sparse-active detection; sparse-hidden-vs-deleted disambiguation) resolved empirically (git 2.43) plus reuse of wsdoc newTicketScope/TicketScope — no research escalation." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/worktree_tools.go, server.go, workflow_manual.go, scope_announcement.go, implement_resolver.go, git_merge.go; agents-plugin-tool/internal/wsdoc/tickets_scope.go; agents-plugin-tool/internal/wsgit/git.go; agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md, lead-run/lead-run.md, lead-ticket/lead-ticket.md; new occupied_root_announcement.go |
| scope.surface | public-interface | worktree.acquire MCP tool schema changes (server.go#L3736-3745: adds sparse_paths, target_branch becomes optional) and wsgit.Client.Commit's refusal text (internal/wsgit/git.go) both change wire-level tool contracts |
| scope.new_public_symbol | yes | Decision 13 adds the exported field `Cone` to exported `wsdoc.TicketScopeInfo` (additive, not marshalled by any consumer); occupiedRootAnnotation, the wsgit local parent-parsing helper, and the worktreeAcquireResult field remain unexported |
| scope.new_type_contract | yes | provisionWorktree(ctx, runner, root, base, targetBranch, poolConfigValue string) (worktree_tools.go#L196) gains a sparsePaths []string parameter, and the worktree.acquire tool schema's required list changes from [base, target_branch] to [base] |
| scope.test_surface | new-files | new occupied_root_announcement_test.go (Phase 2) plus extensions to existing worktree_tools_test.go, git_merge_test.go, and wsgit/git_test.go |
| complexity.reuse_points | confirmed | provisionWorktree/releaseWorktree pool lifecycle (worktree_tools.go, landed 260910-feat-lead-run-worktree-parallel-route), scopeAnnouncement two-site injection pattern (workflow_manual.go#L289,L321), wsdoc.SparseCheckoutActive (internal/wsdoc/tickets_scope.go#L426), parseImplBranchRoot (implement_resolver.go#L840) are all read and reused rather than reimplemented |
| complexity.side_effect_risk | moderate | new direct-base-branch checkout path (`git switch --no-guess -- base`, no target_branch) and cone-mode sparse-checkout mutation are new git-state-mutating branches in provisionWorktree that did not exist before this ticket |
| risk.correctness | moderate | several new edge cases are spelled out but unverified against real git output: sparse-reuse shape matching, rootless impl/<stem> banner text, and Decision 11's explicitly-deferred submodule/sparse-cone interaction |
| risk.fit | low | extends the existing worktree.acquire/pool/banner primitives per Decisions 1-2 (rejected both a plumbing-commit primitive and a new docs-specific tool), matching the shape rationale_query shows for the origin ticket |
| risk.test | moderate | Phase 1's own contract text flags git-version-dependent behavior ("check what git sparse-checkout set actually writes in this Git version") as something the implementer must verify empirically, not something already pinned |
| risk.security_or_contract | moderate | changes a shipped MCP tool's required-args schema (worktree.acquire) and wsgit's refusal wire text that other shipped rsrc playbooks and tests depend on verbatim, governed by shipped-surface-boundary.md and ws-mcp.md |

## Phases

### Phase 1: `worktree.acquire` sparse shape and direct base checkout

Goal: `worktree.acquire(base, sparse_paths: [...])` provisions a pooled
worktree that materializes only the cone directories; `target_branch` becomes
optional and its omission checks `base` out directly.

Contract:

- Schema (`server.go` tool list, `worktree.acquire`): add
  `sparse_paths` (array of strings); `required` becomes `["base"]`. Exact
  description texts:
  - `target_branch`: `Branch to create (when absent) and check out in the
    worktree, e.g. impl/<parent>/<slug>. Omit to check out base itself; base
    must then be a local branch not checked out in another worktree.`
  - `sparse_paths`: `Optional cone-mode sparse-checkout directories, e.g.
    ["ai-docs"]. Omitted: full checkout. A pooled worktree is reused only when
    its sparse state matches.`
  - Tool description: append one sentence to the existing text: `With
    sparse_paths the worktree is created without a checkout and materializes
    only those cone directories — the lead's housekeeping worktree while a
    worker occupies the root.`
- Dispatch handler: `target_branch` optional; pass `sparse_paths`
  (`stringList`) into `provisionWorktree`. Keep the mint/detach-on-mint-failure
  behavior.
- `provisionWorktree(ctx, runner, root, base, targetBranch string, sparsePaths
  []string, poolConfigValue string)`:
  - Reuse scan: a candidate is eligible only when, in addition to today's
    rules, its sparse state equals `len(sparsePaths) > 0`
    (`wsdoc.SparseCheckoutActive` or `git config --get core.sparseCheckout`,
    whichever the implementer verifies is cheapest and correct for a linked
    worktree — sparse-checkout config is per-worktree via
    `extensions.worktreeConfig`, so check what `git sparse-checkout set`
    actually writes in this Git version and read that).
  - New sparse worktree: `git worktree add --detach --no-checkout <path>
    <base>`, then `git sparse-checkout set --cone <paths...>` in the new
    worktree, then the existing branch step populates the tree (`git switch`
    honors the sparse patterns). Suggested, not mandated: the implementer
    verifies the exact sequence with the tests below.
  - Sparse reuse: re-run `git sparse-checkout set --cone <paths...>` before
    the branch step so a stale pattern set never survives.
  - Branch step when `targetBranch == ""`: `git switch --no-guess -- <base>`;
    on failure return the git error wrapped as
    `worktree.acquire: check out base %q: %v; the actual checkout is held
    elsewhere — release the worktree holding it first, or pass target_branch`.
    When `targetBranch != ""` the existing create-or-switch logic is
    unchanged.
  - Hygiene, submodule sync, and `releaseWorktree` unchanged. Record in the
    Result what `git submodule update --init --recursive` did in a sparse
    worktree whose `.gitmodules` names a path outside the cone (Decision 11).
- The `pool` and `worker_key` result fields are unchanged; add
  `sparse: true|false` to `worktreeAcquireResult` (JSON and text) so a caller
  can see the shape it got.

Verification (extend `worktree_tools_test.go` with the existing temp-repo
harness):

- `TestProvisionWorktreeSparsePaths`: repo with `ai-docs/x.md` and `src/y.go`;
  acquire with `sparse_paths: ["ai-docs"]`; assert `ai-docs/x.md` exists and
  `src/y.go` does not in the worktree, and `git sparse-checkout list` reports
  `ai-docs`.
- `TestProvisionWorktreeSparseReuseMatchesShape`: a released sparse worktree
  is reused by a sparse request (and the pattern set is re-applied) and
  skipped by a full request; a released full worktree is skipped by a sparse
  request.
- `TestProvisionWorktreeBaseDirectCheckout`: `target_branch` omitted checks
  `base` out (`git symbolic-ref --short HEAD` == base) and a commit made there
  advances `base`.
- `TestProvisionWorktreeBaseDirectCheckoutRefusesHeldBranch`: when `base` is
  checked out in the primary worktree the call fails and the error contains
  `check out base`.
- `TestWorktreeAcquireRejectsNonLeadAndBadArgs`: update — `target_branch`
  missing is no longer an error; `base` missing still is.
- `TestWorktreeAcquireReleaseDispatch`: add a sparse variant asserting the
  `sparse: true` field and that `git.commit` through the returned
  `worker_key` succeeds on a file under `ai-docs/`.
- `cd agents-plugin-tool && go test ./...` green.

### Result (76a45c38) - 2026-09-18

Delivered as specified. `worktree.acquire` gained `sparse_paths` (cone-mode dir
list) and made `target_branch` optional; schema `required` is now `["base"]`
with the three settled description texts and the appended tool-description
sentence. `worktreeAcquireResult` carries `sparse: true|false` (JSON and text).

Sparse provisioning sequence: `git worktree add --detach --no-checkout <base>`,
then `git sparse-checkout set --cone <paths...>` in the new worktree, then the
branch step populates only the cone. Cone patterns are re-applied on every
sparse acquire (new and reused) so a stale pattern set never survives. Reuse
shape-matching skips a candidate when `wsdoc.SparseCheckoutActive(e.Path) !=
(len(sparsePaths) > 0)` (verified cheapest correct read for a linked worktree,
since `git sparse-checkout set` writes per-worktree `config.worktree`).
Direct-base checkout (`targetBranch == ""`) runs `git switch --no-guess --
<base>` and wraps a held-branch failure with the settled "check out base %q ...
release the worktree holding it first, or pass target_branch" error.
`releaseWorktree` is unchanged (reset/clean/detach only; no `sparse-checkout
disable`), so a released sparse worktree keeps its cone for reuse.

Decision 11 (submodule sync in a sparse worktree whose `.gitmodules` names a
path outside the cone): empirically pinned on git 2.50.1. Cone mode always
materializes the root-level `.gitmodules`, so the existing gate
(`os.Stat(.gitmodules)`, worktree_tools.go:360) fires and `git submodule update
--init --recursive` runs. It is **not** a no-op: it clones and checks out the
out-of-cone submodule path (e.g. `vendor/sub`) into the worktree, exit 0, no
error and no warning. Housekeeping provisioning therefore still pays the
submodule clone cost even when the submodule sits outside the cone; the result
is correct (no failure), just not maximally lean. No shipped behavior change was
made for this — the ticket only asked to record it.
> Forward: a leaner housekeeping cone in a submodule-heavy downstream repo would
> want to skip or shallow out-of-cone submodule sync; candidate follow-up idea
> if provisioning cost is observed to matter.

Tests (all green, `env -u WS_MAILBOX go test ./...`):
`TestProvisionWorktreeSparsePaths`, `TestProvisionWorktreeSparseReuseMatchesShape`
(sparse-reuses-sparse, sparse-skips-full, full-skips-sparse),
`TestProvisionWorktreeBaseDirectCheckout` (commit advances base),
`TestProvisionWorktreeBaseDirectCheckoutRefusesHeldBranch`,
`TestWorktreeAcquireSparseDispatch`; `TestWorktreeAcquireRejectsNonLeadAndBadArgs`
updated to drop the now-invalid missing-`target_branch` case while keeping the
missing-`base` guard.

### Phase 2: occupied-root banner, `git.commit` refusal hint, `git.merge` held-target refusal

Depends on Phase 1 (the banner names the landed `sparse_paths` argument, and
the held-target refusal resolves the same `worktree_pool` config).

Contract:

- New `occupiedRootAnnouncement(root string) string` in
  `internal/mcp/occupied_root_announcement.go`, injected next to
  `scopeAnnouncement` on both the FRESH-with-root and CONTINUE paths of
  `handleWorkflowManual`, **gated by the caller** (Decision 6): the
  FRESH-with-root path always injects (its key is minted with `parent: ""`);
  the CONTINUE path injects only when `rec.Parent == ""`. The function
  resolves `git symbolic-ref --quiet --short HEAD` in `root` (one
  subprocess); any error, detached `HEAD`, or a branch without the `impl/`
  prefix returns `""`. Parent via `parseImplBranchRoot`.
  Exact text with a parent (`<parent>` and `<branch>` substituted):

  ```text
  > **Occupied root.** `HEAD` here is on `<branch>`, a worker's branch (live, or not yet restored by the lead). Do not commit, move tickets, or switch branches in this root: the write lands on the worker's branch. Housekeeping meanwhile: `worktree.acquire(base: "<parent>", sparse_paths: ["ai-docs"])` checks `<parent>` out in a sparse pooled worktree; commit there with the returned key, and `worktree.release(key: ...)` it before anything checks `<parent>` out elsewhere.
  ```

  Rootless `impl/<stem>` (no parent): the same text with the third sentence
  replaced by:

  ```text
  Housekeeping meanwhile: `worktree.acquire(base: <the branch this worker was spawned from>, sparse_paths: ["ai-docs"])` checks that branch out in a sparse pooled worktree; commit there with the returned key, and `worktree.release(key: ...)` it before anything checks that branch out elsewhere.
  ```

- `wsgit.Client.Commit` mismatch refusal: when `actualBranch` has the `impl/`
  prefix, append this suffix (with `<parent>` from `parseImplBranchRoot`'s
  equivalent in `wsgit` — add a small local helper rather than importing
  `internal/mcp`; when no parent parses, substitute `<the branch this worker
  was spawned from>` without quotes):

  ```text
  ; the actual checkout is a worker's impl/ branch — housekeeping meanwhile goes through worktree.acquire(base: "<parent>", sparse_paths: ["ai-docs"]) and its returned key, not this root
  ```

  The detached-`HEAD` refusal is unchanged.

- Ticket-scope banner under cone mode (Decision 13): `wsdoc.TicketScopeInfo`
  gains `Cone bool`, set from `core.sparseCheckoutCone` read through the same
  config files `newTicketScope` already consults (so a linked worktree's
  `config.worktree` is honored). `scopeAnnouncement` returns `""` when
  `info.Cone`. `SparseCheckoutActive`, `newTicketScope`'s activation rule,
  and every other `TicketScope` consumer are unchanged. The Result notes the
  Decision 13 side effect (a cone checkout excluding `ai-docs` no longer gets
  a hidden-stem notice) in one line.

- `git.merge` held-target refusal (Decision 14): `mergeImplBranch` gains a
  trailing `poolRoot string` parameter (`""` = unknown). A new closure
  `checkTargetHeld`, called once, right after the first `checkWorktree()`
  and before the pre-switch `checkTips()` — not inside `checkWorktree`,
  whose post-switch pass could only ever match the root itself — calls
  `listWorktrees(ctx, runner, root)`; on error,
  `gitFailure("worktree_inspection", "Cannot list worktrees", ...)`.
  Otherwise, when an entry has `Branch == "refs/heads/"+mergeRoot` and its
  `Path` differs from the session root (compare through
  `canonicalGitRoot(root)`; fall back to `filepath.Clean(root)` when that
  fails), `add("target_held_elsewhere", <reason>, <resolution>)`.
  `worktreeEntry` gains `Prunable bool`, set when the porcelain record has a
  `prunable` line (a worktree whose directory is gone but whose record still
  holds the branch for git's checkout rule). Reason text, `<target>`,
  `<path>`, `<branch>` substituted, where `<kind>` is `a ws pool worktree: a
  parallel lead's housekeeping checkout (worktree.acquire with sparse_paths)`
  when `poolRoot != ""` and `<path>` is under it, else `another worktree`:

  ```text
  Target "<target>" is checked out at <path>, <kind>. Nothing was merged; "<branch>" is retained and HEAD is unchanged.
  ```

  Resolution text:

  ```text
  Do not remove, switch, or force that worktree. Treat this as a merge stop: report that "<target>" is held elsewhere, end the turn with the branch retained, and retry git.merge unchanged once the holder has released it (worktree.release for a pool worktree).
  ```

  When the holder is `Prunable`, `<kind>` is `a stale worktree record whose
  directory is gone` regardless of pool membership, and the resolution is
  instead:

  ```text
  The holder cannot release a branch it no longer has. Run `git worktree prune` in the repository, then retry git.merge unchanged.
  ```

  The `server.go` `case "git.merge"` handler resolves `poolRoot` the way
  `case "worktree.acquire"` does (`wsconfig.ItemWorktreePool` through the
  session-key resolver, then `resolvePoolRoot(value, mainRoot)` with
  `mainRoot` = the first `listWorktrees` entry's path); any failure in that
  resolution passes `""` rather than failing the merge. The existing
  `switch_failed` diagnostic is unchanged: it now only reaches the
  overlapping-change case it describes. `mergeImplBranch`'s other callers (tests)
  pass `""`.

Verification:

- `git_merge_test.go` `TestImplMergeRefusesTargetHeldElsewhere`: from
  `mergeFixture(t, "develop")`, `git worktree add <t.TempDir()>/pool/held
  develop`; `mergeImplBranch(..., poolRoot: <t.TempDir()>/pool)` returns
  `policy_blocked`, an error containing `checked out at`, a
  `target_held_elsewhere` `must_resolve` diagnostic whose reason contains
  `ws pool worktree` and the held path, `HEAD` unchanged, and `develop`'s
  tip unchanged; the same with `poolRoot: ""` (or a holder outside the pool)
  says `another worktree` and not `ws pool worktree`. A third case with no
  other worktree merges as before, proving the inspection is inert. A fourth
  case removes the held worktree's directory with `os.RemoveAll` (no
  `prune`), so the record is prunable: the reason contains `stale worktree
  record` and the resolution contains `git worktree prune`; after `git
  worktree prune` the merge lands.
- `worktree_tools_test.go`: `listWorktrees` sets `Prunable` for a record
  whose directory was removed and leaves it false otherwise.
- `TestImplMergeTolerantWorktree`'s overlapping-change case still yields
  `switch_failed`, unchanged.

- `occupied_root_announcement_test.go` on the `scope_announcement_test.go`
  pattern: banner present on FRESH-with-root and CONTINUE when the temp repo's
  `HEAD` is `impl/develop/foo` (asserting `base: "develop"`), present without
  a `base:` quote for `impl/foo`, absent on `develop` and on detached `HEAD`;
  absent on CONTINUE for a key minted with a non-empty `parent` (mint one
  through the store as `playbook.render` does) even though the root's `HEAD`
  is `impl/develop/foo`.
- `scope_announcement_test.go`: a worktree with `git sparse-checkout set
  --cone ai-docs` renders no `Sparse-checkout scope is active` banner on
  either path; the existing `--no-cone` cases (including
  `TestScopeAnnouncementFiresWithNoTicketHidden`) are unchanged.
- `wsgit/git_test.go`: mismatch refusal on an `impl/develop/foo` checkout
  contains `worktree.acquire(base: "develop"`; on a `feature/x` checkout it
  does not contain `worktree.acquire`; detached refusal text unchanged.
- `go test ./...` green.

### Result (a0093703) - 2026-09-18

Delivered as specified.

`occupiedRootAnnouncement` (new `occupied_root_announcement.go`): one `git
symbolic-ref --quiet --short HEAD`; detached `HEAD`, non-`impl/` branch, or any
error return `""` (advisory only, never a render failure). Parent via
`parseImplBranchRoot`; the rootless `impl/<stem>` case (`parent == ""`) uses the
"the branch this worker was spawned from" text variant. Gated by the caller in
`handleWorkflowManual`: the FRESH-with-root path always injects (its key is
minted with `parent: ""`); the CONTINUE path injects only when `rec.Parent ==
""` — so a worker never sees the "do not commit here" banner about its own
branch (Decision 6).

`wsgit.Client.Commit`: the impl-branch escape-hatch suffix is appended only when
`actualBranch` has the `impl/` prefix (quoted parent when it parses, unquoted
"the branch this worker was spawned from" otherwise). A local `implBranchParent`
twin avoids an mcp→wsgit import cycle; its `idx <= 0` guard and mcp's `idx < 0`
differ only for the degenerate `impl//stem` and both yield "no parent". The
detached-`HEAD` refusal is unchanged.

Decision 13 (ticket-scope banner under cone mode): `TicketScopeInfo` gained
`Cone bool`, set from `core.sparseCheckoutCone` read through the same config
files `newTicketScope` already consults (so a linked worktree's `config.worktree`
is honored). `scopeAnnouncement` returns `""` when `info.Cone`.
`SparseCheckoutActive`, the activation rule, and the `tickets.move`/`close` gate
stay keyed on `Active`, so `git.commit`'s sparse staging in a cone worktree still
works. Side effect (recorded per contract): a cone checkout that excludes
`ai-docs` no longer receives a hidden-stem notice; acceptable because the
scope-banner remedy ("restore with git sparse-checkout disable") would be false
under a deliberately-shaped development/housekeeping cone.

Decision 14 (`git.merge` held-target refusal): `mergeImplBranch` gained a
trailing `poolRoot string` param. A new `checkTargetHeld` closure runs once,
right after the first `checkWorktree()` and before `checkTips()`/switch; when a
`listWorktrees` entry holds `refs/heads/<mergeRoot>` at a path other than the
session root it adds a `target_held_elsewhere` (`must_resolve`) diagnostic, and
the diagnostics check returns `blocked()` before anything is switched or merged
— HEAD and the target tip stay put. `<kind>` is the pool-housekeeping text when
`poolRoot != ""` and the path is under it, "another worktree" otherwise, and the
stale-record text with the `git worktree prune` resolution when the holder is
`Prunable`. `worktreeEntry` gained `Prunable bool` from the porcelain `prunable`
line. `server.go` resolves `poolRoot` the way `worktree.acquire` does
(`ItemWorktreePool` via the session-key resolver, `resolvePoolRoot(value,
mainRoot)`), failing open to `""` rather than failing the merge.

Correctness-review Minor (recorded, not fixed — inert for this contract): in
`checkTargetHeld` the session-root skip compares a symlink-resolved
`canonicalGitRoot` against a non-resolved `filepath.Clean(e.Path)`. It could
mis-skip only in a degenerate self-merge (source branch == target), which is
already caught earlier as `already_contained`; the branch filter
`e.Branch != "refs/heads/"+mergeRoot` excludes the session-root entry first in
every real flow.

Tests (green): `TestImplMergeRefusesTargetHeldElsewhere` (pool-holder,
plain-holder, no-other-worktree-merges, prunable-holder incl. post-`prune`
success), `TestListWorktreesPrunable`, `TestOccupiedRootAnnouncement*`
(parent-gating on FRESH vs CONTINUE vs child-key),
`TestScopeAnnouncementSilentUnderConeMode` (the `--no-cone` cases still fire),
`TestCommitRefusesImplBranchNamesEscapeHatch`.

### Phase 3: prose at the write sites and mirrors

Depends on Phase 2 (the prose refers to the banner).

Edit exactly these three rsrc playbooks; the wording below is settled — apply
it verbatim, then run the mirror/manifest regens from `## Constraints`.

- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`, `### Git`,
  append as a new final paragraph:

  ```text
  A root whose `HEAD` is on an `impl/*` branch is a worker's until the lead
  restores it (the occupied-root banner names the parent branch): a commit or
  ticket move there lands on the worker's branch. Housekeeping meanwhile goes
  through `{{.McpNamespace}}/worktree.acquire(base: <parent branch>,
  sparse_paths: ["ai-docs"])`, which checks the parent branch out in a sparse
  pooled worktree and returns the key to commit with there; release it with
  `{{.McpNamespace}}/worktree.release(key: <that key>)` before anything checks
  the parent branch out elsewhere, because Git holds one checkout per branch:
  a `{{.McpNamespace}}/git.merge` into a branch still held that way is
  refused, not stolen.
  ```

- `agents-plugin/rsrc/lead-run/lead-run.md`, Spawn step 6, replace the whole
  item with:

  ```text
  6. Wait for the host's completion notification. Do not poll, and do not edit
     the ticket or move `HEAD` meanwhile; housekeeping that cannot wait uses
     the sparse worktree the workflow manual's `### Git` section describes,
     released before **Handle the report** merges.
  ```

- `agents-plugin/rsrc/lead-run/lead-run.md`, `## Handle the report`, in the
  paragraph beginning `Merging is yours.`, insert after the sentence ending
  `as a bounded resolution task.`:

  ```text
  A `target_held_elsewhere` refusal means the target branch is checked out
  in another worktree, typically a parallel lead's sparse housekeeping
  worktree: that is a merge stop, not a conflict — leave the impl branch
  retained and `HEAD` where it is, report the holder's path, and end the
  turn; the next invocation merges once the holder has released it.
  ```

- `agents-plugin/rsrc/lead-ticket/lead-ticket.md`, `## Output`, insert after
  the sentence ending `the commit is refused if the checkout has since moved.`:

  ```text
  A root occupied by a worker (`HEAD` on `impl/*`) takes no ticket write:
  author and commit in the sparse worktree the workflow manual's `### Git`
  section describes, with the key it returns.
  ```

- `lead-discuss` is deliberately untouched (Decision 7).

Verification:

- Regens, the `agents-plugin-pi/rsrc/` byte copy, and `python3 -m unittest
  discover agents-plugin-wsflow/tests` from `## Constraints`; `go test ./...`
  green (rsrc manifest, wsflow mirror, and pi mirror drift guards).
- One fresh-reader audit pass over the four changed paragraphs per
  `skill-authoring.md` `## Audits`; record findings and their classification
  in the Result.
- Dogfood once and record it in the Result: with a throwaway
  `impl/develop/<slug>` checked out in a scratch clone, `workflow_manual`
  shows the banner, `worktree.acquire(base: "develop", sparse_paths:
  ["ai-docs"])` returns a sparse worktree on `develop`, `workflow_manual`
  with the returned key shows neither the occupied-root nor the
  sparse-checkout-scope banner, a `git.commit` through that key lands on
  `develop`, a `git.merge` of the throwaway impl branch from the scratch
  clone's key is refused with `target_held_elsewhere` naming the pool path,
  `worktree.release` detaches the sparse worktree, and the same `git.merge`
  then lands.

### Result (0fa9d218) - 2026-09-18

Delivered as specified. The four settled paragraphs were applied verbatim at the
three canonical write sites (`agents-plugin/rsrc/lead-workflow-manual/…`,
`lead-run/…` Spawn step 6 and Handle-the-report, `lead-ticket/…` Output);
`lead-discuss` untouched (Decision 7). Regenerated `manifest.json` and produced
byte-identical `agents-plugin-wsflow/rsrc/` and `agents-plugin-pi/rsrc/` mirrors
(12 files). Verified byte-identity of all three changed files across both
mirrors; `go test ./...` drift guards (rsrc manifest, wsflow mirror, pi mirror)
and `python3 -m unittest discover agents-plugin-wsflow/tests` (12 tests) green.

Fresh-reader audit (per `skill-authoring.md` `## Audits`), four changed
paragraphs; verdict: no blocking issues. Findings and classification:
- "…is a worker's until the lead restores it" — reader flagged "restores it" as
  an unoperationalized end-state (important-as-read). Classification: non-blocking
  against verbatim-settled prose; the action is operationalized downstream in
  `lead-run` Handle-the-report (the parent branch is checked back out as part of
  merging the impl branch). Not changed (prose is settled).
- "(the occupied-root banner names the parent branch)" — flagged as an orphaned
  reference cited as the source for the `base:` argument. Classification:
  false-positive at the shipped surface — the banner is a real runtime artifact
  (`occupiedRootAnnouncement`, Phase 2) injected into the *same* `workflow_manual`
  output that carries this `### Git` section, so a reader sees the banner and the
  reference together. Not changed.
- lands-on-worker-branch (manual) vs takes-no-ticket-write (lead-ticket) — reader
  read a surface tension. Classification: minor, reconciled by `git.commit`'s
  `expected_branch` guard (refuses a stale-checkout commit); both are true. Not
  changed (settled prose).
- "housekeeping that cannot wait" unscoped; `target_held_elsewhere` "typically"
  hedge; lead-ticket Output not locally restating `worktree.release` — all minor,
  the safe fallback matches the cautious default and the release obligation is
  stated in the cross-referenced `### Git` section. Not changed.

Dogfood (recorded per contract): driven against a freshly built `ws-mcp serve
--stdio` over a scratch repo with a throwaway `impl/develop/throwaway-slug`
checked out in the root. Observed, in order: FRESH `workflow_manual` shows the
occupied-root banner naming `base: "develop"` and mints the lead key;
`worktree.acquire(base: "develop", sparse_paths: ["ai-docs"])` returns
`sparse: true` on a worktree whose `HEAD` is `develop` with `ai-docs/` present
and out-of-cone `src/` absent; `workflow_manual` with the returned worker key
shows neither the occupied-root nor the sparse-checkout-scope banner; a
`git.commit` through that key lands on `develop`; `git.merge impl/develop/throwaway-slug`
is refused (MCP error) with text `Target "develop" is checked out at <pool path>,
a ws pool worktree: a parallel lead's housekeeping checkout (worktree.acquire
with sparse_paths). Nothing was merged; "impl/develop/throwaway-slug" is retained
and HEAD is unchanged.` — HEAD and branch confirmed unmoved; `worktree.release`
detaches the sparse worktree; the same `git.merge` then lands (`status: merged`,
`branch_deleted: true`, source now an ancestor of `develop`).

## Sage Review Round 1 (2026-09-18)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Occupied-root banner fires for the worker whose branch it is | important | missing |
| 2 | Cone-sparse housekeeping worktree silently activates the ticket-scope banner | important | missing |
| 3 | Phase 3 omits the agents-plugin-pi rsrc mirror resync | minor | autonomous |
| 4 | Prior Art mis-describes the wsgit refusal-text assertions | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
