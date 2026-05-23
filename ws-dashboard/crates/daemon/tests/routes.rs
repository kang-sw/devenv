// CONTRACT: Router smoke tests for Phase 1 live here.
// HINT: Use `tower::ServiceExt` against `router::build_router` rather than
// binding sockets.
//
// Required behavior targets:
// - `/pair` is reachable without an existing owner session.
// - `/healthz` rejects before pairing.
// - valid pairing installs an HTTP-only owner session cookie.
// - invalid, reused, missing, and expired pairing tokens do not install a
//   session.
// - `/healthz` and `/` succeed with the owner session cookie.
// - browser Host/Origin checks reject clearly invalid entrypoints without
//   weakening ordinary loopback usage.
// - narrow bearer auth supports CLI/smoke callers without replacing cookies.
// - future WebSocket upgrade paths reject unauthenticated requests before
//   endpoint behavior is considered.
// - health output stays minimal and does not expose token, host paths, cache
//   paths, Git roots, wsstate internals, or diagnostics.
// - `/api/dashboard/resources` is protected and returns the live opened-workRoot
//   resource view-model contract that frontend work consumes.

use std::fs;
#[cfg(unix)]
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;

use futures_util::{SinkExt, StreamExt};

use axum::body::Body;
use axum::http::{header, Method, Request, StatusCode};
use tokio::net::TcpListener;
use tokio::sync::Mutex;
use tokio::time::timeout;
use tokio_tungstenite::tungstenite::{client::IntoClientRequest, Message as TungsteniteMessage};
use tower::ServiceExt;
use ws_dashboard_daemon::auth::{OwnerAuthState, PairingTokenPolicy};
use ws_dashboard_daemon::config::ServeConfig;
use ws_dashboard_daemon::persistent_state::DashboardStateStore;
use ws_dashboard_daemon::router::{build_router, AppState};
use ws_dashboard_daemon::terminal::TerminalRegistry;
use ws_dashboard_daemon::work_root_activity::{
    resolve_work_root_agents_dir, WorkRootActivityProjectionConfig, WorkRootActivityProjector,
};
use ws_dashboard_daemon::work_root_files::OpenedWorkRoots;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum TestShellProfile {
    UnixSh,
    CmdExe,
    PowerShell,
}

struct TerminalTestCommands {
    echo_and_exit: String,
    exit: String,
}

fn posix_single_quote(value: &str) -> String {
    format!("'{}'", value.replace('\'', "'\\''"))
}

fn cmd_escape(value: &str) -> String {
    value
        .chars()
        .flat_map(|ch| match ch {
            '&' | '<' | '>' | '|' | '^' | '%' => vec!['^', ch],
            _ => vec![ch],
        })
        .collect()
}

fn terminal_test_commands(profile: TestShellProfile, marker: &str) -> TerminalTestCommands {
    // CONTRACT: Backend terminal route tests must not embed POSIX command
    // strings in shared behavior checks. This helper maps observable terminal
    // intent to Unix shell, cmd.exe, and PowerShell command syntax.
    match profile {
        TestShellProfile::UnixSh => TerminalTestCommands {
            echo_and_exit: format!("printf '%s\\n' {}\nexit\n", posix_single_quote(marker)),
            exit: "exit\n".to_owned(),
        },
        TestShellProfile::CmdExe => TerminalTestCommands {
            echo_and_exit: format!("echo {}\r\nexit\r\n", cmd_escape(marker)),
            exit: "exit\r\n".to_owned(),
        },
        TestShellProfile::PowerShell => TerminalTestCommands {
            echo_and_exit: format!("Write-Output '{}'\r\nexit\r\n", marker.replace('\'', "''")),
            exit: "exit\r\n".to_owned(),
        },
    }
}

fn terminal_test_commands_for_current_platform(marker: &str) -> TerminalTestCommands {
    // CONTRACT: Current-platform tests should use the same helper as explicit
    // profile tests so native Windows evidence exercises cmd.exe/PowerShell
    // syntax instead of POSIX-only commands.
    let profile = if cfg!(windows) {
        TestShellProfile::CmdExe
    } else {
        TestShellProfile::UnixSh
    };
    terminal_test_commands(profile, marker)
}

#[test]
fn terminal_test_command_profiles_have_exit_sequences() {
    for profile in [
        TestShellProfile::UnixSh,
        TestShellProfile::CmdExe,
        TestShellProfile::PowerShell,
    ] {
        let commands = terminal_test_commands(profile, "PORTABLE-MARKER");
        assert!(commands.echo_and_exit.contains("PORTABLE-MARKER"));
        assert!(commands.echo_and_exit.contains("exit"));
        assert!(commands.exit.contains("exit"));
    }
    let commands = terminal_test_commands_for_current_platform("CURRENT-MARKER");
    assert!(commands.echo_and_exit.contains("CURRENT-MARKER"));
}

static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

fn app_state() -> AppState {
    app_state_with_opened_and_store(OpenedWorkRoots::default(), DashboardStateStore::disabled())
}

fn app_state_with_opened_and_store(
    opened_work_roots: OpenedWorkRoots,
    dashboard_state: DashboardStateStore,
) -> AppState {
    AppState {
        config: ServeConfig::default_loopback(),
        auth: OwnerAuthState::new_ephemeral(),
        opened_work_roots,
        dashboard_state,
        terminals: TerminalRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
        registry_persist_lock: Arc::new(Mutex::new(())),
    }
}

fn app_state_with_static_dir(static_dir: PathBuf) -> AppState {
    AppState {
        config: ServeConfig {
            static_dir: Some(static_dir),
            ..ServeConfig::default_loopback()
        },
        auth: OwnerAuthState::new_ephemeral(),
        opened_work_roots: OpenedWorkRoots::default(),
        dashboard_state: DashboardStateStore::disabled(),
        terminals: TerminalRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
        registry_persist_lock: Arc::new(Mutex::new(())),
    }
}

fn write_static_fixture() -> PathBuf {
    let root = temp_fixture_path("static");
    fs::create_dir_all(root.join("assets")).expect("create static fixture assets dir");
    fs::write(
        root.join("index.html"),
        "<!doctype html><title>fixture dashboard</title><div id=\"root\"></div>",
    )
    .expect("write static fixture index");
    fs::write(
        root.join("assets/app.js"),
        "console.log('fixture dashboard');",
    )
    .expect("write static fixture asset");
    root
}

fn write_root_picker_fixture() -> PathBuf {
    let root = temp_fixture_path("picker");
    fs::create_dir_all(root.join("zeta")).expect("create zeta dir");
    fs::create_dir_all(root.join("alpha")).expect("create alpha dir");
    fs::write(root.join("not-a-directory.txt"), "ignored\n").expect("write ignored file");
    root
}

fn temp_fixture_path(name: &str) -> PathBuf {
    let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    std::env::temp_dir().join(format!(
        "ws-dashboard-{name}-{}-{unique}",
        std::process::id()
    ))
}

fn remove_static_fixture(path: &Path) {
    let _ = fs::remove_dir_all(path);
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
    assert_eq!(
        response.headers().get(header::LOCATION),
        Some(&header::HeaderValue::from_static("/"))
    );
    assert!(!response
        .headers()
        .get(header::LOCATION)
        .expect("pair redirect location")
        .to_str()
        .expect("location header is ASCII")
        .contains(token));
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

async fn pair_response(app: axum::Router, token: Option<&str>) -> axum::response::Response {
    let uri = token
        .map(|token| format!("/pair?token={token}"))
        .unwrap_or_else(|| "/pair".to_owned());

    app.oneshot(
        Request::builder()
            .uri(uri)
            .body(Body::empty())
            .expect("pair request"),
    )
    .await
    .expect("pair response")
}

fn assert_pair_failure_does_not_install_cookie_or_redirect_to_app(
    response: &axum::response::Response,
) {
    assert!(response.headers().get(header::SET_COOKIE).is_none());
    assert!(response.headers().get(header::LOCATION).is_none());
}

#[tokio::test]
async fn pair_is_the_only_unauthenticated_browser_route() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/pair")
                .body(Body::empty())
                .expect("pair request"),
        )
        .await
        .expect("pair response");

    assert_ne!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn non_pair_browser_routes_reject_before_pairing() {
    let app = build_router(app_state());

    for uri in ["/healthz", "/", "/favicon.ico"] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .body(Body::empty())
                    .expect("unauthenticated request"),
            )
            .await
            .expect("unauthenticated response");

        assert_eq!(response.status(), StatusCode::UNAUTHORIZED, "{uri}");
    }
}

#[tokio::test]
async fn valid_pairing_installs_http_only_owner_session_cookie_once() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!("/pair?token={token}"))
                .body(Body::empty())
                .expect("pair request"),
        )
        .await
        .expect("pair response");

    let set_cookie = response
        .headers()
        .get(header::SET_COOKIE)
        .expect("owner session cookie");
    let set_cookie = set_cookie.to_str().expect("cookie header is ASCII");
    assert_eq!(response.status(), StatusCode::SEE_OTHER);
    assert_eq!(
        response.headers().get(header::LOCATION),
        Some(&header::HeaderValue::from_static("/"))
    );
    assert!(!response
        .headers()
        .get(header::LOCATION)
        .expect("pair redirect location")
        .to_str()
        .expect("location header is ASCII")
        .contains(&token));
    assert!(set_cookie.contains("ws-dashboard-owner="));
    assert!(set_cookie.contains("HttpOnly"));

    let reused = app
        .oneshot(
            Request::builder()
                .uri(format!("/pair?token={token}"))
                .body(Body::empty())
                .expect("reused pair request"),
        )
        .await
        .expect("reused pair response");

    assert_eq!(reused.status(), StatusCode::GONE);
    assert!(reused.headers().get(header::SET_COOKIE).is_none());
}

#[tokio::test]
async fn invalid_missing_and_reused_pairing_tokens_do_not_install_sessions() {
    let missing = pair_response(build_router(app_state()), None).await;
    assert_eq!(missing.status(), StatusCode::BAD_REQUEST);
    assert_pair_failure_does_not_install_cookie_or_redirect_to_app(&missing);

    let invalid = pair_response(build_router(app_state()), Some("not-the-token")).await;
    assert_eq!(invalid.status(), StatusCode::UNAUTHORIZED);
    assert_pair_failure_does_not_install_cookie_or_redirect_to_app(&invalid);

    let reused_state = app_state();
    let token = reused_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let reused_app = build_router(reused_state);
    let first = pair_response(reused_app.clone(), Some(&token)).await;
    assert_eq!(first.status(), StatusCode::SEE_OTHER);
    let reused = pair_response(reused_app, Some(&token)).await;
    assert_eq!(reused.status(), StatusCode::GONE);
    assert_pair_failure_does_not_install_cookie_or_redirect_to_app(&reused);
}

#[tokio::test]
async fn expired_pairing_tokens_do_not_install_sessions() {
    // CONTRACT: A zero TTL is the deterministic expired-token fixture.
    let expired_state = AppState {
        config: ServeConfig::default_loopback(),
        auth: OwnerAuthState::new_ephemeral_with_policy(PairingTokenPolicy::new(Duration::ZERO)),
        opened_work_roots: OpenedWorkRoots::default(),
        dashboard_state: DashboardStateStore::disabled(),
        terminals: TerminalRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
        registry_persist_lock: Arc::new(Mutex::new(())),
    };
    let expired_token = expired_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let expired = pair_response(build_router(expired_state), Some(&expired_token)).await;
    assert_eq!(expired.status(), StatusCode::UNAUTHORIZED);
    assert_pair_failure_does_not_install_cookie_or_redirect_to_app(&expired);
}

#[tokio::test]
async fn health_and_static_ui_succeed_with_owner_session_cookie() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    for uri in ["/healthz", "/"] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .body(Body::empty())
                    .expect("authenticated request"),
            )
            .await
            .expect("authenticated response");

        assert_eq!(response.status(), StatusCode::OK);
    }
}

#[tokio::test]
async fn static_dashboard_assets_stay_owner_authenticated() {
    let static_dir = write_static_fixture();
    let app = build_router(app_state_with_static_dir(static_dir.clone()));

    for uri in ["/", "/assets/app.js"] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .body(Body::empty())
                    .expect("unauthenticated static request"),
            )
            .await
            .expect("unauthenticated static response");

        assert_eq!(response.status(), StatusCode::UNAUTHORIZED, "{uri}");
    }

    remove_static_fixture(&static_dir);
}

#[tokio::test]
async fn static_dashboard_assets_succeed_with_owner_session_cookie() {
    let static_dir = write_static_fixture();
    let state = app_state_with_static_dir(static_dir.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let index = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("authenticated static index request"),
        )
        .await
        .expect("authenticated static index response");
    assert_eq!(index.status(), StatusCode::OK);
    let index_body = axum::body::to_bytes(index.into_body(), 4096)
        .await
        .expect("static index body bytes");
    let index_body = std::str::from_utf8(&index_body).expect("static index body utf8");
    assert!(index_body.contains("fixture dashboard"));

    let asset = app
        .oneshot(
            Request::builder()
                .uri("/assets/app.js")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("authenticated static asset request"),
        )
        .await
        .expect("authenticated static asset response");
    assert_eq!(asset.status(), StatusCode::OK);
    assert_eq!(
        asset.headers().get(header::CONTENT_TYPE),
        Some(&header::HeaderValue::from_static(
            "application/javascript; charset=utf-8"
        ))
    );
    let asset_body = axum::body::to_bytes(asset.into_body(), 4096)
        .await
        .expect("static asset body bytes");
    let asset_body = std::str::from_utf8(&asset_body).expect("static asset body utf8");
    assert!(asset_body.contains("fixture dashboard"));

    remove_static_fixture(&static_dir);
}

#[tokio::test]
async fn server_scoped_dashboard_routes_refresh_to_protected_shell() {
    let static_dir = write_static_fixture();
    let state = app_state_with_static_dir(static_dir.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    for uri in [
        "/servers",
        "/servers/server-local",
        "/servers/server-local/workspaces/workspace-devenv",
    ] {
        let unauthenticated = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .body(Body::empty())
                    .expect("unauthenticated server-scoped shell request"),
            )
            .await
            .expect("unauthenticated server-scoped shell response");
        assert_eq!(unauthenticated.status(), StatusCode::UNAUTHORIZED, "{uri}");

        let authenticated = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .body(Body::empty())
                    .expect("authenticated server-scoped shell request"),
            )
            .await
            .expect("authenticated server-scoped shell response");
        assert_eq!(authenticated.status(), StatusCode::OK, "{uri}");
        let body = axum::body::to_bytes(authenticated.into_body(), 4096)
            .await
            .expect("server-scoped shell body bytes");
        let body = std::str::from_utf8(&body).expect("server-scoped shell body utf8");
        assert!(body.contains("fixture dashboard"), "{uri}");
    }

    remove_static_fixture(&static_dir);
}

#[tokio::test]
async fn dashboard_resources_api_is_owner_authenticated() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/resources")
                .body(Body::empty())
                .expect("unauthenticated dashboard resources request"),
        )
        .await
        .expect("unauthenticated dashboard resources response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn dashboard_resources_api_returns_empty_live_view_before_open() {
    // CONTRACT: with no workRoot opened the canonical route returns an honest
    // empty live view (server present, `workspaces: []`) and never the static
    // mock fixture workspaces.
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/resources")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("authenticated dashboard resources request"),
        )
        .await
        .expect("authenticated dashboard resources response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("dashboard resources body bytes");
    let value: serde_json::Value =
        serde_json::from_slice(&body).expect("dashboard resources JSON body");

    assert!(value.get("server").is_some());
    assert_eq!(value["server"]["id"], "server-local");
    assert_eq!(value["server"]["state"]["loading"], false);
    assert_eq!(value["server"]["state"]["stale"], false);

    let workspaces = value["workspaces"].as_array().expect("workspaces array");
    assert!(
        workspaces.is_empty(),
        "empty live view exposes no workspaces before any open"
    );
    assert!(
        !body_contains_workspace(&value, "workspace-devenv"),
        "live route must not return the mock fixture workspace"
    );
}

#[tokio::test]
async fn dashboard_resources_api_includes_opened_work_root() {
    // CONTRACT: the brief's required 1->2->3 sequence. After an owner opens a
    // real workRoot, GET /api/dashboard/resources includes that opened workRoot
    // and not only the static mock fixture workspace.
    let root = temp_fixture_path("live-resources");
    fs::create_dir_all(&root).expect("create live resources workRoot");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/resources")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("authenticated dashboard resources request"),
        )
        .await
        .expect("authenticated dashboard resources response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("dashboard resources body bytes");
    let value: serde_json::Value =
        serde_json::from_slice(&body).expect("dashboard resources JSON body");

    assert_eq!(value["server"]["id"], "server-local");
    assert!(
        work_root_ids(&value).iter().any(|id| id == &work_root_id),
        "opened workRoot {work_root_id} must appear in the live resources view"
    );
    assert!(
        !body_contains_workspace(&value, "workspace-devenv"),
        "live route must not return the mock fixture workspace"
    );

    // CONTRACT: workRoots stay camelCase in the serialized view-model.
    let workspace = &value["workspaces"][0];
    assert!(
        workspace.get("workRoots").is_some(),
        "workspace exposes camelCase workRoots"
    );
    assert!(
        workspace.get("work_roots").is_none(),
        "workspace must not leak snake_case work_roots"
    );

    remove_static_fixture(&root);
}

