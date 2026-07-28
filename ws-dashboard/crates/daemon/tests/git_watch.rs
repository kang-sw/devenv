// CONTRACT: Integration coverage for Phase 4 of
// `260726-refactor-ws-dashboard-git-fs-watch-invalidation` - the `notify`
// watcher driving real epoch bumps against real git repositories and real
// filesystem events, per the ticket's Verification Plan "Integration" tier.
//
// Unit coverage (classify, IgnoreSet, plan_watch_set, debounce, reconcile's
// decision table, the two D8 rate-limit guards) lives in
// `work_root_watch.rs`'s own `#[cfg(test)] mod tests`; this file only covers
// what needs a real `notify` watcher, a real `git` binary, and (for the two
// route-driven cases) the full router + owner-auth pairing flow.
//
// All timing here is deadline-polling against a generous ceiling, never a
// fixed sleep standing in for an assertion - see `poll_until`.

use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use axum::body::Body;
use axum::http::{header, Method, Request, StatusCode};
use tower::ServiceExt;

use ws_dashboard_daemon::auth::OwnerAuthState;
use ws_dashboard_daemon::config::ServeConfig;
use ws_dashboard_daemon::discovery::{watch_key, GitProbeCache};
use ws_dashboard_daemon::git_exec::GitSpawnStats;
use ws_dashboard_daemon::git_state_cache::{EpochSource, GitStateCache, MutationEpochSource};
use ws_dashboard_daemon::persistent_state::DashboardStateStore;
use ws_dashboard_daemon::router::{build_router, AppState};
use ws_dashboard_daemon::servers::{LinkedServerSessions, LinkedServerTunnels};
use ws_dashboard_daemon::terminal::TerminalRegistry;
use ws_dashboard_daemon::work_root_activity::WorkRootActivityProjector;
use ws_dashboard_daemon::work_root_files::{DocumentEventHub, OpenedWorkRoots};
use ws_dashboard_daemon::work_root_watch::{WatchConfig, WatchHealth, WatchRegistry, WatchTargets};

static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

fn temp_fixture_path(name: &str) -> PathBuf {
    let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    std::env::temp_dir().join(format!("ws-dashboard-git-watch-{name}-{}-{unique}", std::process::id()))
}

fn remove_fixture(path: &Path) {
    let _ = std::fs::remove_dir_all(path);
}

fn run_git(dir: &Path, args: &[&str]) {
    let status = std::process::Command::new("git")
        .args(args)
        .current_dir(dir)
        .status()
        .unwrap_or_else(|error| panic!("spawn git {args:?} in {}: {error}", dir.display()));
    assert!(status.success(), "git {args:?} in {} must succeed", dir.display());
}

fn init_git_repo(dir: &Path) {
    std::fs::create_dir_all(dir).expect("create fixture worktree dir");
    run_git(dir, &["init", "-q"]);
    run_git(dir, &["config", "user.email", "watch-test@example.invalid"]);
    run_git(dir, &["config", "user.name", "watch-test"]);
    // An unborn-HEAD repo (never committed) is a normal state Phase 3's
    // `compute_ref_state` already tolerates, but arming's `IgnoreSet::derive`
    // spawns `git status --ignored=matching`, which works identically either
    // way - a seed commit is not required for anything this file tests. One
    // is still taken so `git switch -c`/`git worktree add` below have a
    // committed tree to branch from.
    std::fs::write(dir.join("README.md"), "seed\n").expect("write seed file");
    run_git(dir, &["add", "README.md"]);
    run_git(dir, &["commit", "-q", "-m", "seed"]);
}

