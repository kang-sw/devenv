---
title: Named worktree profiles for worktree.acquire/release
related:
  261020-research-worktree-profiles: origin; its Outcome Ledger (Verified Findings, Confirmed Decisions 1-31) is this ticket's evidence and contract
  260910-feat-lead-run-worktree-parallel-route: origin of worktree.acquire/release, the pool, the conservative reuse rule, and hygiene clean -ffdx
  260918-feat-ws-sparse-housekeeping-worktree-occupied-root: added sparse_paths and the boolean sparse-shape reuse match that profiles replace
  260917-feat-ws-committed-project-config-scope: repo config scope; its "config.tune does not write repo for now" choice is reversed here
  260806-feat-worktree-ticket-scope: ticket-board sparse scope and lead-scope-worktree, retired in Phase 4
  260924-research-origin-ticket-ownership-index: references lead-scope-worktree; reference updated in Phase 4
  260923-bug-worktree-acquire-concurrent-pool-claim: adjacent pool-claim race, deliberately not addressed here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 9419f414981f48c5
sage-review-completeness-reviewed: 9419f414981f48c5
---

# Named worktree profiles for worktree.acquire/release

## Background

A downstream project (a large C++/MSBuild Windows repository, about 22 GB per
full checkout plus Debug build) wants `worktree.acquire` as the standard entry
point for build-capable parallel worktrees. Today acquire/release serves two
uses well (full-checkout code workers, `["ai-docs"]` housekeeping) but cannot
express a non-cone sparse shape, runs no repository setup step, deletes every
ignored build output on each hygiene pass (`git clean -ffdx`), and mixes all
uses in one pool because reuse matches only a boolean "sparse active". It also
breaks in submodule repositories.

The fix is a named worktree profile: one unit that binds reuse partitioning,
declared sparse shape, preserved ignored paths, and lifecycle hooks. The
discussion and evidence live in `261020-research-worktree-profiles`; this
ticket carries its Confirmed Decisions as contract.

## Decisions

### Profile model and config

- **Profile = pool-partition key + lifecycle definition.** A pooled worktree
  is reused only by an acquire naming the same profile.
