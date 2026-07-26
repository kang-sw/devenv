# Plan: 260726-refactor-ws-dashboard-git-fs-watch-invalidation — Phase 2: Resolve git routes against one work root, not all of them

## Relevant Ticket Contract

- Rewrite `resolve_git_context` to resolve a single root via
  `state.opened_work_roots.get(id)` → `None` ⇒ `Unknown` (404); `activation !=
  Online` ⇒ `Offline` (409); `discovery::discover_work_root` (make
  `pub(crate)`) → `availability != Available` ⇒ `Unavailable` (409); `kind`
  not `GitPrimaryRoot | GitLinkedWorktree` ⇒ `NonGit` (400); else
  `GitContext { root_path }`. Drop the `use
  crate::resources::live_dashboard_resources` import. `2N+W` → 1 spawn per
  call.
- **Reuse `resolve_online_available_work_root`** (`work_root_files.rs:798`)
  for the 404/409/409 gate instead of writing a second one; layer only the
  git-`kind` check on top via `discover_work_root`.
- **Memoize `git_identity`** (`work_root_activity.rs:2408`) per root keyed by
  a new `WatchKey`. Not a `OnceCell` — must be evictable. Cache `Some` under
  the normal TTL, cache `None` under a short negative TTL
  (`WS_DASHBOARD_GIT_IDENTITY_NEGATIVE_TTL_MS`, default 3000). This is the
  phase's actual measurable win (0.67/s from the 3s activity poll, ~20/s
  while the Activity pane is open) — it only materializes if `None` is
  cached; "always re-probe on `None`" was explicitly rejected because
  `git_identity` returns `None` for every plain-directory/bare-repo root.
- Add `discovery::watch_key`.
- Behavior deltas to accept and pin: git routes no longer trigger the
  discovered-worktree registry sync (5s resources poll still covers new
  worktrees); an unavailable git root now returns 409 instead of 404
  (convergence onto `resolve_online_available_work_root`'s existing
  invariant, not a new contract).
- **Correct expected win**: `GitProbeCache` is already shared across
  `/git/status`, `/git/branches`, and `/api/dashboard/resources`, so the
  steady-state fan-out reduction from the `resolve_git_context` rewrite alone
  is near zero. The real wins are the `git_identity` memo, removal of the
  inline latency spike on a probe-TTL miss, the 409 correctness convergence,
  and Phase 3's structural prerequisite (git routes no longer depend on
  `live_dashboard_resources`).
- Verification boundary: `/api/dashboard/diag/git` delta with Activity pane
  open drops ~20/s, closed ~0.67/s (dogfood-only, not a `cargo test`
  assertion). Full `cargo test -p ws-dashboard-daemon` green with the named
  `routes.rs` git-toolbar/resources pins executed by name. The 404→409
  change alters visible toolbar text (`refreshGit` surfaces the daemon
  `error` string verbatim) so this phase needs its own `frontend/e2e/`
  browser assertion for the moved-root message — do not defer to Phase 4.

## Out of Scope

- Phase 1 (git exec seam) — already landed (`0c48065a`); every git call in
  this phase must go through the existing `git_exec::capture` wrappers
  (`git_text`/`run_git`/`git_output`), which already thread `&GitSpawnStats`.
- Phase 3 (result cache / `GitStateCache` / `EpochSource`) and Phase 4 (the
  `notify` watcher, `WatchTargets`, `DiscoveredWorkRoot` widening with
  `git_dir`/`common_dir`) — not touched here.
- The reconcile-driven eviction of the `git_identity` memo on a non-Available
  transition — ticket explicitly assigns that to Phase 4's reconcile hook.
  Phase 2 only needs `GitIdentityCache` to expose an `evict` method for
  Phase 4 to call later; no call site is wired now.
- `git_worktree.rs`'s 8 direct `Command::new("git")` sites — explicitly
  outside the seam/budget per Phase 1's Result and a separate ticket
  (`260726-refactor-dashboard-worktree-git-spawns-through-exec-seam`).

## Codebase Findings

- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L344-376` — current
  `resolve_git_context` calls `live_dashboard_resources(&state.opened_work_roots,
  &state.git_probe_cache, &state.git_spawn_stats)` and linear-scans all
  workspaces/roots for one id (`2N+W` spawns). `status_for_path`/
  `branches_for_path`/`run_git`/`git_text` (L378-619, L666-690) already route
  through `git_exec::capture` with `&GitSpawnStats` (Phase 1 landed) — no
  change needed there.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L798-819` — confirmed:
  `resolve_online_available_work_root(state, work_root_id) ->
  Result<PathBuf, WorkRootAccessError>` does exactly `get → activation →
  is_dir/read_dir`, zero git spawns. `WorkRootAccessError` (L771-796) is
  `pub`, has `.status() -> StatusCode` and `.message() -> &'static str`, and
  its three variants (`Unknown`/`Offline`/`Unavailable`) map to identical
  strings and status codes as `GitContextError`'s current
  `Unknown`/`Offline`/`Unavailable` (404 "unknown workRoot", 409 "workRoot
  offline", 409 "workRoot unavailable" — verified byte-for-byte in
  `git_toolbar.rs#L115-132`). **It is directly reusable with no new
  conversion layer beyond a trivial `match`/`From`** — `GitContextError` only
  needs to add nothing to those three variants, just keep its extra `NonGit`
  variant that `WorkRootAccessError` has no equivalent for.
- `ws-dashboard/crates/daemon/src/discovery.rs#L358-422` — `discover_work_root`
  is a **private** `fn` (not `pub(crate)` yet) returning a **private**
  `struct DiscoveredWorkRoot { path, workspace_key, kind, status, availability,
  error }` with **all-private fields**. Making the function `pub(crate)` per
  the ticket is not sufficient by itself: Rust requires the returned type
  (and any field `git_toolbar.rs` reads — `.kind` and `.availability`) to be
  at least as visible as the function, so `DiscoveredWorkRoot` and those two
  fields must also become `pub(crate)`, or the crate will not compile once
  `git_toolbar.rs` calls it. This is a real, non-obvious blocker the ticket
  text doesn't call out.
- `ws-dashboard/crates/daemon/src/discovery.rs#L524-540` — `GitProbeKey::for_path`
  already implements almost exactly the algorithm Decisions describes for
  `WatchKey` (canonicalize/normalize → `\` ⇒ `/` → lowercase on Windows). The
  new `watch_key`/`WatchKey` should mirror this shape as a **separate** type
  (ticket: "a separate `WatchKey` costs ~15 lines and zero blast radius");
  reusing `GitProbeKey` directly would conflate the two caches' keying axes
  the ticket explicitly wants kept apart.
- `ws-dashboard/crates/daemon/src/discovery.rs#L560-582` — `ProbeSlots<T>::get_or_probe`
  is the two-level-lock memo+single-flight pattern (map lock released before
  the per-key `Mutex`), but it takes **one `ttl: Duration` for every cached
  value**. The `git_identity` memo needs **two different TTLs depending on
  whether the probe returned `Some` or `None`**, which `ProbeSlots` cannot
  express as-is. A bespoke small cache (mirroring `ProbeSlots`'s locking
  shape, but selecting `positive_ttl` vs `negative_ttl` based on
  `cached.value.is_some()`) is needed instead of reusing `ProbeSlots<Option<GitIdentity>>`
  verbatim.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L2408-2438` —
  `git_identity` does exactly 2 `git_output`/`capture` calls (`rev-parse
  --show-toplevel`, `rev-parse --path-format=absolute --git-common-dir`),
  already through the seam (Phase 1 landed), `ExpectedNonZero`-classified by
  the comment above it (ok as-is).
- **Confirmed call chain reaching `git_identity` on the 200ms Activity SSE
  loop** (the load this bullet must actually remove):
  `work_root_activity.rs#L403` (`tokio::time::sleep(200ms)`) →
  `#L406` `state.projector.watch_snapshot(...)` →
  `#L174-195` `WorkRootActivityProjector::watch_snapshot` (`spawn_blocking`) →
  `#L587-615` `watch_snapshot_blocking` → calls **both**
  `#L538-549` `project_blocking` (which itself calls
  `resolve_work_root_state_dir` **twice**, at `#L547` and `#L562`) **and**
  `#L602` `activity_item_versions` (which calls `resolve_work_root_state_dir`
  again at `#L623-624`) → `#L522-536` `resolve_work_root_state_dir` calls
  `git_identity` at `#L527` uncached. **Net: 3 uncached `git_identity` calls
  (6 git spawns) per SSE tick per open root today**, not the 2-spawn estimate
  implied by "0.67/s from the 3s poll" read literally — the memo must sit at
  (or below) `resolve_work_root_state_dir`/`git_identity` so all three call
  sites within one tick share one cached answer, not just the first.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L87-98` —
  `WorkRootActivityProjector` (`#[derive(Clone, Debug, Default, Eq,
  PartialEq)]`) only holds `cache_home`/`codex_home`; `git_stats` is threaded
  as an explicit method/function parameter throughout (`project`,
  `project_with_recent_limit`, `watch_snapshot`, `project_blocking`,
  `watch_snapshot_blocking`, `activity_item_versions`,
  `resolve_work_root_state_dir`, `git_identity`/`git_output`). The new
  `GitIdentityCache` must follow this exact established threading pattern
  (Code Standards #4: explicit dependencies) rather than becoming
  ambient/global state — add it as a new `WorkRootActivityProjector` field
  (needs its own cheap-`Clone`/`Default` impl, mirroring `GitProbeCache`'s
  `Arc<...State>` inner-Arc shape) and pass `&GitIdentityCache` down the same
  chain as `git_stats`.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L507-520` —
  `resolve_work_root_agents_dir` is the one function Phase 1's Result
  confirms has "zero `src/` callers" and is allowed a throwaway
  `GitSpawnStats::default()`. The same exemption applies to a throwaway
  `GitIdentityCache::default()` there — no behavioral-contract violation.