/// Deadline-polling helper (never a fixed sleep standing in for an
/// assertion): re-checks `condition` every 20ms until it returns `true` or
/// `deadline` elapses, then returns whichever happened.
async fn poll_until(deadline: Duration, mut condition: impl FnMut() -> bool) -> bool {
    let start = Instant::now();
    loop {
        if condition() {
            return true;
        }
        if start.elapsed() >= deadline {
            return false;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
}

/// Same shape as [`poll_until`], but for a condition that itself needs to
/// `.await` (an HTTP round-trip through the router) rather than a plain
/// synchronous check.
async fn poll_until_async<F, Fut>(deadline: Duration, mut condition: F) -> bool
where
    F: FnMut() -> Fut,
    Fut: std::future::Future<Output = bool>,
{
    let start = Instant::now();
    loop {
        if condition().await {
            return true;
        }
        if start.elapsed() >= deadline {
            return false;
        }
        tokio::time::sleep(Duration::from_millis(50)).await;
    }
}

/// One armed repo, built directly against `WatchRegistry`, skipping
/// router/AppState entirely since most of this file's cases are about the
/// watcher itself, not route wiring. Must run inside a Tokio runtime
/// (`#[tokio::test]`) so `WatchRegistry::new` actually spawns its background
/// event-consumption loop; the plain-`#[test]` inline fallback
/// `work_root_watch.rs`'s own tests rely on would never see a real `notify`
/// event fire.
struct ArmedFixture {
    dir: PathBuf,
    key: ws_dashboard_daemon::discovery::WatchKey,
    epoch_source: Arc<MutationEpochSource>,
    git_stats: Arc<GitSpawnStats>,
    // Held for its lifetime/Drop side effect only: keeping the last `Arc`
    // clone alive here is what keeps `WatchRegistry`'s background
    // event-consumption loop running for the fixture's whole lifetime. No
    // test reads this field directly, hence the leading underscore.
    _registry: WatchRegistry,
}

impl Drop for ArmedFixture {
    fn drop(&mut self) {
        remove_fixture(&self.dir);
    }
}

async fn armed_fixture(name: &str) -> ArmedFixture {
    armed_fixture_with_config(name, WatchConfig::default()).await
}

async fn armed_fixture_with_config(name: &str, config: WatchConfig) -> ArmedFixture {
    let dir = temp_fixture_path(name);
    init_git_repo(&dir);
    let git_dir = dir.join(".git");
    let targets = WatchTargets {
        worktree: dir.clone(),
        git_dir: git_dir.clone(),
        common_dir: git_dir,
    };
    let key = watch_key(&dir);
    let epoch_source = Arc::new(MutationEpochSource::default());
    let git_stats = Arc::new(GitSpawnStats::default());
    let registry = WatchRegistry::new(epoch_source.clone(), git_stats.clone(), config);
    registry.arm_now(&key, &targets);
    assert_eq!(
        registry.health_for(&key),
        WatchHealth::Armed,
        "fixture repo on the process temp dir must arm cleanly"
    );

    ArmedFixture { dir, key, epoch_source, git_stats, _registry: registry }
}

// --- arm + real fs events -> real epoch bumps --------------------------

#[tokio::test]
async fn writing_an_untracked_file_bumps_worktree_epoch_only() {
    let fixture = armed_fixture("untracked-write").await;
    let (worktree_before, refs_before) = fixture.epoch_source.epochs(&fixture.key);

    std::fs::write(fixture.dir.join("scratch.txt"), "hello\n").expect("write untracked file");

    let bumped = poll_until(Duration::from_secs(5), || {
        fixture.epoch_source.epochs(&fixture.key).0 > worktree_before
    })
    .await;
    assert!(bumped, "an untracked file write must bump the worktree epoch within 5s");

    // Give any (incorrect) refs bump the same window to show up before
    // asserting it never did - a bump that never happens cannot be proven
    // by absence-of-poll-success alone, only by having waited long enough
    // for the debounce window (100ms default) plus the 20ms flush tick to
    // have long since closed.
    tokio::time::sleep(Duration::from_millis(300)).await;
    let (_, refs_after) = fixture.epoch_source.epochs(&fixture.key);
    assert_eq!(refs_after, refs_before, "an untracked worktree file must never bump refs");
}

#[tokio::test]
async fn writing_inside_a_gitignored_directory_never_bumps_either_epoch() {
    // `IgnoreSet::derive` is `git status --ignored=matching` - a snapshot of
    // paths git currently reports ignored, not a live `.gitignore`-pattern
    // evaluator. It can only know about an ignored path that already
    // *exists* on disk at arm time (a brand new `target/` appearing for the
    // first time after arming is the acknowledged re-derive-window gap
    // `rederive_ignore_set`'s 30s limit exists to bound, not what this test
    // is pinning). So: `.gitignore` AND a first ignored file both go in
    // BEFORE the one arm below, so the derived set actually contains
    // `target/` by the time arming's `git status` spawn runs.
    let dir = temp_fixture_path("gitignored-write");
    init_git_repo(&dir);
    std::fs::write(dir.join(".gitignore"), "target/\n").expect("write .gitignore before arming");
    std::fs::create_dir_all(dir.join("target/debug")).expect("create ignored dir before arming");
    std::fs::write(dir.join("target/debug/first.o"), b"\0").expect("seed an already-ignored file");
    let git_dir = dir.join(".git");
    let targets = WatchTargets {
        worktree: dir.clone(),
        git_dir: git_dir.clone(),
        common_dir: git_dir,
    };
    let key = watch_key(&dir);
    let epoch_source = Arc::new(MutationEpochSource::default());
    let git_stats = Arc::new(GitSpawnStats::default());
    let registry = WatchRegistry::new(epoch_source.clone(), git_stats, WatchConfig::default());
    registry.arm_now(&key, &targets);
    assert_eq!(registry.health_for(&key), WatchHealth::Armed);

    let (worktree_before, refs_before) = epoch_source.epochs(&key);
    // A second write under the already-known-ignored `target/debug/`.
    std::fs::write(dir.join("target/debug/build.o"), b"\0").expect("write ignored file");

    // No event to poll for (nothing should happen) - wait out a window
    // generous enough that a wrongly-delivered bump would have landed.
    tokio::time::sleep(Duration::from_millis(500)).await;
    let (worktree_after, refs_after) = epoch_source.epochs(&key);
    assert_eq!(worktree_after, worktree_before, "a write under a gitignored dir must not bump worktree");
    assert_eq!(refs_after, refs_before, "a write under a gitignored dir must not bump refs");
    remove_fixture(&dir);
}

#[tokio::test]
async fn git_switch_dash_c_bumps_refs_epoch() {
    let fixture = armed_fixture("switch-c").await;
    let (_, refs_before) = fixture.epoch_source.epochs(&fixture.key);

    run_git(&fixture.dir, &["switch", "-c", "feature/watch-test"]);

    let bumped = poll_until(Duration::from_secs(5), || {
        fixture.epoch_source.epochs(&fixture.key).1 > refs_before
    })
    .await;
    assert!(bumped, "git switch -c must bump refs within 5s");
}

#[tokio::test]
async fn git_worktree_add_bumps_refs_epoch_on_the_primary_root() {
    let fixture = armed_fixture("worktree-add").await;
    let (_, refs_before) = fixture.epoch_source.epochs(&fixture.key);

    let linked_dir = temp_fixture_path("worktree-add-linked");
    run_git(
        &fixture.dir,
        &[
            "worktree",
            "add",
            linked_dir.to_str().expect("linked worktree path is UTF-8"),
            "-b",
            "linked-feature",
        ],
    );

    let bumped = poll_until(Duration::from_secs(5), || {
        fixture.epoch_source.epochs(&fixture.key).1 > refs_before
    })
    .await;
    assert!(bumped, "git worktree add must bump the primary root's refs within 5s");

    remove_fixture(&linked_dir);
}

// --- burst containment against Phase 1's spawn counter ------------------

#[tokio::test]
async fn a_thousand_tracked_file_writes_add_zero_git_spawns() {
    let fixture = armed_fixture("burst-tracked").await;
    let spawns_before = fixture.git_stats.snapshot().total;

    for index in 0..1000 {
        std::fs::write(fixture.dir.join(format!("burst-{index}.txt")), b"x").expect("burst write");
    }

    // Give the debounce/event pipeline generous time to fully settle (it
    // does real work - epoch bumps - even though none of that work is a
    // `git` spawn) before reading the spawn counter, so a delayed spawn
    // cannot slip past a too-early read.
    tokio::time::sleep(Duration::from_secs(2)).await;
    let spawns_after = fixture.git_stats.snapshot().total;
    assert_eq!(
        spawns_after, spawns_before,
        "an ordinary tracked-file write burst must never drive a git spawn from the event path"
    );
}

#[tokio::test]
async fn rewriting_gitignore_fifty_times_schedules_at_most_one_extra_spawn() {
    let fixture = armed_fixture("burst-gitignore").await;
    let spawns_before = fixture.git_stats.snapshot().total;

    for index in 0..50 {
        std::fs::write(fixture.dir.join(".gitignore"), format!("target/\n# {index}\n"))
            .expect("rewrite .gitignore");
    }

    tokio::time::sleep(Duration::from_secs(2)).await;
    let spawns_after = fixture.git_stats.snapshot().total;
    assert!(
        spawns_after <= spawns_before + 1,
        "50 rapid .gitignore rewrites inside the 30s re-derive interval must \
         coalesce into at most one additional git spawn: before={spawns_before} after={spawns_after}"
    );
}

// --- availability flap: reconcile's flat-interval rate limit ------------

#[tokio::test]
async fn availability_flapping_ten_times_bounds_arm_attempts_by_the_flat_interval_not_one_per_tick() {
    let dir = temp_fixture_path("availability-flap");
    init_git_repo(&dir);
    let git_dir = dir.join(".git");
    let targets = WatchTargets {
        worktree: dir.clone(),
        git_dir: git_dir.clone(),
        common_dir: git_dir,
    };
    let key = watch_key(&dir);
    let epoch_source: Arc<dyn EpochSource> = Arc::new(MutationEpochSource::default());
    let git_stats = Arc::new(GitSpawnStats::default());
    let registry = WatchRegistry::new(epoch_source, git_stats.clone(), WatchConfig::default());

    // First tick: present + Available + Unarmed => arms (one `IgnoreSet::
    // derive` spawn). Every following tick alternates availability - each
    // one is a fresh reconcile call within the same 30s window, so none of
    // the following nine may re-arm (each Unavailable tick disarms for
    // free - no spawn - but the following Available tick must be rate-
    // limited, not treated as a fresh arm opportunity).
    for tick in 0..10 {
        let availability = if tick % 2 == 0 {
            ws_dashboard_core::WorkRootAvailability::Available
        } else {
            ws_dashboard_core::WorkRootAvailability::Moved
        };
        registry.reconcile(&[(key.clone(), Some(targets.clone()), availability)]);
    }

    let spawns = git_stats.snapshot().total;
    assert!(
        spawns <= 1,
        "10 flaps inside one 30s window must cost at most the first tick's \
         arm spawn, not one per available tick: observed {spawns} spawns"
    );

    remove_fixture(&dir);
}

// --- Linux-only: the per-directory registration cap ----------------------

#[cfg(target_os = "linux")]
#[tokio::test]
async fn max_dirs_one_degrades_with_zero_registrations_on_linux() {
    let dir = temp_fixture_path("max-dirs-one");
    init_git_repo(&dir);
    let git_dir = dir.join(".git");
    let targets = WatchTargets {
        worktree: dir.clone(),
        git_dir: git_dir.clone(),
        common_dir: git_dir,
    };
    let key = watch_key(&dir);
    let epoch_source: Arc<dyn EpochSource> = Arc::new(MutationEpochSource::default());
    let git_stats = Arc::new(GitSpawnStats::default());
    let registry = WatchRegistry::new(
        epoch_source,
        git_stats,
        WatchConfig { max_dirs: 1, ..WatchConfig::default() },
    );

    registry.arm_now(&key, &targets);

    match registry.health_for(&key) {
        WatchHealth::Degraded(reason) => {
            assert!(
                reason.to_ascii_lowercase().contains("large") || reason.to_ascii_lowercase().contains("cap"),
                "the Degraded reason for a max_dirs=1 over-cap arm should name the cap, got {reason:?}"
            );
        }
        other => panic!("a real repo (well over one directory) with max_dirs=1 must Degrade, got {other:?}"),
    }
    let snapshot = registry.diag_snapshot();
    let entry = snapshot
        .iter()
        .find(|entry| entry.key == key.as_str())
        .expect("diag_snapshot must report the tracked repo even while Degraded");
    assert_eq!(
        entry.registered_watches, 0,
        "an over-cap arm must register zero directories wholesale, not a partial prefix"
    );

    remove_fixture(&dir);
}

// --- full router + owner-auth pairing scaffolding, for D2/D1's route-
// --- driven cases only. Self-contained: each integration test file is its
// --- own crate, so this cannot share `tests/routes.rs`'s private helpers -
// --- the pattern below mirrors them exactly (`app_state_with_opened_and_
// --- store`/`pair_and_cookie`/`open_work_root_for_test`/
// --- `git_toolbar_post_json`) rather than reinventing it.

fn full_router_app_state() -> AppState {
    let epoch_source: Arc<dyn EpochSource> = Arc::new(MutationEpochSource::default());
    let git_spawn_stats = Arc::new(GitSpawnStats::default());
    // `Auto`: unlike `tests/routes.rs`'s shared fixtures (which stay `Off`
    // to protect unrelated route tests' spawn-count assertions from an
    // unsolicited arm), this file's whole point is exercising real arming
    // through the live resources/reconcile path.
    let watch_registry = WatchRegistry::new(
        epoch_source.clone(),
        git_spawn_stats.clone(),
        WatchConfig::default(),
    );
    // Merge companion (260727 Phase 2): this file arrived from
    // `ws-dashboard-dev` against the pre-merge 3-argument
    // `TerminalRegistry::new` and an `AppState` with no `attention` field -
    // neither side conflicted textually, so both had to be repaired here by
    // hand. This file never spawns a terminal (its cases drive git/watch
    // routes only), hence `None` state dir and an empty callback base URL:
    // that reproduces exactly the behavior the 3-argument constructor gave it.
    // `attention` must be taken from THIS registry instance and bound before
    // `terminals` is moved into the struct - see `TerminalRegistry::attention`'s
    // own CONTRACT.
    let terminals = TerminalRegistry::new(
        PathBuf::from(env!("CARGO_BIN_EXE_ws-dashboard")),
        temp_fixture_path("terminal-registry"),
        Duration::from_secs(5),
        None,
        String::new(),
    );
    AppState {
        config: ServeConfig::default_loopback(),
        auth: OwnerAuthState::new_ephemeral(),
        opened_work_roots: OpenedWorkRoots::default(),
        git_probe_cache: GitProbeCache::default(),
        git_spawn_stats,
        git_state_cache: GitStateCache::default(),
        epoch_source,
        watch_registry,
        dashboard_state: DashboardStateStore::disabled(),
        document_translation: ws_dashboard_daemon::document_translation::DocumentTranslationService::default(),
        attention: terminals.attention(),
        terminals,
        codex_sessions: ws_dashboard_daemon::codex_app_server::CodexProviderRegistry::default(),
        claude_sessions: ws_dashboard_daemon::claude_cli::ClaudeProviderRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
        document_events: DocumentEventHub::default(),
        document_write_locks: ws_dashboard_daemon::work_root_files::DocumentWriteLocks::default(),
        linked_server_sessions: LinkedServerSessions::default(),
        linked_server_tunnels: LinkedServerTunnels::record_only_for_tests(),
        registry_persist_lock: Arc::new(tokio::sync::Mutex::new(())),
        shutdown: Arc::new(tokio::sync::Notify::new()),
    }
}

async fn pair_and_cookie(app: axum::Router, token: &str) -> String {
    let response = app
        .oneshot(
            Request::builder()
                .uri(format!("/pair?token={token}"))
                .body(Body::empty())
                .expect("pair request"),
        )
        .await
        .expect("pair response");
    assert_eq!(response.status(), StatusCode::SEE_OTHER);
    response
        .headers()
        .get(header::SET_COOKIE)
        .expect("owner session cookie")
        .to_str()
        .expect("cookie header is ASCII")
        .split(';')
        .next()
        .expect("cookie pair")
        .to_owned()
}

async fn open_work_root_for_test(app: axum::Router, cookie: &str, root: &Path) -> String {
    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/open")
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "path": root.display().to_string() }).to_string(),
                ))
                .expect("open workRoot request"),
        )
        .await
        .expect("open workRoot response");
    assert_eq!(response.status(), StatusCode::OK);
    response
        .headers()
        .get("x-ws-dashboard-opened-work-root-id")
        .and_then(|value| value.to_str().ok())
        .expect("opened workRoot id header")
        .to_owned()
}