- **Defined in layered config**, one key per profile:
  `worktree.profile.<name>`. Key dispatch gains this as a prefix family
  alongside `prompt.`. The resolver gains structured (JSON object) values
  instead of adding a second resolver-bypassing writer like `agents.tier`.
  Removal is `config.tune(reset: true)`.
  Rejected: a runtime `worktree.profile.set` MCP registration (profiles must be
  versioned with the repository's scripts); a single `worktree.profiles` map
  key (per-scope whole-value override would prevent per-profile layering).
- **Scopes.** Profile definitions are accepted from `project`, `repo`,
  `global`, and `builtin`; never from `session`. Hooks are accepted from every
  one of those scopes. Rejected: hooks only from hand-edited files (the user
  does not hand-author files; an agent would edit the file anyway).
- **`config.tune` writes the `repo` scope** (`.ws-workflow/config.json` in the
  caller's worktree), following the `note.write(layer: "repo")` precedent: the
  tool writes the tracked file and staging rides the caller's `git.commit`.
  This applies to every repo-scoped key, not only profiles; keys the repo
  layer never reads (global-only keys, `agents.tier`) refuse `scope: "repo"`.
  The write path resolves the repo root from the session (the
  `sessionConfigOptions` anchoring `config.list` already uses), not the empty
  `wsconfig.Options{}` today's tune writers build. The tune result names the
  written path and says it must be included in the caller's next
  `git.commit` paths. Rejected: keeping `repo`
  read-only (260917's "for now" choice) - it protects nothing when agents edit
  the file and skips write-time validation on the scope that holds most
  profiles.
- **Forward-compatible on-disk layout.** Older ws versions parse every config
  file into `map[string]string` overrides and fail the whole resolution on a
  parse error, so a profile object must never be stored as a value inside
  `overrides`. Store structured values in a separate top-level member that an
  older reader's `json.Unmarshal` ignores: `structured_overrides`, keyed by
  the full config key (`"worktree.profile.<name>": { ... }`), mirroring
  `overrides`. The member name is fixed here because it is a committed
  on-disk format downstream repositories carry. Rejected: an object under
  `overrides` (takes down every knob for a teammate on an older plugin);
  string-encoded JSON in `overrides` (unreadable for agents editing the file).
- **Override granularity.** A higher scope replaces a whole profile; no field
  merge. Builtin profiles can be overridden by name.
- **Name validation:** `^[a-z][a-z0-9-]*$`.
- **Schema and validation.** The `config.list` catalog shows the family
  pattern with a `value_schema` (JSON Schema) plus the resolved profiles with
  per-field provenance scope. Validation runs at `config.tune` write and at
  resolve time; a bad committed file fails acquire and manual render loudly,
  naming the file and key.
- **Profile shape** (field names normative):
  ```jsonc
  {
    "description": "...",
    "sparse":   { "mode": "cone" | "patterns", "paths": [ ... ] },
    "preserve": [ "x64/", "*/x64/", ... ],
    "hooks": {
      "post_provision": [ { "argv": [ ... ], "cwd": null } ],
      "post_acquire":   [ ... ],
      "pre_release":    [ ... ],
      "pre_remove":     [ ... ]
    }
  }
  ```
  - `sparse.mode: "patterns"` is non-cone sparse-checkout, kept alongside cone
    so an exclusion ("everything except `_deps/`") is declarable without shell
    or MSYS involvement. An absent `sparse` means a full checkout.
  - Declared sparse is applied when a worktree enters the profile (fresh
    provision, or re-adoption after a definition change), not on every
    acquire; sparse changes a hook makes afterwards persist across reuse. This
    removes today's narrow-then-widen rewrite of out-of-cone files on every
    reuse.
  - `preserve` patterns are excluded from the hygiene `git clean` in both
    acquire and release (for example via `git clean -ffdx -e <pattern>`).
- **Builtin profiles** replace `sparse_paths`: `full` (no sparse) and
  `housekeeping` (cone `["ai-docs"]`). `sparse_paths` is removed from the
  acquire schema; the workflow schema is not a durable artifact. Rejected:
  keeping `sparse_paths` as an anonymous ad-hoc profile (blurs the
  pool-partition key).
- **Profile is a required acquire parameter.** No default-profile knob
  (rejected: the lead must choose explicitly). The workflow manual's
  worktree guidance renders the resolved profile list (name, description,
  provenance scope) and tells the lead to pass a profile to `acquire` for
  workers. No separate `worktree.profile.list` tool.
- **Profile marker.** A marker file in the worktree's git admin dir records
  the profile name plus a hash of the profile's provisioning inputs and
  survives release (the lease file is deleted on release, so it cannot carry
  the profile). The hash covers only `sparse` and `hooks.post_provision`, so
  editing `description`, `preserve`, or another hook does not re-provision the
  pool. A hash mismatch at acquire means a provisioning change: re-apply
  declared sparse and run `post_provision` (re-adoption). Rejected: hashing the
  whole definition (re-provisions every pooled worktree on a description
  edit, costly for large checkouts).
- **Ticket visibility invariant.** After every acquire-path hook (that is,
  after `post_acquire`, since a hook may narrow sparse), an acquired worktree
  must expose `ai-docs/tickets` (workers record results into the ticket);
  otherwise acquire fails loudly naming the profile.

### Lifecycle and hooks

- **Hook points.**
  - `post_provision`: once when a worktree enters the profile (fresh create
    or re-adoption), after the declared sparse is applied and checked out.
  - `post_acquire`: every acquire, after hygiene and branch switch (after
    `post_provision` on a fresh one).
  - `pre_release`: before release hygiene.
  - `pre_remove`: before `worktree.remove` deletes the worktree.
- **Hook execution.** Each hook is an argv array (not a shell string), default
  cwd = the worktree (`cwd` is relative to it when set). Values reach hooks by
  env vars `WS_WORKTREE_DIR`, `WS_BASE`, `WS_TARGET_BRANCH`, not `${...}`
  templating. No `WS_REUSED`: the hook choice already distinguishes fresh
  from reuse.
- **Hook timeout.** Each hook runs under a fixed 10-minute timeout; a timeout
  is a hook failure (fail-loud, quarantine where applicable). Rejected: no
  timeout (a hanging hook would block the MCP call indefinitely); a
  per-hook timeout field (not in the agreed profile shape).
- **Hook provenance.** acquire/release/remove results report each executed
  hook's argv, source scope, and exit code.
- **Hook failure is fail-loud.** A failing hook fails its operation and the
  error strongly directs the agent to fix the profile/pool. A failing
  `pre_release` or `pre_remove` leaves the worktree in its pre-operation
  state; a failing `post_provision` or `post_acquire` follows Quarantine
  below (a fresh worktree has no pre-operation state, and Quarantine runs
  release-equivalent hygiene).
- **Quarantine.** On `post_acquire` failure: run release-equivalent hygiene
  (without `pre_release`) so the target branch is freed, mark the worktree
  quarantined in its marker (argv, scope, exit code, stderr tail, time), and
  return an error that counts the profile's quarantined worktrees (2+ means a
  likely profile/hook defect). Never auto-retry on a fresh worktree. A
  `post_provision` failure uses the same quarantine. A quarantined worktree is
  never reused; only `worktree.remove` clears it. A quarantined worktree
  holds no lease: `post_provision` and `post_acquire` run inside
  provisioning, before the dispatcher writes the lease (today it writes the
  lease only after provisioning succeeds), so the lease-recency refusal of
  `worktree.remove` never blocks clearing a fresh quarantine. Rejected: an
  explicit `clear_quarantine` on release (would return a possibly broken
  worktree to the pool).
- **`post_provision` outputs** (override files, hardlinks) survive hygiene
  only when listed in `preserve`. The acquire result warns when a path a
  `post_provision` hook created is ignored and not covered by `preserve`.
  Rejected: documentation only (a result warning is seen where docs are not).
- **Legacy adoption.** Pooled worktrees without a marker are adopted lazily
  into builtin profiles only, by today's boolean rule (sparse-active =>
  `housekeeping` whatever its cone, otherwise `full`), stamping the marker on
  claim with an empty hash so the first claim re-adopts once and normalizes
  an arbitrary prior cone to the builtin's declared sparse. Custom profiles never adopt unmarked entries. Releasing an unmarked
  leased worktree runs today's hygiene only. `worktree.list` labels unmarked
  entries `legacy`.

### `worktree.remove` and the nudge

- **`worktree.remove`** is a new tool. It checks every refusal below first,
  then runs `pre_remove`, removes the git worktree, and cleans the lease and
  marker; a refused target never runs a hook. `pre_remove` comes from the
  profile named in the marker; when that profile no longer resolves (deleted
  or renamed), remove skips `pre_remove` and says so in its result, so a
  worktree of a removed profile stays removable. It keeps the checked-out branch
  (as `git worktree remove` does), so its refusals guard unmerged work
  rather than branch deletion: it refuses a dirty worktree, one whose
  checked-out branch holds commits reachable from no other branch, and one
  whose detached `HEAD` holds commits reachable from no ref.
  - It also refuses the primary worktree and any target outside the owned
    pool roots, as `releaseWorktree` already does
    (`agents-plugin-tool/internal/mcp/worktree_tools.go#L425-L429`).
  - It refuses a leased worktree whose activity (the nudge's
    `max(lease acquired_at, HEAD commit time)`) is newer than
    `worktree.nudge_stale_days`, so a live worker's clean, commit-less
    worktree cannot be removed under it while the nudge-listed stale leases
    stay removable. Rejected: refusing every leased worktree (blocks the
    nudge's own remedy; a stale lease would need a release that runs hygiene
    on abandoned work); a `force` parameter (new schema surface); no lease
    check.
- **Stale-pool nudge**, at most once a day per pool, at acquire time, listing
  worktrees with a checked-out branch and no activity for 7 days or more, or
  detached with 1 GB or more of preserved ignored artifacts.
  - Activity = `max(lease acquired_at, HEAD commit time)`.
  - Preserved size is measured at release and recorded in the marker; it is
    never walked at acquire.
  - The throttle timestamp is stored per pool in the git common dir.
  - Thresholds are config knobs `worktree.nudge_stale_days` (default 7) and
    `worktree.nudge_preserved_gb` (default 1), outside the
    `worktree.profile.` family.
  - The workflow manual's worktree guidance (where the profile list renders)
    tells the lead to relay the nudge to the user.
  Rejected: auto-deleting stale worktrees.

### Submodules (downstream Issue 4)

- Every checkout-family git invocation on the acquire, release, and remove
  paths passes `-c submodule.recurse=false`: `worktree add`,
  `sparse-checkout set`, `reset --hard`, `clean`, `switch`, and
  `checkout --detach` (including the dispatcher rollback in `server.go`). The
  explicit `git submodule update --init --recursive` stays.

### Pi package

- `agents-plugin-pi/` is in scope: every changed shipped playbook under
  `agents-plugin/rsrc/` is resynced into its byte-mirrored `rsrc/` copy
  (`ai-docs/manuals/ws-mcp.md`; `TestPiMirrorUpToDate` enforces it), and
  Phase 4 deletes its `lead-scope-worktree` skill, playbook, and manifest
  entries along with the `agents-plugin/` and wsflow copies.

### Ticket-scope retirement (downstream Issue 5)

- Retire `lead-scope-worktree` (skill, playbook, wsflow mirror, manifest
  entries, tests). The user explicitly requested this deletion (an "Always
  ask" item); the ticket-ownership concept supersedes it.
- Delete the `scopeAnnouncement` session-bootstrap banner. Acquired worktrees
  never run `workflow_manual` (they are subagent implementation spaces), so a
  profile-aware replacement is unnecessary.
- Keep generic sparse safety: `SparseCheckoutActive`, `git add --sparse`
  staging, the move/close `check-rules` gate, and the create collision
  refusal. Add a `tickets.create` warning when the created path is outside
  the active sparse scope.

## Constraints

- Out of scope: the concurrent pool-claim race
  (`260923-bug-worktree-acquire-concurrent-pool-claim`); pool caps or
  eviction; MSYS path-conversion handling beyond what `patterns` mode avoids.
- The shipped surface stays project-neutral: builtin profiles, manual text,
  and tool descriptions name no downstream path; the downstream's `_deps`/
  `x64` layout appears only as test fixtures' generic stand-ins
  (`ai-docs/manuals/shipped-surface-boundary.md`).
- Every playbook or skill text change follows `ai-docs/manuals/skill-authoring.md`
  and mirrors into wsflow per `ai-docs/manuals/wsflow-mirroring.md`.
- MCP edits follow `ai-docs/manuals/ws-mcp.md`.
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Prior Decisions

- 260917-feat-ws-committed-project-config-scope (2026-09-17, Result e73ed082): "Read-only surface: `ScopeRepo` is excluded from `ScopeSchemaEnum`, so `config.tune` never offers `repo` as a writable scope; the committed file is hand-edited." — bearing: constrains
- 260918-feat-ws-sparse-housekeeping-worktree-occupied-root (2026-09-18, Decisions): "Pool reuse matches sparse shape. A pooled candidate is eligible only when its `core.sparseCheckout` state matches the request; a sparse reuse re-applies the requested pattern set every time." — bearing: constrains
- 76a45c38 (2026-09-18, commit): "Submodule sync stays unconditional; verified empirically that `submodule update --init --recursive` is a clean no-op (exit 0) under a cone that excludes the submodule path (Decision 11)." — bearing: supports
- 76a45c38 (2026-09-18, commit): "Empirically pinned on git 2.50.1: cone worktree materializes only the cone; core.sparseCheckoutCone=true for cone / false for --no-cone; held-branch switch fails exit 128." — bearing: supports
- 95d18ff6 (2026-09-15, commit): "Reuse eligibility is the conservative golden rule from the ticket: under the owned pool prefix, at detached HEAD, and clean. A branch-checked-out or dirty worktree is skipped so no in-progress work is stomped." — bearing: constrains
- 95d18ff6 (2026-09-15, commit): "Branch setup uses `git switch` (branch-only verb) not `checkout`, and never resets an existing branch to base, so committed work on a re-entered target_branch is preserved." — bearing: constrains
- 261020-research-worktree-profiles (2026-10-20, Confirmed Decisions): "`post_provision` outputs (override files, hardlinks) survive only when listed in `preserve`; document this or warn about it (which one is settled when the actionable ticket is derived)." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/worktree_tools.go, server.go, config_registry.go, occupied_root_announcement.go, git_merge.go, scope_announcement.go, workflow_manual.go; internal/wsconfig/scope.go, repo.go; internal/wsgit/git.go; agents-plugin, wsflow, and Pi rsrc/skills lead-workflow-manual, lead-run, lead-scope-worktree |
| scope.surface | public-interface | worktree.acquire schema replaces sparse_paths with required profile; config.tune gains repo scope; shipped skill deletion |
| scope.new_public_symbol | yes | worktree.remove tool, worktree.profile.name config key family, acquire profile parameter, builtin full and housekeeping profiles |
| scope.new_type_contract | yes | profile JSON object with sparse, preserve, hooks fields; structured config values in the wsconfig resolver; profile marker file |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/worktree_tools_test.go, config_repo_scope_test.go, scope_announcement_test.go, agents-plugin/tests/test_skill_dispatch_contracts.py, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py |
| complexity.reuse_points | confirmed | wsdoc.SparseCheckoutActive at internal/wsdoc/tickets_scope.go#L457, prompt. prefix dispatch at config_registry.go#L113, repo scope loader internal/wsconfig/repo.go, lease in git admin dir worktree_lease.go |
| complexity.side_effect_risk | high | destructive reset and clean -ffdx with new exclusions, arbitrary hook argv execution, and new worktree deletion |
| risk.correctness | high | pool partitioning, marker hash re-adoption, quarantine, and preserve exclusions must not lose work or reuse broken worktrees |
| risk.fit | high | spans config resolver, MCP schemas, shipped playbooks with wsflow and Pi mirrors, and a skill deletion |
| risk.test | moderate | needs git fixtures for submodules, cone and patterns sparse, hooks, and live git checks recorded in Results |
| risk.security_or_contract | high | committed repo-scope config can now carry executable hooks and config.tune writes a tracked file; acquire schema breaks callers |

## Phases

### Phase 1: Structured config values, profile key family, repo-scope writes, submodule fix

Deliver the config substrate and the independent Issue 4 fix.

- Resolver structured-value support; `worktree.profile.<name>` prefix family
  in key dispatch; name validation; JSON Schema validation at write and at
  resolve; session-scope refusal for the family; whole-profile override;
  `config.list` catalog entry with `value_schema` and resolved profiles with
  provenance.
- `config.tune` writes the `repo` scope for every key (writes the file, no
  staging).
- Builtin `full` and `housekeeping` profile definitions exist and resolve
  (acquire does not consume them yet).
- `-c submodule.recurse=false` on every checkout-family invocation listed in
  Decisions.

Verification: unit tests for resolver precedence of structured values across
project/repo/global/builtin, session refusal, bad-name and bad-schema
refusal at write, a malformed committed file failing resolve with file and
key named, `config.tune` repo write creating/updating
`.ws-workflow/config.json` without staging and naming the path in its result,
a repo write refused for a global-only key, reset removing a profile, and a
pre-change `Config` parse (plain `overrides` map) of a file carrying a
profile still succeeding. An
acquire test with a submodule fixture under `submodule.recurse=true` passes.

### Result (1cdd2f6f7) - 2026-10-20

- Landed: wsconfig structured values in a separate `structured_overrides`
  member (project > repo > global > builtin, whole-value override, session
  refused); `config.tune` repo-scope writes for every key through a
  member-preserving writer that never stages and names the written path;
  the `worktree.profile.<name>` family with name and strict schema
  validation at write and at resolve (a bad committed profile fails naming
  file and key), builtin `full` and `housekeeping`, and a `config.list` knob
  carrying `value_schema` plus resolved profiles with provenance;
  `-c submodule.recurse=false` on every checkout-family call, including the
  dispatcher rollback.
- Verification: `go test ./...` in `agents-plugin-tool` passed. New tests:
  TestListStructuredPrecedence, TestStructuredRefusesSessionScope,
  TestRepoStructuredWritePreservesOtherMembers,
  TestUnsetStructuredRemovesAndNoOps, TestRepoWriteNeedsRoot,
  TestRepoWriteRefusedForGlobalOnlyKey,
  TestListStructuredMalformedFileNamesPath,
  TestPreStructuredConfigShapeParsesStructuredFile,
  TestParseWorktreeProfile, TestValidateWorktreeProfileName,
  TestBuiltinWorktreeProfilesResolve,
  TestResolveWorktreeProfilesRejectsBadCommittedProfile,
  TestConfigTuneWorktreeProfileRepoWrite, TestConfigTuneScalarRepoWrite,
  TestConfigListWorktreeProfileKnob, TestWorktreeAcquireSubmoduleRecurseTrue.
- Decisions: `structured_overrides` is a new member because an older ws
  decodes `overrides` as `map[string]string`; caveat: an older ws writing the
  project or global file drops the member. The profile family defaults to
  project scope (repo writes a tracked file, so it is explicit); declared
  adapter keys still may not default to repo, and `worktree.` is a reserved
  adapter namespace. Validation is a strict decoder mirroring the published
  schema, not a schema library. `config.list` reports a bad profile as the
  knob's error rather than failing the whole catalog.

### Phase 2: Profile-partitioned acquire/release with hooks and quarantine

Depends on Phase 1. Replace `sparse_paths` with the required `profile`
parameter and implement marker, declared sparse (cone and patterns),
`preserve` in both hygiene points, the four-point lifecycle minus
`pre_remove`, env-var hook execution, provenance reporting, fail-loud
failure, quarantine, the `post_provision` preserve warning, legacy adoption,
and the ticket-visibility check. Land it as two reviewable commits: first
profile partitioning (parameter, marker, builtins, legacy adoption, sparse,
preserve, visibility check), then hooks (execution, provenance, timeout,
failure, quarantine, preserve warning). Update the acquire tool description
(dropping the "sparse state matches" overstatement), `worktree.list`
(profile, `legacy`, quarantine shown), and every caller that passes
`sparse_paths`: the occupied-root banner, the `git.merge` and `wsgit`
refusal strings, the workflow manual's `### Git` housekeeping line (which
also exists in the byte-synced Pi mirror
`agents-plugin-pi/rsrc/lead-workflow-manual/lead-workflow-manual.md#L108`, resynced
from `agents-plugin/rsrc/` per `ai-docs/manuals/ws-mcp.md#L149-L152`, and the
wsflow mirror), and
`lead-run`'s worker acquire step, which now names a profile. The workflow
manual renders the resolved profile list and tells the lead to pass a profile
to `acquire` for workers.

Verification: tests that a `full` acquire never reuses a `housekeeping`
worktree and vice versa; a preserved ignored file survives acquire and
release hygiene while an unpreserved one is cleaned; a hook's sparse widening
survives reuse; a definition change triggers re-adoption; a failing
`post_acquire` quarantines, frees the branch, and is never reused; a
`patterns` profile excluding a directory leaves it unchecked-out; an
unmarked sparse worktree is adopted as `housekeeping`; a profile that hides
`ai-docs/tickets` fails acquire; hooks receive `WS_WORKTREE_DIR`, `WS_BASE`,
`WS_TARGET_BRANCH` and run in the worktree or the relative `cwd`; results
report each hook's argv, scope, and exit code; a failing `pre_release` leaves
the worktree leased and unchanged; the quarantine error counts the profile's
quarantined worktrees; a timed-out hook is a failure; an unpreserved ignored
path created by `post_provision` produces the warning; releasing an unmarked
leased worktree runs today's hygiene only; a failing `post_provision`
quarantines on both a fresh create and a re-adoption, and leaves no lease.
Live check: `git sparse-checkout set --cone`
on a worktree left in non-cone mode behaves as the implementation assumes
(record the observation in the Result).

### Result (5a7122c03, 1b9d8d617) - 2026-10-20

- Landed as the two planned commits. 5a7122c03 (partitioning): required
  `profile` (`sparse_paths` rejected with a pointer to `housekeeping`),
  profile marker in the worktree's git admin dir with provisioning hash,
  partitioned reuse, declared cone/patterns sparse applied only on entry,
  `preserve` at both hygiene points, lazy legacy adoption into builtins,
  ticket-board visibility check, `worktree.list` profile/`legacy`, workflow
  manual profile list in `### Git` plus worker-profile guidance, `lead-run`
  acquire step naming a profile, and the banner/`git.merge`/`wsgit` strings
  pointing at `profile: "housekeeping"`; wsflow and Pi mirrors regenerated.
  1b9d8d617 (hooks): argv hooks at post_provision/post_acquire/pre_release
  with `WS_WORKTREE_DIR`/`WS_BASE`/`WS_TARGET_BRANCH`, worktree or relative
  `cwd`, 10-minute timeout, provenance (argv, scope, exit code) in results,
  quarantine in the marker with a per-profile count, quarantined entries
  shown by `worktree.list` and never reused, pre_release failure leaving the
  worktree leased and unchanged, and the post_provision preserve warning.
- Verification: `go test ./...` in `agents-plugin-tool` passed (mcp 448s);
  `python3 -m unittest discover` over `agents-plugin/tests` and
  `agents-plugin-wsflow/tests` passed. New
  tests: TestProvisionWorktreeHousekeepingProfile,
  TestProvisionWorktreeReusePartitionsByProfile,
  TestProvisionWorktreePreserveSurvivesHygiene,
  TestProvisionWorktreeDefinitionChangeReadopts,
  TestProvisionWorktreePatternsProfileExcludesDir,
  TestProvisionWorktreeAdoptsLegacyIntoBuiltinsOnly,
  TestProvisionWorktreeHiddenTicketBoardFails,
  TestReleaseWorktreeLegacyAndVanishedProfile,
  TestWorkflowManualRendersWorktreeProfiles,
  TestWorktreeAcquireHousekeepingDispatch,
  TestWorktreeHooksEnvCwdAndProvenance,
  TestWorktreeHookSparseWideningSurvivesReuse,
  TestWorktreeHookPostProvisionPreserveWarning,
  TestWorktreeHookPostAcquireFailureQuarantines,
  TestWorktreeHookPostProvisionFailureQuarantines (fresh create and
  re-adoption, no lease), TestWorktreeHookPreReleaseFailureLeavesWorktree,
  TestWorktreeHookTimeoutFails. Mutation checks: dropping the quarantine
  skip and dropping the timeout each fail their test.
- Live check (git 2.50.1): `git sparse-checkout set --cone ai-docs` on a
  worktree in non-cone mode flips `core.sparseCheckoutCone` false to true and
  materializes only the cone, so applying a declared sparse needs no
  `disable` step between modes.
- Decisions: the marker is stamped with an empty hash before provisioning,
  so a part-way failure stays in this profile's partition and re-provisions
  on its next claim. A hidden ticket board returns the worktree to the pool
  with release hygiene and a loud error, not a quarantine (no hook failed).
  `workflow_manual` fails on a bad committed profile, as it already does for
  a malformed committed config file. Hook tests re-execute the test binary as
  the hook so they run on the Windows CI leg. The quarantine record keeps a
  combined stdout+stderr tail rather than stderr only (cosmetic: hooks often
  report failures on stdout). At pre_release `WS_BASE` is empty and
  `WS_TARGET_BRANCH` is the checked-out branch; an acquire without
  `target_branch` passes the base as `WS_TARGET_BRANCH`. The preserve warning
  diffs `git clean -ndx` against `git clean -nd` with the preserve
  exclusions.

### Phase 3: worktree.remove and the stale-pool nudge

Depends on Phase 2. Add `worktree.remove` (with `pre_remove`, refusals, lease
and marker cleanup, quarantine clearing), release-time size recording, and
the throttled acquire nudge with its config knobs; the lead playbook text
relays the nudge to the user.

Verification: tests for remove refusing a dirty worktree, a branch with
unique commits, the primary worktree, a path outside the pool, and a leased
worktree newer than the stale threshold (while removing a stale-leased one),
removing a quarantined worktree, refusals checked before `pre_remove` runs,
a failing `pre_remove` leaving the worktree in place, the remove result
reporting each executed hook's argv, scope, and exit code, removal of a
worktree whose marker names a profile that no longer resolves (skipping
`pre_remove` with a result note), and the nudge firing once
per day with thresholds honored. Live check: whether `git worktree remove`
needs `--force` with preserved ignored files or initialized submodules
present (record in the Result).

### Result (6ad303b09) - 2026-10-20

- Landed: lead-only `worktree.remove` (by path or worker key) checking every
  refusal first (primary, outside the owned pool, unregistered, dirty, a
  branch holding commits no other branch reaches, a detached `HEAD` holding
  commits no ref reaches, a lease active within
  `worktree.nudge_stale_days`), then the marker profile's `pre_remove`
  (skipped with a result note when the profile no longer resolves), then
  `git worktree remove --force`, which takes the admin dir (lease, marker,
  quarantine) with it and keeps the branch; release and remove share one
  ownership-refusal helper. Release records the preserved ignored size in
  the marker. Acquire carries a once-a-day-per-pool `nudge` (throttle file
  in the git common dir) for idle branch holders and detached worktrees at
  or above `worktree.nudge_preserved_gb`; both knobs are integer
  `config.tune` keys (defaults 7 and 1) listed by `config.list`. The
  workflow manual's `### Git` section tells the lead to relay nudge entries
  to the user. `worktree.remove` is in the lead-only and session-key lists,
  the three `runtime.json` tool contracts, and the Pi bridge tool list.
- Verification: `go test ./... -count=1 -timeout 25m` in
  `agents-plugin-tool` passed; `python3 -m unittest discover` over
  `agents-plugin/tests` (78) and `agents-plugin-wsflow/tests` (15) passed;
  `node --test test/bridge.test.ts` in `agents-plugin-pi` passed (80/80,
  tool-contract count now 65). New tests: TestRemoveWorktreeRefusals (dirty,
  unique branch commits, unique detached commits, primary, outside pool,
  fresh lease refused before `pre_remove`; stale lease removed with hook
  provenance and the branch kept),
  TestRemoveWorktreeFailingPreRemoveKeepsWorktree,
  TestRemoveWorktreeQuarantinedAndVanishedProfile,
  TestRemoveWorktreeWithInitializedSubmodule, TestWorktreeRemoveDispatch,
  TestReleaseRecordsPreservedBytes, TestPoolNudgeThrottleAndThresholds,
  TestWorktreeNudgeKnobsAndAcquireWiring. Mutation checks: dropping the
  lease-recency refusal, the throttle, or `--force` each fails its test.
- Live check (git 2.50.1): `git worktree remove` without `--force` removes a
  worktree whose only extra content is ignored files (they go with it), but
  refuses one with an initialized submodule ("working trees containing
  submodules cannot be moved or removed"). Remove therefore runs its own
  dirty and unmerged-work refusals first and then always passes `--force`.
- Decisions: branch uniqueness uses `rev-list --count refs/heads/<b> --not
  --exclude=<b> --branches --remotes` (the short name; `--exclude` with the
  full ref does not exclude it), detached uniqueness uses `--not --branches
  --tags --remotes` (`--all` includes `HEAD` itself). The nudge knobs reuse
  the declared-integer key validation; a hand-edited value that does not
  parse falls back to the default. The nudge runs after the lease is
  written, so the worktree just acquired is never listed, and a nudge
  failure is a result warning, never an acquire failure.

### Phase 4: Retire ticket-board scoping and close out

Depends on Phase 2. Delete `lead-scope-worktree` (skill, playbook, wsflow
mirror, manifest entries, dispatch/bundle tests; the Pi package also carries
copies at `agents-plugin-pi/rsrc/lead-scope-worktree/`,
`agents-plugin-pi/skills/lead-scope-worktree/SKILL.md`, and entries in
`agents-plugin-pi/rsrc/manifest.json#L26` and
`agents-plugin-pi/skills/manifest.json#L14`) and the `scopeAnnouncement`
banner with its tests; add the `tickets.create` out-of-scope warning; update
`260924-research-origin-ticket-ownership-index`'s reference to
`lead-scope-worktree`; add a release-note line telling downstream worktrees
that applied a ticket scope to run `git sparse-checkout disable`. The repo
has no release-notes file (the release workflow writes a generic body), so
the line goes into the Phase 4 Result as a `Forward:` note for the ship step
to put into the GitHub release body of the release that carries it.

Verification: `go test ./...` in `agents-plugin-tool`, the plugin and wsflow
Python test suites, and `claude plugin validate agents-plugin` pass;
`grep -rn "lead-scope-worktree\|sparse_paths"` over `agents-plugin*/`
(excluding `node_modules/`) finds no live reference; a test asserting that the
removed parameter is rejected may name it. Pi `skills/` deletions are done by
hand (`TestPiMirrorUpToDate` covers only Pi `rsrc/`). Downstream follow-up (not checkable here): MSBuild
incremental build after reuse with preserve and stable sparse in place.