#[tokio::test]
async fn work_root_registry_activation_controls_keep_offline_roots_visible_and_gate_routes() {
    let root = temp_fixture_path("activation-gate");
    fs::create_dir_all(&root).expect("create activation root");
    fs::write(root.join("README.md"), "hello\n").expect("write activation file");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;
    let terminal_id = create_terminal_for_test(app.clone(), cookie.as_str(), &work_root_id).await;
    let (open_socket_addr, open_socket_server) = spawn_test_server(app.clone()).await;
    let mut open_socket_request =
        format!("ws://{open_socket_addr}/api/dashboard/terminals/{terminal_id}/socket")
            .into_client_request()
            .expect("pre-offline websocket request");
    open_socket_request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let (mut open_socket, _) = tokio_tungstenite::connect_async(open_socket_request)
        .await
        .expect("pre-offline websocket connects");

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/activation",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "activation": "offline" }).to_string(),
                ))
                .expect("offline activation request"),
        )
        .await
        .expect("offline activation response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("offline activation body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("activation JSON");
    let root_value = &value["workspaces"][0]["workRoots"][0];
    assert_eq!(root_value["id"], work_root_id);
    assert_eq!(root_value["availability"], "available");
    assert_eq!(root_value["activation"], "offline");
    assert_eq!(root_value["state"]["status"], "offline");

    for (method, uri, body, expected_error) in [
        (
            Method::GET,
            format!("/api/dashboard/work-roots/{work_root_id}/files"),
            Body::empty(),
            "workRoot offline",
        ),
        (
            Method::GET,
            format!("/api/dashboard/work-roots/{work_root_id}/files/read?path=README.md"),
            Body::empty(),
            "workRoot offline",
        ),
        (
            Method::GET,
            format!("/api/dashboard/work-roots/{work_root_id}/terminals"),
            Body::empty(),
            "workRoot offline",
        ),
        (
            Method::POST,
            format!("/api/dashboard/work-roots/{work_root_id}/terminals"),
            Body::from(serde_json::json!({ "columns": 80, "rows": 24 }).to_string()),
            "workRoot offline",
        ),
        (
            Method::GET,
            format!("/api/dashboard/terminals/{terminal_id}/output"),
            Body::empty(),
            "workRoot offline",
        ),
        (
            Method::POST,
            format!("/api/dashboard/terminals/{terminal_id}/input"),
            Body::from(serde_json::json!({ "data": "echo blocked\n" }).to_string()),
            "workRoot offline",
        ),
        (
            Method::POST,
            format!("/api/dashboard/terminals/{terminal_id}/resize"),
            Body::from(serde_json::json!({ "columns": 100, "rows": 30 }).to_string()),
            "workRoot offline",
        ),
        (
            Method::DELETE,
            format!("/api/dashboard/terminals/{terminal_id}"),
            Body::empty(),
            "workRoot offline",
        ),
        (
            Method::GET,
            format!("/api/dashboard/work-roots/{work_root_id}/activity"),
            Body::empty(),
            "workRoot offline",
        ),
        (
            Method::GET,
            format!(
                "/api/dashboard/work-roots/{work_root_id}/activity/items/agent:test/transcript"
            ),
            Body::empty(),
            "workRoot offline",
        ),
        (
            Method::GET,
            format!("/api/dashboard/work-roots/{work_root_id}/activity/events"),
            Body::empty(),
            "workRoot offline",
        ),
    ] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method(method)
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(body)
                    .expect("offline gated request"),
            )
            .await
            .expect("offline gated response");
        assert_eq!(response.status(), StatusCode::CONFLICT);
        let body = axum::body::to_bytes(response.into_body(), 4096)
            .await
            .expect("offline error body");
        let value: serde_json::Value = serde_json::from_slice(&body).expect("offline error JSON");
        assert_eq!(value["error"], expected_error);
        assert!(!String::from_utf8_lossy(&body).contains(root.to_string_lossy().as_ref()));
    }

    open_socket
        .send(TungsteniteMessage::Text(
            serde_json::json!({
                "type": "input",
                "data": "echo WS-OFFLINE-BYPASS\n"
            })
            .to_string()
            .into(),
        ))
        .await
        .expect("send offline websocket input frame");
    let mut socket_closed = false;
    for _ in 0..4 {
        match timeout(Duration::from_secs(2), open_socket.next()).await {
            Ok(Some(Ok(TungsteniteMessage::Close(_)))) | Ok(None) => {
                socket_closed = true;
                break;
            }
            Ok(Some(Ok(_))) => continue,
            Ok(Some(Err(_))) | Err(_) => {
                socket_closed = true;
                break;
            }
        }
    }
    assert!(socket_closed, "offline websocket closes after client input");
    open_socket_server.abort();

    let (addr, server) = spawn_test_server(app.clone()).await;
    let mut websocket_request = format!("ws://{addr}/api/dashboard/terminals/{terminal_id}/socket")
        .into_client_request()
        .expect("offline websocket request");
    websocket_request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let error = tokio_tungstenite::connect_async(websocket_request)
        .await
        .expect_err("offline websocket rejects");
    match error {
        tokio_tungstenite::tungstenite::Error::Http(response) => {
            assert_eq!(response.status(), StatusCode::CONFLICT);
        }
        other => panic!("unexpected offline websocket error: {other}"),
    }
    server.abort();

    let online_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/activation",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "activation": "online" }).to_string(),
                ))
                .expect("online activation request"),
        )
        .await
        .expect("online activation response");
    assert_eq!(online_response.status(), StatusCode::OK);
    let close_response = app
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!("/api/dashboard/terminals/{terminal_id}"))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("terminal cleanup request"),
        )
        .await
        .expect("terminal cleanup response");
    assert_eq!(close_response.status(), StatusCode::NO_CONTENT);

    remove_static_fixture(&root);
}

#[tokio::test]
async fn online_missing_work_root_returns_bounded_unavailable_without_path_leak() {
    let root = temp_fixture_path("activation-missing");
    fs::create_dir_all(&root).expect("create missing root");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;
    let terminal_id = create_terminal_for_test(app.clone(), cookie.as_str(), &work_root_id).await;
    fs::remove_dir_all(&root).expect("remove root after registry membership");

    let resources = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/resources")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("resources request"),
        )
        .await
        .expect("resources response");
    assert_eq!(resources.status(), StatusCode::OK);
    let resources_body = axum::body::to_bytes(resources.into_body(), 64 * 1024)
        .await
        .expect("resources body");
    let value: serde_json::Value = serde_json::from_slice(&resources_body).expect("resources JSON");
    assert_eq!(value["workspaces"][0]["workRoots"][0]["id"], work_root_id);
    assert_eq!(
        value["workspaces"][0]["workRoots"][0]["activation"],
        "online"
    );
    assert!(["missing", "moved"].contains(
        &value["workspaces"][0]["workRoots"][0]["availability"]
            .as_str()
            .expect("availability")
    ));

    let response = app.clone();
    for uri in [
        format!("/api/dashboard/work-roots/{work_root_id}/files"),
        format!("/api/dashboard/work-roots/{work_root_id}/files/read?path=README.md"),
        format!("/api/dashboard/work-roots/{work_root_id}/activity"),
        format!("/api/dashboard/work-roots/{work_root_id}/activity/items/agent:test/transcript"),
        format!("/api/dashboard/work-roots/{work_root_id}/activity/events"),
        format!("/api/dashboard/work-roots/{work_root_id}/terminals"),
        format!("/api/dashboard/terminals/{terminal_id}/output"),
    ] {
        let response = response
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .body(Body::empty())
                    .expect("missing workRoot gated request"),
            )
            .await
            .expect("missing workRoot gated response");

        assert_eq!(response.status(), StatusCode::CONFLICT);
        let body = axum::body::to_bytes(response.into_body(), 4096)
            .await
            .expect("unavailable body");
        let value: serde_json::Value = serde_json::from_slice(&body).expect("unavailable JSON");
        assert_eq!(value["error"], "workRoot unavailable");
        assert!(!String::from_utf8_lossy(&body).contains(root.to_string_lossy().as_ref()));
    }

    for (uri, body) in [
        (
            format!("/api/dashboard/work-roots/{work_root_id}/terminals"),
            Body::from(serde_json::json!({ "columns": 80, "rows": 24 }).to_string()),
        ),
        (
            format!("/api/dashboard/terminals/{terminal_id}/input"),
            Body::from(serde_json::json!({ "data": "echo blocked\n" }).to_string()),
        ),
        (
            format!("/api/dashboard/terminals/{terminal_id}/resize"),
            Body::from(serde_json::json!({ "columns": 100, "rows": 30 }).to_string()),
        ),
    ] {
        let response = response
            .clone()
            .oneshot(
                Request::builder()
                    .method(Method::POST)
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(body)
                    .expect("missing workRoot terminal POST request"),
            )
            .await
            .expect("missing workRoot terminal POST response");
        assert_eq!(response.status(), StatusCode::CONFLICT);
        let body = axum::body::to_bytes(response.into_body(), 4096)
            .await
            .expect("terminal unavailable body");
        let value: serde_json::Value =
            serde_json::from_slice(&body).expect("terminal unavailable JSON");
        assert_eq!(value["error"], "workRoot unavailable");
        assert!(!String::from_utf8_lossy(&body).contains(root.to_string_lossy().as_ref()));
    }

    let delete_response = response
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!("/api/dashboard/terminals/{terminal_id}"))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("missing workRoot terminal delete request"),
        )
        .await
        .expect("missing workRoot terminal delete response");
    assert_eq!(delete_response.status(), StatusCode::CONFLICT);
    let delete_body = axum::body::to_bytes(delete_response.into_body(), 4096)
        .await
        .expect("delete unavailable body");
    let value: serde_json::Value =
        serde_json::from_slice(&delete_body).expect("delete unavailable JSON");
    assert_eq!(value["error"], "workRoot unavailable");
    assert!(!String::from_utf8_lossy(&delete_body).contains(root.to_string_lossy().as_ref()));

    let (addr, server) = spawn_test_server(response.clone()).await;
    let mut websocket_request = format!("ws://{addr}/api/dashboard/terminals/{terminal_id}/socket")
        .into_client_request()
        .expect("unavailable websocket request");
    websocket_request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let error = tokio_tungstenite::connect_async(websocket_request)
        .await
        .expect_err("unavailable websocket rejects");
    match error {
        tokio_tungstenite::tungstenite::Error::Http(response) => {
            assert_eq!(response.status(), StatusCode::CONFLICT);
        }
        other => panic!("unexpected unavailable websocket error: {other}"),
    }
    server.abort();
    fs::create_dir_all(&root).expect("restore root for terminal cleanup");
    let cleanup = response
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!("/api/dashboard/terminals/{terminal_id}"))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("terminal cleanup request"),
        )
        .await
        .expect("terminal cleanup response");
    assert_eq!(cleanup.status(), StatusCode::NO_CONTENT);
    remove_static_fixture(&root);
}

#[tokio::test]
async fn open_work_root_header_identifies_requested_root_with_ambiguous_labels() {
    let base = temp_fixture_path("ambiguous-open");
    let first = base.join("first").join("same-name");
    let second = base.join("second").join("same-name");
    fs::create_dir_all(&first).expect("create first same-name root");
    fs::create_dir_all(&second).expect("create second same-name root");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let first_id = open_work_root_for_test(app.clone(), cookie.as_str(), &first).await;
    let expected_second_id = ws_dashboard_daemon::discovery::local_work_root_id_for_path(&second);

    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/open")
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "path": second.display().to_string() }).to_string(),
                ))
                .expect("open second same-name root request"),
        )
        .await
        .expect("open second same-name root response");
    assert_eq!(response.status(), StatusCode::OK);
    assert_eq!(
        response
            .headers()
            .get("x-ws-dashboard-opened-work-root-id")
            .and_then(|value| value.to_str().ok()),
        Some(expected_second_id.as_str())
    );
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("open second body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("open second JSON");
    let ids = work_root_ids(&value);
    assert!(ids.contains(&first_id));
    assert!(ids.iter().any(|id| id == expected_second_id.as_str()));

    remove_static_fixture(&base);
}

#[tokio::test]
async fn work_root_activation_rolls_back_when_registry_persist_fails() {
    let root = temp_fixture_path("activation-persist-fails");
    fs::create_dir_all(&root).expect("create activation root");
    let state_file_directory = temp_fixture_path("activation-state-dir");
    fs::create_dir_all(&state_file_directory).expect("create state-file directory");
    let opened = OpenedWorkRoots::from_paths(vec![root.clone()]);
    let work_root_id = ws_dashboard_daemon::discovery::local_work_root_id_for_path(&root);
    let state = app_state_with_opened_and_store(
        opened,
        DashboardStateStore::at_path(&state_file_directory),
    );
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/activation",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "activation": "offline" }).to_string(),
                ))
                .expect("activation request"),
        )
        .await
        .expect("activation response");

    assert_eq!(response.status(), StatusCode::INTERNAL_SERVER_ERROR);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("persist error body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("persist error JSON");
    assert_eq!(value["error"], "persist activation failed");

    let resources = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/resources")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("resources request"),
        )
        .await
        .expect("resources response");
    assert_eq!(resources.status(), StatusCode::OK);
    let body = axum::body::to_bytes(resources.into_body(), 64 * 1024)
        .await
        .expect("resources body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("resources JSON");
    assert_eq!(
        value["workspaces"][0]["workRoots"][0]["activation"],
        "online"
    );

    remove_static_fixture(&root);
    remove_static_fixture(&state_file_directory);
}

#[tokio::test]
async fn open_work_root_does_not_advertise_id_when_registry_persist_fails() {
    let root = temp_fixture_path("open-persist-fails");
    fs::create_dir_all(&root).expect("create open root");
    let state_file_directory = temp_fixture_path("open-state-dir");
    fs::create_dir_all(&state_file_directory).expect("create state-file directory");
    let state = app_state_with_opened_and_store(
        OpenedWorkRoots::default(),
        DashboardStateStore::at_path(&state_file_directory),
    );
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/open")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "path": root.display().to_string() }).to_string(),
                ))
                .expect("open workRoot request"),
        )
        .await
        .expect("open workRoot response");

    assert_eq!(response.status(), StatusCode::INTERNAL_SERVER_ERROR);
    assert!(response
        .headers()
        .get("x-ws-dashboard-opened-work-root-id")
        .is_none());
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("open persist error body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("open persist error JSON");
    assert_eq!(value["error"], "persist workRoot failed");

    let resources = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/resources")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("resources request"),
        )
        .await
        .expect("resources response");
    assert_eq!(resources.status(), StatusCode::OK);
    let body = axum::body::to_bytes(resources.into_body(), 64 * 1024)
        .await
        .expect("resources body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("resources JSON");
    assert!(value["workspaces"]
        .as_array()
        .expect("workspaces array")
        .is_empty());

    remove_static_fixture(&root);
    remove_static_fixture(&state_file_directory);
}

#[tokio::test]
async fn root_picker_routes_are_owner_authenticated() {
    let app = build_router(app_state());

    let requests = [
        Request::builder()
            .uri("/api/dashboard/root-picker")
            .body(Body::empty())
            .expect("root picker request"),
        Request::builder()
            .method(Method::POST)
            .uri("/api/dashboard/root-picker/directories")
            .body(Body::empty())
            .expect("create directory request"),
        Request::builder()
            .method(Method::POST)
            .uri("/api/dashboard/work-roots/open")
            .body(Body::empty())
            .expect("open workRoot request"),
        Request::builder()
            .method(Method::POST)
            .uri("/api/dashboard/work-roots/root-local-test/activation")
            .body(Body::empty())
            .expect("activate workRoot request"),
    ];

    for request in requests {
        let response = app
            .clone()
            .oneshot(request)
            .await
            .expect("unauthenticated root picker response");

        assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
    }
}

#[tokio::test]
async fn root_picker_lists_directory_candidates_with_owner_cookie() {
    let root = write_root_picker_fixture();
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/root-picker?path={}",
                    root.display()
                ))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("authenticated root picker request"),
        )
        .await
        .expect("authenticated root picker response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("root picker body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("root picker JSON");
    assert_eq!(
        value["currentPath"],
        root.canonicalize()
            .expect("canonical root picker path")
            .display()
            .to_string()
    );
    let entries = value["entries"].as_array().expect("entries array");
    assert_eq!(entries.len(), 2);
    assert_eq!(entries[0]["name"], "alpha");
    assert_eq!(entries[0]["entryType"], "directory");
    assert_eq!(entries[0]["selectable"], true);
    assert_eq!(entries[1]["name"], "zeta");

    remove_static_fixture(&root);
}