- `ws-dashboard/crates/daemon/src/server.rs#L119` —
  `work_root_activity: WorkRootActivityProjector::default()` is the only
  production construction site; adding a `Default`-derivable field keeps
  this call site unchanged.
- `ws-dashboard/crates/daemon/src/discovery.rs#L690-696` —
  `git_probe_ttl_from_env` is the existing pattern for a TTL env-var reader
  (`OnceLock`-backed, `WS_DASHBOARD_GIT_PROBE_TTL_MS`, default 30000ms). The
  ticket names only the **negative** TTL env var
  (`WS_DASHBOARD_GIT_IDENTITY_NEGATIVE_TTL_MS`, default 3000) and says cache
  `Some` under "the normal TTL" without naming a distinct env var for it.
  **Open implementation call, not a blocker**: reuse
  `WS_DASHBOARD_GIT_PROBE_TTL_MS`'s value for the positive TTL (consistent
  with "normal TTL" reading as the existing probe-cache convention) rather
  than inventing a second positive-TTL knob; only add the one new negative-TTL
  env var the ticket names.
- `ws-dashboard/crates/daemon/tests/routes.rs#L7672-7739` —
  `git_toolbar_status_gates_and_reports_counts_without_paths` currently pins
  exactly 4 cases (200 available / 400 plain / 409 offline / 404 unknown-id).
  No existing case exercises `WorkRootAccessError::Unavailable`/moved-root.
  `discover_work_root`'s own "moved" classification
  (`discovery.rs#L389-395`) fires when `fs::metadata` returns `NotFound` but
  the parent still exists — the existing discovery unit test at
  `discovery.rs#L1121-1138` (`local_provider_prunes_workspaces_without_available_work_roots`)
  shows the fixture shape (`parent.join("moved")`, parent created, child
  never created) to reuse for the new routes.rs case; opening that path and
  then calling `/git/status` will already be caught by
  `resolve_online_available_work_root`'s `is_dir`/`read_dir` gate before
  `discover_work_root` runs, so this single test scenario exercises the
  primary path, not a corner case.
- `ws-dashboard/crates/core/src/resources.rs#L40-43` — `WorkRootKind` has
  exactly `PlainDirectory | GitPrimaryRoot | GitLinkedWorktree`, already
  imported in `git_toolbar.rs#L10`.
- `ws-dashboard/frontend/src/gitToolbar.ts#L71-73` — `refreshGit`'s fetch
  helper throws `body?.error` verbatim, confirming the 404→409 message swap
  is browser-visible and needs the e2e assertion the ticket requires; no
  existing `frontend/e2e/dashboard-acceptance.spec.ts` case currently asserts
  a moved/unavailable-root toolbar message (only worktree-move-pane UX tests
  exist, unrelated).

## Implementation Plan

1. `ws-dashboard/crates/daemon/src/discovery.rs`: change `fn
   discover_work_root` (L367) to `pub(crate) fn discover_work_root`; change
   `struct DiscoveredWorkRoot` (L358) to `pub(crate) struct DiscoveredWorkRoot`
   and mark at least the `kind` and `availability` fields `pub(crate)` (leave
   `path`/`workspace_key`/`status`/`error` private unless another Phase-2 use
   surfaces).
