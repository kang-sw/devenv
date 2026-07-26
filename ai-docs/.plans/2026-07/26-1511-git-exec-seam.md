# Plan: 260726-refactor-ws-dashboard-git-fs-watch-invalidation — Phase 1: Git exec seam — bounded wait, kill on timeout, stderr logging, spawn counters

## Relevant Ticket Contract

- New `crates/daemon/src/git_exec.rs` owning the single git-spawn seam:
  `GitOutcome { status, stdout, stderr, elapsed }`, `GitFailure { Spawn(io::Error),
  Timeout, Status(i32) }`, `GitFailureExpectation { Unexpected, ExpectedNonZero }`,
  `GitSpawnStats { total: AtomicU64, timeouts: AtomicU64, failures: AtomicU64,
  by_subcommand: Mutex<BTreeMap<GitSubcommand, u64>> }`, and
  `pub fn capture(stats: &GitSpawnStats, root: &Path, args: &[&str], expect:
  GitFailureExpectation, budget: Duration) -> Result<GitOutcome, GitFailure>`.
- `GitSpawnStats` is owned explicitly (`Arc<GitSpawnStats>` in `AppState`, threaded
  as `&GitSpawnStats`), never a process-global static.
- `capture` must drain stdout/stderr concurrently with the bounded wait (reader
  thread per pipe to EOF; deadline wait; `child.kill()` on expiry; join readers).
  A test must pin success (not `Timeout`) for a child emitting >1 MB on stdout.
- Bounded wait + `child.kill()` on expiry, default 10 s, env
  `WS_DASHBOARD_GIT_TIMEOUT_MS`.
- Warn only on unexpected failure: `tracing::warn!` fires on `Spawn`/`Timeout`
  always, and on non-zero exit only when the call site passed
  `GitFailureExpectation::Unexpected`. Expected non-zero exits still increment
  counters, just don't log.
- `by_subcommand` keyed by an interned `GitSubcommand` enum (cannot be derived
  from a borrowed `args: &[&str]` directly — must be parsed out of it).
- Rewrite `git_text`/`run_git` (`git_toolbar.rs:566-587`), `GitDiscovery::probe`,
  `probe_git_worktree_paths` (`discovery.rs`), and `git_output`
  (`work_root_activity.rs:2379-2392`) as thin wrappers over `capture`.
- New owner-authed `GET /api/dashboard/diag/git` next to `dashboard_build_info`
  (`router.rs:114`/`:562`) returning
  `{ totalSpawns, timeouts, failures, bySubcommand, uptimeMs }`.
- **Scope boundary (from orientation): Phase 1 must change no observable git
  behavior other than adding the diag route and the timeout kill.** No route
  status/error-message/schema deltas in this phase.
- Verification boundary: unit tests for kill-on-timeout, >1MB-stdout survival,
  and `ExpectedNonZero` incrementing `failures` without logging; live 60s-apart
  diag reads on the dogfood daemon (not reproducible here). Not covered: no
  route-contract test changes, because no observable git behavior changes.

## Out of Scope

- Phases 2-5 (single-root resolution, result cache, the `notify` watcher, the
  retired Phase 5 heading) — explicitly deferred per orientation.
- Rewriting `git_worktree.rs`'s ~8 direct `Command::new("git")` call sites
  (lines 214, 421, 443, 457, 881, 894, 971, 979). The ticket's Phase 1 bullet
  names exactly four rewrite targets (`git_text`/`run_git`, `GitDiscovery::probe`,
  `probe_git_worktree_paths`, `git_output`) and does not list `git_worktree.rs`.
  Leaving it unrewritten means `totalSpawns`/`bySubcommand` will undercount real
  git invocations from worktree add/remove flows — worth surfacing to the lead,
  not a Phase 1 blocker since it changes no observable behavior either way.
- Any change to `resolve_git_context`, `git_identity`'s caching, or route
  status codes — those are Phase 2/3 concerns.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L566-L587` — confirmed current
  `git_text`/`run_git`: bare `.output()`, no timeout, stderr discarded. Matches
  ticket line numbers exactly.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L359-L544` — every other
  function in the file (`status_for_path`, `branches_for_path`, `sync_for_path`,
  `sync_for_branch`, `rev_counts`, `branch_exists`, `branch_checked_out_elsewhere`,
  `checked_out_branches`, `valid_branch_name`) is a thin caller of `git_text`/
  `run_git` taking only `root: &Path`. None currently accept a shared-handle
  parameter, so adding `&GitSpawnStats` to `git_text`/`run_git` ripples a `stats`
  parameter through **all nine** of these, up to the three route handlers where
  `state: AppState` is already in scope (`git_switch_branch`, `git_create_branch`,
  `mutate_no_body`, and the two GET handlers via `resolve_git_context`/
  `status_for_path`/`branches_for_path`). Purely mechanical, no design ambiguity,
  but a wider touch than the ticket bullet's single-line summary suggests.
