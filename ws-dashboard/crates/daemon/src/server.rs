use std::future::{Future, IntoFuture};
use std::net::{IpAddr, SocketAddr};
use std::sync::Arc;
use std::time::Duration;

use tokio::net::TcpListener;
use tokio::sync::watch;
use tokio::sync::Mutex;
use tracing::info;

use crate::auth::OwnerAuthState;
use crate::config::{validate_bind_guard, ServeConfig};
use crate::persistent_state::DashboardStateStore;
use crate::router::{build_router, AppState};
use crate::terminal::TerminalRegistry;
use crate::work_root_activity::WorkRootActivityProjector;
use crate::work_root_files::{OpenedWorkRoots, RegisteredWorkRoot};

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StartupInfo {
    pub bound_addr: SocketAddr,
    pub pairing_url: String,
    pub direct_dashboard_url: Option<String>,
    pub link_passphrase: String,
    pub owner_auth_enabled: bool,
}

pub const DEFAULT_SHUTDOWN_GRACE_PERIOD: Duration = Duration::from_millis(750);

// CONTRACT (260725 Phase 4): no existing periodic-background-task precedent
// in this codebase to match (see plan Codebase Findings) - chosen generous
// enough that it never contends with normal terminal churn (a stale
// `agent-profiles/` directory sitting around for up to this long between
// sweeps is harmless; the important guarantee is ordering relative to
// `boot_reconcile`, not sweep frequency).
// `pub(crate)` only so `agent_profile_gc`'s TESTS can pass the same window the
// production wiring below passes (a duplicated `Duration::from_secs(300)`
// literal there would silently keep testing the old window if this constant
// ever moves). The sweep itself still takes the window as a parameter and
// never reaches back up into this module for it - see
// `sweep_agent_profiles`' own doc comment.
pub(crate) const AGENT_PROFILE_GC_SWEEP_PERIOD: Duration = Duration::from_secs(300);

pub async fn run(config: ServeConfig) -> anyhow::Result<()> {
    run_with_shutdown(config, shutdown_signal())
        .await
        .map(|_| ())
}

pub async fn run_with_shutdown<F>(config: ServeConfig, shutdown: F) -> anyhow::Result<StartupInfo>
where
    F: Future<Output = ()> + Send + 'static,
{
    run_with_shutdown_and_grace(config, shutdown, DEFAULT_SHUTDOWN_GRACE_PERIOD).await
}

