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
use rusqlite::{params, Connection};

use axum::body::Body;
use axum::http::{header, Method, Request, StatusCode};
use tokio::net::TcpListener;
use tokio::sync::Mutex;
use tokio::time::timeout;
use tokio_tungstenite::tungstenite::{client::IntoClientRequest, Message as TungsteniteMessage};
use tower::ServiceExt;
use ws_dashboard_core::{ServerId, ServerKind};
use ws_dashboard_daemon::auth::{OwnerAuthState, PairingTokenPolicy};
use ws_dashboard_daemon::config::ServeConfig;
use ws_dashboard_daemon::document_translation::{
    DocumentTranslationService, TranslationProviderConfig,
};
use ws_dashboard_daemon::persistent_state::{DashboardStateStore, PersistedLinkedServer};
use ws_dashboard_daemon::router::{build_router, AppState};
use ws_dashboard_daemon::servers::{LinkedServerSessions, LinkedServerTunnels};
use ws_dashboard_daemon::terminal::TerminalRegistry;
use ws_dashboard_daemon::work_root_activity::{
    resolve_work_root_agents_dir, WorkRootActivityProjectionConfig, WorkRootActivityProjector,
};
use ws_dashboard_daemon::work_root_files::{DocumentEventHub, OpenedWorkRoots};

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
        document_translation: DocumentTranslationService::default(),
        terminals: TerminalRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
        document_events: DocumentEventHub::default(),
        document_write_locks: ws_dashboard_daemon::work_root_files::DocumentWriteLocks::default(),
        linked_server_sessions: LinkedServerSessions::default(),
        linked_server_tunnels: LinkedServerTunnels::record_only_for_tests(),
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
        document_translation: DocumentTranslationService::default(),
        terminals: TerminalRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
        document_events: DocumentEventHub::default(),
        document_write_locks: ws_dashboard_daemon::work_root_files::DocumentWriteLocks::default(),
        linked_server_sessions: LinkedServerSessions::default(),
        linked_server_tunnels: LinkedServerTunnels::record_only_for_tests(),
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
        document_translation: DocumentTranslationService::default(),
        terminals: TerminalRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
        document_events: DocumentEventHub::default(),
        document_write_locks: ws_dashboard_daemon::work_root_files::DocumentWriteLocks::default(),
        linked_server_sessions: LinkedServerSessions::default(),
        linked_server_tunnels: LinkedServerTunnels::record_only_for_tests(),
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
async fn dashboard_resources_discovers_linked_git_worktrees_from_opened_primary() {
    if skip_without_git("dashboard_resources_discovers_linked_git_worktrees_from_opened_primary") {
        return;
    }
    let base = temp_fixture_path("resources-linked-worktree");
    let primary = base.join("primary");
    let linked = base.join("linked");
    fs::create_dir_all(&primary).expect("create primary workRoot");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "dashboard\n").expect("write seed file");
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

    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let primary_id = open_work_root_for_test(app.clone(), cookie.as_str(), &primary).await;

    let value = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    let roots = value["workspaces"][0]["workRoots"]
        .as_array()
        .expect("workRoots array");
    assert_eq!(roots.len(), 2);
    assert!(roots
        .iter()
        .any(|root| root["id"] == primary_id && root["kind"] == "gitPrimaryRoot"));
    let linked_root = roots
        .iter()
        .find(|root| root["kind"] == "gitLinkedWorktree")
        .expect("linked worktree root");
    let linked_id = linked_root["id"].as_str().expect("linked id");
    assert!(
        !String::from_utf8_lossy(&serde_json::to_vec(&value).expect("encode resources"))
            .contains(linked.to_string_lossy().as_ref())
    );

    let files = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!("/api/dashboard/work-roots/{linked_id}/files"))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("linked files request"),
        )
        .await
        .expect("linked files response");
    assert_eq!(files.status(), StatusCode::OK);

    let offline_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/work-roots/{linked_id}/activation"))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "activation": "offline" }).to_string(),
                ))
                .expect("linked offline activation request"),
        )
        .await
        .expect("linked offline activation response");
    assert_eq!(offline_response.status(), StatusCode::OK);
    let offline_body = axum::body::to_bytes(offline_response.into_body(), 64 * 1024)
        .await
        .expect("linked offline activation body");
    let offline_value: serde_json::Value =
        serde_json::from_slice(&offline_body).expect("linked offline activation JSON");
    let offline_root = work_root_by_id(&offline_value, linked_id);
    assert_eq!(offline_root["activation"], "offline");
    assert_eq!(offline_root["state"]["status"], "offline");
    assert!(offline_root["actions"]
        .as_array()
        .expect("actions array")
        .iter()
        .any(|action| action["id"] == "workRoot.activation.online" && action["enabled"] == true));

    let activity = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!("/api/dashboard/work-roots/{linked_id}/activity"))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("linked offline activity request"),
        )
        .await
        .expect("linked offline activity response");
    assert_eq!(activity.status(), StatusCode::CONFLICT);

    let online_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/work-roots/{linked_id}/activation"))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "activation": "online" }).to_string(),
                ))
                .expect("linked online activation request"),
        )
        .await
        .expect("linked online activation response");
    assert_eq!(online_response.status(), StatusCode::OK);
    let online_body = axum::body::to_bytes(online_response.into_body(), 64 * 1024)
        .await
        .expect("linked online activation body");
    let online_value: serde_json::Value =
        serde_json::from_slice(&online_body).expect("linked online activation JSON");
    let online_root = work_root_by_id(&online_value, linked_id);
    assert_eq!(online_root["activation"], "online");
    assert!(online_root["actions"]
        .as_array()
        .expect("actions array")
        .iter()
        .any(|action| action["id"] == "workRoot.activation.offline" && action["enabled"] == true));

    remove_static_fixture(&base);
}

#[tokio::test]
async fn dashboard_resources_uses_registry_activation_for_opened_discovered_duplicates() {
    if skip_without_git(
        "dashboard_resources_uses_registry_activation_for_opened_discovered_duplicates",
    ) {
        return;
    }
    let base = temp_fixture_path("resources-duplicate-activation");
    let primary = base.join("a-primary");
    let linked = base.join("z-linked");
    fs::create_dir_all(&primary).expect("create primary workRoot");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "dashboard\n").expect("write seed file");
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

    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let _primary_id = open_work_root_for_test(app.clone(), cookie.as_str(), &primary).await;
    let linked_id = open_work_root_for_test(app.clone(), cookie.as_str(), &linked).await;

    let offline_value =
        set_work_root_activation_for_test(app.clone(), cookie.as_str(), &linked_id, "offline")
            .await;
    let offline_root = work_root_by_id(&offline_value, &linked_id);
    assert_eq!(offline_root["activation"], "offline");

    let refreshed = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    let refreshed_root = work_root_by_id(&refreshed, &linked_id);
    assert_eq!(
        refreshed_root["activation"], "offline",
        "the discovered sibling row must reuse the opened registry activation"
    );
    assert!(refreshed_root["actions"]
        .as_array()
        .expect("actions array")
        .iter()
        .any(|action| action["id"] == "workRoot.activation.online" && action["enabled"] == true));

    let activity = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!("/api/dashboard/work-roots/{linked_id}/activity"))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("duplicate offline activity request"),
        )
        .await
        .expect("duplicate offline activity response");
    assert_eq!(
        activity.status(),
        StatusCode::CONFLICT,
        "route gate and resource projection must agree on offline activation"
    );

    remove_static_fixture(&base);
}

#[tokio::test]
async fn dashboard_resources_refresh_prunes_workspace_without_available_work_roots() {
    let root = temp_fixture_path("refresh-recompute");
    fs::create_dir_all(&root).expect("create refresh root");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let offline_response = app
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
    assert_eq!(offline_response.status(), StatusCode::OK);

    let available = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    let root_view = only_work_root(&available);
    assert_eq!(root_view["id"], work_root_id);
    assert_eq!(root_view["activation"], "offline");
    assert_eq!(root_view["availability"], "available");

    fs::remove_dir_all(&root).expect("remove refresh root after registry membership");
    let missing = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    assert!(
        missing["workspaces"]
            .as_array()
            .expect("workspaces array")
            .is_empty(),
        "no-active-workRoot refresh prunes the unavailable workspace"
    );

    fs::create_dir_all(&root).expect("restore refresh root");
    let restored = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    assert!(
        restored["workspaces"]
            .as_array()
            .expect("workspaces array")
            .is_empty(),
        "pruned workspaces reappear only after an explicit open"
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
    assert!(
        value["workspaces"]
            .as_array()
            .expect("workspaces array")
            .is_empty(),
        "resource refresh prunes the missing root workspace"
    );

    let response = app.clone();
    for (uri, expected_error) in [
        (
            format!("/api/dashboard/work-roots/{work_root_id}/files"),
            "unknown workRoot",
        ),
        (
            format!("/api/dashboard/work-roots/{work_root_id}/files/read?path=README.md"),
            "unknown workRoot",
        ),
        (
            format!("/api/dashboard/work-roots/{work_root_id}/activity"),
            "unknown workRoot",
        ),
        (
            format!(
                "/api/dashboard/work-roots/{work_root_id}/activity/items/agent:test/transcript"
            ),
            "unknown workRoot",
        ),
        (
            format!("/api/dashboard/work-roots/{work_root_id}/activity/events"),
            "unknown workRoot",
        ),
        (
            format!("/api/dashboard/work-roots/{work_root_id}/terminals"),
            "unknown workRoot",
        ),
        (
            format!("/api/dashboard/terminals/{terminal_id}/output"),
            "unknown terminal",
        ),
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

        assert_eq!(response.status(), StatusCode::NOT_FOUND);
        let body = axum::body::to_bytes(response.into_body(), 4096)
            .await
            .expect("unavailable body");
        let value: serde_json::Value = serde_json::from_slice(&body).expect("unavailable JSON");
        assert_eq!(value["error"], expected_error);
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
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
        let body = axum::body::to_bytes(response.into_body(), 4096)
            .await
            .expect("terminal unavailable body");
        let value: serde_json::Value =
            serde_json::from_slice(&body).expect("terminal unavailable JSON");
        assert!(["unknown workRoot", "unknown terminal"]
            .contains(&value["error"].as_str().expect("error string")));
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
    assert_eq!(delete_response.status(), StatusCode::NOT_FOUND);
    let delete_body = axum::body::to_bytes(delete_response.into_body(), 4096)
        .await
        .expect("delete unavailable body");
    let value: serde_json::Value =
        serde_json::from_slice(&delete_body).expect("delete unavailable JSON");
    assert_eq!(value["error"], "unknown terminal");
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
            assert_eq!(response.status(), StatusCode::NOT_FOUND);
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
    assert_eq!(cleanup.status(), StatusCode::NOT_FOUND);
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
            .uri("/api/dashboard/root-picker/pins")
            .body(Body::empty())
            .expect("pin directory request"),
        Request::builder()
            .method(Method::DELETE)
            .uri("/api/dashboard/root-picker/pins")
            .body(Body::empty())
            .expect("unpin directory request"),
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
        Request::builder()
            .method(Method::DELETE)
            .uri("/api/dashboard/workspaces/workspace-local-test")
            .body(Body::empty())
            .expect("remove workspace request"),
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
    assert_eq!(entries[0]["kindLabel"], "Folder");
    assert!(entries[0]["modifiedTime"].as_str().is_some());
    assert!(entries[0]["size"].is_null());
    assert_eq!(entries[1]["name"], "zeta");
    let places = value["places"].as_array().expect("known places array");
    assert!(
        !places.is_empty(),
        "daemon-derived picker places are exposed"
    );
    assert!(
        places.iter().all(|place| place["available"] == true),
        "unavailable picker places are filtered by the daemon"
    );
    assert!(
        places
            .iter()
            .any(|place| place["kind"] == "home" || place["kind"] == "root"),
        "known places include a platform root or home directory"
    );
    assert!(
        places.iter().all(|place| place["source"] == "builtIn"),
        "initial known places are distinguished from owner pins"
    );

    remove_static_fixture(&root);
}

#[tokio::test]
async fn root_picker_pins_round_trip_through_dashboard_state() {
    let root = write_root_picker_fixture();
    let missing = temp_fixture_path("missing-pin").join("gone");
    let state_file_directory = temp_fixture_path("picker-pins-state");
    let store = DashboardStateStore::at_path(state_file_directory.join("opened-workroots.json"));
    let state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    for pin in [&root, &missing] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method(Method::POST)
                    .uri("/api/dashboard/root-picker/pins")
                    .header(header::COOKIE, cookie.as_str())
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::from(
                        serde_json::json!({ "path": pin.display().to_string() }).to_string(),
                    ))
                    .expect("pin directory request"),
            )
            .await
            .expect("pin directory response");
        assert_eq!(response.status(), StatusCode::OK);
    }

    let persisted_pins = store.load_root_picker_pins().await;
    assert_eq!(persisted_pins.len(), 2);
    assert!(persisted_pins.contains(&root));
    assert!(persisted_pins.contains(&missing));

    let restarted_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let restarted_token = restarted_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let restarted_app = build_router(restarted_state);
    let restarted_cookie = pair_and_cookie(restarted_app.clone(), &restarted_token).await;
    let response = restarted_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/root-picker?path={}",
                    root.display()
                ))
                .header(header::COOKIE, restarted_cookie.as_str())
                .body(Body::empty())
                .expect("root picker with pins request"),
        )
        .await
        .expect("root picker with pins response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("root picker pins body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("root picker pins JSON");
    let pins: Vec<_> = value["places"]
        .as_array()
        .expect("places array")
        .iter()
        .filter(|place| place["source"] == "pin")
        .collect();
    assert_eq!(pins.len(), 2);
    let root_canonical = root.canonicalize().unwrap().display().to_string();
    let missing_display = missing.display().to_string();
    assert!(pins.iter().any(|place| {
        place["path"].as_str() == Some(root_canonical.as_str()) && place["available"] == true
    }));
    assert!(pins.iter().any(|place| {
        place["path"].as_str() == Some(missing_display.as_str()) && place["available"] == false
    }));

    let unpin = restarted_app
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri("/api/dashboard/root-picker/pins")
                .header(header::COOKIE, restarted_cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "path": root.canonicalize().unwrap().display().to_string() })
                        .to_string(),
                ))
                .expect("unpin directory request"),
        )
        .await
        .expect("unpin directory response");
    assert_eq!(unpin.status(), StatusCode::OK);

    remove_static_fixture(&root);
    remove_static_fixture(&state_file_directory);
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
async fn dashboard_servers_api_lists_local_and_persisted_linked_servers() {
    let state_file_root = temp_fixture_path("servers-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-windows"),
            label: "Windows dogfood".to_owned(),
            kind: ServerKind::SshRemote,
            ssh_target: Some("owner@example.test".to_owned()),
            endpoint_hint: Some("http://127.0.0.1:4100".to_owned()),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("persist linked server seed");
    let state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("authenticated servers request"),
        )
        .await
        .expect("authenticated servers response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("servers body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("servers JSON body");
    assert_eq!(value["servers"][0]["id"], "server-local");
    assert_eq!(value["servers"][0]["status"], "connected");
    assert_eq!(value["servers"][1]["id"], "server-windows");
    assert_eq!(value["servers"][1]["kind"], "sshRemote");
    assert_eq!(value["servers"][1]["status"], "authRequired");
    assert_eq!(value["servers"][1]["actions"][0]["id"], "enterPassphrase");
    assert!(
        !body
            .windows(b"owner@example.test".len())
            .any(|window| window == b"owner@example.test"),
        "server list must not expose SSH target"
    );

    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn ssh_server_start_persists_tunnel_metadata_without_exposing_secrets() {
    let state_file_root = temp_fixture_path("ssh-server-start-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    let state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/ssh/start")
                .header(header::COOKIE, cookie.clone())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "serverId": "server-windows",
                        "label": "Windows dogfood",
                        "sshTarget": "owner@example.test",
                        "remoteEndpoint": "http://127.0.0.1:4100",
                        "localPort": 49155
                    })
                    .to_string(),
                ))
                .expect("ssh server start request"),
        )
        .await
        .expect("ssh server start response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("ssh server start body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("ssh start JSON");
    assert_eq!(value["id"], "server-windows");
    assert_eq!(value["status"], "authRequired");
    assert_eq!(value["actions"][0]["id"], "enterPassphrase");
    assert!(
        !body
            .windows(b"owner@example.test".len())
            .any(|window| window == b"owner@example.test"),
        "start response must not expose SSH target"
    );
    assert!(
        !body
            .windows(b"49155".len())
            .any(|window| window == b"49155"),
        "start response must not expose local forwarded endpoint"
    );

    let restored = store.load_linked_servers().await;
    assert_eq!(restored.len(), 1);
    assert_eq!(restored[0].id.as_str(), "server-windows");
    assert_eq!(
        restored[0].ssh_target.as_deref(),
        Some("owner@example.test")
    );
    assert_eq!(
        restored[0].endpoint_hint.as_deref(),
        Some("http://127.0.0.1:49155")
    );
    assert_eq!(
        restored[0].remote_endpoint_hint.as_deref(),
        Some("http://127.0.0.1:4100")
    );

    let servers = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("servers after ssh start request"),
        )
        .await
        .expect("servers after ssh start response");
    let servers_body = axum::body::to_bytes(servers.into_body(), 64 * 1024)
        .await
        .expect("servers after ssh start body");
    let servers_value: serde_json::Value =
        serde_json::from_slice(&servers_body).expect("servers after ssh start JSON");
    assert_eq!(servers_value["servers"][1]["status"], "authRequired");

    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn ssh_server_start_captures_remote_startup_and_links_with_passphrase() {
    let remote_state = app_state();
    let passphrase = remote_state
        .auth
        .link_passphrase()
        .expose_for_owner_record()
        .to_owned();
    let remote_app = build_router(remote_state);
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;
    let remote_port = remote_addr
        .rsplit_once(':')
        .and_then(|(_, port)| port.parse::<u16>().ok())
        .expect("remote test port");

    let state_file_root = temp_fixture_path("ssh-server-start-capture-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    let state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let startup_output = format!(
        "ws-dashboard owner pairing URL: http://127.0.0.1:{remote_port}/pair?token=redacted\nws-dashboard remote link passphrase: {passphrase}\n"
    );

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/ssh/start")
                .header(header::COOKIE, cookie.clone())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "serverId": "server-windows",
                        "label": "Windows dogfood",
                        "sshTarget": "owner@example.test",
                        "startupCommand": startup_output,
                        "localPort": remote_port
                    })
                    .to_string(),
                ))
                .expect("ssh server start capture request"),
        )
        .await
        .expect("ssh server start capture response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("ssh server start capture body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("ssh capture JSON");
    assert_eq!(value["id"], "server-windows");
    assert_eq!(value["status"], "connected");
    assert!(
        !body
            .windows(passphrase.as_bytes().len())
            .any(|window| window == passphrase.as_bytes()),
        "start response must not expose captured passphrase"
    );

    let resources = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-windows/resources")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("linked resources after startup capture request"),
        )
        .await
        .expect("linked resources after startup capture response");
    assert_eq!(resources.status(), StatusCode::OK);
    let resources_body = axum::body::to_bytes(resources.into_body(), 64 * 1024)
        .await
        .expect("linked resources after startup capture body");
    let resources_value: serde_json::Value =
        serde_json::from_slice(&resources_body).expect("linked resources JSON");
    assert_eq!(resources_value["server"]["id"], "server-windows");

    remote_server.abort();
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn linked_server_reconnect_restores_tunnel_required_after_restart() {
    let state_file_root = temp_fixture_path("ssh-reconnect-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-windows"),
            label: "Windows dogfood".to_owned(),
            kind: ServerKind::SshRemote,
            ssh_target: Some("owner@example.test".to_owned()),
            endpoint_hint: Some("http://127.0.0.1:49155".to_owned()),
            remote_endpoint_hint: Some("http://127.0.0.1:4100".to_owned()),
        }])
        .await
        .expect("persist linked server seed");
    let state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let servers = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers")
                .header(header::COOKIE, cookie.clone())
                .body(Body::empty())
                .expect("servers before reconnect request"),
        )
        .await
        .expect("servers before reconnect response");
    let servers_body = axum::body::to_bytes(servers.into_body(), 64 * 1024)
        .await
        .expect("servers before reconnect body");
    let servers_value: serde_json::Value =
        serde_json::from_slice(&servers_body).expect("servers before reconnect JSON");
    assert_eq!(servers_value["servers"][1]["status"], "tunnelRequired");
    assert_eq!(
        servers_value["servers"][1]["actions"][0]["id"],
        "reconnectTunnel"
    );

    let reconnected = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/server-windows/tunnel/reconnect")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("server reconnect request"),
        )
        .await
        .expect("server reconnect response");

    assert_eq!(reconnected.status(), StatusCode::OK);
    let reconnected_body = axum::body::to_bytes(reconnected.into_body(), 64 * 1024)
        .await
        .expect("server reconnect body");
    let reconnected_value: serde_json::Value =
        serde_json::from_slice(&reconnected_body).expect("server reconnect JSON");
    assert_eq!(reconnected_value["status"], "authRequired");
    assert_eq!(reconnected_value["actions"][0]["id"], "enterPassphrase");

    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn server_scoped_resources_route_dispatches_local_and_refuses_linked_servers() {
    let root = temp_fixture_path("server-scoped-resources");
    let state_file_root = temp_fixture_path("server-scoped-resources-state");
    fs::create_dir_all(&root).expect("create server scoped workRoot");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-windows"),
            label: "Windows dogfood".to_owned(),
            kind: ServerKind::SshRemote,
            ssh_target: Some("owner@example.test".to_owned()),
            endpoint_hint: Some("http://127.0.0.1:4100".to_owned()),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("persist linked server seed");
    let state =
        app_state_with_opened_and_store(OpenedWorkRoots::from_paths(vec![root.clone()]), store);
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let local = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-local/resources")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("authenticated local server resources request"),
        )
        .await
        .expect("authenticated local server resources response");
    assert_eq!(local.status(), StatusCode::OK);
    let local_body = axum::body::to_bytes(local.into_body(), 64 * 1024)
        .await
        .expect("local server resources body bytes");
    let local_value: serde_json::Value =
        serde_json::from_slice(&local_body).expect("local server resources JSON");
    assert_eq!(local_value["server"]["id"], "server-local");
    assert_eq!(
        local_value["workspaces"][0]["workRoots"][0]["label"],
        root.file_name()
            .and_then(|name| name.to_str())
            .expect("server scoped root filename")
    );

    let linked = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-windows/resources")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("authenticated linked server resources request"),
        )
        .await
        .expect("authenticated linked server resources response");
    assert_eq!(linked.status(), StatusCode::CONFLICT);
    let linked_body = axum::body::to_bytes(linked.into_body(), 64 * 1024)
        .await
        .expect("linked server resources body bytes");
    let linked_value: serde_json::Value =
        serde_json::from_slice(&linked_body).expect("linked server resources JSON");
    assert_eq!(linked_value["error"], "linked server auth required");
    assert!(
        !linked_body
            .windows(b"owner@example.test".len())
            .any(|window| window == b"owner@example.test"),
        "linked refusal must not expose SSH target"
    );

    let unknown = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-missing/resources")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("authenticated unknown server resources request"),
        )
        .await
        .expect("authenticated unknown server resources response");
    assert_eq!(unknown.status(), StatusCode::NOT_FOUND);

    remove_static_fixture(&root);
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn server_scoped_one_shot_routes_are_protected_and_dispatch_local_aliases() {
    let root = temp_fixture_path("server-scoped-root-picker-local");
    let child = root.join("child");
    fs::create_dir_all(&child).expect("create server scoped root picker fixture");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);

    let unauthenticated = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-local/root-picker?path={}",
                    root.display()
                ))
                .body(Body::empty())
                .expect("unauthenticated server scoped root-picker request"),
        )
        .await
        .expect("unauthenticated server scoped root-picker response");
    assert_eq!(unauthenticated.status(), StatusCode::UNAUTHORIZED);

    let cookie = pair_and_cookie(app.clone(), &token).await;
    let legacy = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/root-picker?path={}",
                    root.display()
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("legacy root-picker request"),
        )
        .await
        .expect("legacy root-picker response");
    assert_eq!(legacy.status(), StatusCode::OK);
    let legacy_body = axum::body::to_bytes(legacy.into_body(), 64 * 1024)
        .await
        .expect("legacy root-picker body");

    let scoped = app
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-local/root-picker?path={}",
                    root.display()
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("server scoped local root-picker request"),
        )
        .await
        .expect("server scoped local root-picker response");
    assert_eq!(scoped.status(), StatusCode::OK);
    let scoped_body = axum::body::to_bytes(scoped.into_body(), 64 * 1024)
        .await
        .expect("server scoped local root-picker body");
    assert_eq!(scoped_body, legacy_body);

    remove_static_fixture(&root);
}

