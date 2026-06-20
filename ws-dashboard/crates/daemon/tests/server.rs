// CONTRACT: Server config and startup smoke tests live here.
//
// Required behavior targets:
// - default serving config binds to `127.0.0.1`.
// - bind-mode CLI vocabulary records local, tunnel, and public reachability
//   intent.
// - public bind attempts require explicit public mode.
// - public bind mode cannot start without owner authentication enabled.
// - startup info builds a local owner pairing URL and remote link passphrase
//   after the listener address is known.
// - shutdown hooks can terminate the server without leaving a background task.

use std::net::{IpAddr, Ipv4Addr, SocketAddr, TcpListener as StdTcpListener};
use std::time::Duration;

use clap::Parser;
use tokio::net::TcpStream;
use tokio::sync::oneshot;
use ws_dashboard_daemon::auth::OwnerAuthState;
use ws_dashboard_daemon::cli::{BindMode, Cli, ServeArgs};
use ws_dashboard_daemon::config::{validate_bind_guard, ServeConfig};
use ws_dashboard_daemon::server::{run_with_shutdown, run_with_shutdown_and_grace, startup_info};

#[test]
fn default_serving_config_binds_to_loopback() {
    let config = ServeConfig::default_loopback();

    assert_eq!(config.bind_addr.ip(), Ipv4Addr::LOCALHOST);
    assert_eq!(config.bind_mode, BindMode::Local);
    assert!(config.owner_auth_enabled);
}

#[test]
fn serve_cli_defaults_to_local_loopback_config() {
    let config = Cli::parse_from(["ws-dashboard", "serve"])
        .into_serve_config()
        .expect("default serve config");

    assert_eq!(config.bind_addr, SocketAddr::from((Ipv4Addr::LOCALHOST, 0)));
    assert_eq!(config.bind_mode, BindMode::Local);
    assert!(config.owner_auth_enabled);
}

#[test]
fn tunnel_mode_without_host_keeps_loopback_binding() {
    let config = Cli::parse_from(["ws-dashboard", "serve", "--bind-mode", "tunnel"])
        .into_serve_config()
        .expect("tunnel mode config");

    assert_eq!(config.bind_addr.ip(), IpAddr::V4(Ipv4Addr::LOCALHOST));
    assert_eq!(config.bind_mode, BindMode::Tunnel);
}

#[test]
fn accidental_public_bind_requires_explicit_public_mode() {
    let err = ServeConfig::from_args(ServeArgs {
        host: "0.0.0.0".to_owned(),
        bind_mode: BindMode::Local,
        no_auth: false,
        port: 0,
        static_dir: None,
    })
    .expect_err("local mode must reject public bind address");

    assert!(err.to_string().contains("--bind-mode public"));
}

#[test]
fn explicit_public_bind_mode_accepts_public_host_with_owner_auth() {
    let config = ServeConfig::from_args(ServeArgs {
        host: "0.0.0.0".to_owned(),
        bind_mode: BindMode::Public,
        no_auth: false,
        port: 0,
        static_dir: None,
    })
    .expect("public mode with owner auth");

    assert_eq!(config.bind_addr.ip(), IpAddr::V4(Ipv4Addr::UNSPECIFIED));
    assert_eq!(config.bind_mode, BindMode::Public);
    assert!(config.owner_auth_enabled);
}

#[test]
fn loopback_no_auth_sets_disabled_owner_auth() {
    let config = Cli::parse_from(["ws-dashboard", "serve", "--no-auth", "--host", "127.0.0.1"])
        .into_serve_config()
        .expect("loopback no-auth config");

    assert_eq!(config.bind_addr.ip(), IpAddr::V4(Ipv4Addr::LOCALHOST));
    assert_eq!(config.bind_mode, BindMode::Local);
    assert!(!config.owner_auth_enabled);
}

#[test]
fn public_bind_mode_requires_owner_auth() {
    let err = validate_bind_guard(BindMode::Public, IpAddr::V4(Ipv4Addr::UNSPECIFIED), false)
        .expect_err("public bind mode without owner auth");

    assert!(err.to_string().contains("--bind-mode public"));
}

#[test]
fn no_auth_rejects_public_bind_mode() {
    let err = Cli::parse_from([
        "ws-dashboard",
        "serve",
        "--no-auth",
        "--bind-mode",
        "public",
    ])
    .into_serve_config()
    .expect_err("no-auth public mode");

    assert!(err.to_string().contains("--bind-mode public"));
}