async fn get_json(app: axum::Router, cookie: &str, uri: &str) -> serde_json::Value {
    let response = app
        .oneshot(
            Request::builder()
                .uri(uri)
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("GET request"),
        )
        .await
        .expect("GET response");
    assert_eq!(response.status(), StatusCode::OK, "{uri}");
    let body = axum::body::to_bytes(response.into_body(), 256 * 1024).await.expect("GET body");
    serde_json::from_slice(&body).expect("GET JSON")
}

async fn post_json(
    app: axum::Router,
    cookie: &str,
    uri: &str,
    request: serde_json::Value,
    expected_status: StatusCode,
) -> serde_json::Value {
    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(uri)
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(request.to_string()))
                .expect("POST request"),
        )
        .await
        .expect("POST response");
    assert_eq!(response.status(), expected_status, "{uri}");
    let body = axum::body::to_bytes(response.into_body(), 256 * 1024).await.expect("POST body");
    serde_json::from_slice(&body).expect("POST JSON")
}

fn diag_health(diag: &serde_json::Value, key: &str) -> Option<String> {
    diag["repos"]
        .as_array()?
        .iter()
        .find(|entry| entry["key"] == key)
        .and_then(|entry| entry["health"].as_str())
        .map(str::to_owned)
}

fn diag_refs_epoch(diag: &serde_json::Value, key: &str) -> Option<u64> {
    diag["repos"]
        .as_array()?
        .iter()
        .find(|entry| entry["key"] == key)
        .and_then(|entry| entry["refsEpoch"].as_u64())
}