#[tokio::test]
async fn server_scoped_one_shot_mutation_routes_dispatch_equivalent_local_aliases() {
    let create_legacy_parent = temp_fixture_path("server-scoped-create-legacy-parent");
    let create_scoped_parent = temp_fixture_path("server-scoped-create-scoped-parent");
    let open_root = temp_fixture_path("server-scoped-open-local-root");
    fs::create_dir_all(&create_legacy_parent).expect("create legacy create parent");
    fs::create_dir_all(&create_scoped_parent).expect("create scoped create parent");
    fs::create_dir_all(&open_root).expect("create open local root");

    let (legacy_app, legacy_cookie) = paired_test_app().await;
    let (scoped_app, scoped_cookie) = paired_test_app().await;

    let create_name = "created-child";
    let (legacy_create_status, _, legacy_create) = request_json_for_test(
        legacy_app.clone(),
        Method::POST,
        "/api/dashboard/root-picker/directories".to_owned(),
        &legacy_cookie,
        serde_json::json!({
            "parentPath": create_legacy_parent.display().to_string(),
            "name": create_name,
        }),
    )
    .await;
    let (scoped_create_status, _, scoped_create) = request_json_for_test(
        scoped_app.clone(),
        Method::POST,
        "/api/dashboard/servers/server-local/root-picker/directories".to_owned(),
        &scoped_cookie,
        serde_json::json!({
            "parentPath": create_scoped_parent.display().to_string(),
            "name": create_name,
        }),
    )
    .await;
    assert_eq!(legacy_create_status, StatusCode::OK);
    assert_eq!(scoped_create_status, StatusCode::OK);
    assert_eq!(legacy_create["name"], scoped_create["name"]);
    assert_eq!(legacy_create["entryType"], scoped_create["entryType"]);
    assert_eq!(legacy_create["selectable"], scoped_create["selectable"]);
    assert!(create_legacy_parent.join(create_name).is_dir());
    assert!(create_scoped_parent.join(create_name).is_dir());

    let pin_path = open_root.display().to_string();
    let (legacy_pin_status, _, legacy_pin) = request_json_for_test(
        legacy_app.clone(),
        Method::POST,
        "/api/dashboard/root-picker/pins".to_owned(),
        &legacy_cookie,
        serde_json::json!({ "path": pin_path }),
    )
    .await;
    let (scoped_pin_status, _, scoped_pin) = request_json_for_test(
        scoped_app.clone(),
        Method::POST,
        "/api/dashboard/servers/server-local/root-picker/pins".to_owned(),
        &scoped_cookie,
        serde_json::json!({ "path": pin_path }),
    )
    .await;
    assert_eq!(legacy_pin_status, StatusCode::OK);
    assert_eq!(scoped_pin_status, StatusCode::OK);
    assert_eq!(legacy_pin, scoped_pin);

    let (legacy_unpin_status, _, legacy_unpin) = request_json_for_test(
        legacy_app.clone(),
        Method::DELETE,
        "/api/dashboard/root-picker/pins".to_owned(),
        &legacy_cookie,
        serde_json::json!({ "path": pin_path }),
    )
    .await;
    let (scoped_unpin_status, _, scoped_unpin) = request_json_for_test(
        scoped_app.clone(),
        Method::DELETE,
        "/api/dashboard/servers/server-local/root-picker/pins".to_owned(),
        &scoped_cookie,
        serde_json::json!({ "path": pin_path }),
    )
    .await;
    assert_eq!(legacy_unpin_status, StatusCode::OK);
    assert_eq!(scoped_unpin_status, StatusCode::OK);
    assert_eq!(legacy_unpin, scoped_unpin);

    let (legacy_open_status, legacy_open_headers, legacy_open) = request_json_for_test(
        legacy_app.clone(),
        Method::POST,
        "/api/dashboard/work-roots/open".to_owned(),
        &legacy_cookie,
        serde_json::json!({ "path": open_root.display().to_string() }),
    )
    .await;
    let (scoped_open_status, scoped_open_headers, scoped_open) = request_json_for_test(
        scoped_app.clone(),
        Method::POST,
        "/api/dashboard/servers/server-local/work-roots/open".to_owned(),
        &scoped_cookie,
        serde_json::json!({ "path": open_root.display().to_string() }),
    )
    .await;
    assert_eq!(legacy_open_status, StatusCode::OK);
    assert_eq!(scoped_open_status, StatusCode::OK);
    let legacy_opened_id = legacy_open_headers
        .get("x-ws-dashboard-opened-work-root-id")
        .expect("legacy opened id header");
    let scoped_opened_id = scoped_open_headers
        .get("x-ws-dashboard-opened-work-root-id")
        .expect("scoped opened id header");
    assert_eq!(legacy_opened_id, scoped_opened_id);
    assert_eq!(legacy_open, scoped_open);

    let work_root_id = legacy_opened_id
        .to_str()
        .expect("opened workRoot id header string");
    let (legacy_activation_status, _, legacy_activation) = request_json_for_test(
        legacy_app,
        Method::POST,
        format!("/api/dashboard/work-roots/{work_root_id}/activation"),
        &legacy_cookie,
        serde_json::json!({ "activation": "offline" }),
    )
    .await;
    let (scoped_activation_status, _, scoped_activation) = request_json_for_test(
        scoped_app,
        Method::POST,
        format!("/api/dashboard/servers/server-local/work-roots/{work_root_id}/activation"),
        &scoped_cookie,
        serde_json::json!({ "activation": "offline" }),
    )
    .await;
    assert_eq!(legacy_activation_status, StatusCode::OK);
    assert_eq!(scoped_activation_status, StatusCode::OK);
    assert_eq!(legacy_activation, scoped_activation);
    assert_eq!(
        work_root_by_id(&scoped_activation, work_root_id)["activation"],
        "offline"
    );

    remove_static_fixture(&create_legacy_parent);
    remove_static_fixture(&create_scoped_parent);
    remove_static_fixture(&open_root);
}

#[tokio::test]
async fn server_scoped_one_shot_routes_return_bounded_refusals() {
    let state_file_root = temp_fixture_path("server-scoped-one-shot-refusal-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-windows"),
            label: "Windows dogfood".to_owned(),
            kind: ServerKind::SshRemote,
            ssh_target: Some("owner@example.test".to_owned()),
            endpoint_hint: Some("http://127.0.0.1:4100".to_owned()),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("persist linked server seed");
    let state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let unknown = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-missing/root-picker")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("unknown server scoped route request"),
        )
        .await
        .expect("unknown server scoped route response");
    assert_eq!(unknown.status(), StatusCode::NOT_FOUND);
    let unknown_body = axum::body::to_bytes(unknown.into_body(), 4096)
        .await
        .expect("unknown response body");
    let unknown_value: serde_json::Value =
        serde_json::from_slice(&unknown_body).expect("unknown response JSON");
    assert_eq!(unknown_value["error"], "unknown server");

    let auth_required = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-windows/root-picker")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("auth-required server scoped route request"),
        )
        .await
        .expect("auth-required server scoped route response");
    assert_eq!(auth_required.status(), StatusCode::CONFLICT);
    let auth_body = axum::body::to_bytes(auth_required.into_body(), 4096)
        .await
        .expect("auth-required body");
    let auth_value: serde_json::Value =
        serde_json::from_slice(&auth_body).expect("auth-required response JSON");
    assert_eq!(auth_value["error"], "linked server auth required");
    assert!(
        !auth_body
            .windows(b"owner@example.test".len())
            .any(|window| window == b"owner@example.test"),
        "server scoped refusal must not expose SSH target"
    );

    let not_forwarded = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-windows/terminals/terminal-1/socket")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("non-allowlisted server scoped route request"),
        )
        .await
        .expect("non-allowlisted server scoped route response");
    assert_eq!(not_forwarded.status(), StatusCode::NOT_FOUND);

    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn linked_server_one_shot_forwarding_preserves_bearer_errors_and_rewrites_resources() {
    let remote_root = temp_fixture_path("server-scoped-one-shot-remote-root");
    fs::create_dir_all(&remote_root).expect("create remote one-shot root");
    let remote_state = app_state_with_opened_and_store(
        OpenedWorkRoots::default(),
        DashboardStateStore::disabled(),
    );
    let passphrase = remote_state
        .auth
        .link_passphrase()
        .expose_for_owner_record()
        .to_owned();
    let remote_app = build_router(remote_state);
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("server-scoped-one-shot-forwarding-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-windows"),
            label: "Windows dogfood".to_owned(),
            kind: ServerKind::Manual,
            ssh_target: None,
            endpoint_hint: Some(format!("http://{remote_addr}")),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("persist linked server seed");
    let local_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = local_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let local_app = build_router(local_state);
    let cookie = pair_and_cookie(local_app.clone(), &token).await;

    let linked = local_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/server-windows/link-auth")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "passphrase": passphrase }).to_string(),
                ))
                .expect("local link auth request"),
        )
        .await
        .expect("local link auth response");
    assert_eq!(linked.status(), StatusCode::OK);

    let open = local_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/server-windows/work-roots/open")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "path": remote_root.display().to_string() }).to_string(),
                ))
                .expect("server scoped remote open request"),
        )
        .await
        .expect("server scoped remote open response");
    assert_eq!(open.status(), StatusCode::OK);
    assert!(
        open.headers()
            .get("x-ws-dashboard-opened-work-root-id")
            .is_some(),
        "forwarded open response must preserve opened-id header"
    );
    let open_body = axum::body::to_bytes(open.into_body(), 64 * 1024)
        .await
        .expect("server scoped remote open body");
    let open_value: serde_json::Value =
        serde_json::from_slice(&open_body).expect("server scoped remote open JSON");
    assert_eq!(open_value["server"]["id"], "server-windows");
    assert_eq!(open_value["server"]["label"], "Windows dogfood");
    assert_eq!(
        open_value["workspaces"][0]["workRoots"][0]["resourcePath"]["serverId"],
        "server-windows"
    );

    let bad_create = local_app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/server-windows/root-picker/directories")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "parentPath": remote_root.display().to_string(),
                        "name": "nested/bad"
                    })
                    .to_string(),
                ))
                .expect("server scoped remote bad create request"),
        )
        .await
        .expect("server scoped remote bad create response");
    assert_eq!(bad_create.status(), StatusCode::BAD_REQUEST);
    let bad_body = axum::body::to_bytes(bad_create.into_body(), 4096)
        .await
        .expect("server scoped remote bad create body");
    let bad_value: serde_json::Value =
        serde_json::from_slice(&bad_body).expect("server scoped remote bad create JSON");
    assert_eq!(
        bad_value["error"],
        "directory name must be one path segment"
    );

    remote_server.abort();
    remove_static_fixture(&remote_root);
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn remote_link_auth_exchanges_passphrase_for_bearer_without_browser_pairing() {
    let state = app_state();
    let passphrase = state
        .auth
        .link_passphrase()
        .expose_for_owner_record()
        .to_owned();
    let app = build_router(state);

    let rejected = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/link-auth")
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "passphrase": "wrong" }).to_string(),
                ))
                .expect("wrong link auth request"),
        )
        .await
        .expect("wrong link auth response");
    assert_eq!(rejected.status(), StatusCode::UNAUTHORIZED);

    let accepted = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/link-auth")
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "passphrase": passphrase }).to_string(),
                ))
                .expect("link auth request"),
        )
        .await
        .expect("link auth response");
    assert_eq!(accepted.status(), StatusCode::OK);
    let body = axum::body::to_bytes(accepted.into_body(), 64 * 1024)
        .await
        .expect("link auth body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("link auth JSON");
    let bearer = value["bearerToken"]
        .as_str()
        .expect("bearer token string")
        .to_owned();

    let health = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::AUTHORIZATION, format!("Bearer {bearer}"))
                .body(Body::empty())
                .expect("bearer health request"),
        )
        .await
        .expect("bearer health response");
    assert_eq!(health.status(), StatusCode::OK);
}

