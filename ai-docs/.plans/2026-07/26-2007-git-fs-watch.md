# Plan: 260726-refactor-ws-dashboard-git-fs-watch-invalidation — Phase 4: The `notify` watcher — real epochs on every platform

## Relevant Ticket Contract

- Phase 4 is the whole watcher: `classify` → `IgnoreSet::derive` → `plan_watch_set` →
  arming (Windows/macOS recursive vs Linux walk-and-cap) → event pipeline
  (debounced epoch bumps) → `reconcile` hook → wire the real `EpochSource` into
  `GitStateCache` → config knobs. Depends on Phase 3 (epoch consumer already
  exists, stubbed) and the "Already Landed" `DiscoveredWorkRoot` widening
  (`git_dir`/`common_dir`).
- `cfg`-gated surface must stay minimal: only the registration backend, the
  Linux inotify budget read, and mount-type resolution. `classify`,
  `IgnoreSet`, `RepoEpochs`, debounce, and `reconcile` stay `cfg`-free and
  unit-testable on Linux/WSL.
- Constraints are binding: never register an uncounted watch (Linux walk+cap,
  Windows/macOS structural count-of-1); read both
  `max_user_instances`/`max_user_watches`; foreign-mount allowlist gate before
  arming (Linux `/proc/self/mountinfo`, Windows `GetDriveType`, macOS
  `statfs.f_fstypename`); Linux-only process-wide + per-repo caps with
  deterministic arm order; `git_dir`/`common_dir` excluded from *registration*
  on Linux (not merely from `classify`); daemon must never fail to boot on
  watcher init failure; poll-path git stays lock-free
  (`--no-optional-locks`); registry side effects in `resources.rs` keep
  running unconditionally; Windows worktree-remove must disarm before running.
- Config: `WS_DASHBOARD_GIT_WATCH=off|auto|force` (default `auto` on every
  platform), `WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS` (default 100),
  `WS_DASHBOARD_GIT_WATCH_MAX_DIRS` (default 1024, Linux-only),
  `WS_DASHBOARD_GIT_ARMED_TTL_MS` (120 000), reuse
  `WS_DASHBOARD_GIT_CACHE_TTL_MS` for the degraded/unarmed value (2 000).
- Verification boundary is explicitly three-tiered (unit / integration / live-
  only-not-covered) in the ticket; the live tier is Windows-dogfood-only by
  the ticket's own text.
- Carry-forward #1 (step 7 Prerequisite): `GitProbeCache::evict` has a
  pre-existing key-derivation mismatch (`18037cc3`); ticket asks whether
  reconcile can proceed without fixing it.
- Carry-forward #2 (step 8): whether to re-key the refs axis by `common_dir`
  as part of wiring the real `EpochSource`, or defer it.
- Carry-forward #3: the diag-delta acceptance numbers (~20/s pane-open,
  ~0.67/s pane-closed) have never been measured, three phases running.

## Out of Scope

- Phase 5 heading — absorbed into Phase 4, nothing to implement there.
- SSE push for git state, automatic `git fetch`, `WorkRootId` derivation
  changes, gitoxide, retiring the Activity Console — all ticket Non-Goals.
- `git_worktree.rs`'s 8 direct `Command::new("git")` sites — tracked
  separately (`260726-refactor-dashboard-worktree-git-spawns-through-exec-seam`).
- Fixing `GitProbeCache::evict`'s key-derivation bug as a general-purpose
  correctness fix — see Codebase Findings; determined not load-bearing for
  Phase 4's own correctness, recommended as a follow-up idea ticket instead of
  an inline fix.
- Re-keying `GitStateCache`'s refs axis by `common_dir` — see Codebase
  Findings; determined to be a materially larger refactor than "wire the real
  `EpochSource`" implies, and Phase 4's own event-fanout rule (step 4) already
  closes most of the practical staleness gap. Recommended as a separate
  follow-up, not inline in this phase.
- `open_work_root` (`root_picker.rs:216-234`) building its own throwaway
  `LocalDashboardResourcesProvider` outside `live_dashboard_resources_with_sync`
  — it will not hit the reconcile hook on the immediate open response, only on
  the next 5s canonical poll. Same self-healing class as other bypasses this
  ticket already accepts; not a Phase 4 blocker.