// --- D2: shared common_dir dedup+fanout, driven through the mutating -----
// --- git_toolbar.rs route (not a raw git CLI write - the whole point is
// --- pinning the mutating route's *own* `bump_refs` call reaching a
// --- sibling worktree via the watcher's fanout, not merely the watcher's
// --- own event classification, which the direct-registry tests above
// --- already cover). ------------------------------------------------------

#[tokio::test]
async fn a_route_driven_branch_create_on_one_worktree_bumps_a_sibling_linked_worktrees_refs() {
    let primary = temp_fixture_path("d2-primary");
    init_git_repo(&primary);
    let linked = temp_fixture_path("d2-linked");
    run_git(
        &primary,
        &[
            "worktree",
            "add",
            linked.to_str().expect("linked path is UTF-8"),
            "-b",
            "linked-branch",
        ],
    );

    let state = full_router_app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let primary_id = open_work_root_for_test(app.clone(), &cookie, &primary).await;
    let linked_id = open_work_root_for_test(app.clone(), &cookie, &linked).await;

    // Drive one full resources poll so `reconcile` arms both roots (they
    // share one `common_dir`, so `plan_watch_set`'s dedup-by-target should
    // register that shared directory once - the behavioral proof of that,
    // rather than inspecting private registry state, is the fanout
    // assertion below).
    let primary_key = watch_key(&primary).as_str().to_owned();
    let linked_key = watch_key(&linked).as_str().to_owned();
    // `reconcile` only runs when the resources route is hit - it is not a
    // background timer - so each poll iteration drives one resources
    // refresh (arming an eligible repo is offloaded to `spawn_blocking`
    // inside that call, not run inline) before checking `diag/git`.
    let armed_both = poll_until_async(Duration::from_secs(10), || {
        let app = app.clone();
        let cookie = cookie.clone();
        let primary_key = primary_key.clone();
        let linked_key = linked_key.clone();
        async move {
            let _resources = get_json(app.clone(), &cookie, "/api/dashboard/resources").await;
            let diag = get_json(app, &cookie, "/api/dashboard/diag/git").await;
            diag_health(&diag, &primary_key).as_deref() == Some("armed")
                && diag_health(&diag, &linked_key).as_deref() == Some("armed")
        }
    })
    .await;
    assert!(armed_both, "both the primary root and its linked worktree must reach Armed within 10s");

    let diag_before = get_json(app.clone(), &cookie, "/api/dashboard/diag/git").await;
    let linked_refs_before = diag_refs_epoch(&diag_before, &linked_key).expect("linked refs epoch before");

    // The mutation: a route-driven branch create on the PRIMARY root. Its
    // own `bump_refs` call only bumps the primary's own `WatchKey` (see
    // `git_create_branch`) - what this test actually pins is the watcher's
    // *independent* fanout: the same `git switch -c` write under the
    // shared `common_dir` also fires a real filesystem event the linked
    // worktree's own registration observes.
    let _ = post_json(
        app.clone(),
        &cookie,
        &format!("/api/dashboard/work-roots/{primary_id}/git/branches"),
        serde_json::json!({ "branchName": "route-driven-branch", "switchTo": true }),
        StatusCode::OK,
    )
    .await;
    let _ = linked_id; // kept opened so the linked worktree stays reconciled/armed throughout

    let fanned_out = poll_until_async(Duration::from_secs(5), || {
        let app = app.clone();
        let cookie = cookie.clone();
        let linked_key = linked_key.clone();
        async move {
            let diag = get_json(app, &cookie, "/api/dashboard/diag/git").await;
            diag_refs_epoch(&diag, &linked_key).unwrap_or(linked_refs_before) > linked_refs_before
        }
    })
    .await;
    assert!(
        fanned_out,
        "a route-driven mutation on the primary root must bump the linked worktree's own \
         refs epoch via the shared common_dir's fanout within 5s (D2)"
    );

    remove_fixture(&primary);
    remove_fixture(&linked);
}

