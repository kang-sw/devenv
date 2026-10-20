---
title: Named worktree profiles for worktree.acquire/release
related:
  260910-feat-lead-run-worktree-parallel-route: origin of worktree.acquire/release, the pool, the conservative reuse rule, and hygiene clean -ffdx
  260918-feat-ws-sparse-housekeeping-worktree-occupied-root: added sparse_paths and boolean sparse-shape reuse matching (Decision 5) that profiles replace
  260806-feat-worktree-ticket-scope: ticket-board sparse scope and lead-scope-worktree, retired by this work's final cleanup phase
  260917-feat-ws-committed-project-config-scope: repo config scope; decided config.tune does not write repo "for now"
  260924-research-origin-ticket-ownership-index: intended successor of ticket-board scoping; references lead-scope-worktree
  260923-bug-worktree-acquire-concurrent-pool-claim: adjacent pool-claim race, not addressed here
---

# Named worktree profiles for worktree.acquire/release

## Background

A downstream project (a large C++/MSBuild Windows repository) wants
`worktree.acquire` as the standard entry point for build-capable parallel
worktrees. A full checkout plus a Debug build is about 22 GB per worktree:
4.7 GB of LFS binaries in one `_deps` directory and about 5 GB of runtime DLLs
staged into `x64/`. Their plan is a lightweight worktree: `_deps`
sparse-excluded and shared from the main checkout through an MSBuild property,
DLLs hardlinked into the output folder. They submitted five issues:

1. No way to express a sparse exclusion and no post-acquire step; a repo
   script must run by hand after every acquire.
2. Pool reuse matching after a script changes the sparse scope (their
   hypothesis: the worktree never matches again and the pool grows).
3. `release` runs `git clean -ffdx`, deleting ignored build outputs and a
   per-worktree override file, so every reuse is a full rebuild.
4. Acquire failed in a submodule repo ("could not reset submodule index",
   missing `.git/worktrees/<name>/modules/<sub>`), leaving a broken stub.
5. Tickets a session creates disappear under its own sparse ticket scope;
   on Git Bash, glob-less sparse patterns are mangled by MSYS path conversion.

The discussion concluded that the acquire/release design is sound for its
original two use cases (full-checkout code workers and `["ai-docs"]`
housekeeping) but lacks a unit that binds reuse matching, preservation, and
setup hooks together. That unit is a named worktree profile.

## Outcome Ledger

### Verified Findings

Source reading only; nothing was executed.

- `worktree.acquire` sparse support is cone-only: the sole sparse call is
  `git sparse-checkout set --cone <paths...>`, run on every sparse acquire,
  new or reused (search `sparse-checkout", "set", "--cone"` in
  `agents-plugin-tool/internal/mcp/worktree_tools.go`). No pattern, exclude,
  or hook option exists; `worktree_pool` is the only worktree config item.
- New worktrees are created `git worktree add --detach --no-checkout`.
- Reuse matching (`provisionWorktree` reuse loop) requires: under the pool
  root, not the main root, detached, `wsdoc.SparseCheckoutActive(path) ==
  wantSparse`, and empty `git status --porcelain`. `SparseCheckoutActive` is
  a boolean on `core.sparseCheckout`; patterns and cone mode are not compared.
  The lease is not consulted and records no requested shape.
- Consequence for downstream Issue 2: a worktree whose scope a script widened
  is still sparse-active, so it is reused and re-narrowed by `set --cone`,
  not skipped forever. The real costs are (a) cross-use mixing in one pool
  (a `["ai-docs"]` housekeeping acquire can claim and narrow a build
  worktree and vice versa) and (b) narrow-then-widen on every reuse rewrites
  every out-of-cone tracked file, resetting mtimes, which likely defeats
  MSBuild incremental builds even if outputs were preserved (inference, not
  measured).
- The `sparse_paths` tool description says "A pooled worktree is reused only
  when its sparse state matches", which overstates the boolean check.
- Both acquire hygiene and release run `git reset --hard` + `git clean -ffdx`
  (release then `git checkout --detach` and deletes the lease file
  `ws-worktree-lease.json` in the worktree admin dir). There is no preserve
  list, no non-`-x` mode, and no knob. Release never deletes a worktree; there
  is no pool cap, eviction, GC, or remove tool.
- Submodules: when `.gitmodules` exists, acquire runs `git submodule update
  --init --recursive`; failure is a warning. No hygiene or switch command
  passes `--no-recurse-submodules` or `-c submodule.recurse=false`, so a user
  `submodule.recurse=true` applies to them. No acquire test uses a submodule
  fixture (recorded as a gap in 260910).