2. `ws-dashboard/crates/daemon/src/discovery.rs`: add a `WatchKey` newtype and
   `pub(crate) fn watch_key(path: &Path) -> WatchKey`, mirroring
   `GitProbeKey::for_path` (L524-540)'s canonicalize/backslash/lowercase
   algorithm as a distinct type per the ticket's "separate `WatchKey`"
   decision. Keep it small (~15 lines) and free of git spawns.
3. `ws-dashboard/crates/daemon/src/git_toolbar.rs`: rewrite `resolve_git_context`
   (L344-376):
   - Replace the `live_dashboard_resources` call with
     `resolve_online_available_work_root(state, work_root_id)`, mapping its
     `WorkRootAccessError` to the existing `GitContextError::{Unknown,Offline,Unavailable}`
     (identical messages/status codes, confirmed above).
   - On success, call `discovery::discover_work_root(&root_path,
     &state.git_probe_cache, &state.git_spawn_stats)`; if
     `.availability != WorkRootAvailability::Available` return
     `GitContextError::Unavailable`; if `.kind` is not
     `GitPrimaryRoot`/`GitLinkedWorktree` return `GitContextError::NonGit`;
     else return `GitContext { root_path }`.
   - Remove the now-unused `use crate::resources::live_dashboard_resources;`
     import (L13); add `use crate::discovery::discover_work_root;` and `use
     crate::work_root_files::{resolve_online_available_work_root,
     WorkRootAccessError};`.
4. `ws-dashboard/crates/daemon/src/work_root_activity.rs`: add a
   `GitIdentityCache` type near `GitIdentity`/`git_identity` (L2400-2438):
   a two-level-lock memo (mirroring `ProbeSlots`'s locking shape in
   `discovery.rs#L548-582`) keyed by `WatchKey`, that selects `positive_ttl`
   when the cached value is `Some` and `negative_ttl`
   (`WS_DASHBOARD_GIT_IDENTITY_NEGATIVE_TTL_MS`, default 3000) when it is
   `None`; expose `get_or_probe(&WatchKey, impl FnOnce() -> Option<GitIdentity>)
   -> Option<GitIdentity>` and `evict(&WatchKey)` (unused by Phase 2, kept for
   Phase 4). Give it a cheap-`Clone`/`Default` impl (inner `Arc<...State>`,
   same shape as `GitProbeCache`).
5. Thread `&GitIdentityCache` through the exact chain identified above,
   parallel to the existing `git_stats` parameter: add a field on
   `WorkRootActivityProjector` (L87-90, keep `#[derive(Default)]` working);
   thread through `project`/`project_with_recent_limit` (L100-117+),
   `watch_snapshot` (L174-195), `project_blocking` (L538-549),
   `watch_snapshot_blocking` (L587-615), `activity_item_versions`
   (L617-659), and `resolve_work_root_state_dir` (L522-536) — replace the
   direct `git_identity(root_path, git_stats)` call at L527 with
   `git_identity_cache.get_or_probe(&discovery::watch_key(root_path), ||
   git_identity(root_path, git_stats))`. Give `resolve_work_root_agents_dir`
   (L507-520) a throwaway `GitIdentityCache::default()` alongside its
   existing throwaway `GitSpawnStats::default()`.
6. `ws-dashboard/crates/daemon/src/discovery.rs`: no availability/eviction
   change needed beyond step 1 — the existing `git_probes.evict` call at
   L416-420 stays as-is; the new identity cache's eviction stays unwired
   until Phase 4 per Out of Scope.

## Verification Plan

- `cargo test -p ws-dashboard-daemon` (lib + `tests/routes.rs`) full green;
  confirm `git_toolbar_status_gates_and_reports_counts_without_paths` and the
  resources-poll pins are reported by name, not just by count.
- Extend `git_toolbar_status_gates_and_reports_counts_without_paths`
  (`tests/routes.rs#L7672`) with a 5th case: open a work root at
  `base.join("moved")` (parent created, child never created — same fixture
  shape as `discovery.rs`'s `local_provider_prunes_workspaces_without_available_work_roots`),
  then assert `GET .../git/status` → 409 with `error == "workRoot
  unavailable"`.
- Add a unit test for the `GitIdentityCache` negative-TTL behavior: probing a
  plain-directory root's `git_identity` twice inside the negative TTL window
  must not increase the `GitSpawnStats` total (assert via the shared
  counters, same technique as
  `dashboard_diag_git_reports_spawn_counters_that_increase_after_git_toolbar_calls`);
  a git-identified root's second call inside the positive TTL must likewise
  add zero spawns. Explicitly cover a plain-directory root per the ticket's
  "verify with a plain-directory root selected, not only a git one."
- Add/extend a `watch_key` unit test in `discovery.rs` pinning the
  canonicalize/backslash/lowercase normalization contract, following the
  existing `GitProbeKey` test conventions in that file.
- `frontend/e2e/dashboard-acceptance.spec.ts`: add a browser-level assertion
  that a moved/unavailable git work root surfaces the daemon's "workRoot
  unavailable" message through the toolbar UI (per `gitToolbar.ts#L71-73`'s
  verbatim `error` passthrough) — required for this phase, not deferrable to
  Phase 4.