#[tokio::test]
async fn root_picker_can_create_empty_directory_candidate() {
    let root = write_root_picker_fixture();
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/root-picker/directories")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "parentPath": root.display().to_string(),
                        "name": "new-root"
                    })
                    .to_string(),
                ))
                .expect("create empty directory request"),
        )
        .await
        .expect("create empty directory response");

    assert_eq!(response.status(), StatusCode::OK);
    assert!(root.join("new-root").is_dir());
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("created entry body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("created entry JSON");
    assert_eq!(value["name"], "new-root");
    assert_eq!(value["entryType"], "directory");
    assert_eq!(value["selectable"], true);

    let rejected = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/root-picker/directories")
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "parentPath": root.display().to_string(),
                        "name": "../bad"
                    })
                    .to_string(),
                ))
                .expect("invalid create empty directory request"),
        )
        .await
        .expect("invalid create empty directory response");
    assert_eq!(rejected.status(), StatusCode::BAD_REQUEST);

    remove_static_fixture(&root);
}

#[tokio::test]
async fn root_picker_can_open_existing_directory_into_dashboard_model() {
    let root = write_root_picker_fixture();
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/open")
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "path": root.display().to_string()
                    })
                    .to_string(),
                ))
                .expect("open workRoot request"),
        )
        .await
        .expect("open workRoot response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("open workRoot body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("workRoot JSON");
    assert_eq!(value["server"]["id"], "server-local");
    assert_eq!(
        value["workspaces"][0]["workRoots"][0]["kind"],
        "plainDirectory"
    );
    assert_eq!(value["workspaces"][0]["workRoots"][0]["status"], "online");
    assert_eq!(
        value["workspaces"][0]["workRoots"][0]["actions"][0]["id"],
        "openRoot"
    );

    remove_static_fixture(&root);
}

#[tokio::test]
async fn dashboard_resources_api_includes_remembered_work_root_after_restart_seed() {
    // CONTRACT: daemon startup can seed OpenedWorkRoots from persisted paths,
    // so the canonical resources route shows remembered roots before the owner
    // manually opens a new one in this process.
    let root = temp_fixture_path("remembered-resources");
    let state_file_root = temp_fixture_path("remembered-resources-state");
    fs::create_dir_all(&root).expect("create remembered resources workRoot");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    let opened_before_restart = OpenedWorkRoots::from_paths(vec![root.clone()]);
    store
        .persist_opened_work_roots(&opened_before_restart)
        .await
        .expect("persist remembered workRoot");
    let remembered = store.load_opened_work_roots().await;
    let state = app_state_with_opened_and_store(OpenedWorkRoots::from_paths(remembered), store);
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/resources")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("authenticated remembered resources request"),
        )
        .await
        .expect("authenticated remembered resources response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("remembered resources body bytes");
    let value: serde_json::Value =
        serde_json::from_slice(&body).expect("remembered resources JSON body");

    assert_eq!(
        value["workspaces"][0]["workRoots"][0]["label"],
        root.file_name()
            .and_then(|name| name.to_str())
            .expect("remembered root filename")
    );

    remove_static_fixture(&root);
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn open_work_root_persists_opened_work_root_paths() {
    // CONTRACT: opening a workRoot updates daemon-owned local state after the
    // in-memory registration succeeds.
    let root = temp_fixture_path("persist-open-root");
    let state_file_root = temp_fixture_path("persist-open-root-state");
    fs::create_dir_all(&root).expect("create persisted open workRoot");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    let state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let work_root_id = open_work_root_for_test(app, cookie.as_str(), &root).await;
    let remembered = store.load_opened_work_roots().await;

    assert_eq!(remembered, vec![root.clone()]);
    assert!(
        work_root_id.starts_with("root-local-"),
        "persisted open should return normal workRoot id, got {work_root_id}"
    );

    remove_static_fixture(&root);
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn open_work_root_returns_aggregated_view_of_all_opened_roots() {
    // CONTRACT: open_work_root returns the aggregated live view of every opened
    // workRoot, so the immediate open response matches the canonical resources
    // route and does not drop previously opened roots.
    let first = temp_fixture_path("open-aggregate-first");
    let second = temp_fixture_path("open-aggregate-second");
    fs::create_dir_all(&first).expect("create first workRoot");
    fs::create_dir_all(&second).expect("create second workRoot");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let first_id = open_work_root_for_test(app.clone(), cookie.as_str(), &first).await;

    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/open")
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "path": second.display().to_string() }).to_string(),
                ))
                .expect("open second workRoot request"),
        )
        .await
        .expect("open second workRoot response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("open second workRoot body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("open JSON");
    let ids = work_root_ids(&value);
    assert_eq!(
        ids.len(),
        2,
        "aggregated view includes both opened workRoots"
    );
    assert!(
        ids.iter().any(|id| id == &first_id),
        "aggregated view keeps the previously opened workRoot {first_id}"
    );

    remove_static_fixture(&first);
    remove_static_fixture(&second);
}

#[tokio::test]
async fn work_root_file_listing_routes_are_owner_authenticated() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/root-local-test/files")
                .body(Body::empty())
                .expect("unauthenticated workRoot files request"),
        )
        .await
        .expect("unauthenticated workRoot files response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn work_root_file_listing_routes_succeeds_after_opening_work_root() {
    let root = temp_fixture_path("work-root-files");
    fs::create_dir_all(root.join("src")).expect("create src dir");
    fs::write(root.join("README.md"), "hello\n").expect("write readme");
    fs::write(root.join("src/main.rs"), "fn main() {}\n").expect("write main");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let open_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/open")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "path": root.display().to_string()
                    })
                    .to_string(),
                ))
                .expect("open workRoot request"),
        )
        .await
        .expect("open workRoot response");
    assert_eq!(open_response.status(), StatusCode::OK);
    let open_body = axum::body::to_bytes(open_response.into_body(), 64 * 1024)
        .await
        .expect("open workRoot body bytes");
    let open_value: serde_json::Value = serde_json::from_slice(&open_body).expect("open JSON");
    let work_root_id = open_value["workspaces"][0]["workRoots"][0]["id"]
        .as_str()
        .expect("workRoot id");

    let response = app
        .oneshot(
            Request::builder()
                .uri(format!("/api/dashboard/work-roots/{work_root_id}/files"))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("workRoot files request"),
        )
        .await
        .expect("workRoot files response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("workRoot files body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("workRoot files JSON");
    assert_eq!(value["workRootId"], work_root_id);
    assert_eq!(value["path"], "");
    assert_eq!(value["status"], "ok");
    let entries = value["entries"].as_array().expect("entries array");
    assert_eq!(entries.len(), 2);
    assert_eq!(entries[0]["name"], "src");
    assert_eq!(entries[0]["path"], "src");
    assert_eq!(entries[0]["kind"], "directory");
    assert_eq!(entries[0]["readable"], true);
    assert_eq!(entries[0]["previewEligible"], false);
    assert_eq!(entries[1]["name"], "README.md");
    assert_eq!(entries[1]["path"], "README.md");
    assert_eq!(entries[1]["kind"], "file");
    assert_eq!(entries[1]["readable"], true);
    assert_eq!(entries[1]["previewEligible"], true);

    remove_static_fixture(&root);
}

#[tokio::test]
async fn work_root_file_listing_routes_rejects_traversal() {
    let parent = temp_fixture_path("work-root-traversal");
    let root = parent.join("root");
    fs::create_dir_all(&root).expect("create root dir");
    fs::write(parent.join("outside.txt"), "secret\n").expect("write outside file");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/files?path=../outside.txt"
                ))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("traversal workRoot files request"),
        )
        .await
        .expect("traversal workRoot files response");

    assert_ne!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("traversal response body bytes");
    let body = String::from_utf8(body.to_vec()).expect("traversal body is UTF-8");
    assert!(!body.contains("outside.txt"));
    assert!(!body.contains(&parent.display().to_string()));

    remove_static_fixture(&parent);
}

#[tokio::test]
async fn work_root_file_listing_routes_reports_unknown_work_root() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/root-local-unknown/files")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("unknown workRoot files request"),
        )
        .await
        .expect("unknown workRoot files response");

    assert_eq!(response.status(), StatusCode::NOT_FOUND);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("unknown body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("unknown JSON");
    assert_eq!(value["error"], "unknown workRoot");
}

// CONTRACT: WorkRoot Activity Phase 1 route tests should cover:
// - unauthenticated `/api/dashboard/work-roots/{workRootId}/activity` rejects
//   before reading wsstate or opened workRoot state;
// - unknown opened workRoot ids return 404 `{ "error": "unknown workRoot" }`;
// - an opened workRoot with no wsstate agents returns `status: "ok"` and an
//   empty `agents` list;
// - fixture wsstate with idle/running/failed agent records returns summary
//   counts and bounded row data;
// - malformed agent/current-call JSON degrades individual rows instead of
//   failing the whole route;
// - response bodies never contain host paths, cache paths, session ids, pids,
//   stdout/stderr paths, `agent.json`, or `current/state.json`.

fn app_state_with_activity_cache_home(cache_home: PathBuf) -> AppState {
    app_state_with_activity_cache_and_codex_home(cache_home, None)
}

fn app_state_with_activity_cache_and_codex_home(
    cache_home: PathBuf,
    codex_home: Option<PathBuf>,
) -> AppState {
    AppState {
        config: ServeConfig::default_loopback(),
        auth: OwnerAuthState::new_ephemeral(),
        opened_work_roots: OpenedWorkRoots::default(),
        dashboard_state: DashboardStateStore::disabled(),
        terminals: TerminalRegistry::default(),
        work_root_activity: WorkRootActivityProjector::new(WorkRootActivityProjectionConfig {
            codex_home,
            cache_home: Some(cache_home),
        }),
        registry_persist_lock: Arc::new(Mutex::new(())),
    }
}

#[tokio::test]
async fn work_root_activity_route_is_owner_authenticated() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/root-local-unknown/activity")
                .body(Body::empty())
                .expect("unauthenticated workRoot activity request"),
        )
        .await
        .expect("unauthenticated workRoot activity response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn work_root_activity_route_reports_unknown_work_root() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/root-local-unknown/activity")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("unknown workRoot activity request"),
        )
        .await
        .expect("unknown workRoot activity response");

    assert_eq!(response.status(), StatusCode::NOT_FOUND);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("unknown activity body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("unknown activity JSON");
    assert_eq!(value["error"], "unknown workRoot");
}

#[tokio::test]
async fn work_root_activity_events_route_is_owner_authenticated() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/root-local-unknown/activity/events")
                .body(Body::empty())
                .expect("unauthenticated workRoot activity events request"),
        )
        .await
        .expect("unauthenticated workRoot activity events response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn work_root_activity_events_route_reports_unknown_work_root() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/root-local-unknown/activity/events")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("unknown workRoot activity events request"),
        )
        .await
        .expect("unknown workRoot activity events response");

    assert_eq!(response.status(), StatusCode::NOT_FOUND);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("unknown activity events body bytes");
    let value: serde_json::Value =
        serde_json::from_slice(&body).expect("unknown activity events JSON");
    assert_eq!(value["error"], "unknown workRoot");
}