pub async fn run_with_shutdown_and_grace<F>(
    config: ServeConfig,
    shutdown: F,
    grace_period: Duration,
) -> anyhow::Result<StartupInfo>
where
    F: Future<Output = ()> + Send + 'static,
{
    // CONTRACT: Server startup prints or otherwise exposes exactly one owner
    // pairing URL once the bound address is known.
    // CONTRACT: Graceful shutdown is part of the daemon shell; request logging
    // must avoid leaking pairing query strings.
    validate_bind_guard(
        config.bind_mode,
        config.bind_addr.ip(),
        config.owner_auth_enabled,
    )?;
    let auth = OwnerAuthState::new_ephemeral();
    let listener = TcpListener::bind(config.bind_addr).await?;
    // CONTRACT (ticket 260723-bug-dashboard-terminal-helper-inherits-daemon-listen-socket):
    // the listening socket must be non-inheritable on Windows so detached
    // terminal-helpers spawned later do not inherit it and pin the port after
    // this daemon exits (which would fail a same-port restart with
    // WSAEADDRINUSE / os error 10048). No-op on Unix.
    #[cfg(windows)]
    crate::terminal_platform::windows::mark_socket_non_inheritable(&listener)?;
    let bound_addr = listener.local_addr()?;
    let info = startup_info(bound_addr, &auth, config.owner_auth_enabled);

    // CONTRACT (260725 Phase 4): computed once, unconditionally, so it can
    // be threaded into BOTH `write_bound_base_url` below and
    // `TerminalRegistry::boot_reconcile` further down - the single source
    // of truth `agent_callback.rs`'s own CONTRACT insists on (never re-read
    // `bound-base-url.json` to derive a per-terminal `callback.json`'s
    // `baseUrl`, which would reintroduce the rejected multi-daemon-steal
    // shape).
    let base_url = format!("http://{}", display_addr(bound_addr));
    let state_dir = crate::persistent_state::default_state_dir();

    // CONTRACT (260725 Phase 3 step 3, "ephemeral port"): written
    // unconditionally on every bind, strictly BEFORE `boot_reconcile` below -
    // it needs no terminal/registry state, only the bound address, and
    // writing it first means any adopted terminal's eventual (Phase 4)
    // callback rewrite can already find a fresh file. Best-effort: a warning
    // on failure, not a startup abort, mirrors how startup already tolerates
    // a missing state dir elsewhere (`opened_work_roots` seeding).
    if let Some(state_dir) = state_dir.as_deref() {
        if let Err(error) = crate::agent_callback::write_bound_base_url(state_dir, &base_url) {
            tracing::warn!(%error, "failed to write bound-base-url file");
        }
    }

    eprintln!("ws-dashboard owner pairing URL: {}", info.pairing_url);
    if let Some(url) = info.direct_dashboard_url.as_deref() {
        eprintln!("ws-dashboard no-auth debug mode active: {url}");
    }
    eprintln!(
        "ws-dashboard remote link passphrase: {}",
        info.link_passphrase
    );
    info!(bound_addr = %info.bound_addr, "ws-dashboard daemon listening");

    let dashboard_state = DashboardStateStore::default_local();
    let opened_work_roots = OpenedWorkRoots::default();
    for entry in dashboard_state.load_work_root_registry().await {
        opened_work_roots.register_registry_entry(
            crate::discovery::local_work_root_id_for_path(&entry.path),
            RegisteredWorkRoot {
                path: entry.path,
                activation: entry.activation,
                provenance: entry.provenance,
            },
        );
    }
    // CONTRACT (ticket 260723 Phase 1 "Boot reconcile policy"): must
    // complete before `build_router`/`axum::serve` starts accepting
    // connections, so an adopted terminal is already visible to the very
    // first `list_terminals`/WS-reattach request this daemon instance
    // serves.
    let terminals = TerminalRegistry::boot_reconcile(
        crate::terminal::default_helper_binary(),
        crate::terminal::default_registry_dir(),
        crate::terminal::DEFAULT_RECONCILE_CONNECT_TIMEOUT,
        state_dir.clone(),
        base_url,
    )
    .await;

    // CONTRACT (260725 Phase 4, GC sweep ordering - LOAD-BEARING): this
    // spawn statement is textually AND temporally AFTER the already-awaited
    // `boot_reconcile` call above. Running the sweep any earlier (e.g.
    // against a freshly-constructed, still-empty registry) would see zero
    // live terminal ids and delete the profile directory of every helper
    // `boot_reconcile` is about to adopt a moment later - see
    // `agent_profile_gc.rs`'s module CONTRACT and
    // `crates/daemon/tests/terminal_notify_callback_restart.rs`'s ordering
    // regression test, which is what actually proves this placement rather
    // than merely documenting it.
    // CONTRACT (260725 Phase 4 review cycle 1, finding B): unlike this
    // crate's other untracked `tokio::spawn` calls (`spawn_ipc_reader_task`,
    // the `claude_cli`/`codex_app_server` reader loops), this task is
    // self-bounding on NOTHING - it is an infinite `loop { tick; sweep }`
    // with no connection/pipe to close underneath it. Left untracked, it
    // would run forever regardless of this daemon's own shutdown signal,
    // unlike `shutdown_task` below (this crate's one other background task,
    // tracked and `.abort()`-ed in both `select!` arms). Track the
    // `JoinHandle` and abort it alongside `shutdown_task` so it participates
    // in the same graceful-shutdown path instead of leaking.
    // CONTRACT (260726 Phase 1): both the sweep PERIOD and the
    // delivery-failure escalation's GRACE WINDOW are supplied from this one
    // call site, out of the same constant. The escalation rule deliberately
    // does not reach up here for it: `agent_profile_gc`/`notify_failure` are
    // leaf modules, and having them import a constant from this top-level
    // wiring module would invert the layering just to name a number. Passing
    // it down also lets the rule's unit tests choose their own window.
    let gc_sweep_task = state_dir.map(|state_dir| {
        let sweep_registry = terminals.clone();
        tokio::spawn(async move {
            // Owned by THIS task (never a module static), so the warn-once
            // set dies with the task on the `.abort()` shutdown path below.
            let mut notify_failure_watch =
                crate::notify_failure::NotifyFailureWatch::default();
            crate::agent_profile_gc::sweep_agent_profiles(
                &state_dir,
                &sweep_registry,
                &mut notify_failure_watch,
                AGENT_PROFILE_GC_SWEEP_PERIOD,
            )
            .await;
            let mut interval = tokio::time::interval(AGENT_PROFILE_GC_SWEEP_PERIOD);
            // The first tick fires immediately; the sweep above already
            // covered "run once immediately at boot", so this first tick is
            // consumed without triggering a redundant second sweep.
            interval.tick().await;
            loop {
                interval.tick().await;
                crate::agent_profile_gc::sweep_agent_profiles(
                    &state_dir,
                    &sweep_registry,
                    &mut notify_failure_watch,
                    AGENT_PROFILE_GC_SWEEP_PERIOD,
                )
                .await;
            }
        })
    });

    // In-app "shut down dashboard" trigger: an HTTP handler fires this Notify,
    // which the shutdown_task below selects on alongside the external signal.
    let shutdown_notify = Arc::new(tokio::sync::Notify::new());
    // `epoch_source` and `git_spawn_stats` are shared with `watch_registry`
    // below: a watcher-driven epoch bump must land through the exact same
    // `Arc` `git_toolbar.rs`'s `AppState.epoch_source` reads, and the
    // registry's own `git status`/walk spawns must count against the same
    // `AppState.git_spawn_stats` diag totals as every other route (ticket
    // step 8).
    let epoch_source: Arc<dyn crate::git_state_cache::EpochSource> =
        Arc::new(crate::git_state_cache::MutationEpochSource::default());
    let git_spawn_stats = Arc::new(crate::git_exec::GitSpawnStats::default());
    let watch_registry = crate::work_root_watch::WatchRegistry::new(
        epoch_source.clone(),
        git_spawn_stats.clone(),
        crate::work_root_watch::WatchConfig::from_env(),
    );
    let app = build_router(AppState {
        config,
        auth,
        opened_work_roots,
        git_probe_cache: crate::discovery::GitProbeCache::default(),
        git_spawn_stats,
        git_state_cache: crate::git_state_cache::GitStateCache::default(),
        epoch_source,
        watch_registry,
        dashboard_state,
        document_translation: crate::document_translation::DocumentTranslationService::from_env(),
        // CONTRACT (260725 Phase 5): must be `terminals.attention()` - a
        // clone of the SAME hub `terminals` internally holds - captured
        // BEFORE `terminals` is moved into the field below. See
        // `TerminalRegistry::attention`'s own CONTRACT.
        attention: terminals.attention(),
        terminals,
        codex_sessions: crate::codex_app_server::CodexProviderRegistry::default(),
        claude_sessions: crate::claude_cli::ClaudeProviderRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
        document_events: crate::work_root_files::DocumentEventHub::default(),
        document_write_locks: crate::work_root_files::DocumentWriteLocks::default(),
        linked_server_sessions: crate::servers::LinkedServerSessions::default(),
        linked_server_tunnels: crate::servers::LinkedServerTunnels::default(),
        registry_persist_lock: Arc::new(Mutex::new(())),
        shutdown: shutdown_notify.clone(),
    });
    let (shutdown_tx, shutdown_rx) = watch::channel(false);
    let shutdown_task = tokio::spawn(async move {
        tokio::select! {
            () = shutdown => {}
            () = shutdown_notify.notified() => {}
        }
        let _ = shutdown_tx.send(true);
    });
    let server = axum::serve(listener, app)
        .with_graceful_shutdown(wait_for_shutdown(shutdown_rx.clone()))
        .into_future();
    tokio::pin!(server);

    tokio::select! {
        result = &mut server => {
            shutdown_task.abort();
            if let Some(gc_sweep_task) = &gc_sweep_task {
                gc_sweep_task.abort();
            }
            result?;
        }
        () = force_after_shutdown(shutdown_rx, grace_period) => {
            shutdown_task.abort();
            if let Some(gc_sweep_task) = &gc_sweep_task {
                gc_sweep_task.abort();
            }
            tracing::warn!(
                grace_period_ms = grace_period.as_millis(),
                "forcing ws-dashboard daemon shutdown after grace period"
            );
        }
    }

    Ok(info)
}