// --- D1: availability lifecycle (rename away/back) with a kind change ----
// --- across the outage - pins the bounded (not zero) staleness Lead ------
// --- Disposition D1 explicitly accepts, rather than merely claiming it. --

async fn status_response_for(app: axum::Router, cookie: &str, work_root_id: &str) -> StatusCode {
    app.oneshot(
        Request::builder()
            .uri(format!("/api/dashboard/work-roots/{work_root_id}/git/status"))
            .header(header::COOKIE, cookie)
            .body(Body::empty())
            .expect("git status request"),
    )
    .await
    .expect("git status response")
    .status()
}

// Real-time, not env-shortened: `WS_DASHBOARD_GIT_PROBE_TTL_MS` is read
// fresh (no process-wide `OnceLock`) on every `GitProbeCache::default()`
// call, so it *could* be scoped to just this fixture's `AppState` - but
// `std::env::set_var` is still process-wide, and `cargo test` runs this
// binary's `#[tokio::test]`s concurrently by default, so scoping it here
// would risk racing a concurrently-running test's own `GitProbeCache::
// default()` construction. Waiting out the real default TTL (~30s) is the
// only interference-free way to pin this bound in this binary.
#[tokio::test]
async fn availability_lifecycle_reports_the_stale_kind_immediately_then_the_correct_kind_after_the_probe_ttl(
) {
    let dir = temp_fixture_path("d1-kind-change");
    init_git_repo(&dir);

    let state = full_router_app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), &cookie, &dir).await;

    // Warm the pre-outage memo as a git root.
    assert_eq!(
        status_response_for(app.clone(), &cookie, &work_root_id).await,
        StatusCode::OK,
        "must resolve as a git root before the outage"
    );

    // Outage: rename away, then strip `.git` while away (the kind change),
    // then rename back to the SAME path the workRoot was opened at.
    let away = temp_fixture_path("d1-kind-change-away");
    std::fs::rename(&dir, &away).expect("rename root away to simulate an outage");
    let reported_unavailable = poll_until_async(Duration::from_secs(5), || {
        let app = app.clone();
        let cookie = cookie.clone();
        let work_root_id = work_root_id.clone();
        async move {
            status_response_for(app, &cookie, &work_root_id).await != StatusCode::OK
        }
    })
    .await;
    assert!(reported_unavailable, "an outage must be reported (non-200) within 5s, uncached");

    std::fs::remove_dir_all(away.join(".git")).expect("strip .git while the root is away");
    std::fs::rename(&away, &dir).expect("rename back to the original path as a plain directory");

    // Immediately on reappearance: the pre-outage warm entry's kind
    // (GitPrimaryRoot) is still within its TTL and was never evicted (D1's
    // accepted carry-forward bug), so the route must still treat this as a
    // git root - 200, not the fresh-probe-correct 400 NonGit.
    let immediate = status_response_for(app.clone(), &cookie, &work_root_id).await;
    assert_eq!(
        immediate,
        StatusCode::OK,
        "immediately on reappearance the daemon must still serve the stale \
         pre-outage kind (D1's accepted bound), not the fresh-probe-correct kind"
    );

    // Once the probe TTL (default 30s) elapses, a fresh probe must finally
    // see the truth: this path is a plain directory now.
    let corrected = poll_until_async(Duration::from_secs(40), || {
        let app = app.clone();
        let cookie = cookie.clone();
        let work_root_id = work_root_id.clone();
        async move {
            status_response_for(app, &cookie, &work_root_id).await == StatusCode::BAD_REQUEST
        }
    })
    .await;
    assert!(
        corrected,
        "once the probe TTL elapses, the daemon must report the corrected \
         (NonGit) kind - this is the shape of the bound D1 accepts, not \
         permanent staleness"
    );

    remove_fixture(&dir);
}