#[tokio::test]
async fn work_root_activity_events_streams_initial_fallback_snapshot_without_private_fields() {
    if skip_without_git(
        "work_root_activity_events_streams_initial_fallback_snapshot_without_private_fields",
    ) {
        return;
    }
    let root = temp_fixture_path("work-root-activity-events-initial");
    let cache_home = temp_fixture_path("work-root-activity-events-initial-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");
    write_agent_metadata(
        &agents_dir,
        "reviewer",
        &serde_json::json!({
            "schema_version": 1,
            "name": "reviewer",
            "backend": "codex",
            "status": "running",
            "session_id": "private-session",
            "last_output_path": cache_home.join("proj/private/stdout").display().to_string()
        }),
    );
    write_current_call(
        &agents_dir,
        "reviewer",
        r#"{"status":"running","execution_id":"000123","pid":4242,"stdout_path":"/tmp/stdout","stderr_path":"/tmp/stderr"}"#,
    );
    write_agent_output(&agents_dir, "reviewer", "hello from output\n");

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response = fetch_work_root_activity_events(app, cookie.as_str(), &work_root_id, "").await;
    assert_eq!(response.status(), StatusCode::OK);
    assert_eq!(
        response
            .headers()
            .get(header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok()),
        Some("text/event-stream")
    );
    let events = read_activity_sse_events(response, 4).await;
    assert_eq!(events[0]["type"], "modeChanged");
    assert_eq!(events[0]["updateMode"], "pollFallback");
    assert_eq!(events[1]["type"], "snapshotInvalidated");
    assert_eq!(events[1]["reason"], "fallback");
    assert!(events
        .iter()
        .any(|event| event["type"] == "itemUpserted" && event["item"]["id"] == "agent:reviewer"));
    assert!(events.iter().any(|event| event["type"] == "heartbeat"));

    let body_text = serde_json::to_string(&events).expect("events JSON string");
    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "private-session".to_owned(),
        "pid".to_owned(),
        "stdout_path".to_owned(),
        "stderr_path".to_owned(),
        "agent.json".to_owned(),
        "output.md".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "activity events response must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_events_streams_fallback_for_missing_agents_directory() {
    if skip_without_git("work_root_activity_events_streams_fallback_for_missing_agents_directory") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-events-missing-agents");
    let cache_home = temp_fixture_path("work-root-activity-events-missing-agents-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    fs::create_dir_all(&cache_home).expect("create activity cache root");
    init_git_repo(&root);

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response = fetch_work_root_activity_events(app, cookie.as_str(), &work_root_id, "").await;
    assert_eq!(response.status(), StatusCode::OK);
    let events = read_activity_sse_events(response, 3).await;

    assert_eq!(events[0]["type"], "modeChanged");
    assert_eq!(events[0]["updateMode"], "pollFallback");
    assert_eq!(events[1]["type"], "snapshotInvalidated");
    assert_eq!(events[1]["reason"], "fallback");
    assert_eq!(events[2]["type"], "heartbeat");
    assert!(!events.iter().any(|event| event["type"] == "itemUpserted"));

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_events_reconnect_cursor_stays_bounded() {
    if skip_without_git("work_root_activity_events_reconnect_cursor_stays_bounded") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-events-cursor");
    let cache_home = temp_fixture_path("work-root-activity-events-cursor-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");
    write_agent_metadata(
        &agents_dir,
        "cursor",
        &serde_json::json!({ "schema_version": 1, "name": "cursor", "status": "idle" }),
    );

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response =
        fetch_work_root_activity_events(app.clone(), cookie.as_str(), &work_root_id, "after=2")
            .await;
    assert_eq!(response.status(), StatusCode::OK);
    let events = read_activity_sse_events(response, 3).await;
    assert_eq!(events[0]["type"], "snapshotInvalidated");
    assert_eq!(events[0]["reason"], "watchReset");
    assert_eq!(events[0]["cursor"], "0000000000000003");

    let overflow_response =
        fetch_work_root_activity_events(app, cookie.as_str(), &work_root_id, "after=not-a-cursor")
            .await;
    assert_eq!(overflow_response.status(), StatusCode::OK);
    let overflow = read_activity_sse_events(overflow_response, 1).await;
    assert_eq!(overflow[0]["type"], "snapshotInvalidated");
    assert_eq!(overflow[0]["reason"], "overflow");

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_events_poll_fallback_is_scoped_to_subscribed_work_root() {
    if skip_without_git("work_root_activity_events_poll_fallback_is_scoped_to_subscribed_work_root")
    {
        return;
    }
    let base = temp_fixture_path("work-root-activity-events-scoped");
    let root_a = base.join("root-a");
    let root_b = base.join("root-b");
    let cache_home = base.join("cache");
    fs::create_dir_all(&root_a).expect("create activity workRoot A");
    fs::create_dir_all(&root_b).expect("create activity workRoot B");
    init_git_repo(&root_a);
    init_git_repo(&root_b);
    let agents_a = resolve_work_root_agents_dir(&cache_home, &root_a)
        .expect("resolve wsstate agents dir for workRoot A");
    let agents_b = resolve_work_root_agents_dir(&cache_home, &root_b)
        .expect("resolve wsstate agents dir for workRoot B");
    write_agent_metadata(
        &agents_a,
        "alpha",
        &serde_json::json!({ "schema_version": 1, "name": "alpha", "status": "idle" }),
    );
    write_agent_metadata(
        &agents_b,
        "bravo",
        &serde_json::json!({ "schema_version": 1, "name": "bravo", "status": "idle" }),
    );

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_a = open_work_root_for_test(app.clone(), cookie.as_str(), &root_a).await;
    let _work_root_b = open_work_root_for_test(app.clone(), cookie.as_str(), &root_b).await;

    let response =
        fetch_work_root_activity_events(app.clone(), cookie.as_str(), &work_root_a, "").await;
    assert_eq!(response.status(), StatusCode::OK);
    let mut stream = response.into_body().into_data_stream();
    let mut buffer = String::new();
    let mut seen = Vec::<serde_json::Value>::new();

    timeout(Duration::from_secs(5), async {
        while !seen
            .iter()
            .any(|event| event["type"] == "itemUpserted" && event["item"]["id"] == "agent:alpha")
        {
            let chunk = stream
                .next()
                .await
                .expect("initial scoped SSE chunk")
                .expect("initial scoped SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("initial scoped SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("initial scoped item event");

    write_agent_metadata(
        &agents_b,
        "bravo",
        &serde_json::json!({ "schema_version": 1, "name": "bravo", "status": "running" }),
    );

    let before_subscribed_change = seen.len();
    timeout(Duration::from_millis(700), async {
        while seen.len() < before_subscribed_change + 2 {
            let chunk = stream
                .next()
                .await
                .expect("cross-root scoped SSE chunk")
                .expect("cross-root scoped SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("cross-root scoped SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .ok();
    assert!(
        !seen.iter().any(|event| {
            event["type"] == "itemUpserted" && event["item"]["id"] == "agent:bravo"
        }),
        "subscribed workRoot stream must not emit sibling workRoot activity"
    );

    write_agent_metadata(
        &agents_a,
        "alpha",
        &serde_json::json!({ "schema_version": 1, "name": "alpha", "status": "running" }),
    );

    timeout(Duration::from_secs(5), async {
        while !seen.iter().any(|event| {
            event["type"] == "itemUpserted"
                && event["item"]["id"] == "agent:alpha"
                && event["item"]["status"] == "running"
        }) {
            let chunk = stream
                .next()
                .await
                .expect("subscribed scoped SSE chunk")
                .expect("subscribed scoped SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("subscribed scoped SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("subscribed workRoot update event");

    remove_static_fixture(&base);
}

#[tokio::test]
async fn work_root_activity_events_poll_fallback_reports_agent_changes_and_deletions() {
    if skip_without_git(
        "work_root_activity_events_poll_fallback_reports_agent_changes_and_deletions",
    ) {
        return;
    }
    let root = temp_fixture_path("work-root-activity-events-changes");
    let cache_home = temp_fixture_path("work-root-activity-events-changes-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");
    write_agent_metadata(
        &agents_dir,
        "delta",
        &serde_json::json!({ "schema_version": 1, "name": "delta", "status": "idle" }),
    );
    write_agent_output(&agents_dir, "delta", "first\n");

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response =
        fetch_work_root_activity_events(app.clone(), cookie.as_str(), &work_root_id, "").await;
    assert_eq!(response.status(), StatusCode::OK);
    let mut stream = response.into_body().into_data_stream();
    let mut buffer = String::new();
    let mut seen = Vec::<serde_json::Value>::new();

    timeout(Duration::from_secs(5), async {
        while !seen.iter().any(|event| event["type"] == "itemUpserted") {
            let chunk = stream
                .next()
                .await
                .expect("initial SSE chunk")
                .expect("initial SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("initial SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("initial item event");

    write_current_call(
        &agents_dir,
        "delta",
        r#"{"status":"running","execution_id":"run-1","started_at":"2026-05-21T00:00:00Z"}"#,
    );

    timeout(Duration::from_secs(5), async {
        while !seen.iter().any(|event| {
            event["type"] == "itemUpserted"
                && event["item"]["id"] == "agent:delta"
                && event["item"]["live"] == true
        }) {
            let chunk = stream
                .next()
                .await
                .expect("state change SSE chunk")
                .expect("state change SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("state change SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("current state update event");

    write_agent_output(&agents_dir, "delta", "first\nsecond\n");

    timeout(Duration::from_secs(5), async {
        while !seen.iter().any(|event| {
            event["type"] == "transcriptUpdated" && event["activityId"] == "agent:delta"
        }) {
            let chunk = stream
                .next()
                .await
                .expect("change SSE chunk")
                .expect("change SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("change SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("transcript update event");

    fs::remove_dir_all(agents_dir.join("delta")).expect("remove agent dir");

    timeout(Duration::from_secs(5), async {
        while !seen
            .iter()
            .any(|event| event["type"] == "itemRemoved" && event["activityId"] == "agent:delta")
        {
            let chunk = stream
                .next()
                .await
                .expect("remove SSE chunk")
                .expect("remove SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("remove SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("item removal event");

    write_agent_metadata(
        &agents_dir,
        "delta",
        &serde_json::json!({ "schema_version": 1, "name": "delta", "status": "idle" }),
    );

    timeout(Duration::from_secs(5), async {
        while !seen.iter().any(|event| {
            event["type"] == "itemUpserted"
                && event["item"]["id"] == "agent:delta"
                && event["item"]["status"] == "idle"
        }) {
            let chunk = stream
                .next()
                .await
                .expect("recreate SSE chunk")
                .expect("recreate SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("recreate SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("item recreate event");

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_events_emit_transcript_update_for_native_codex_mutation() {
    if skip_without_git(
        "work_root_activity_events_emit_transcript_update_for_native_codex_mutation",
    ) {
        return;
    }
    let root = temp_fixture_path("work-root-activity-events-native");
    let cache_home = temp_fixture_path("work-root-activity-events-native-cache");
    let codex_home = temp_fixture_path("work-root-activity-events-native-home");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");
    let session_id = "thread-events-secret";
    let session_path = write_codex_session(
        &codex_home,
        session_id,
        r#"{"timestamp":"2026-05-22T00:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"first native line"}}
"#,
    );
    write_agent_metadata(
        &agents_dir,
        "native-events",
        &serde_json::json!({
            "schema_version": 1,
            "name": "native-events",
            "backend": "codex",
            "status": "idle",
            "session_id": session_id
        }),
    );

    let state =
        app_state_with_activity_cache_and_codex_home(cache_home.clone(), Some(codex_home.clone()));
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response =
        fetch_work_root_activity_events(app.clone(), cookie.as_str(), &work_root_id, "").await;
    assert_eq!(response.status(), StatusCode::OK);
    let mut stream = response.into_body().into_data_stream();
    let mut buffer = String::new();
    let mut seen = Vec::<serde_json::Value>::new();

    timeout(Duration::from_secs(5), async {
        while !seen.iter().any(|event| {
            event["type"] == "itemUpserted" && event["item"]["id"] == "agent:native-events"
        }) {
            let chunk = stream
                .next()
                .await
                .expect("initial native SSE chunk")
                .expect("initial native SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("initial native SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("initial native item event");

    fs::write(
        &session_path,
        r#"{"timestamp":"2026-05-22T00:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"first native line"}}
{"timestamp":"2026-05-22T00:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"second native line"}}
"#,
    )
    .expect("mutate native session fixture");

    timeout(Duration::from_secs(5), async {
        while !seen.iter().any(|event| {
            event["type"] == "transcriptUpdated"
                && event["activityId"] == "agent:native-events"
                && event["transcriptCursor"] == "0"
        }) {
            let chunk = stream
                .next()
                .await
                .expect("native transcript SSE chunk")
                .expect("native transcript SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("native transcript SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("native transcript update event");

    let body_text = serde_json::to_string(&seen).expect("native SSE events JSON string");
    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        codex_home.display().to_string(),
        session_path.display().to_string(),
        session_id.to_owned(),
        "rollout-".to_owned(),
        ".jsonl".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "native transcript update event must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
    remove_static_fixture(&codex_home);
}

fn drain_sse_events(buffer: &mut String, events: &mut Vec<serde_json::Value>) {
    while let Some(boundary) = buffer.find("\n\n") {
        let frame = buffer[..boundary].to_owned();
        *buffer = buffer[(boundary + 2)..].to_owned();
        events.push(activity_sse_frame_data(&frame));
    }
}

fn activity_sse_frame_data(frame: &str) -> serde_json::Value {
    assert!(
        frame.lines().any(|line| line == "event: activity"),
        "activity SSE frame must name event: activity; frame was {frame:?}"
    );
    let data = frame
        .lines()
        .find_map(|line| line.strip_prefix("data: "))
        .expect("activity SSE frame data field");
    serde_json::from_str(data).expect("activity SSE data JSON")
}

#[tokio::test]
async fn work_root_activity_route_returns_empty_named_agent_projection() {
    let root = temp_fixture_path("work-root-activity-empty");
    let cache_home = temp_fixture_path("work-root-activity-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    fs::create_dir_all(&cache_home).expect("create activity cache fixture root");
    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri(format!("/api/dashboard/work-roots/{work_root_id}/activity"))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("empty workRoot activity request"),
        )
        .await
        .expect("empty workRoot activity response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("empty activity body bytes");
    let body_text = String::from_utf8(body.to_vec()).expect("empty activity body is UTF-8");
    let value: serde_json::Value = serde_json::from_str(&body_text).expect("empty activity JSON");
    assert_eq!(value["workRootId"], work_root_id);
    assert_eq!(value["status"], "ok");
    assert_eq!(value["updateMode"], "snapshot");
    assert!(value["feedCursor"]
        .as_str()
        .expect("feed cursor")
        .starts_with("snapshot:"));
    assert!(value["selectedItemId"].is_null());
    assert_eq!(
        value["summary"],
        serde_json::json!({
            "total": 0,
            "active": 0,
            "blocked": 0,
            "failed": 0,
            "unavailable": 0
        })
    );
    assert_eq!(
        value["agents"]
            .as_array()
            .expect("activity agents array")
            .len(),
        0
    );
    assert!(value.get("work_root_id").is_none());

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "agent.json".to_owned(),
        "current/state.json".to_owned(),
        "session_id".to_owned(),
        "stdout".to_owned(),
        "stderr".to_owned(),
        "pid".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "activity response must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

fn git_available() -> bool {
    Command::new("git")
        .arg("--version")
        .output()
        .map(|output| output.status.success())
        .unwrap_or(false)
}

fn run_git(path: &Path, args: &[&str]) {
    let output = Command::new("git")
        .arg("-C")
        .arg(path)
        .args(args)
        .output()
        .expect("run git");
    assert!(
        output.status.success(),
        "git {args:?} failed: {}",
        String::from_utf8_lossy(&output.stderr)
    );
}

fn init_git_repo(path: &Path) {
    run_git(path, &["init"]);
    run_git(
        path,
        &["config", "user.email", "ws-dashboard@example.local"],
    );
    run_git(path, &["config", "user.name", "ws dashboard"]);
}

fn skip_without_git(test_name: &str) -> bool {
    if git_available() {
        return false;
    }
    // Visible signal so a git-less environment does not hide this coverage
    // behind a silently-green result.
    eprintln!("SKIP {test_name}: the `git` binary is unavailable");
    true
}

fn write_agent_metadata(agents_dir: &Path, agent_key: &str, agent_json: &serde_json::Value) {
    let agent_dir = agents_dir.join(agent_key);
    fs::create_dir_all(&agent_dir).expect("create agent fixture dir");
    fs::write(
        agent_dir.join("agent.json"),
        serde_json::to_string_pretty(agent_json).expect("encode agent fixture"),
    )
    .expect("write agent.json fixture");
}

fn write_agent_metadata_raw(agents_dir: &Path, agent_key: &str, raw: &str) {
    let agent_dir = agents_dir.join(agent_key);
    fs::create_dir_all(&agent_dir).expect("create agent fixture dir");
    fs::write(agent_dir.join("agent.json"), raw).expect("write raw agent.json fixture");
}

fn write_current_call(agents_dir: &Path, agent_key: &str, raw: &str) {
    let current_dir = agents_dir.join(agent_key).join("current");
    fs::create_dir_all(&current_dir).expect("create current fixture dir");
    fs::write(current_dir.join("state.json"), raw).expect("write state.json fixture");
}

fn write_agent_output(agents_dir: &Path, agent_key: &str, raw: &str) {
    let agent_dir = agents_dir.join(agent_key);
    fs::create_dir_all(&agent_dir).expect("create agent output fixture dir");
    fs::write(agent_dir.join("output.md"), raw).expect("write output.md fixture");
}

fn write_codex_session(codex_home: &Path, session_id: &str, raw: &str) -> PathBuf {
    let session_dir = codex_home
        .join("sessions")
        .join("2026")
        .join("05")
        .join("22");
    fs::create_dir_all(&session_dir).expect("create codex session fixture dir");
    let session_path = session_dir.join(format!("rollout-2026-05-22T00-00-00-{session_id}.jsonl"));
    fs::write(&session_path, raw).expect("write codex session fixture");
    session_path
}

async fn fetch_work_root_activity(
    app: axum::Router,
    cookie: &str,
    work_root_id: &str,
) -> (StatusCode, String) {
    fetch_work_root_activity_path(
        app,
        cookie,
        &format!("/api/dashboard/work-roots/{work_root_id}/activity"),
    )
    .await
}

async fn fetch_work_root_activity_path(
    app: axum::Router,
    cookie: &str,
    path: &str,
) -> (StatusCode, String) {
    let response = app
        .oneshot(
            Request::builder()
                .uri(path)
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("workRoot activity request"),
        )
        .await
        .expect("workRoot activity response");
    let status = response.status();
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("workRoot activity body bytes");
    (
        status,
        String::from_utf8(body.to_vec()).expect("activity body is UTF-8"),
    )
}

async fn fetch_work_root_activity_transcript(
    app: axum::Router,
    cookie: &str,
    work_root_id: &str,
    activity_id: &str,
    query: &str,
) -> (StatusCode, String) {
    let suffix = if query.is_empty() {
        String::new()
    } else {
        format!("?{query}")
    };
    fetch_work_root_activity_path(
        app,
        cookie,
        &format!(
            "/api/dashboard/work-roots/{work_root_id}/activity/items/{activity_id}/transcript{suffix}"
        ),
    )
    .await
}

async fn read_activity_sse_events(
    response: axum::response::Response,
    expected_count: usize,
) -> Vec<serde_json::Value> {
    let mut stream = response.into_body().into_data_stream();
    let mut buffer = String::new();
    let mut events = Vec::new();
    timeout(Duration::from_secs(5), async {
        while events.len() < expected_count {
            let Some(chunk) = stream.next().await else {
                break;
            };
            let chunk = chunk.expect("activity SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("activity SSE UTF-8"));
            while let Some(boundary) = buffer.find("\n\n") {
                let frame = buffer[..boundary].to_owned();
                buffer = buffer[(boundary + 2)..].to_owned();
                events.push(activity_sse_frame_data(&frame));
            }
        }
    })
    .await
    .expect("timely activity SSE events");
    events
}

async fn fetch_work_root_activity_events(
    app: axum::Router,
    cookie: &str,
    work_root_id: &str,
    query: &str,
) -> axum::response::Response {
    let suffix = if query.is_empty() {
        String::new()
    } else {
        format!("?{query}")
    };
    app.oneshot(
        Request::builder()
            .uri(format!(
                "/api/dashboard/work-roots/{work_root_id}/activity/events{suffix}"
            ))
            .header(header::COOKIE, cookie)
            .body(Body::empty())
            .expect("workRoot activity events request"),
    )
    .await
    .expect("workRoot activity events response")
}

#[tokio::test]
async fn work_root_activity_route_projects_named_agent_records() {
    if skip_without_git("work_root_activity_route_projects_named_agent_records") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-records");
    let cache_home = temp_fixture_path("work-root-activity-records-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    write_agent_metadata(
        &agents_dir,
        "reviewer",
        &serde_json::json!({
            "schema_version": 1,
            "name": "reviewer",
            "backend": "codex",
            "harness": "codex",
            "tier": "core",
            "model": "gpt-5.3-codex",
            "effort": "medium",
            "session_id": "session-abc",
            "status": "idle",
            "last_call_at": "2026-05-17T09:00:00Z",
            "last_output_path": "/cache/agents/reviewer/output.md",
            "pid": 4242,
            "stdout_path": "/cache/agents/reviewer/current/stdout"
        }),
    );

    write_agent_output(
        &agents_dir,
        "reviewer",
        "# Review result\nprivate paths stay daemon-side\n",
    );

    write_agent_metadata(
        &agents_dir,
        "builder",
        &serde_json::json!({
            "schema_version": 1,
            "name": "builder",
            "backend": "claude",
            "status": "running",
            "session_id": "session-def",
            "last_call_at": "2026-05-17T10:00:00Z"
        }),
    );
    write_current_call(
        &agents_dir,
        "builder",
        &serde_json::json!({
            "schema_version": 1,
            "agent_name": "builder",
            "status": "running",
            "execution_id": "000123",
            "started_at": "2026-05-17T10:00:00Z",
            "updated_at": "2026-05-17T10:01:00Z",
            "pid": 5151,
            "stdout_path": "/cache/agents/builder/current/stdout",
            "stderr_path": "/cache/agents/builder/current/stderr"
        })
        .to_string(),
    );

    write_agent_metadata(
        &agents_dir,
        "planner",
        &serde_json::json!({
            "schema_version": 1,
            "name": "planner",
            "backend": "codex",
            "status": "blocked",
            "last_call_at": "2026-05-17T09:30:00Z"
        }),
    );

    write_agent_metadata(
        &agents_dir,
        "retired",
        &serde_json::json!({
            "schema_version": 1,
            "name": "retired",
            "backend": "gemini",
            "status": "erased"
        }),
    );

    write_agent_metadata(
        &agents_dir,
        "tester",
        &serde_json::json!({
            "schema_version": 1,
            "name": "tester",
            "backend": "gemini",
            "status": "failed",
            "last_call_at": "2026-05-17T08:00:00Z"
        }),
    );
    write_current_call(
        &agents_dir,
        "tester",
        &serde_json::json!({
            "schema_version": 1,
            "agent_name": "tester",
            "status": "failed",
            "execution_id": "000077",
            "started_at": "2026-05-17T07:59:00Z",
            "updated_at": "2026-05-17T08:00:00Z",
            "finished_at": "2026-05-17T08:00:00Z",
            "cleanup_needed": true,
            "error": "backend exited with status 1"
        })
        .to_string(),
    );

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity(app, cookie.as_str(), &work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("named-agent activity JSON");

    assert_eq!(value["workRootId"], work_root_id);
    assert_eq!(value["status"], "ok");
    assert_eq!(value["updateMode"], "snapshot");
    assert!(value["feedCursor"]
        .as_str()
        .expect("feed cursor")
        .starts_with("snapshot:"));
    assert_eq!(value["selectedItemId"], "agent:builder");
    assert_eq!(
        value["summary"],
        serde_json::json!({
            "total": 5,
            "active": 1,
            "blocked": 1,
            "failed": 1,
            "unavailable": 0
        })
    );

    let items = value["items"].as_array().expect("activity items array");
    assert_eq!(items.len(), 5);
    assert_eq!(items[0]["id"], "agent:builder");
    assert_eq!(items[0]["kind"], "namedAgent");
    assert_eq!(items[0]["live"], true);
    assert_eq!(items[0]["source"]["kind"], "namedAgent");
    assert_eq!(items[0]["startedAt"], "2026-05-17T10:00:00Z");
    let reviewer_item = items
        .iter()
        .find(|item| item["id"] == "agent:reviewer")
        .expect("reviewer activity item");
    assert_eq!(reviewer_item["transcript"]["status"], "available");
    assert_eq!(reviewer_item["transcript"]["available"], true);
    assert_eq!(reviewer_item["metadata"]["agentId"], "reviewer");

    let agents = value["agents"].as_array().expect("activity agents array");
    assert_eq!(agents.len(), 5);
    let agent_row = |agent_id: &str| -> &serde_json::Value {
        agents
            .iter()
            .find(|agent| agent["agentId"] == agent_id)
            .unwrap_or_else(|| panic!("missing agent row {agent_id}"))
    };

    let builder = agent_row("builder");
    assert_eq!(builder["status"], "running");
    assert_eq!(builder["sessionPresent"], true);
    assert_eq!(builder["currentCall"]["status"], "running");
    assert_eq!(builder["currentCall"]["active"], true);
    assert_eq!(builder["currentCall"]["terminal"], false);
    assert_eq!(builder["currentCall"]["executionId"], "000123");
    assert!(builder["diagnostics"]
        .as_array()
        .expect("builder diagnostics array")
        .is_empty());

    let reviewer = agent_row("reviewer");
    assert_eq!(reviewer["status"], "idle");
    assert_eq!(reviewer["backend"], "codex");
    assert_eq!(reviewer["model"], "gpt-5.3-codex");
    assert_eq!(reviewer["lastCallAt"], "2026-05-17T09:00:00Z");
    assert_eq!(reviewer["sessionPresent"], true);
    assert!(reviewer["currentCall"].is_null());
    assert!(reviewer["detailHints"]
        .as_array()
        .expect("reviewer detail hints array")
        .iter()
        .any(|hint| hint == "recent output available"));

    let planner = agent_row("planner");
    assert_eq!(planner["status"], "blocked");
    assert_eq!(planner["sessionPresent"], false);
    assert!(planner["currentCall"].is_null());

    let retired = agent_row("retired");
    assert_eq!(retired["status"], "erased");

    let tester = agent_row("tester");
    assert_eq!(tester["status"], "failed");
    assert_eq!(tester["currentCall"]["status"], "failed");
    assert_eq!(tester["currentCall"]["active"], false);
    assert_eq!(tester["currentCall"]["terminal"], true);
    assert_eq!(tester["currentCall"]["cleanupNeeded"], true);
    assert_eq!(
        tester["currentCall"]["error"],
        "backend exited with status 1"
    );

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "agent.json".to_owned(),
        "current/state.json".to_owned(),
        "session_id".to_owned(),
        "session-abc".to_owned(),
        "session-def".to_owned(),
        "stdout".to_owned(),
        "stderr".to_owned(),
        "pid".to_owned(),
        "output.md".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "activity response must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_limits_recent_agent_projection() {
    if skip_without_git("work_root_activity_route_limits_recent_agent_projection") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-recent-limit");
    let cache_home = temp_fixture_path("work-root-activity-recent-limit-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    for index in 0..5 {
        write_agent_metadata(
            &agents_dir,
            &format!("agent-{index}"),
            &serde_json::json!({
                "schema_version": 1,
                "name": format!("agent-{index}"),
                "backend": "codex",
                "status": "idle"
            }),
        );
    }

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity_path(
        app,
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{work_root_id}/activity?recentLimit=2"),
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("recent-limited activity JSON");
    assert_eq!(value["summary"]["total"], 2);
    assert_eq!(
        value["agents"]
            .as_array()
            .expect("recent-limited agents array")
            .len(),
        2
    );
}

#[tokio::test]
async fn work_root_activity_transcript_route_auth_unknown_and_backfill_bounds() {
    if skip_without_git("work_root_activity_transcript_route_auth_unknown_and_backfill_bounds") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-transcript");
    let cache_home = temp_fixture_path("work-root-activity-transcript-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    write_agent_metadata(
        &agents_dir,
        "writer",
        &serde_json::json!({
            "schema_version": 1,
            "name": "writer",
            "backend": "codex",
            "status": "idle",
            "last_output_path": "/private/cache/agents/writer/output.md",
            "session_id": "session-secret"
        }),
    );
    write_agent_output(
        &agents_dir,
        "writer",
        "first block\nsecond block\nthird block\n",
    );

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);

    let unauthenticated = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/root-local-unknown/activity/items/agent:writer/transcript")
                .body(Body::empty())
                .expect("unauthenticated transcript request"),
        )
        .await
        .expect("unauthenticated transcript response");
    assert_eq!(unauthenticated.status(), StatusCode::UNAUTHORIZED);

    let cookie = pair_and_cookie(app.clone(), &token).await;
    let missing = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/root-local-unknown/activity/items/agent:writer/transcript")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("unknown transcript workRoot request"),
        )
        .await
        .expect("unknown transcript workRoot response");
    assert_eq!(missing.status(), StatusCode::NOT_FOUND);

    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;
    let (unknown_status, unknown_body) = fetch_work_root_activity_transcript(
        app.clone(),
        cookie.as_str(),
        &work_root_id,
        "exec:missing",
        "",
    )
    .await;
    assert_eq!(unknown_status, StatusCode::NOT_FOUND);
    assert!(unknown_body.contains("unknown activity"));

    let (tail_status, tail_body) = fetch_work_root_activity_transcript(
        app.clone(),
        cookie.as_str(),
        &work_root_id,
        "agent:writer",
        "limit=2",
    )
    .await;
    assert_eq!(tail_status, StatusCode::OK);
    let tail: serde_json::Value =
        serde_json::from_str(&tail_body).expect("tail activity transcript JSON");
    assert_eq!(tail["blocks"].as_array().expect("tail blocks").len(), 2);
    assert_eq!(tail["blocks"][0]["cursor"], "1");
    assert_eq!(tail["blocks"][0]["text"], "second block");
    assert_eq!(tail["blocks"][1]["cursor"], "2");
    assert_eq!(tail["blocks"][1]["text"], "third block");
    assert_eq!(tail["nextCursor"], "1");
    assert_eq!(tail["hasMore"], true);

    let (older_status, older_body) = fetch_work_root_activity_transcript(
        app.clone(),
        cookie.as_str(),
        &work_root_id,
        "agent:writer",
        "limit=2&before=1",
    )
    .await;
    assert_eq!(older_status, StatusCode::OK);
    let older: serde_json::Value =
        serde_json::from_str(&older_body).expect("older activity transcript JSON");
    assert_eq!(older["blocks"].as_array().expect("older blocks").len(), 1);
    assert_eq!(older["blocks"][0]["cursor"], "0");
    assert_eq!(older["blocks"][0]["text"], "first block");
    assert_eq!(older["nextCursor"], "0");
    assert_eq!(older["hasMore"], false);

    let (status, body_text) = fetch_work_root_activity_transcript(
        app,
        cookie.as_str(),
        &work_root_id,
        "agent:writer",
        "limit=2&cursor=0",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("activity transcript JSON");
    assert_eq!(value["workRootId"], work_root_id);
    assert_eq!(value["activityId"], "agent:writer");
    assert_eq!(value["status"], "available");
    assert_eq!(value["sourceStatus"], "ok");
    assert_eq!(value["source"]["kind"], "namedAgent");
    assert_eq!(value["blocks"].as_array().expect("blocks").len(), 2);
    assert_eq!(value["blocks"][0]["cursor"], "0");
    assert_eq!(value["blocks"][0]["renderKind"], "markdown");
    assert_eq!(value["blocks"][0]["text"], "first block");
    assert_eq!(value["nextCursor"], "2");
    assert_eq!(value["hasMore"], true);

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "session-secret".to_owned(),
        "session_id".to_owned(),
        "output.md".to_owned(),
        "agent.json".to_owned(),
        "/private/cache".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "transcript response must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_transcript_route_reads_codex_native_session_backfill() {
    if skip_without_git("work_root_activity_transcript_route_reads_codex_native_session_backfill") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-codex-native");
    let cache_home = temp_fixture_path("work-root-activity-codex-native-cache");
    let codex_home = temp_fixture_path("work-root-activity-codex-native-home");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    let session_id = "thread-native-secret";
    let session_path = write_codex_session(
        &codex_home,
        session_id,
        r#"{"timestamp":"2026-05-22T00:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-private"}}
{"timestamp":"2026-05-22T00:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"I can help with the activity transcript."}}
{"timestamp":"2026-05-22T00:00:02Z","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":{"cmd":"cat /private/cache/native.jsonl"}}}
{"timestamp":"2026-05-22T00:00:03Z","type":"response_item","payload":{"type":"function_call_output","output":"private /host/path result"}}
{"timestamp":"2026-05-22T00:00:04Z","type":"event_msg","payload":{"type":"task_complete","last_agent_message":"done"}}
{"timestamp":"2026-05-22T00:00:05Z","type":"event_msg","payload":{"type":"user_message","message":"Please inspect /private/cache and continue"}}
{"timestamp":"2026-05-22T00:00:06Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I will inspect the safe summary."}]}}
{"timestamp":"2026-05-22T00:00:07Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"patch touching /private/cache/native.jsonl"}}
{"timestamp":"2026-05-22T00:00:08Z","type":"response_item","payload":{"type":"custom_tool_call_output","output":"private custom output"}}
{"timestamp":"2026-05-22T00:00:09Z","type":"event_msg","payload":{"type":"mcp_tool_call_end","status":"success","duration":12,"invocation":{"tool":"tickets_status","arguments":{"path":"/private/cache"}},"result":{"text":"/private/mcp-result"}}}
{"timestamp":"2026-05-22T00:00:10Z","type":"event_msg","payload":{"type":"exec_command_end","status":"success","exit_code":0,"duration":34,"command":"cat /private/cache/native.jsonl","cwd":"/private/cache","stdout":"private stdout","stderr":""}}
{"timestamp":"2026-05-22T00:00:11Z","type":"event_msg","payload":{"type":"patch_apply_end","status":"success","success":true,"changes":[{"path":"/private/cache/native.jsonl"}],"stdout":"applied /private/cache/native.jsonl","stderr":""}}
{"timestamp":"2026-05-22T00:00:12Z","type":"event_msg","payload":{"type":"turn_aborted","reason":"user_interrupt"}}
{"timestamp":"2026-05-22T00:00:13Z","type":"event_msg","payload":{"type":"token_count","info":{"path":"/private/cache"}}}
{"timestamp":"2026-05-22T00:00:14Z","type":"session_meta","id":"thread-native-secret","cwd":"/private/cache"}
"#,
    );
    write_agent_metadata(
        &agents_dir,
        "native",
        &serde_json::json!({
            "schema_version": 1,
            "name": "native",
            "backend": "codex",
            "harness": "codex",
            "status": "idle",
            "last_call_at": "2026-05-22T00:00:04Z",
            "session_id": session_id,
            "last_output_path": "/private/cache/native/output.md",
            "pid": 999,
            "stdout_path": "/private/cache/native/current/stdout",
            "stderr_path": "/private/cache/native/current/stderr"
        }),
    );

    let state =
        app_state_with_activity_cache_and_codex_home(cache_home.clone(), Some(codex_home.clone()));
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (feed_status, feed_body) =
        fetch_work_root_activity(app.clone(), cookie.as_str(), &work_root_id).await;
    assert_eq!(feed_status, StatusCode::OK);
    let feed: serde_json::Value = serde_json::from_str(&feed_body).expect("native feed JSON");
    assert_eq!(feed["items"][0]["transcript"]["status"], "available");
    assert_eq!(feed["items"][0]["transcript"]["available"], true);

    let (status, body_text) = fetch_work_root_activity_transcript(
        app,
        cookie.as_str(),
        &work_root_id,
        "agent:native",
        "limit=20",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("native transcript JSON");
    assert_eq!(value["status"], "available");
    assert_eq!(value["sourceStatus"], "ok");
    assert_eq!(value["blocks"].as_array().expect("blocks").len(), 13);
    assert_eq!(value["blocks"][0]["renderKind"], "status");
    assert_eq!(value["blocks"][0]["title"], "Task started");
    assert_eq!(value["blocks"][1]["renderKind"], "assistant");
    assert_eq!(
        value["blocks"][1]["text"],
        "I can help with the activity transcript."
    );
    assert_eq!(value["blocks"][2]["renderKind"], "toolCall");
    assert_eq!(value["blocks"][2]["data"]["name"], "shell");
    assert_eq!(value["blocks"][5]["renderKind"], "user");
    assert_eq!(
        value["blocks"][5]["text"],
        "Please inspect [redacted] and continue"
    );
    assert_eq!(value["blocks"][6]["renderKind"], "assistant");
    assert_eq!(value["blocks"][7]["renderKind"], "toolCall");
    assert_eq!(value["blocks"][7]["data"]["name"], "apply_patch");
    assert_eq!(value["blocks"][8]["renderKind"], "toolResult");
    assert_eq!(value["blocks"][9]["title"], "MCP tool result");
    assert_eq!(value["blocks"][10]["title"], "Command result");
    assert_eq!(value["blocks"][10]["data"]["exitCode"], 0);
    assert_eq!(value["blocks"][11]["title"], "Patch apply");
    assert_eq!(value["blocks"][11]["data"]["changes"], 1);
    assert_eq!(value["blocks"][12]["title"], "Turn aborted");
    assert_eq!(value["nextCursor"], "0");
    assert_eq!(value["hasMore"], false);

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        codex_home.display().to_string(),
        session_path.display().to_string(),
        session_id.to_owned(),
        "turn-private".to_owned(),
        "/private/cache".to_owned(),
        "/host/path".to_owned(),
        "native.jsonl".to_owned(),
        "session_id".to_owned(),
        "stdout".to_owned(),
        "stderr".to_owned(),
        "pid".to_owned(),
        "function_call_output".to_owned(),
        "custom_tool_call_output".to_owned(),
        "cat ".to_owned(),
        "private stdout".to_owned(),
        "private custom output".to_owned(),
        "mcp-result".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "native transcript response must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
    remove_static_fixture(&codex_home);
}

#[tokio::test]
async fn work_root_activity_transcript_route_degrades_malformed_codex_native_records() {
    if skip_without_git(
        "work_root_activity_transcript_route_degrades_malformed_codex_native_records",
    ) {
        return;
    }
    let root = temp_fixture_path("work-root-activity-codex-malformed");
    let cache_home = temp_fixture_path("work-root-activity-codex-malformed-cache");
    let codex_home = temp_fixture_path("work-root-activity-codex-malformed-home");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    let session_id = "thread-malformed-secret";
    write_codex_session(
        &codex_home,
        session_id,
        r#"{"timestamp":"2026-05-22T00:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"safe assistant text"}}
not json with /private/native/path and thread-malformed-secret
{"timestamp":"2026-05-22T00:00:02Z","type":"debug_event","payload":{"type":"unknown","path":"/private/native/path"}}
"#,
    );
    write_agent_metadata(
        &agents_dir,
        "native-broken",
        &serde_json::json!({
            "schema_version": 1,
            "name": "native-broken",
            "backend": "codex",
            "status": "idle",
            "session_id": session_id
        }),
    );

    let state =
        app_state_with_activity_cache_and_codex_home(cache_home.clone(), Some(codex_home.clone()));
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity_transcript(
        app,
        cookie.as_str(),
        &work_root_id,
        "agent:native-broken",
        "",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("malformed native transcript JSON");
    assert_eq!(value["status"], "degraded");
    assert_eq!(value["sourceStatus"], "degraded");
    let blocks = value["blocks"].as_array().expect("blocks");
    assert_eq!(blocks.len(), 3);
    assert_eq!(blocks[0]["renderKind"], "assistant");
    assert_eq!(blocks[1]["degraded"], true);
    assert_eq!(blocks[1]["title"], "Malformed transcript record");
    assert_eq!(blocks[2]["degraded"], true);
    assert_eq!(blocks[2]["title"], "Unsupported transcript record");
    assert!(!value["diagnostics"]
        .as_array()
        .expect("diagnostics")
        .is_empty());

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        codex_home.display().to_string(),
        session_id.to_owned(),
        "/private/native/path".to_owned(),
        "not json".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "malformed native transcript response must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
    remove_static_fixture(&codex_home);
}

#[tokio::test]
async fn work_root_activity_transcript_route_falls_back_when_codex_native_missing() {
    if skip_without_git("work_root_activity_transcript_route_falls_back_when_codex_native_missing")
    {
        return;
    }
    let root = temp_fixture_path("work-root-activity-codex-missing");
    let cache_home = temp_fixture_path("work-root-activity-codex-missing-cache");
    let codex_home = temp_fixture_path("work-root-activity-codex-missing-home");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    write_agent_metadata(
        &agents_dir,
        "fallback",
        &serde_json::json!({
            "schema_version": 1,
            "name": "fallback",
            "backend": "codex",
            "status": "idle",
            "session_id": "thread-missing-secret"
        }),
    );
    write_agent_output(&agents_dir, "fallback", "fallback output\n");

    let state =
        app_state_with_activity_cache_and_codex_home(cache_home.clone(), Some(codex_home.clone()));
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity_transcript(
        app,
        cookie.as_str(),
        &work_root_id,
        "agent:fallback",
        "",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("fallback transcript JSON");
    assert_eq!(value["status"], "available");
    assert_eq!(value["sourceStatus"], "ok");
    assert_eq!(value["blocks"][0]["renderKind"], "markdown");
    assert_eq!(value["blocks"][0]["text"], "fallback output");
    assert!(!body_text.contains("thread-missing-secret"));

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
    remove_static_fixture(&codex_home);
}

#[cfg(unix)]
#[tokio::test]
async fn work_root_activity_transcript_route_degrades_when_codex_native_unreadable() {
    if skip_without_git("work_root_activity_transcript_route_degrades_when_codex_native_unreadable")
    {
        return;
    }
    let root = temp_fixture_path("work-root-activity-codex-unreadable");
    let cache_home = temp_fixture_path("work-root-activity-codex-unreadable-cache");
    let codex_home = temp_fixture_path("work-root-activity-codex-unreadable-home");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    let session_id = "thread-unreadable-secret";
    let session_path = write_codex_session(
        &codex_home,
        session_id,
        "unreadable native content with /private/native/path\n",
    );
    fs::set_permissions(&session_path, fs::Permissions::from_mode(0o000))
        .expect("make native session unreadable");
    write_agent_metadata(
        &agents_dir,
        "unreadable",
        &serde_json::json!({
            "schema_version": 1,
            "name": "unreadable",
            "backend": "codex",
            "status": "idle",
            "session_id": session_id
        }),
    );
    write_agent_output(
        &agents_dir,
        "unreadable",
        "fallback after unreadable native\n",
    );

    let state =
        app_state_with_activity_cache_and_codex_home(cache_home.clone(), Some(codex_home.clone()));
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity_transcript(
        app,
        cookie.as_str(),
        &work_root_id,
        "agent:unreadable",
        "",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("unreadable fallback transcript JSON");
    assert_eq!(value["status"], "degraded");
    assert_eq!(value["sourceStatus"], "degraded");
    assert_eq!(value["blocks"][0]["renderKind"], "markdown");
    assert_eq!(
        value["blocks"][0]["text"],
        "fallback after unreadable native"
    );
    assert!(value["diagnostics"]
        .as_array()
        .expect("diagnostics")
        .iter()
        .any(|diagnostic| diagnostic == "native transcript source unreadable"));

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        codex_home.display().to_string(),
        session_path.display().to_string(),
        session_id.to_owned(),
        "/private/native/path".to_owned(),
        "unreadable native content".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "unreadable native fallback response must not leak {forbidden}"
        );
    }

    let _ = fs::set_permissions(&session_path, fs::Permissions::from_mode(0o600));
    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
    remove_static_fixture(&codex_home);
}

#[tokio::test]
async fn work_root_activity_feed_orders_attention_before_alphabetical() {
    if skip_without_git("work_root_activity_feed_orders_attention_before_alphabetical") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-ordering");
    let cache_home = temp_fixture_path("work-root-activity-ordering-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    write_agent_metadata(
        &agents_dir,
        "z-live",
        &serde_json::json!({ "schema_version": 1, "name": "z live", "status": "running" }),
    );
    write_current_call(
        &agents_dir,
        "z-live",
        &serde_json::json!({
            "schema_version": 1,
            "status": "running",
            "started_at": "2026-05-17T10:00:00Z",
            "updated_at": "2026-05-17T10:00:01Z"
        })
        .to_string(),
    );
    write_agent_metadata(
        &agents_dir,
        "a-idle",
        &serde_json::json!({
            "schema_version": 1,
            "name": "a idle",
            "status": "idle",
            "last_call_at": "2026-05-17T11:00:00Z"
        }),
    );
    write_agent_metadata(
        &agents_dir,
        "m-blocked",
        &serde_json::json!({ "schema_version": 1, "name": "m blocked", "status": "blocked" }),
    );
    write_agent_metadata(
        &agents_dir,
        "b-failed",
        &serde_json::json!({ "schema_version": 1, "name": "b failed", "status": "failed" }),
    );

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity(app, cookie.as_str(), &work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value = serde_json::from_str(&body_text).expect("activity feed JSON");
    let item_ids = value["items"]
        .as_array()
        .expect("items")
        .iter()
        .map(|item| item["id"].as_str().expect("item id").to_owned())
        .collect::<Vec<_>>();
    assert_eq!(
        item_ids,
        vec![
            "agent:z-live",
            "agent:b-failed",
            "agent:m-blocked",
            "agent:a-idle"
        ],
        "live/attention items sort ahead of more recent alphabetical idle rows"
    );

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_transcript_degrades_empty_and_unavailable_sources() {
    if skip_without_git("work_root_activity_transcript_degrades_empty_and_unavailable_sources") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-transcript-empty");
    let cache_home = temp_fixture_path("work-root-activity-transcript-empty-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    write_agent_metadata(
        &agents_dir,
        "empty",
        &serde_json::json!({ "schema_version": 1, "name": "empty", "status": "idle" }),
    );
    write_agent_metadata_raw(&agents_dir, "broken", "{ bad json");

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (empty_status, empty_body) = fetch_work_root_activity_transcript(
        app.clone(),
        cookie.as_str(),
        &work_root_id,
        "agent:empty",
        "",
    )
    .await;
    assert_eq!(empty_status, StatusCode::OK);
    let empty: serde_json::Value =
        serde_json::from_str(&empty_body).expect("empty transcript JSON");
    assert_eq!(empty["status"], "empty");
    assert_eq!(empty["sourceStatus"], "missing");
    assert_eq!(empty["blocks"].as_array().expect("blocks").len(), 0);
    assert_eq!(empty["hasMore"], false);

    let (broken_status, broken_body) = fetch_work_root_activity_transcript(
        app,
        cookie.as_str(),
        &work_root_id,
        "agent:broken",
        "",
    )
    .await;
    assert_eq!(broken_status, StatusCode::OK);
    let broken: serde_json::Value =
        serde_json::from_str(&broken_body).expect("broken transcript JSON");
    assert_eq!(broken["status"], "unavailable");
    assert_eq!(broken["sourceStatus"], "degraded");
    assert!(broken["diagnostics"].as_array().expect("diagnostics").len() >= 1);
    assert!(!broken_body.contains("agent.json"));

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_degrades_malformed_records() {
    if skip_without_git("work_root_activity_route_degrades_malformed_records") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-malformed");
    let cache_home = temp_fixture_path("work-root-activity-malformed-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    // Healthy idle agent.
    write_agent_metadata(
        &agents_dir,
        "healthy",
        &serde_json::json!({
            "schema_version": 1,
            "name": "healthy",
            "backend": "codex",
            "status": "idle"
        }),
    );

    // Malformed agent.json must degrade only its own row.
    write_agent_metadata_raw(&agents_dir, "broken-meta", "{ this is not valid json");

    // Agent directory with no agent.json file at all.
    fs::create_dir_all(agents_dir.join("missing-meta")).expect("create missing-meta agent dir");

    // Valid metadata but unreadable current-call state.
    write_agent_metadata(
        &agents_dir,
        "broken-call",
        &serde_json::json!({
            "schema_version": 1,
            "name": "broken-call",
            "backend": "claude",
            "status": "running"
        }),
    );
    write_current_call(&agents_dir, "broken-call", "}{ not json either");

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity(app, cookie.as_str(), &work_root_id).await;
    // CONTRACT: malformed records degrade individual rows; the route still
    // succeeds instead of failing the whole projection.
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("degraded activity JSON");

    assert_eq!(value["status"], "degraded");
    assert_eq!(value["summary"]["total"], 4);
    assert_eq!(value["summary"]["unavailable"], 2);

    let items = value["items"].as_array().expect("activity items array");
    assert_eq!(items.len(), 4);
    let item_row = |item_id: &str| -> &serde_json::Value {
        items
            .iter()
            .find(|item| item["id"] == item_id)
            .unwrap_or_else(|| panic!("missing activity item {item_id}"))
    };
    let item_diagnostics_of = |item: &serde_json::Value| -> Vec<String> {
        item["diagnostics"]
            .as_array()
            .expect("item diagnostics array")
            .iter()
            .map(|entry| entry.as_str().expect("diagnostic string").to_owned())
            .collect()
    };
    let broken_call_item = item_row("agent:broken-call");
    assert_eq!(broken_call_item["status"], "running");
    assert_eq!(broken_call_item["attention"], true);
    assert_eq!(broken_call_item["transcript"]["status"], "empty");
    assert!(!item_diagnostics_of(broken_call_item).is_empty());
    let broken_meta_item = item_row("agent:broken-meta");
    assert_eq!(broken_meta_item["status"], "unavailable");
    assert_eq!(broken_meta_item["attention"], true);
    assert_eq!(broken_meta_item["transcript"]["status"], "unavailable");
    assert_eq!(broken_meta_item["transcript"]["available"], false);
    assert!(!item_diagnostics_of(broken_meta_item).is_empty());
    let missing_meta_item = item_row("agent:missing-meta");
    assert_eq!(missing_meta_item["status"], "unavailable");
    assert_eq!(missing_meta_item["attention"], true);
    assert_eq!(missing_meta_item["transcript"]["status"], "unavailable");
    assert!(!item_diagnostics_of(missing_meta_item).is_empty());
    let healthy_item = item_row("agent:healthy");
    assert_eq!(healthy_item["status"], "idle");
    assert_eq!(healthy_item["attention"], false);
    assert!(item_diagnostics_of(healthy_item).is_empty());

    let agents = value["agents"].as_array().expect("activity agents array");
    assert_eq!(agents.len(), 4);
    let agent_row = |agent_id: &str| -> &serde_json::Value {
        agents
            .iter()
            .find(|agent| agent["agentId"] == agent_id)
            .unwrap_or_else(|| panic!("missing agent row {agent_id}"))
    };
    let diagnostics_of = |agent: &serde_json::Value| -> Vec<String> {
        agent["diagnostics"]
            .as_array()
            .expect("diagnostics array")
            .iter()
            .map(|entry| entry.as_str().expect("diagnostic string").to_owned())
            .collect()
    };

    // Valid metadata, but the current-call record cannot be parsed.
    let broken_call = agent_row("broken-call");
    assert_eq!(broken_call["status"], "running");
    assert!(broken_call["currentCall"].is_null());
    assert!(!diagnostics_of(broken_call).is_empty());

    // Unreadable agent.json degrades the row to unavailable.
    let broken_meta = agent_row("broken-meta");
    assert_eq!(broken_meta["status"], "unavailable");
    assert!(broken_meta["name"].is_null());
    assert!(!diagnostics_of(broken_meta).is_empty());

    // Missing agent.json also degrades the row to unavailable.
    let missing_meta = agent_row("missing-meta");
    assert_eq!(missing_meta["status"], "unavailable");
    assert!(missing_meta["name"].is_null());
    assert!(missing_meta["currentCall"].is_null());
    assert!(!diagnostics_of(missing_meta).is_empty());

    // The healthy row is unaffected by sibling degradation.
    let healthy = agent_row("healthy");
    assert_eq!(healthy["status"], "idle");
    assert!(diagnostics_of(healthy).is_empty());

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "agent.json".to_owned(),
        "state.json".to_owned(),
        "session_id".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "activity response must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_returns_empty_for_git_workroot_without_agents_dir() {
    if skip_without_git(
        "work_root_activity_route_returns_empty_for_git_workroot_without_agents_dir",
    ) {
        return;
    }
    // A Git workRoot resolves a wsstate layout, but no `agents/` directory has
    // been created yet: `scan_named_agents` must short-circuit to an empty,
    // healthy projection rather than fail.
    let root = temp_fixture_path("work-root-activity-no-agents");
    let cache_home = temp_fixture_path("work-root-activity-no-agents-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    fs::create_dir_all(&cache_home).expect("create activity cache fixture root");
    init_git_repo(&root);

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity(app, cookie.as_str(), &work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("git no-agents activity JSON");

    assert_eq!(value["status"], "ok");
    assert_eq!(value["summary"]["total"], 0);
    assert_eq!(
        value["agents"]
            .as_array()
            .expect("activity agents array")
            .len(),
        0
    );

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_projects_linked_git_workroot_feed() {
    if skip_without_git("work_root_activity_route_projects_linked_git_workroot_feed") {
        return;
    }
    let base = temp_fixture_path("work-root-activity-linked-route");
    let cache_home = base.join("cache");
    let primary = base.join("primary");
    let linked = base.join("linked");
    fs::create_dir_all(&primary).expect("create primary workRoot");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "layout fixture\n").expect("write seed file");
    run_git(&primary, &["add", "README.md"]);
    run_git(&primary, &["commit", "-m", "seed"]);
    run_git(
        &primary,
        &[
            "worktree",
            "add",
            linked.to_str().expect("linked path utf-8"),
        ],
    );

    let linked_agents_dir = resolve_work_root_agents_dir(&cache_home, &linked)
        .expect("resolve linked wsstate agents dir");
    write_agent_metadata(
        &linked_agents_dir,
        "linked-reviewer",
        &serde_json::json!({
            "schema_version": 1,
            "name": "linked reviewer",
            "backend": "codex",
            "status": "running",
            "last_call_at": "2026-05-17T12:00:00Z"
        }),
    );

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let linked_work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &linked).await;

    let (status, body_text) =
        fetch_work_root_activity(app, cookie.as_str(), &linked_work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value = serde_json::from_str(&body_text).expect("linked feed JSON");
    assert_eq!(value["summary"]["total"], 1);
    assert_eq!(value["items"][0]["id"], "agent:linked-reviewer");
    assert_eq!(value["items"][0]["kind"], "namedAgent");
    assert_eq!(value["agents"][0]["agentId"], "linked-reviewer");

    remove_static_fixture(&base);
}

#[test]
fn work_root_activity_resolves_git_primary_and_linked_worktree_layout() {
    if skip_without_git("work_root_activity_resolves_git_primary_and_linked_worktree_layout") {
        return;
    }
    // Independently verify the wsstate layout derivation (not just route
    // self-consistency): the agents directory is `<cache>/proj/<key>/agents`,
    // the primary key is an 8-hex project key, and the linked-worktree key is
    // `<projectKey>@<worktreeId>` sharing that same project key.
    let base = temp_fixture_path("work-root-activity-layout");
    let cache_home = base.join("cache");
    let primary = base.join("primary");
    let linked = base.join("linked");
    fs::create_dir_all(&primary).expect("create primary workRoot");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "layout fixture\n").expect("write seed file");
    run_git(&primary, &["add", "README.md"]);
    run_git(&primary, &["commit", "-m", "seed"]);
    run_git(
        &primary,
        &[
            "worktree",
            "add",
            linked.to_str().expect("linked path utf-8"),
        ],
    );

    let primary_dir = resolve_work_root_agents_dir(&cache_home, &primary)
        .expect("resolve primary worktree layout");
    let linked_dir =
        resolve_work_root_agents_dir(&cache_home, &linked).expect("resolve linked worktree layout");

    // Layout shape: `<cache>/proj/<key>/agents`.
    assert!(primary_dir.starts_with(&cache_home));
    assert_eq!(primary_dir.file_name().expect("agents leaf"), "agents");
    let primary_key_dir = primary_dir.parent().expect("primary key dir");
    assert_eq!(
        primary_key_dir
            .parent()
            .expect("proj dir")
            .file_name()
            .expect("proj leaf"),
        "proj"
    );

    let primary_key = primary_key_dir
        .file_name()
        .and_then(|name| name.to_str())
        .expect("primary key utf-8")
        .to_owned();
    assert_eq!(
        primary_key.len(),
        8,
        "project key is an 8-hex digest prefix"
    );
    assert!(
        primary_key.bytes().all(|byte| byte.is_ascii_hexdigit()),
        "project key is hex: {primary_key}"
    );

    let linked_key = linked_dir
        .parent()
        .and_then(|dir| dir.file_name())
        .and_then(|name| name.to_str())
        .expect("linked key utf-8")
        .to_owned();
    let (linked_project, linked_worktree) = linked_key
        .split_once('@')
        .expect("linked worktree key joins project and worktree ids with '@'");
    assert_eq!(
        linked_project, primary_key,
        "linked worktree key shares the primary project key"
    );
    assert_eq!(linked_worktree.len(), 8, "worktree id is an 8-hex prefix");
    assert!(linked_worktree.bytes().all(|byte| byte.is_ascii_hexdigit()));
    assert_ne!(
        linked_worktree, primary_key,
        "linked worktree id differs from the project key"
    );

    remove_static_fixture(&base);
}

#[test]
fn work_root_activity_rejects_non_git_and_bare_repository_layout() {
    if skip_without_git("work_root_activity_rejects_non_git_and_bare_repository_layout") {
        return;
    }
    let base = temp_fixture_path("work-root-activity-reject");
    let cache_home = base.join("cache");
    let plain = base.join("plain");
    let bare = base.join("bare.git");
    fs::create_dir_all(&plain).expect("create plain dir");
    fs::create_dir_all(&bare).expect("create bare dir");

    // A plain non-Git directory has no wsstate layout.
    assert!(
        resolve_work_root_agents_dir(&cache_home, &plain).is_none(),
        "non-Git directory must not resolve a wsstate agents dir"
    );

    // A bare repository is not a usable worktree and is rejected.
    run_git(&bare, &["init", "--bare"]);
    assert!(
        resolve_work_root_agents_dir(&cache_home, &bare).is_none(),
        "bare repository must not resolve a wsstate agents dir"
    );

    remove_static_fixture(&base);
}

#[tokio::test]
async fn work_root_file_listing_routes_reports_non_directory_target() {
    let root = temp_fixture_path("work-root-non-dir");
    fs::create_dir_all(&root).expect("create root dir");
    fs::write(root.join("file.txt"), "not a directory\n").expect("write file");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/files?path=file.txt"
                ))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("non-directory workRoot files request"),
        )
        .await
        .expect("non-directory workRoot files response");

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("non-directory body bytes");
    let body = String::from_utf8(body.to_vec()).expect("non-directory body is UTF-8");
    assert!(body.contains("not a directory"));
    assert!(!body.contains(&root.display().to_string()));

    remove_static_fixture(&root);
}

fn work_root_ids(value: &serde_json::Value) -> Vec<String> {
    value["workspaces"]
        .as_array()
        .map(|workspaces| {
            workspaces
                .iter()
                .flat_map(|workspace| {
                    workspace["workRoots"]
                        .as_array()
                        .cloned()
                        .unwrap_or_default()
                })
                .filter_map(|root| root["id"].as_str().map(str::to_owned))
                .collect()
        })
        .unwrap_or_default()
}

fn body_contains_workspace(value: &serde_json::Value, workspace_id: &str) -> bool {
    value["workspaces"]
        .as_array()
        .map(|workspaces| {
            workspaces
                .iter()
                .any(|workspace| workspace["id"] == workspace_id)
        })
        .unwrap_or(false)
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
                    serde_json::json!({
                        "path": root.display().to_string()
                    })
                    .to_string(),
                ))
                .expect("open workRoot request"),
        )
        .await
        .expect("open workRoot response");
    assert_eq!(response.status(), StatusCode::OK);
    let opened_header = response
        .headers()
        .get("x-ws-dashboard-opened-work-root-id")
        .and_then(|value| value.to_str().ok())
        .map(str::to_owned);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("open workRoot body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("open JSON");
    let opened_id = opened_header.expect("opened workRoot id header");
    assert!(
        work_root_ids(&value).iter().any(|id| id == &opened_id),
        "opened header id {opened_id} must be present in response body"
    );
    opened_id
}

async fn create_terminal_for_test(app: axum::Router, cookie: &str, work_root_id: &str) -> String {
    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/terminals"
                ))
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "columns": 80, "rows": 24, "title": "Test terminal" })
                        .to_string(),
                ))
                .expect("create terminal request"),
        )
        .await
        .expect("create terminal response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("create terminal body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("create terminal JSON");
    value["terminalId"]
        .as_str()
        .expect("terminal id")
        .to_owned()
}

async fn spawn_test_server(app: axum::Router) -> (String, tokio::task::JoinHandle<()>) {
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind test server");
    let addr = listener.local_addr().expect("test server addr");
    let handle = tokio::spawn(async move {
        axum::serve(listener, app).await.expect("test server");
    });
    (format!("127.0.0.1:{}", addr.port()), handle)
}

#[tokio::test]
async fn work_root_file_read_routes_are_owner_authenticated() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/root-local-test/files/read?path=README.md")
                .body(Body::empty())
                .expect("unauthenticated workRoot file read request"),
        )
        .await
        .expect("unauthenticated workRoot file read response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn work_root_file_read_routes_return_text_file() {
    let root = temp_fixture_path("work-root-read");
    fs::create_dir_all(root.join("src")).expect("create src dir");
    fs::write(root.join("src/main.rs"), "fn main() {}\n").expect("write rust file");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/files/read?path=src/main.rs"
                ))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("read workRoot file request"),
        )
        .await
        .expect("read workRoot file response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("read body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("read JSON");
    assert_eq!(value["workRootId"], work_root_id);
    assert_eq!(value["path"], "src/main.rs");
    assert_eq!(value["name"], "main.rs");
    assert_eq!(value["status"], "ok");
    assert_eq!(value["readOnly"], true);
    assert_eq!(value["content"], "fn main() {}\n");
    assert_eq!(value["sizeBytes"], 13);
    assert_eq!(value["extension"], "rs");
    assert_eq!(value["languageHint"], "rust");

    remove_static_fixture(&root);
}

#[tokio::test]
async fn work_root_file_read_routes_reject_traversal_without_path_leak() {
    let parent = temp_fixture_path("work-root-read-traversal");
    let root = parent.join("root");
    fs::create_dir_all(&root).expect("create root dir");
    fs::write(parent.join("outside.txt"), "secret\n").expect("write outside file");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/files/read?path=../outside.txt"
                ))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("traversal read request"),
        )
        .await
        .expect("traversal read response");

    assert_ne!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("traversal body bytes");
    let body = String::from_utf8(body.to_vec()).expect("traversal body is UTF-8");
    assert!(!body.contains("outside.txt"));
    assert!(!body.contains("secret"));
    assert!(!body.contains(&parent.display().to_string()));

    remove_static_fixture(&parent);
}

#[tokio::test]
async fn work_root_file_read_routes_report_unknown_work_root() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/root-local-unknown/files/read?path=README.md")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("unknown read request"),
        )
        .await
        .expect("unknown read response");

    assert_eq!(response.status(), StatusCode::NOT_FOUND);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("unknown body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("unknown JSON");
    assert_eq!(value["error"], "unknown workRoot");
}

#[tokio::test]
async fn work_root_file_read_routes_report_missing_directory_binary_and_oversized() {
    let root = temp_fixture_path("work-root-read-errors");
    fs::create_dir_all(root.join("src")).expect("create src dir");
    fs::write(root.join("binary.bin"), b"hello\0world").expect("write binary");
    fs::write(root.join("large.txt"), vec![b'a'; 600 * 1024]).expect("write large file");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let no_path_response = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/files/read"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("read no path request"),
        )
        .await
        .expect("read no path response");
    assert_eq!(no_path_response.status(), StatusCode::BAD_REQUEST);
    let no_path_body = axum::body::to_bytes(no_path_response.into_body(), 4096)
        .await
        .expect("no path body bytes");
    let no_path_value: serde_json::Value =
        serde_json::from_slice(&no_path_body).expect("no path JSON");
    assert_eq!(no_path_value["error"], "file path is required");

    for (path, status, error) in [
        ("", StatusCode::BAD_REQUEST, "file path is required"),
        ("missing.txt", StatusCode::NOT_FOUND, "file not found"),
        ("src", StatusCode::BAD_REQUEST, "path is a directory"),
        (
            "binary.bin",
            StatusCode::BAD_REQUEST,
            "unsupported text file",
        ),
        ("large.txt", StatusCode::BAD_REQUEST, "file is too large"),
    ] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(format!(
                        "/api/dashboard/work-roots/{work_root_id}/files/read?path={path}"
                    ))
                    .header(header::COOKIE, cookie.as_str())
                    .body(Body::empty())
                    .expect("read error request"),
            )
            .await
            .expect("read error response");
        assert_eq!(response.status(), status, "{path}");
        let body = axum::body::to_bytes(response.into_body(), 4096)
            .await
            .expect("error body bytes");
        let value: serde_json::Value = serde_json::from_slice(&body).expect("error JSON");
        assert_eq!(value["error"], error, "{path}");
        assert!(!String::from_utf8(body.to_vec())
            .expect("body UTF-8")
            .contains(&root.display().to_string()));
    }

    remove_static_fixture(&root);
}

#[cfg(unix)]
#[tokio::test]
async fn work_root_file_read_routes_report_unreadable_file() {
    let root = temp_fixture_path("work-root-read-unreadable");
    fs::create_dir_all(&root).expect("create root dir");
    let unreadable = root.join("unreadable.txt");
    fs::write(&unreadable, "hidden\n").expect("write unreadable file");
    let mut permissions = fs::metadata(&unreadable)
        .expect("unreadable metadata")
        .permissions();
    permissions.set_mode(0o000);
    fs::set_permissions(&unreadable, permissions).expect("make file unreadable");

    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/files/read?path=unreadable.txt"
                ))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("unreadable read request"),
        )
        .await
        .expect("unreadable read response");

    assert_eq!(response.status(), StatusCode::FORBIDDEN);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("unreadable body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("unreadable JSON");
    assert_eq!(value["error"], "file unavailable");

    let mut permissions = fs::metadata(&unreadable)
        .expect("unreadable metadata after read")
        .permissions();
    permissions.set_mode(0o644);
    fs::set_permissions(&unreadable, permissions).expect("restore unreadable permissions");
    remove_static_fixture(&root);
}

#[tokio::test]
async fn work_root_terminal_routes_are_owner_authenticated() {
    let app = build_router(app_state());
    let requests = [
        Request::builder()
            .method(Method::POST)
            .uri("/api/dashboard/work-roots/root-local-test/terminals")
            .body(Body::from("{}"))
            .expect("create terminal request"),
        Request::builder()
            .uri("/api/dashboard/work-roots/root-local-test/terminals")
            .body(Body::empty())
            .expect("list terminal request"),
        Request::builder()
            .uri("/api/dashboard/terminals/term_test/output")
            .body(Body::empty())
            .expect("output terminal request"),
        Request::builder()
            .method(Method::POST)
            .uri("/api/dashboard/terminals/term_test/input")
            .body(Body::from("{}"))
            .expect("input terminal request"),
        Request::builder()
            .method(Method::POST)
            .uri("/api/dashboard/terminals/term_test/resize")
            .body(Body::from("{}"))
            .expect("resize terminal request"),
        Request::builder()
            .uri("/api/dashboard/terminals/term_test/socket")
            .header(header::UPGRADE, "websocket")
            .header(header::CONNECTION, "upgrade")
            .header(header::HOST, "127.0.0.1")
            .body(Body::empty())
            .expect("terminal websocket request"),
        Request::builder()
            .method(Method::DELETE)
            .uri("/api/dashboard/terminals/term_test")
            .body(Body::empty())
            .expect("close terminal request"),
    ];

    for request in requests {
        let response = app
            .clone()
            .oneshot(request)
            .await
            .expect("unauthenticated terminal response");
        assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
    }
}

#[tokio::test]
async fn work_root_terminal_routes_reject_unknown_work_root() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    for request in [
        Request::builder()
            .method(Method::POST)
            .uri("/api/dashboard/work-roots/root-local-missing/terminals")
            .header(header::COOKIE, cookie.as_str())
            .header(header::CONTENT_TYPE, "application/json")
            .body(Body::from("{}"))
            .expect("unknown create terminal request"),
        Request::builder()
            .uri("/api/dashboard/work-roots/root-local-missing/terminals")
            .header(header::COOKIE, cookie.as_str())
            .body(Body::empty())
            .expect("unknown list terminal request"),
    ] {
        let response = app
            .clone()
            .oneshot(request)
            .await
            .expect("unknown workRoot terminal response");
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
        let body = axum::body::to_bytes(response.into_body(), 4096)
            .await
            .expect("unknown terminal body bytes");
        let value: serde_json::Value =
            serde_json::from_slice(&body).expect("unknown terminal JSON");
        assert_eq!(value["error"], "unknown workRoot");
    }
}

#[tokio::test]
async fn work_root_terminal_routes_create_list_output_input_resize_and_close() {
    let root = temp_fixture_path("terminal-root");
    fs::create_dir_all(&root).expect("create terminal root dir");
    fs::create_dir_all(root.join("nested")).expect("create terminal nested cwd dir");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let invalid_create = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/terminals"
                ))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "columns": 0, "rows": 24 }).to_string(),
                ))
                .expect("invalid create terminal request"),
        )
        .await
        .expect("invalid create terminal response");
    assert_eq!(invalid_create.status(), StatusCode::BAD_REQUEST);

    let invalid_cwd_create = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/terminals"
                ))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "columns": 80, "rows": 24, "cwdHint": "../outside" })
                        .to_string(),
                ))
                .expect("invalid cwd terminal request"),
        )
        .await
        .expect("invalid cwd terminal response");
    assert_eq!(invalid_cwd_create.status(), StatusCode::BAD_REQUEST);

    let create = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/terminals"
                ))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "columns": 80, "rows": 24, "title": "Test terminal", "cwdHint": "nested" })
                        .to_string(),
                ))
                .expect("create terminal request"),
        )
        .await
        .expect("create terminal response");
    assert_eq!(create.status(), StatusCode::OK);
    let create_body = axum::body::to_bytes(create.into_body(), 4096)
        .await
        .expect("create terminal body bytes");
    let created: serde_json::Value =
        serde_json::from_slice(&create_body).expect("create terminal JSON");
    let terminal_id = created["terminalId"]
        .as_str()
        .expect("terminal id")
        .to_owned();
    assert!(terminal_id.starts_with("term_"));
    assert_eq!(created["workRootId"], work_root_id);
    assert_eq!(created["status"], "running");
    assert_eq!(created["columns"], 80);
    assert_eq!(created["rows"], 24);
    assert_eq!(created["cwdHint"], "nested");
    assert!(!create_body
        .windows(root.display().to_string().len())
        .any(|window| window == root.display().to_string().as_bytes()));

    let list = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/terminals"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("list terminal request"),
        )
        .await
        .expect("list terminal response");
    assert_eq!(list.status(), StatusCode::OK);
    let list_body = axum::body::to_bytes(list.into_body(), 4096)
        .await
        .expect("list terminal body bytes");
    let listed: serde_json::Value = serde_json::from_slice(&list_body).expect("list terminal JSON");
    assert_eq!(listed.as_array().expect("terminal array").len(), 1);
    assert_eq!(listed[0]["terminalId"], terminal_id);

    let invalid_resize = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/terminals/{terminal_id}/resize"))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "columns": 301, "rows": 30 }).to_string(),
                ))
                .expect("invalid resize terminal request"),
        )
        .await
        .expect("invalid resize terminal response");
    assert_eq!(invalid_resize.status(), StatusCode::BAD_REQUEST);

    let resized = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/terminals/{terminal_id}/resize"))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "columns": 100, "rows": 30 }).to_string(),
                ))
                .expect("resize terminal request"),
        )
        .await
        .expect("resize terminal response");
    assert_eq!(resized.status(), StatusCode::OK);
    let resized_body = axum::body::to_bytes(resized.into_body(), 4096)
        .await
        .expect("resize terminal body bytes");
    let resized: serde_json::Value = serde_json::from_slice(&resized_body).expect("resize JSON");
    assert_eq!(resized["columns"], 100);
    assert_eq!(resized["rows"], 30);

    let input = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/terminals/{terminal_id}/input"))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "data": terminal_test_commands_for_current_platform("ws-terminal-test").echo_and_exit })
                        .to_string(),
                ))
                .expect("input terminal request"),
        )
        .await
        .expect("input terminal response");
    assert_eq!(input.status(), StatusCode::NO_CONTENT);

    let mut output_text = String::new();
    for _ in 0..40 {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(format!(
                        "/api/dashboard/terminals/{terminal_id}/output?after=0"
                    ))
                    .header(header::COOKIE, cookie.as_str())
                    .body(Body::empty())
                    .expect("output terminal request"),
            )
            .await
            .expect("output terminal response");
        assert_eq!(response.status(), StatusCode::OK);
        let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
            .await
            .expect("output body bytes");
        let value: serde_json::Value = serde_json::from_slice(&body).expect("output JSON");
        output_text = value["chunks"]
            .as_array()
            .expect("chunks array")
            .iter()
            .filter_map(|chunk| chunk["data"].as_str())
            .collect::<Vec<_>>()
            .join("");
        if output_text.contains("ws-terminal-test") {
            break;
        }
        tokio::time::sleep(Duration::from_millis(50)).await;
    }
    assert!(output_text.contains("ws-terminal-test"), "{output_text:?}");

    let close = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!("/api/dashboard/terminals/{terminal_id}"))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("close terminal request"),
        )
        .await
        .expect("close terminal response");
    assert_eq!(close.status(), StatusCode::NO_CONTENT);

    let input_after_close = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/terminals/{terminal_id}/input"))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "data": "echo nope\n" }).to_string(),
                ))
                .expect("closed input request"),
        )
        .await
        .expect("closed input response");
    assert_eq!(input_after_close.status(), StatusCode::NOT_FOUND);

    let resize_after_close = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/terminals/{terminal_id}/resize"))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "columns": 80, "rows": 24 }).to_string(),
                ))
                .expect("closed resize request"),
        )
        .await
        .expect("closed resize response");
    assert_eq!(resize_after_close.status(), StatusCode::NOT_FOUND);

    let output_after_close = app
        .oneshot(
            Request::builder()
                .uri(format!("/api/dashboard/terminals/{terminal_id}/output"))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("closed output request"),
        )
        .await
        .expect("closed output response");
    assert_eq!(output_after_close.status(), StatusCode::NOT_FOUND);

    remove_static_fixture(&root);
}