pub fn startup_info(
    bound_addr: SocketAddr,
    auth: &OwnerAuthState,
    owner_auth_enabled: bool,
) -> StartupInfo {
    StartupInfo {
        bound_addr,
        pairing_url: format!(
            "http://{}/pair?token={}",
            display_addr(bound_addr),
            auth.pairing_token().expose_for_owner_url()
        ),
        direct_dashboard_url: (!owner_auth_enabled)
            .then(|| format!("http://{}/", display_addr(bound_addr))),
        link_passphrase: auth.link_passphrase().expose_for_owner_record().to_owned(),
        owner_auth_enabled,
    }
}

async fn shutdown_signal() {
    if let Err(error) = tokio::signal::ctrl_c().await {
        tracing::warn!(%error, "failed to install ctrl-c shutdown signal");
    }
}

async fn wait_for_shutdown(mut shutdown_rx: watch::Receiver<bool>) {
    if *shutdown_rx.borrow() {
        return;
    }
    while shutdown_rx.changed().await.is_ok() {
        if *shutdown_rx.borrow() {
            return;
        }
    }
}

async fn force_after_shutdown(shutdown_rx: watch::Receiver<bool>, grace_period: Duration) {
    wait_for_shutdown(shutdown_rx).await;
    tokio::time::sleep(grace_period).await;
}

fn display_addr(addr: SocketAddr) -> String {
    match addr.ip() {
        IpAddr::V4(_) => addr.to_string(),
        IpAddr::V6(ip) => format!("[{ip}]:{}", addr.port()),
    }
}