- `ws-dashboard/crates/daemon/src/discovery.rs#L333-L400` and `#L603-L644` —
  established precedent for threading a second borrowed handle: `discover_work_root`
  and `discover_existing_dir` already take `git_probes: &GitProbeCache`;
  `GitProbeCache::discover`/`::worktree_paths` (private methods) call
  `GitDiscovery::probe(path)` (`:663`) and `probe_git_worktree_paths(path)`
  (`:720`) through closures passed to a `ProbeSlots::get_or_probe` memo. Thread
  `&GitSpawnStats` the same way: add the param to `discover_work_root`,
  `discover_existing_dir`, `GitProbeCache::discover`, `GitProbeCache::worktree_paths`,
  `GitDiscovery::probe`, `probe_git_worktree_paths`.
- `ws-dashboard/crates/daemon/src/discovery.rs#L42-L75` —
  `LocalDashboardResourcesProvider` holds `git_probes: GitProbeCache` as a field,
  set via a `with_git_probe_cache` builder (not passed per-call). Add an
  analogous `git_stats: GitSpawnStats`-holding field (an `Arc<GitSpawnStats>` or a
  cheap-clone wrapper) + `with_git_spawn_stats` builder, mirroring the existing
  pattern exactly, and use `&self.git_stats` at the two `discover_work_root` call
  sites (`:83`, `:102`).
- `ws-dashboard/crates/daemon/src/resources.rs#L26-L84` — `live_dashboard_resources`
  and `live_dashboard_resources_with_sync` (both `pub fn`, taking
  `git_probes: &GitProbeCache`) must gain a `git_stats: &GitSpawnStats` (or owned
  `Arc`) parameter, forwarded into `LocalDashboardResourcesProvider::with_git_spawn_stats`.
  Six call sites need the extra argument: `git_toolbar.rs:332`, `git_worktree.rs:280`,
  `:343`, `:603`, `:761`, and `resources.rs:32` (`local_dashboard_resources_view`,
  which already holds `state: &AppState` and can pass `&state.git_probe_cache`'s
  sibling `state.git_spawn_stats`).
- `ws-dashboard/crates/daemon/src/discovery.rs#L935-L1050` — the module's own
  `#[cfg(test)]` block calls `discover_work_root(&root, &probes)` at four spots
  (`:942`, `:947`, `:1038`, `:1039`); each needs a stats arg added (a fresh
  `GitSpawnStats::default()` per test is fine — tests don't assert on counters
  here).
- `ws-dashboard/crates/daemon/src/router.rs#L80-L103` — `AppState` struct;
  add `pub git_spawn_stats: Arc<GitSpawnStats>` next to `git_probe_cache`, with a
  doc comment following the existing style at `:84-86`.
- `ws-dashboard/crates/daemon/src/server.rs#L108-L125` — the one production
  `AppState { ... }` literal; add `git_spawn_stats: Arc::new(GitSpawnStats::default())`
  (or an env-seeded constructor if `capture`'s timeout budget is read once at
  boot rather than per-call — either is fine, `git_probe_cache` already reads its
  own env var lazily via `Default`, so mirroring that is simplest).