#[tokio::test]
async fn terminal_websocket_attaches_for_owner_and_forwards_io_and_resize() {
    let root = temp_fixture_path("terminal-websocket-root");
    fs::create_dir_all(&root).expect("create terminal websocket root dir");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;
    let terminal_id = create_terminal_for_test(app.clone(), cookie.as_str(), &work_root_id).await;
    let (addr, server) = spawn_test_server(app.clone()).await;

    let mut request = format!("ws://{addr}/api/dashboard/terminals/{terminal_id}/socket")
        .into_client_request()
        .expect("websocket request");
    request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let (mut socket, response) = tokio_tungstenite::connect_async(request)
        .await
        .expect("connect terminal websocket");
    assert_eq!(response.status(), StatusCode::SWITCHING_PROTOCOLS);

    socket
        .send(TungsteniteMessage::Text(
            serde_json::json!({ "type": "resize", "columns": 100, "rows": 30 })
                .to_string()
                .into(),
        ))
        .await
        .expect("send websocket resize");
    for attempt in 0..40 {
        let list = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(format!(
                        "/api/dashboard/work-roots/{work_root_id}/terminals"
                    ))
                    .header(header::COOKIE, cookie.as_str())
                    .body(Body::empty())
                    .expect("list resized terminal request"),
            )
            .await
            .expect("list resized terminal response");
        assert_eq!(list.status(), StatusCode::OK);
        let body = axum::body::to_bytes(list.into_body(), 4096)
            .await
            .expect("list resized terminal body bytes");
        let value: serde_json::Value = serde_json::from_slice(&body).expect("list resized JSON");
        let listed = value.as_array().expect("listed terminals array");
        let terminal = listed
            .iter()
            .find(|value| value["terminalId"] == terminal_id)
            .expect("resized terminal listed");
        if terminal["columns"] == 100 && terminal["rows"] == 30 {
            break;
        }
        assert!(
            attempt < 39,
            "resize frame did not update daemon terminal size: {terminal:?}"
        );
        tokio::time::sleep(Duration::from_millis(25)).await;
    }

    socket
        .send(TungsteniteMessage::Text(
            serde_json::json!({ "type": "input", "data": terminal_test_commands_for_current_platform("WS-SOCKET-TEST").echo_and_exit })
                .to_string()
                .into(),
        ))
        .await
        .expect("send websocket input");

    let mut text = String::new();
    for _ in 0..80 {
        let Some(message) = tokio::time::timeout(Duration::from_secs(2), socket.next())
            .await
            .expect("websocket message timeout")
        else {
            break;
        };
        let message = message.expect("websocket message");
        let TungsteniteMessage::Text(payload) = message else {
            continue;
        };
        let value: serde_json::Value =
            serde_json::from_str(&payload).expect("websocket frame JSON");
        assert_eq!(value["terminalId"], terminal_id);
        match value["type"].as_str() {
            Some("output") => {
                text.push_str(value["chunk"]["data"].as_str().expect("output data"));
            }
            Some("exit") => break,
            Some("status") => {}
            other => panic!("unexpected websocket frame type: {other:?}"),
        }
        if text.contains("WS-SOCKET-TEST") {
            break;
        }
    }
    assert!(text.contains("WS-SOCKET-TEST"), "{text:?}");

    socket.close(None).await.expect("close websocket");
    server.abort();
    remove_static_fixture(&root);
}

