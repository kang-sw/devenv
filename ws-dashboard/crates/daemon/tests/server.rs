// CONTRACT: Server smoke tests for Phase 1 live here.
//
// Required behavior targets:
// - default serving config binds to `127.0.0.1`.
// - startup info builds a local owner pairing URL after the listener address is
//   known.
// - shutdown hooks can terminate the server without leaving a background task.

use std::net::{Ipv4Addr, SocketAddr};

use ws_dashboard_daemon::auth::OwnerAuthState;
use ws_dashboard_daemon::config::ServeConfig;
use ws_dashboard_daemon::server::{run_with_shutdown, startup_info};

#[test]
fn default_serving_config_binds_to_loopback() {
    let config = ServeConfig::default_loopback();

    assert_eq!(config.bind_addr.ip(), Ipv4Addr::LOCALHOST);
}

#[test]
fn startup_info_builds_local_pairing_url() {
    let auth = OwnerAuthState::new_ephemeral();
    let addr = SocketAddr::from((Ipv4Addr::LOCALHOST, 0));

    let info = startup_info(addr, &auth);

    assert_eq!(info.bound_addr, addr);
    assert!(info.pairing_url.starts_with("http://127.0.0.1:"));
    assert!(info
        .pairing_url
        .contains(auth.pairing_token().expose_for_owner_url()));
}

#[tokio::test]
async fn shutdown_hook_can_terminate_server_task() {
    let info = run_with_shutdown(ServeConfig::default_loopback(), async {})
        .await
        .expect("server exits after test shutdown hook");

    assert_eq!(info.bound_addr.ip(), Ipv4Addr::LOCALHOST);
}