- Windows/macOS execution of any kind. This sandbox is Linux/WSL only.

## Codebase Findings

- `crates/daemon/Cargo.toml` (workspace root `Cargo.toml:15-35`) — **`notify`
  is not a dependency anywhere** (direct or transitive; confirmed absent from
  `Cargo.lock`). Must be added:
  `notify = { version = "8", default-features = false }`. Confirms the
  ticket's Decisions claim that `crossbeam-channel` is already in
  `Cargo.lock` via `tracing-appender` (`Cargo.lock:1701-1712`, dependency of
  `tracing-appender 0.2.5`) — the "don't justify default-features=false by
  avoiding crossbeam" reasoning is verified correct.
- `crates/daemon/Cargo.toml:30-35` (windows-sys features) — only
  `Win32_Foundation`, `Win32_Security`, `Win32_System_JobObjects`,
  `Win32_System_Threading` are enabled. `GetDriveTypeW` (needed for the
  Windows mount-allowlist gate) lives under `Win32_Storage_FileSystem`, not
  yet enabled — add that feature. Cannot be verified by a Linux build; flag
  for the implementer.
- `crates/daemon/src/discovery.rs:358-365` (`DiscoveredWorkRoot`) — currently
  `{ path, workspace_key, kind, status, availability, error }`, no
  `git_dir`/`common_dir`. Confirms "Already Landed" note: the widening has not
  happened.
- `crates/daemon/src/discovery.rs:766-771,782-829` (`GitDiscovery`/`probe`) —
  the `rev-parse` batch already fetches `--git-dir` (parsed at line 817 into
  local `git_dir`) but the `GitDiscovery` struct only stores `common_dir` and
  `worktree_dir`; `git_dir` is used solely to compute `kind` (line 818) then
  discarded. Confirms the ticket's "already computes both and throws them
  away" claim precisely — `GitDiscovery` needs a `git_dir: PathBuf` field
  added, not just `DiscoveredWorkRoot`.
- `crates/daemon/src/discovery.rs:110-172` (`dashboard_resources_with_registry_sync`)
  and `199-203` (`DashboardResourcesSync`) — `DiscoveredWorkRoot` is
  crate-private and consumed only to build the public `WorkRootView` (which
  has no `git_dir`/`common_dir` fields and must not gain them — those are
  filesystem paths serialized to the frontend). The reconcile hook's required
  input (`&[(WatchKey, Option<WatchTargets>, WorkRootAvailability)]`) does not
  exist as an output of this function today for either the owner-candidate
  loop or the discovered-linked-worktree loop (lines 115-152). Needs a new
  side-channel field on `DashboardResourcesSync` (parallel to
  `discovered_registry_roots`/`pruned_work_root_ids`), populated in both
  loops, alongside the `WorkspaceBuilder::push` call.
