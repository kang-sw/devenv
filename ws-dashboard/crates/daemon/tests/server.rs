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
use ws_dashboard_daemon::server::startup_info;

#[test]
#[ignore = "Phase 1 skeleton scaffold: config normalization is implemented by the next pass"]
fn default_serving_config_binds_to_loopback() {
    let config = ServeConfig::default_loopback();

    assert_eq!(config.bind_addr.ip(), Ipv4Addr::LOCALHOST);
}

#[test]
#[ignore = "Phase 1 skeleton scaffold: startup URL construction is implemented by the next pass"]
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
#[ignore = "Phase 1 skeleton scaffold: graceful shutdown harness is implemented by the next pass"]
async fn shutdown_hook_can_terminate_server_task() {
    // Intentionally held as an integration-test target without binding sockets
    // until the server runner exposes a test shutdown hook.
    let _config = ServeConfig::default_loopback();
}