#[tokio::test]
async fn terminal_websocket_rejects_unknown_and_closed_terminals_before_upgrade() {
    let root = temp_fixture_path("terminal-websocket-closed-root");
    fs::create_dir_all(&root).expect("create terminal websocket closed root dir");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;
    let terminal_id = create_terminal_for_test(app.clone(), cookie.as_str(), &work_root_id).await;

    let (addr, server) = spawn_test_server(app.clone()).await;
    let mut unknown_request = format!("ws://{addr}/api/dashboard/terminals/term_missing/socket")
        .into_client_request()
        .expect("unknown websocket request");
    unknown_request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let error = tokio_tungstenite::connect_async(unknown_request)
        .await
        .expect_err("unknown websocket rejects");
    match error {
        tokio_tungstenite::tungstenite::Error::Http(response) => {
            assert_eq!(response.status(), StatusCode::NOT_FOUND);
        }
        other => panic!("unexpected unknown websocket error: {other}"),
    }

    let _ = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/terminals/{terminal_id}/input"))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "data": "exit\n" }).to_string(),
                ))
                .expect("exit input request"),
        )
        .await
        .expect("exit input response");
    for _ in 0..40 {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(format!(
                        "/api/dashboard/terminals/{terminal_id}/output?after=0"
                    ))
                    .header(header::COOKIE, cookie.as_str())
                    .body(Body::empty())
                    .expect("poll exited terminal request"),
            )
            .await
            .expect("poll exited terminal response");
        let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
            .await
            .expect("poll exited terminal body bytes");
        let value: serde_json::Value = serde_json::from_slice(&body).expect("poll output JSON");
        if value["status"] != "running" {
            break;
        }
        tokio::time::sleep(Duration::from_millis(50)).await;
    }

    let mut closed_request = format!("ws://{addr}/api/dashboard/terminals/{terminal_id}/socket")
        .into_client_request()
        .expect("closed websocket request");
    closed_request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let error = tokio_tungstenite::connect_async(closed_request)
        .await
        .expect_err("closed websocket rejects");
    match error {
        tokio_tungstenite::tungstenite::Error::Http(response) => {
            assert_eq!(response.status(), StatusCode::GONE);
        }
        other => panic!("unexpected closed websocket error: {other}"),
    }

    server.abort();
    remove_static_fixture(&root);
}