- `crates/daemon/src/discovery.rs:416-420` (`discover_work_root`'s
  eviction) and `743-747` (`GitProbeCache::evict`) — **confirms carry-forward
  #1 at the source level.** `evict(path)` recomputes
  `GitProbeKey::for_path(path)`, which calls `canonical_or_normalized(path)`
  (line 887-890): tries `.canonicalize()`, falls back to
  `normalize_candidate_path` only on failure. Eviction is called exactly when
  `discovered.availability != Available` (missing/moved/inaccessible), i.e.
  precisely when `.canonicalize()` on that same path is *already* failing for
  the same underlying reason (the path is gone) — so the eviction key is
  structurally forced onto the non-canonical fallback form, while the warm
  entry (created while the directory existed) was keyed on the *canonical*
  form. On Linux with no symlinks in the path, canonicalize of an existing
  path is usually string-identical to the normalized form, so the bug is
  latent in this sandbox; on Windows canonicalize always adds a `\\?\` prefix,
  so the mismatch is real and the evict is a structural no-op there, exactly
  as the ticket states. **Determination:** not load-bearing for Phase 4.
  Reconcile's disarm/absent branches key off `WorkRootAvailability`, which
  comes from uncached `fs::metadata`/`read_dir` (line 374-378 CONTRACT
  comment) and is unaffected by this bug. Only the arm branch's
  `WatchTargets` (kind/git_dir/common_dir) can be stale, bounded by the
  existing 30s `GitProbeCache` TTL — the same staleness class Phase 2's
  Result already accepts explicitly ("a plain directory the user `git init`s
  reads as non-Git for up to 30s"). Recommend: do not fix inline; file a
  follow-up idea ticket for the key-derivation bug (matching how Phase 2/3
  filed their own discovered staleness gaps as separate tickets rather than
  fixing them in-phase).
- `crates/daemon/src/git_state_cache.rs:61-77,91-121,177-257` (`EpochSource`
  trait, `MutationEpochSource`, `GitStateCache`) — **confirms carry-forward #2
  is a bigger refactor than implied.** `GitStateCache` is one
  `HashMap<WatchKey, Arc<Mutex<GitCacheSlot>>>` where `GitCacheSlot` holds
  *both* `worktree` and `refs` parts under the same key; `EpochSource::bump_refs`
  takes the same `WatchKey` type as `bump_worktree`. Re-keying only the refs
  axis by `common_dir` requires: splitting the single map into two
  independently-keyed stores (worktree-path-keyed vs common-dir-keyed),
  changing `bump_refs`'s key type (or adding a second key type/trait method),
  and threading `common_dir` into every git_toolbar.rs call site that
  currently only has `root: &Path` — `status_for_path`/`branches_for_path`
  (`git_toolbar.rs:447-498`) and the four mutating routes'
  `watch_key(&context.root_path)` calls (`git_toolbar.rs:241-243,313-315,396-407`).
  `GitContext` (`git_toolbar.rs:429-445`, `resolve_git_context`) carries only
  `root_path` today — no common_dir. **Determination:** defer, do not do
  inline. Phase 4's own step-4 dedup+fanout rule ("register once per distinct
  target, fan events out to every owning repo") already closes most of the
  practical gap Phase 3's carry-forward was written against: any real
  branch/fetch/switch operation writes `refs/heads`/`packed-refs` etc., which
  the watcher observes and fans out to every sibling worktree sharing that
  `common_dir` via the same reverse index used for FS events — so the residual
  staleness for a mutating-route bump not yet fanned out is bounded by the
  debounce window (default 100ms, capped 500ms), not "up to the TTL" as
  Phase 3's note (written before Phase 4's fanout existed) assumed. Recommend
  filing a follow-up idea ticket if the owner still wants formal re-keying.
- `crates/daemon/src/router.rs:82-122` (`AppState`), `crates/daemon/src/server.rs:108-127`
  (construction) — `epoch_source: Arc::new(MutationEpochSource::default())`
  is the exact wiring point step 8 must change. The doc comments on
  `MutationEpochSource` (`git_state_cache.rs:79-90`) and `EpochSource`
  (`git_state_cache.rs:53-60`) already state the intended shape: "Phase 4
  adds the FS-event pipeline as a second writer to this same store... without
  touching any `git_toolbar.rs` call site." Concretely: construct one
  `Arc<MutationEpochSource>` in `server.rs`, hand one clone to `AppState.epoch_source`
  and another to the new watcher module's constructor so its event-processing
  task calls the existing `bump_worktree`/`bump_refs` methods directly — no
  new `EpochSource` implementation needed.
- `crates/daemon/src/router.rs:610-625` (`dashboard_diag_git`) — extension
  point for step 9's `{ repos: [...] }` field confirmed; sits next to
  `dashboard_build_info` as the ticket states.
- `crates/daemon/src/resources.rs:59-91` — `live_dashboard_resources_with_sync`
  is the confirmed reconcile call site (called after `sync_discovered_roots`
  at line 89). Its signature (`opened, git_probes, git_stats`) needs a fourth
  parameter for the watch registry handle, threaded from
  `local_dashboard_resources_view` (`resources.rs:29-53`, has `&AppState`).
  **Enumerated all callers of `live_dashboard_resources`/`_with_sync`** (the
  same class of gap Phase 1's Result flagged for `open_work_root` and spawn
  stats): `git_worktree.rs:285,348,612,770` (4 sites, via the `.0`-only
  `live_dashboard_resources` wrapper) plus `resources.rs:36` (canonical
  route). All 5 go through `live_dashboard_resources_with_sync` internally,
  so patching that one function's signature covers all of them without a
  repeat of the Phase 1 miss — `root_picker.rs`'s `open_work_root` is the one
  path that bypasses it entirely (see Out of Scope).
- `crates/daemon/src/git_worktree.rs:514-558` (`git_worktree_remove_submit`)
  — confirmed insertion point for the Constraints "disarm before running,
  re-arm via next reconcile" rule: disarm must happen before line 541-548
  (`command.output()` running `git worktree remove`), using
  `watch_key(&context.target_path)`. The existing `state.git_probe_cache.clear()`
  / `state.git_state_cache.clear()` calls at lines 554/558 run *after* the
  command, so the disarm call is a new, earlier insertion, not adjacent to
  those.
- `crates/daemon/src/git_exec.rs:118-136` (`GitSubcommand`) — `Status` variant
  already exists (used by `changes_for_path`'s
  `status --porcelain … --no-optional-locks`), and `from_args` matches on the
  first non-`--`-prefixed token. `IgnoreSet::derive`'s
  `git status --porcelain=v1 -unormal --ignored=matching -z` will
  automatically classify as `GitSubcommand::Status` — no new enum variant
  needed.
- `crates/daemon/src/terminal.rs:452` (`output_signal: watch::Sender<u64>`) —
  confirmed as the in-repo monotonic-counter pattern the ticket cites; no
  further reuse beyond the pattern itself (epochs here are per-key in a
  `HashMap`, not a single `watch::Sender`).
- `crates/daemon/src/work_root_files.rs:798-816`
  (`resolve_online_available_work_root`) — confirmed zero-git-spawn
  availability gate already reused by `resolve_git_context`
  (`git_toolbar.rs:433`); nothing new needed here for Phase 4.
- `crates/daemon/tests/routes.rs:154` (`app_state()`), `:9678,9692,9701,13010`
  (`run_git`/`init_git_repo`/`skip_without_git`/`git_toolbar_get_json`) and
  `git_toolbar.rs:791,839` (`run_git`/`init_fixture_repo`) — all Prior Art
  fixture helpers confirmed present with the cited signatures; integration
  tests can reuse them directly.
- `crates/core/src/resources.rs:21-27,40-44` — `WorkRootAvailability`
  (`Available/Missing/Moved/Inaccessible/Unknown`) and `WorkRootKind`
  (`PlainDirectory/GitPrimaryRoot/GitLinkedWorktree`) confirmed as stated.

## Implementation Plan

Follow the ticket's own step ordering 1-9 (`classify` → `IgnoreSet::derive` →
`plan_watch_set` → arming → event pipeline → Linux incremental registration →
reconcile → wire `EpochSource` → config), which is already fully specified at
the algorithm level in the ticket text — this plan adds only the codebase
integration points that are not already explicit there.

1. `Cargo.toml`: add `notify = { version = "8", default-features = false }`
   to `crates/daemon/Cargo.toml`; add `Win32_Storage_FileSystem` to the
   `windows-sys` feature list (needed for `GetDriveTypeW`, unverifiable on
   this Linux sandbox — flag for Windows-side follow-up).
2. New `crates/daemon/src/work_root_watch.rs`: implement steps 1-6 from the
   ticket (`classify`, `IgnoreSet::derive`, `plan_watch_set`, arming,
   event pipeline with leading+trailing debounce, Linux incremental
   registration) exactly as specified there — this is the ticket's own
   correctness core and needs no codebase-integration decisions beyond what
   is already in the ticket text.
3. Widen `GitDiscovery` (`discovery.rs:766-771`) with a `git_dir: PathBuf`
   field, populated from the already-parsed local at line 817 (currently
   discarded). Widen `DiscoveredWorkRoot` (`discovery.rs:358-365`) with
   `git_dir`/`common_dir: Option<PathBuf>` (`None` for non-git roots),
   populated in `discover_existing_dir` (`discovery.rs:442-472`) from the
   `GitDiscovery` match arm.
4. Extend `DashboardResourcesSync` (`discovery.rs:199-203`) with a new field
   carrying reconcile input, e.g.
   `watch_targets: Vec<(WatchKey, Option<WatchTargets>, WorkRootAvailability)>`,
   populated in both discovery loops of `dashboard_resources_with_registry_sync`
   (`discovery.rs:115-152`, the owner-candidate loop and the linked-worktree
   loop) alongside the existing `WorkspaceBuilder::push` calls, using
   `discovery::watch_key(&discovered.path)` for the key. Do **not** add
   `git_dir`/`common_dir` to the public `WorkRootView` — this data stays
   internal to the daemon crate.
5. Implement step 7 (`reconcile`) per the ticket's semantics table and the
   two rate-limit rules verbatim. Call it from
   `live_dashboard_resources_with_sync` (`resources.rs:70-91`) right after
   `opened.sync_discovered_roots(...)` (line 89), passing the new
   `watch_targets` vec. Thread a watch-registry handle as a new parameter
   through `live_dashboard_resources_with_sync`/`live_dashboard_resources`
   and from `local_dashboard_resources_view` (which already holds
   `&AppState`); update all 5 confirmed call sites
   (`resources.rs:36`, `git_worktree.rs:285,348,612,770`).
   Do not fix `GitProbeCache::evict`'s key-derivation bug as part of this
   step (see Codebase Findings); file a follow-up idea ticket instead if the
   owner wants it addressed.
6. Insert the Windows disarm-before-remove call in
   `git_worktree_remove_submit` (`git_worktree.rs:514-558`), before the
   `command.output()` call at line 543, keyed by
   `watch_key(&context.target_path)`. Re-arm happens naturally via the next
   reconcile tick (no explicit re-arm call needed here).
7. Wire the real `EpochSource` (step 8): in `server.rs`, construct one
   `Arc<MutationEpochSource>`, pass one clone to `AppState.epoch_source`
   (replacing the current bare construction at `server.rs:115`) and another
   into the new watcher module's constructor so its event task calls
   `bump_worktree`/`bump_refs` directly on the shared instance — no new
   `EpochSource` implementation required. Select TTL by `WatchHealth` at the
   `git_toolbar.rs` call sites (`status_for_path`/`branches_for_path`,
   `git_toolbar.rs:447-498`) using `WS_DASHBOARD_GIT_ARMED_TTL_MS` (120s) vs
   the existing `git_cache_ttl_from_env()` (2s) for degraded/unarmed. Do not
   re-key the refs axis by `common_dir` (see Codebase Findings); leave it
   per-`WatchKey`/per-worktree as today.
8. Config knobs (step 9): `WS_DASHBOARD_GIT_WATCH`,
   `WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS`, `WS_DASHBOARD_GIT_WATCH_MAX_DIRS`,
   `WS_DASHBOARD_GIT_ARMED_TTL_MS` — follow the `OnceLock`-read-once pattern
   already established by `git_exec::git_timeout_from_env` and
   `git_state_cache::git_cache_ttl_from_env`.
9. Extend `dashboard_diag_git` (`router.rs:610-625`) with the
   `{ repos: [{ health, worktreeEpoch, refsEpoch, lastEventMs,
   registeredWatches }] }` field, sourced from the watch registry handle
   added to `AppState`.
10. Update `ai-docs/spec/ws-web-dashboard/index.md`
    `## Git-Aware WorkRoot Toolbar {#260524-ws-dashboard-git-aware-workroot-toolbar}`
    and the `ws-web-dashboard` mental model's poll-path git rule, per the
    ticket's Spec Impact section, after implementation lands.