#[tokio::test]
async fn local_link_auth_connects_and_forwards_remote_resources() {
    let remote_root = temp_fixture_path("remote-linked-resources");
    fs::create_dir_all(&remote_root).expect("create remote linked workRoot");
    let remote_state = app_state_with_opened_and_store(
        OpenedWorkRoots::from_paths(vec![remote_root.clone()]),
        DashboardStateStore::disabled(),
    );
    let passphrase = remote_state
        .auth
        .link_passphrase()
        .expose_for_owner_record()
        .to_owned();
    let remote_app = build_router(remote_state);
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("local-link-auth-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-windows"),
            label: "Windows dogfood".to_owned(),
            kind: ServerKind::SshRemote,
            ssh_target: Some("owner@example.test".to_owned()),
            endpoint_hint: Some(format!("http://{remote_addr}")),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("persist linked server seed");
    let local_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = local_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let local_app = build_router(local_state);
    let cookie = pair_and_cookie(local_app.clone(), &token).await;

    let linked = local_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/server-windows/link-auth")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "passphrase": passphrase }).to_string(),
                ))
                .expect("local link auth request"),
        )
        .await
        .expect("local link auth response");
    assert_eq!(linked.status(), StatusCode::OK);
    let linked_body = axum::body::to_bytes(linked.into_body(), 64 * 1024)
        .await
        .expect("local link auth body bytes");
    let linked_value: serde_json::Value =
        serde_json::from_slice(&linked_body).expect("local link auth JSON");
    assert_eq!(linked_value["id"], "server-windows");
    assert_eq!(linked_value["status"], "connected");
    assert!(
        !linked_body
            .windows(b"owner@example.test".len())
            .any(|window| window == b"owner@example.test"),
        "link auth response must not expose SSH target"
    );

    let servers = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("servers after link auth request"),
        )
        .await
        .expect("servers after link auth response");
    let servers_body = axum::body::to_bytes(servers.into_body(), 64 * 1024)
        .await
        .expect("servers after link auth body bytes");
    let servers_value: serde_json::Value =
        serde_json::from_slice(&servers_body).expect("servers after link auth JSON");
    assert_eq!(servers_value["servers"][1]["status"], "connected");

    let resources = local_app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-windows/resources")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("linked resources request"),
        )
        .await
        .expect("linked resources response");
    assert_eq!(resources.status(), StatusCode::OK);
    let resources_body = axum::body::to_bytes(resources.into_body(), 64 * 1024)
        .await
        .expect("linked resources body bytes");
    let resources_value: serde_json::Value =
        serde_json::from_slice(&resources_body).expect("linked resources JSON");
    assert_eq!(resources_value["server"]["id"], "server-windows");
    assert_eq!(resources_value["server"]["label"], "Windows dogfood");
    assert_eq!(
        resources_value["workspaces"][0]["workRoots"][0]["resourcePath"]["serverId"],
        "server-windows"
    );

    remote_server.abort();
    remove_static_fixture(&remote_root);
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn endpoint_link_registers_manual_server_and_forwards_resources() {
    let remote_root = temp_fixture_path("endpoint-linked-resources");
    fs::create_dir_all(&remote_root).expect("create endpoint linked workRoot");
    let remote_state = app_state_with_opened_and_store(
        OpenedWorkRoots::from_paths(vec![remote_root.clone()]),
        DashboardStateStore::disabled(),
    );
    let passphrase = remote_state
        .auth
        .link_passphrase()
        .expose_for_owner_record()
        .to_owned();
    let remote_app = build_router(remote_state);
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("endpoint-link-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    let local_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store.clone());
    let token = local_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let local_app = build_router(local_state);
    let cookie = pair_and_cookie(local_app.clone(), &token).await;

    let linked = local_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/link")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "serverId": "server-manual",
                        "label": "Manual endpoint",
                        "endpoint": format!("http://{remote_addr}/"),
                        "passphrase": passphrase
                    })
                    .to_string(),
                ))
                .expect("endpoint link request"),
        )
        .await
        .expect("endpoint link response");
    assert_eq!(linked.status(), StatusCode::OK);
    let linked_body = axum::body::to_bytes(linked.into_body(), 64 * 1024)
        .await
        .expect("endpoint link body bytes");
    let linked_value: serde_json::Value =
        serde_json::from_slice(&linked_body).expect("endpoint link JSON");
    assert_eq!(linked_value["id"], "server-manual");
    assert_eq!(linked_value["kind"], "manual");
    assert_eq!(linked_value["status"], "connected");

    let restored = store.load_linked_servers().await;
    assert_eq!(restored.len(), 1);
    assert_eq!(restored[0].id.as_str(), "server-manual");
    assert_eq!(restored[0].kind, ServerKind::Manual);
    assert_eq!(restored[0].ssh_target, None);
    assert_eq!(restored[0].remote_endpoint_hint, None);
    let expected_endpoint = format!("http://{remote_addr}");
    assert_eq!(
        restored[0].endpoint_hint.as_deref(),
        Some(expected_endpoint.as_str())
    );

    let resources = local_app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-manual/resources")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("endpoint linked resources request"),
        )
        .await
        .expect("endpoint linked resources response");
    assert_eq!(resources.status(), StatusCode::OK);
    let resources_body = axum::body::to_bytes(resources.into_body(), 64 * 1024)
        .await
        .expect("endpoint linked resources body bytes");
    let resources_value: serde_json::Value =
        serde_json::from_slice(&resources_body).expect("endpoint linked resources JSON");
    assert_eq!(resources_value["server"]["id"], "server-manual");
    assert_eq!(resources_value["server"]["label"], "Manual endpoint");

    let restarted_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let restart_token = restarted_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let restarted_app = build_router(restarted_state);
    let restart_cookie = pair_and_cookie(restarted_app.clone(), &restart_token).await;
    let restarted_servers = restarted_app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers")
                .header(header::COOKIE, restart_cookie)
                .body(Body::empty())
                .expect("servers after endpoint link restart request"),
        )
        .await
        .expect("servers after endpoint link restart response");
    let restarted_body = axum::body::to_bytes(restarted_servers.into_body(), 64 * 1024)
        .await
        .expect("servers after endpoint link restart body bytes");
    let restarted_value: serde_json::Value =
        serde_json::from_slice(&restarted_body).expect("servers after restart JSON");
    assert_eq!(restarted_value["servers"][1]["status"], "authRequired");

    remote_server.abort();
    remove_static_fixture(&remote_root);
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn endpoint_link_keeps_compatible_server_visible_after_wrong_passphrase() {
    let remote_state = app_state();
    let remote_app = build_router(remote_state);
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("endpoint-link-wrong-passphrase-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    let local_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store.clone());
    let token = local_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let local_app = build_router(local_state);
    let cookie = pair_and_cookie(local_app.clone(), &token).await;

    let linked = local_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/link")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "serverId": "server-manual",
                        "label": "Manual endpoint",
                        "endpoint": format!("http://{remote_addr}"),
                        "passphrase": "wrong"
                    })
                    .to_string(),
                ))
                .expect("endpoint wrong-passphrase link request"),
        )
        .await
        .expect("endpoint wrong-passphrase link response");
    assert_eq!(linked.status(), StatusCode::OK);
    let linked_body = axum::body::to_bytes(linked.into_body(), 64 * 1024)
        .await
        .expect("endpoint wrong-passphrase body bytes");
    let linked_value: serde_json::Value =
        serde_json::from_slice(&linked_body).expect("endpoint wrong-passphrase JSON");
    assert_eq!(linked_value["status"], "authRequired");

    let servers = local_app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("servers after wrong-passphrase endpoint link request"),
        )
        .await
        .expect("servers after wrong-passphrase endpoint link response");
    let servers_body = axum::body::to_bytes(servers.into_body(), 64 * 1024)
        .await
        .expect("servers after wrong-passphrase endpoint link body");
    let servers_value: serde_json::Value =
        serde_json::from_slice(&servers_body).expect("servers after wrong-passphrase JSON");
    assert_eq!(servers_value["servers"][1]["kind"], "manual");
    assert_eq!(servers_value["servers"][1]["status"], "authRequired");
    assert_eq!(
        servers_value["servers"][1]["actions"][0]["id"],
        "enterPassphrase"
    );

    remote_server.abort();
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
async fn git_worktree_add_routes_are_owner_authenticated() {
    let app = build_router(app_state());
    for (method, uri) in [
        (
            Method::GET,
            "/api/dashboard/workspaces/workspace-local-missing/git-worktree-add/options",
        ),
        (
            Method::POST,
            "/api/dashboard/workspaces/workspace-local-missing/git-worktree-add/preview",
        ),
        (
            Method::POST,
            "/api/dashboard/workspaces/workspace-local-missing/git-worktree-add",
        ),
    ] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method(method.clone())
                    .uri(uri)
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::from("{}"))
                    .expect("unauthenticated git worktree request"),
            )
            .await
            .expect("unauthenticated git worktree response");
        assert_eq!(response.status(), StatusCode::UNAUTHORIZED, "{uri}");
    }
}

#[tokio::test]
async fn git_worktree_add_previews_and_submits_new_branch_with_resource_refresh() {
    if skip_without_git("git_worktree_add_previews_and_submits_new_branch_with_resource_refresh") {
        return;
    }
    let base = temp_fixture_path("git-worktree-create");
    let primary = base.join("primary");
    fs::create_dir_all(&primary).expect("create primary");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "seed\n").expect("write seed");
    run_git(&primary, &["add", "README.md"]);
    run_git(&primary, &["commit", "-m", "seed"]);

    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    open_work_root_for_test(app.clone(), cookie.as_str(), &primary).await;
    let resources = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    let workspace_id = resources["workspaces"][0]["id"]
        .as_str()
        .expect("workspace id")
        .to_owned();

    let options = git_worktree_options_json(app.clone(), cookie.as_str(), &workspace_id).await;
    assert_eq!(options["git"]["available"], true);
    assert!(options["branches"]
        .as_array()
        .expect("branches")
        .iter()
        .any(|branch| branch["checkedOut"] == true));

    let preview = git_worktree_preview_json(
        app.clone(),
        cookie.as_str(),
        &workspace_id,
        serde_json::json!({
            "worktreeName": "Feature One",
            "branch": { "mode": "auto" },
            "path": { "mode": "auto" }
        }),
    )
    .await;
    assert_eq!(preview["status"], "willCreateBranch");
    assert_eq!(preview["branchName"], "Feature-One");
    assert!(preview["targetPathLabel"]
        .as_str()
        .expect("target label")
        .contains("ws-worktree"));

    let submit = git_worktree_submit_json(
        app.clone(),
        cookie.as_str(),
        &workspace_id,
        serde_json::json!({
            "worktreeName": "Feature One",
            "branch": { "mode": "auto" },
            "path": { "mode": "auto" },
            "activate": true
        }),
        StatusCode::OK,
    )
    .await;
    let created = submit["createdWorkRootId"]
        .as_str()
        .expect("created workRoot id");
    assert!(work_root_ids(&submit["resources"]).contains(&created.to_owned()));
    assert!(submit["resources"]["workspaces"]
        .as_array()
        .expect("workspaces")
        .iter()
        .any(|workspace| workspace["id"] == workspace_id));
    let body = serde_json::to_string(&submit).expect("submit JSON string");
    assert!(
        !body.contains(primary.to_string_lossy().as_ref()),
        "submit response must not leak primary path"
    );

    remove_static_fixture(&base);
}

#[tokio::test]
async fn git_worktree_add_existing_branch_is_yellow_and_submit_checks_out_branch() {
    if skip_without_git("git_worktree_add_existing_branch_is_yellow_and_submit_checks_out_branch") {
        return;
    }
    let base = temp_fixture_path("git-worktree-existing");
    let primary = base.join("primary");
    let target = base.join("manual-existing");
    fs::create_dir_all(&primary).expect("create primary");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "seed\n").expect("write seed");
    run_git(&primary, &["add", "README.md"]);
    run_git(&primary, &["commit", "-m", "seed"]);
    run_git(&primary, &["branch", "existing-topic"]);

    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    open_work_root_for_test(app.clone(), cookie.as_str(), &primary).await;
    let resources = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    let workspace_id = resources["workspaces"][0]["id"]
        .as_str()
        .expect("workspace id")
        .to_owned();

    let request = serde_json::json!({
        "worktreeName": "Existing Topic",
        "branch": { "mode": "manual", "name": "existing-topic" },
        "path": { "mode": "custom", "targetPath": target.display().to_string() }
    });
    let preview =
        git_worktree_preview_json(app.clone(), cookie.as_str(), &workspace_id, request.clone())
            .await;
    assert_eq!(preview["status"], "willCheckoutExisting");
    let submit = git_worktree_submit_json(
        app.clone(),
        cookie.as_str(),
        &workspace_id,
        json_with_activate(request, true),
        StatusCode::OK,
    )
    .await;
    assert!(target.join("README.md").is_file());
    assert_eq!(current_git_branch(&target), "existing-topic");
    assert!(submit["createdWorkRootId"]
        .as_str()
        .expect("created id")
        .starts_with("root-local-"));

    remove_static_fixture(&base);
}

