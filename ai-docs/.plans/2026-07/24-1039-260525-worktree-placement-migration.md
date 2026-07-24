# Plan: 260525-feat-ws-dashboard-workroot-polishing-backlog — Phase 1: worktree default-placement migration slice

## Relevant Ticket Contract
- Owner direction (ticket Phase 1, "Long-term worktree placement direction",
  and `260711-idea-dashboard-workroot-scoped-artifact-consolidation`
  Decisions): move default new-worktree placement from
  `<root>/.git/ws-worktree/<name>` to `<root>/.ws-dashboard/worktrees/<name>`.
- Owner judged migration risk low: worktree discovery is `git worktree
  list`-based, not path-bound, and no placement under `.git/ws-worktree/` has
  ever shipped, so there is no back-compat to carry — **confirmed in survey**
  (see Codebase Findings: `discovery.rs` uses `git worktree list
  --porcelain`, never a hardcoded expected path).
- `.ws-dashboard/worktrees/<name>` needs no physical symlink/junction — it is
  daemon-resolved via `git rev-parse --git-common-dir`'s parent, consistent
  with the idea ticket's "scripts/" pattern (not the `.ws-dashboard-shared`
  physical-link pattern, which is unrelated to worktree placement).
- Sequencing: land as part of this phase's create-path change, not a
  standalone migration (per idea ticket Decisions).
- Bare-repo edge case (`common_dir.parent()` not being a real working
  directory) is owner-confirmed out of scope — "too uncommon a setup to
  design around right now."

## Out of Scope
- Broad Phase 1 lifecycle-polish candidate areas (unavailable/recovery
  states, remove/forget copy, refresh timing, pinned directory behavior,
  workspace grouping) — explicitly excluded by the survey scope reminder.
- Phase 2 git-toolbar polish (`gitToolbar.ts` index.lock contention, etc.) —
  explicitly excluded.
- The already-shipped Phase 1 delete op and Phase 3 removal/hide UX (Result
  `bf7deab0`) — done, not touched by this slice.
- `.ws-dashboard/scripts/`, `.ws-dashboard-shared` physical linking, and the
  conditional git-tracking (`*.local.*`) convention — owned by
  `260711-idea-dashboard-workroot-scoped-artifact-consolidation`, not this
  slice; only the `worktrees/` subpath is in scope here.
- Migrating/backfilling worktrees created before this change (existing
  `.git/ws-worktree/*` worktrees keep working as-is via `git worktree list`
  discovery; no migration tooling requested).

## Codebase Findings
- `crates/daemon/src/git_worktree.rs#L753-L815` (`resolve_workspace_git`) —
  resolves `common_dir` via `git rev-parse --path-format=absolute
  --git-common-dir` (always the root repo's real `.git`, even when called
  from inside a linked worktree — confirms idea-ticket claim). `common_dir`
  is the single anchor to derive the new base from: `common_dir.parent()` is
  the root workroot directory. Line 797 currently builds
  `let base = common_dir.join("ws-worktree")` and line 798 does `let _ =
  fs::create_dir_all(&base);` (errors silently ignored — same tolerance
  should carry forward).
- `crates/daemon/src/git_worktree.rs#L737-L745` (`GitWorkspaceContext`
  struct) — has no field carrying the resolved base dir today;
  `resolve_preview_with_context` independently recomputes
  `common_dir.join("ws-worktree")` (line 664), duplicating the join. Adding
  a `worktree_base_dir: PathBuf` field computed once in
  `resolve_workspace_git` removes this duplication (existing pattern: the
  struct already centralizes `root_path`/`common_dir`/`root_label` for reuse
  across `resolve_preview_with_context` and `options_for_context`).
- `crates/daemon/src/git_worktree.rs#L652-L667` (`resolve_preview_with_context`)
  — `GitWorktreePathRequest::Auto` branch builds `target_path` from
  `context.common_dir.join("ws-worktree").join(&filesystem_name)`; this is
  the actual create-path placement logic to change.