## Verification Plan

- *Unit* (fully runnable on this Linux/WSL sandbox — `classify`,
  `IgnoreSet`, debounce, `plan_watch_set`, and `reconcile`'s decision table
  are specified `cfg`-free): the ~25-case `classify` table, debounce
  leading+trailing shape, `IgnoreSet` `-unormal` argv pin, `watch_key`
  normalization, `plan_watch_set` fixture-tree pin, reconcile's
  `Degraded`-does-not-re-arm and arm-bumps-both-epochs cases — all exactly as
  specified in the ticket's Verification boundary.
- *Integration* (`crates/daemon/tests/git_watch.rs`, runnable here against
  real temp repos): arm/write/poll-epoch flows, burst containment against
  Phase 1's spawn counter, `.gitignore` 50x re-derive-once pin, availability
  flap rate-limiting, shared-`common_dir` dedup+fanout, availability
  lifecycle (rename away/back). These run "armed on every platform including
  the Linux/WSL dev host" per the ticket's own text — the Linux registration
  path (walk-and-cap) is exercised natively here. The `#[cfg(target_os =
  "linux")]`-gated cases (`WS_DASHBOARD_GIT_WATCH_MAX_DIRS=1` degrade,
  incremental registration, create+write race) run here too.
- *Windows/macOS-specific claims — explicitly not executable in this
  sandbox.* This environment is Linux/WSL only. The following can only be
  argued from source reading and `notify` crate documentation, never run
  here: the Windows `#[cfg(windows)]` registration path itself (the three
  `RecursiveMode::Recursive` calls), `GetDriveTypeW` mount resolution,
  `worktree-remove-while-armed` sharing-violation avoidance, macOS FSEvents
  registration and `statfs.f_fstypename` resolution. State this explicitly
  in any implementation/review report rather than claiming cross-platform
  coverage.