- Live/dogfood only (not a `cargo test` gate): two `/api/dashboard/diag/git`
  reads with the Activity pane open vs. closed, to confirm the ~20/s and
  ~0.67/s drops respectively; explicitly not the fan-out figure per the
  ticket's own correction.

## Escalations

- None.

## Lead Dispositions

These override the sections above wherever they conflict. Every claim below was
verified by the lead against source at `9d946cb3`; file:line references are the
evidence, not a paraphrase of the ticket.

### D1 (binding, replaces plan steps 4-5): derive `git_identity` from the existing shared `GitProbeCache` — do not add a second cache

`GitDiscovery::probe` (`discovery.rs:706-752`) already issues **one** spawn:
`git rev-parse --show-toplevel --path-format=absolute --git-common-dir --git-dir`,
and stores `worktree_dir` + `common_dir` (both `normalize_candidate_path`d) plus
`kind`. `git_identity` (`work_root_activity.rs:2408-2438`) issues **two** spawns
for a strict subset of those same flags, then canonicalizes both results and
requires the common dir to be named `.git`. So the identity is derivable from the
already-memoized probe with **zero additional spawns**, and a cold miss costs 1
spawn instead of today's 2.

Do this instead of `GitIdentityCache`:

- Move `GitIdentity { worktree_root, common_root }` into `discovery.rs` as
  `pub(crate)` — it is git-topology knowledge, which is discovery's
  responsibility, and it must sit next to the probe it now derives from.
- Add `pub(crate) fn GitProbeCache::git_identity(&self, path: &Path, git_stats:
  &GitSpawnStats) -> Option<GitIdentity>`: call the existing private
  `self.discover(...)`, then `canonicalize(worktree_dir)`,
  `canonicalize(common_dir)`, require `file_name() == Some(".git")`,
  `canonicalize(common_dir.parent()?)`. Preserve every `?`/`.ok()?` bail-out the
  current `git_identity` has — this must stay a total function returning `None`.
- Delete `work_root_activity.rs`'s local `GitIdentity` and `git_identity`, and
  delete `git_output` too if `git_identity` was its only caller. Leave no dead
  code and no new warnings.
- Thread `&GitProbeCache` down the same chain as `git_stats`
  (`project`/`project_with_recent_limit`/`watch_snapshot`/`project_blocking`/
  `watch_snapshot_blocking`/`activity_item_versions`/
  `resolve_work_root_state_dir`), sourced from `AppState.git_probe_cache`.
  **Do not add a field to `WorkRootActivityProjector`**: it derives
  `Eq, PartialEq` at `work_root_activity.rs:86`, and an `Arc`-backed cache field
  breaks that derive and forces a hand-written impl — the exact cost Phase 1 paid
  for `LocalDashboardResourcesProvider`. The explicit parameter is both the
  established pattern here and the cheaper one.
- The memo is genuinely shared with the 5 s resources poll: `GitProbeKey::for_path`
  (`discovery.rs:527-539`) keys on `canonical_or_normalized`, so the toolbar,
  Activity, and resources paths collapse onto one entry for the same directory
  regardless of path spelling. That is what makes the Activity cost ~0 rather
  than merely lower.

Rejected: a bespoke dual-TTL `GitIdentityCache`. It duplicates `ProbeSlots`'s
two-level-lock single-flight logic (`discovery.rs:559-582`) — the subtle part,
which a reviewer would then have to re-verify — and leaves the daemon with two
git-topology probes that normalize differently. The survey's premise that
`ProbeSlots` "takes one `ttl` for every cached value" is imprecise: `ttl` is a
per-call argument (`discovery.rs:560`). The real limitation is that the caller
cannot pick the TTL from the *cached* value, which is why D2 exists.

### D2 (binding, deliberate deviation from the ticket): no `WS_DASHBOARD_GIT_IDENTITY_NEGATIVE_TTL_MS`; negative caching is the existing 30 s probe TTL

