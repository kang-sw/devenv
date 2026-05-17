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
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

use futures_util::{SinkExt, StreamExt};

use axum::body::Body;
use axum::http::{header, Method, Request, StatusCode};
use tokio::net::TcpListener;
use tokio_tungstenite::tungstenite::{client::IntoClientRequest, Message as TungsteniteMessage};
use tower::ServiceExt;
use ws_dashboard_daemon::auth::{OwnerAuthState, PairingTokenPolicy};
use ws_dashboard_daemon::config::ServeConfig;
use ws_dashboard_daemon::router::{build_router, AppState};
use ws_dashboard_daemon::terminal::TerminalRegistry;
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
    AppState {
        config: ServeConfig::default_loopback(),
        auth: OwnerAuthState::new_ephemeral(),
        opened_work_roots: OpenedWorkRoots::default(),
        terminals: TerminalRegistry::default(),
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
        terminals: TerminalRegistry::default(),
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
        terminals: TerminalRegistry::default(),
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
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("open workRoot body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("open JSON");
    value["workspaces"][0]["workRoots"][0]["id"]
        .as_str()
        .expect("workRoot id")
        .to_owned()
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
                    serde_json::json!({ "columns": 80, "rows": 24, "title": "Test terminal" })
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
