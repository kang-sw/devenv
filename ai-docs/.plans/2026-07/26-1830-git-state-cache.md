# Plan: Replace interval-driven git polling with FS-watch-driven epoch invalidation — Phase 3: Result cache for `/git/status` and `/git/branches`, epoch stubbed

## Relevant Ticket Contract

- `status_for_path` and `branches_for_path` take `(&GitStateCache, &EpochSource, &Path)` and read/fill two slot parts (`worktree`, `refs`) keyed by `WatchKey`. Stub shape (ticket, Phase 3 heading):
  ```rust
  struct RefState { branch_name, detached_oid, upstream, sync, branch_list, checked_out }
  struct GitCacheSlot { worktree: Option<(u64, Instant, GitChangeSummary)>, refs: Option<(u64, Instant, RefState)> }
  pub struct GitStateCache { slots: Arc<Mutex<HashMap<WatchKey, Arc<Mutex<GitCacheSlot>>>>> }
  ```
- `changes_for_path` stays a pure function; its four in-file tests must keep working verbatim.
- **Sample the epoch BEFORE invoking git, and stamp the slot with that sample** — not a value read after the git call returns. A unit test must pin it: bump the epoch between the sample and the fill, then assert the next read is a miss.
- `EpochSource` is a trait with a `StaticZero` impl in this phase and the watcher impl in Phase 4, so this phase is TTL-only and independently testable, and Phase 4 becomes purely "make the epoch real."
- Mutating routes bump epochs directly so a user action is never TTL-delayed: `git_switch_branch`, `git_create_branch`, and the fetch/push/pull `mutate_no_body` paths bump `refs`; `git_pull_ff_only` also bumps `worktree`.
- TTL from `WS_DASHBOARD_GIT_CACHE_TTL_MS`, default 2000 while `StaticZero`.
- Verification boundary: hit `/git/status` twice inside the TTL and assert via Phase 1's counter that the second call adds **zero** spawns; then `POST /git/switch-branch` and assert the next `/git/status` reflects the new branch immediately (epoch bump beats TTL). `git_toolbar_branches_switch_and_create_revalidate_state` (`tests/routes.rs:7981`) must pass unmodified. Not covered: cross-`serverRoute` behavior — already asserted by existing forwarding tests, no new test needed there.
- Phase 3 depends on Phase 1 (spawn counter makes "second call added zero spawns" assertable) — landed (`git_exec::capture` + `GitSpawnStats`, `GET /api/dashboard/diag/git`).
- Phase 2 landed `resolve_git_context` on `resolve_online_available_work_root` + `GitProbeCache::git_root_kind` (git_toolbar.rs:364-380); its Result explicitly deferred `WatchKey`/`watch_key` to Phase 3 (Lead Disposition D3, `ai-docs/.plans/2026-07/26-1717-per-root-git-context.md:382-387`): "Adding it now means `#[allow(dead_code)]`... Phase 3's `GitStateCache` is its first real consumer." **Phase 3 owns introducing `WatchKey`/`discovery::watch_key`.**
- Estimated diff ~+270/−95 production, ~+160 test, 4 files.

## Out of Scope

- Phase 4: the real `notify` watcher, `WatchTargets`, `IgnoreSet`, `classify`, `plan_watch_set`, the `reconcile` hook, and the FS-event pipeline that eventually drives `EpochSource`'s bumps. Phase 3 ships a `StaticZero`-named impl whose only bump source is the explicit mutating-route calls (see Codebase Findings — this is a naming trap, not a literal always-zero source).
- `DiscoveredWorkRoot` widening (`git_dir`/`common_dir` fields) — folded into Phase 4 per ticket "Already Landed" section.
- Frontend scheduler changes, SSE push, `DocumentEventHub`-style protocol.
- `servers.rs` — confirmed below that local delegation calls the public route handlers (`git_status`, `git_branches`, `git_create_branch`), not the internal `status_for_path`/`branches_for_path` functions, so no `servers.rs` edit is needed.
- `git_worktree.rs`'s 8 direct `Command::new("git")` sites (tracked separately as `260726-refactor-dashboard-worktree-git-spawns-through-exec-seam`).
- Any change to `GitProbeCache` / `ProbeSlots<T>` in `discovery.rs` — reused for its normalization pattern only (see Codebase Findings on why `GitStateCache` does not literally reuse `ProbeSlots<T>`).

