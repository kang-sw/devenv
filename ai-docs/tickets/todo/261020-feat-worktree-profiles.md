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
  This applies to every key, not only profiles. Rejected: keeping `repo`
  read-only (260917's "for now" choice) - it protects nothing when agents edit
  the file and skips write-time validation on the scope that holds most
  profiles.
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
  the profile name plus a hash of the resolved definition and survives
  release (the lease file is deleted on release, so it cannot carry the
  profile). A hash mismatch at acquire means a definition change:
  re-apply declared sparse and run `post_provision` (re-adoption).
- **Ticket visibility invariant.** After provisioning, an acquired worktree
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
- **Hook provenance.** acquire/release/remove results report each executed
  hook's argv, source scope, and exit code.
- **Hook failure is fail-loud.** A failing hook fails its operation, leaves
  the worktree in its pre-operation state, and the error strongly directs the
  agent to fix the profile/pool.
- **Quarantine.** On `post_acquire` failure: run release-equivalent hygiene
  (without `pre_release`) so the target branch is freed, mark the worktree
  quarantined in its marker (argv, scope, exit code, stderr tail, time), and
  return an error that counts the profile's quarantined worktrees (2+ means a
  likely profile/hook defect). Never auto-retry on a fresh worktree. A
  `post_provision` failure uses the same quarantine. A quarantined worktree is
  never reused; only `worktree.remove` clears it. Rejected: an explicit
  `clear_quarantine` on release (would return a possibly broken worktree to
  the pool).
- **`post_provision` outputs** (override files, hardlinks) survive hygiene
  only when listed in `preserve`. The acquire result warns when a path a
  `post_provision` hook created is ignored and not covered by `preserve`.
  Rejected: documentation only (a result warning is seen where docs are not).
- **Legacy adoption.** Pooled worktrees without a marker are adopted lazily
  into builtin profiles only, by today's boolean rule (sparse-active =>
  `housekeeping` whatever its cone, otherwise `full`), stamping the marker on
  claim. Custom profiles never adopt unmarked entries. Releasing an unmarked
  leased worktree runs today's hygiene only. `worktree.list` labels unmarked
  entries `legacy`.

### `worktree.remove` and the nudge

- **`worktree.remove`** is a new tool. It runs `pre_remove`, removes the git
  worktree, and cleans the lease and marker. It refuses a dirty worktree, or
  one whose checked-out branch holds commits reachable from nowhere else.
- **Stale-pool nudge**, at most once a day per pool, at acquire time, listing
  worktrees with a checked-out branch and no activity for 7 days or more, or
  detached with 1 GB or more of preserved ignored artifacts.
  - Activity = `max(lease acquired_at, HEAD commit time)`.
  - Preserved size is measured at release and recorded in the marker; it is
    never walked at acquire.
  - The throttle timestamp is stored per pool in the git common dir.
  - Thresholds are config knobs defaulting to 7 days / 1 GB.
  - Shipped playbook text tells the lead to relay the nudge to the user.
  Rejected: auto-deleting stale worktrees.

### Submodules (downstream Issue 4)

- Hygiene and branch-switch git commands pass `-c submodule.recurse=false`;
  the explicit `git submodule update --init --recursive` stays.

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
- `-c submodule.recurse=false` on hygiene and switch commands.

Verification: unit tests for resolver precedence of structured values across
project/repo/global/builtin, session refusal, bad-name and bad-schema
refusal at write, a malformed committed file failing resolve with file and
key named, `config.tune` repo write creating/updating
`.ws-workflow/config.json` without staging, and reset removing a profile. An
acquire test with a submodule fixture under `submodule.recurse=true` passes.

### Phase 2: Profile-partitioned acquire/release with hooks and quarantine

Depends on Phase 1. Replace `sparse_paths` with the required `profile`
parameter and implement marker, declared sparse (cone and patterns),
`preserve` in both hygiene points, the four-point lifecycle minus
`pre_remove`, env-var hook execution, provenance reporting, fail-loud
failure, quarantine, the `post_provision` preserve warning, legacy adoption,
and the ticket-visibility check. Update the acquire tool description
(dropping the "sparse state matches" overstatement), `worktree.list`
(profile, `legacy`, quarantine shown), and every caller that passes
`sparse_paths`: the occupied-root banner, the `git.merge` and `wsgit`
refusal strings, the workflow manual's `### Git` housekeeping line, and
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
`ai-docs/tickets` fails acquire. Live check: `git sparse-checkout set --cone`
on a worktree left in non-cone mode behaves as the implementation assumes
(record the observation in the Result).

### Phase 3: worktree.remove and the stale-pool nudge

Depends on Phase 2. Add `worktree.remove` (with `pre_remove`, refusals, lease
and marker cleanup, quarantine clearing), release-time size recording, and
the throttled acquire nudge with its config knobs; the lead playbook text
relays the nudge to the user.

Verification: tests for remove refusing a dirty worktree and a branch with
unique commits, removing a quarantined worktree, and the nudge firing once
per day with thresholds honored. Live check: whether `git worktree remove`
needs `--force` with preserved ignored files or initialized submodules
present (record in the Result).

### Phase 4: Retire ticket-board scoping and close out

Depends on Phase 2. Delete `lead-scope-worktree` (skill, playbook, wsflow
mirror, manifest entries, dispatch/bundle tests) and the `scopeAnnouncement`
banner with its tests; add the `tickets.create` out-of-scope warning; update
`260924-research-origin-ticket-ownership-index`'s reference to
`lead-scope-worktree`; add a release-note line telling downstream worktrees
that applied a ticket scope to run `git sparse-checkout disable`.

Verification: `go test ./...` in `agents-plugin-tool`, the plugin and wsflow
Python test suites, and `claude plugin validate agents-plugin` pass;
`grep -rn "lead-scope-worktree\|sparse_paths"` over `agents-plugin*` finds
nothing outside history. Downstream follow-up (not checkable here): MSBuild
incremental build after reuse with preserve and stable sparse in place.