#[tokio::test]
async fn git_worktree_add_blocks_checked_out_invalid_conflict_and_non_git_inputs() {
    if skip_without_git("git_worktree_add_blocks_checked_out_invalid_conflict_and_non_git_inputs") {
        return;
    }
    let base = temp_fixture_path("git-worktree-blocks");
    let primary = base.join("primary");
    let conflict = base.join("conflict");
    let plain = base.join("plain");
    fs::create_dir_all(&primary).expect("create primary");
    fs::create_dir_all(&conflict).expect("create conflict");
    fs::create_dir_all(&plain).expect("create plain");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "seed\n").expect("write seed");
    run_git(&primary, &["add", "README.md"]);
    run_git(&primary, &["commit", "-m", "seed"]);

    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    open_work_root_for_test(app.clone(), cookie.as_str(), &primary).await;
    open_work_root_for_test(app.clone(), cookie.as_str(), &plain).await;
    let resources = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    let git_workspace_id = resources["workspaces"]
        .as_array()
        .expect("workspaces")
        .iter()
        .find(|workspace| {
            workspace["workRoots"]
                .as_array()
                .expect("roots")
                .iter()
                .any(|root| root["kind"] == "gitPrimaryRoot")
        })
        .and_then(|workspace| workspace["id"].as_str())
        .expect("git workspace id")
        .to_owned();
    let plain_workspace_id = resources["workspaces"]
        .as_array()
        .expect("workspaces")
        .iter()
        .find(|workspace| {
            workspace["workRoots"]
                .as_array()
                .expect("roots")
                .iter()
                .any(|root| root["kind"] == "plainDirectory")
        })
        .and_then(|workspace| workspace["id"].as_str())
        .expect("plain workspace id")
        .to_owned();

    let checked_out = git_worktree_preview_json(
        app.clone(),
        cookie.as_str(),
        &git_workspace_id,
        serde_json::json!({
            "worktreeName": "Main Copy",
            "branch": { "mode": "manual", "name": current_git_branch(&primary) },
            "path": { "mode": "custom", "targetPath": base.join("main-copy").display().to_string() }
        }),
    )
    .await;
    assert_eq!(checked_out["status"], "blocked");
    assert!(blocker_codes(&checked_out).contains(&"branchAlreadyCheckedOut".to_owned()));

    let invalid = git_worktree_preview_json(
        app.clone(),
        cookie.as_str(),
        &git_workspace_id,
        serde_json::json!({
            "worktreeName": "..",
            "branch": { "mode": "manual", "name": "bad branch name" },
            "path": { "mode": "custom", "targetPath": base.join("invalid").display().to_string() }
        }),
    )
    .await;
    assert_eq!(invalid["status"], "blocked");
    assert!(blocker_codes(&invalid).contains(&"invalidWorktreeName".to_owned()));
    assert!(blocker_codes(&invalid).contains(&"invalidBranchName".to_owned()));

    let target_conflict = git_worktree_preview_json(
        app.clone(),
        cookie.as_str(),
        &git_workspace_id,
        serde_json::json!({
            "worktreeName": "Conflict Branch",
            "branch": { "mode": "auto" },
            "path": { "mode": "custom", "targetPath": conflict.display().to_string() }
        }),
    )
    .await;
    assert_eq!(target_conflict["status"], "blocked");
    assert!(blocker_codes(&target_conflict).contains(&"targetExists".to_owned()));
    let blocked_submit_target = base.join("blocked-submit-main-copy");
    let blocked_submit = git_worktree_submit_json(
        app.clone(),
        cookie.as_str(),
        &git_workspace_id,
        serde_json::json!({
            "worktreeName": "Blocked Main Copy",
            "branch": { "mode": "manual", "name": current_git_branch(&primary) },
            "path": { "mode": "custom", "targetPath": blocked_submit_target.display().to_string() },
            "activate": true
        }),
        StatusCode::BAD_REQUEST,
    )
    .await;
    assert_eq!(blocked_submit["status"], "blocked");
    assert!(blocker_codes(&blocked_submit).contains(&"branchAlreadyCheckedOut".to_owned()));
    assert!(
        !blocked_submit_target.exists(),
        "blocked submit must not create the target worktree"
    );

    let missing_parent = git_worktree_preview_json(app.clone(), cookie.as_str(), &git_workspace_id, serde_json::json!({
        "worktreeName": "Missing Parent",
        "branch": { "mode": "auto" },
        "path": { "mode": "custom", "targetPath": base.join("missing-parent").join("child").display().to_string() }
    })).await;
    assert_eq!(missing_parent["status"], "blocked");
    assert!(blocker_codes(&missing_parent).contains(&"targetParentMissing".to_owned()));

    let non_git_options =
        git_worktree_options_json(app.clone(), cookie.as_str(), &plain_workspace_id).await;
    assert_eq!(non_git_options["git"]["available"], false);
    let non_git = git_worktree_preview_json(
        app.clone(),
        cookie.as_str(),
        &plain_workspace_id,
        serde_json::json!({
            "worktreeName": "No Git",
            "branch": { "mode": "auto" },
            "path": { "mode": "auto" }
        }),
    )
    .await;
    assert_eq!(non_git["status"], "blocked");
    assert!(blocker_codes(&non_git).contains(&"notGitWorkspace".to_owned()));

    let unknown = git_worktree_submit_json(
        app.clone(),
        cookie.as_str(),
        "workspace-local-unknown",
        serde_json::json!({
            "worktreeName": "Unknown",
            "branch": { "mode": "auto" },
            "path": { "mode": "auto" },
            "activate": true
        }),
        StatusCode::BAD_REQUEST,
    )
    .await;
    assert_eq!(
        unknown["error"],
        "workspace is not available for Git worktree creation"
    );

    remove_static_fixture(&base);
}

#[tokio::test]
async fn git_toolbar_routes_are_owner_authenticated() {
    let app = build_router(app_state());
    for (method, uri) in [
        (
            Method::GET,
            "/api/dashboard/work-roots/root-local-missing/git/status",
        ),
        (
            Method::GET,
            "/api/dashboard/work-roots/root-local-missing/git/branches",
        ),
        (
            Method::POST,
            "/api/dashboard/work-roots/root-local-missing/git/switch-branch",
        ),
        (
            Method::POST,
            "/api/dashboard/work-roots/root-local-missing/git/branches",
        ),
        (
            Method::POST,
            "/api/dashboard/work-roots/root-local-missing/git/fetch",
        ),
        (
            Method::POST,
            "/api/dashboard/work-roots/root-local-missing/git/push",
        ),
        (
            Method::POST,
            "/api/dashboard/work-roots/root-local-missing/git/pull-ff-only",
        ),
    ] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method(method.clone())
                    .uri(uri)
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::from("{}"))
                    .expect("unauthenticated git toolbar request"),
            )
            .await
            .expect("unauthenticated git toolbar response");
        assert_eq!(response.status(), StatusCode::UNAUTHORIZED, "{uri}");
    }
}

#[tokio::test]
async fn git_toolbar_status_gates_and_reports_counts_without_paths() {
    if skip_without_git("git_toolbar_status_gates_and_reports_counts_without_paths") {
        return;
    }
    let base = temp_fixture_path("git-toolbar-status");
    let primary = base.join("primary");
    let plain = base.join("plain");
    fs::create_dir_all(&primary).expect("create primary");
    fs::create_dir_all(&plain).expect("create plain");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "one\ntwo\n").expect("write seed");
    run_git(&primary, &["add", "README.md"]);
    run_git(&primary, &["commit", "-m", "seed"]);
    let branch = current_git_branch(&primary);
    fs::write(primary.join("README.md"), "one\nthree\nfour\n").expect("modify tracked");
    fs::write(primary.join("new.txt"), "untracked\n").expect("write untracked");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let git_id = open_work_root_for_test(app.clone(), cookie.as_str(), &primary).await;
    let plain_id = open_work_root_for_test(app.clone(), cookie.as_str(), &plain).await;
    let status = git_toolbar_get_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/status"),
        StatusCode::OK,
    )
    .await;
    assert_eq!(status["available"], true);
    assert_eq!(status["branch"]["name"], branch);
    assert_eq!(status["changes"]["addedLines"], 2);
    assert_eq!(status["changes"]["removedLines"], 1);
    assert_eq!(status["changes"]["modifiedFiles"], 1);
    assert_eq!(status["changes"]["untrackedFiles"], 1);
    assert_eq!(status["operations"]["canFetch"], true);
    assert!(!serde_json::to_string(&status)
        .expect("status JSON")
        .contains(primary.to_string_lossy().as_ref()));
    let plain_status = git_toolbar_get_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{plain_id}/git/status"),
        StatusCode::BAD_REQUEST,
    )
    .await;
    assert_eq!(plain_status["error"], "workRoot is not a Git workRoot");
    let offline =
        set_work_root_activation_for_test(app.clone(), cookie.as_str(), &git_id, "offline").await;
    assert!(work_root_by_id(&offline, &git_id)["activation"] == "offline");
    let offline_status = git_toolbar_get_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/status"),
        StatusCode::CONFLICT,
    )
    .await;
    assert_eq!(offline_status["error"], "workRoot offline");
    let unknown = git_toolbar_get_json(
        app.clone(),
        cookie.as_str(),
        "/api/dashboard/work-roots/root-local-unknown/git/branches",
        StatusCode::NOT_FOUND,
    )
    .await;
    assert_eq!(unknown["error"], "unknown workRoot");
    remove_static_fixture(&base);
}

#[tokio::test]
async fn git_toolbar_branches_switch_and_create_revalidate_state() {
    if skip_without_git("git_toolbar_branches_switch_and_create_revalidate_state") {
        return;
    }
    let base = temp_fixture_path("git-toolbar-branches");
    let primary = base.join("primary");
    let linked = base.join("linked-topic");
    fs::create_dir_all(&primary).expect("create primary");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "seed\n").expect("write seed");
    run_git(&primary, &["add", "README.md"]);
    run_git(&primary, &["commit", "-m", "seed"]);
    let original_branch = current_git_branch(&primary);
    run_git(&primary, &["checkout", "-b", "conflict-target"]);
    fs::write(primary.join("README.md"), "conflict target\n").expect("write conflict target");
    run_git(&primary, &["add", "README.md"]);
    run_git(&primary, &["commit", "-m", "conflict target"]);
    run_git(&primary, &["switch", &original_branch]);
    run_git(&primary, &["checkout", "-b", "base-source"]);
    fs::write(primary.join("base.txt"), "base source\n").expect("write base source");
    run_git(&primary, &["add", "base.txt"]);
    run_git(&primary, &["commit", "-m", "base source"]);
    let base_source_oid = git_stdout(&primary, &["rev-parse", "HEAD"]);
    run_git(&primary, &["switch", &original_branch]);
    run_git(&primary, &["branch", "topic"]);
    run_git(
        &primary,
        &[
            "worktree",
            "add",
            linked.to_str().expect("linked path"),
            "topic",
        ],
    );
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let git_id = open_work_root_for_test(app.clone(), cookie.as_str(), &primary).await;
    let branches = git_toolbar_get_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/branches"),
        StatusCode::OK,
    )
    .await;
    let topic = branches["branches"]
        .as_array()
        .expect("branches")
        .iter()
        .find(|branch| branch["name"] == "topic")
        .expect("topic branch");
    assert_eq!(topic["checkedOut"], true);
    assert_eq!(topic["disabledReason"], "Already checked out");
    let blocked = git_toolbar_post_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/switch-branch"),
        serde_json::json!({"branchName":"topic"}),
        StatusCode::BAD_REQUEST,
    )
    .await;
    assert_eq!(blocked["error"], "branch is already checked out");
    let created = git_toolbar_post_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/branches"),
        serde_json::json!({"branchName":"browser-created","baseBranch":"base-source","switchTo":true}),
        StatusCode::OK,
    )
    .await;
    assert_eq!(created["branch"]["name"], "browser-created");
    assert_eq!(current_git_branch(&primary), "browser-created");
    assert_eq!(
        git_stdout(&primary, &["rev-parse", "HEAD"]),
        base_source_oid
    );
    let duplicate_create = git_toolbar_post_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/branches"),
        serde_json::json!({"branchName":"browser-created","switchTo":true}),
        StatusCode::BAD_REQUEST,
    )
    .await;
    assert_eq!(duplicate_create["error"], "branch cannot be created");
    assert_eq!(current_git_branch(&primary), "browser-created");
    fs::write(primary.join("README.md"), "dirty\n").expect("make dirty");
    let dirty = git_toolbar_post_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/switch-branch"),
        serde_json::json!({"branchName": "conflict-target"}),
        StatusCode::BAD_REQUEST,
    )
    .await;
    assert_eq!(dirty["error"], "branch switch failed");
    assert!(!serde_json::to_string(&dirty)
        .expect("dirty JSON")
        .contains(primary.to_string_lossy().as_ref()));
    remove_static_fixture(&base);
}

#[tokio::test]
async fn git_toolbar_fetch_push_and_pull_ff_only_use_safe_git_defaults() {
    if skip_without_git("git_toolbar_fetch_push_and_pull_ff_only_use_safe_git_defaults") {
        return;
    }
    let base = temp_fixture_path("git-toolbar-sync");
    let remote = base.join("remote.git");
    let primary = base.join("primary");
    let other = base.join("other");
    fs::create_dir_all(&base).expect("create sync base");
    run_git(&base, &["init", "--bare", remote.to_str().expect("remote")]);
    fs::create_dir_all(&primary).expect("create primary");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "seed\n").expect("write seed");
    run_git(&primary, &["add", "README.md"]);
    run_git(&primary, &["commit", "-m", "seed"]);
    let branch = current_git_branch(&primary);
    run_git(
        &primary,
        &[
            "remote",
            "add",
            "origin",
            remote.to_str().expect("remote path"),
        ],
    );
    run_git(&primary, &["push", "-u", "origin", &branch]);
    run_git(
        &base,
        &[
            "clone",
            remote.to_str().expect("remote path"),
            other.to_str().expect("other path"),
        ],
    );
    run_git(
        &other,
        &["config", "user.email", "ws-dashboard@example.local"],
    );
    run_git(&other, &["config", "user.name", "ws dashboard"]);
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let git_id = open_work_root_for_test(app.clone(), cookie.as_str(), &primary).await;
    fs::write(primary.join("local.txt"), "local\n").expect("write local");
    run_git(&primary, &["add", "local.txt"]);
    run_git(&primary, &["commit", "-m", "local"]);
    let ahead = git_toolbar_get_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/status"),
        StatusCode::OK,
    )
    .await;
    assert_eq!(ahead["sync"]["ahead"], 1);
    assert_eq!(ahead["operations"]["canPush"], true);
    let pushed = git_toolbar_post_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/push"),
        serde_json::json!({}),
        StatusCode::OK,
    )
    .await;
    assert_eq!(pushed["sync"]["ahead"], 0);
    run_git(&other, &["pull", "--ff-only"]);
    fs::write(other.join("remote.txt"), "remote\n").expect("write remote");
    run_git(&other, &["add", "remote.txt"]);
    run_git(&other, &["commit", "-m", "remote"]);
    run_git(&other, &["push"]);
    let fetched = git_toolbar_post_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/fetch"),
        serde_json::json!({}),
        StatusCode::OK,
    )
    .await;
    assert_eq!(fetched["sync"]["behind"], 1);
    assert_eq!(fetched["operations"]["canPullFfOnly"], true);
    let pulled = git_toolbar_post_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/pull-ff-only"),
        serde_json::json!({}),
        StatusCode::OK,
    )
    .await;
    assert_eq!(pulled["sync"]["behind"], 0);
    run_git(&other, &["pull", "--ff-only"]);
    fs::write(primary.join("local2.txt"), "local2\n").expect("write local2");
    run_git(&primary, &["add", "local2.txt"]);
    run_git(&primary, &["commit", "-m", "local2"]);
    fs::write(other.join("remote2.txt"), "remote2\n").expect("write remote2");
    run_git(&other, &["add", "remote2.txt"]);
    run_git(&other, &["commit", "-m", "remote2"]);
    run_git(&other, &["push"]);
    let push_rejected = git_toolbar_post_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/push"),
        serde_json::json!({}),
        StatusCode::BAD_REQUEST,
    )
    .await;
    assert_eq!(push_rejected["error"], "push failed");
    git_toolbar_post_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/fetch"),
        serde_json::json!({}),
        StatusCode::OK,
    )
    .await;
    let pull_rejected = git_toolbar_post_json(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{git_id}/git/pull-ff-only"),
        serde_json::json!({}),
        StatusCode::BAD_REQUEST,
    )
    .await;
    assert_eq!(pull_rejected["error"], "pull --ff-only failed");
    assert_eq!(current_git_branch(&primary), branch);
    assert!(
        !primary.join(".git").join("MERGE_HEAD").exists(),
        "ff-only pull failure must not leave a merge in progress"
    );
    assert!(
        !primary.join(".git").join("rebase-merge").exists()
            && !primary.join(".git").join("rebase-apply").exists(),
        "ff-only pull failure must not leave a rebase in progress"
    );
    assert!(
        git_stdout(&primary, &["diff", "--name-only", "--diff-filter=U"]).is_empty(),
        "ff-only pull failure must not leave conflicted index entries"
    );
    remove_static_fixture(&base);
}