- Ticket scope is applied by the lead running git from the
  `lead-scope-worktree` playbook, not by ws code. The playbook command
  includes a glob-less leading-slash pattern
  (`/ai-docs/tickets/idea/.gitkeep`) that matches the MSYS path-conversion
  hazard; no `MSYS_NO_PATHCONV` handling exists anywhere. Whether this explains
  the unreproduced Windows status-directory vanish in 260806 is a hypothesis.
- `tickets.create` does not widen the scope; `tickets.move`/`close` refuse a
  scope-blocked destination via `git sparse-checkout check-rules` with a
  "widen and retry" message.
- `scope_announcement.go` renders a session-bootstrap banner for any non-cone
  sparse checkout, remedy "`git sparse-checkout disable`", and stays silent for
  cone mode. A non-cone build profile would trigger it with a harmful remedy.
- Config: the resolver is a flat `map[string]string` overlay; the only
  structured knob (`agents.tier`) bypasses it with a dedicated writer.
  `tuningField` carries only name/description/enum. `config.tune` already
  accepts `value: string | object`. Key dispatch (`resolveConfigEntryForKey`)
  accepts exact registry keys, any `prompt.<suffix>`, and manifest-declared
  keys; anything else is `unknown config key`.
- Scopes resolve `session > project > repo > global > builtin`. `project` is
  machine-local (`~/.ws@<id>/config.json`); `repo` is the committed
  `.ws-workflow/config.json`, read-only to `config.tune` by the 260917 decision
  "Read-only / hand-edited for now" — no stronger rationale is recorded.

### Confirmed Decisions

1. **Profiles.** Introduce named worktree profiles. A profile is both the
   pool-partition key (reuse only within the same profile) and the lifecycle
   definition.
2. **Definition source.** Profiles are defined through the layered config, not
   a runtime registration tool. Profile definitions are not accepted from the
   `session` scope.
3. **Config key family.** One key per profile, `worktree.profile.<name>`,
   added as a prefix family in key dispatch alongside `prompt.`. Values are
   JSON objects, so the resolver gains structured-value support (rather than a
   second resolver-bypassing writer like `agents.tier`). Removal is
   `config.tune(reset: true)`.
4. **Profile shape** (agreed sketch, field names normative):
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
   - `sparse.mode: "patterns"` (non-cone) is kept alongside cone, so a
     portable declaration works without shell or MSYS involvement.
   - Declared sparse is applied at provision and when the profile definition
     changes, not on every acquire; sparse changes a hook makes afterwards
     persist across reuse.
   - `preserve` is honored by the hygiene clean in both acquire and release.
5. **Hook lifecycle.** `post_provision` runs once when a worktree enters the
   profile (fresh create, or re-adoption after a definition change);
   `post_acquire` runs on every acquire after hygiene and branch switch (after
   `post_provision` on a fresh one); `pre_release` runs before release hygiene;
   `pre_remove` runs before removal. Hooks are accepted from every non-session
   scope (the user does not hand-author files, so a hand-edit-only rule would
   protect nothing).
6. **Hook failure.** A failing hook fails its operation and leaves the worktree
   in its pre-operation state. Failures are fail-loud and must strongly direct
   the agent to fix the pool/profile.
7. **Quarantine.** A worktree whose `post_acquire` fails is not reused while
   failing; quarantine is cleared only by removing the worktree with
   `worktree.remove` (no explicit unquarantine flag).
8. **`worktree.remove`.** Add a removal tool that refuses unsafe targets and
   cleans the lease and profile marker.
9. **Stale-pool nudge.** At most once a day, at acquire time, nudge about
   accumulated worktrees: branch checked out with no new commits for a week or
   more, or detached with 1 GB or more of ignored artifacts.
10. **Profile is a required acquire parameter.** No default-profile knob. The
    lead chooses explicitly; the workflow manual's worktree section renders the
    resolved profile list (name, description, provenance scope) and instructs
    the lead to pass the profile to `acquire` for workers. No separate
    `worktree.profile.list` tool.
11. **Ticket visibility invariant.** An acquired worktree must expose
    `ai-docs/tickets` (workers record results into the ticket); checked after
    provisioning, fail-loud.
12. **`sparse_paths` is removed**, replaced by builtin profiles `full` and
    `housekeeping` (cone `["ai-docs"]`). The workflow schema is not a durable
    artifact, so breaking it is acceptable; compatibility concern is limited to
    already-pooled worktrees.
13. **Same landing for Issues 4 and 5** with the profile feature.
14. **Retire `lead-scope-worktree`** (skill, playbook, wsflow mirror) as the
    final cleanup phase of this work. The user explicitly requested this
    deletion (an "Always ask" item in `AGENTS.md`). The ticket-ownership
    concept supersedes it.
15. **Delete the `scopeAnnouncement` session-bootstrap banner.** Acquired
    worktrees never run `workflow_manual` (they are subagent implementation
    spaces), so a profile-aware replacement is unnecessary.

### Proposals

