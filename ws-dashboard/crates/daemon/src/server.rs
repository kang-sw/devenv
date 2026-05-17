use std::future::Future;
use std::net::{IpAddr, SocketAddr};

use tokio::net::TcpListener;
use tracing::info;

use crate::auth::OwnerAuthState;
use crate::config::ServeConfig;
use crate::router::{build_router, AppState};
use crate::terminal::TerminalRegistry;
use crate::work_root_activity::WorkRootActivityProjector;
use crate::work_root_files::OpenedWorkRoots;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StartupInfo {
    pub bound_addr: SocketAddr,
    pub pairing_url: String,
}

pub async fn run(config: ServeConfig) -> anyhow::Result<()> {
    run_with_shutdown(config, shutdown_signal())
        .await
        .map(|_| ())
}

pub async fn run_with_shutdown<F>(config: ServeConfig, shutdown: F) -> anyhow::Result<StartupInfo>
where
    F: Future<Output = ()> + Send + 'static,
{
    // CONTRACT: Server startup prints or otherwise exposes exactly one owner
    // pairing URL once the bound address is known.
    // CONTRACT: Graceful shutdown is part of the daemon shell; request logging
    // must avoid leaking pairing query strings.
    let auth = OwnerAuthState::new_ephemeral();
    let listener = TcpListener::bind(config.bind_addr).await?;
    let bound_addr = listener.local_addr()?;
    let info = startup_info(bound_addr, &auth);

    eprintln!("ws-dashboard owner pairing URL: {}", info.pairing_url);
    info!(bound_addr = %info.bound_addr, "ws-dashboard daemon listening");

    let app = build_router(AppState {
        config,
        auth,
        opened_work_roots: OpenedWorkRoots::default(),
        terminals: TerminalRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
    });
    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown)
        .await?;

    Ok(info)
}

pub fn startup_info(bound_addr: SocketAddr, auth: &OwnerAuthState) -> StartupInfo {
    StartupInfo {
        bound_addr,
        pairing_url: format!(
            "http://{}/pair?token={}",
            display_addr(bound_addr),
            auth.pairing_token().expose_for_owner_url()
        ),
    }
}

async fn shutdown_signal() {
    if let Err(error) = tokio::signal::ctrl_c().await {
        tracing::warn!(%error, "failed to install ctrl-c shutdown signal");
    }
}

fn display_addr(addr: SocketAddr) -> String {
    match addr.ip() {
        IpAddr::V4(_) => addr.to_string(),
        IpAddr::V6(ip) => format!("[{ip}]:{}", addr.port()),
    }
}