#[tokio::test]
async fn workspace_remove_route_forgets_workspace_without_deleting_files_or_paths() {
    let first = temp_fixture_path("workspace-remove-first");
    let second = temp_fixture_path("workspace-remove-second");
    let state_file_root = temp_fixture_path("workspace-remove-state");
    fs::create_dir_all(&first).expect("create first workRoot");
    fs::create_dir_all(&second).expect("create second workRoot");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    let state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let first_id = open_work_root_for_test(app.clone(), cookie.as_str(), &first).await;
    let second_id = open_work_root_for_test(app.clone(), cookie.as_str(), &second).await;
    let resources = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    let workspace_id = resources["workspaces"]
        .as_array()
        .expect("workspaces array")
        .iter()
        .find(|workspace| {
            workspace["workRoots"]
                .as_array()
                .expect("workRoots array")
                .iter()
                .any(|root| root["id"] == first_id)
        })
        .and_then(|workspace| workspace["id"].as_str())
        .expect("workspace id containing first root")
        .to_owned();

    let response = app
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!("/api/dashboard/workspaces/{workspace_id}"))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("workspace remove request"),
        )
        .await
        .expect("workspace remove response");

    assert_eq!(response.status(), StatusCode::OK);
    assert!(
        first.is_dir(),
        "workspace removal must not delete files on disk"
    );
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("workspace remove body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("remove JSON");
    let body_text = String::from_utf8_lossy(&body);
    assert!(!body_text.contains(first.to_string_lossy().as_ref()));
    let ids = work_root_ids(&value);
    assert!(!ids.contains(&first_id));
    assert!(ids.contains(&second_id));
    assert_eq!(store.load_opened_work_roots().await, vec![second.clone()]);

    remove_static_fixture(&first);
    remove_static_fixture(&second);
    remove_static_fixture(&state_file_root);
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
        document_translation: DocumentTranslationService::default(),
        terminals: TerminalRegistry::default(),
        document_events: DocumentEventHub::default(),
        document_write_locks: ws_dashboard_daemon::work_root_files::DocumentWriteLocks::default(),
        work_root_activity: WorkRootActivityProjector::new(WorkRootActivityProjectionConfig {
            codex_home,
            cache_home: Some(cache_home),
        }),
        linked_server_sessions: LinkedServerSessions::default(),
        linked_server_tunnels: LinkedServerTunnels::record_only_for_tests(),
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

    delete_agent_def(&agents_dir, "delta");

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
    upsert_agent_def(
        agents_dir,
        agent_json
            .get("state_path")
            .and_then(|value| value.as_str())
            .unwrap_or(agent_key),
        agent_json
            .get("name")
            .and_then(|value| value.as_str())
            .unwrap_or(""),
        agent_key,
        agent_json
            .get("backend")
            .and_then(|value| value.as_str())
            .unwrap_or(""),
        agent_json
            .get("harness")
            .and_then(|value| value.as_str())
            .unwrap_or(""),
        agent_json
            .get("tier")
            .and_then(|value| value.as_str())
            .unwrap_or(""),
        agent_json
            .get("model")
            .and_then(|value| value.as_str())
            .unwrap_or(""),
        agent_json
            .get("effort")
            .and_then(|value| value.as_str())
            .unwrap_or(""),
        agent_json
            .get("session_id")
            .and_then(|value| value.as_str())
            .unwrap_or(""),
        agent_json
            .get("status")
            .and_then(|value| value.as_str())
            .unwrap_or(""),
        agent_json
            .get("last_call_at")
            .and_then(|value| value.as_str())
            .unwrap_or(""),
        agent_json
            .get("last_output_path")
            .and_then(|value| value.as_str())
            .unwrap_or(""),
    );
}

fn write_agent_metadata_raw(agents_dir: &Path, agent_key: &str, raw: &str) {
    let agent_dir = agents_dir.join(agent_key);
    fs::create_dir_all(&agent_dir).expect("create agent fixture dir");
    upsert_agent_def(
        agents_dir, agent_key, "", agent_key, "", "", "", "", "", "", raw, "", "",
    );
}

#[allow(clippy::too_many_arguments)]
fn upsert_agent_def(
    agents_dir: &Path,
    agent_key: &str,
    public_name: &str,
    state_path: &str,
    backend: &str,
    harness: &str,
    tier: &str,
    model: &str,
    effort: &str,
    session_id: &str,
    status: &str,
    last_call_at: &str,
    last_output_path: &str,
) {
    let state_dir = agents_dir.parent().expect("agents dir has state parent");
    fs::create_dir_all(agents_dir).expect("create wsstate agents dir");
    let connection =
        Connection::open(state_dir.join("state.sqlite")).expect("open state.sqlite fixture");
    connection
        .execute_batch(
            "CREATE TABLE IF NOT EXISTS agent_defs (
                agent_key TEXT PRIMARY KEY,
                actor_id TEXT,
                public_name TEXT,
                state_path TEXT,
                schema_version INTEGER,
                backend TEXT,
                harness TEXT,
                tier TEXT,
                model TEXT,
                effort TEXT,
                session_id TEXT,
                status TEXT,
                created_at TEXT,
                updated_at TEXT,
                last_seen_at TEXT,
                last_call_at TEXT,
                last_output_path TEXT
            );",
        )
        .expect("create agent_defs fixture schema");
    connection
        .execute(
            "INSERT OR REPLACE INTO agent_defs (
                agent_key, actor_id, public_name, state_path, schema_version,
                backend, harness, tier, model, effort, session_id, status,
                created_at, updated_at, last_seen_at, last_call_at, last_output_path
            ) VALUES (?1, '', ?2, ?3, 1, ?4, ?5, ?6, ?7, ?8, ?9, ?10, '', '', '', ?11, ?12)",
            params![
                agent_key,
                public_name,
                state_path,
                backend,
                harness,
                tier,
                model,
                effort,
                session_id,
                status,
                last_call_at,
                last_output_path
            ],
        )
        .expect("insert agent_defs fixture row");
}

#[allow(clippy::too_many_arguments)]
fn upsert_agent_instance(
    agents_dir: &Path,
    instance_id: &str,
    agent_key: &str,
    public_name: &str,
    state_path: &str,
    backend: &str,
    harness: &str,
    tier: &str,
    model: &str,
    effort: &str,
    session_id: &str,
    status: &str,
    last_call_at: &str,
    last_output_path: &str,
    cleanup_state: &str,
    cleanup_error: &str,
    pinned: bool,
) {
    let state_dir = agents_dir.parent().expect("agents dir has state parent");
    fs::create_dir_all(agents_dir).expect("create wsstate agents dir");
    let connection =
        Connection::open(state_dir.join("state.sqlite")).expect("open state.sqlite fixture");
    connection
        .execute_batch(
            "CREATE TABLE IF NOT EXISTS agent_instances (
                instance_id TEXT PRIMARY KEY,
                agent_key TEXT,
                public_name TEXT,
                state_path TEXT,
                backend TEXT,
                harness TEXT,
                tier TEXT,
                model TEXT,
                effort TEXT,
                session_id TEXT,
                status TEXT,
                created_at TEXT,
                updated_at TEXT,
                last_seen_at TEXT,
                last_call_at TEXT,
                last_output_path TEXT,
                cleanup_state TEXT,
                cleanup_attempted_at TEXT,
                cleanup_error TEXT,
                retention_eligible_at TEXT,
                retention_checked_at TEXT,
                retention_next_check_at TEXT,
                pinned INTEGER
            );",
        )
        .expect("create agent_instances fixture schema");
    connection
        .execute(
            "INSERT OR REPLACE INTO agent_instances (
                instance_id, agent_key, public_name, state_path, backend, harness, tier, model,
                effort, session_id, status, created_at, updated_at, last_seen_at, last_call_at,
                last_output_path, cleanup_state, cleanup_attempted_at, cleanup_error,
                retention_eligible_at, retention_checked_at, retention_next_check_at, pinned
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, '', ?12, '', ?12, ?13, ?14, '', ?15, '', '', '', ?16)",
            params![
                instance_id,
                agent_key,
                public_name,
                state_path,
                backend,
                harness,
                tier,
                model,
                effort,
                session_id,
                status,
                last_call_at,
                last_output_path,
                cleanup_state,
                cleanup_error,
                if pinned { 1 } else { 0 }
            ],
        )
        .expect("insert agent_instances fixture row");
}

fn delete_agent_def(agents_dir: &Path, agent_key: &str) {
    let state_dir = agents_dir.parent().expect("agents dir has state parent");
    let connection =
        Connection::open(state_dir.join("state.sqlite")).expect("open state.sqlite fixture");
    connection
        .execute(
            "DELETE FROM agent_defs WHERE agent_key = ?1",
            params![agent_key],
        )
        .expect("delete agent_defs fixture row");
    let _ = fs::remove_dir_all(agents_dir.join(agent_key));
}

fn update_agent_def_registry_fields(
    agents_dir: &Path,
    agent_key: &str,
    status: &str,
    updated_at: &str,
    last_seen_at: &str,
    last_output_path: &str,
) {
    let state_dir = agents_dir.parent().expect("agents dir has state parent");
    let connection =
        Connection::open(state_dir.join("state.sqlite")).expect("open state.sqlite fixture");
    connection
        .execute(
            "UPDATE agent_defs
             SET status = ?2,
                 created_at = COALESCE(NULLIF(created_at, ''), ?3),
                 updated_at = ?3,
                 last_seen_at = ?4,
                 last_output_path = ?5
             WHERE agent_key = ?1",
            params![
                agent_key,
                status,
                updated_at,
                last_seen_at,
                last_output_path
            ],
        )
        .expect("update agent_defs registry-only fields");
}

fn update_agent_instance_registry_fields(
    agents_dir: &Path,
    instance_id: &str,
    cleanup_state: &str,
    updated_at: &str,
    cleanup_attempted_at: &str,
    retention_eligible_at: &str,
    retention_checked_at: &str,
    retention_next_check_at: &str,
) {
    let state_dir = agents_dir.parent().expect("agents dir has state parent");
    let connection =
        Connection::open(state_dir.join("state.sqlite")).expect("open state.sqlite fixture");
    connection
        .execute(
            "UPDATE agent_instances
             SET updated_at = ?2,
                 last_seen_at = ?2,
                 cleanup_state = ?3,
                 cleanup_attempted_at = ?4,
                 retention_eligible_at = ?5,
                 retention_checked_at = ?6,
                 retention_next_check_at = ?7
             WHERE instance_id = ?1",
            params![
                instance_id,
                updated_at,
                cleanup_state,
                cleanup_attempted_at,
                retention_eligible_at,
                retention_checked_at,
                retention_next_check_at
            ],
        )
        .expect("update agent_instances registry-only fields");
}

fn write_current_call(agents_dir: &Path, agent_key: &str, raw: &str) {
    write_current_call_at_state_path(agents_dir, agent_key, raw);
}

fn write_current_call_at_state_path(agents_dir: &Path, state_path: &str, raw: &str) {
    assert!(
        !state_path.contains('/') && !state_path.contains('\\'),
        "test fixture state_path must stay simple"
    );
    let current_dir = agents_dir.join(state_path).join("current");
    fs::create_dir_all(&current_dir).expect("create current fixture dir");
    fs::write(current_dir.join("state.json"), raw).expect("write state.json fixture");
}

fn write_agent_output(agents_dir: &Path, agent_key: &str, raw: &str) {
    write_agent_output_at_state_path(agents_dir, agent_key, raw);
}