Presented in discussion without an explicit confirmation; settle before
deriving an actionable ticket.

- Profile marker in the worktree admin dir that survives release (profile name
  plus definition hash); the lease is deleted on release, so it cannot carry
  the profile.
- Hook execution: argv arrays (not shell strings), default cwd = worktree,
  values passed by env vars (`WS_WORKTREE_DIR`, `WS_BASE`,
  `WS_TARGET_BRANCH`) rather than `${...}` templating; no `WS_REUSED`, since
  the hook choice already distinguishes fresh from reuse.
- Hook provenance: acquire/release/remove results report each executed hook's
  argv, source scope, and exit code; `config.list` and the manual's profile
  list show per-field provenance.
- `post_acquire` failure handling detail: run release-equivalent hygiene
  (without `pre_release`) so the target branch is freed, mark the worktree
  quarantined in its marker with argv/scope/exit/stderr tail/time, return a
  fail-loud error that counts quarantined worktrees in the profile (2+ ⇒
  likely a profile/hook defect), and never auto-retry on a fresh worktree.
  `post_provision` failure uses the same quarantine.
- `post_provision` outputs (override files, hardlinks) only survive if listed
  in `preserve`; document or warn.
- Legacy adoption: unmarked pooled worktrees are adopted lazily into builtin
  profiles only, by today's boolean rule (sparse-active ⇒ `housekeeping`
  whatever its cone, otherwise `full`), stamping the marker on claim; custom
  profiles never adopt unmarked entries. A release of an unmarked leased
  worktree runs today's hygiene only. `worktree.list` labels unmarked entries
  `legacy`.
- `worktree.remove` refusals: dirty worktree, or a checked-out branch holding
  commits reachable from nowhere else.
- Nudge detail: activity = `max(lease acquired_at, HEAD commit time)`;
  preserved size measured at release and recorded in the marker (never walked
  at acquire); throttle timestamp stored per pool in the git common dir;
  thresholds as config knobs defaulting to 7 days / 1 GB; shipped playbook text
  tells the lead to relay the nudge to the user.
- Profile name validation, e.g. `^[a-z][a-z0-9-]*$`.
- `config.list` catalog shows the family pattern with a `value_schema` (JSON
  Schema) plus resolved profiles; validation at `config.tune` write and at
  resolve time (a bad committed file fails acquire and manual render loudly,
  naming file and key).
- Override granularity: a higher scope replaces a whole profile; no field
  merge. Builtin profiles can be overridden by name.
- Issue 4 fix: pass `-c submodule.recurse=false` to hygiene/switch commands,
  keep the explicit `submodule update`, add a submodule fixture test.
- Retirement keeps generic sparse safety: `SparseCheckoutActive`,
  `git add --sparse` staging, the move/close `check-rules` gate, the create
  collision refusal; add a `tickets.create` warning when the created path is
  outside the active sparse scope.
- Retirement follow-through: a release note line on `git sparse-checkout
  disable` for downstream worktrees that applied a scope; update
  `260924-research-origin-ticket-ownership-index`'s reference to
  `lead-scope-worktree`.
- Fix the `sparse_paths`/acquire tool description wording as part of the
  schema change.

### Open Questions

- Should `config.tune` write the `repo` scope? The 260917 refusal was a
  minimal "for now" choice; agents edit the committed JSON anyway, and
  `note.write(layer: "repo")` is a precedent for writing a tracked file while
  staging rides the caller's `git.commit`.
- Live checks still needed:
  - `git sparse-checkout set --cone` on a worktree left in non-cone mode;
  - whether `git worktree remove` needs `--force` with preserved ignored files
    or submodules present;
  - whether MSBuild builds incrementally after reuse once preserve and
    stable sparse are in place (the downstream's Phase 2 check).

### Rejected Alternatives

- Runtime `worktree.profile.set` MCP registration: profiles travel with repo
  scripts and must be versioned with the repository.
- Hooks only from hand-edited files (config.tune refusing hook fields): the
  user does not hand-author files, so an agent would edit the file anyway.
- Keeping `sparse_paths` as an anonymous ad-hoc profile: blurs the
  pool-partition key.
- Hook-only provisioning on an empty checkout with no declared sparse or
  preserve: ws itself runs the hygiene clean (so preserve must be ws-owned),
  and repo-resident hook scripts do not exist before the first checkout
  (declared sparse is the bootstrap).
- Auto-deleting stale worktrees: replaced by the daily nudge plus
  `worktree.remove`.
- Explicit `clear_quarantine` on release: would return a possibly broken
  worktree to the pool.
- A config default-profile knob: replaced by a required profile parameter
  chosen by the lead.
- A profile-aware replacement banner: acquired worktrees never render the
  workflow manual.
- A single `worktree.profiles` map key: per-scope whole-value override would
  prevent per-profile layering.