## Codebase Findings

- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L382-L431` (`status_for_path`) issues, in the typical case (named branch, no upstream): `branch --show-current` (L383-389, always) + `rev-parse --abbrev-ref --symbolic-full-name @{upstream}` (L401-412, always attempted, `ExpectedNonZero`) + `changes_for_path`'s 2 spawns (L492-568: `diff-index -M --numstat HEAD --` and `status --porcelain=v1 --untracked-files=all`) = **4 spawns**. Conditionally +1 for `rev-parse --short HEAD` (L392-397) when detached, +1 for `rev-list --left-right --count` via `rev_counts` (L593-605) when an upstream resolved. Together with `resolve_git_context`'s 1 memoized `git_root_kind` probe (cold), this matches the ticket's own stated "five spawns `/git/status` issues" (Phase 2 Result, "Not verified" section).
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L433-L490` (`branches_for_path`) issues `branch --show-current` (L434-440, always) + `worktree list --porcelain` via `checked_out_branches` (L451, L640-652, always) + `for-each-ref --format=... refs/heads` (L452-461, always) + one `rev-list --left-right --count` per branch line with a non-empty upstream (L471, L582-604) = **`3 + B` spawns**, matching the ticket's Decisions figure exactly. Conditionally +1 for detached `rev-parse --short HEAD` (L441-448).
- **Numeric hit/miss delta for the new cache.** `RefState` (per the stub) is the *union* of what both routes need (`branch_name, detached_oid, upstream, sync` for status; `branch_list, checked_out` for branches). So the refs-slot cold-fill cost is the union, not either route's isolated cost: `branch --show-current` (1) + `rev-parse --short HEAD` (0-1) + `@{upstream}` (1) + current-branch `rev-list` (0-1) + `worktree list --porcelain` (1) + `for-each-ref` (1) + B×`rev-list` = **4+B to 6+B spawns**, once per refs-epoch generation, regardless of which route triggers it first. Combined with the worktree slot's 2 spawns, the very first cold call to *either* route now costs **6+B to 8+B** spawns instead of today's isolated 4-6 or 3+B — but every subsequent call to *either* route within the same epoch/TTL window costs **0**. This is the correct reading of the ticket's stub, not a regression to fix; flagging it because a narrower `RefState` (e.g. one holding only what the calling route needs) would silently break the "second call adds zero spawns" guarantee for the *other* route and must not be built instead.
- **`EpochSource`/`StaticZero` naming trap.** The literal name "StaticZero" plus "this phase is TTL-only" could be misread as "epochs are hardcoded to the constant 0 forever," which would make the ticket's own Phase 3 acceptance test impossible: "`POST /git/switch-branch` and assert the next `/git/status` reflects the new branch immediately (epoch bump beats TTL)" is an explicit **Phase 3** verification-boundary item, not deferred to Phase 4. The only reading that satisfies both the name and the test is: `EpochSource` must expose bump methods and hold real per-key mutable counters; "StaticZero" describes the *absence of a background/watcher-driven writer* in this phase (the only writers are the explicit mutating-route calls), not a literal immutable constant. Phase 4 adds a second writer (the FS-event pipeline) to the same store/trait; it does not change the trait's call sites in `git_toolbar.rs`.
- `ws-dashboard/crates/daemon/src/discovery.rs#L524-L540` (`GitProbeKey::for_path`) — the exact normalization `WatchKey` needs per Decisions ("canonicalize → normalize → `\` ⇒ `/` → lowercase on Windows"): `canonical_or_normalized(path)` → replace `\` with `/` → lowercase on Windows. `GitProbeKey` is intentionally a separate type from `WorkRootId` (see doc comment L515-522) for the same reason `WatchKey` must be separate (ticket Decisions, "`WorkRootId` derivation stays frozen"). Add `WatchKey`/`discovery::watch_key(path) -> WatchKey` as a sibling of `GitProbeKey` in `discovery.rs`, `pub(crate)` visibility, reusing the same normalization logic (a `From<&Path>` or a shared private helper both key types call is fine — do not duplicate the `\`→`/`→lowercase chain verbatim without at least factoring the shared piece, since two independently-maintained copies is exactly the kind of drift the ticket's `GitProbeKey` doc comment warns about).
- `ws-dashboard/crates/daemon/src/discovery.rs#L548-L605` (`ProbeSlots<T>`) — the existing two-level-lock memo (outer `Mutex<HashMap<K, Arc<Mutex<Option<CachedProbe<T>>>>>>`, map lock released before the per-key lock is acquired, single-flight). **`GitStateCache` should NOT literally reuse `ProbeSlots<T>`, but should reuse its locking discipline as a pattern.** Reasons: (1) `ProbeSlots::get_or_probe`'s validity check is TTL-only (`cached.probed_at.elapsed() < ttl`); `GitStateCache` needs a compound check (`stored_epoch == current_epoch && age < ttl`), and `get_or_probe` has no parameter for an externally-supplied comparator value. (2) `GitCacheSlot` has two *independently* revalidated parts (`worktree: GitChangeSummary`, `refs: RefState`) with different epoch kinds and different value types; a single-generic-`T` `ProbeSlots<T>` would need `T = GitCacheSlot` and then lose the ability to revalidate/fill one part while reusing the other — which is the entire point of splitting the ticket's two epochs. Forcing this through `ProbeSlots<T>` would mean reimplementing the per-field validity logic inside the closure anyway, negating the reuse. Build `GitStateCache` as its own small struct with the same map-lock/per-key-lock shape (mirrors the ticket's stub verbatim), not as a `ProbeSlots<GitCacheSlot>` instantiation.
- `ws-dashboard/crates/daemon/src/codex_app_server.rs#L709-L711` (`pub trait CodexWorkRootResolver: Send + Sync { fn resolve_cwd(...) -> Result<...>; }`, stored as `Arc<dyn CodexWorkRootResolver>`) and the identical `ClaudeWorkRootResolver` pattern in `claude_cli.rs` — this codebase's existing idiom for a pluggable strategy behind a stable field type, injected once at construction (production vs. test). `EpochSource` should follow this exact shape: `pub(crate) trait EpochSource: Send + Sync { fn epochs(&self, key: &WatchKey) -> (u64, u64); fn bump_worktree(&self, key: &WatchKey); fn bump_refs(&self, key: &WatchKey); }`, stored as `pub epoch_source: Arc<dyn EpochSource>` in `AppState`. A generic `AppState<E>` is ruled out structurally: axum's `Router<S>` requires one concrete `S` shared across the whole router (`router.rs:111` `build_router(state: AppState) -> Router`), so parameterizing `AppState` would force parameterizing `build_router` and every handler's `State<AppState>` extractor. An enum (`EpochSourceKind::StaticZero | Watched(...)`) would also work and avoids one virtual-dispatch indirection, but every future variant addition requires editing the match wherever `epochs()`/`bump_*` are called, whereas `Arc<dyn EpochSource>` requires editing only the one construction site (`server.rs:108-126`) — matching "Phase 4 becomes purely 'make the epoch real'" more literally. Recommend the trait-object shape; it is also the path of least resistance given the codebase already has two working precedents.
- `ws-dashboard/crates/daemon/src/router.rs#L80-L109` (`AppState`) and `#L111` (`build_router`) — add `pub git_state_cache: GitStateCache` and `pub epoch_source: Arc<dyn EpochSource>` fields next to the existing `git_probe_cache`/`git_spawn_stats` fields, following the same "shared across routes, not rebuilt per request" doc-comment convention already on those two fields.
- `ws-dashboard/crates/daemon/src/server.rs#L108-L126` — the sole production `AppState { ... }` literal; add the two new fields (`GitStateCache::default()`, `Arc::new(StaticZeroEpochSource::default())` or similar).
- `ws-dashboard/crates/daemon/tests/routes.rs` has **five** additional full `AppState { ... }` literals that will need the two new fields added mechanically: `app_state_with_opened_and_store` (L164-187), `app_state_with_static_dir` (L189-211), the `expired_pairing_tokens_do_not_install_sessions` test's inline literal (L422+), `app_state_with_activity_cache_and_codex_home` (L8461+), and `app_state_with_translation_provider` (L14226+). None use struct-update (`..`) syntax, so this is six total edit sites (server.rs + 5), each a one-line addition — flagged so the executor budgets for it rather than discovering it mid-implementation.
- `ws-dashboard/crates/daemon/src/servers.rs#L1359-L1397` (`server_scoped_git_status`, `server_scoped_git_branches`) — confirmed local delegation calls the public handlers `git_status(State(state), AxumPath(work_root_id)).await` / `git_branches(...)` / `git_create_branch(...)` directly, not `status_for_path`/`branches_for_path`. No signature or logic change needed in `servers.rs`.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L221` (`git_switch_branch` success arm), `#L281` (`git_create_branch` success arm), `#L345` (`mutate_no_body` success arm) — in current code, each of these three functions has **exactly one** `Ok(())` match arm following its `run_git(...)` call, and that arm is the *only* 2xx response path in the function; every other return (validation failures, `resolve_git_context` errors) is a 4xx/404/409 response that never mutated anything. So there is no existing code path that returns success without reaching the `Ok(())` arm — the epoch bump is safe to insert directly inside that arm, before the (now cache/epoch-aware) call to `status_for_path`. `mutate_no_body` (`git_toolbar.rs#L332-355`) is shared by `git_fetch`/`git_push` (bump `refs` only) and `git_pull_ff_only` (bump `refs` **and** `worktree`); it needs a new parameter (e.g. an `EpochBump` enum with `RefsOnly`/`RefsAndWorktree` variants) so the three call sites (`git_fetch` L293-304, `git_push` L306-317, `git_pull_ff_only` L319-330) can each pass the right one.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L492-L568` (`changes_for_path`) — already a pure function `fn(&Path, &GitSpawnStats) -> GitChangeSummary` with no cache/epoch dependency; its signature does not need to change. The four in-file tests at L751, L794, L821, L874 all call it with the existing 2-argument form and stay untouched. This directly confirms the ticket's claim.
- `ws-dashboard/crates/daemon/tests/routes.rs#L7672` (`git_toolbar_status_gates_and_reports_counts_without_paths`) and `#L7981` (`git_toolbar_branches_switch_and_create_revalidate_state`) are the two named pins the Verification boundary requires to keep passing; `#L7921` (`git_toolbar_status_adds_zero_spawns_after_resources_poll_warms_the_shared_discovery_memo`) is the existing zero-spawn pattern (compares `diag/git` `totalSpawns` before/after) to model the new "second call adds zero spawns" test on.
- `ws-dashboard/crates/daemon/src/git_exec.rs#L468` (`capture`) and `#L190-L246` (`GitSpawnStatsSnapshot`/`GitSpawnStats::snapshot`) — unaffected; `status_for_path`/`branches_for_path`'s fill closures keep calling `git_text`/`run_git` exactly as today, just wrapped by the cache instead of always executing.
- `ws-dashboard/crates/daemon/src/discovery.rs#L727-L733` (`git_probe_ttl_from_env`) is the existing pattern to model `WS_DASHBOARD_GIT_CACHE_TTL_MS` parsing on (`env::var(...).ok().and_then(|raw| raw.trim().parse::<u64>().ok()).unwrap_or(DEFAULT)`).

