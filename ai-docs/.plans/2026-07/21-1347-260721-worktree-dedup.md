# Plan: 260721-bug-dashboard-worktree-create-duplicate-add — Phase 1: reproduce duplicate mechanism and add path-based deduplication

## Relevant Ticket Contract

- Confirm the mechanism, then add path-based (filesystem-path) dedup so a
  work root already present is not re-added.
- **Verification boundary**: creating a worktree yields exactly one entry for
  the new work root; legitimately distinct work roots (other primary roots,
  pre-existing worktrees) still appear as separate entries; `test:resource-model`
  and `test:workbench` stay green; manual dogfood confirms no duplicate.
- Spec Impact: none — `ai-docs/spec/ws-web-dashboard/index.md` `## Git
  Worktree Creation {#260524-ws-dashboard-git-worktree-creation}` already
  specifies a single created/selected linked workRoot; this restores
  conformance, it does not change the spec.
- Ticket's own hedge: `serverScopedIdentity` (frontend) vs.
  `local_work_root_id_for_path` (daemon) are both floated as candidate dedup
  keys — this survey traced the actual data flow to decide between them (see
  Codebase Findings).

## Out of Scope

- The "double-apply" pattern in `handleWorkRootOpened` (`App.tsx:689-717`,
  extra `loadResources("open")` after `applyExternalResources`) and the
  background poll race in `resourceRefresh.ts` (`applyExternalResources`
  bumping sequence without checking `isInFlight()`) — traced and ruled out as
  the mechanism for *this* bug (see Codebase Findings); tracked separately
  under `260710-idea-dashboard-open-work-root-full-registry-redundant-rediscovery`
  if still relevant.
- Any frontend change to `resourceModel.ts` / `mergeResourcesByServer` /
  `App.tsx`'s `onCreated` handler — traced and ruled out as the fix seam (see
  Codebase Findings: the frontend never receives a raw filesystem path to key
  a dedup on).
- Must **not** collapse a base/primary root and its own linked-worktree child
  into one entry — they are, and must remain, distinct `WorkRootView` rows
  (verified: canonicalizing a path only resolves symlink/`.`/`..` aliasing of
  *the same* physical directory; it cannot make two genuinely different
  directories hash the same).
- Any change to `discover_work_root`'s stored `DiscoveredWorkRoot.path` /
  `label_for_path` display path — out of scope; the fix touches only the
  identity-hash inputs, not the path used for labels/status, so no worktree's
  displayed label changes.