The ticket's negative TTL exists so a plain directory the user later `git init`s
is not stuck on a memoized `None`. Under D1 that hazard is already bounded twice:
the 30 s probe TTL, and `discover_work_root`'s unconditional
`git_probes.evict(...)` on **any** non-`Available` result
(`discovery.rs:416-420`), plus `GitProbeCache::clear()` after this daemon's own
worktree mutations.

The positive argument for 30 s over 3 s: it makes the Activity pane's identity
**consistent with the sidebar classification the user is looking at**. The
sidebar's git-vs-plain label comes from the same 30 s memo, so a 3 s identity TTL
would make the pane self-correct ~27 s before the root stops being labelled a
plain directory. The ticket could not weigh that because it assumed identity
needed a cache of its own.

Escape hatch if the fit reviewer rejects this: add `negative_ttl` to
`GitProbeCacheState` and change `ProbeSlots::get_or_probe`'s `ttl: Duration` to
`ttl_for: impl Fn(&T) -> Duration` (the two existing call sites pass
`|_| self.inner.ttl`). That is ~10 lines on top of D1, not a redesign. **Do not
pre-build it.**

### D3 (binding): defer `WatchKey` / `watch_key` to Phase 3

Under D1 nothing in Phase 2 consumes it — the memo keys on the existing
`GitProbeKey`. Phase 3's `GitStateCache` (ticket line 735) is its first real
consumer. Adding it now means `#[allow(dead_code)]`, and the impl-playbook's
warning rule does not accept suppression without a cause to point at.

### D4 (binding, drops plan step 1): leave `discover_work_root` and `DiscoveredWorkRoot` private

Widening them is unnecessary. After `resolve_online_available_work_root`,
availability is already settled: every non-`Available` case discovery can produce
(`Inaccessible` / `Moved` / `Missing`, `discovery.rs:379-414`) is already rejected
by that gate's `is_dir()` / `read_dir()` check — the survey observed this itself
when designing the moved-root test. The only thing `resolve_git_context` still
needs is the git-vs-plain answer, so add
`pub(crate) fn GitProbeCache::git_root_kind(&self, path, git_stats) -> Option<WorkRootKind>`
and keep discovery's types private. Match the returned kind explicitly against
`GitPrimaryRoot | GitLinkedWorktree` rather than treating `Some` as "is git", so a
future third git kind cannot pass silently.

### D5 (confirmed, binding): reuse `resolve_online_available_work_root` as-is

Verified byte-identical, so the mapping is a trivial `match`/`From` and needs no
conversion layer: messages `"unknown workRoot"` / `"workRoot offline"` /
`"workRoot unavailable"` and codes 404/409/409 at `work_root_files.rs:779-791`
against `git_toolbar.rs:116-132`. `GitContextError` keeps its extra `NonGit`
variant. Also verified `WorkRootActivation` has exactly `Online | Offline`
(`crates/core/src/resources.rs:31-34`), so that gate's `== Offline` test is
equivalent to the ticket's `!= Online` wording — no behavior gap there.

Also still binding from the ticket: drop the
`use crate::resources::live_dashboard_resources;` import from `git_toolbar.rs`.

### D6 (binding): required tests, in addition to the Verification Plan above

- **Identity equivalence.** For a primary root and for a linked worktree, assert
  the resolved wsstate `proj/<key>` directory is **unchanged** from the
  pre-change two-spawn derivation. This is the one way a normalization difference
  between `normalize_candidate_path`-then-`canonicalize` and raw `canonicalize`
  could silently repoint every Activity state-dir lookup. If equivalence cannot
  be made to hold, **stop and report** — do not adjust the expected key to match
  the new output.
- **`None` cases preserved:** a bare repo and a plain directory must both still
  yield no state dir.
- **Zero-spawn:** with the discovery memo warm, an Activity snapshot resolution
  over the same root adds 0 to `GitSpawnStats`, and a `/git/status` call adds 0.
  Include a plain-directory root explicitly, per the ticket.
- The moved-root 409 `routes.rs` case and the `frontend/e2e/` moved-root toolbar
  assertion stand as planned.

### D7 (confirmed): the three-calls-per-tick finding stands

`resolve_work_root_state_dir` is reached three times per 200 ms SSE tick
(`project_blocking` at `:547` and `:562`, `activity_item_versions` at `:623`).
Under D1 all three collapse onto one memo entry, so no per-call-site plumbing is
needed beyond passing the cache down the chain.