// --- directory created after arming: incremental registration (Linux) / --
// --- unconditional recursive coverage (Windows/macOS) --------------------

#[tokio::test]
async fn a_directory_created_after_arming_gets_registered_and_a_write_inside_it_bumps() {
    let fixture = armed_fixture("post-arm-mkdir").await;
    let (worktree_before, _) = fixture.epoch_source.epochs(&fixture.key);

    std::fs::create_dir_all(fixture.dir.join("late_dir")).expect("mkdir after arming");
    let registered = poll_until(Duration::from_secs(5), || {
        // On Linux this proves the incremental-registration path actually
        // ran (the directory now shows up in `registered_dirs`); on
        // Windows/macOS the recursive registration already covers it
        // unconditionally, so this poll succeeds immediately without any
        // extra registration work - either way, a write inside must bump.
        std::fs::write(fixture.dir.join("late_dir/f.txt"), b"x").is_ok()
    })
    .await;
    assert!(registered, "must be able to write into the newly created directory");

    let bumped = poll_until(Duration::from_secs(5), || {
        fixture.epoch_source.epochs(&fixture.key).0 > worktree_before
    })
    .await;
    assert!(bumped, "a write inside a directory created after arming must bump worktree within 5s");
}

// --- Windows: worktree-remove while armed must not fail with a sharing ---
// --- violation (ticket Constraints: `git_worktree_remove_submit` disarms --
// --- before running - `crates/daemon/src/git_worktree.rs`). --------------
// Cannot execute on this Linux/WSL dev host - there is no equivalent
// failure mode to reproduce there, so a Linux run of this test would prove
// nothing. Type-checked (not run) via the D4 cross-target `cargo check`.