- Cross-daemon-restart persisted-ID migration concerns — checked and are a
  non-issue: `OpenedWorkRoots::from_paths` (`work_root_files.rs:114-118`)
  re-derives `WorkRootId` fresh from the persisted **path** on every load via
  `local_work_root_id_for_path`, it does not deserialize a stored ID, so the
  fix applies uniformly on next computation with no stale-ID migration step
  needed.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx:1417-1430` — `GitWorktreeAddModal`'s
  `onCreated` applies `response.resources` via `applyExternalResources` in a
  **single** direct call; no extra fetch is fired in this branch (confirmed
  by reading the exact code, not just the ticket's notes). Since
  `mergeResourcesByServer` (`resourceModel.ts:204-209`) *replaces* (not
  merges into) the per-server tree, and this is the only apply call on the
  create path, **whatever duplicate the user sees must already exist inside
  the single `response.resources` tree returned by the daemon** — it cannot
  be a frontend accumulation artifact. This rules out the frontend as the
  bug's origin.
- `ws-dashboard/frontend/src/resourceModel.ts:148-160` (`WorkRootView`) —
  confirmed the frontend view model carries only an opaque `id` /
  `resourcePath.workRootId` and a basename `label` (`label_for_path` in the
  daemon uses `file_name()`), **never a raw filesystem path**. This rules out
  `mergeResourcesByServer`/`serverScopedIdentity` as a viable dedup seam: if
  two daemon-issued ids for the same physical directory already differ (the
  actual root cause, see below), the frontend has no path data to recognize
  them as the same root, and keying on `label` instead would incorrectly
  collapse two *different* directories that happen to share a basename
  (e.g. two repos each containing a dir named the same). Frontend dedup is
  therefore both unnecessary (once the daemon key is fixed) and unsafe as a
  substitute.
- `ws-dashboard/crates/daemon/src/git_worktree.rs:242-284`
  (`git_worktree_add_submit`) — after `git worktree add` succeeds, registers
  the new worktree as its own top-level registry entry
  (`register_registry_entry`, `WorkRootProvenance::Opened`), persists it,
  then computes the **entire** response resources via one
  `live_dashboard_resources(&state.opened_work_roots)` call. This is a single
  discovery pass — confirms the duplicate must originate inside this one
  pass, in the daemon.
- `ws-dashboard/crates/daemon/src/resources.rs:50-67`
  (`live_dashboard_resources_with_sync`) — candidates fed into
  `LocalDashboardResourcesProvider` come from
  `opened.owner_candidate_roots()`.
- `ws-dashboard/crates/daemon/src/work_root_files.rs:225-231`
  (`owner_candidate_roots`) — filters registry entries by
  `provenance == WorkRootProvenance::Opened`. The pre-existing primary root
  (opened earlier by the user) *and* the just-created worktree (registered
  with `Opened` provenance at `git_worktree.rs:253`) are **both**
  simultaneously `Opened`-provenance candidates during this exact discovery
  pass — i.e. `self.candidates` contains both the primary root path and the
  new worktree path as two independent top-level entries at the same time.
- `ws-dashboard/crates/daemon/src/discovery.rs:69-104`
  (`dashboard_resources_with_registry_sync`) — for the primary-root candidate:
  discovers it, then also runs `git_worktree_paths(&candidate.path)` (`git
  worktree list --porcelain` from the primary), which now includes the
  brand-new worktree path, and pushes it into the **same** `workspace_key`
  bucket. For the new-worktree candidate (also present in `self.candidates`
  per the previous finding): discovers it independently, resolves to the
  **same** `workspace_key` (both keyed by `stable_path_hash(&git.common_dir)`,
  and `common_dir` is the same physical `.git` for both), and attempts
  `workspace.push(...)` into that same bucket.
- `ws-dashboard/crates/daemon/src/discovery.rs:185-194`
  (`WorkspaceBuilder::push`) — **already** guards against exactly this
  double-insertion: `if self.work_roots.iter().any(|root| root.id ==
  work_root_id) { return; }`. This guard is provably sufficient *whenever*
  both occurrences compute the same `work_root_id` for the same physical
  directory — which is the crux of the actual bug (see next finding).
- `ws-dashboard/crates/daemon/src/discovery.rs:521-526`
  (`local_work_root_id_for_path`) and `discovery.rs:504-512`
  (`normalize_candidate_path`) — `local_work_root_id_for_path` hashes
  `normalize_candidate_path(path)` via `stable_path_hash` (`discovery.rs:579-590`,
  a plain FNV-1a hash over the path's **string bytes** — confirmed by
  reading it, not a semantic path comparison). `normalize_candidate_path`
  only cwd-joins a relative path to make it absolute; it does **not**
  resolve symlinks or `.`/`..` components. Two literal path strings that
  refer to the *same physical directory* (e.g. one reached through a
  symlinked path segment, one not, or one derived from `git rev-parse
  --path-format=absolute` output vs. one built by a plain Rust `PathBuf`
  join as in `resolve_preview_with_context`'s
  `context.common_dir.join("ws-worktree").join(&filesystem_name)`,
  `git_worktree.rs:337-341`) hash to **different** `WorkRootId`s. This
  silently defeats the `WorkspaceBuilder::push` dedup guard above and is the
  most direct, evidenced explanation for the reported duplicate: the new
  worktree's own top-level discovery and its discovery-as-a-linked-path from
  the primary can diverge textually and thus land as two distinct
  `WorkRootView` rows (in the same or, if `common_dir`'s hash also diverges,
  a different `workspace_key` bucket — both failure modes share the same
  root cause and the same fix).
- `ws-dashboard/crates/daemon/src/discovery.rs:514-519`
  (`paths_equivalent`) — **existing precedent** in this same file for
  exactly this problem: it already does
  `left.canonicalize()`/`right.canonicalize()` with a fallback to
  `normalize_candidate_path` comparison when canonicalization fails (e.g.
  path doesn't exist). This is the pattern to reuse/generalize rather than
  invent a new one.
- `ws-dashboard/crates/daemon/src/discovery.rs:374-388` (git branch of
  `discover_existing_dir`) — `workspace_key.id` is built from
  `stable_path_hash(&git.common_dir)`, i.e. the **same** kind of pure textual
  hash, independently vulnerable to the same divergence as `work_root_id`.
- `ws-dashboard/crates/daemon/src/discovery.rs:521-526` callers (system-wide
  scope check, via grep) — `local_work_root_id_for_path` is the **single**
  `WorkRootId` derivation used everywhere: `discovery.rs:83-84,191`,
  `git_worktree.rs:242`, `server.rs:79`, `work_root_files.rs:123,252,259`.
  Fixing this one function fixes identity consistency system-wide with one
  change point, reusing the existing per-workspace dedup guard rather than
  adding new dedup logic anywhere else.
- `ws-dashboard/crates/daemon/src/discovery.rs:694-738`
  (`local_provider_distinguishes_git_primary_roots_and_linked_worktrees`) —
  **existing test gap**: this test is structurally the exact repro shape
  (primary root + its own linked worktree, both passed as separate top-level
  `LocalWorkRootCandidate`s) but only asserts `kinds.contains(...)` for each
  kind, **not** `work_roots.len() == 2` — a duplicate entry would currently
  pass this test silently.
- `ws-dashboard/crates/daemon/src/discovery.rs:669-692`
  (`local_provider_prunes_symlink_when_target_disappears`) — confirms
  `symlink` (unix) is already imported and used in this test module,
  establishing the pattern for a deterministic repro test that forces the
  same-physical-directory-different-string-form condition without depending
  on incidental environment path aliasing.
- `ws-dashboard/frontend/package.json:12` — `test:resource-model` and
  (`:15`) `test:workbench` are the ticket's named frontend verification
  gates; both stay unaffected/green since no frontend file changes.

## Implementation Plan

1. In `ws-dashboard/crates/daemon/src/discovery.rs`, refactor the
   canonicalize-with-fallback pattern already used ad hoc in
   `paths_equivalent` (`discovery.rs:514-519`) into one shared helper, e.g.:

   ```rust
   fn canonical_or_normalized(path: &Path) -> PathBuf {
       path.canonicalize().unwrap_or_else(|_| normalize_candidate_path(path))
   }

   fn paths_equivalent(left: &Path, right: &Path) -> bool {
       canonical_or_normalized(left) == canonical_or_normalized(right)
   }
   ```

2. Use `canonical_or_normalized` inside `local_work_root_id_for_path`
   (`discovery.rs:521-526`) before hashing:

   ```rust
   pub fn local_work_root_id_for_path(path: &Path) -> WorkRootId {
       OpaqueId::from(format!(
           "root-local-{}",
           stable_path_hash(&canonical_or_normalized(path))
       ))
   }
   ```

   This is the single change point that fixes `WorkRootId` consistency for
   every caller listed in Codebase Findings (discovery, git worktree create,
   server registration, work-root-files registry sync) without touching
   `discover_work_root`'s stored `path` field, so displayed labels are
   unaffected.

3. In `discover_existing_dir`'s git branch (`discovery.rs:374-388`), wrap the
   `workspace_key` hash input the same way so cross-candidate
   `workspace_key` bucketing is consistent regardless of which
   worktree/candidate path the `git rev-parse --git-common-dir` call was
   made from:

   ```rust
   id: OpaqueId::from(format!(
       "workspace-local-{}",
       stable_path_hash(&canonical_or_normalized(&git.common_dir))
   )),
   ```

4. Strengthen `local_provider_distinguishes_git_primary_roots_and_linked_worktrees`
   (`discovery.rs:694-738`) to assert `view.workspaces[0].work_roots.len() ==
   2` (currently missing — a real gap that would let a duplicate pass
   silently).

5. Add a new unix test in the same `mod tests` block (following the
   `symlink()` pattern from `local_provider_prunes_symlink_when_target_disappears`,
   `discovery.rs:669-692`) that deterministically reproduces the divergent
   identity-hash condition: create a primary git repo, add a linked
   worktree, then pass the linked worktree as a candidate **twice** — once
   via its direct path and once via a symlink alias pointing at the same
   directory (mirroring how the create-worktree flow's Rust-joined
   `target_path` and the daemon's own `git worktree list --porcelain`
   report of that same path can diverge textually) — and assert exactly one
   `WorkRootView` results for that worktree both before (to document/confirm
   the bug) and after the fix (to prove it). If a plain symlink alias for
   both `candidates` entries doesn't itself reproduce the divergence under
   `cargo test` locally (canonicalize may already collapse the symlink case
   trivially), keep the test as the fix's regression guard for path-alias
   dedup rather than as the literal reproduction of the reported bug — the
   fix and its rationale (Codebase Findings) stand independently of whether
   this specific synthetic alias reproduces it.

## Verification Plan

- `cd ws-dashboard && cargo test -p ws-dashboard-daemon discovery::` — run
  the `discovery.rs` test module, including the strengthened
  `local_provider_distinguishes_git_primary_roots_and_linked_worktrees` and
  new symlink-alias dedup test.
- `cd ws-dashboard/frontend && npm run test:resource-model` — confirm no
  frontend regression (no frontend files touched by this fix, but this is
  the ticket's named gate).
- `cd ws-dashboard/frontend && npm run test:workbench` — ticket's other
  named gate.
- `cd ws-dashboard/frontend && npm run test:git` — exercises
  `gitWorktreeAdd.test.ts`, the closest existing frontend coverage of the
  create-worktree submit flow.
- Manual dogfood: create a worktree through the dashboard's "create
  worktree" action; confirm exactly one entry appears for the new work
  root, and that the primary root and any pre-existing linked worktrees
  still each appear as their own separate entries (not collapsed).

## Escalations

- None.