- *Live-only, stated as not covered rather than deferred silently*: sustained
  spawns/s and CPU% on a real Windows dogfood daemon, RSS/handle-count delta
  across 9 roots, `/proc/*/fd` inotify descriptor accounting vs
  `registeredWatches`, watcher-task CPU during a real `cargo build`/`git gc`/
  `git fetch`, and the `frontend/e2e/` toolbar-freshness assertion — none of
  these are executable from this sandbox and must be reported as such, not
  implied as tested.
- **Diag-delta acceptance (~20/s pane-open, ~0.67/s pane-closed), unmeasured
  for three phases running.** This sandbox cannot reproduce the literal
  Windows-dogfood figure (topology/root-count-specific), but nothing prevents
  attempting the *same* measurement locally: run `ws-dashboard-daemon`
  against a handful of this workspace's real repos (`devenv`, this worktree,
  at least one plain-directory root to exercise the `None`-memoized
  `git_identity` case), open the Activity Console SSE endpoint as a
  subscriber, and diff two `/api/dashboard/diag/git` reads 60s apart with the
  pane subscribed vs. not. Take this measurement as part of Phase 4 rather
  than deferring a fourth time — report the actually-measured local numbers
  explicitly labeled as "this host, not Windows dogfood," rather than
  restating the unmeasured Windows figures as if verified.