#[test]
fn no_auth_rejects_non_loopback_hosts() {
    let err = Cli::parse_from(["ws-dashboard", "serve", "--no-auth", "--host", "0.0.0.0"])
        .into_serve_config()
        .expect_err("no-auth non-loopback host");

    assert!(err.to_string().contains("loopback bind address"));
}

#[test]
fn startup_info_builds_local_pairing_url_and_remote_link_passphrase() {
    let auth = OwnerAuthState::new_ephemeral();
    let addr = SocketAddr::from((Ipv4Addr::LOCALHOST, 0));

    let info = startup_info(addr, &auth, true);

    assert_eq!(info.bound_addr, addr);
    assert!(info.owner_auth_enabled);
    assert!(info.pairing_url.starts_with("http://127.0.0.1:"));
    assert!(info
        .pairing_url
        .contains(auth.pairing_token().expose_for_owner_url()));
    assert_eq!(info.direct_dashboard_url, None);
    assert_eq!(
        info.link_passphrase,
        auth.link_passphrase().expose_for_owner_record()
    );
}

#[test]
fn startup_info_reports_direct_dashboard_url_for_no_auth() {
    let auth = OwnerAuthState::new_ephemeral();
    let addr = SocketAddr::from((Ipv4Addr::LOCALHOST, 4387));

    let info = startup_info(addr, &auth, false);

    assert!(!info.owner_auth_enabled);
    assert_eq!(
        info.direct_dashboard_url.as_deref(),
        Some("http://127.0.0.1:4387/")
    );
    assert!(!info
        .direct_dashboard_url
        .as_deref()
        .expect("direct dashboard URL")
        .contains(auth.pairing_token().expose_for_owner_url()));
}

#[tokio::test]
async fn shutdown_hook_can_terminate_server_task() {
    let info = run_with_shutdown(ServeConfig::default_loopback(), async {})
        .await
        .expect("server exits after test shutdown hook");

    assert_eq!(info.bound_addr.ip(), Ipv4Addr::LOCALHOST);
}

#[tokio::test]
async fn shutdown_grace_period_bounds_open_idle_connections() {
    let addr = unused_loopback_addr();
    let mut config = ServeConfig::default_loopback();
    config.bind_addr = addr;
    let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();
    let server = tokio::spawn(run_with_shutdown_and_grace(
        config,
        async move {
            let _ = shutdown_rx.await;
        },
        Duration::from_millis(25),
    ));
    let idle_connection = connect_with_retry(addr).await;

    shutdown_tx.send(()).expect("send shutdown");
    let info = tokio::time::timeout(Duration::from_secs(1), server)
        .await
        .expect("server shutdown should be bounded by grace period")
        .expect("server task should not panic")
        .expect("server exits after bounded shutdown");

    assert_eq!(info.bound_addr, addr);
    drop(idle_connection);
}

#[tokio::test]
async fn daemon_security_smoke_covers_loopback_startup_and_public_guards() {
    let info = run_with_shutdown(ServeConfig::default_loopback(), async {})
        .await
        .expect("loopback startup succeeds");
    assert_eq!(info.bound_addr.ip(), Ipv4Addr::LOCALHOST);

    let accidental_public = ServeConfig::from_args(ServeArgs {
        host: "0.0.0.0".to_owned(),
        bind_mode: BindMode::Local,
        no_auth: false,
        port: 0,
        static_dir: None,
    })
    .expect_err("accidental public bind");
    assert!(accidental_public.to_string().contains("--bind-mode public"));

    let disabled_owner_auth =
        validate_bind_guard(BindMode::Public, IpAddr::V4(Ipv4Addr::UNSPECIFIED), false)
            .expect_err("public bind without owner auth");
    assert!(disabled_owner_auth
        .to_string()
        .contains("--bind-mode public"));
}

fn unused_loopback_addr() -> SocketAddr {
    let listener =
        StdTcpListener::bind(SocketAddr::from((Ipv4Addr::LOCALHOST, 0))).expect("reserve port");
    listener.local_addr().expect("reserved port addr")
}

async fn connect_with_retry(addr: SocketAddr) -> TcpStream {
    let mut last_error = None;
    for _ in 0..40 {
        match TcpStream::connect(addr).await {
            Ok(stream) => return stream,
            Err(error) => {
                last_error = Some(error);
                tokio::time::sleep(Duration::from_millis(25)).await;
            }
        }
    }

    panic!(
        "server did not accept connections at {addr}: {:?}",
        last_error
    );
}
