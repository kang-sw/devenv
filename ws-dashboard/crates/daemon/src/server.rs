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
    )
    .await;
    let app = build_router(AppState {
        config,
        auth,
        opened_work_roots,
        dashboard_state,
        document_translation: crate::document_translation::DocumentTranslationService::from_env(),
        terminals,
        codex_sessions: crate::codex_app_server::CodexProviderRegistry::default(),
        claude_sessions: crate::claude_cli::ClaudeProviderRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
        document_events: crate::work_root_files::DocumentEventHub::default(),
        document_write_locks: crate::work_root_files::DocumentWriteLocks::default(),
        linked_server_sessions: crate::servers::LinkedServerSessions::default(),
        linked_server_tunnels: crate::servers::LinkedServerTunnels::default(),
        registry_persist_lock: Arc::new(Mutex::new(())),
    });
    let (shutdown_tx, shutdown_rx) = watch::channel(false);
    let shutdown_task = tokio::spawn(async move {
        shutdown.await;
        let _ = shutdown_tx.send(true);
    });
    let server = axum::serve(listener, app)
        .with_graceful_shutdown(wait_for_shutdown(shutdown_rx.clone()))
        .into_future();
    tokio::pin!(server);

    tokio::select! {
        result = &mut server => {
            shutdown_task.abort();
            result?;
        }
        () = force_after_shutdown(shutdown_rx, grace_period) => {
            shutdown_task.abort();
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