- `cargo test -p ws-dashboard-daemon --lib` and `--test routes` full green,
  clippy warning count unchanged from baseline (`22` per Phase 3's Result),
  named pins executed and reported by name per the ticket's own convention.

## Escalations

- None.

## Lead Dispositions

Authority order for implementation: this section > the rest of this plan >
the ticket. I independently re-verified the survey's three most consequential
claims against source before accepting anything below: `notify` is absent
from `Cargo.lock` (grepped directly); `GitDiscovery::probe` parses `git_dir`
at `discovery.rs:817` only to compute `kind` at `:818-822` and the struct at
`:766-771` has no field to keep it (read directly); `GitProbeCache::evict`
(`:742-746`) recomputes `GitProbeKey::for_path(path)`, the same
canonicalize-or-normalize call the warm entry used, called from
`discover_existing_dir` (`:415-419`) exactly when availability just went
non-`Available`, i.e. exactly when canonicalize on that path is failing (read
directly). All three check out as stated.

**D1 — Carry-forward #1 (`GitProbeCache::evict` prerequisite): defer, but
require a test that cannot pass under the bug.** The survey's determination
is correct and I traced the failure mode myself to be sure: while a root is
missing, `canonical_or_normalized` is deterministic on the raw (non-canonical)
path string regardless of whether the target exists, so every lookup during
the outage computes the same non-canonical key — the stale canonical-keyed
warm entry from before the outage is simply orphaned, never read, so
`WorkRootAvailability` (computed uncached, unaffected by this bug) is
reported correctly throughout. The one real gap is on **reappearance**: if
the root comes back before the pre-outage warm entry's 30s TTL expires *and*
something changed during the outage (kind flip: a plain directory now sits
where a git repo was, or vice versa — not merely "still the same repo"), the
stale entry is served for up to the remainder of that TTL. This is the same
staleness class Phase 2's Result already accepted explicitly. Do not fix the
key-derivation bug in this phase; file the follow-up idea ticket as the
survey recommends. **But:** the Verification Plan's "availability lifecycle
(rename away/back)" integration test must specifically exercise a **kind
change across the outage** (git repo → plain directory, or the reverse), not
merely an unchanged-repo reappear — an unchanged-repo case passes identically
whether or not the evict bug is present, so it would not actually pin the
bounded-staleness claim. Assert the daemon reports the wrong (pre-outage)
kind immediately on reappearance and the correct kind once the 30s TTL
elapses — that is the shape of the bound this disposition is accepting, and
it must be pinned, not just claimed.

**D2 — Carry-forward #2 (re-key refs axis by `common_dir`): defer, and I
verified the closing argument myself.** A mutating route's own `git switch`/
`git branch` write lands under the shared `common_dir` (`refs/heads/*` or
`packed-refs`), which `plan_watch_set`'s git-internal targets
(`refs/**`, `worktrees/**`) register on Linux, and which a recursive
Windows/macOS registration covers unconditionally. Step 4's dedup-by-target
plus reverse-index fanout means that write's FS event reaches every repo
sharing that `common_dir`, bounded by the debounce window (100ms default,
500ms cap) rather than the 2s/120s cache TTL. So once Phase 4 is live, the
sibling-worktree staleness Phase 3's carry-forward described is closed for
real traffic, not merely bounded better. Do not re-key inline. **But:** the
Verification Plan's "shared-`common_dir` dedup+fanout" integration test must
specifically drive the mutation through the **`git_toolbar.rs` mutating
route** (`POST /git/switch-branch` or `/create-branch`), not a raw `git`
CLI write in the test fixture — the whole point being verified is that a
route-driven mutation in worktree A becomes visible in worktree B's
`/git/branches` response within the debounce window, and a fixture that
writes directly to `refs/heads` bypasses the very code path (the mutating
route's *own* `bump_refs` call, which only bumps A's `WatchKey`) that made
this a carry-forward in the first place.

**D3 — Diag-delta acceptance measurement: required, not merely
recommended.** Promoting the survey's recommendation to a hard requirement:
this is the fourth phase in this ticket's Result trail; `_index.md`'s carry
forward from Phase 3 says plainly "Phase 4 should take it." Run the daemon
against a handful of real repos in this workspace with the Activity Console
SSE endpoint subscribed and diff two `/api/dashboard/diag/git` reads 60s
apart, both pane-open and pane-closed. Report the actually-measured local
numbers explicitly labeled "this host, not Windows dogfood" — do not restate
the unmeasured Windows figures (~20/s, ~0.67/s) as if they were now verified.
If this genuinely cannot be done (say why, with evidence, not silently), that
is the only acceptable way to defer it a fourth time.

**D4 — Attempt cross-target `cargo check` for the platform-gated code.** A
Linux `cargo check`/`cargo build` does not compile `#[cfg(windows)]` or
`#[cfg(target_os = "macos")]` items at all, so a syntax or type error in
either platform arm is invisible to every check this sandbox can run.
Before reporting, attempt `rustup target add x86_64-pc-windows-gnu` and
`rustup target add x86_64-apple-darwin` (both are pure `std`-artifact
installs, no linker/SDK required for `cargo check`, which does not link),
then `cargo check --target <target> -p ws-dashboard-daemon` for each. Report
which of the two attempts succeeded, and for any that did, whether it caught
anything the native Linux check did not. If a target add or check fails for
environment reasons (network, missing component), report that plainly rather
than silently skipping — this is the only mechanical verification available
for the Windows/macOS arms in this sandbox, weak as it is.

**D5 — `cfg`-gating boundary is a hard review-check item, not a style
preference.** Only the registration backend (recursive vs. walk-and-register),
the process-wide inotify budget read, and mount-type resolution may carry a
`#[cfg(...)]`. `classify`, `IgnoreSet` derivation/parsing, `RepoEpochs`,
debounce/coalescing, and the `reconcile` decision table must compile and run
unit tests identically on Linux with no `#[cfg]` at all — this is where the
ticket says the correctness risk lives, and it is what makes this phase's
core actually testable in a Linux-only sandbox. Fit review must check this
list exhaustively, not spot-check it.

**D6 — Incremental commits following the ticket's own step order, each
tested before the next.** This is the largest diff of the four phases by a
wide margin (an entire new subsystem plus five integration points). The
ticket's own text says to implement in step order "because the first step is
the correctness core and is fully testable without any I/O" — treat that as
a commit-boundary instruction, not merely an exposition order. Expected
checkpoints, each green before starting the next: (1) `classify` + its
~25-case table, (2) `IgnoreSet::derive` + `plan_watch_set` + their fixture
tests, (3) arming (platform-split) + the event pipeline with debounce,
(4) Linux incremental registration, (5) `DiscoveredWorkRoot`/`GitDiscovery`
widening + `DashboardResourcesSync` extension, (6) `reconcile` + its two
rate-limit rules, (7) wiring the real `EpochSource` + TTL selection,
(8) config knobs + diag extension. If a later checkpoint reveals an earlier
one needs rework, that is expected and fine — report it, do not silently
patch around it.

**D7 — New visibility surface follows the Phase 3 precedent.** Any new type
that `AppState` needs as a `pub` field (the watch registry handle) is `pub`
for that reason alone, matching `GitStateCache`/`MutationEpochSource`'s
precedent from Phase 3 — do not re-litigate the `pub` vs `pub(crate)`
question from scratch. Everything else (`classify`, `IgnoreSet`,
`plan_watch_set`, `reconcile`, `WatchTargets` internals) stays `pub(crate)`
or private unless a concrete external-crate construction site (a `tests/`
file building a literal) forces otherwise.

**D8 — `reconcile`'s two rate-limit rules each need a dedicated
failure-mode unit test.** "Arm attempts rate-limited regardless of why the
repo is unarmed" has two independent guards per the ticket
(`Degraded`-is-sticky-with-backoff, and a flat 30s minimum between arm
attempts) — the ticket itself explains why the second is not redundant with
the first (`not Available ⇒ disarm` yields `Unarmed`, which is arm-eligible,
so a flapping root would re-arm every reconcile tick with the backoff never
consulted if the 30s gate were absent). Each guard needs its own test that
fails if that specific guard is removed — not one combined happy-path arm
test that both guards would trivially pass. This is the same class of gap
Phase 3's review found three times (D2 single-flight, D7 epoch-sample-order,
the `current_branch_counts` reuse) — a test whose assertion also holds under
the broken behavior it claims to pin.

**D9 — `open_work_root`'s reconcile bypass: accept as out of scope, but name
it in the Result, not just this plan.** Consistent with every other
self-healing bypass this ticket has already accepted (Phase 2's 30s
`GitProbeCache` TTL bound, D1's kind-flip-on-reappear bound above). The
Phase 4 `### Result` I write must state this bypass explicitly rather than
letting it live only in this plan's Out of Scope section.

**D10 — Plan step 4's `DashboardResourcesSync` extension: confirmed
complete via the survey's own enumeration.** All 5 call sites of
`live_dashboard_resources`/`_with_sync` were enumerated
(`resources.rs:36`, `git_worktree.rs:285,348,612,770`) and all funnel through
the one function being patched, avoiding a repeat of Phase 1's uncounted-
call-site miss. No further action needed here beyond what the plan already
states.

Nothing in this plan is escalated to research; the ticket specifies the
algorithm level in full and my dispositions above are refinements/guardrails,
not open design questions.