fn write_agent_output_at_state_path(agents_dir: &Path, state_path: &str, raw: &str) {
    assert!(
        !state_path.contains('/') && !state_path.contains('\\'),
        "test fixture state_path must stay simple"
    );
    let agent_dir = agents_dir.join(state_path);
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
async fn work_root_activity_route_resolves_payloads_by_registry_state_path() {
    if skip_without_git("work_root_activity_route_resolves_payloads_by_registry_state_path") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-state-path");
    let cache_home = temp_fixture_path("work-root-activity-state-path-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    upsert_agent_def(
        &agents_dir,
        "reviewer-role",
        "reviewer role",
        "payload-reviewer",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "state-path-private-session",
        "running",
        "2026-05-17T10:00:00Z",
        "/private/cache/output.md",
    );
    write_current_call_at_state_path(
        &agents_dir,
        "payload-reviewer",
        r#"{"status":"running","execution_id":"state-path-run","started_at":"2026-05-17T10:00:00Z","updated_at":"2026-05-17T10:01:00Z"}"#,
    );
    write_agent_output_at_state_path(
        &agents_dir,
        "payload-reviewer",
        "state path transcript line\n",
    );

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) =
        fetch_work_root_activity(app.clone(), cookie.as_str(), &work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("state_path activity JSON");

    assert_eq!(value["summary"]["total"], 1);
    assert_eq!(value["agents"][0]["agentId"], "reviewer-role");
    assert_eq!(
        value["agents"][0]["currentCall"]["executionId"],
        "state-path-run"
    );
    assert_eq!(value["items"][0]["id"], "agent:reviewer-role");
    assert_eq!(value["items"][0]["transcript"]["available"], true);

    let (transcript_status, transcript_body) = fetch_work_root_activity_transcript(
        app,
        cookie.as_str(),
        &work_root_id,
        "agent:reviewer-role",
        "",
    )
    .await;
    assert_eq!(transcript_status, StatusCode::OK);
    let transcript: serde_json::Value =
        serde_json::from_str(&transcript_body).expect("state_path transcript JSON");
    assert_eq!(transcript["status"], "available");
    assert_eq!(
        transcript["blocks"][0]["text"],
        "state path transcript line"
    );

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "state-path-private-session".to_owned(),
        "payload-reviewer".to_owned(),
        "agent.json".to_owned(),
        "state.sqlite".to_owned(),
        "/private/cache/output.md".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden) && !transcript_body.contains(&forbidden),
            "state_path activity responses must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_projects_retained_agent_instances_as_items_only() {
    if skip_without_git("work_root_activity_route_projects_retained_agent_instances_as_items_only")
    {
        return;
    }
    let root = temp_fixture_path("work-root-activity-retained-instances");
    let cache_home = temp_fixture_path("work-root-activity-retained-instances-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    upsert_agent_def(
        &agents_dir,
        "reviewer",
        "reviewer current",
        "payload-current-reviewer",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "current-session-secret",
        "running",
        "2026-05-17T10:00:00Z",
        "/private/cache/current/output.md",
    );
    write_agent_output_at_state_path(
        &agents_dir,
        "payload-current-reviewer",
        "current role transcript\n",
    );

    upsert_agent_instance(
        &agents_dir,
        "reviewer:private/state/current-secret",
        "reviewer",
        "reviewer current duplicate",
        "payload-current-reviewer",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "current-instance-session-secret",
        "running",
        "2026-05-17T10:00:00Z",
        "/private/cache/current/output.md",
        "current",
        "",
        false,
    );

    upsert_agent_instance(
        &agents_dir,
        "reviewer:private/state/active-secret",
        "reviewer",
        "reviewer active protected",
        "payload-historical-active",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "active-session-secret",
        "running",
        "2026-05-17T09:30:00Z",
        "/private/cache/active/output.md",
        "active",
        "",
        false,
    );
    write_agent_output_at_state_path(
        &agents_dir,
        "payload-historical-active",
        "active protected transcript\n",
    );

    upsert_agent_instance(
        &agents_dir,
        "reviewer:private/state/completed-secret",
        "reviewer",
        "reviewer old completed",
        "payload-historical-completed",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "historical-session-secret",
        "completed",
        "2026-05-17T09:00:00Z",
        "/private/cache/historical/output.md",
        "retired",
        "",
        false,
    );
    write_agent_output_at_state_path(
        &agents_dir,
        "payload-historical-completed",
        "historical completed transcript\n",
    );

    upsert_agent_instance(
        &agents_dir,
        "reviewer:private/state/failed-secret",
        "reviewer",
        "reviewer old failed",
        "payload-historical-failed",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "failed-session-secret",
        "failed",
        "2026-05-17T08:00:00Z",
        "/private/cache/failed/output.md",
        "cleanup_failed",
        "private cleanup path /tmp/secret",
        false,
    );
    write_agent_output_at_state_path(
        &agents_dir,
        "payload-historical-failed",
        "failed transcript\n",
    );

    upsert_agent_instance(
        &agents_dir,
        "reviewer:private/state/cancelled-secret",
        "reviewer",
        "reviewer old cancelled",
        "payload-historical-cancelled",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "cancelled-session-secret",
        "cancelled",
        "2026-05-17T07:00:00Z",
        "/private/cache/cancelled/output.md",
        "retired",
        "",
        false,
    );

    upsert_agent_instance(
        &agents_dir,
        "reviewer:private/state/retired-secret",
        "reviewer",
        "reviewer old retired",
        "payload-historical-retired",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "retired-session-secret",
        "retired",
        "2026-05-17T06:00:00Z",
        "",
        "retired",
        "",
        false,
    );
    write_current_call_at_state_path(
        &agents_dir,
        "payload-historical-retired",
        r#"{"status":"completed","execution_id":"retired-run","started_at":"2026-05-17T05:59:00Z","updated_at":"2026-05-17T06:00:00Z","finished_at":"2026-05-17T06:00:00Z"}"#,
    );

    upsert_agent_instance(
        &agents_dir,
        "reviewer:private/state/deleted-secret",
        "reviewer",
        "reviewer deleted",
        "payload-historical-deleted",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "deleted-session-secret",
        "completed",
        "2026-05-17T05:00:00Z",
        "/private/cache/deleted/output.md",
        "cleanup_deleted",
        "",
        false,
    );
    write_agent_output_at_state_path(
        &agents_dir,
        "payload-historical-deleted",
        "deleted transcript\n",
    );

    upsert_agent_instance(
        &agents_dir,
        "reviewer:private/state/tombstone-secret",
        "reviewer",
        "reviewer tombstone",
        "payload-historical-tombstone",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "tombstone-session-secret",
        "completed",
        "2026-05-17T04:30:00Z",
        "/private/cache/tombstone/output.md",
        "retention_tombstone",
        "",
        false,
    );
    write_agent_output_at_state_path(
        &agents_dir,
        "payload-historical-tombstone",
        "tombstone transcript\n",
    );

    upsert_agent_instance(
        &agents_dir,
        "reviewer:private/state/internal-secret",
        "reviewer",
        "reviewer internal",
        "payload-historical-internal",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "internal-session-secret",
        "completed",
        "2026-05-17T04:15:00Z",
        "/private/cache/internal/output.md",
        "internal_retention_marker",
        "",
        false,
    );
    write_agent_output_at_state_path(
        &agents_dir,
        "payload-historical-internal",
        "internal transcript\n",
    );

    upsert_agent_instance(
        &agents_dir,
        "reviewer:private/state/status-only-secret",
        "reviewer",
        "reviewer status only",
        "payload-historical-status-only",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "status-only-session-secret",
        "completed",
        "2026-05-17T04:00:00Z",
        "",
        "retired",
        "",
        false,
    );
    fs::create_dir_all(agents_dir.join("payload-historical-status-only"))
        .expect("create status-only historical payload dir");

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) =
        fetch_work_root_activity(app.clone(), cookie.as_str(), &work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("retained instance activity JSON");

    assert_eq!(value["summary"]["total"], 1);
    assert_eq!(value["agents"].as_array().expect("agents array").len(), 1);
    assert_eq!(value["agents"][0]["agentId"], "reviewer");

    let items = value["items"].as_array().expect("items array");
    assert_eq!(items.len(), 5);
    assert!(items.iter().any(|item| item["id"] == "agent:reviewer"));
    assert!(items.iter().any(|item| item["status"] == "failed"));
    assert!(items.iter().any(|item| item["status"] == "cancelled"));
    assert!(items.iter().any(|item| item["status"] == "completed"));
    assert!(items.iter().any(|item| item["status"] == "retired"));
    assert!(!body_text.contains("reviewer current duplicate"));
    assert!(!body_text.contains("reviewer active protected"));
    assert!(!body_text.contains("reviewer deleted"));
    assert!(!body_text.contains("reviewer tombstone"));
    assert!(!body_text.contains("reviewer internal"));
    assert!(!body_text.contains("reviewer status only"));
    assert_eq!(
        items
            .iter()
            .filter(|item| item["id"]
                .as_str()
                .is_some_and(|id| id.starts_with("agent-instance:")))
            .count(),
        4
    );

    let historical_item = items
        .iter()
        .find(|item| {
            item["id"]
                .as_str()
                .is_some_and(|id| id.starts_with("agent-instance:"))
                && item["status"] == "completed"
        })
        .expect("completed historical item");
    let historical_id = historical_item["id"].as_str().expect("historical id");
    assert_ne!(historical_id, "agent:reviewer");
    assert_eq!(historical_item["metadata"]["agentId"], "reviewer");
    assert_eq!(historical_item["metadata"]["historical"], true);
    assert_eq!(historical_item["transcript"]["available"], true);

    let (current_status, current_body) = fetch_work_root_activity_transcript(
        app.clone(),
        cookie.as_str(),
        &work_root_id,
        "agent:reviewer",
        "",
    )
    .await;
    assert_eq!(current_status, StatusCode::OK);
    let current_transcript: serde_json::Value =
        serde_json::from_str(&current_body).expect("current role transcript JSON");
    assert_eq!(
        current_transcript["blocks"][0]["text"],
        "current role transcript"
    );

    let (historical_status, historical_body) =
        fetch_work_root_activity_transcript(app, cookie.as_str(), &work_root_id, historical_id, "")
            .await;
    assert_eq!(historical_status, StatusCode::OK);
    let historical_transcript: serde_json::Value =
        serde_json::from_str(&historical_body).expect("historical transcript JSON");
    assert_eq!(historical_transcript["activityId"], historical_id);
    assert_eq!(historical_transcript["status"], "available");
    assert_eq!(
        historical_transcript["blocks"][0]["text"],
        "historical completed transcript"
    );

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "current-session-secret".to_owned(),
        "current-instance-session-secret".to_owned(),
        "historical-session-secret".to_owned(),
        "failed-session-secret".to_owned(),
        "cancelled-session-secret".to_owned(),
        "retired-session-secret".to_owned(),
        "deleted-session-secret".to_owned(),
        "active-session-secret".to_owned(),
        "tombstone-session-secret".to_owned(),
        "internal-session-secret".to_owned(),
        "status-only-session-secret".to_owned(),
        "reviewer:private/state/current-secret".to_owned(),
        "reviewer:private/state/active-secret".to_owned(),
        "reviewer:private/state/completed-secret".to_owned(),
        "reviewer:private/state/failed-secret".to_owned(),
        "reviewer:private/state/cancelled-secret".to_owned(),
        "reviewer:private/state/retired-secret".to_owned(),
        "reviewer:private/state/deleted-secret".to_owned(),
        "reviewer:private/state/tombstone-secret".to_owned(),
        "reviewer:private/state/internal-secret".to_owned(),
        "reviewer:private/state/status-only-secret".to_owned(),
        "payload-current-reviewer".to_owned(),
        "payload-historical-active".to_owned(),
        "payload-historical-completed".to_owned(),
        "payload-historical-failed".to_owned(),
        "payload-historical-cancelled".to_owned(),
        "payload-historical-retired".to_owned(),
        "payload-historical-deleted".to_owned(),
        "payload-historical-tombstone".to_owned(),
        "payload-historical-internal".to_owned(),
        "payload-historical-status-only".to_owned(),
        "state.sqlite".to_owned(),
        "/private/cache/current/output.md".to_owned(),
        "/private/cache/active/output.md".to_owned(),
        "/private/cache/historical/output.md".to_owned(),
        "/private/cache/failed/output.md".to_owned(),
        "/private/cache/cancelled/output.md".to_owned(),
        "/private/cache/deleted/output.md".to_owned(),
        "/private/cache/tombstone/output.md".to_owned(),
        "/private/cache/internal/output.md".to_owned(),
        "private cleanup path".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden)
                && !current_body.contains(&forbidden)
                && !historical_body.contains(&forbidden),
            "retained instance responses must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_events_emit_retained_item_for_registry_only_lifecycle() {
    if skip_without_git("work_root_activity_events_emit_retained_item_for_registry_only_lifecycle")
    {
        return;
    }
    let root = temp_fixture_path("work-root-activity-events-retained-registry");
    let cache_home = temp_fixture_path("work-root-activity-events-retained-registry-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    upsert_agent_def(
        &agents_dir,
        "retained-role",
        "retained current",
        "payload-current-retained",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "",
        "idle",
        "2026-05-25T00:00:00Z",
        "",
    );

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
        while !seen.iter().any(|event| {
            event["type"] == "itemUpserted" && event["item"]["id"] == "agent:retained-role"
        }) {
            let chunk = stream
                .next()
                .await
                .expect("initial retained SSE chunk")
                .expect("initial retained SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("initial retained SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("initial retained current event");

    upsert_agent_instance(
        &agents_dir,
        "retained-role:private/state/registry-only-secret",
        "retained-role",
        "registry retained",
        "payload-retained-registry-only",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "retained-registry-session-secret",
        "completed",
        "2026-05-25T00:02:00Z",
        "",
        "retired",
        "",
        true,
    );
    update_agent_instance_registry_fields(
        &agents_dir,
        "retained-role:private/state/registry-only-secret",
        "retired",
        "2026-05-25T00:03:00Z",
        "2026-05-25T00:04:00Z",
        "2026-05-25T00:05:00Z",
        "2026-05-25T00:06:00Z",
        "2026-05-25T00:07:00Z",
    );

    let mut historical_id = String::new();
    timeout(Duration::from_secs(5), async {
        while historical_id.is_empty() {
            let chunk = stream
                .next()
                .await
                .expect("retained upsert SSE chunk")
                .expect("retained upsert SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("retained upsert SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
            if let Some(id) = seen.iter().find_map(|event| {
                (event["type"] == "itemUpserted")
                    .then(|| event["item"]["id"].as_str())
                    .flatten()
                    .filter(|id| id.starts_with("agent-instance:"))
                    .map(str::to_owned)
            }) {
                historical_id = id;
            }
        }
    })
    .await
    .expect("retained registry-only item upsert event");

    update_agent_instance_registry_fields(
        &agents_dir,
        "retained-role:private/state/registry-only-secret",
        "cleanup_deleted",
        "2026-05-25T00:08:00Z",
        "2026-05-25T00:04:00Z",
        "2026-05-25T00:05:00Z",
        "2026-05-25T00:06:00Z",
        "2026-05-25T00:07:00Z",
    );

    timeout(Duration::from_secs(5), async {
        while !seen
            .iter()
            .any(|event| event["type"] == "itemRemoved" && event["activityId"] == historical_id)
        {
            let chunk = stream
                .next()
                .await
                .expect("retained removal SSE chunk")
                .expect("retained removal SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("retained removal SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("retained registry-only removal event");

    let (status, body_text) =
        fetch_work_root_activity(app.clone(), cookie.as_str(), &work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("retained lifecycle activity JSON");
    assert_eq!(value["summary"]["total"], 1);
    assert_eq!(value["agents"].as_array().expect("agents").len(), 1);

    let events_text = serde_json::to_string(&seen).expect("retained events JSON string");
    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "retained-role:private/state/registry-only-secret".to_owned(),
        "retained-registry-session-secret".to_owned(),
        "payload-retained-registry-only".to_owned(),
        "state.sqlite".to_owned(),
        "2026-05-25T00:08:00Z".to_owned(),
    ] {
        assert!(
            !events_text.contains(&forbidden) && !body_text.contains(&forbidden),
            "retained lifecycle responses must not leak {forbidden}"
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
        upsert_agent_def(
            &agents_dir,
            &format!("agent-{index}"),
            &format!("agent-{index}"),
            &format!("agent-{index}"),
            "codex",
            "",
            "",
            "",
            "",
            "",
            "idle",
            "",
            "",
        );
        update_agent_def_registry_fields(
            &agents_dir,
            &format!("agent-{index}"),
            "idle",
            &format!("2026-05-25T00:0{index}:00Z"),
            &format!("2026-05-25T00:0{index}:00Z"),
            "",
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
    let agent_ids = value["agents"]
        .as_array()
        .expect("recent-limited agents array")
        .iter()
        .map(|agent| agent["agentId"].as_str().expect("agent id").to_owned())
        .collect::<Vec<_>>();
    assert_eq!(agent_ids, vec!["agent-3", "agent-4"]);
    let fallback_agent = value["agents"]
        .as_array()
        .expect("recent-limited agents array")
        .iter()
        .find(|agent| agent["agentId"] == "agent-4")
        .expect("agent-4 row");
    assert_eq!(fallback_agent["lastCallAt"], "2026-05-25T00:04:00Z");

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_recent_agent_limit_uses_latest_payload_or_registry_signal() {
    if skip_without_git(
        "work_root_activity_route_recent_agent_limit_uses_latest_payload_or_registry_signal",
    ) {
        return;
    }
    let root = temp_fixture_path("work-root-activity-current-combined-recency");
    let cache_home = temp_fixture_path("work-root-activity-current-combined-recency-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    for (agent_key, name, updated_at) in [
        ("payload-wins", "payload wins", "1970-01-01T00:00:01Z"),
        ("registry-newer", "registry newer", "1970-01-01T00:00:02Z"),
    ] {
        upsert_agent_def(
            &agents_dir,
            agent_key,
            name,
            agent_key,
            "codex",
            "",
            "",
            "",
            "",
            "",
            "idle",
            "",
            "",
        );
        update_agent_def_registry_fields(
            &agents_dir,
            agent_key,
            "idle",
            updated_at,
            updated_at,
            "",
        );
    }
    write_agent_output(&agents_dir, "payload-wins", "payload recency wins\n");

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity_path(
        app,
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{work_root_id}/activity?recentLimit=1"),
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("current combined recency activity JSON");
    let agent_ids = value["agents"]
        .as_array()
        .expect("recent-limited agents array")
        .iter()
        .map(|agent| agent["agentId"].as_str().expect("agent id").to_owned())
        .collect::<Vec<_>>();
    assert_eq!(agent_ids, vec!["payload-wins"]);

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_recent_agent_limit_orders_fractional_registry_timestamps() {
    if skip_without_git(
        "work_root_activity_route_recent_agent_limit_orders_fractional_registry_timestamps",
    ) {
        return;
    }
    let root = temp_fixture_path("work-root-activity-current-fractional-recency");
    let cache_home = temp_fixture_path("work-root-activity-current-fractional-recency-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    for (agent_key, name, updated_at) in [
        (
            "fractional-early",
            "fractional early",
            "2026-05-25T00:00:00.100Z",
        ),
        (
            "fractional-late",
            "fractional late",
            "2026-05-25T00:00:00.900Z",
        ),
    ] {
        upsert_agent_def(
            &agents_dir,
            agent_key,
            name,
            agent_key,
            "codex",
            "",
            "",
            "",
            "",
            "",
            "idle",
            "",
            "",
        );
        update_agent_def_registry_fields(
            &agents_dir,
            agent_key,
            "idle",
            updated_at,
            updated_at,
            "",
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
        &format!("/api/dashboard/work-roots/{work_root_id}/activity?recentLimit=1"),
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("fractional current recency activity JSON");
    let agent_ids = value["agents"]
        .as_array()
        .expect("recent-limited agents array")
        .iter()
        .map(|agent| agent["agentId"].as_str().expect("agent id").to_owned())
        .collect::<Vec<_>>();
    assert_eq!(agent_ids, vec!["fractional-late"]);

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_limits_recent_retained_items_by_registry_metadata() {
    if skip_without_git(
        "work_root_activity_route_limits_recent_retained_items_by_registry_metadata",
    ) {
        return;
    }
    let root = temp_fixture_path("work-root-activity-retained-recent-limit");
    let cache_home = temp_fixture_path("work-root-activity-retained-recent-limit-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    write_agent_metadata(
        &agents_dir,
        "current",
        &serde_json::json!({
            "schema_version": 1,
            "name": "current",
            "status": "idle"
        }),
    );
    update_agent_def_registry_fields(
        &agents_dir,
        "current",
        "idle",
        "2026-05-25T00:00:00Z",
        "2026-05-25T00:00:00Z",
        "",
    );

    for (instance_id, name, updated_at, last_output_path) in [
        (
            "current:private/state/old-retained-secret",
            "old retained",
            "2026-05-25T00:01:00Z",
            "/zzzz/private/output.md",
        ),
        (
            "current:private/state/new-retained-secret",
            "new retained",
            "2026-05-25T00:03:00Z",
            "",
        ),
    ] {
        upsert_agent_instance(
            &agents_dir,
            instance_id,
            "current",
            name,
            name.replace(' ', "-").as_str(),
            "codex",
            "codex",
            "core",
            "gpt-5.3-codex",
            "medium",
            "",
            "completed",
            "2026-05-24T23:59:00Z",
            last_output_path,
            "retired",
            "",
            true,
        );
        update_agent_instance_registry_fields(
            &agents_dir,
            instance_id,
            "retired",
            updated_at,
            updated_at,
            updated_at,
            updated_at,
            updated_at,
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
        &format!("/api/dashboard/work-roots/{work_root_id}/activity?recentLimit=1"),
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("retained recent-limited activity JSON");
    assert_eq!(value["summary"]["total"], 1);
    assert_eq!(value["agents"].as_array().expect("agents array").len(), 1);
    let labels = value["items"]
        .as_array()
        .expect("items array")
        .iter()
        .map(|item| item["label"].as_str().expect("item label").to_owned())
        .collect::<Vec<_>>();
    assert!(
        labels
            .iter()
            .any(|label| label == "new retained (historical)"),
        "registry-recent retained row should be selected; labels were {labels:?}"
    );
    assert!(
        labels
            .iter()
            .all(|label| label != "old retained (historical)"),
        "less-recent retained row should be omitted; labels were {labels:?}"
    );

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "current:private/state/old-retained-secret".to_owned(),
        "current:private/state/new-retained-secret".to_owned(),
        "/zzzz/private/output.md".to_owned(),
        "state.sqlite".to_owned(),
        "2026-05-25T00:03:00Z".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "retained recent response must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_recent_retained_limit_uses_latest_payload_or_registry_signal() {
    if skip_without_git(
        "work_root_activity_route_recent_retained_limit_uses_latest_payload_or_registry_signal",
    ) {
        return;
    }
    let root = temp_fixture_path("work-root-activity-retained-combined-recency");
    let cache_home = temp_fixture_path("work-root-activity-retained-combined-recency-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    upsert_agent_def(
        &agents_dir,
        "current",
        "current",
        "current",
        "codex",
        "",
        "",
        "",
        "",
        "",
        "idle",
        "",
        "",
    );

    for (instance_id, name, state_path, updated_at) in [
        (
            "current:private/state/payload-retained-secret",
            "payload retained",
            "payload-retained-combined",
            "1970-01-01T00:00:01Z",
        ),
        (
            "current:private/state/registry-retained-secret",
            "registry retained",
            "registry-retained-combined",
            "1970-01-01T00:00:02Z",
        ),
    ] {
        upsert_agent_instance(
            &agents_dir,
            instance_id,
            "current",
            name,
            state_path,
            "codex",
            "",
            "",
            "",
            "",
            "",
            "completed",
            "1970-01-01T00:00:00Z",
            "",
            "retired",
            "",
            true,
        );
        update_agent_instance_registry_fields(
            &agents_dir,
            instance_id,
            "retired",
            updated_at,
            updated_at,
            updated_at,
            updated_at,
            updated_at,
        );
    }
    write_agent_output_at_state_path(
        &agents_dir,
        "payload-retained-combined",
        "payload retained recency wins\n",
    );

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity_path(
        app,
        cookie.as_str(),
        &format!("/api/dashboard/work-roots/{work_root_id}/activity?recentLimit=1"),
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("retained combined recency activity JSON");
    let labels = value["items"]
        .as_array()
        .expect("items array")
        .iter()
        .map(|item| item["label"].as_str().expect("item label").to_owned())
        .collect::<Vec<_>>();
    assert!(
        labels
            .iter()
            .any(|label| label == "payload retained (historical)"),
        "payload-recent retained row should be selected; labels were {labels:?}"
    );
    assert!(
        labels
            .iter()
            .all(|label| label != "registry retained (historical)"),
        "registry-only retained row should lose to newer payload mtime; labels were {labels:?}"
    );

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "current:private/state/payload-retained-secret".to_owned(),
        "current:private/state/registry-retained-secret".to_owned(),
        "payload-retained-combined".to_owned(),
        "registry-retained-combined".to_owned(),
        "state.sqlite".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "retained combined recency response must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_recent_retained_limit_orders_fractional_registry_timestamps() {
    if skip_without_git(
        "work_root_activity_route_recent_retained_limit_orders_fractional_registry_timestamps",
    ) {
        return;
    }
    let root = temp_fixture_path("work-root-activity-retained-fractional-recency");
    let cache_home = temp_fixture_path("work-root-activity-retained-fractional-recency-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    upsert_agent_def(
        &agents_dir,
        "current",
        "current",
        "current",
        "codex",
        "",
        "",
        "",
        "",
        "",
        "idle",
        "",
        "",
    );

    for (instance_id, name, state_path, updated_at) in [
        (
            "current:private/state/fractional-early-secret",
            "fractional early",
            "fractional-retained-early",
            "2026-05-25T00:00:00.100Z",
        ),
        (
            "current:private/state/fractional-late-secret",
            "fractional late",
            "fractional-retained-late",
            "2026-05-25T00:00:00.900Z",
        ),
    ] {
        upsert_agent_instance(
            &agents_dir,
            instance_id,
            "current",
            name,
            state_path,
            "codex",
            "",
            "",
            "",
            "",
            "",
            "completed",
            "2026-05-25T00:00:00Z",
            "",
            "retired",
            "",
            true,
        );
        update_agent_instance_registry_fields(
            &agents_dir,
            instance_id,
            "retired",
            updated_at,
            updated_at,
            updated_at,
            updated_at,
            updated_at,
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
        &format!("/api/dashboard/work-roots/{work_root_id}/activity?recentLimit=1"),
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("fractional retained recency activity JSON");
    let labels = value["items"]
        .as_array()
        .expect("items array")
        .iter()
        .map(|item| item["label"].as_str().expect("item label").to_owned())
        .collect::<Vec<_>>();
    assert!(
        labels
            .iter()
            .any(|label| label == "fractional late (historical)"),
        "later fractional retained row should be selected; labels were {labels:?}"
    );
    assert!(
        labels
            .iter()
            .all(|label| label != "fractional early (historical)"),
        "earlier fractional retained row should be omitted; labels were {labels:?}"
    );

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "current:private/state/fractional-early-secret".to_owned(),
        "current:private/state/fractional-late-secret".to_owned(),
        "fractional-retained-early".to_owned(),
        "fractional-retained-late".to_owned(),
        "state.sqlite".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "fractional retained recency response must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
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

    // Unrecognized registry status must degrade only its own row.
    write_agent_metadata_raw(&agents_dir, "broken-meta", "{ this is not valid json");

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
    assert_eq!(value["summary"]["total"], 3);
    assert_eq!(value["summary"]["unavailable"], 1);

    let items = value["items"].as_array().expect("activity items array");
    assert_eq!(items.len(), 3);
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
    let healthy_item = item_row("agent:healthy");
    assert_eq!(healthy_item["status"], "idle");
    assert_eq!(healthy_item["attention"], false);
    assert!(item_diagnostics_of(healthy_item).is_empty());

    let agents = value["agents"].as_array().expect("activity agents array");
    assert_eq!(agents.len(), 3);
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

    // Unrecognized registry status degrades the row to unavailable.
    let broken_meta = agent_row("broken-meta");
    assert_eq!(broken_meta["status"], "unavailable");
    assert!(broken_meta["name"].is_null());
    assert!(!diagnostics_of(broken_meta).is_empty());

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
async fn work_root_activity_route_returns_empty_for_incompatible_registry_state() {
    if skip_without_git("work_root_activity_route_returns_empty_for_incompatible_registry_state") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-incompatible-registry");
    let cache_home = temp_fixture_path("work-root-activity-incompatible-registry-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");
    let state_dir = agents_dir.parent().expect("agents dir parent");
    fs::create_dir_all(state_dir).expect("create wsstate state dir");
    let connection =
        Connection::open(state_dir.join("state.sqlite")).expect("open incompatible registry");
    connection
        .execute_batch("CREATE TABLE incompatible_registry (id TEXT PRIMARY KEY);")
        .expect("write incompatible registry schema");

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity(app, cookie.as_str(), &work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("incompatible registry activity JSON");

    assert_eq!(value["status"], "ok");
    assert_eq!(value["summary"]["total"], 0);
    assert_eq!(value["agents"].as_array().expect("agents").len(), 0);
    assert!(!body_text.contains("state.sqlite"));
    assert!(!body_text.contains(&cache_home.display().to_string()));

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn work_root_activity_route_soft_degrades_when_registry_is_locked() {
    if skip_without_git("work_root_activity_route_soft_degrades_when_registry_is_locked") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-locked-registry");
    let cache_home = temp_fixture_path("work-root-activity-locked-registry-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");
    upsert_agent_def(
        &agents_dir,
        "locked",
        "locked",
        "locked-payload",
        "codex",
        "codex",
        "core",
        "gpt-5.3-codex",
        "medium",
        "locked-private-session",
        "running",
        "2026-05-17T10:00:00Z",
        "",
    );
    let state_dir = agents_dir.parent().expect("agents dir parent");
    let lock_connection =
        Connection::open(state_dir.join("state.sqlite")).expect("open lock fixture registry");
    lock_connection
        .execute_batch(
            "PRAGMA locking_mode=EXCLUSIVE;
             BEGIN EXCLUSIVE;
             UPDATE agent_defs SET updated_at = '2026-05-17T10:01:00Z' WHERE agent_key = 'locked';",
        )
        .expect("hold exclusive registry lock");

    let state = app_state_with_activity_cache_home(cache_home.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let (status, body_text) = fetch_work_root_activity(app, cookie.as_str(), &work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let value: serde_json::Value =
        serde_json::from_str(&body_text).expect("locked registry activity JSON");
    assert_eq!(value["status"], "ok");
    assert_eq!(value["summary"]["total"], 0);
    assert_eq!(value["agents"].as_array().expect("agents").len(), 0);

    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "state.sqlite".to_owned(),
        "locked-private-session".to_owned(),
        "locked-payload".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "locked registry response must not leak {forbidden}"
        );
    }

    drop(lock_connection);
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

#[tokio::test]
async fn work_root_activity_events_emit_current_item_for_registry_only_update() {
    if skip_without_git("work_root_activity_events_emit_current_item_for_registry_only_update") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-events-registry-current");
    let cache_home = temp_fixture_path("work-root-activity-events-registry-current-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");
    write_agent_metadata(
        &agents_dir,
        "registry-current",
        &serde_json::json!({
            "schema_version": 1,
            "name": "registry current",
            "status": "idle"
        }),
    );
    update_agent_def_registry_fields(
        &agents_dir,
        "registry-current",
        "idle",
        "2026-05-25T00:00:00Z",
        "2026-05-25T00:00:00Z",
        "",
    );

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
        while !seen.iter().any(|event| {
            event["type"] == "itemUpserted"
                && event["item"]["id"] == "agent:registry-current"
                && event["item"]["status"] == "idle"
        }) {
            let chunk = stream
                .next()
                .await
                .expect("initial registry SSE chunk")
                .expect("initial registry SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("initial registry SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("initial registry item event");

    update_agent_def_registry_fields(
        &agents_dir,
        "registry-current",
        "blocked",
        "2026-05-25T00:01:00Z",
        "2026-05-25T00:01:00Z",
        "",
    );

    timeout(Duration::from_secs(5), async {
        while !seen.iter().any(|event| {
            event["type"] == "itemUpserted"
                && event["item"]["id"] == "agent:registry-current"
                && event["item"]["status"] == "blocked"
        }) {
            let chunk = stream
                .next()
                .await
                .expect("registry-only SSE chunk")
                .expect("registry-only SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("registry-only SSE UTF-8"));
            drain_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("registry-only item update event");

    let body_text = serde_json::to_string(&seen).expect("registry SSE events JSON string");
    for forbidden in [
        root.display().to_string(),
        cache_home.display().to_string(),
        "state.sqlite".to_owned(),
    ] {
        assert!(
            !body_text.contains(&forbidden),
            "registry-only current event must not leak {forbidden}"
        );
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

async fn paired_test_app() -> (axum::Router, String) {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    (app, cookie)
}

async fn request_json_for_test(
    app: axum::Router,
    method: Method,
    uri: String,
    cookie: &str,
    body: serde_json::Value,
) -> (StatusCode, axum::http::HeaderMap, serde_json::Value) {
    let response = app
        .oneshot(
            Request::builder()
                .method(method)
                .uri(uri)
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(body.to_string()))
                .expect("JSON route request"),
        )
        .await
        .expect("JSON route response");
    let status = response.status();
    let headers = response.headers().clone();
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("JSON route body bytes");
    let value = serde_json::from_slice(&body).expect("JSON route response body");
    (status, headers, value)
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

async fn git_toolbar_get_json(
    app: axum::Router,
    cookie: &str,
    uri: &str,
    expected_status: StatusCode,
) -> serde_json::Value {
    let response = app
        .oneshot(
            Request::builder()
                .uri(uri)
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("git toolbar GET request"),
        )
        .await
        .expect("git toolbar GET response");
    assert_eq!(response.status(), expected_status, "{uri}");
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("git toolbar GET body");
    serde_json::from_slice(&body).expect("git toolbar GET JSON")
}

async fn git_toolbar_post_json(
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
                .expect("git toolbar POST request"),
        )
        .await
        .expect("git toolbar POST response");
    assert_eq!(response.status(), expected_status, "{uri}");
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("git toolbar POST body");
    serde_json::from_slice(&body).expect("git toolbar POST JSON")
}

async fn git_worktree_options_json(
    app: axum::Router,
    cookie: &str,
    workspace_id: &str,
) -> serde_json::Value {
    let response = app
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/workspaces/{workspace_id}/git-worktree-add/options"
                ))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("git worktree options request"),
        )
        .await
        .expect("git worktree options response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("git worktree options body");
    serde_json::from_slice(&body).expect("git worktree options JSON")
}

async fn git_worktree_preview_json(
    app: axum::Router,
    cookie: &str,
    workspace_id: &str,
    request: serde_json::Value,
) -> serde_json::Value {
    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/workspaces/{workspace_id}/git-worktree-add/preview"
                ))
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(request.to_string()))
                .expect("git worktree preview request"),
        )
        .await
        .expect("git worktree preview response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("git worktree preview body");
    serde_json::from_slice(&body).expect("git worktree preview JSON")
}

async fn git_worktree_submit_json(
    app: axum::Router,
    cookie: &str,
    workspace_id: &str,
    request: serde_json::Value,
    expected_status: StatusCode,
) -> serde_json::Value {
    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/workspaces/{workspace_id}/git-worktree-add"
                ))
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(request.to_string()))
                .expect("git worktree submit request"),
        )
        .await
        .expect("git worktree submit response");
    assert_eq!(response.status(), expected_status);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("git worktree submit body");
    serde_json::from_slice(&body).expect("git worktree submit JSON")
}

fn json_with_activate(mut value: serde_json::Value, activate: bool) -> serde_json::Value {
    value
        .as_object_mut()
        .expect("request object")
        .insert("activate".to_owned(), serde_json::Value::Bool(activate));
    value
}

fn blocker_codes(value: &serde_json::Value) -> Vec<String> {
    value["blockers"]
        .as_array()
        .expect("blockers")
        .iter()
        .filter_map(|blocker| blocker["code"].as_str().map(str::to_owned))
        .collect()
}

fn current_git_branch(path: &Path) -> String {
    let output = Command::new("git")
        .arg("-C")
        .arg(path)
        .args(["rev-parse", "--abbrev-ref", "HEAD"])
        .output()
        .expect("current git branch");
    assert!(output.status.success());
    String::from_utf8_lossy(&output.stdout).trim().to_owned()
}

fn git_stdout(path: &Path, args: &[&str]) -> String {
    let output = Command::new("git")
        .arg("-C")
        .arg(path)
        .args(args)
        .output()
        .expect("git stdout");
    assert!(
        output.status.success(),
        "git {args:?} failed: {}",
        String::from_utf8_lossy(&output.stderr)
    );
    String::from_utf8_lossy(&output.stdout).trim().to_owned()
}

async fn dashboard_resources_json(app: axum::Router, cookie: &str) -> serde_json::Value {
    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/resources")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("resources request"),
        )
        .await
        .expect("resources response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("resources body");
    serde_json::from_slice(&body).expect("resources JSON")
}

fn only_work_root(value: &serde_json::Value) -> &serde_json::Value {
    let roots = value["workspaces"]
        .as_array()
        .expect("workspaces array")
        .iter()
        .flat_map(|workspace| {
            workspace["workRoots"]
                .as_array()
                .expect("workRoots array")
                .iter()
        })
        .collect::<Vec<_>>();
    assert_eq!(roots.len(), 1, "expected exactly one known workRoot");
    roots[0]
}

fn work_root_by_id<'a>(value: &'a serde_json::Value, work_root_id: &str) -> &'a serde_json::Value {
    value["workspaces"]
        .as_array()
        .expect("workspaces array")
        .iter()
        .flat_map(|workspace| {
            workspace["workRoots"]
                .as_array()
                .expect("workRoots array")
                .iter()
        })
        .find(|root| root["id"] == work_root_id)
        .expect("workRoot id present")
}

async fn set_work_root_activation_for_test(
    app: axum::Router,
    cookie: &str,
    work_root_id: &str,
    activation: &str,
) -> serde_json::Value {
    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/activation"
                ))
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "activation": activation }).to_string(),
                ))
                .expect("set workRoot activation request"),
        )
        .await
        .expect("set workRoot activation response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("set workRoot activation body");
    serde_json::from_slice(&body).expect("set workRoot activation JSON")
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

fn app_state_with_translation_provider(base_url: String, default_model: Option<&str>) -> AppState {
    AppState {
        config: ServeConfig::default_loopback(),
        auth: OwnerAuthState::new_ephemeral(),
        opened_work_roots: OpenedWorkRoots::default(),
        dashboard_state: DashboardStateStore::disabled(),
        document_translation: DocumentTranslationService::new(Some(TranslationProviderConfig {
            id: "test-provider".to_owned(),
            label: "Test provider".to_owned(),
            base_url,
            api_key: Some("test-key-redacted".to_owned()),
            default_model: default_model.map(str::to_owned),
            timeout_ms: 5_000,
        })),
        terminals: TerminalRegistry::default(),
        work_root_activity: WorkRootActivityProjector::default(),
        document_events: DocumentEventHub::default(),
        document_write_locks: ws_dashboard_daemon::work_root_files::DocumentWriteLocks::default(),
        linked_server_sessions: LinkedServerSessions::default(),
        linked_server_tunnels: LinkedServerTunnels::record_only_for_tests(),
        registry_persist_lock: Arc::new(Mutex::new(())),
    }
}

async fn start_fake_openai_provider(response_content: &'static str) -> (String, Arc<AtomicU64>) {
    async fn models() -> axum::Json<serde_json::Value> {
        axum::Json(serde_json::json!({ "data": [{ "id": "fake-model" }] }))
    }
    async fn chat(
        axum::extract::State(state): axum::extract::State<(Arc<AtomicU64>, &'static str)>,
        axum::Json(_request): axum::Json<serde_json::Value>,
    ) -> axum::Json<serde_json::Value> {
        state.0.fetch_add(1, Ordering::SeqCst);
        axum::Json(serde_json::json!({
            "choices": [{ "message": { "content": state.1 } }]
        }))
    }

    let calls = Arc::new(AtomicU64::new(0));
    let app = axum::Router::new()
        .route("/v1/models", axum::routing::get(models))
        .route("/v1/chat/completions", axum::routing::post(chat))
        .with_state((calls.clone(), response_content));
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind fake provider");
    let addr = listener.local_addr().expect("fake provider addr");
    tokio::spawn(async move {
        let _ = axum::serve(listener, app).await;
    });
    (format!("http://{addr}/v1"), calls)
}

fn translation_request_body() -> serde_json::Value {
    serde_json::json!({
        "source": {
            "kind": "workRootFile",
            "workRootId": "root-local-test",
            "path": "docs/readme.md",
            "contentHash": "fnv1a32:testhash",
            "format": "markdown",
            "title": "readme.md"
        },
        "provider": { "id": "test-provider", "model": "fake-model" },
        "locale": { "source": null, "target": "ko" },
        "blocks": [
            {
                "blockId": "paragraph-1",
                "ordinal": 0,
                "kind": "paragraph",
                "markdown": "Hello",
                "plainText": "Hello",
                "lineStart": 1,
                "lineEnd": 1,
                "pathref": "@docs/readme.md#L1",
                "translatable": true
            },
            {
                "blockId": "code-2",
                "ordinal": 1,
                "kind": "code",
                "markdown": "```sh\necho hi\n```",
                "plainText": "echo hi",
                "lineStart": 2,
                "lineEnd": 4,
                "pathref": "@docs/readme.md#L2-L4",
                "translatable": false
            }
        ]
    })
}

#[tokio::test]
async fn document_translation_routes_require_owner_auth() {
    let app = build_router(app_state());
    let providers = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/document-translation/providers")
                .body(Body::empty())
                .expect("providers request"),
        )
        .await
        .expect("providers response");
    assert_eq!(providers.status(), StatusCode::UNAUTHORIZED);

    let translate = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/document-translation/translate")
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(translation_request_body().to_string()))
                .expect("translate request"),
        )
        .await
        .expect("translate response");
    assert_eq!(translate.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn document_translation_provider_status_probes_models_without_secrets() {
    let (base_url, _calls) = start_fake_openai_provider("{\"blocks\":[]}").await;
    let state = app_state_with_translation_provider(base_url, None);
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/document-translation/providers")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("providers request"),
        )
        .await
        .expect("providers response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("providers body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("providers json");
    assert_eq!(value["providers"][0]["reachable"], true);
    assert_eq!(value["providers"][0]["models"][0]["id"], "fake-model");
    assert!(!String::from_utf8_lossy(&body).contains("test-key-redacted"));
}

#[tokio::test]
async fn document_translation_translate_validates_blocks_and_reuses_cache() {
    let (base_url, calls) = start_fake_openai_provider(
        r#"{"blocks":[{"blockId":"paragraph-1","translatedContent":"안녕하세요"}]}"#,
    )
    .await;
    let state = app_state_with_translation_provider(base_url, Some("fake-model"));
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    for expected_hit in [false, true] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method(Method::POST)
                    .uri("/api/dashboard/document-translation/translate")
                    .header(header::COOKIE, cookie.clone())
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::from(translation_request_body().to_string()))
                    .expect("translate request"),
            )
            .await
            .expect("translate response");
        assert_eq!(response.status(), StatusCode::OK);
        let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
            .await
            .expect("translate body");
        let value: serde_json::Value = serde_json::from_slice(&body).expect("translate json");
        assert_eq!(value["cache"]["hit"], expected_hit);
        assert_eq!(value["blocks"][0]["status"], "ok");
        assert_eq!(value["blocks"][0]["translatedMarkdown"], "안녕하세요");
        assert_eq!(value["blocks"][1]["status"], "omitted");
        assert!(!String::from_utf8_lossy(&body).contains("translatedContent"));
        assert!(!String::from_utf8_lossy(&body).contains("test-key-redacted"));
    }
    let mut source_locale_request = translation_request_body();
    source_locale_request["locale"]["source"] = serde_json::json!("en");
    let source_locale_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/document-translation/translate")
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(source_locale_request.to_string()))
                .expect("source locale translate request"),
        )
        .await
        .expect("source locale translate response");
    assert_eq!(source_locale_response.status(), StatusCode::OK);
    let source_locale_body = axum::body::to_bytes(source_locale_response.into_body(), 64 * 1024)
        .await
        .expect("source locale body");
    let source_locale_value: serde_json::Value =
        serde_json::from_slice(&source_locale_body).expect("source locale json");
    assert_eq!(source_locale_value["cache"]["hit"], false);
    assert_eq!(calls.load(Ordering::SeqCst), 2);
}

#[tokio::test]
async fn document_translation_bounds_parse_failures_and_duplicate_request_ids() {
    let (base_url, _calls) = start_fake_openai_provider(
        r#"{"blocks":[{"blockId":"paragraph-1","translatedContent":"하나"},{"blockId":"RAW_UNKNOWN_BLOCK_ID_SENTINEL","translatedContent":"RAW_TRANSLATION_SENTINEL"},{"blockId":"RAW_MISSING_TRANSLATION_BLOCK_ID_SENTINEL"},{"blockId":"paragraph-1","translatedContent":"둘"}]}"#,
    )
    .await;
    let state = app_state_with_translation_provider(base_url, Some("fake-model"));
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let mut duplicate = translation_request_body();
    duplicate["blocks"][1]["blockId"] = serde_json::json!("paragraph-1");
    let duplicate_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/document-translation/translate")
                .header(header::COOKIE, cookie.clone())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(duplicate.to_string()))
                .expect("duplicate request"),
        )
        .await
        .expect("duplicate response");
    assert_eq!(duplicate_response.status(), StatusCode::BAD_REQUEST);

    let mut absolute_path = translation_request_body();
    absolute_path["source"]["path"] = serde_json::json!("/Users/example/private.md");
    let absolute_path_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/document-translation/translate")
                .header(header::COOKIE, cookie.clone())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(absolute_path.to_string()))
                .expect("absolute path request"),
        )
        .await
        .expect("absolute path response");
    assert_eq!(absolute_path_response.status(), StatusCode::BAD_REQUEST);

    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/document-translation/translate")
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(translation_request_body().to_string()))
                .expect("translate request"),
        )
        .await
        .expect("translate response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("translate body");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("translate json");
    assert_eq!(value["status"], "partial");
    assert!(
        value["unmatched"]
            .as_array()
            .expect("unmatched array")
            .len()
            >= 2
    );
    let body_text = String::from_utf8_lossy(&body);
    assert!(!body_text.contains("RAW_UNKNOWN_BLOCK_ID_SENTINEL"));
    assert!(!body_text.contains("RAW_TRANSLATION_SENTINEL"));
    assert!(!body_text.contains("RAW_MISSING_TRANSLATION_BLOCK_ID_SENTINEL"));

    let (parse_base_url, _parse_calls) =
        start_fake_openai_provider("RAW_PARSE_FAILURE_SENTINEL").await;
    let parse_state = app_state_with_translation_provider(parse_base_url, Some("fake-model"));
    let parse_token = parse_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let parse_app = build_router(parse_state);
    let parse_cookie = pair_and_cookie(parse_app.clone(), &parse_token).await;
    let parse_response = parse_app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/document-translation/translate")
                .header(header::COOKIE, parse_cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(translation_request_body().to_string()))
                .expect("parse failure request"),
        )
        .await
        .expect("parse failure response");
    assert_eq!(parse_response.status(), StatusCode::OK);
    let parse_body = axum::body::to_bytes(parse_response.into_body(), 64 * 1024)
        .await
        .expect("parse body");
    let parse_body_text = String::from_utf8_lossy(&parse_body);
    assert!(!parse_body_text.contains("RAW_PARSE_FAILURE_SENTINEL"));
}

#[tokio::test]
async fn work_root_file_write_routes_save_and_reject_conflicts() {
    let root = temp_fixture_path("write-file");
    fs::create_dir_all(&root).expect("create root");
    fs::write(root.join("doc.md"), "# Before\n").expect("write doc");
    let opened = OpenedWorkRoots::from_paths(vec![root.clone()]);
    let work_root_id = opened.register_path(root.clone());
    let state = app_state_with_opened_and_store(opened, DashboardStateStore::disabled());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let read = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/read?path=doc.md",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie.clone())
                .body(Body::empty())
                .expect("read request"),
        )
        .await
        .expect("read response");
    assert_eq!(read.status(), StatusCode::OK);
    let read_body = axum::body::to_bytes(read.into_body(), 64 * 1024)
        .await
        .expect("read body");
    let read_json: serde_json::Value = serde_json::from_slice(&read_body).expect("read json");
    let base_hash = read_json["contentHash"].as_str().expect("content hash");
    assert!(base_hash.starts_with("sha256:"));

    let stale = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/write",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie.clone())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "path": "doc.md",
                        "baseContentHash": "sha256:stale",
                        "content": "# Stale\n"
                    })
                    .to_string(),
                ))
                .expect("stale write request"),
        )
        .await
        .expect("stale write response");
    assert_eq!(stale.status(), StatusCode::CONFLICT);

    let write = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/write",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "path": "doc.md",
                        "baseContentHash": base_hash,
                        "content": "# After\n"
                    })
                    .to_string(),
                ))
                .expect("write request"),
        )
        .await
        .expect("write response");
    assert_eq!(write.status(), StatusCode::OK);
    let write_body = axum::body::to_bytes(write.into_body(), 64 * 1024)
        .await
        .expect("write body");
    let write_json: serde_json::Value = serde_json::from_slice(&write_body).expect("write json");
    assert!(write_json["contentHash"]
        .as_str()
        .unwrap()
        .starts_with("sha256:"));
    assert_eq!(
        fs::read_to_string(root.join("doc.md")).expect("saved doc"),
        "# After\n"
    );
    remove_static_fixture(&root);
}