- `ws-dashboard/crates/daemon/tests/routes.rs` — **five** more `AppState { ... }`
  literals need the new field: `:168` (`app_state_with_opened_and_store`), `:189`
  (`app_state_with_static_dir`), `:420` (an inline `expired_state` literal), `:8220`
  (`app_state_with_activity_cache_home`/`_and_codex_home`), `:13984`
  (`app_state_with_translation_provider`). Missing any one of these is a compile
  error, not a silent gap, so this is low-risk but must not be missed.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L2349-L2392` — confirmed
  `git_identity`/`git_output` at the ticket's cited shape. `git_output` is called
  only from `git_identity` (`:2358`, `:2359-2362`, two calls: `rev-parse
  --show-toplevel` and `rev-parse --path-format=absolute --git-common-dir`).
  `git_identity` is called only from `resolve_work_root_state_dir` (`:486-496`),
  which is called from `resolve_work_root_agents_dir` (`:482`, `pub fn`),
  `project_blocking` (`:498`, called at `:126`), `watch_snapshot_blocking`'s
  callee `activity_item_versions` (`:568`, called at `:553`), and
  `named_agent_transcript_blocking` (`:1340`, called at `:151`). The three
  top-level route handlers (`work_root_activity` `:187`,
  `work_root_activity_transcript` `:243`, `work_root_activity_events` `:274`) all
  already have `state: AppState` in scope before calling into
  `state.work_root_activity.{project_with_recent_limit,named_agent_transcript,
  watch_snapshot}`. Thread `&GitSpawnStats` as an explicit parameter through this
  entire chain (`git_output` → `git_identity` → `resolve_work_root_state_dir` →
  `project_blocking`/`activity_item_versions`/`named_agent_transcript_blocking`
  → the three `WorkRootActivityProjector` async methods), passed in from the
  three handlers via `&state.git_spawn_stats`. No new struct field needed on
  `WorkRootActivityProjector` itself — passing explicitly at the call boundary
  matches Code Standards #4 (explicit dependencies) and avoids widening that
  struct's `Default`/equality surface.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L472-L484` and
  **`ws-dashboard/crates/daemon/tests/routes.rs` (34+ call sites, e.g. `:3926`,
  `:8341`, `:8449`, `:8498-8500`, `:8609`, `:8746`, `:9432`, `:9671`, `:9767`,
  `:10178`...`:12121`)** — **risk signal.** `resolve_work_root_agents_dir(cache_home,
  root_path)` is `pub fn`, documented as "Exposed so daemon route tests can seed
  fixture cache trees at the same location the projector reads" — it is a
  test-fixture-seeding utility, not part of the request-serving hot path. If its
  signature grows a mandatory `&GitSpawnStats` parameter (needed transitively
  because it calls `resolve_work_root_state_dir` → `git_identity` →
  `git_output` → `capture`), every one of these 34+ test call sites breaks.
  **Recommended to keep this mechanical and low-blast-radius:** have
  `resolve_work_root_agents_dir` construct and pass its own throwaway
  `GitSpawnStats::default()` internally rather than taking a parameter. It is
  not part of the production spawn-counting surface (nothing user-facing reads
  its counters), so a discarded per-call instance is harmless and keeps this
  function's public signature — and all 34+ test call sites — unchanged. This
  is a wiring decision, not a policy change, and does not affect observable git
  behavior.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L365-L373` and `#L360-L361` —
  the two calls the ticket names as routine-failure examples:
  `rev-parse --abbrev-ref --symbolic-full-name @{upstream}` (fails for every
  branch with no upstream) and `rev-parse --short HEAD` (fails on unborn HEAD)
  must use `GitFailureExpectation::ExpectedNonZero`.