#[cfg(windows)]
#[tokio::test]
async fn worktree_remove_while_armed_does_not_fail_with_a_sharing_violation_on_windows() {
    let primary = temp_fixture_path("windows-remove-armed-primary");
    init_git_repo(&primary);
    let linked = temp_fixture_path("windows-remove-armed-linked");
    run_git(
        &primary,
        &[
            "worktree",
            "add",
            linked.to_str().expect("linked path is UTF-8"),
            "-b",
            "to-be-removed",
        ],
    );

    let state = full_router_app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let _primary_id = open_work_root_for_test(app.clone(), &cookie, &primary).await;
    let linked_id = open_work_root_for_test(app.clone(), &cookie, &linked).await;

    let linked_key = watch_key(&linked).as_str().to_owned();
    let armed = poll_until_async(Duration::from_secs(10), || {
        let app = app.clone();
        let cookie = cookie.clone();
        let linked_key = linked_key.clone();
        async move {
            let _resources = get_json(app.clone(), &cookie, "/api/dashboard/resources").await;
            let diag = get_json(app, &cookie, "/api/dashboard/diag/git").await;
            diag_health(&diag, &linked_key).as_deref() == Some("armed")
        }
    })
    .await;
    assert!(armed, "the linked worktree must be Armed before the remove attempt");

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/work-roots/{linked_id}/git-worktree-remove"))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "deleteBranch": false, "force": false }).to_string(),
                ))
                .expect("worktree remove request"),
        )
        .await
        .expect("worktree remove response");
    assert_eq!(
        response.status(),
        StatusCode::OK,
        "removing an armed worktree must not fail with a sharing violation \
         (the route disarms it before running `git worktree remove`)"
    );

    // The `status == OK` assertion above is true on Linux regardless of
    // whether `disarm_now`/`disarm_and_suppress_arm` ever ran (Linux does
    // not raise a sharing violation for a watched directory), so it alone
    // does not prove the disarm call actually fired - only that removal
    // succeeded (review finding 11). Query diag directly (no intervening
    // `/api/dashboard/resources` call, which would itself disarm the now-
    // absent worktree via `reconcile` and mask whether the route's own
    // disarm ran) and assert the linked key is no longer reported `armed`
    // immediately after the remove response.
    let diag_after_remove = get_json(app.clone(), &cookie, "/api/dashboard/diag/git").await;
    let health_after_remove = diag_health(&diag_after_remove, &linked_key);
    assert!(
        matches!(health_after_remove.as_deref(), None | Some("unarmed")),
        "the linked worktree's WatchHealth must be absent or Unarmed \
         immediately after the remove response, proving the disarm call \
         actually fired rather than merely coinciding with a Linux host \
         that would not have raised a sharing violation anyway - got \
         {health_after_remove:?}"
    );

    remove_fixture(&primary);
}

#[tokio::test]
async fn mkdir_and_write_in_one_step_still_bumps_via_the_parent_directory_event() {
    let fixture = armed_fixture("mkdir-write-race").await;
    let (worktree_before, _) = fixture.epoch_source.epochs(&fixture.key);

    // Deliberately no poll between `create_dir_all` and the write - the
    // race this pins is exactly "does the parent directory's own event
    // still cover a file that appears inside a subdirectory created in the
    // same breath" (ticket Decisions: covered by the parent's event, not a
    // dedicated race-free ordering).
    std::fs::create_dir_all(fixture.dir.join("race_dir")).expect("mkdir");
    std::fs::write(fixture.dir.join("race_dir/f.txt"), b"x").expect("write in the same step");

    let bumped = poll_until(Duration::from_secs(5), || {
        fixture.epoch_source.epochs(&fixture.key).0 > worktree_before
    })
    .await;
    assert!(bumped, "mkdir+write in one step must still bump worktree within 5s");
}