## Implementation Plan

1. `ws-dashboard/crates/daemon/src/discovery.rs`: add `WatchKey` (newtype wrapping the same normalized-`String` shape as `GitProbeKey`) and `pub(crate) fn watch_key(path: &Path) -> WatchKey`, next to `GitProbeKey`/`GitProbeKey::for_path` (L523-540). Factor the shared `canonical_or_normalized` → `\`→`/` → lowercase-on-Windows chain so both key types call one private helper rather than duplicating it.
2. New module `ws-dashboard/crates/daemon/src/git_state_cache.rs` (mirrors the existing `git_exec.rs` split-out-a-seam precedent): define `EpochSource` trait (`epochs`, `bump_worktree`, `bump_refs`, keyed by `WatchKey`), a `StaticZeroEpochSource` impl backed by `Mutex<HashMap<WatchKey, (u64, u64)>>` (or an `Arc<(AtomicU64, AtomicU64)>` per key) that starts every key at `(0, 0)` and only advances via the bump methods, plus `RefState` (`branch_name: Option<String>, detached_oid: Option<String>, upstream: Option<String>, sync: GitSyncSummary, branch_list: Vec<GitBranchEntry>, checked_out: BTreeSet<String>`, deriving `Clone`), `GitCacheSlot`, and `GitStateCache` with the two-level lock from the ticket stub and two get-or-fill methods (`worktree`/`refs`) whose signature takes the **already-sampled** epoch as a parameter (caller samples before calling git, per the correctness requirement) plus a `ttl: Duration` and a `probe: impl FnOnce() -> T` closure. Add `git_cache_ttl_from_env()` modeled on `discovery.rs#L727-733`.
3. `ws-dashboard/crates/daemon/src/git_toolbar.rs`: add a `compute_ref_state(root: &Path, stats: &GitSpawnStats) -> RefState` helper that folds today's inline branch-name/detached-oid/upstream/sync computation from `status_for_path` (L383-413) together with today's branch-list/checked-out computation from `branches_for_path` (L434-484) into one function — this is the refs-slot fill closure. Rewrite `status_for_path`/`branches_for_path` (L382-490) to take `(&GitStateCache, &dyn EpochSource, root: &Path, stats: &GitSpawnStats)`: compute `let key = watch_key(root);` then `let (wt_epoch, refs_epoch) = epoch_source.epochs(&key);` **first, before either fill closure runs**; then `cache.worktree(&key, wt_epoch, ttl, || changes_for_path(root, stats))` and `cache.refs(&key, refs_epoch, ttl, || compute_ref_state(root, stats))`; assemble `WorkRootGitStatus`/`GitBranchList` from the two results exactly as today.
4. Update the 6 call sites that invoke `status_for_path`/`branches_for_path` to pass `&state.git_state_cache, &*state.epoch_source` (or equivalent): `git_status` (L147-165), `git_branches` (L167-185), `git_switch_branch` (L187-231), `git_create_branch` (L233-291), `mutate_no_body` (L332-355).
5. Add the epoch bump calls, exactly in the three `Ok(())` success arms identified above (`git_toolbar.rs#L221`, `#L281`, `#L345`), calling `state.epoch_source.bump_refs(&key)` before the now-cache-aware `status_for_path` call. Add an `EpochBump` parameter to `mutate_no_body` and thread `RefsOnly` from `git_fetch`/`git_push` and `RefsAndWorktree` from `git_pull_ff_only`, bumping `worktree` too in that case.
6. `ws-dashboard/crates/daemon/src/router.rs`: add `pub git_state_cache: GitStateCache` and `pub epoch_source: Arc<dyn EpochSource>` to `AppState` (L80-109), with doc comments matching the existing `git_probe_cache`/`git_spawn_stats` convention.
7. `ws-dashboard/crates/daemon/src/server.rs#L108-126`: construct `git_state_cache: GitStateCache::default()` and `epoch_source: Arc::new(git_state_cache::StaticZeroEpochSource::default())`.
8. `ws-dashboard/crates/daemon/tests/routes.rs`: add the same two fields to the five full `AppState { ... }` literals listed in Codebase Findings.
9. Unit tests in `git_state_cache.rs`: (a) TTL-only hit/miss with a fixed epoch; (b) the epoch-sample-before-git race — construct a fake `EpochSource`/counter the test controls, run a fill whose probe closure bumps the epoch mid-execution (simulating a concurrent write landing during the git spawn), assert the slot is stamped with the pre-probe epoch, then assert the *next* read (which samples the now-bumped epoch) is a miss; (c) `bump_worktree`/`bump_refs` are independent (bumping one does not invalidate the other's slot).
10. Integration test in `tests/routes.rs` (new, or extend `git_toolbar_branches_switch_and_create_revalidate_state`): hit `/git/status` twice inside the TTL, assert `diag/git` `totalSpawns` delta is 0 on the second call; `POST /git/switch-branch`; assert the next `/git/status` reflects the new branch and that the `diag/git` counter shows the epoch-driven recompute happened (non-zero delta), i.e. bump beats TTL.

## Verification Plan

- `cargo test -p ws-dashboard-daemon --lib` — new `git_state_cache` unit tests plus the full existing lib suite (142+ tests as of Phase 2's Result).
- `cargo test -p ws-dashboard-daemon --test routes` — full suite green; explicitly check by name: `git_toolbar_status_gates_and_reports_counts_without_paths`, `git_toolbar_branches_switch_and_create_revalidate_state`, `git_toolbar_status_adds_zero_spawns_after_resources_poll_warms_the_shared_discovery_memo`, plus the new zero-spawn-on-second-call and bump-beats-TTL tests from step 10.
- `cargo clippy -p ws-dashboard-daemon` — compare warning count against the Phase 1/2 baseline (both phases used a byte-identical clippy set as their own tripwire for "no unrelated behavior changed").
- Not covered / manual-only: the ticket's own Verification boundary marks cross-`serverRoute` behavior as covered by existing forwarding tests (no new test needed), and live Windows dogfood spawn-rate measurement is out of scope for this phase (carried forward from Phase 1/2's "unresolved and deferred" sections).

## Escalations

- None.

## Lead Dispositions

These override the sections above wherever they conflict. Verified by the lead
against source at `a23f986c`.

### D1 (binding): keep the union `RefState` — it is where the win comes from, not a cost to accept

The survey framed the union refs slot as a cold-cost increase to tolerate. That
reading is wrong, because it assumes the two routes are requested independently.
They are not: `refreshGit` (`frontend/src/App.tsx:6670-6674`) fetches
`/git/status` **and** `/git/branches` together in one `Promise.all`, on the 5 s
interval set at `frontend/src/gitToolbar.ts:237` (`intervalMs = 5000`), plus on
focus and visibility return. The union is exactly what the product asks for on
every tick.

So today each tick pays for both handlers computing their own refs state
independently — `branch --show-current` runs **twice**, the detached
`rev-parse --short HEAD` twice when detached, and the current-branch
`rev-list --left-right --count` twice when an upstream resolves. One shared refs
fill collapses those duplicates. The union's benefit is therefore independent of
the TTL and independent of the epoch: it applies on a completely cold tick.

Do not build a narrower per-route refs slot. Beyond losing the de-duplication, it
would break the "second call adds zero spawns" guarantee for whichever route did
not fill it.

### D2 (binding, and the reason D1 works): single-flight is load-bearing — pin it with a test

Because the two requests are issued concurrently by one `Promise.all`, they will
both miss a cold slot at the same moment. What makes them collapse into one fill
is the per-key lock: the second request blocks, then finds the value the first
one stored. If the implementation instead lets both fill, every functional
assertion in this phase still passes while the headline reduction silently does
not happen.

Required test: issue `/git/status` and `/git/branches` **concurrently** against a
cold cache for the same root, and assert the refs computation ran once — as a
delta against a `GET /api/dashboard/diag/git` baseline, never an absolute. This is
the one test that distinguishes "the cache works" from "the cache pays off".

### D3 (binding): do not name it `StaticZero`

The survey is right that the ticket's name invites a literally-constant source,
which would make the ticket's own Phase 3 acceptance item ("switch-branch bump
beats TTL") unimplementable. Fix the trap rather than documenting around it: name
the type `MutationEpochSource` — real per-key counters, whose only writers in this
phase are the mutating routes; Phase 4 adds the FS-event writer to the same store
without touching call sites. Record the ticket's `StaticZero` label in its doc
comment so the mapping stays traceable.

### D4 (confirmed): `Arc<dyn EpochSource>` on `AppState`

Endorsed as surveyed, for both stated reasons: the codebase already has two
working precedents (`CodexWorkRootResolver`, `ClaudeWorkRootResolver` behind
`Arc<dyn ...>`), and axum's `Router<S>` needs one concrete state type, so a
generic `AppState<E>` would infect `build_router` and every extractor.

### D5 (confirmed, with one added constraint): `GitStateCache` is its own struct

Not `ProbeSlots<GitCacheSlot>`, for the two reasons surveyed — the validity check
is compound (`stored_epoch == current_epoch && age < ttl`, which
`get_or_probe` has no parameter for), and the slot has two independently
revalidated parts, which a single generic `T` would force to invalidate together.

Added constraint: it must mirror `ProbeSlots`'s locking discipline exactly — the
map lock released **before** the per-key lock is acquired, so the map is never
held across a `git` spawn — and its comment must say so with a pointer to
`ProbeSlots`. Under D2 that discipline is a correctness requirement of this phase,
not a stylistic echo.

### D6 (binding): keep the TTL default at 2000, and do not claim a TTL-driven win

At a 5 s poll and a 2 s TTL every steady-state tick still misses, by design: with
no watcher, a short ceiling is what keeps stale git state from being served. So
this phase's spawn reduction is **intra-tick de-duplication (D1) plus burst
coalescing** — not TTL hits. The TTL-driven win arrives only with Phase 4's armed
120 s ceiling. State this plainly in the Phase 2-style `### Result` rather than
reporting a cache hit rate that the default configuration cannot produce; two
phases of this ticket have already had to correct an overstated win.

### D7 (confirmed): epoch sampled before the fill

Sample both epochs once, before either fill closure runs, and stamp each slot with
that pre-fill sample. Pin it with the deterministic race the survey describes: a
probe closure that bumps the epoch mid-execution, then assert the slot carries the
pre-probe epoch and the next read misses.

### D8 (confirmed): one normalization helper, two key types

`WatchKey` stays a type distinct from `GitProbeKey` (and from `WorkRootId`, whose
derivation is frozen), but the `canonical_or_normalized` → `\`→`/` →
lowercase-on-Windows chain is factored into one private helper both key types
call. Two hand-maintained copies of that chain is the drift `GitProbeKey`'s own
doc comment warns about.

### Out of scope, stated so a reviewer does not read it as an omission

`branches_for_path`'s `checked_out_branches` runs `worktree list --porcelain`,
which `GitProbeCache::worktree_paths` also runs under a different key and a
different TTL. Collapsing those two is a real further saving and is **not** part of
this phase; it needs the two caches' invalidation axes reconciled, which Phase 4's
reconcile hook is the natural place for.