#[tokio::test]
async fn instance_event_stream_route_is_owner_authenticated() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/instance-events/stream-devenv-main")
                .body(Body::empty())
                .expect("unauthenticated event stream request"),
        )
        .await
        .expect("unauthenticated event stream response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn instance_event_stream_route_returns_fixture_events_with_backfill() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/instance-events/stream-devenv-main")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("authenticated event stream request"),
        )
        .await
        .expect("authenticated event stream response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("event stream body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("event stream JSON");
    assert_eq!(value["streamId"], "stream-devenv-main");
    assert_eq!(value["events"].as_array().expect("events array").len(), 5);
    assert_eq!(
        value["events"][0]["resourcePath"]["workRootId"],
        "root-devenv-primary"
    );
    assert_eq!(value["events"][0]["streamId"], "stream-devenv-main");

    let backfill = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/instance-events/stream-devenv-main?after=0000000002")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("authenticated backfill request"),
        )
        .await
        .expect("authenticated backfill response");
    assert_eq!(backfill.status(), StatusCode::OK);
    let body = axum::body::to_bytes(backfill.into_body(), 64 * 1024)
        .await
        .expect("backfill body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("backfill JSON");
    assert_eq!(value["events"].as_array().expect("events array").len(), 3);
    assert_eq!(value["events"][0]["cursor"], "0000000003");

    let unknown_cursor = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/instance-events/stream-devenv-main?after=missing")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("authenticated unknown cursor request"),
        )
        .await
        .expect("authenticated unknown cursor response");
    assert_eq!(unknown_cursor.status(), StatusCode::OK);
    let body = axum::body::to_bytes(unknown_cursor.into_body(), 64 * 1024)
        .await
        .expect("unknown cursor body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("unknown cursor JSON");
    assert!(value["events"].as_array().expect("events array").is_empty());
}