- `crates/daemon/src/git_worktree.rs#L157-L181` and `#L611-L627`
  (`git_worktree_add_options` error branch / `options_for_context`) — both
  hardcode the UI hint label `worktree_base_dir_label: ".git/ws-worktree"`
  (a generic display string, not derived from a live path in either call
  site today) — needs the matching label update.
- `crates/daemon/src/discovery.rs#L486-L505` (`git_worktree_paths`) —
  confirms discovery is `git worktree list --porcelain`-based, not
  path-pattern-based; no coupling to `.git/ws-worktree` found anywhere in
  discovery or dedup logic (`paths_equivalent`, `local_work_root_id_for_path`).
- **Risk signal — git-status hygiene regression**: today's placement lives
  *inside* `.git/`, which the root repo's own `git status`/`git add` never
  traverses, so created worktrees are invisible to the root repo's working
  tree by construction. Moving to `<root>/.ws-dashboard/worktrees/<name>` — a
  real subdirectory of the working tree — loses that free invisibility: an
  unignored worktree checkout would show up as untracked content in the root
  repo's `git status`. No existing `.gitignore`/`.git/info/exclude` handling
  exists anywhere in `crates/daemon/src` (`grep -rn "info/exclude\|gitignore"`
  returned nothing), so this is new. Not explicitly decided in the ticket or
  the idea ticket's Decisions section — the Decisions section's conditional
  git-tracking convention (`*.local.*`) is scoped to `.ws-dashboard/scripts/`
  content, not to `worktrees/`, and worktree checkouts are never meant to be
  tracked or offered for tracking at all. The safe, narrowly-scoped fix is an
  idempotent local-only entry (`/.ws-dashboard/worktrees/`) appended to
  `<root>/.git/info/exclude` at the same point the base directory is
  ensured — mirrors the existing best-effort/local-only precedent set for
  `.ws-dashboard-shared` linking in the idea ticket ("best-effort, silent
  skip on failure"). This is inferred to preserve pre-migration behavior
  parity, not an invented policy; flagged for lead awareness below.
- `crates/daemon/tests/routes.rs#L6631-L6650` — only daemon test exercising
  the `Auto` path placement end-to-end; asserts
  `preview["targetPathLabel"].contains("ws-worktree")`. All other `Auto`-mode
  test call sites (`routes.rs` lines 5571, 5823, 6870, 6884) don't assert the
  literal path string, so this is the only daemon test needing a change.
  Worktree-removal tests (`routes.rs#L6982+`) use an explicit `Custom` target
  path via `add_linked_worktree_for_test`, unaffected.
- `frontend/src/gitWorktreeAddModal.tsx#L217-L219` — client-side placeholder
  fallback (`options?.defaults.worktreeBaseDirLabel ?? ".git/ws-worktree"`)
  shown before the server's `defaults` response loads; needs the same label
  string update.
- `frontend/e2e/dashboard-acceptance.spec.ts#L1076-L1082` — hardcodes
  `path.join(gitWorkRoot, ".git", "ws-worktree", "Browser-Gate-Branch")` to
  write a scratch file simulating a dirty worktree for the B-1 removal-modal
  data-loss-banner assertion. Must be updated to the new path segments even
  though this spec currently red-lines at an earlier step due to the
  pre-existing locator bug tracked in
  `260722-bug-e2e-open-work-root-locator-ambiguity` (cannot be run to green
  regardless of this change).
- `frontend/src/gitWorktreeAdd.test.ts#L43,#L135` — mock server-response
  fixtures set `defaults: { worktreeBaseDirLabel: ".git/ws-worktree" }`; this
  string is never asserted against elsewhere in the file (pure passthrough
  fixture data), so updating it is optional hygiene, not required for the
  test to pass.
- `ai-docs/spec/ws-web-dashboard/index.md#L601-L622` (`Git Worktree Creation`
  anchor `260524-ws-dashboard-git-worktree-creation`) — prose states
  "Automatic path naming targets the workspace Git root's
  `.git/ws-worktree/<branch-compatible-name>` convention." Needs updating to
  match (doc-drift-on-contact per AGENTS.md).

## Implementation Plan
1. `crates/daemon/src/git_worktree.rs`: add `worktree_base_dir: PathBuf` to
   `GitWorkspaceContext` (line ~737-745). In `resolve_workspace_git`
   (line ~753-815), replace `let base = common_dir.join("ws-worktree");`
   (line 797) with a helper that derives `common_dir.parent().map(|root|
   root.join(".ws-dashboard").join("worktrees"))`, falling back to the old
   `common_dir.join("ws-worktree")` shape only if `common_dir` unexpectedly
   has no parent (keeps `fs::create_dir_all` error-tolerant as today); store
   the result in the new context field and keep using it for the existing
   `fs::create_dir_all(&base)` call.
2. In the same function, after ensuring the base directory exists, append an
   idempotent local-only ignore entry for `/.ws-dashboard/worktrees/` to
   `<root_path-derived-root>/.git/info/exclude` (read the file if present,
   skip the append if the line is already there, best-effort/silent on any
   I/O failure — do not fail worktree creation over this). Root path for the
   exclude file is `common_dir` itself for a non-worktree common dir
   (`common_dir/info/exclude`), since `common_dir` already *is* `<root>/.git`.
3. `resolve_preview_with_context` (line ~652-667): change the
   `GitWorktreePathRequest::Auto` arm to
   `context.worktree_base_dir.join(&filesystem_name)` instead of
   `context.common_dir.join("ws-worktree").join(&filesystem_name)`.
4. Update both hardcoded UI label sites to `.ws-dashboard/worktrees`:
   `git_worktree_add_options` error branch (line 175) and
   `options_for_context` (line 624).
5. `frontend/src/gitWorktreeAddModal.tsx#L219`: update the fallback literal
   `".git/ws-worktree"` to `".ws-dashboard/worktrees"`.
6. `crates/daemon/tests/routes.rs#L6650`: update the assertion to
   `.contains(".ws-dashboard")` (or the more specific
   `"ws-dashboard/worktrees"`).
7. `frontend/e2e/dashboard-acceptance.spec.ts#L1076-L1082`: update the
   hardcoded path segments from `".git", "ws-worktree"` to
   `".ws-dashboard", "worktrees"`.
8. Optional hygiene: update `frontend/src/gitWorktreeAdd.test.ts#L43,#L135`
   mock fixture strings to `.ws-dashboard/worktrees` for consistency (not
   required for tests to pass).
9. `ai-docs/spec/ws-web-dashboard/index.md#L613`: update the "Automatic path
   naming" sentence to describe the `.ws-dashboard/worktrees/<branch-
   compatible-name>` convention.

## Verification Plan
- `cargo test -p ws-dashboard-daemon git_worktree_add_previews_and_submits_new_branch_with_resource_refresh` —
  confirms the new Auto-path placement end-to-end (requires `git` on PATH;
  test self-skips otherwise via `skip_without_git`).
- `cargo test -p ws-dashboard-daemon git_worktree` — broader regression pass
  over add/remove daemon route tests to confirm no coupling was missed.
- Manual/added check: after a test worktree add, run `git status --porcelain`
  at the root repo and confirm the new `.ws-dashboard/worktrees/<name>`
  directory does not appear untracked (validates the `.git/info/exclude`
  step in Implementation Plan step 2). Add as a new daemon test assertion if
  convenient, since no existing test covers root git-status cleanliness.
- Frontend: `npm test` (or the project's existing `gitWorktreeAdd.test.ts`
  run target) for the modal fallback-label change.
- The `dashboard-acceptance.spec.ts` e2e edit cannot be verified to green
  today (pre-existing locator bug, tracked separately); type-check only
  (matches the precedent already accepted in the ticket's Result section for
  the Phase 3 slice).

## Escalations
- None.