#[tokio::test]
async fn work_root_file_write_routes_are_owner_authenticated_and_reject_traversal() {
    let root = temp_fixture_path("write-auth");
    fs::create_dir_all(&root).expect("create root");
    fs::write(root.join("doc.md"), "# Before\n").expect("write doc");
    let opened = OpenedWorkRoots::from_paths(vec![root.clone()]);
    let work_root_id = opened.register_path(root.clone());
    let state = app_state_with_opened_and_store(opened, DashboardStateStore::disabled());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let body = serde_json::json!({
        "path": "../outside.md",
        "baseContentHash": "sha256:none",
        "content": "bad"
    })
    .to_string();
    let unauth = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/write",
                    work_root_id.as_str()
                ))
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(body.clone()))
                .expect("unauth write"),
        )
        .await
        .expect("unauth response");
    assert_eq!(unauth.status(), StatusCode::UNAUTHORIZED);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let traversal = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/write",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(body))
                .expect("traversal write"),
        )
        .await
        .expect("traversal response");
    assert_eq!(traversal.status(), StatusCode::BAD_REQUEST);
    let traversal_body = axum::body::to_bytes(traversal.into_body(), 4096)
        .await
        .expect("traversal body");
    assert!(!String::from_utf8_lossy(&traversal_body).contains(root.to_string_lossy().as_ref()));
    remove_static_fixture(&root);
}