#[tokio::test]
async fn instance_event_stream_route_reports_missing_stream() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/instance-events/missing")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("authenticated missing event stream request"),
        )
        .await
        .expect("authenticated missing event stream response");

    assert_eq!(response.status(), StatusCode::NOT_FOUND);
}

#[tokio::test]
async fn browser_auth_accepts_loopback_host_and_origin_with_owner_cookie() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let loopback_host = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::COOKIE, cookie.as_str())
                .header(header::HOST, "127.0.0.1:3000")
                .body(Body::empty())
                .expect("loopback host request"),
        )
        .await
        .expect("loopback host response");
    assert_eq!(loopback_host.status(), StatusCode::OK);

    let loopback_origin = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::COOKIE, cookie.as_str())
                .header(header::ORIGIN, "http://localhost:3000")
                .body(Body::empty())
                .expect("loopback origin request"),
        )
        .await
        .expect("loopback origin response");
    assert_eq!(loopback_origin.status(), StatusCode::OK);

    let loopback_host_and_origin = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::COOKIE, cookie.as_str())
                .header(header::HOST, "localhost:3000")
                .header(header::ORIGIN, "http://127.0.0.1:3000")
                .body(Body::empty())
                .expect("loopback host and origin request"),
        )
        .await
        .expect("loopback host and origin response");
    assert_eq!(loopback_host_and_origin.status(), StatusCode::OK);
}

#[tokio::test]
async fn browser_auth_rejects_invalid_host_and_origin_with_owner_cookie() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    for (header_name, header_value) in [
        (header::HOST, "evil.example.test"),
        (header::ORIGIN, "https://evil.example.test"),
    ] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri("/healthz")
                    .header(header::COOKIE, cookie.as_str())
                    .header(&header_name, header_value)
                    .body(Body::empty())
                    .expect("invalid browser entrypoint request"),
            )
            .await
            .expect("invalid browser entrypoint response");

        assert_eq!(response.status(), StatusCode::FORBIDDEN, "{header_name}");
    }
}

#[tokio::test]
async fn bearer_auth_can_access_http_smoke_routes_without_cookie() {
    let state = app_state();
    let bearer = state.auth.issue_bearer_token().as_authorization_header();
    let app = build_router(state);

    let response = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::AUTHORIZATION, bearer)
                .body(Body::empty())
                .expect("bearer health request"),
        )
        .await
        .expect("bearer health response");

    assert_eq!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn invalid_bearer_auth_cannot_access_http_smoke_routes_without_cookie() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::AUTHORIZATION, "Bearer not-the-owner-token")
                .body(Body::empty())
                .expect("invalid bearer health request"),
        )
        .await
        .expect("invalid bearer health response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
    assert!(response.headers().get(header::SET_COOKIE).is_none());
}

#[tokio::test]
async fn websocket_upgrade_requests_are_auth_gated_before_upgrade_acceptance() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/ws")
                .header(header::UPGRADE, "websocket")
                .header(header::CONNECTION, "upgrade")
                .header(header::HOST, "127.0.0.1")
                .body(Body::empty())
                .expect("websocket upgrade request"),
        )
        .await
        .expect("websocket upgrade response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn health_output_stays_minimal() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let session_value = cookie
        .split_once('=')
        .expect("cookie name and value")
        .1
        .to_owned();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("health request"),
        )
        .await
        .expect("health response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 1024)
        .await
        .expect("health body bytes");
    let body = std::str::from_utf8(&body).expect("health body utf8");

    assert_eq!(body, "ok\n");
    for forbidden in [
        token.as_str(),
        session_value.as_str(),
        "wsstate",
        "target",
        "cache",
        "git",
        "diagnostic",
    ] {
        assert!(!body.contains(forbidden));
    }
}

#[tokio::test]
async fn daemon_security_smoke_covers_auth_and_health_boundary() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);

    let unauthenticated_http = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .body(Body::empty())
                .expect("unauthenticated health request"),
        )
        .await
        .expect("unauthenticated health response");
    assert_eq!(unauthenticated_http.status(), StatusCode::UNAUTHORIZED);

    let unauthenticated_websocket = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/ws")
                .header(header::UPGRADE, "websocket")
                .header(header::CONNECTION, "upgrade")
                .header(header::HOST, "127.0.0.1")
                .body(Body::empty())
                .expect("unauthenticated websocket request"),
        )
        .await
        .expect("unauthenticated websocket response");
    assert_eq!(unauthenticated_websocket.status(), StatusCode::UNAUTHORIZED);

    let pair = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!("/pair?token={token}"))
                .body(Body::empty())
                .expect("pair request"),
        )
        .await
        .expect("pair response");
    assert_eq!(pair.status(), StatusCode::SEE_OTHER);
    assert_eq!(
        pair.headers().get(header::LOCATION),
        Some(&header::HeaderValue::from_static("/"))
    );
    let cookie = pair
        .headers()
        .get(header::SET_COOKIE)
        .expect("owner session cookie")
        .to_str()
        .expect("cookie header is ASCII")
        .split(';')
        .next()
        .expect("cookie pair")
        .to_owned();

    let reused_pair = pair_response(app.clone(), Some(&token)).await;
    assert_eq!(reused_pair.status(), StatusCode::GONE);
    assert!(reused_pair.headers().get(header::SET_COOKIE).is_none());

    let health = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("authenticated health request"),
        )
        .await
        .expect("authenticated health response");
    assert_eq!(health.status(), StatusCode::OK);
    let health_body = axum::body::to_bytes(health.into_body(), 1024)
        .await
        .expect("health body bytes");
    let health_body = std::str::from_utf8(&health_body).expect("health body utf8");
    assert_eq!(health_body, "ok\n");
    for forbidden in [
        token.as_str(),
        "wsstate",
        "target",
        "cache",
        "git",
        "diagnostic",
    ] {
        assert!(!health_body.contains(forbidden));
    }

    let ui = app
        .oneshot(
            Request::builder()
                .uri("/")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("authenticated UI request"),
        )
        .await
        .expect("authenticated UI response");
    assert_eq!(ui.status(), StatusCode::OK);
}