- **Additional routine-failure call sites inferred from the ticket's own
  rationale, not explicitly named** — worth pinning explicitly so Phase 1
  actually achieves its stated goal of not burying the real signal:
  - `git_toolbar.rs#L535-L544` (`branch_exists`'s `show-ref --verify --quiet
    refs/heads/{branch}`) — routinely non-zero for a branch name that does not
    exist; that's the check's entire purpose.
  - `git_toolbar.rs#L562-L563` (`valid_branch_name`'s `check-ref-format
    --branch`) — validates user-entered branch names; a bad name is an expected
    non-zero exit, not a daemon-side failure.
  - `work_root_activity.rs#L2358-L2362` (`git_identity`'s two `rev-parse` calls)
    — the Background section states this path runs every 200 ms per root while
    the Activity pane is open, over every opened root including plain
    directories. A plain-directory root makes `rev-parse --show-toplevel` fail
    routinely (that's how `git_identity` reports "not a repo"). Marking these
    `Unexpected` would reproduce exactly the warning-stream noise problem this
    phase's "warn only on unexpected failure" bullet exists to prevent.
  All other rewritten call sites (`switch`, `switch -c`, `fetch`, `push`,
  `pull --ff-only`, `worktree list --porcelain`, `diff-index`/`status` in
  `changes_for_path`, `for-each-ref`, `rev-list --left-right --count`,
  `GitDiscovery::probe`'s `rev-parse` batch, `probe_git_worktree_paths`'s
  `worktree list --porcelain`) keep `GitFailureExpectation::Unexpected` — a
  genuine failure there is exactly what today's silent-discard bug is hiding.
- `ws-dashboard/crates/daemon/src/router.rs#L108-L126` and `#L558-L577` —
  `dashboard_build_info` is the sibling handler pattern to copy: plain
  `async fn(State(state): State<AppState>) -> Response`, `axum::Json(serde_json::json!({...}))`,
  registered in the `protected` router (which is the daemon's owner-auth
  boundary — see the `dashboard_shutdown` comment at `:579-583` confirming
  "real deployments gate this behind owner auth" for routes nested there). No
  separate per-route auth annotation exists; nesting under `protected` at
  `router.rs:114`-adjacent is sufficient to satisfy "owner-authed".
- `ws-dashboard/crates/daemon/Cargo.toml#L27` — confirms `tracing-appender` is a
  direct dependency, and `crossbeam-channel` is already in the lockfile
  transitively through it (verified via `grep` in `Cargo.lock`), matching the
  ticket's dependency-count rebuttal for `notify` (relevant to Phase 4 only,
  confirmed here since it was cheap to check).
- No existing `wait-timeout`-style crate dependency; `capture`'s bounded
  wait/kill must be hand-rolled with `std::process::Child::try_wait` polling
  plus `std::thread::spawn` reader threads — exactly as the ticket already
  specifies, so no new dependency or design choice is needed here.
- `ws-dashboard/crates/daemon/src/discovery.rs#L646-L652` — `git_probe_ttl_from_env`
  is the established local pattern for a module-local env-var-driven `Duration`
  default (not centralized in `config.rs`). Mirror this shape for
  `WS_DASHBOARD_GIT_TIMEOUT_MS` inside the new `git_exec.rs`, not in `config.rs`.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L611-L797` (the four in-file
  tests, using `init_fixture_repo`/`run_git`/`git_text`) — these call `run_git`/
  `git_text` directly with the current two-arg shape; once those gain a
  `stats`/`GitSpawnStats` parameter, all in-file test calls (fixture setup calls
  like `run_git(&dir, &["init", "-q"])` at `:630-635`, `:714-715`, `:723`, plus
  whatever assertions the four tests make) need a `&GitSpawnStats::default()` (or
  a shared per-test instance) threaded in too.
- `ws-dashboard/crates/daemon/tests/routes.rs#L154-L209`,`#L8971` — test helpers
  `app_state()`/`app_state_without_owner_auth()`/`skip_without_git`/
  `init_git_repo`/`run_git` (test-local, distinct from `git_toolbar.rs`'s
  in-file test helpers of the same name) are the fixture primitives the new
  unit/integration tests for `capture` (kill-on-timeout, >1MB stdout,
  `ExpectedNonZero` non-logging) should reuse or sit alongside; `git_exec.rs`'s
  own tests are better as an in-module `#[cfg(test)]` block (per the `.rs` file
  being new) rather than added to `tests/routes.rs`, since they test the seam in
  isolation, not an HTTP route.

## Implementation Plan

1. Create `ws-dashboard/crates/daemon/src/git_exec.rs`:
   - `GitSubcommand` enum: interned variants for every subcommand token observed
     across the four rewrite targets (`branch`, `rev-parse`, `for-each-ref`,
     `rev-list`, `show-ref`, `switch`, `fetch`, `push`, `pull`,
     `check-ref-format`, `worktree`, `diff-index`, `status`) plus a catch-all
     `Other` variant (test fixtures use `init`/`config`/`add`/`commit`/`mv`,
     which don't need dedicated variants). A helper `GitSubcommand::from_args(args:
     &[&str])` must skip leading `--`-prefixed flags (e.g. `--no-optional-locks`
     precedes `diff-index`/`status` in `changes_for_path`) to find the real
     subcommand token, defaulting to `Other` if none is found.
   - `GitOutcome`, `GitFailure`, `GitFailureExpectation` per the ticket's given
     shapes.
   - `GitSpawnStats`: atomics + `Mutex<BTreeMap<GitSubcommand, u64>>`, plus a
     `started_at: Instant` field (set in `Default`/`new`) to compute `uptimeMs`
     for the diag route without touching `AppState` further. Expose small
     `record_*`/snapshot accessor methods rather than exposing the raw fields.
   - `capture(stats, root, args, expect, budget)`: spawn via an internal
     `capture_with_program(program: &str, stats, root, args, expect, budget)` so
     unit tests can inject a non-`git` long-running/large-output program
     (`capture` itself calls it with `"git"`). Pipe stdout/stderr, spawn one
     `std::thread::spawn` reader per pipe draining to `Vec<u8>`/String, poll
     `child.try_wait()` against a deadline computed from `budget`, `child.kill()`
     + join readers on expiry → `GitFailure::Timeout`. Map spawn errors to
     `GitFailure::Spawn`; non-zero exit (after successful wait) to
     `GitFailure::Status(status.code().unwrap_or(-1))`. Always increment `total`
     and `by_subcommand`; increment `timeouts` on `Timeout`, `failures` on
     `Spawn`/`Timeout`/`Status`. `tracing::warn!(subcommand, code, stderr =
     %truncate(stderr, 512), elapsed_ms)` on `Spawn`/`Timeout` unconditionally,
     and on `Status` only when `expect == Unexpected`.
   - `git_timeout_from_env()`: local env-var parser for
     `WS_DASHBOARD_GIT_TIMEOUT_MS` (default 10_000), mirroring
     `discovery.rs`'s `git_probe_ttl_from_env` shape.
2. `router.rs`: add `pub git_spawn_stats: Arc<crate::git_exec::GitSpawnStats>` to
   `AppState` (`:80-103`), doc-commented like `git_probe_cache`. Add
   `dashboard_diag_git` handler near `dashboard_build_info` (`:562-577`)
   returning `axum::Json(serde_json::json!({ "totalSpawns": ..., "timeouts":
   ..., "failures": ..., "bySubcommand": ..., "uptimeMs": ... }))`, and register
   `GET /api/dashboard/diag/git` in the `protected` router next to
   `/api/dashboard/build-info` (`:114`).
3. `server.rs#L108-L125`: add `git_spawn_stats: Arc::new(GitSpawnStats::default())`
   to the one production `AppState { ... }` literal.
4. `tests/routes.rs`: add the same field to all five `AppState { ... }` literals
   (`:168`, `:189`, `:420`, `:8220`, `:13984`).
5. `git_toolbar.rs`:
   - Rewrite `git_text`/`run_git` (`:566-587`) as thin wrappers over `capture`,
     each taking an added `stats: &GitSpawnStats` parameter and a
     `GitFailureExpectation` (either as a parameter, or via a second wrapper
     pair like `git_text_expect_nonzero`/`run_git_expect_nonzero` if that keeps
     call sites cleaner — implementer's call, no policy impact either way).
   - Thread `stats` through the nine internal callers listed in Codebase
     Findings, up to `git_switch_branch`, `git_create_branch`, `mutate_no_body`,
     and the two GET handlers (via `resolve_git_context`, `status_for_path`,
     `branches_for_path`), all of which already have `state: AppState` in
     scope — pass `&state.git_spawn_stats`.
   - Apply `GitFailureExpectation::ExpectedNonZero` at the four call sites
     identified above (`upstream` rev-parse, unborn-HEAD rev-parse,
     `branch_exists`'s show-ref, `valid_branch_name`'s check-ref-format);
     `Unexpected` everywhere else in this file.
   - Update the four in-file tests (`:611-797`) to pass a
     `&GitSpawnStats::default()` (or one shared per test) into every `run_git`/
     `git_text` call.
6. `discovery.rs`:
   - Add `stats: &GitSpawnStats` to `discover_work_root` (`:333`),
     `discover_existing_dir` (`:384`), `GitProbeCache::discover`/`::worktree_paths`
     (`:614`, `:622`), `GitDiscovery::probe` (`:663`), `probe_git_worktree_paths`
     (`:720`) — all `GitFailureExpectation::Unexpected` (these are probes whose
     failure already means "not a git root", handled by the `Option`/`Vec`
     return, not by expectation-driven logging suppression — a genuine spawn
     failure or timeout here is still worth a warning).
   - Add a `git_stats`-equivalent field + `with_git_spawn_stats` builder to
     `LocalDashboardResourcesProvider` (`:42-75`), used at the two
     `discover_work_root` call sites (`:83`, `:102`).
   - Update the four test call sites (`:942`, `:947`, `:1038`, `:1039`) with a
     fresh `GitSpawnStats::default()`.
7. `resources.rs`: add a `git_stats` parameter to `live_dashboard_resources`
   (`:55-60`) and `live_dashboard_resources_with_sync` (`:65-84`), forwarded into
   `.with_git_spawn_stats(...)`; update `local_dashboard_resources_view`
   (`:26-49`, already has `state: &AppState`) to pass `&state.git_spawn_stats`.
   Update the five external call sites: `git_toolbar.rs:332`, `git_worktree.rs:280`,
   `:343`, `:603`, `:761` — each already has `state: AppState`/`&state.git_probe_cache`
   in scope, so add the sibling `&state.git_spawn_stats` argument.
8. `work_root_activity.rs`:
   - Rewrite `git_output` (`:2379-2392`) as a thin wrapper over `capture`, adding
     a `stats: &GitSpawnStats` parameter and using
     `GitFailureExpectation::ExpectedNonZero` for both calls inside `git_identity`
     (per the risk-signal finding above — a non-git root is a routine, expected
     result here, not a failure worth logging).
   - Thread `stats` through `git_identity` (`:2357`), `resolve_work_root_state_dir`
     (`:486`), `project_blocking` (`:498`), `activity_item_versions` (`:568`),
     `named_agent_transcript_blocking` (`:1340`), and the three
     `WorkRootActivityProjector` async methods (`project`/`project_with_recent_limit`
     `:99-136`, `named_agent_transcript` `:138-164`, `watch_snapshot` `:166+`),
     passed in from the three route handlers (`:187`, `:243`, `:274`) via
     `&state.git_spawn_stats`.
   - Leave `resolve_work_root_agents_dir` (`:482-484`) with its current
     two-argument public signature; have it construct its own
     `GitSpawnStats::default()` internally before calling
     `resolve_work_root_state_dir`, per the risk-signal finding — this keeps all
     34+ `tests/routes.rs` call sites unchanged.
9. Leave `git_worktree.rs`'s direct `Command::new("git")` call sites untouched
   (out of scope per ticket's explicit Phase 1 rewrite-target list); only its
   five `live_dashboard_resources` call sites need the added `git_stats` argument
   from step 7.
10. Add unit tests in `git_exec.rs`'s own `#[cfg(test)]` module: kill-on-timeout
    using `capture_with_program` with an injected long-running non-`git` command
    and a short budget; a >1 MB stdout emitter surviving without `Timeout`; an
    `ExpectedNonZero` call incrementing `failures` without emitting a
    `tracing::warn!` (assert via a test subscriber or by checking the counter
    only, per whatever the test harness already supports elsewhere in this
    crate — check for an existing tracing-capture test helper before writing a
    new one).

## Verification Plan

- `cargo test -p ws-dashboard-daemon` (or the crate's actual package name —
  confirm via `Cargo.toml` `[package] name`) — full green, including the four
  existing `git_toolbar.rs` in-file tests, the `discovery.rs` probe-cache tests,
  and the new `git_exec.rs` unit tests.
- `cargo test -p ws-dashboard-daemon --test routes` — full green; this phase's
  own "Not covered" note says no route-contract test changes are expected
  because no observable git behavior changes other than the new diag route and
  the timeout kill, so any existing `routes.rs` assertion breaking is a signal
  something leaked into observable behavior.
- Add one `tests/routes.rs` assertion (or extend an existing git-toolbar test)
  hitting `GET /api/dashboard/diag/git` and asserting the JSON shape
  (`totalSpawns`, `timeouts`, `failures`, `bySubcommand`, `uptimeMs` present) and
  that `totalSpawns` increases after a `/git/status` call — this is the minimal
  route-level pin the ticket implies exists ("a route any test or dogfood check
  can read") even though it isn't spelled out as a numbered test in the
  ticket's verification boundary.
- Live verification (two `/api/dashboard/diag/git` reads 60 s apart on the
  Windows dogfood daemon to derive spawns/s) is explicitly out of scope for this
  sandbox — not applicable here, flagged per the ticket's own tiering.

## Escalations

- None. All findings above are mechanical threading, consistent with the
  established `GitProbeCache`-handle precedent already in the codebase, plus
  two low-risk, non-policy wiring choices (a discarded per-call
  `GitSpawnStats::default()` for the test-fixture-seeding
  `resolve_work_root_agents_dir`, and `ExpectedNonZero` on two call sites not
  explicitly named by the ticket text but directly implied by its own stated
  rationale). No contract facts conflict; no strategy or reuse judgment beyond
  what the ticket already settles.

## Lead Dispositions

Settled by the lead after verifying each signal against current source. These
override the surrounding plan text where they disagree.

### 1. Throwaway stats in `resolve_work_root_agents_dir` — APPROVED, with a required comment

Approved, but **not** for the reason the finding gives ("nothing user-facing
reads its counters" — that reason would also license hiding the hot path). The
actual reason, verified: `grep -rn resolve_work_root_agents_dir src/` returns
exactly one hit, its own definition at `work_root_activity.rs:482`. It has
**zero production callers**; every production path into `git_identity` goes
through `resolve_work_root_state_dir` (4 call sites: `:506`, `:521`, `:574`,
`:1357`), which step 8 threads real stats through. So the discarded instance is
confined to a test seam and Phase 3's "the second call added zero spawns"
assertion stays falsifiable.

That invariant is load-bearing and invisible at the call site, so it must be
written down: add a comment on the throwaway construction stating that
`resolve_work_root_agents_dir` has no production callers and that **adding one
would create an uncounted git-spawn path**, directing a future caller to
`resolve_work_root_state_dir` instead. Without that comment this is a latent
trap, not a wiring choice.

### 2. `git_worktree.rs`'s 8 direct spawns — CONFIRMED out of scope, but the counter's meaning must be stated

Out of Phase 1 scope: the ticket's rewrite-target list is a hard phase boundary,
and Phase 1's acceptance gate (two diag reads 60 s apart on an otherwise idle
daemon) is a *poll-path* rate that worktree add/remove flows do not contribute
to. Widening the rewrite would also pull `git worktree add`'s long-running,
legitimately slow invocations under a 10 s default budget, which is a behavior
change this phase is explicitly forbidden from making.

But the undercount must not be presented as a total. Required in this phase:
the `dashboard_diag_git` handler carries a comment stating the counters cover
git spawns routed through `git_exec::capture` — i.e. the poll and discovery
paths — and **not** `git_worktree.rs`'s 8 direct `Command::new("git")` sites.
Do not rename the JSON fields; the ticket's Spec Impact fixes them as
`totalSpawns`/`timeouts`/`failures`/`bySubcommand`/`uptimeMs`, and renaming is
an observable-contract change. The lead files the follow-up ticket for
converting `git_worktree.rs` at this phase's documentation gate.

### 3. `ExpectedNonZero` extensions — APPROVED, and extended once more

Approved as listed: `branch_exists`'s `show-ref --verify --quiet`,
`valid_branch_name`'s `check-ref-format --branch`, and both of `git_identity`'s
`rev-parse` calls. Accepted trade-off on the last one: a genuinely broken repo
will no longer warn on non-zero exit there. That is the ticket's own stated
choice, and `Spawn`/`Timeout` still warn unconditionally, so a hang or a missing
binary remains visible.

**Additional override — step 6 is wrong for one call site.** Step 6 assigns
`Unexpected` to all of `discovery.rs`, reasoning that "a genuine spawn failure
or timeout here is still worth a warning." That conflates the two failure
classes: `Spawn` and `Timeout` warn *unconditionally* regardless of
expectation, so `ExpectedNonZero` suppresses nothing but the non-zero-exit
warning. And `GitDiscovery::probe`'s `rev-parse` batch exits non-zero routinely
by design — it is the check that answers "is this root a git repo at all", and
the ticket's own Phase 2 acceptance criterion is stated for a **plain-directory
root**. Under `Unexpected` that produces a continuous warning stream at one
warning per root per probe-TTL expiry, which is precisely the noise this phase
exists to prevent.

Therefore: `GitDiscovery::probe` → `ExpectedNonZero`.
`probe_git_worktree_paths` stays `Unexpected` — it runs only after the root is
already known to be a repo, so a non-zero exit there is genuinely surprising.

### 4. Verification-plan addition is accepted

The route-level `GET /api/dashboard/diag/git` assertion the plan adds beyond the
ticket's numbered unit tests (shape present, `totalSpawns` increases after a
`/git/status` call) is in scope and wanted: it is the mechanism Phase 3 asserts
against, so it must be pinned by a test in the phase that introduces it.

Note for the assertion's own correctness: `tests/routes.rs` runs git-spawning
tests concurrently, but each test builds its own `AppState` with its own
`Arc<GitSpawnStats>`, so a per-state counter delta is not cross-test polluted.
Assert a *delta* against a baseline read, never an absolute value.