#[tokio::test]
async fn work_root_file_write_routes_serialize_same_source_optimistic_saves() {
    let root = temp_fixture_path("write-concurrent");
    fs::create_dir_all(&root).expect("create root");
    fs::write(root.join("doc.md"), "# Before\n").expect("write doc");
    let opened = OpenedWorkRoots::from_paths(vec![root.clone()]);
    let work_root_id = opened.register_path(root.clone());
    let state = app_state_with_opened_and_store(opened, DashboardStateStore::disabled());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let read = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/read?path=doc.md",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie.clone())
                .body(Body::empty())
                .expect("read before concurrent writes"),
        )
        .await
        .expect("read before concurrent writes response");
    let read_body = axum::body::to_bytes(read.into_body(), 64 * 1024)
        .await
        .expect("read body");
    let read_json: serde_json::Value = serde_json::from_slice(&read_body).expect("read json");

    let write_request = |content: &'static str| {
        let app = app.clone();
        let cookie = cookie.clone();
        let work_root_id = work_root_id.clone();
        let base_content_hash = read_json["contentHash"].clone();
        async move {
            app.oneshot(
                Request::builder()
                    .method(Method::POST)
                    .uri(format!(
                        "/api/dashboard/work-roots/{}/files/write",
                        work_root_id.as_str()
                    ))
                    .header(header::COOKIE, cookie)
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::from(
                        serde_json::json!({
                            "path": "doc.md",
                            "baseContentHash": base_content_hash,
                            "content": content
                        })
                        .to_string(),
                    ))
                    .expect("concurrent write"),
            )
            .await
            .expect("concurrent write response")
            .status()
        }
    };

    let (left, right) = tokio::join!(write_request("# Left\n"), write_request("# Right\n"));
    let mut statuses = [left, right];
    statuses.sort_by_key(|status| status.as_u16());
    assert_eq!(statuses, [StatusCode::OK, StatusCode::CONFLICT]);
    remove_static_fixture(&root);
}

#[tokio::test]
async fn work_root_file_write_routes_reject_unknown_unavailable_oversized_and_binary() {
    let root = temp_fixture_path("write-validation");
    fs::create_dir_all(&root).expect("create root");
    fs::write(root.join("doc.md"), "# Before\n").expect("write doc");
    fs::write(root.join("binary.bin"), b"abc\0def").expect("write binary");
    let opened = OpenedWorkRoots::from_paths(vec![root.clone()]);
    let work_root_id = opened.register_path(root.clone());
    let state = app_state_with_opened_and_store(opened, DashboardStateStore::disabled());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let unknown = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/missing-root/files/write")
                .header(header::COOKIE, cookie.clone())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "path": "doc.md",
                        "baseContentHash": "sha256:none",
                        "content": "ignored"
                    })
                    .to_string(),
                ))
                .expect("unknown write"),
        )
        .await
        .expect("unknown write response");
    assert_eq!(unknown.status(), StatusCode::NOT_FOUND);

    let oversized = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/write",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie.clone())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "path": "doc.md",
                        "baseContentHash": "sha256:none",
                        "content": "x".repeat(1024 * 1024 + 1)
                    })
                    .to_string(),
                ))
                .expect("oversized write"),
        )
        .await
        .expect("oversized write response");
    assert_eq!(oversized.status(), StatusCode::BAD_REQUEST);

    let binary = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/write",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie.clone())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "path": "binary.bin",
                        "baseContentHash": "sha256:none",
                        "content": "text"
                    })
                    .to_string(),
                ))
                .expect("binary write"),
        )
        .await
        .expect("binary write response");
    assert_eq!(binary.status(), StatusCode::BAD_REQUEST);

    remove_static_fixture(&root);
    let unavailable = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/write",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({
                        "path": "doc.md",
                        "baseContentHash": "sha256:none",
                        "content": "ignored"
                    })
                    .to_string(),
                ))
                .expect("unavailable write"),
        )
        .await
        .expect("unavailable write response");
    assert_eq!(unavailable.status(), StatusCode::CONFLICT);
}

#[tokio::test]
async fn work_root_document_events_route_is_authenticated_and_rejects_unknown_work_root() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let unauth = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/missing-root/documents/events")
                .body(Body::empty())
                .expect("unauth document events"),
        )
        .await
        .expect("unauth document events response");
    assert_eq!(unauth.status(), StatusCode::UNAUTHORIZED);

    let cookie = pair_and_cookie(app.clone(), &token).await;
    let unknown = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/work-roots/missing-root/documents/events")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("unknown document events"),
        )
        .await
        .expect("unknown document events response");
    assert_eq!(unknown.status(), StatusCode::NOT_FOUND);
}

#[tokio::test]
async fn work_root_document_events_deliver_save_invalidation() {
    let root = temp_fixture_path("document-events-save");
    fs::create_dir_all(&root).expect("create root");
    fs::write(root.join("doc.md"), "# Before\n").expect("write doc");
    let opened = OpenedWorkRoots::from_paths(vec![root.clone()]);
    let work_root_id = opened.register_path(root.clone());
    let state = app_state_with_opened_and_store(opened, DashboardStateStore::disabled());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let events = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{}/documents/events",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie.clone())
                .body(Body::empty())
                .expect("document events request"),
        )
        .await
        .expect("document events response");
    assert_eq!(events.status(), StatusCode::OK);
    let mut stream = events.into_body().into_data_stream();

    let read = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/read?path=doc.md",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie.clone())
                .body(Body::empty())
                .expect("read before write"),
        )
        .await
        .expect("read before write response");
    let read_body = axum::body::to_bytes(read.into_body(), 64 * 1024)
        .await
        .expect("read body");
    let read_json: serde_json::Value = serde_json::from_slice(&read_body).expect("read json");
    let write_body = serde_json::json!({
        "path": "doc.md",
        "baseContentHash": read_json["contentHash"],
        "content": "# After\n"
    })
    .to_string();
    let write = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{}/files/write",
                    work_root_id.as_str()
                ))
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(write_body))
                .expect("write request"),
        )
        .await
        .expect("write response");
    assert_eq!(write.status(), StatusCode::OK);
    let write_body = axum::body::to_bytes(write.into_body(), 64 * 1024)
        .await
        .expect("write body");
    let write_json: serde_json::Value = serde_json::from_slice(&write_body).expect("write json");

    let mut buffer = String::new();
    let mut seen = Vec::<serde_json::Value>::new();
    timeout(Duration::from_secs(5), async {
        while !seen.iter().any(|event| {
            event["type"] == "document.contentChanged"
                && event["source"]["path"] == "doc.md"
                && event["contentHash"] == write_json["contentHash"]
        }) {
            let chunk = stream
                .next()
                .await
                .expect("document SSE chunk")
                .expect("document SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("document SSE UTF-8"));
            drain_document_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("save invalidation event");

    remove_static_fixture(&root);
}

fn drain_document_sse_events(buffer: &mut String, events: &mut Vec<serde_json::Value>) {
    while let Some(boundary) = buffer.find("\n\n") {
        let frame = buffer[..boundary].to_owned();
        *buffer = buffer[(boundary + 2)..].to_owned();
        if !frame.lines().any(|line| line == "event: document") {
            continue;
        }
        let data = frame
            .lines()
            .find_map(|line| line.strip_prefix("data: "))
            .expect("document SSE frame data field");
        events.push(serde_json::from_str(data).expect("document SSE data JSON"));
    }
}
