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
use axum::response::IntoResponse;
use tokio::net::TcpListener;
use tokio::sync::{Mutex, Notify};
use tokio::time::timeout;
use tokio_tungstenite::tungstenite::{client::IntoClientRequest, Message as TungsteniteMessage};
use tower::ServiceExt;
use ws_dashboard_core::claude_projection::ClaudeProjector;
use ws_dashboard_core::codex_projection::CodexProjector;
use ws_dashboard_core::{ServerId, ServerKind, WorkRootId};
use ws_dashboard_daemon::claude_cli::{ClaudeConnection, ClaudeProviderRegistry};
use ws_dashboard_daemon::codex_app_server::{CodexConnection, CodexProviderRegistry};
use ws_dashboard_daemon::codex_routes::CodexControlRequest;
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

// CONTRACT (260723 Phase 1 risk signal "helper binary path resolution must
// not hardcode current_exe()"): `TerminalRegistry::default()`'s production
// fallback resolves the helper binary via `std::env::current_exe()`, which
// inside THIS test binary would resolve to the `routes` test executable
// itself, not `ws-dashboard` - any test that actually creates and drives a
// real terminal must go through this constructor instead, pointed at the
// Cargo-provided real compiled binary. Each call gets its own isolated
// registry directory so concurrent tests never share terminal socket paths.
fn test_terminal_registry() -> TerminalRegistry {
    TerminalRegistry::new(
        PathBuf::from(env!("CARGO_BIN_EXE_ws-dashboard")),
        temp_fixture_path("terminal-registry"),
        Duration::from_secs(5),
    )
}

fn app_state() -> AppState {
    app_state_with_opened_and_store(OpenedWorkRoots::default(), DashboardStateStore::disabled())
}

fn app_state_without_owner_auth() -> AppState {
    let mut state = app_state();
    state.config.owner_auth_enabled = false;
    state
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
        terminals: test_terminal_registry(),
        codex_sessions: ws_dashboard_daemon::codex_app_server::CodexProviderRegistry::default(),
        claude_sessions: ws_dashboard_daemon::claude_cli::ClaudeProviderRegistry::default(),
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
        terminals: test_terminal_registry(),
        codex_sessions: ws_dashboard_daemon::codex_app_server::CodexProviderRegistry::default(),
        claude_sessions: ws_dashboard_daemon::claude_cli::ClaudeProviderRegistry::default(),
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
        terminals: test_terminal_registry(),
        codex_sessions: ws_dashboard_daemon::codex_app_server::CodexProviderRegistry::default(),
        claude_sessions: ws_dashboard_daemon::claude_cli::ClaudeProviderRegistry::default(),
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

    for (method, uri, body) in [
        (
            Method::GET,
            "/api/dashboard/servers/server-local/work-roots/root-test/files",
            Body::empty(),
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-local/work-roots/root-test/files/read?path=src/main.rs",
            Body::empty(),
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/work-roots/root-test/files/write",
            Body::from(
                serde_json::json!({
                    "path": "src/main.rs",
                    "baseContentHash": "sha256:missing",
                    "content": "changed"
                })
                .to_string(),
            ),
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-local/work-roots/root-test/documents/events",
            Body::empty(),
        ),
    ] {
        let unauthenticated = app
            .clone()
            .oneshot(
                Request::builder()
                    .method(method)
                    .uri(uri)
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(body)
                    .expect("unauthenticated server scoped file/document request"),
            )
            .await
            .expect("unauthenticated server scoped file/document response");
        assert_eq!(unauthenticated.status(), StatusCode::UNAUTHORIZED, "{uri}");
    }

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
async fn server_scoped_work_root_files_and_document_routes_dispatch_equivalent_local_aliases() {
    let root = temp_fixture_path("server-scoped-file-local-root");
    fs::create_dir_all(root.join("src")).expect("create local alias file root");
    fs::write(root.join("src/main.rs"), "fn main() {}\n").expect("write local alias file");

    let state = app_state_with_opened_and_store(
        OpenedWorkRoots::from_paths(vec![root.clone()]),
        DashboardStateStore::disabled(),
    );
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    let legacy_list = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/files?path=src"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("legacy file list request"),
        )
        .await
        .expect("legacy file list response");
    let scoped_list = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-local/work-roots/{work_root_id}/files?path=src"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("server scoped local file list request"),
        )
        .await
        .expect("server scoped local file list response");
    assert_eq!(legacy_list.status(), StatusCode::OK);
    assert_eq!(scoped_list.status(), StatusCode::OK);
    assert_eq!(
        axum::body::to_bytes(scoped_list.into_body(), 64 * 1024)
            .await
            .expect("scoped list body"),
        axum::body::to_bytes(legacy_list.into_body(), 64 * 1024)
            .await
            .expect("legacy list body")
    );

    let legacy_read = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/files/read?path=src/main.rs"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("legacy file read request"),
        )
        .await
        .expect("legacy file read response");
    let scoped_read = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-local/work-roots/{work_root_id}/files/read?path=src/main.rs"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("server scoped local file read request"),
        )
        .await
        .expect("server scoped local file read response");
    assert_eq!(legacy_read.status(), StatusCode::OK);
    assert_eq!(scoped_read.status(), StatusCode::OK);
    let legacy_read_body = axum::body::to_bytes(legacy_read.into_body(), 64 * 1024)
        .await
        .expect("legacy read body");
    let scoped_read_body = axum::body::to_bytes(scoped_read.into_body(), 64 * 1024)
        .await
        .expect("scoped read body");
    assert_eq!(scoped_read_body, legacy_read_body);
    let read_json: serde_json::Value =
        serde_json::from_slice(&legacy_read_body).expect("legacy read JSON");

    let legacy_events = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/documents/events"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("legacy document events request"),
        )
        .await
        .expect("legacy document events response");
    let scoped_events = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-local/work-roots/{work_root_id}/documents/events"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("server scoped document events request"),
        )
        .await
        .expect("server scoped document events response");
    assert_eq!(legacy_events.status(), StatusCode::OK);
    assert_eq!(scoped_events.status(), StatusCode::OK);
    let mut legacy_stream = legacy_events.into_body().into_data_stream();
    let mut scoped_stream = scoped_events.into_body().into_data_stream();

    let write_body = serde_json::json!({
        "path": "src/main.rs",
        "baseContentHash": read_json["contentHash"],
        "content": "fn main() { println!(\"scoped\"); }\n"
    });
    let (scoped_write_status, _, scoped_write) = request_json_for_test(
        app.clone(),
        Method::POST,
        format!("/api/dashboard/servers/server-local/work-roots/{work_root_id}/files/write"),
        &cookie,
        write_body,
    )
    .await;
    assert_eq!(scoped_write_status, StatusCode::OK);
    assert_eq!(scoped_write["sizeBytes"], 34);

    let missing_content_type = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/servers/server-local/work-roots/{work_root_id}/files/write"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::from(
                    serde_json::json!({
                        "path": "src/main.rs",
                        "baseContentHash": scoped_write["contentHash"],
                        "content": "missing content type\n"
                    })
                    .to_string(),
                ))
                .expect("server scoped write missing content-type request"),
        )
        .await
        .expect("server scoped write missing content-type response");
    let legacy_missing_content_type = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/files/write"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::from(
                    serde_json::json!({
                        "path": "src/main.rs",
                        "baseContentHash": scoped_write["contentHash"],
                        "content": "missing content type\n"
                    })
                    .to_string(),
                ))
                .expect("legacy write missing content-type request"),
        )
        .await
        .expect("legacy write missing content-type response");
    assert_eq!(
        missing_content_type.status(),
        legacy_missing_content_type.status(),
        "server-local scoped write should preserve legacy JSON content-type boundary"
    );

    let (stale_hash_status, _, stale_hash) = request_json_for_test(
        app.clone(),
        Method::POST,
        format!("/api/dashboard/servers/server-local/work-roots/{work_root_id}/files/write"),
        &cookie,
        serde_json::json!({
            "path": "src/main.rs",
            "baseContentHash": read_json["contentHash"],
            "content": "stale hash should conflict\n"
        }),
    )
    .await;
    assert_eq!(stale_hash_status, StatusCode::CONFLICT);
    assert_eq!(stale_hash["error"], "content hash mismatch");

    let missing_hash_body = serde_json::json!({
        "path": "src/main.rs",
        "content": "missing hash"
    })
    .to_string();
    let missing_hash = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/servers/server-local/work-roots/{work_root_id}/files/write"
                ))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(missing_hash_body.clone()))
                .expect("server scoped missing base hash write request"),
        )
        .await
        .expect("server scoped missing base hash write response");
    let legacy_missing_hash = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/work-roots/{work_root_id}/files/write"))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(missing_hash_body))
                .expect("legacy missing base hash write request"),
        )
        .await
        .expect("legacy missing base hash write response");
    assert_eq!(missing_hash.status(), StatusCode::UNPROCESSABLE_ENTITY);
    assert_eq!(
        missing_hash.status(),
        legacy_missing_hash.status(),
        "server-local scoped write should preserve legacy JSON data-error status"
    );

    for (label, stream) in [
        ("legacy", &mut legacy_stream),
        ("scoped", &mut scoped_stream),
    ] {
        let mut buffer = String::new();
        let mut seen = Vec::<serde_json::Value>::new();
        timeout(Duration::from_secs(5), async {
            while !seen.iter().any(|event| {
                event["type"] == "document.contentChanged"
                    && event["source"]["path"] == "src/main.rs"
                    && event["contentHash"] == scoped_write["contentHash"]
            }) {
                let chunk = stream
                    .next()
                    .await
                    .unwrap_or_else(|| panic!("{label} document SSE chunk"))
                    .unwrap_or_else(|_| panic!("{label} document SSE body chunk"));
                buffer.push_str(std::str::from_utf8(&chunk).expect("document SSE UTF-8"));
                drain_document_sse_events(&mut buffer, &mut seen);
            }
        })
        .await
        .unwrap_or_else(|_| panic!("{label} save invalidation event"));
    }

    remove_static_fixture(&root);
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

    let unknown_file = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-missing/work-roots/root-test/files")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("unknown server scoped file request"),
        )
        .await
        .expect("unknown server scoped file response");
    assert_eq!(unknown_file.status(), StatusCode::NOT_FOUND);

    let unknown_document_events = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-missing/work-roots/root-test/documents/events")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("unknown server scoped document events request"),
        )
        .await
        .expect("unknown server scoped document events response");
    assert_eq!(unknown_document_events.status(), StatusCode::NOT_FOUND);

    for uri in [
        "/api/dashboard/servers/server-windows/root-picker",
        "/api/dashboard/servers/server-windows/work-roots/root-test/files",
        "/api/dashboard/servers/server-windows/work-roots/root-test/files/read?path=README.md",
        "/api/dashboard/servers/server-windows/work-roots/root-test/documents/events",
        // Phase 5 allowlisted GET aliases resolve through the same forwarding
        // refusal path, so a tokenless linked server refuses them with the
        // bounded auth-required conflict rather than 404.
        "/api/dashboard/servers/server-windows/work-roots/root-test/activity",
        "/api/dashboard/servers/server-windows/work-roots/root-test/activity/events",
        "/api/dashboard/servers/server-windows/work-roots/root-test/activity/items/agent-x/transcript",
        "/api/dashboard/servers/server-windows/work-roots/root-test/git/status",
        "/api/dashboard/servers/server-windows/work-roots/root-test/git/branches",
        "/api/dashboard/servers/server-windows/workspaces/workspace-test/git-worktree-add/options",
        // Phase 6 registers the terminal HTTP lifecycle GET aliases (list,
        // output), so a tokenless linked server refuses them with the same
        // bounded auth-required conflict rather than 404.
        "/api/dashboard/servers/server-windows/work-roots/root-test/terminals",
        "/api/dashboard/servers/server-windows/terminals/terminal-1/output",
    ] {
        let auth_required = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .body(Body::empty())
                    .expect("auth-required server scoped route request"),
            )
            .await
            .expect("auth-required server scoped route response");
        assert_eq!(auth_required.status(), StatusCode::CONFLICT, "{uri}");
        let auth_body = axum::body::to_bytes(auth_required.into_body(), 4096)
            .await
            .expect("auth-required body");
        let auth_value: serde_json::Value =
            serde_json::from_slice(&auth_body).expect("auth-required response JSON");
        assert_eq!(auth_value["error"], "linked server auth required", "{uri}");
        assert!(
            !auth_body
                .windows(b"owner@example.test".len())
                .any(|window| window == b"owner@example.test"),
            "server scoped refusal must not expose SSH target for {uri}"
        );
    }

    // Gateway-owned translation is local-only, so it stays unregistered under
    // the server-scoped prefix and still 404s. (The terminal socket route is
    // now registered by Phase 7; its refusal-before-upgrade behavior is
    // covered by the dedicated WebSocket tests, since a plain GET without an
    // Upgrade header is rejected by the WebSocketUpgrade extractor before the
    // refusal logic runs.)
    for uri in ["/api/dashboard/servers/server-windows/document-translation/providers"] {
        let not_forwarded = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .body(Body::empty())
                    .expect("non-allowlisted server scoped route request"),
            )
            .await
            .expect("non-allowlisted server scoped route response");
        assert_eq!(not_forwarded.status(), StatusCode::NOT_FOUND, "{uri}");
    }

    // Phase 6 also registers the terminal mutation aliases (input, resize,
    // close), which refuse a tokenless linked server with the bounded
    // auth-required conflict before any upstream request.
    for (method, uri) in [
        (
            Method::POST,
            "/api/dashboard/servers/server-windows/terminals/terminal-1/input",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-windows/terminals/terminal-1/resize",
        ),
        (
            Method::DELETE,
            "/api/dashboard/servers/server-windows/terminals/terminal-1",
        ),
    ] {
        let auth_required = app
            .clone()
            .oneshot(
                Request::builder()
                    .method(method)
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::from(
                        serde_json::json!({ "data": "x", "columns": 80, "rows": 24 }).to_string(),
                    ))
                    .expect("terminal mutation refusal request"),
            )
            .await
            .expect("terminal mutation refusal response");
        assert_eq!(auth_required.status(), StatusCode::CONFLICT, "{uri}");
        let auth_body = axum::body::to_bytes(auth_required.into_body(), 4096)
            .await
            .expect("terminal mutation refusal body");
        let auth_value: serde_json::Value =
            serde_json::from_slice(&auth_body).expect("terminal mutation refusal JSON");
        assert_eq!(auth_value["error"], "linked server auth required", "{uri}");
    }

    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn server_scoped_one_shot_routes_refuse_dotted_server_routes() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let dotted = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server.windows/root-picker")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("dotted server route request"),
        )
        .await
        .expect("dotted server route response");
    assert_eq!(dotted.status(), StatusCode::BAD_REQUEST);
    let dotted_body = axum::body::to_bytes(dotted.into_body(), 4096)
        .await
        .expect("dotted route body");
    let dotted_value: serde_json::Value =
        serde_json::from_slice(&dotted_body).expect("dotted route JSON");
    assert_eq!(
        dotted_value["error"],
        "invalid server route; re-add the linked server under a dot-free route"
    );
    assert!(
        !dotted_body
            .windows(b"server.windows".len())
            .any(|window| window == b"server.windows"),
        "invalid-route refusal must not echo the requested route"
    );
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
async fn server_scoped_one_shot_route_returns_bounded_tunnel_required_after_endpoint_drops() {
    // A linked-server session can outlive its tunnel: the owner links while an
    // endpoint hint is present, then the tunnel is torn down (e.g. SSH drop)
    // without a fresh /link-auth, leaving a live session but no endpoint hint.
    // `resolve_server_scoped_forwarding` must refuse this with a bounded 409
    // rather than attempting to forward with no destination.
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

    let state_file_root = temp_fixture_path("server-scoped-one-shot-tunnel-required-state");
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

    // Simulate the tunnel dropping: the persisted server loses its endpoint
    // hint while the in-memory session token (established above) survives.
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-windows"),
            label: "Windows dogfood".to_owned(),
            kind: ServerKind::Manual,
            ssh_target: None,
            endpoint_hint: None,
            remote_endpoint_hint: None,
        }])
        .await
        .expect("clear linked server endpoint hint");

    let response = local_app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-windows/root-picker")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("tunnel-required server scoped route request"),
        )
        .await
        .expect("tunnel-required server scoped route response");
    assert_eq!(response.status(), StatusCode::CONFLICT);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("tunnel-required response body");
    let value: serde_json::Value =
        serde_json::from_slice(&body).expect("tunnel-required response JSON");
    assert_eq!(value["error"], "linked server tunnel required");
    assert_eq!(
        value.as_object().expect("tunnel-required object").len(),
        1,
        "tunnel-required refusal must not leak extra fields"
    );

    remote_server.abort();
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn server_scoped_one_shot_route_returns_bounded_bad_gateway_when_endpoint_unreachable() {
    // Once a session is established, the linked endpoint can still become
    // unreachable (closed port, restarted process, etc.). The forwarding
    // path must map the raw `reqwest` connection failure to a bounded 502
    // without leaking the failed endpoint or the underlying transport error.
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

    let state_file_root = temp_fixture_path("server-scoped-one-shot-unreachable-state");
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

    // Reserve a loopback port and immediately drop the listener so the port
    // is closed (connection refused) rather than merely unassigned; this
    // simulates an endpoint that has gone unreachable without depending on
    // external network conditions or DNS behavior.
    let closed_listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind closed-port listener");
    let closed_addr = closed_listener.local_addr().expect("closed listener addr");
    drop(closed_listener);
    let closed_endpoint = format!("http://127.0.0.1:{}", closed_addr.port());

    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-windows"),
            label: "Windows dogfood".to_owned(),
            kind: ServerKind::Manual,
            ssh_target: None,
            endpoint_hint: Some(closed_endpoint.clone()),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("point linked server at closed port");

    let response = local_app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-windows/root-picker")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("unreachable server scoped route request"),
        )
        .await
        .expect("unreachable server scoped route response");
    assert_eq!(response.status(), StatusCode::BAD_GATEWAY);
    let body = axum::body::to_bytes(response.into_body(), 4096)
        .await
        .expect("unreachable response body");
    let value: serde_json::Value =
        serde_json::from_slice(&body).expect("unreachable response JSON");
    assert_eq!(value["error"], "linked server unreachable");
    assert_eq!(
        value.as_object().expect("unreachable object").len(),
        1,
        "unreachable refusal must not leak extra fields"
    );
    assert!(
        !body
            .windows(closed_addr.port().to_string().len())
            .any(|window| window == closed_addr.port().to_string().as_bytes()),
        "unreachable refusal must not leak the failed endpoint port"
    );
    assert!(
        !body.windows(3).any(|window| window == b"127"),
        "unreachable refusal must not leak the failed endpoint address"
    );

    remote_server.abort();
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn linked_server_work_root_files_and_document_forwarding_preserves_bearer_and_sse() {
    let remote_root = temp_fixture_path("server-scoped-file-remote-root");
    fs::create_dir_all(remote_root.join("docs")).expect("create remote docs root");
    fs::write(remote_root.join("docs/readme.md"), "# Remote\n").expect("write remote doc");
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

    let state_file_root = temp_fixture_path("server-scoped-file-forwarding-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-windows"),
            label: "Windows dogfood".to_owned(),
            kind: ServerKind::Manual,
            ssh_target: Some("owner@example.test".to_owned()),
            endpoint_hint: Some(format!("http://{remote_addr}")),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("persist linked file server seed");
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
    let work_root_id = open
        .headers()
        .get("x-ws-dashboard-opened-work-root-id")
        .expect("forwarded opened id header")
        .to_str()
        .expect("forwarded opened id string")
        .to_owned();

    let list = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-windows/work-roots/{work_root_id}/files?path=docs"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("server scoped remote file list request"),
        )
        .await
        .expect("server scoped remote file list response");
    assert_eq!(list.status(), StatusCode::OK);
    let list_body = axum::body::to_bytes(list.into_body(), 64 * 1024)
        .await
        .expect("remote list body");
    let list_json: serde_json::Value =
        serde_json::from_slice(&list_body).expect("remote list JSON");
    assert_eq!(list_json["workRootId"], work_root_id);
    assert_eq!(list_json["path"], "docs");
    assert_eq!(list_json["entries"][0]["name"], "readme.md");

    let read = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-windows/work-roots/{work_root_id}/files/read?path=docs/readme.md"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("server scoped remote file read request"),
        )
        .await
        .expect("server scoped remote file read response");
    assert_eq!(read.status(), StatusCode::OK);
    let read_body = axum::body::to_bytes(read.into_body(), 64 * 1024)
        .await
        .expect("remote read body");
    let read_json: serde_json::Value =
        serde_json::from_slice(&read_body).expect("remote read JSON");
    assert_eq!(read_json["content"], "# Remote\n");

    let events = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-windows/work-roots/{work_root_id}/documents/events"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("server scoped remote document events request"),
        )
        .await
        .expect("server scoped remote document events response");
    assert_eq!(events.status(), StatusCode::OK);
    assert_eq!(
        events
            .headers()
            .get(header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok())
            .map(|value| value.starts_with("text/event-stream")),
        Some(true)
    );
    let mut stream = events.into_body().into_data_stream();

    let write = request_json_for_test(
        local_app.clone(),
        Method::POST,
        format!("/api/dashboard/servers/server-windows/work-roots/{work_root_id}/files/write"),
        &cookie,
        serde_json::json!({
            "path": "docs/readme.md",
            "baseContentHash": read_json["contentHash"],
            "content": "# Remote updated\n"
        }),
    )
    .await;
    assert_eq!(write.0, StatusCode::OK);
    assert_eq!(write.2["sizeBytes"], 17);

    let mut buffer = String::new();
    let mut seen = Vec::<serde_json::Value>::new();
    timeout(Duration::from_secs(5), async {
        while !seen.iter().any(|event| {
            event["type"] == "document.contentChanged"
                && event["source"]["workRootId"] == work_root_id
                && event["source"]["path"] == "docs/readme.md"
                && event["contentHash"] == write.2["contentHash"]
        }) {
            let chunk = stream
                .next()
                .await
                .expect("forwarded document SSE chunk")
                .expect("forwarded document SSE body chunk");
            buffer.push_str(std::str::from_utf8(&chunk).expect("forwarded SSE UTF-8"));
            drain_document_sse_events(&mut buffer, &mut seen);
        }
    })
    .await
    .expect("forwarded document invalidation event");

    let reread = local_app
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-windows/work-roots/{work_root_id}/files/read?path=docs/readme.md"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("server scoped remote reread request"),
        )
        .await
        .expect("server scoped remote reread response");
    let reread_body = axum::body::to_bytes(reread.into_body(), 64 * 1024)
        .await
        .expect("remote reread body");
    let reread_json: serde_json::Value =
        serde_json::from_slice(&reread_body).expect("remote reread JSON");
    assert_eq!(reread_json["content"], "# Remote updated\n");

    remote_server.abort();
    remove_static_fixture(&remote_root);
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn linked_server_file_forwarding_preserves_upstream_errors_and_rejects_invalid_sse() {
    async fn link_auth() -> axum::response::Response {
        (
            StatusCode::OK,
            axum::Json(serde_json::json!({ "bearerToken": "test-token" })),
        )
            .into_response()
    }
    async fn file_list_error() -> axum::response::Response {
        (
            StatusCode::IM_A_TEAPOT,
            [(header::CONTENT_TYPE, "application/problem+json")],
            Body::from(r#"{"error":"list failed"}"#),
        )
            .into_response()
    }
    async fn file_read_error() -> axum::response::Response {
        (
            StatusCode::BAD_GATEWAY,
            [(header::CONTENT_TYPE, "application/json")],
            Body::from(r#"{"error":"read upstream failed"}"#),
        )
            .into_response()
    }
    async fn file_write_conflict() -> axum::response::Response {
        (
            StatusCode::CONFLICT,
            [(header::CONTENT_TYPE, "application/json")],
            Body::from(r#"{"error":"content hash mismatch"}"#),
        )
            .into_response()
    }
    async fn document_events_error() -> axum::response::Response {
        (
            StatusCode::SERVICE_UNAVAILABLE,
            [(header::CONTENT_TYPE, "application/json")],
            Body::from(r#"{"error":"events unavailable"}"#),
        )
            .into_response()
    }
    async fn document_events_invalid_content_type() -> axum::response::Response {
        (
            StatusCode::OK,
            [(header::CONTENT_TYPE, "application/json")],
            Body::from(r#"{"not":"sse"}"#),
        )
            .into_response()
    }

    let remote_app = axum::Router::new()
        .route("/api/dashboard/link-auth", axum::routing::post(link_auth))
        .route(
            "/api/dashboard/work-roots/root-error/files",
            axum::routing::get(file_list_error),
        )
        .route(
            "/api/dashboard/work-roots/root-error/files/read",
            axum::routing::get(file_read_error),
        )
        .route(
            "/api/dashboard/work-roots/root-error/files/write",
            axum::routing::post(file_write_conflict),
        )
        .route(
            "/api/dashboard/work-roots/root-events-error/documents/events",
            axum::routing::get(document_events_error),
        )
        .route(
            "/api/dashboard/work-roots/root-events-invalid/documents/events",
            axum::routing::get(document_events_invalid_content_type),
        )
        .route(
            "/api/dashboard/work-roots/root-activity-error/activity/events",
            axum::routing::get(document_events_error),
        )
        .route(
            "/api/dashboard/work-roots/root-activity-invalid/activity/events",
            axum::routing::get(document_events_invalid_content_type),
        );
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("server-scoped-file-upstream-errors-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-errors"),
            label: "Error fixture".to_owned(),
            kind: ServerKind::Manual,
            ssh_target: None,
            endpoint_hint: Some(format!("http://{remote_addr}")),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("persist upstream error fixture server");
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
                .uri("/api/dashboard/servers/server-errors/link-auth")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "passphrase": "ok" }).to_string(),
                ))
                .expect("link custom error fixture server"),
        )
        .await
        .expect("link custom error fixture response");
    assert_eq!(linked.status(), StatusCode::OK);

    for (method, uri, expected_status, expected_content_type, expected_error) in [
        (
            Method::GET,
            "/api/dashboard/servers/server-errors/work-roots/root-error/files?path=missing",
            StatusCode::IM_A_TEAPOT,
            "application/problem+json",
            "list failed",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-errors/work-roots/root-error/files/read?path=missing.txt",
            StatusCode::BAD_GATEWAY,
            "application/json",
            "read upstream failed",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-errors/work-roots/root-error/files/write",
            StatusCode::CONFLICT,
            "application/json",
            "content hash mismatch",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-errors/work-roots/root-events-error/documents/events",
            StatusCode::SERVICE_UNAVAILABLE,
            "application/json",
            "events unavailable",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-errors/work-roots/root-activity-error/activity/events",
            StatusCode::SERVICE_UNAVAILABLE,
            "application/json",
            "events unavailable",
        ),
    ] {
        let response = local_app
            .clone()
            .oneshot(
                Request::builder()
                    .method(method)
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::from(
                        serde_json::json!({
                            "path": "missing.txt",
                            "baseContentHash": "sha256:stale",
                            "content": "new"
                        })
                        .to_string(),
                    ))
                    .expect("upstream error preservation request"),
            )
            .await
            .expect("upstream error preservation response");
        assert_eq!(response.status(), expected_status, "{uri}");
        assert_eq!(
            response
                .headers()
                .get(header::CONTENT_TYPE)
                .and_then(|value| value.to_str().ok())
                .map(|value| value.starts_with(expected_content_type)),
            Some(true),
            "{uri} content-type"
        );
        let body = axum::body::to_bytes(response.into_body(), 4096)
            .await
            .expect("upstream error body");
        let value: serde_json::Value = serde_json::from_slice(&body).expect("upstream error JSON");
        assert_eq!(value["error"], expected_error, "{uri}");
    }

    let invalid_sse = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-errors/work-roots/root-events-invalid/documents/events")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("invalid upstream SSE content-type request"),
        )
        .await
        .expect("invalid upstream SSE content-type response");
    assert_eq!(invalid_sse.status(), StatusCode::BAD_GATEWAY);
    let invalid_body = axum::body::to_bytes(invalid_sse.into_body(), 4096)
        .await
        .expect("invalid SSE response body");
    let invalid_value: serde_json::Value =
        serde_json::from_slice(&invalid_body).expect("invalid SSE response JSON");
    assert_eq!(
        invalid_value["error"],
        "linked server document events stream unavailable"
    );

    let invalid_activity_sse = local_app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-errors/work-roots/root-activity-invalid/activity/events")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("invalid upstream activity SSE content-type request"),
        )
        .await
        .expect("invalid upstream activity SSE content-type response");
    assert_eq!(invalid_activity_sse.status(), StatusCode::BAD_GATEWAY);
    let invalid_activity_body = axum::body::to_bytes(invalid_activity_sse.into_body(), 4096)
        .await
        .expect("invalid activity SSE response body");
    let invalid_activity_value: serde_json::Value =
        serde_json::from_slice(&invalid_activity_body).expect("invalid activity SSE response JSON");
    assert_eq!(
        invalid_activity_value["error"],
        "linked server activity events stream unavailable"
    );

    remote_server.abort();
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn server_scoped_activity_git_workspace_routes_are_owner_authenticated() {
    let app = build_router(app_state());
    let cases: [(Method, &str); 14] = [
        (
            Method::GET,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/activity",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/activity/items/agent-x/transcript",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/activity/events",
        ),
        (
            Method::DELETE,
            "/api/dashboard/servers/server-local/workspaces/workspace-x",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-local/workspaces/workspace-x/git-worktree-add/options",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/workspaces/workspace-x/git-worktree-add/preview",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/workspaces/workspace-x/git-worktree-add",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/git/status",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/git/branches",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/git/branches",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/git/switch-branch",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/git/fetch",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/git/push",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/git/pull-ff-only",
        ),
    ];
    for (method, uri) in cases {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method(method.clone())
                    .uri(uri)
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::from("{}"))
                    .expect("unauthenticated server-scoped request"),
            )
            .await
            .expect("unauthenticated server-scoped response");
        assert_eq!(response.status(), StatusCode::UNAUTHORIZED, "{uri}");
    }
}

// CONTRACT: the `server-local` alias must be byte-for-byte equivalent to the
// legacy bare route for the plain activity feed and transcript routes, for a
// mixed-source feed (a named-agent-compat row plus a live Codex app-server
// session merged in via `merge_activity_items`), matching the same
// equivalence pattern already proven for terminal
// (`server_scoped_terminal_local_aliases_match_legacy_lifecycle`) and
// git/worktree (`server_scoped_git_and_worktree_local_aliases_match_legacy_routes`).
#[tokio::test]
async fn server_scoped_activity_local_aliases_match_legacy_routes() {
    if skip_without_git("server_scoped_activity_local_aliases_match_legacy_routes") {
        return;
    }
    let root = temp_fixture_path("server-scoped-activity-alias-parity");
    let cache_home = temp_fixture_path("server-scoped-activity-alias-parity-cache");
    fs::create_dir_all(&root).expect("create activity workRoot");
    init_git_repo(&root);
    let agents_dir = resolve_work_root_agents_dir(&cache_home, &root)
        .expect("resolve wsstate agents dir for git workRoot");

    // Named-agent-compat row (legacy `namedAgent` source kind), with a
    // transcript available so the transcript route equivalence has content
    // to compare too.
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
        "# Review result\nalias parity fixture\n",
    );

    let codex_sessions = CodexProviderRegistry::default();
    let mut state = app_state_with_activity_cache_home(cache_home.clone());
    state.codex_sessions = codex_sessions.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    // Live Codex app-server session row (`agent.codex` source kind, merged
    // into the same unified feed by `work_root_activity`), for a
    // mixed-source feed. Registered under the same `server-local` id and the
    // just-opened `work_root_id` so both the plain and server-scoped routes
    // resolve to it; `codex_sessions` shares the registry handle installed
    // into `state` above, so this insert is visible to the running `app`.
    let mut projector = CodexProjector::new();
    projector.ingest_line(
        r#"{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"alias-parity-item","text":"alias parity codex transcript"}}}"#,
    );
    let connection = spawn_codex_reply_peer(serde_json::json!({ "turn": { "id": "turn-alias-parity" } }));
    codex_sessions
        .insert_session_for_tests(
            "server-local",
            "codex:alias-parity",
            WorkRootId::from(work_root_id.clone()),
            "thread-alias-parity",
            connection,
            projector,
        )
        .expect("seed live codex session for alias-parity fixture");

    // The unscoped feed must already be mixed-source before comparing
    // aliases, otherwise this test would not exercise what it claims to.
    let (feed_status, feed_body) =
        fetch_work_root_activity(app.clone(), cookie.as_str(), &work_root_id).await;
    assert_eq!(feed_status, StatusCode::OK);
    let feed: serde_json::Value = serde_json::from_str(&feed_body).expect("mixed feed JSON");
    let items = feed["items"].as_array().expect("mixed feed items array");
    assert_eq!(items.len(), 2);
    let source_kinds: std::collections::BTreeSet<&str> = items
        .iter()
        .map(|item| item["source"]["kind"].as_str().expect("source kind"))
        .collect();
    assert_eq!(
        source_kinds,
        std::collections::BTreeSet::from(["namedAgent", "agent.codex"]),
        "fixture must produce a mixed-source feed"
    );

    let legacy_activity_uri = format!("/api/dashboard/work-roots/{work_root_id}/activity");
    let alias_activity_uri =
        format!("/api/dashboard/servers/server-local/work-roots/{work_root_id}/activity");
    let legacy_activity =
        get_status_and_body(app.clone(), cookie.as_str(), &legacy_activity_uri).await;
    let alias_activity =
        get_status_and_body(app.clone(), cookie.as_str(), &alias_activity_uri).await;
    assert_eq!(
        alias_activity.0, legacy_activity.0,
        "status mismatch for activity feed"
    );
    assert_eq!(
        normalize_volatile_json(&alias_activity.1),
        normalize_volatile_json(&legacy_activity.1),
        "body mismatch for activity feed"
    );

    // The plain `.../activity/items/{id}/transcript` route only resolves
    // named-agent-style ids (see `activity_source_from_id`); the live Codex
    // session's transcript is served by the dedicated
    // `.../activity/codex-sessions/{id}/transcript` route instead, which is
    // out of scope for this equivalence test (no server-scoped alias for
    // that route needs proving here; it is already exercised by
    // `server_scoped_codex_prompt_short_circuits_local_and_forwards_remote`).
    let legacy_transcript_uri =
        format!("/api/dashboard/work-roots/{work_root_id}/activity/items/agent:reviewer/transcript");
    let alias_transcript_uri = format!(
        "/api/dashboard/servers/server-local/work-roots/{work_root_id}/activity/items/agent:reviewer/transcript"
    );
    let legacy_transcript =
        get_status_and_body(app.clone(), cookie.as_str(), &legacy_transcript_uri).await;
    let alias_transcript =
        get_status_and_body(app.clone(), cookie.as_str(), &alias_transcript_uri).await;
    assert_eq!(
        legacy_transcript.0,
        StatusCode::OK,
        "legacy transcript route must resolve, not just match the alias's status"
    );
    assert_eq!(
        alias_transcript.0, legacy_transcript.0,
        "status mismatch for named-agent transcript"
    );
    assert_eq!(
        normalize_volatile_json(&alias_transcript.1),
        normalize_volatile_json(&legacy_transcript.1),
        "body mismatch for named-agent transcript"
    );

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

#[tokio::test]
async fn server_scoped_terminal_routes_are_owner_authenticated() {
    let app = build_router(app_state());
    let cases: [(Method, &str); 6] = [
        (
            Method::GET,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/terminals",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/work-roots/root-local-x/terminals",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-local/terminals/terminal-x/output",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/terminals/terminal-x/input",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-local/terminals/terminal-x/resize",
        ),
        (
            Method::DELETE,
            "/api/dashboard/servers/server-local/terminals/terminal-x",
        ),
    ];
    for (method, uri) in cases {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method(method.clone())
                    .uri(uri)
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::from("{}"))
                    .expect("unauthenticated server-scoped terminal request"),
            )
            .await
            .expect("unauthenticated server-scoped terminal response");
        assert_eq!(response.status(), StatusCode::UNAUTHORIZED, "{uri}");
    }
}

// CONTRACT: the `server-local` terminal aliases must behave identically to the
// legacy bare terminal routes for the full create -> list -> output -> input ->
// resize -> close lifecycle, and must reject malformed JSON bodies with the same
// status axum's `Json<T>` extractor returns for the legacy route.
#[tokio::test]
async fn server_scoped_terminal_local_aliases_match_legacy_lifecycle() {
    async fn send_raw(
        app: axum::Router,
        method: Method,
        uri: &str,
        cookie: &str,
        content_type: Option<&str>,
        body: &str,
    ) -> StatusCode {
        let mut builder = Request::builder()
            .method(method)
            .uri(uri)
            .header(header::COOKIE, cookie);
        if let Some(content_type) = content_type {
            builder = builder.header(header::CONTENT_TYPE, content_type);
        }
        app.oneshot(
            builder
                .body(Body::from(body.to_owned()))
                .expect("raw terminal request"),
        )
        .await
        .expect("raw terminal response")
        .status()
    }

    let legacy_root = temp_fixture_path("server-scoped-terminal-legacy-root");
    let scoped_root = temp_fixture_path("server-scoped-terminal-scoped-root");
    fs::create_dir_all(&legacy_root).expect("create legacy terminal root");
    fs::create_dir_all(&scoped_root).expect("create scoped terminal root");

    let (legacy_app, legacy_cookie) = paired_test_app().await;
    let (scoped_app, scoped_cookie) = paired_test_app().await;
    let legacy_work_root =
        open_work_root_for_test(legacy_app.clone(), legacy_cookie.as_str(), &legacy_root).await;
    let scoped_work_root =
        open_work_root_for_test(scoped_app.clone(), scoped_cookie.as_str(), &scoped_root).await;

    // create
    let (legacy_create_status, _, legacy_create) = request_json_for_test(
        legacy_app.clone(),
        Method::POST,
        format!("/api/dashboard/work-roots/{legacy_work_root}/terminals"),
        &legacy_cookie,
        serde_json::json!({ "columns": 80, "rows": 24, "title": "Lifecycle" }),
    )
    .await;
    let (scoped_create_status, _, scoped_create) = request_json_for_test(
        scoped_app.clone(),
        Method::POST,
        format!("/api/dashboard/servers/server-local/work-roots/{scoped_work_root}/terminals"),
        &scoped_cookie,
        serde_json::json!({ "columns": 80, "rows": 24, "title": "Lifecycle" }),
    )
    .await;
    assert_eq!(legacy_create_status, StatusCode::OK);
    assert_eq!(scoped_create_status, legacy_create_status);
    assert_eq!(legacy_create["title"], scoped_create["title"]);
    assert_eq!(legacy_create["columns"], scoped_create["columns"]);
    assert_eq!(legacy_create["rows"], scoped_create["rows"]);
    assert_eq!(legacy_create["status"], "running");
    assert_eq!(scoped_create["status"], "running");
    let legacy_terminal = legacy_create["terminalId"]
        .as_str()
        .expect("legacy terminal id")
        .to_owned();
    let scoped_terminal = scoped_create["terminalId"]
        .as_str()
        .expect("scoped terminal id")
        .to_owned();

    // list
    let legacy_list = legacy_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{legacy_work_root}/terminals"
                ))
                .header(header::COOKIE, legacy_cookie.as_str())
                .body(Body::empty())
                .expect("legacy terminal list request"),
        )
        .await
        .expect("legacy terminal list response");
    let scoped_list = scoped_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-local/work-roots/{scoped_work_root}/terminals"
                ))
                .header(header::COOKIE, scoped_cookie.as_str())
                .body(Body::empty())
                .expect("scoped terminal list request"),
        )
        .await
        .expect("scoped terminal list response");
    assert_eq!(legacy_list.status(), StatusCode::OK);
    assert_eq!(scoped_list.status(), StatusCode::OK);
    let legacy_list_body = axum::body::to_bytes(legacy_list.into_body(), 64 * 1024)
        .await
        .expect("legacy list body");
    let scoped_list_body = axum::body::to_bytes(scoped_list.into_body(), 64 * 1024)
        .await
        .expect("scoped list body");
    let legacy_list_json: serde_json::Value =
        serde_json::from_slice(&legacy_list_body).expect("legacy list JSON");
    let scoped_list_json: serde_json::Value =
        serde_json::from_slice(&scoped_list_body).expect("scoped list JSON");
    assert!(legacy_list_json
        .as_array()
        .expect("legacy list array")
        .iter()
        .any(|session| session["terminalId"] == legacy_terminal.as_str()));
    assert!(scoped_list_json
        .as_array()
        .expect("scoped list array")
        .iter()
        .any(|session| session["terminalId"] == scoped_terminal.as_str()));

    // output
    let legacy_output = legacy_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/terminals/{legacy_terminal}/output?after=0"
                ))
                .header(header::COOKIE, legacy_cookie.as_str())
                .body(Body::empty())
                .expect("legacy terminal output request"),
        )
        .await
        .expect("legacy terminal output response");
    let scoped_output = scoped_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-local/terminals/{scoped_terminal}/output?after=0"
                ))
                .header(header::COOKIE, scoped_cookie.as_str())
                .body(Body::empty())
                .expect("scoped terminal output request"),
        )
        .await
        .expect("scoped terminal output response");
    assert_eq!(legacy_output.status(), StatusCode::OK);
    assert_eq!(scoped_output.status(), StatusCode::OK);
    let scoped_output_body = axum::body::to_bytes(scoped_output.into_body(), 64 * 1024)
        .await
        .expect("scoped output body");
    let scoped_output_json: serde_json::Value =
        serde_json::from_slice(&scoped_output_body).expect("scoped output JSON");
    assert!(scoped_output_json["chunks"].is_array());
    assert!(scoped_output_json["nextSequence"].is_number());

    // input
    let legacy_input = legacy_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!("/api/dashboard/terminals/{legacy_terminal}/input"))
                .header(header::COOKIE, legacy_cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "data": "echo hi\n" }).to_string(),
                ))
                .expect("legacy terminal input request"),
        )
        .await
        .expect("legacy terminal input response");
    let scoped_input = scoped_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/servers/server-local/terminals/{scoped_terminal}/input"
                ))
                .header(header::COOKIE, scoped_cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "data": "echo hi\n" }).to_string(),
                ))
                .expect("scoped terminal input request"),
        )
        .await
        .expect("scoped terminal input response");
    assert_eq!(legacy_input.status(), StatusCode::NO_CONTENT);
    assert_eq!(scoped_input.status(), StatusCode::NO_CONTENT);

    // resize
    let (legacy_resize_status, _, legacy_resize) = request_json_for_test(
        legacy_app.clone(),
        Method::POST,
        format!("/api/dashboard/terminals/{legacy_terminal}/resize"),
        &legacy_cookie,
        serde_json::json!({ "columns": 100, "rows": 40 }),
    )
    .await;
    let (scoped_resize_status, _, scoped_resize) = request_json_for_test(
        scoped_app.clone(),
        Method::POST,
        format!("/api/dashboard/servers/server-local/terminals/{scoped_terminal}/resize"),
        &scoped_cookie,
        serde_json::json!({ "columns": 100, "rows": 40 }),
    )
    .await;
    assert_eq!(legacy_resize_status, StatusCode::OK);
    assert_eq!(scoped_resize_status, StatusCode::OK);
    assert_eq!(legacy_resize["columns"], 100);
    assert_eq!(scoped_resize["columns"], 100);
    assert_eq!(legacy_resize["rows"], 40);
    assert_eq!(scoped_resize["rows"], 40);

    // close
    let legacy_close = legacy_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!("/api/dashboard/terminals/{legacy_terminal}"))
                .header(header::COOKIE, legacy_cookie.as_str())
                .body(Body::empty())
                .expect("legacy terminal close request"),
        )
        .await
        .expect("legacy terminal close response");
    let scoped_close = scoped_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!(
                    "/api/dashboard/servers/server-local/terminals/{scoped_terminal}"
                ))
                .header(header::COOKIE, scoped_cookie.as_str())
                .body(Body::empty())
                .expect("scoped terminal close request"),
        )
        .await
        .expect("scoped terminal close response");
    assert_eq!(legacy_close.status(), StatusCode::NO_CONTENT);
    assert_eq!(scoped_close.status(), StatusCode::NO_CONTENT);

    // closed terminals disappear identically
    let scoped_after_close = scoped_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-local/terminals/{scoped_terminal}/output?after=0"
                ))
                .header(header::COOKIE, scoped_cookie.as_str())
                .body(Body::empty())
                .expect("scoped closed terminal output request"),
        )
        .await
        .expect("scoped closed terminal output response");
    assert_eq!(scoped_after_close.status(), StatusCode::NOT_FOUND);

    // JSON-body boundary parity: create/input/resize aliases must classify a
    // missing content type (415), a data error (422), and a syntax error (400)
    // the same way the legacy `Json<T>` extractor does. Assert equivalence
    // against the legacy route rather than hardcoding the status.
    let boundary_cases: [(&str, &str, &str); 3] = [
        (
            "/api/dashboard/work-roots/root-x/terminals",
            "/api/dashboard/servers/server-local/work-roots/root-x/terminals",
            r#"{"columns":"x"}"#,
        ),
        (
            "/api/dashboard/terminals/terminal-x/input",
            "/api/dashboard/servers/server-local/terminals/terminal-x/input",
            r#"{"data":5}"#,
        ),
        (
            "/api/dashboard/terminals/terminal-x/resize",
            "/api/dashboard/servers/server-local/terminals/terminal-x/resize",
            r#"{"columns":"wide"}"#,
        ),
    ];
    for (legacy_uri, scoped_uri, data_error_body) in boundary_cases {
        // missing content type -> 415
        let legacy_missing = send_raw(
            legacy_app.clone(),
            Method::POST,
            legacy_uri,
            &legacy_cookie,
            None,
            "{}",
        )
        .await;
        let scoped_missing = send_raw(
            scoped_app.clone(),
            Method::POST,
            scoped_uri,
            &scoped_cookie,
            None,
            "{}",
        )
        .await;
        assert_eq!(scoped_missing, legacy_missing, "missing content type {scoped_uri}");
        assert_eq!(scoped_missing, StatusCode::UNSUPPORTED_MEDIA_TYPE, "{scoped_uri}");

        // data error (valid JSON, wrong field type) -> 422
        let legacy_data = send_raw(
            legacy_app.clone(),
            Method::POST,
            legacy_uri,
            &legacy_cookie,
            Some("application/json"),
            data_error_body,
        )
        .await;
        let scoped_data = send_raw(
            scoped_app.clone(),
            Method::POST,
            scoped_uri,
            &scoped_cookie,
            Some("application/json"),
            data_error_body,
        )
        .await;
        assert_eq!(scoped_data, legacy_data, "data error {scoped_uri}");
        assert_eq!(scoped_data, StatusCode::UNPROCESSABLE_ENTITY, "{scoped_uri}");

        // syntax error -> 400
        let legacy_syntax = send_raw(
            legacy_app.clone(),
            Method::POST,
            legacy_uri,
            &legacy_cookie,
            Some("application/json"),
            "{",
        )
        .await;
        let scoped_syntax = send_raw(
            scoped_app.clone(),
            Method::POST,
            scoped_uri,
            &scoped_cookie,
            Some("application/json"),
            "{",
        )
        .await;
        assert_eq!(scoped_syntax, legacy_syntax, "syntax error {scoped_uri}");
        assert_eq!(scoped_syntax, StatusCode::BAD_REQUEST, "{scoped_uri}");
    }

    remove_static_fixture(&legacy_root);
    remove_static_fixture(&scoped_root);
}

// Forward the full terminal lifecycle to a real linked daemon and confirm the
// close propagates upstream (the remote terminal is gone afterward).
#[tokio::test]
async fn linked_server_terminal_forwarding_preserves_lifecycle() {
    let remote_root = temp_fixture_path("server-scoped-terminal-remote-root");
    fs::create_dir_all(&remote_root).expect("create remote terminal root");
    let remote_state =
        app_state_with_opened_and_store(OpenedWorkRoots::default(), DashboardStateStore::disabled());
    let passphrase = remote_state
        .auth
        .link_passphrase()
        .expose_for_owner_record()
        .to_owned();
    let remote_app = build_router(remote_state);
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("server-scoped-terminal-forwarding-state");
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
        .expect("persist linked terminal server seed");
    let local_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = local_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let local_app = build_router(local_state);
    let cookie = pair_and_cookie(local_app.clone(), &token).await;

    let (link_status, ..) = request_json_for_test(
        local_app.clone(),
        Method::POST,
        "/api/dashboard/servers/server-windows/link-auth".to_owned(),
        &cookie,
        serde_json::json!({ "passphrase": passphrase }),
    )
    .await;
    assert_eq!(link_status, StatusCode::OK);

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
                .expect("remote open request"),
        )
        .await
        .expect("remote open response");
    assert_eq!(open.status(), StatusCode::OK);
    let work_root_id = open
        .headers()
        .get("x-ws-dashboard-opened-work-root-id")
        .expect("forwarded opened id header")
        .to_str()
        .expect("forwarded opened id string")
        .to_owned();

    // create
    let (create_status, _, create) = request_json_for_test(
        local_app.clone(),
        Method::POST,
        format!("/api/dashboard/servers/server-windows/work-roots/{work_root_id}/terminals"),
        &cookie,
        serde_json::json!({ "columns": 80, "rows": 24, "title": "Remote" }),
    )
    .await;
    assert_eq!(create_status, StatusCode::OK);
    assert_eq!(create["status"], "running");
    let terminal_id = create["terminalId"]
        .as_str()
        .expect("remote terminal id")
        .to_owned();

    // list
    let list = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-windows/work-roots/{work_root_id}/terminals"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("remote terminal list request"),
        )
        .await
        .expect("remote terminal list response");
    assert_eq!(list.status(), StatusCode::OK);
    let list_body = axum::body::to_bytes(list.into_body(), 64 * 1024)
        .await
        .expect("remote list body");
    let list_json: serde_json::Value = serde_json::from_slice(&list_body).expect("remote list JSON");
    assert!(list_json
        .as_array()
        .expect("remote list array")
        .iter()
        .any(|session| session["terminalId"] == terminal_id.as_str()));

    // output
    let output = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-windows/terminals/{terminal_id}/output?after=0"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("remote terminal output request"),
        )
        .await
        .expect("remote terminal output response");
    assert_eq!(output.status(), StatusCode::OK);

    // input
    let input = local_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/servers/server-windows/terminals/{terminal_id}/input"
                ))
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "data": "echo remote\n" }).to_string(),
                ))
                .expect("remote terminal input request"),
        )
        .await
        .expect("remote terminal input response");
    assert_eq!(input.status(), StatusCode::NO_CONTENT);

    // resize
    let (resize_status, _, resize) = request_json_for_test(
        local_app.clone(),
        Method::POST,
        format!("/api/dashboard/servers/server-windows/terminals/{terminal_id}/resize"),
        &cookie,
        serde_json::json!({ "columns": 120, "rows": 48 }),
    )
    .await;
    assert_eq!(resize_status, StatusCode::OK);
    assert_eq!(resize["columns"], 120);
    assert_eq!(resize["rows"], 48);

    // close
    let close = local_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!(
                    "/api/dashboard/servers/server-windows/terminals/{terminal_id}"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("remote terminal close request"),
        )
        .await
        .expect("remote terminal close response");
    assert_eq!(close.status(), StatusCode::NO_CONTENT);

    // closing the remote terminal closed it upstream: output now 404s
    let after_close = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-windows/terminals/{terminal_id}/output?after=0"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("remote closed terminal output request"),
        )
        .await
        .expect("remote closed terminal output response");
    assert_eq!(after_close.status(), StatusCode::NOT_FOUND);

    remote_server.abort();
    remove_static_fixture(&remote_root);
    remove_static_fixture(&state_file_root);
}

// End-to-end relay against two real daemons: a real terminal on the remote
// daemon, connected through the local gateway's server-scoped socket route,
// with typed input relayed upstream and terminal output relayed back to the
// browser. Also proves browser-initiated close tears down the relay without
// hanging.
#[tokio::test]
async fn linked_server_terminal_websocket_relays_real_two_daemon_io() {
    let remote_root = temp_fixture_path("server-scoped-ws-relay-remote-root");
    fs::create_dir_all(&remote_root).expect("create remote ws relay root");
    let remote_state =
        app_state_with_opened_and_store(OpenedWorkRoots::default(), DashboardStateStore::disabled());
    let passphrase = remote_state
        .auth
        .link_passphrase()
        .expose_for_owner_record()
        .to_owned();
    let remote_app = build_router(remote_state);
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("server-scoped-ws-relay-state");
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
        .expect("persist linked ws relay server seed");
    let local_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = local_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let local_app = build_router(local_state);
    let cookie = pair_and_cookie(local_app.clone(), &token).await;

    let (link_status, ..) = request_json_for_test(
        local_app.clone(),
        Method::POST,
        "/api/dashboard/servers/server-windows/link-auth".to_owned(),
        &cookie,
        serde_json::json!({ "passphrase": passphrase }),
    )
    .await;
    assert_eq!(link_status, StatusCode::OK);

    let (open_status, open_headers, _) = request_json_for_test(
        local_app.clone(),
        Method::POST,
        "/api/dashboard/servers/server-windows/work-roots/open".to_owned(),
        &cookie,
        serde_json::json!({ "path": remote_root.display().to_string() }),
    )
    .await;
    assert_eq!(open_status, StatusCode::OK);
    let work_root_id = open_headers
        .get("x-ws-dashboard-opened-work-root-id")
        .expect("forwarded opened id header")
        .to_str()
        .expect("forwarded opened id string")
        .to_owned();

    let (create_status, _, create) = request_json_for_test(
        local_app.clone(),
        Method::POST,
        format!("/api/dashboard/servers/server-windows/work-roots/{work_root_id}/terminals"),
        &cookie,
        serde_json::json!({ "columns": 80, "rows": 24, "title": "Remote" }),
    )
    .await;
    assert_eq!(create_status, StatusCode::OK);
    let terminal_id = create["terminalId"]
        .as_str()
        .expect("remote terminal id")
        .to_owned();

    let (local_addr, local_server) = spawn_test_server(local_app.clone()).await;
    let mut request = format!(
        "ws://{local_addr}/api/dashboard/servers/server-windows/terminals/{terminal_id}/socket"
    )
    .into_client_request()
    .expect("linked terminal websocket relay request");
    request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let (mut socket, response) = tokio_tungstenite::connect_async(request)
        .await
        .expect("connect linked terminal websocket relay");
    assert_eq!(response.status(), StatusCode::SWITCHING_PROTOCOLS);

    socket
        .send(TungsteniteMessage::Text(
            serde_json::json!({ "type": "input", "data": terminal_test_commands_for_current_platform("RELAY-WS-MARKER").echo_and_exit })
                .to_string()
                .into(),
        ))
        .await
        .expect("send relayed websocket input");

    let mut text = String::new();
    for _ in 0..80 {
        let Some(message) = timeout(Duration::from_secs(2), socket.next())
            .await
            .expect("relayed websocket message timeout")
        else {
            break;
        };
        let message = message.expect("relayed websocket message");
        let TungsteniteMessage::Text(payload) = message else {
            continue;
        };
        let value: serde_json::Value =
            serde_json::from_str(&payload).expect("relayed websocket frame JSON");
        assert_eq!(value["terminalId"], terminal_id);
        match value["type"].as_str() {
            Some("output") => text.push_str(value["chunk"]["data"].as_str().expect("output data")),
            Some("exit") => break,
            Some("status") => {}
            other => panic!("unexpected relayed websocket frame type: {other:?}"),
        }
        if text.contains("RELAY-WS-MARKER") {
            break;
        }
    }
    assert!(text.contains("RELAY-WS-MARKER"), "{text:?}");

    // Browser-initiated close must tear the relay down; the timeout guard above
    // and clean shutdown below prove the relay loop does not hang.
    socket.close(None).await.expect("close relayed websocket");

    local_server.abort();
    remote_server.abort();
    remove_static_fixture(&remote_root);
    remove_static_fixture(&state_file_root);
}

// Every linked-server refusal must resolve to a bounded HTTP error response
// BEFORE any WebSocket upgrade completes (never a 101, never a half-open
// socket): dot-free rejection (400), unknown server (404), auth required
// (409), and unreachable-on-connect (502).
#[tokio::test]
async fn linked_server_terminal_websocket_refuses_before_upgrade() {
    let remote_state =
        app_state_with_opened_and_store(OpenedWorkRoots::default(), DashboardStateStore::disabled());
    let passphrase = remote_state
        .auth
        .link_passphrase()
        .expose_for_owner_record()
        .to_owned();
    let remote_app = build_router(remote_state);
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("server-scoped-ws-refusal-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![
            PersistedLinkedServer {
                id: ServerId::from("server-authless"),
                label: "Authless".to_owned(),
                kind: ServerKind::Manual,
                ssh_target: None,
                endpoint_hint: Some("http://127.0.0.1:1".to_owned()),
                remote_endpoint_hint: None,
            },
            PersistedLinkedServer {
                id: ServerId::from("server-live"),
                label: "Live".to_owned(),
                kind: ServerKind::Manual,
                ssh_target: None,
                endpoint_hint: Some(format!("http://{remote_addr}")),
                remote_endpoint_hint: None,
            },
        ])
        .await
        .expect("persist linked ws refusal seeds");
    let local_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = local_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let local_app = build_router(local_state);
    let cookie = pair_and_cookie(local_app.clone(), &token).await;

    // Link "server-live" so it holds a bearer token, then drop its upstream so
    // a subsequent socket connect is genuinely unreachable (502).
    let (link_status, ..) = request_json_for_test(
        local_app.clone(),
        Method::POST,
        "/api/dashboard/servers/server-live/link-auth".to_owned(),
        &cookie,
        serde_json::json!({ "passphrase": passphrase }),
    )
    .await;
    assert_eq!(link_status, StatusCode::OK);

    let (local_addr, local_server) = spawn_test_server(local_app.clone()).await;

    async fn expect_refusal(local_addr: &str, cookie: &str, route: &str, expected: StatusCode) {
        let mut request = format!(
            "ws://{local_addr}/api/dashboard/servers/{route}/terminals/term-x/socket"
        )
        .into_client_request()
        .expect("refusal websocket request");
        request
            .headers_mut()
            .insert(header::COOKIE, cookie.parse().expect("cookie header"));
        let error = tokio_tungstenite::connect_async(request)
            .await
            .expect_err("refused websocket must not upgrade");
        match error {
            tokio_tungstenite::tungstenite::Error::Http(response) => {
                assert_eq!(response.status(), expected, "route {route}");
                assert_ne!(
                    response.status(),
                    StatusCode::SWITCHING_PROTOCOLS,
                    "route {route} must not complete the upgrade"
                );
            }
            other => panic!("unexpected refusal error for {route}: {other}"),
        }
    }

    // dot-free rejection (dotted route can never resolve to a forwardable server)
    expect_refusal(&local_addr, &cookie, "server.dotted", StatusCode::BAD_REQUEST).await;
    // unknown server
    expect_refusal(&local_addr, &cookie, "server-missing", StatusCode::NOT_FOUND).await;
    // auth required (linked server with no session token)
    expect_refusal(&local_addr, &cookie, "server-authless", StatusCode::CONFLICT).await;

    // unreachable-on-connect: server-live has a token but its upstream is gone.
    remote_server.abort();
    expect_refusal(&local_addr, &cookie, "server-live", StatusCode::BAD_GATEWAY).await;

    local_server.abort();
    remove_static_fixture(&state_file_root);
}

#[derive(Clone, Default)]
struct TerminalWebSocketRelayProbe {
    upgrades: Arc<Mutex<Vec<(String, Option<String>)>>>,
    frames: Arc<Mutex<Vec<String>>>,
    closed: Arc<Notify>,
}

async fn mock_relay_link_auth() -> impl IntoResponse {
    (
        StatusCode::OK,
        axum::Json(serde_json::json!({ "bearerToken": "relay-terminal-token" })),
    )
}

async fn mock_relay_terminal_socket(
    axum::extract::State(probe): axum::extract::State<TerminalWebSocketRelayProbe>,
    axum::extract::Path(terminal_id): axum::extract::Path<String>,
    uri: axum::extract::OriginalUri,
    headers: axum::http::HeaderMap,
    upgrade: axum::extract::ws::WebSocketUpgrade,
) -> axum::response::Response {
    probe.upgrades.lock().await.push((
        uri.to_string(),
        headers
            .get(header::AUTHORIZATION)
            .and_then(|value| value.to_str().ok())
            .map(str::to_owned),
    ));
    if terminal_id == "term-reject" {
        return StatusCode::GONE.into_response();
    }
    upgrade
        .on_upgrade(move |socket| async move {
            let (mut sender, mut receiver) = socket.split();
            if terminal_id == "term-upstream-close" {
                let _ = sender.send(axum::extract::ws::Message::Close(None)).await;
                probe.closed.notify_waiters();
                return;
            }
            // Emit one of each non-text frame type so upstream->browser relay of
            // binary/ping/pong is exercised.
            let _ = sender
                .send(axum::extract::ws::Message::Binary(
                    axum::body::Bytes::from_static(b"upstream-bytes"),
                ))
                .await;
            let _ = sender
                .send(axum::extract::ws::Message::Ping(
                    axum::body::Bytes::from_static(b"upstream-ping"),
                ))
                .await;
            let _ = sender
                .send(axum::extract::ws::Message::Pong(
                    axum::body::Bytes::from_static(b"upstream-pong"),
                ))
                .await;
            while let Some(Ok(message)) = receiver.next().await {
                match message {
                    axum::extract::ws::Message::Text(text) => {
                        probe.frames.lock().await.push(format!("text:{text}"))
                    }
                    axum::extract::ws::Message::Binary(bytes) => probe
                        .frames
                        .lock()
                        .await
                        .push(format!("binary:{}", String::from_utf8_lossy(&bytes))),
                    axum::extract::ws::Message::Ping(bytes) => probe
                        .frames
                        .lock()
                        .await
                        .push(format!("ping:{}", String::from_utf8_lossy(&bytes))),
                    axum::extract::ws::Message::Pong(bytes) => probe
                        .frames
                        .lock()
                        .await
                        .push(format!("pong:{}", String::from_utf8_lossy(&bytes))),
                    axum::extract::ws::Message::Close(_) => break,
                }
            }
            probe.closed.notify_waiters();
        })
        .into_response()
}

// Relay coverage against a mock upstream: bearer + preserved base path on the
// upstream upgrade, bidirectional non-text (binary/ping/pong) frame relay,
// upstream rejection propagated before upgrade (410), and cleanup when either
// side closes (browser-initiated and upstream-initiated).
#[tokio::test]
async fn linked_server_terminal_websocket_relays_non_text_frames_and_cleans_up() {
    let probe = TerminalWebSocketRelayProbe::default();
    let remote_app = axum::Router::new()
        .route(
            "/gateway/api/dashboard/link-auth",
            axum::routing::post(mock_relay_link_auth),
        )
        .route(
            "/gateway/api/dashboard/terminals/{terminal_id}/socket",
            axum::routing::get(mock_relay_terminal_socket),
        )
        .with_state(probe.clone());
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("server-scoped-ws-nontext-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-windows"),
            label: "Windows dogfood".to_owned(),
            kind: ServerKind::Manual,
            ssh_target: None,
            // Base path baked into the endpoint proves remote_url-then-scheme
            // ordering preserves the prefix on the upstream socket URL.
            endpoint_hint: Some(format!("http://{remote_addr}/gateway")),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("persist linked ws non-text seed");
    let local_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = local_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let local_app = build_router(local_state);
    let cookie = pair_and_cookie(local_app.clone(), &token).await;
    let (local_addr, local_server) = spawn_test_server(local_app.clone()).await;

    let (link_status, ..) = request_json_for_test(
        local_app.clone(),
        Method::POST,
        "/api/dashboard/servers/server-windows/link-auth".to_owned(),
        &cookie,
        serde_json::json!({ "passphrase": "mock-passphrase" }),
    )
    .await;
    assert_eq!(link_status, StatusCode::OK);

    let mut request = format!(
        "ws://{local_addr}/api/dashboard/servers/server-windows/terminals/term-live/socket?after=41"
    )
    .into_client_request()
    .expect("linked terminal non-text websocket request");
    request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let (mut socket, response) = tokio_tungstenite::connect_async(request)
        .await
        .expect("connect linked terminal non-text websocket");
    assert_eq!(response.status(), StatusCode::SWITCHING_PROTOCOLS);

    // upstream -> browser non-text frames
    let mut upstream_non_text = Vec::new();
    for _ in 0..3 {
        let frame = timeout(Duration::from_secs(2), socket.next())
            .await
            .expect("upstream non-text timeout")
            .expect("upstream non-text output")
            .expect("upstream non-text frame");
        match frame {
            TungsteniteMessage::Binary(bytes) => {
                upstream_non_text.push(format!("binary:{}", String::from_utf8_lossy(&bytes)))
            }
            TungsteniteMessage::Ping(bytes) => {
                upstream_non_text.push(format!("ping:{}", String::from_utf8_lossy(&bytes)))
            }
            TungsteniteMessage::Pong(bytes) => {
                upstream_non_text.push(format!("pong:{}", String::from_utf8_lossy(&bytes)))
            }
            other => panic!("unexpected upstream non-text frame: {other:?}"),
        }
    }
    assert!(
        upstream_non_text.contains(&"binary:upstream-bytes".to_owned()),
        "{upstream_non_text:?}"
    );
    assert!(
        upstream_non_text.contains(&"ping:upstream-ping".to_owned()),
        "{upstream_non_text:?}"
    );
    assert!(
        upstream_non_text.contains(&"pong:upstream-pong".to_owned()),
        "{upstream_non_text:?}"
    );

    // browser -> upstream non-text frames
    socket
        .send(TungsteniteMessage::Binary(axum::body::Bytes::from_static(
            b"raw-bytes",
        )))
        .await
        .expect("send browser binary");
    socket
        .send(TungsteniteMessage::Ping(axum::body::Bytes::from_static(
            b"browser-ping",
        )))
        .await
        .expect("send browser ping");
    socket
        .send(TungsteniteMessage::Pong(axum::body::Bytes::from_static(
            b"browser-pong",
        )))
        .await
        .expect("send browser pong");
    // browser-initiated close tears down the upstream side.
    socket.close(None).await.expect("close browser websocket");
    timeout(Duration::from_secs(2), probe.closed.notified())
        .await
        .expect("upstream torn down after browser close");

    let upgrades = probe.upgrades.lock().await.clone();
    assert_eq!(upgrades.len(), 1);
    assert_eq!(
        upgrades[0].0,
        "/gateway/api/dashboard/terminals/term-live/socket?after=41"
    );
    assert_eq!(upgrades[0].1.as_deref(), Some("Bearer relay-terminal-token"));
    let frames = probe.frames.lock().await.clone();
    assert!(frames.contains(&"binary:raw-bytes".to_owned()), "{frames:?}");
    assert!(frames.contains(&"ping:browser-ping".to_owned()), "{frames:?}");
    assert!(frames.contains(&"pong:browser-pong".to_owned()), "{frames:?}");

    // Upstream rejection before upgrade -> bounded refusal, no 101.
    let mut reject_request = format!(
        "ws://{local_addr}/api/dashboard/servers/server-windows/terminals/term-reject/socket"
    )
    .into_client_request()
    .expect("linked terminal reject websocket request");
    reject_request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let error = tokio_tungstenite::connect_async(reject_request)
        .await
        .expect_err("linked terminal upstream rejection");
    match error {
        tokio_tungstenite::tungstenite::Error::Http(response) => {
            assert_eq!(response.status(), StatusCode::GONE);
        }
        other => panic!("unexpected upstream rejection error: {other}"),
    }

    // Upstream-initiated close propagates a Close frame to the browser side.
    let mut upstream_close_request = format!(
        "ws://{local_addr}/api/dashboard/servers/server-windows/terminals/term-upstream-close/socket"
    )
    .into_client_request()
    .expect("linked terminal upstream close websocket request");
    upstream_close_request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let (mut upstream_close_socket, close_response) =
        tokio_tungstenite::connect_async(upstream_close_request)
            .await
            .expect("connect linked upstream close websocket");
    assert_eq!(close_response.status(), StatusCode::SWITCHING_PROTOCOLS);
    let close = timeout(Duration::from_secs(2), upstream_close_socket.next())
        .await
        .expect("upstream close propagation timeout")
        .expect("upstream close propagation")
        .expect("upstream close frame");
    assert!(matches!(close, TungsteniteMessage::Close(_)));

    local_server.abort();
    remote_server.abort();
    remove_static_fixture(&state_file_root);
}

// Mock-upstream coverage: bearer + legacy path forwarding on success, and
// upstream status/body/content-type preservation on failure for all six ops.
#[tokio::test]
async fn linked_server_terminal_forwarding_preserves_bearer_and_upstream_errors() {
    async fn link_auth() -> axum::response::Response {
        (
            StatusCode::OK,
            axum::Json(serde_json::json!({ "bearerToken": "test-token" })),
        )
            .into_response()
    }
    async fn terminals_list(headers: axum::http::HeaderMap) -> axum::response::Response {
        // Prove the bearer token was forwarded on the exact legacy path.
        if headers
            .get(header::AUTHORIZATION)
            .and_then(|value| value.to_str().ok())
            != Some("Bearer test-token")
        {
            return (StatusCode::UNAUTHORIZED, "missing bearer").into_response();
        }
        (
            StatusCode::OK,
            [(header::CONTENT_TYPE, "application/json")],
            Body::from(r#"[{"terminalId":"remote-term","workRootId":"root-ok","title":"Remote","status":"running","columns":80,"rows":24,"createdAtMs":1,"cwdHint":null}]"#),
        )
            .into_response()
    }
    async fn create_error() -> axum::response::Response {
        (
            StatusCode::SERVICE_UNAVAILABLE,
            [(header::CONTENT_TYPE, "application/problem+json")],
            Body::from(r#"{"error":"spawn failed"}"#),
        )
            .into_response()
    }
    async fn list_error() -> axum::response::Response {
        (
            StatusCode::SERVICE_UNAVAILABLE,
            [(header::CONTENT_TYPE, "application/json")],
            Body::from(r#"{"error":"workRoot unreachable"}"#),
        )
            .into_response()
    }
    async fn output_error() -> axum::response::Response {
        (
            StatusCode::NOT_FOUND,
            [(header::CONTENT_TYPE, "application/json")],
            Body::from(r#"{"error":"unknown terminal"}"#),
        )
            .into_response()
    }
    async fn input_error() -> axum::response::Response {
        (
            StatusCode::GONE,
            [(header::CONTENT_TYPE, "application/json")],
            Body::from(r#"{"error":"terminal is closed"}"#),
        )
            .into_response()
    }
    async fn resize_error() -> axum::response::Response {
        (
            StatusCode::BAD_REQUEST,
            [(header::CONTENT_TYPE, "application/json")],
            Body::from(r#"{"error":"invalid terminal size"}"#),
        )
            .into_response()
    }
    async fn close_error() -> axum::response::Response {
        (
            StatusCode::CONFLICT,
            [(header::CONTENT_TYPE, "application/json")],
            Body::from(r#"{"error":"workRoot offline"}"#),
        )
            .into_response()
    }

    let remote_app = axum::Router::new()
        .route("/api/dashboard/link-auth", axum::routing::post(link_auth))
        .route(
            "/api/dashboard/work-roots/root-ok/terminals",
            axum::routing::get(terminals_list),
        )
        .route(
            "/api/dashboard/work-roots/root-err/terminals",
            axum::routing::post(create_error),
        )
        .route(
            "/api/dashboard/work-roots/root-list-err/terminals",
            axum::routing::get(list_error),
        )
        .route(
            "/api/dashboard/terminals/term-err/output",
            axum::routing::get(output_error),
        )
        .route(
            "/api/dashboard/terminals/term-err/input",
            axum::routing::post(input_error),
        )
        .route(
            "/api/dashboard/terminals/term-err/resize",
            axum::routing::post(resize_error),
        )
        .route(
            "/api/dashboard/terminals/term-err",
            axum::routing::delete(close_error),
        );
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("server-scoped-terminal-upstream-errors-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-errors"),
            label: "Error fixture".to_owned(),
            kind: ServerKind::Manual,
            ssh_target: None,
            endpoint_hint: Some(format!("http://{remote_addr}")),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("persist terminal error fixture server");
    let local_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = local_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let local_app = build_router(local_state);
    let cookie = pair_and_cookie(local_app.clone(), &token).await;

    let (link_status, ..) = request_json_for_test(
        local_app.clone(),
        Method::POST,
        "/api/dashboard/servers/server-errors/link-auth".to_owned(),
        &cookie,
        serde_json::json!({ "passphrase": "ok" }),
    )
    .await;
    assert_eq!(link_status, StatusCode::OK);

    // success path: bearer + legacy path forwarding
    let list = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/servers/server-errors/work-roots/root-ok/terminals")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("remote list forwarding request"),
        )
        .await
        .expect("remote list forwarding response");
    assert_eq!(list.status(), StatusCode::OK);
    let list_body = axum::body::to_bytes(list.into_body(), 64 * 1024)
        .await
        .expect("remote list forwarding body");
    let list_json: serde_json::Value =
        serde_json::from_slice(&list_body).expect("remote list forwarding JSON");
    assert_eq!(list_json[0]["terminalId"], "remote-term");

    for (method, uri, expected_status, expected_content_type, expected_error) in [
        (
            Method::POST,
            "/api/dashboard/servers/server-errors/work-roots/root-err/terminals",
            StatusCode::SERVICE_UNAVAILABLE,
            "application/problem+json",
            "spawn failed",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-errors/work-roots/root-list-err/terminals",
            StatusCode::SERVICE_UNAVAILABLE,
            "application/json",
            "workRoot unreachable",
        ),
        (
            Method::GET,
            "/api/dashboard/servers/server-errors/terminals/term-err/output",
            StatusCode::NOT_FOUND,
            "application/json",
            "unknown terminal",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-errors/terminals/term-err/input",
            StatusCode::GONE,
            "application/json",
            "terminal is closed",
        ),
        (
            Method::POST,
            "/api/dashboard/servers/server-errors/terminals/term-err/resize",
            StatusCode::BAD_REQUEST,
            "application/json",
            "invalid terminal size",
        ),
        (
            Method::DELETE,
            "/api/dashboard/servers/server-errors/terminals/term-err",
            StatusCode::CONFLICT,
            "application/json",
            "workRoot offline",
        ),
    ] {
        let response = local_app
            .clone()
            .oneshot(
                Request::builder()
                    .method(method)
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::from(
                        serde_json::json!({ "columns": 80, "rows": 24, "data": "x" }).to_string(),
                    ))
                    .expect("terminal upstream error request"),
            )
            .await
            .expect("terminal upstream error response");
        assert_eq!(response.status(), expected_status, "{uri}");
        assert_eq!(
            response
                .headers()
                .get(header::CONTENT_TYPE)
                .and_then(|value| value.to_str().ok())
                .map(|value| value.starts_with(expected_content_type)),
            Some(true),
            "{uri} content-type"
        );
        let body = axum::body::to_bytes(response.into_body(), 4096)
            .await
            .expect("terminal upstream error body");
        let value: serde_json::Value =
            serde_json::from_slice(&body).expect("terminal upstream error JSON");
        assert_eq!(value["error"], expected_error, "{uri}");
    }

    remote_server.abort();
    remove_static_fixture(&state_file_root);
}

// Collision safety: closing a remote terminal whose bare id coincides with a
// live local terminal must not disturb the local terminal.
#[tokio::test]
async fn server_scoped_terminal_close_does_not_disturb_colliding_local_terminal() {
    async fn link_auth() -> axum::response::Response {
        (
            StatusCode::OK,
            axum::Json(serde_json::json!({ "bearerToken": "test-token" })),
        )
            .into_response()
    }
    async fn remote_close() -> axum::response::Response {
        StatusCode::NO_CONTENT.into_response()
    }

    let local_root = temp_fixture_path("server-scoped-terminal-collision-root");
    fs::create_dir_all(&local_root).expect("create collision local root");

    let remote_app = axum::Router::new()
        .route("/api/dashboard/link-auth", axum::routing::post(link_auth))
        // Match any terminal id under the remote so the colliding local id
        // resolves to a remote-side no-op close.
        .route(
            "/api/dashboard/terminals/{terminal_id}",
            axum::routing::delete(remote_close),
        );
    let (remote_addr, remote_server) = spawn_test_server(remote_app).await;

    let state_file_root = temp_fixture_path("server-scoped-terminal-collision-state");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    store
        .persist_linked_servers(vec![PersistedLinkedServer {
            id: ServerId::from("server-remote"),
            label: "Remote".to_owned(),
            kind: ServerKind::Manual,
            ssh_target: None,
            endpoint_hint: Some(format!("http://{remote_addr}")),
            remote_endpoint_hint: None,
        }])
        .await
        .expect("persist collision fixture server");
    let local_state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store);
    let token = local_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let local_app = build_router(local_state);
    let cookie = pair_and_cookie(local_app.clone(), &token).await;

    let (link_status, ..) = request_json_for_test(
        local_app.clone(),
        Method::POST,
        "/api/dashboard/servers/server-remote/link-auth".to_owned(),
        &cookie,
        serde_json::json!({ "passphrase": "ok" }),
    )
    .await;
    assert_eq!(link_status, StatusCode::OK);

    let work_root_id = open_work_root_for_test(local_app.clone(), cookie.as_str(), &local_root).await;
    let local_terminal =
        create_terminal_for_test(local_app.clone(), cookie.as_str(), &work_root_id).await;

    // Close the *remote* terminal using the same bare id as the live local one.
    let remote_close_response = local_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!(
                    "/api/dashboard/servers/server-remote/terminals/{local_terminal}"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("remote colliding close request"),
        )
        .await
        .expect("remote colliding close response");
    assert_eq!(remote_close_response.status(), StatusCode::NO_CONTENT);

    // The local terminal with the identical bare id must still be alive.
    let local_output = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/terminals/{local_terminal}/output?after=0"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("surviving local terminal output request"),
        )
        .await
        .expect("surviving local terminal output response");
    assert_eq!(local_output.status(), StatusCode::OK);

    remote_server.abort();
    remove_static_fixture(&local_root);
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn server_scoped_git_and_worktree_local_aliases_match_legacy_routes() {
    if skip_without_git("server_scoped_git_and_worktree_local_aliases_match_legacy_routes") {
        return;
    }
    let base = temp_fixture_path("server-scoped-git-alias-parity");
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
    let git_id = open_work_root_for_test(app.clone(), cookie.as_str(), &primary).await;
    let resources = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    let workspace_id = resources["workspaces"][0]["id"]
        .as_str()
        .expect("workspace id")
        .to_owned();

    // CONTRACT: the `server-local` alias must be byte-for-byte equivalent to the
    // legacy bare route. Compare status and body directly rather than asserting
    // hardcoded status codes (Phase 4 review lesson).
    let get_pairs = [
        (
            format!("/api/dashboard/work-roots/{git_id}/git/status"),
            format!("/api/dashboard/servers/server-local/work-roots/{git_id}/git/status"),
        ),
        (
            format!("/api/dashboard/work-roots/{git_id}/git/branches"),
            format!("/api/dashboard/servers/server-local/work-roots/{git_id}/git/branches"),
        ),
        (
            format!("/api/dashboard/workspaces/{workspace_id}/git-worktree-add/options"),
            format!(
                "/api/dashboard/servers/server-local/workspaces/{workspace_id}/git-worktree-add/options"
            ),
        ),
    ];
    for (legacy_uri, alias_uri) in get_pairs {
        let legacy = get_status_and_body(app.clone(), cookie.as_str(), &legacy_uri).await;
        let alias = get_status_and_body(app.clone(), cookie.as_str(), &alias_uri).await;
        assert_eq!(alias.0, legacy.0, "status mismatch for {alias_uri}");
        assert_eq!(
            normalize_volatile_json(&alias.1),
            normalize_volatile_json(&legacy.1),
            "body mismatch for {alias_uri}"
        );
    }

    // Worktree-add preview is a POST body route; the alias must forward the same
    // response as the legacy route for the same request.
    let preview_request = serde_json::json!({
        "worktreeName": "Feature One",
        "branch": { "mode": "auto" },
        "path": { "mode": "auto" }
    });
    let legacy_preview = post_status_and_body(
        app.clone(),
        cookie.as_str(),
        &format!("/api/dashboard/workspaces/{workspace_id}/git-worktree-add/preview"),
        &preview_request,
    )
    .await;
    let alias_preview = post_status_and_body(
        app.clone(),
        cookie.as_str(),
        &format!(
            "/api/dashboard/servers/server-local/workspaces/{workspace_id}/git-worktree-add/preview"
        ),
        &preview_request,
    )
    .await;
    assert_eq!(alias_preview.0, legacy_preview.0);
    assert_eq!(
        normalize_volatile_json(&alias_preview.1),
        normalize_volatile_json(&legacy_preview.1)
    );

    // The alias must also mirror axum's `Json` extractor rejection statuses so a
    // malformed body behaves identically to the legacy route. Compare the alias
    // against the legacy route for each rejection class rather than asserting
    // hardcoded status codes (Phase 4 review lesson): a missing content type
    // (415), a valid-JSON body missing a required field (422 data error), and a
    // syntactically invalid body (400 syntax error).
    let legacy_preview_uri =
        format!("/api/dashboard/workspaces/{workspace_id}/git-worktree-add/preview");
    let alias_preview_uri = format!(
        "/api/dashboard/servers/server-local/workspaces/{workspace_id}/git-worktree-add/preview"
    );
    for (label, content_type, body) in [
        ("missing content type", None, "{}"),
        (
            "data error (missing required field)",
            Some("application/json"),
            "{}",
        ),
        (
            "syntax error (truncated JSON)",
            Some("application/json"),
            "{",
        ),
    ] {
        let build = |uri: &str| {
            let mut builder = Request::builder()
                .method(Method::POST)
                .uri(uri)
                .header(header::COOKIE, cookie.as_str());
            if let Some(content_type) = content_type {
                builder = builder.header(header::CONTENT_TYPE, content_type);
            }
            builder
                .body(Body::from(body))
                .expect("worktree preview rejection request")
        };
        let alias_rejected = app
            .clone()
            .oneshot(build(&alias_preview_uri))
            .await
            .expect("alias worktree preview rejection response");
        let legacy_rejected = app
            .clone()
            .oneshot(build(&legacy_preview_uri))
            .await
            .expect("legacy worktree preview rejection response");
        assert_eq!(
            alias_rejected.status(),
            legacy_rejected.status(),
            "server-local worktree preview alias must match legacy rejection status for {label}"
        );
    }

    // The mutating Git toolbar routes must dispatch through the `server-local`
    // alias to the same handler as the legacy route. Drive each into a
    // deterministic, non-mutating failure (so issuing the request against both
    // routes is safe and repeatable) and compare status and body for
    // equivalence, following the same pattern as the GET/preview pairs above.
    let seed_branch = current_git_branch(&primary);
    let mutation_pairs: [(&str, serde_json::Value); 5] = [
        // create-branch: an already-existing name cannot be created.
        (
            "git/branches",
            serde_json::json!({ "branchName": seed_branch, "switchTo": true }),
        ),
        // switch-branch: an unavailable branch is rejected without switching.
        (
            "git/switch-branch",
            serde_json::json!({ "branchName": "does-not-exist" }),
        ),
        // fetch/push/pull-ff-only: no remote is configured, so each fails.
        ("git/fetch", serde_json::json!({})),
        ("git/push", serde_json::json!({})),
        ("git/pull-ff-only", serde_json::json!({})),
    ];
    for (segment, request) in mutation_pairs {
        let legacy = post_status_and_body(
            app.clone(),
            cookie.as_str(),
            &format!("/api/dashboard/work-roots/{git_id}/{segment}"),
            &request,
        )
        .await;
        let alias = post_status_and_body(
            app.clone(),
            cookie.as_str(),
            &format!("/api/dashboard/servers/server-local/work-roots/{git_id}/{segment}"),
            &request,
        )
        .await;
        assert_eq!(alias.0, legacy.0, "status mismatch for {segment}");
        assert_eq!(
            normalize_volatile_json(&alias.1),
            normalize_volatile_json(&legacy.1),
            "body mismatch for {segment}"
        );
    }

    remove_static_fixture(&base);
}

#[tokio::test]
async fn server_scoped_workspace_remove_local_alias_forgets_workspace() {
    let root = temp_fixture_path("server-scoped-workspace-remove-alias");
    let state_file_root = temp_fixture_path("server-scoped-workspace-remove-alias-state");
    fs::create_dir_all(&root).expect("create workRoot");
    let store = DashboardStateStore::at_path(state_file_root.join("opened-workroots.json"));
    let state = app_state_with_opened_and_store(OpenedWorkRoots::default(), store.clone());
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;
    let resources = dashboard_resources_json(app.clone(), cookie.as_str()).await;
    let workspace_id = resources["workspaces"][0]["id"]
        .as_str()
        .expect("workspace id")
        .to_owned();

    let removed = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!(
                    "/api/dashboard/servers/server-local/workspaces/{workspace_id}"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("server-local workspace remove request"),
        )
        .await
        .expect("server-local workspace remove response");
    assert_eq!(removed.status(), StatusCode::OK);
    let removed_body = axum::body::to_bytes(removed.into_body(), 64 * 1024)
        .await
        .expect("server-local workspace remove body");
    let removed_value: serde_json::Value =
        serde_json::from_slice(&removed_body).expect("server-local workspace remove JSON");
    assert!(!work_root_ids(&removed_value).contains(&root_id));
    assert!(root.is_dir(), "workspace removal must not delete files");
    assert_eq!(store.load_opened_work_roots().await, Vec::<PathBuf>::new());

    remove_static_fixture(&root);
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn linked_server_git_and_worktree_forwarding_rewrites_resources_to_server_route() {
    if skip_without_git("linked_server_git_and_worktree_forwarding_rewrites_resources_to_server_route")
    {
        return;
    }
    let fixture = link_and_open_remote_git_root("git-worktree").await;
    let LinkedRemoteGitFixture {
        local_app,
        cookie,
        work_root_id,
        workspace_id,
        remote_server,
        remote_root,
        state_file_root,
    } = fixture;

    // Read forwarding: git status through the linked server returns the remote
    // repository's branch without leaking the remote path.
    let status = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-windows/work-roots/{work_root_id}/git/status"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("linked git status request"),
        )
        .await
        .expect("linked git status response");
    assert_eq!(status.status(), StatusCode::OK);
    let status_body = axum::body::to_bytes(status.into_body(), 64 * 1024)
        .await
        .expect("linked git status body");
    let status_value: serde_json::Value =
        serde_json::from_slice(&status_body).expect("linked git status JSON");
    assert_eq!(status_value["available"], true);
    assert!(status_value["branch"]["name"].is_string());
    assert!(
        !status_body
            .windows(remote_root.to_string_lossy().len().max(1))
            .any(|window| window == remote_root.to_string_lossy().as_bytes()),
        "linked git status must not leak the remote path"
    );

    let options = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-windows/workspaces/{workspace_id}/git-worktree-add/options"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("linked worktree options request"),
        )
        .await
        .expect("linked worktree options response");
    assert_eq!(options.status(), StatusCode::OK);
    let options_body = axum::body::to_bytes(options.into_body(), 64 * 1024)
        .await
        .expect("linked worktree options body");
    let options_value: serde_json::Value =
        serde_json::from_slice(&options_body).expect("linked worktree options JSON");
    assert_eq!(options_value["git"]["available"], true);

    // Write forwarding with resource rewrite: the worktree-add submit response
    // carries a resources view whose Server Route identities must be rewritten
    // to the browser-visible linked route.
    let submit = request_json_for_test(
        local_app.clone(),
        Method::POST,
        format!(
            "/api/dashboard/servers/server-windows/workspaces/{workspace_id}/git-worktree-add"
        ),
        &cookie,
        serde_json::json!({
            "worktreeName": "Feature One",
            "branch": { "mode": "auto" },
            "path": { "mode": "auto" },
            "activate": true
        }),
    )
    .await;
    assert_eq!(submit.0, StatusCode::OK);
    assert_eq!(submit.2["resources"]["server"]["id"], "server-windows");
    for workspace in submit.2["resources"]["workspaces"]
        .as_array()
        .expect("submit workspaces")
    {
        for root in workspace["workRoots"].as_array().expect("submit roots") {
            assert_eq!(
                root["resourcePath"]["serverId"], "server-windows",
                "worktree-add submit must rewrite workRoot serverId to the linked route"
            );
        }
    }

    // Workspace removal forwards and rewrites the returned resources view too.
    let removed = local_app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::DELETE)
                .uri(format!(
                    "/api/dashboard/servers/server-windows/workspaces/{workspace_id}"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("linked workspace remove request"),
        )
        .await
        .expect("linked workspace remove response");
    assert_eq!(removed.status(), StatusCode::OK);
    let removed_body = axum::body::to_bytes(removed.into_body(), 64 * 1024)
        .await
        .expect("linked workspace remove body");
    let removed_value: serde_json::Value =
        serde_json::from_slice(&removed_body).expect("linked workspace remove JSON");
    assert_eq!(removed_value["server"]["id"], "server-windows");

    remote_server.abort();
    remove_static_fixture(&remote_root);
    remove_static_fixture(&state_file_root);
}

#[tokio::test]
async fn linked_server_activity_events_forwarding_preserves_sse() {
    let fixture = link_and_open_remote_git_root_plain("activity-sse").await;
    let LinkedRemoteGitFixture {
        local_app,
        cookie,
        work_root_id,
        remote_server,
        remote_root,
        state_file_root,
        ..
    } = fixture;

    let events = local_app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/servers/server-windows/work-roots/{work_root_id}/activity/events"
                ))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("linked activity events request"),
        )
        .await
        .expect("linked activity events response");
    assert_eq!(events.status(), StatusCode::OK);
    assert_eq!(
        events
            .headers()
            .get(header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok())
            .map(|value| value.starts_with("text/event-stream")),
        Some(true),
        "forwarded activity events must preserve the SSE content type"
    );
    let mut stream = events.into_body().into_data_stream();
    // The activity events stream emits an initial `activity` event immediately,
    // so a successful proxy delivers at least one chunk through the gateway.
    let first = timeout(Duration::from_secs(5), stream.next())
        .await
        .expect("forwarded activity SSE chunk before timeout")
        .expect("forwarded activity SSE stream item")
        .expect("forwarded activity SSE body chunk");
    assert!(
        std::str::from_utf8(&first)
            .expect("forwarded activity SSE UTF-8")
            .contains("event:activity")
            || std::str::from_utf8(&first)
                .expect("forwarded activity SSE UTF-8")
                .contains("event: activity"),
        "forwarded activity stream must relay the daemon's activity events"
    );

    remote_server.abort();
    remove_static_fixture(&remote_root);
    remove_static_fixture(&state_file_root);
}

struct LinkedRemoteGitFixture {
    local_app: axum::Router,
    cookie: String,
    work_root_id: String,
    workspace_id: String,
    remote_server: tokio::task::JoinHandle<()>,
    remote_root: PathBuf,
    state_file_root: PathBuf,
}

async fn link_and_open_remote_git_root(tag: &str) -> LinkedRemoteGitFixture {
    let remote_root = temp_fixture_path(&format!("linked-remote-{tag}-root"));
    fs::create_dir_all(&remote_root).expect("create remote git root");
    init_git_repo(&remote_root);
    fs::write(remote_root.join("README.md"), "seed\n").expect("write remote seed");
    run_git(&remote_root, &["add", "README.md"]);
    run_git(&remote_root, &["commit", "-m", "seed"]);
    link_and_open_remote_root_at(tag, remote_root).await
}

async fn link_and_open_remote_git_root_plain(tag: &str) -> LinkedRemoteGitFixture {
    let remote_root = temp_fixture_path(&format!("linked-remote-{tag}-root"));
    fs::create_dir_all(&remote_root).expect("create remote root");
    link_and_open_remote_root_at(tag, remote_root).await
}

async fn link_and_open_remote_root_at(tag: &str, remote_root: PathBuf) -> LinkedRemoteGitFixture {
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

    let state_file_root = temp_fixture_path(&format!("linked-remote-{tag}-state"));
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
    let work_root_id = open
        .headers()
        .get("x-ws-dashboard-opened-work-root-id")
        .expect("forwarded opened id header")
        .to_str()
        .expect("forwarded opened id string")
        .to_owned();
    let open_body = axum::body::to_bytes(open.into_body(), 64 * 1024)
        .await
        .expect("remote open body");
    let open_value: serde_json::Value =
        serde_json::from_slice(&open_body).expect("remote open JSON");
    let workspace_id = open_value["workspaces"][0]["id"]
        .as_str()
        .expect("remote workspace id")
        .to_owned();

    LinkedRemoteGitFixture {
        local_app,
        cookie,
        work_root_id,
        workspace_id,
        remote_server,
        remote_root,
        state_file_root,
    }
}

// Strips volatile response fields (live timestamps) so a `server-local` alias
// body can be compared for structural equivalence against the legacy route.
fn normalize_volatile_json(body: &[u8]) -> serde_json::Value {
    let mut value: serde_json::Value =
        serde_json::from_slice(body).expect("normalize JSON body");
    strip_volatile_fields(&mut value);
    value
}

fn strip_volatile_fields(value: &mut serde_json::Value) {
    match value {
        serde_json::Value::Object(map) => {
            map.remove("refreshedAtMs");
            for nested in map.values_mut() {
                strip_volatile_fields(nested);
            }
        }
        serde_json::Value::Array(items) => {
            for item in items {
                strip_volatile_fields(item);
            }
        }
        _ => {}
    }
}

async fn get_status_and_body(
    app: axum::Router,
    cookie: &str,
    uri: &str,
) -> (StatusCode, axum::body::Bytes) {
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
    let status = response.status();
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("GET body");
    (status, body)
}

async fn post_status_and_body(
    app: axum::Router,
    cookie: &str,
    uri: &str,
    request: &serde_json::Value,
) -> (StatusCode, axum::body::Bytes) {
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
    let status = response.status();
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("POST body");
    (status, body)
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
        .contains("ws-dashboard/worktrees"));

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

    let root_status = git_stdout(&primary, &["status", "--porcelain"]);
    assert!(
        !root_status.contains(".ws-dashboard"),
        "new worktree directory must not appear untracked in the root repo's git status: {root_status}"
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

// --- 260525 Phase 1/3: git worktree remove -------------------------------

/// Add a linked worktree at `target` on an auto-derived new branch and return
/// its work-root id. Shared setup for the remove-route tests below.
async fn add_linked_worktree_for_test(
    app: axum::Router,
    cookie: &str,
    workspace_id: &str,
    worktree_name: &str,
    target: &Path,
) -> String {
    let submit = git_worktree_submit_json(
        app,
        cookie,
        workspace_id,
        serde_json::json!({
            "worktreeName": worktree_name,
            "branch": { "mode": "auto" },
            "path": { "mode": "custom", "targetPath": target.display().to_string() },
            "activate": true
        }),
        StatusCode::OK,
    )
    .await;
    submit["createdWorkRootId"]
        .as_str()
        .expect("created worktree id")
        .to_owned()
}

async fn opened_primary_workspace(
    app: axum::Router,
    cookie: &str,
    primary: &Path,
) -> String {
    open_work_root_for_test(app.clone(), cookie, primary).await;
    let resources = dashboard_resources_json(app, cookie).await;
    resources["workspaces"][0]["id"]
        .as_str()
        .expect("workspace id")
        .to_owned()
}

fn seeded_primary_repo(name: &str) -> (PathBuf, PathBuf) {
    let base = temp_fixture_path(name);
    let primary = base.join("primary");
    fs::create_dir_all(&primary).expect("create primary");
    init_git_repo(&primary);
    fs::write(primary.join("README.md"), "seed\n").expect("write seed");
    run_git(&primary, &["add", "README.md"]);
    run_git(&primary, &["commit", "-m", "seed"]);
    (base, primary)
}

#[tokio::test]
async fn git_worktree_remove_routes_are_owner_authenticated() {
    let app = build_router(app_state());
    for (method, uri) in [
        (
            Method::GET,
            "/api/dashboard/work-roots/root-local-missing/git-worktree-remove/preview",
        ),
        (
            Method::POST,
            "/api/dashboard/work-roots/root-local-missing/git-worktree-remove",
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
                    .expect("unauthenticated worktree remove request"),
            )
            .await
            .expect("unauthenticated worktree remove response");
        assert_eq!(response.status(), StatusCode::UNAUTHORIZED, "{uri}");
    }
}

#[tokio::test]
async fn git_worktree_remove_clean_worktree_clears_registry_and_git() {
    if skip_without_git("git_worktree_remove_clean_worktree_clears_registry_and_git") {
        return;
    }
    let (base, primary) = seeded_primary_repo("git-worktree-remove-clean");
    let target = base.join("wt-clean");

    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let workspace_id = opened_primary_workspace(app.clone(), cookie.as_str(), &primary).await;
    let created =
        add_linked_worktree_for_test(app.clone(), cookie.as_str(), &workspace_id, "Topic One", &target)
            .await;
    assert!(target.is_dir(), "worktree directory exists after add");

    let preview = git_worktree_remove_preview_json(app.clone(), cookie.as_str(), &created).await;
    assert_eq!(preview["available"], true);
    assert_eq!(preview["hasUncommittedChanges"], false);
    assert_eq!(preview["branchName"], "Topic-One");
    assert_eq!(preview["branchUnmerged"], false);

    let submit = git_worktree_remove_submit_json(
        app.clone(),
        cookie.as_str(),
        &created,
        serde_json::json!({ "deleteBranch": false, "force": false }),
        StatusCode::OK,
    )
    .await;
    assert_eq!(submit["removedWorkRootId"], created);
    assert_eq!(submit["branchDeleted"], false);
    assert!(!work_root_ids(&submit["resources"]).contains(&created));
    assert!(!target.exists(), "git worktree remove deletes the directory");
    assert!(
        !git_stdout(&primary, &["worktree", "list", "--porcelain"]).contains("wt-clean"),
        "removed worktree must be gone from git worktree list"
    );
    let body = serde_json::to_string(&submit).expect("submit body");
    assert!(
        !body.contains(primary.to_string_lossy().as_ref()),
        "submit response must not leak the primary root path"
    );

    remove_static_fixture(&base);
}

#[tokio::test]
async fn git_worktree_remove_dirty_without_force_is_blocked() {
    if skip_without_git("git_worktree_remove_dirty_without_force_is_blocked") {
        return;
    }
    let (base, primary) = seeded_primary_repo("git-worktree-remove-dirty");
    let target = base.join("wt-dirty");

    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let workspace_id = opened_primary_workspace(app.clone(), cookie.as_str(), &primary).await;
    let created =
        add_linked_worktree_for_test(app.clone(), cookie.as_str(), &workspace_id, "Dirty Topic", &target)
            .await;
    fs::write(target.join("scratch.txt"), "wip\n").expect("write untracked file");

    let preview = git_worktree_remove_preview_json(app.clone(), cookie.as_str(), &created).await;
    assert_eq!(preview["hasUncommittedChanges"], true);
    assert_eq!(preview["untrackedFiles"], 1);

    let blocked = git_worktree_remove_submit_json(
        app.clone(),
        cookie.as_str(),
        &created,
        serde_json::json!({ "deleteBranch": false, "force": false }),
        StatusCode::CONFLICT,
    )
    .await;
    assert!(blocked["error"]
        .as_str()
        .expect("error message")
        .contains("uncommitted"));
    assert!(target.is_dir(), "blocked remove must not delete the worktree");

    let forced = git_worktree_remove_submit_json(
        app.clone(),
        cookie.as_str(),
        &created,
        serde_json::json!({ "deleteBranch": false, "force": true }),
        StatusCode::OK,
    )
    .await;
    assert_eq!(forced["removedWorkRootId"], created);
    assert!(!target.exists(), "forced remove deletes the dirty worktree");

    remove_static_fixture(&base);
}

#[tokio::test]
async fn git_worktree_remove_deletes_merged_branch_but_keeps_unmerged() {
    if skip_without_git("git_worktree_remove_deletes_merged_branch_but_keeps_unmerged") {
        return;
    }
    let (base, primary) = seeded_primary_repo("git-worktree-remove-branch");

    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let workspace_id = opened_primary_workspace(app.clone(), cookie.as_str(), &primary).await;

    // Merged branch: no commits beyond the seed HEAD -> deleteBranch deletes it.
    let merged_target = base.join("wt-merged");
    let merged = add_linked_worktree_for_test(
        app.clone(),
        cookie.as_str(),
        &workspace_id,
        "Merged Topic",
        &merged_target,
    )
    .await;
    let merged_submit = git_worktree_remove_submit_json(
        app.clone(),
        cookie.as_str(),
        &merged,
        serde_json::json!({ "deleteBranch": true, "force": false }),
        StatusCode::OK,
    )
    .await;
    assert_eq!(merged_submit["branchDeleted"], true);
    assert_eq!(merged_submit["branchDeleteSkippedUnmerged"], false);
    assert!(
        git_stdout(&primary, &["branch", "--list", "Merged-Topic"]).is_empty(),
        "merged branch must be deleted"
    );

    // Unmerged branch: a commit made inside the worktree is NOT reachable from
    // HEAD, so deleteBranch must be refused (never `-D`) and the branch kept.
    let dangling_target = base.join("wt-dangling");
    let dangling = add_linked_worktree_for_test(
        app.clone(),
        cookie.as_str(),
        &workspace_id,
        "Dangling Topic",
        &dangling_target,
    )
    .await;
    fs::write(dangling_target.join("README.md"), "seed\nmore\n").expect("edit in worktree");
    run_git(&dangling_target, &["commit", "-aqm", "unique worktree commit"]);

    let unmerged_preview =
        git_worktree_remove_preview_json(app.clone(), cookie.as_str(), &dangling).await;
    assert_eq!(unmerged_preview["branchName"], "Dangling-Topic");
    assert_eq!(unmerged_preview["branchUnmerged"], true);

    let dangling_submit = git_worktree_remove_submit_json(
        app.clone(),
        cookie.as_str(),
        &dangling,
        serde_json::json!({ "deleteBranch": true, "force": false }),
        StatusCode::OK,
    )
    .await;
    assert_eq!(dangling_submit["branchDeleted"], false);
    assert_eq!(dangling_submit["branchDeleteSkippedUnmerged"], true);
    assert!(
        !git_stdout(&primary, &["branch", "--list", "Dangling-Topic"]).is_empty(),
        "unmerged branch must survive a deleteBranch request (never force-deleted)"
    );

    remove_static_fixture(&base);
}

#[tokio::test]
async fn git_worktree_remove_clears_terminal_sessions_for_removed_root() {
    if skip_without_git("git_worktree_remove_clears_terminal_sessions_for_removed_root") {
        return;
    }
    let (base, primary) = seeded_primary_repo("git-worktree-remove-sessions");
    let target = base.join("wt-sessions");

    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let workspace_id = opened_primary_workspace(app.clone(), cookie.as_str(), &primary).await;
    let created = add_linked_worktree_for_test(
        app.clone(),
        cookie.as_str(),
        &workspace_id,
        "Session Topic",
        &target,
    )
    .await;
    let terminal_id = create_terminal_for_test(app.clone(), cookie.as_str(), &created).await;

    git_worktree_remove_submit_json(
        app.clone(),
        cookie.as_str(),
        &created,
        serde_json::json!({ "deleteBranch": false, "force": false }),
        StatusCode::OK,
    )
    .await;

    // A cleared session yields "unknown terminal" (404), not a
    // still-registered-but-offline error.
    let response = app
        .oneshot(
            Request::builder()
                .uri(format!("/api/dashboard/terminals/{terminal_id}/output"))
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("terminal output request"),
        )
        .await
        .expect("terminal output response");
    assert_eq!(response.status(), StatusCode::NOT_FOUND);

    remove_static_fixture(&base);
}

#[tokio::test]
async fn git_worktree_remove_clears_codex_and_claude_sessions_for_removed_root() {
    if skip_without_git("git_worktree_remove_clears_codex_and_claude_sessions_for_removed_root") {
        return;
    }
    let (base, primary) = seeded_primary_repo("git-worktree-remove-agent-sessions");
    let target = base.join("wt-agent-sessions");

    // Invariant 4: agent-session cleanup covers codex AND claude, not only
    // terminals. Install owned registries so the seeded sessions live in the
    // exact instances the removal handler calls `remove_for_work_roots` on.
    let mut state = app_state();
    let codex_registry = CodexProviderRegistry::default();
    let claude_registry = ClaudeProviderRegistry::default();
    state.codex_sessions = codex_registry.clone();
    state.claude_sessions = claude_registry.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let workspace_id = opened_primary_workspace(app.clone(), cookie.as_str(), &primary).await;
    let created = add_linked_worktree_for_test(
        app.clone(),
        cookie.as_str(),
        &workspace_id,
        "Agent Topic",
        &target,
    )
    .await;

    let worktree_id = WorkRootId::from(created.clone());
    codex_registry
        .insert_session_for_tests(
            "server-local",
            "codex:remove",
            worktree_id.clone(),
            "thread-remove",
            spawn_codex_reply_peer(serde_json::json!({ "turn": { "id": "t" } })),
            CodexProjector::new(),
        )
        .expect("seed codex session");
    claude_registry
        .insert_session_for_tests(
            "server-local",
            "claude:remove",
            worktree_id.clone(),
            "session-remove",
            target.clone(),
            spawn_claude_reply_peer("ack"),
            ClaudeProjector::new(),
        )
        .expect("seed claude session");
    assert!(codex_registry
        .session_for("server-local", "codex:remove")
        .is_some());
    assert!(claude_registry
        .session_for("server-local", "claude:remove")
        .is_some());

    git_worktree_remove_submit_json(
        app,
        cookie.as_str(),
        &created,
        serde_json::json!({ "deleteBranch": false, "force": false }),
        StatusCode::OK,
    )
    .await;

    assert!(
        codex_registry
            .session_for("server-local", "codex:remove")
            .is_none(),
        "codex session for the removed worktree must be cleared"
    );
    assert!(
        claude_registry
            .session_for("server-local", "claude:remove")
            .is_none(),
        "claude session for the removed worktree must be cleared"
    );

    remove_static_fixture(&base);
}

#[tokio::test]
async fn git_worktree_remove_persist_failure_keeps_removal_and_clears_sessions() {
    if skip_without_git("git_worktree_remove_persist_failure_keeps_removal_and_clears_sessions") {
        return;
    }
    let (base, primary) = seeded_primary_repo("git-worktree-remove-persist-fail");
    let target = base.join("wt-persist-fail");
    // Create the linked worktree directly on disk (a failing store would
    // reject the add route's own persist), then register both roots so the
    // live resource view groups them into one workspace.
    run_git(
        &primary,
        &[
            "worktree",
            "add",
            target.to_string_lossy().as_ref(),
            "-b",
            "persist-fail-topic",
        ],
    );
    let worktree_id = ws_dashboard_daemon::discovery::local_work_root_id_for_path(&target);
    let worktree_id_str = worktree_id.as_str().to_owned();

    // A store pointing at a directory (not a file) makes every persist fail,
    // reproducing a registry-persist failure AFTER the irreversible disk
    // removal — the exact ordering invariant 3 protects.
    let state_file_directory = temp_fixture_path("git-worktree-remove-persist-fail-store");
    fs::create_dir_all(&state_file_directory).expect("create state-file directory");
    let mut state = app_state_with_opened_and_store(
        OpenedWorkRoots::from_paths(vec![primary.clone(), target.clone()]),
        DashboardStateStore::at_path(&state_file_directory),
    );
    let codex_registry = CodexProviderRegistry::default();
    let claude_registry = ClaudeProviderRegistry::default();
    state.codex_sessions = codex_registry.clone();
    state.claude_sessions = claude_registry.clone();
    codex_registry
        .insert_session_for_tests(
            "server-local",
            "codex:persist-fail",
            worktree_id.clone(),
            "thread-persist-fail",
            spawn_codex_reply_peer(serde_json::json!({ "turn": { "id": "t" } })),
            CodexProjector::new(),
        )
        .expect("seed codex session");
    claude_registry
        .insert_session_for_tests(
            "server-local",
            "claude:persist-fail",
            worktree_id.clone(),
            "session-persist-fail",
            target.clone(),
            spawn_claude_reply_peer("ack"),
            ClaudeProjector::new(),
        )
        .expect("seed claude session");

    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    assert!(codex_registry
        .session_for("server-local", "codex:persist-fail")
        .is_some());
    assert!(claude_registry
        .session_for("server-local", "claude:persist-fail")
        .is_some());

    // Disk removal succeeds; the subsequent persist fails. The corrected
    // rollback must NOT re-register the already-deleted entry (no zombie row
    // pointing at a deleted path), must still clear ALL sessions, and must
    // return the removal result — disk is the source of truth once removed.
    // A regression that re-registers + returns 500 before session cleanup
    // fails this test on the status assertion alone.
    let submit = git_worktree_remove_submit_json(
        app.clone(),
        cookie.as_str(),
        &worktree_id_str,
        serde_json::json!({ "deleteBranch": false, "force": false }),
        StatusCode::OK,
    )
    .await;
    assert_eq!(submit["removedWorkRootId"], worktree_id_str);
    assert!(!work_root_ids(&submit["resources"]).contains(&worktree_id_str));
    assert!(!target.exists(), "disk removal is irreversible and must stand");

    // No zombie re-register: a fresh resources read must not resurrect the id.
    let resources = dashboard_resources_json(app, cookie.as_str()).await;
    assert!(
        !work_root_ids(&resources).contains(&worktree_id_str),
        "persist failure after disk removal must not resurrect the registry entry"
    );

    // Session cleanup still ran despite the persist failure.
    assert!(
        codex_registry
            .session_for("server-local", "codex:persist-fail")
            .is_none(),
        "codex session must be cleared even when persist fails after disk removal"
    );
    assert!(
        claude_registry
            .session_for("server-local", "claude:persist-fail")
            .is_none(),
        "claude session must be cleared even when persist fails after disk removal"
    );

    remove_static_fixture(&base);
    remove_static_fixture(&state_file_directory);
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
        terminals: test_terminal_registry(),
        codex_sessions: ws_dashboard_daemon::codex_app_server::CodexProviderRegistry::default(),
        claude_sessions: ws_dashboard_daemon::claude_cli::ClaudeProviderRegistry::default(),
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
                agent_key, public_name, state_path, schema_version,
                backend, harness, tier, model, effort, session_id, status,
                created_at, updated_at, last_seen_at, last_call_at, last_output_path
            ) VALUES (?1, ?2, ?3, 1, ?4, ?5, ?6, ?7, ?8, ?9, ?10, '', '', '', ?11, ?12)",
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

async fn git_worktree_remove_preview_json(
    app: axum::Router,
    cookie: &str,
    work_root_id: &str,
) -> serde_json::Value {
    let response = app
        .oneshot(
            Request::builder()
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/git-worktree-remove/preview"
                ))
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("git worktree remove preview request"),
        )
        .await
        .expect("git worktree remove preview response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("git worktree remove preview body");
    serde_json::from_slice(&body).expect("git worktree remove preview JSON")
}

async fn git_worktree_remove_submit_json(
    app: axum::Router,
    cookie: &str,
    work_root_id: &str,
    request: serde_json::Value,
    expected_status: StatusCode,
) -> serde_json::Value {
    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri(format!(
                    "/api/dashboard/work-roots/{work_root_id}/git-worktree-remove"
                ))
                .header(header::COOKIE, cookie)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(request.to_string()))
                .expect("git worktree remove submit request"),
        )
        .await
        .expect("git worktree remove submit response");
    assert_eq!(response.status(), expected_status);
    let body = axum::body::to_bytes(response.into_body(), 64 * 1024)
        .await
        .expect("git worktree remove submit body");
    serde_json::from_slice(&body).expect("git worktree remove submit JSON")
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

// The server-local socket route dispatches in-process to the unscoped legacy
// terminal_websocket handler, byte-for-byte, without any relay.
#[tokio::test]
async fn server_scoped_local_terminal_websocket_dispatches_legacy_handler() {
    let root = temp_fixture_path("server-scoped-local-terminal-websocket-root");
    fs::create_dir_all(&root).expect("create server-local terminal websocket root dir");
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;
    let terminal_id = create_terminal_for_test(app.clone(), cookie.as_str(), &work_root_id).await;
    let (addr, server) = spawn_test_server(app.clone()).await;

    let mut request =
        format!("ws://{addr}/api/dashboard/servers/server-local/terminals/{terminal_id}/socket")
            .into_client_request()
            .expect("server-local websocket request");
    request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let (mut socket, response) = tokio_tungstenite::connect_async(request)
        .await
        .expect("connect server-local terminal websocket");
    assert_eq!(response.status(), StatusCode::SWITCHING_PROTOCOLS);

    socket
        .send(TungsteniteMessage::Text(
            serde_json::json!({ "type": "input", "data": terminal_test_commands_for_current_platform("SERVER-SCOPED-WS").echo_and_exit })
                .to_string()
                .into(),
        ))
        .await
        .expect("send server-local websocket input");
    let mut text = String::new();
    for _ in 0..80 {
        let Some(message) = timeout(Duration::from_secs(2), socket.next())
            .await
            .expect("server-local websocket message timeout")
        else {
            break;
        };
        let message = message.expect("server-local websocket message");
        let TungsteniteMessage::Text(payload) = message else {
            continue;
        };
        let value: serde_json::Value =
            serde_json::from_str(&payload).expect("server-local websocket frame JSON");
        assert_eq!(value["terminalId"], terminal_id);
        match value["type"].as_str() {
            Some("output") => text.push_str(value["chunk"]["data"].as_str().expect("output data")),
            Some("exit") => break,
            Some("status") => {}
            other => panic!("unexpected server-local websocket frame type: {other:?}"),
        }
        if text.contains("SERVER-SCOPED-WS") {
            break;
        }
    }
    assert!(text.contains("SERVER-SCOPED-WS"), "{text:?}");

    socket.close(None).await.expect("close server-local websocket");
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

    // Grace-reattach (260723 confirmed decision): an exited-but-in-grace
    // terminal still admits a WebSocket attach so a reconnecting client can
    // observe the final output/exit frame instead of racing a hard reject.
    // `admits_attach()` only turns false once the grace window elapses, so
    // this attach must succeed rather than 410.
    let mut in_grace_request = format!("ws://{addr}/api/dashboard/terminals/{terminal_id}/socket")
        .into_client_request()
        .expect("in-grace websocket request");
    in_grace_request
        .headers_mut()
        .insert(header::COOKIE, cookie.parse().expect("cookie header"));
    let (mut in_grace_socket, in_grace_response) =
        tokio_tungstenite::connect_async(in_grace_request)
            .await
            .expect("in-grace websocket upgrades");
    assert_eq!(in_grace_response.status(), StatusCode::SWITCHING_PROTOCOLS);
    in_grace_socket
        .close(None)
        .await
        .expect("close in-grace websocket");

    // Once the session is explicitly closed (removed from the registry
    // rather than merely exited-in-grace) a further attach must reject
    // before upgrade, same as any other unknown terminal id.
    let close_response = app
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
    assert_eq!(close_response.status(), StatusCode::NO_CONTENT);

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
            assert_eq!(response.status(), StatusCode::NOT_FOUND);
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
async fn no_auth_mode_reaches_protected_http_and_websocket_routes_without_credentials() {
    let app = build_router(app_state_without_owner_auth());

    let health = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .body(Body::empty())
                .expect("no-auth health request"),
        )
        .await
        .expect("no-auth health response");
    assert_eq!(health.status(), StatusCode::OK);

    let resources = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/resources")
                .body(Body::empty())
                .expect("no-auth resources request"),
        )
        .await
        .expect("no-auth resources response");
    assert_eq!(resources.status(), StatusCode::OK);

    let (addr, server) = spawn_test_server(app.clone()).await;
    let websocket_request = format!("ws://{addr}/api/dashboard/terminals/term_test/socket")
        .into_client_request()
        .expect("no-auth terminal websocket request");
    let error = tokio_tungstenite::connect_async(websocket_request)
        .await
        .expect_err("no-auth terminal websocket rejects unknown terminal");
    match error {
        tokio_tungstenite::tungstenite::Error::Http(response) => {
            assert_eq!(response.status(), StatusCode::NOT_FOUND);
        }
        other => panic!("unexpected no-auth websocket error: {other}"),
    }
    server.abort();
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
        terminals: test_terminal_registry(),
        codex_sessions: ws_dashboard_daemon::codex_app_server::CodexProviderRegistry::default(),
        claude_sessions: ws_dashboard_daemon::claude_cli::ClaudeProviderRegistry::default(),
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

// ---------------------------------------------------------------------------
// Codex app-server interactive-session routes (Phase 2)
//
// CONTRACT: These drive the six new HTTP endpoints through
// `tower::ServiceExt::oneshot` against a real Router/AppState, matching the
// established route-test pattern. Sessions are seeded into a shared
// `CodexProviderRegistry` via `insert_session_for_tests` (backed by an
// in-process scripted NDJSON peer) so the write/read wiring, request parsing,
// status-code mapping, LOCAL_SERVER_ID short-circuit vs forward branch, and
// projection privacy are exercised without spawning a real `codex app-server`.
// ---------------------------------------------------------------------------

/// Spawn an in-process NDJSON peer that answers every JSON-RPC request line
/// with the supplied `result`. Returns the client-side `CodexConnection`.
fn spawn_codex_reply_peer(result: serde_json::Value) -> Arc<CodexConnection> {
    use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};

    let (client_side, mut server_side) = tokio::io::duplex(8192);
    let (client_read, client_write) = tokio::io::split(client_side);
    let (connection, _notifications) =
        CodexConnection::from_io(client_read, client_write, Duration::from_secs(5));
    tokio::spawn(async move {
        let (server_read, mut server_write) = tokio::io::split(&mut server_side);
        let mut lines = BufReader::new(server_read).lines();
        while let Ok(Some(line)) = lines.next_line().await {
            let Ok(message) = serde_json::from_str::<serde_json::Value>(&line) else {
                continue;
            };
            // Reply only to requests (those carrying an integer id).
            if let Some(id) = message.get("id").and_then(|value| value.as_i64()) {
                let mut reply =
                    serde_json::to_string(&serde_json::json!({ "id": id, "result": result.clone() }))
                        .expect("serialize codex peer reply");
                reply.push('\n');
                if server_write.write_all(reply.as_bytes()).await.is_err() {
                    break;
                }
                let _ = server_write.flush().await;
            }
        }
    });
    connection
}

#[tokio::test]
async fn codex_session_prompt_and_transcript_round_trip_local() {
    let registry = CodexProviderRegistry::default();
    // Pre-populate the projector so the transcript route has projected content.
    let mut projector = CodexProjector::new();
    projector.ingest_line(
        r#"{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"assistant-secret-item","text":"hello from codex"}}}"#,
    );
    let connection = spawn_codex_reply_peer(serde_json::json!({ "turn": { "id": "turn-secret" } }));
    registry
        .insert_session_for_tests(
            "server-local",
            "codex:roundtrip",
            WorkRootId::from("codex-wr"),
            "thread-secret-id",
            connection,
            projector,
        )
        .expect("seed codex session");

    let mut state = app_state();
    state.codex_sessions = registry.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    // POST prompt -> the scripted peer acknowledges turn/start.
    let prompt_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/codex-wr/activity/codex-sessions/codex:roundtrip/prompt")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "text": "do a thing" }).to_string(),
                ))
                .expect("prompt request"),
        )
        .await
        .expect("prompt response");
    assert_eq!(prompt_response.status(), StatusCode::OK);
    let prompt_body = axum::body::to_bytes(prompt_response.into_body(), 16 * 1024)
        .await
        .expect("prompt body bytes");
    let prompt_json: serde_json::Value =
        serde_json::from_slice(&prompt_body).expect("prompt JSON");
    assert_eq!(prompt_json["accepted"], true);

    // GET transcript -> projected blocks; no provider ids/paths.
    let (status, body) = fetch_work_root_activity_path(
        app,
        cookie.as_str(),
        "/api/dashboard/work-roots/codex-wr/activity/codex-sessions/codex:roundtrip/transcript",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let transcript: serde_json::Value = serde_json::from_str(&body).expect("transcript JSON");
    assert_eq!(transcript["activityId"], "codex:roundtrip");
    assert!(body.contains("hello from codex"), "projected block text missing: {body}");
    for forbidden in [
        "thread-secret-id",
        "assistant-secret-item",
        "turn-secret",
        "threadId",
        "sessionId",
    ] {
        assert!(!body.contains(forbidden), "transcript leaked {forbidden}: {body}");
    }
}

/// Phase 4 (`260713-feat-ws-dashboard-agent-chat-real-adapter-wiring`)
/// coverage gap: `codex_session_prompt_and_transcript_round_trip_local` above
/// only exercises a single prompt + a single transcript GET. This test drives
/// the actual `live: true -> live: false` multi-poll sequence
/// `beginRealStreamingTurn` (`activitySessionClient.ts`) depends on: prompt ->
/// poll #1 (live, initial blocks) -> ingest more transcript content directly
/// into the held session's projector (simulating the harness pushing further
/// turn progress between polls) -> poll #2 (still live, more blocks) ->
/// ingest a `turn/completed` -> poll #3 (`live: false`, final block count).
/// `insert_session_for_tests` returns the live `Arc<CodexSession>` handle
/// precisely so a test can reach into `session.projector` between polls
/// without any new test infrastructure.
#[tokio::test]
async fn codex_session_send_receive_multi_poll_e2e() {
    let registry = CodexProviderRegistry::default();
    let connection = spawn_codex_reply_peer(serde_json::json!({ "turn": { "id": "turn-secret" } }));
    // Seed the projector with a `turn/started` only (no item content yet) so
    // it models the state right after the prompt was accepted and the
    // harness began working — `is_turn_active()` is true, but no blocks
    // exist yet. Real `turn/started` delivery happens over the connection's
    // notification stream in production; the test drives the projector
    // directly (as `insert_session_for_tests`'s own doc comment describes)
    // since the scripted reply peer above only answers RPC requests, not
    // notifications.
    let mut projector = CodexProjector::new();
    projector.ingest_line(r#"{"method":"turn/started","params":{"turn":{"id":"turn-secret"}}}"#);
    let session = registry
        .insert_session_for_tests(
            "server-local",
            "codex:multipoll",
            WorkRootId::from("codex-wr"),
            "thread-secret-id",
            connection,
            projector,
        )
        .expect("seed codex session");

    let mut state = app_state();
    state.codex_sessions = registry.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    // POST prompt -> the scripted peer acknowledges turn/start.
    let prompt_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/codex-wr/activity/codex-sessions/codex:multipoll/prompt")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "text": "do a multi-poll thing" }).to_string(),
                ))
                .expect("prompt request"),
        )
        .await
        .expect("prompt response");
    assert_eq!(prompt_response.status(), StatusCode::OK);

    // Poll #1: the seeded `turn/started` makes the session live; no item
    // content has arrived yet.
    let (status, body) = fetch_work_root_activity_path(
        app.clone(),
        cookie.as_str(),
        "/api/dashboard/work-roots/codex-wr/activity/codex-sessions/codex:multipoll/transcript",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let transcript: serde_json::Value = serde_json::from_str(&body).expect("poll #1 transcript JSON");
    assert_eq!(transcript["live"], true, "poll #1: turn/started was seeded, no terminal event ingested yet");
    assert_eq!(
        transcript["blocks"].as_array().expect("poll #1 blocks array").len(),
        0,
        "poll #1: no item content ingested yet"
    );

    // Between poll #1 and #2: the harness pushes mid-turn progress.
    {
        let projector_handle = session.projector();
        let mut projector = projector_handle.lock().await;
        projector.ingest_line(
            r#"{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"assistant-item-1","text":"working on it"}}}"#,
        );
    }

    // Poll #2: still live, now has the mid-turn block.
    let (status, body) = fetch_work_root_activity_path(
        app.clone(),
        cookie.as_str(),
        "/api/dashboard/work-roots/codex-wr/activity/codex-sessions/codex:multipoll/transcript",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let transcript: serde_json::Value = serde_json::from_str(&body).expect("poll #2 transcript JSON");
    assert_eq!(transcript["live"], true, "poll #2: turn/started was ingested but no terminal event yet");
    let blocks = transcript["blocks"].as_array().expect("poll #2 blocks array");
    assert_eq!(blocks.len(), 1, "poll #2: one mid-turn block ingested since poll #1");
    assert!(body.contains("working on it"), "poll #2 missing mid-turn block text: {body}");

    // Between poll #2 and #3: the harness pushes a second block then the
    // terminal turn/completed event.
    {
        let projector_handle = session.projector();
        let mut projector = projector_handle.lock().await;
        projector.ingest_line(
            r#"{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"assistant-item-2","text":"done with it"}}}"#,
        );
        projector.ingest_line(
            r#"{"method":"turn/completed","params":{"turn":{"id":"turn-secret","status":"completed"}}}"#,
        );
    }

    // Poll #3: terminal event observed -> live:false, final block count.
    let (status, body) = fetch_work_root_activity_path(
        app,
        cookie.as_str(),
        "/api/dashboard/work-roots/codex-wr/activity/codex-sessions/codex:multipoll/transcript",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let transcript: serde_json::Value = serde_json::from_str(&body).expect("poll #3 transcript JSON");
    assert_eq!(transcript["live"], false, "poll #3: turn/completed flips is_turn_active to false");
    let blocks = transcript["blocks"].as_array().expect("poll #3 blocks array");
    assert_eq!(blocks.len(), 2, "poll #3: both mid-turn blocks present, turn/completed adds no block itself");
    assert!(body.contains("working on it"), "poll #3 missing first block text: {body}");
    assert!(body.contains("done with it"), "poll #3 missing second block text: {body}");
    // `260713-bug-dashboard-agent-chat-transcript-role-turnid-echo` Phase 2:
    // the new `turnId` wire field is a ticket-approved, explicit exception to
    // the general "provider ids never cross the boundary" rule -- it exists
    // specifically to carry a browser-side bubble-merge-equality key, and
    // both blocks in this single turn legitimately share the provider's own
    // turn id here. Item ids (`assistant-item-1`/`-2`), the thread id, and
    // the raw `threadId`/`sessionId` field names remain forbidden -- only
    // `turnId` is exempted.
    assert_eq!(blocks[0]["turnId"], "turn-secret", "poll #3: turnId carries the provider turn id for bubble-merge grouping");
    assert_eq!(blocks[1]["turnId"], "turn-secret", "poll #3: both blocks from the one turn share the same turnId");
    for forbidden in [
        "thread-secret-id",
        "assistant-item-1",
        "assistant-item-2",
        "threadId",
        "sessionId",
    ] {
        assert!(!body.contains(forbidden), "multi-poll transcript leaked {forbidden}: {body}");
    }
}

#[tokio::test]
async fn codex_session_control_skills_projects_without_raw_json() {
    let registry = CodexProviderRegistry::default();
    let connection = spawn_codex_reply_peer(serde_json::json!({
        "data": [{
            "name": "do-thing",
            "description": "does a thing",
            "source": "/home/x/.codex/skills/do-thing/SKILL.md",
            "cwd": "/private/host"
        }]
    }));
    registry
        .insert_session_for_tests(
            "server-local",
            "codex:skills",
            WorkRootId::from("codex-wr"),
            "thread-secret-id",
            connection,
            CodexProjector::new(),
        )
        .expect("seed codex session");

    let mut state = app_state();
    state.codex_sessions = registry.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/codex-wr/activity/codex-sessions/codex:skills/control")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(serde_json::json!({ "action": "skills" }).to_string()))
                .expect("skills control request"),
        )
        .await
        .expect("skills control response");
    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 16 * 1024)
        .await
        .expect("skills control body bytes");
    let body_text = String::from_utf8(body.to_vec()).expect("skills body UTF-8");
    let value: serde_json::Value = serde_json::from_str(&body_text).expect("skills control JSON");
    assert_eq!(value["applied"], true);
    assert_eq!(value["data"]["count"], 1);
    assert!(body_text.contains("do-thing"));
    assert!(body_text.contains("does a thing"));
    // CONTRACT: raw provider paths / private fields never cross the boundary.
    for forbidden in [
        "/home/x/.codex",
        "SKILL.md",
        "/private/host",
        "source",
        "cwd",
    ] {
        assert!(
            !body_text.contains(forbidden),
            "skills control response leaked {forbidden}: {body_text}"
        );
    }
}

/// `260713` Phase 3: the `Fork` control-request shape deserializes and
/// dispatches to `provider.fork`. This mirrors
/// `codex_session_control_skills_projects_without_raw_json`'s route-test
/// pattern, but only exercises what is testable without a real process spawn
/// (see plan Codebase Findings "Testability gap"): a missing/unknown source
/// `activityId` must be rejected the same way other control arms reject it,
/// with no panic or process spawn attempted. The full spawn-a-new-connection
/// + live `thread/fork` round-trip has no existing test seam in this file
/// (same gap `create_codex_session` already has) and stays a manual/Phase-4
/// check per the plan.
#[tokio::test]
async fn codex_session_control_fork_rejects_unknown_source_session() {
    let registry = CodexProviderRegistry::default();
    let state = {
        let mut state = app_state();
        state.codex_sessions = registry;
        state
    };
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/codex-wr/activity/codex-sessions/codex:missing/control")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "action": "fork", "cutCursor": "3" }).to_string(),
                ))
                .expect("fork control request"),
        )
        .await
        .expect("fork control response");
    // Same "unknown Codex session" mapping the other control arms use
    // (`provider_error_response`'s `codex.unknown_session` -> 404).
    assert_eq!(response.status(), StatusCode::NOT_FOUND);
    let body = axum::body::to_bytes(response.into_body(), 16 * 1024)
        .await
        .expect("fork control body bytes");
    let body_text = String::from_utf8(body.to_vec()).expect("fork control body UTF-8");
    let value: serde_json::Value = serde_json::from_str(&body_text).expect("fork control JSON");
    assert_eq!(value["code"], "codex.unknown_session");
}

/// The `Fork` variant must deserialize the wire shape the frontend already
/// sends (`{"action":"fork","cutCursor":...}`), including the `cutCursor:
/// null` "fork the whole thread" case, without rejecting the request body
/// before it ever reaches the provider.
#[test]
fn codex_control_request_fork_deserializes_wire_shape() {
    let with_cursor: CodexControlRequest =
        serde_json::from_value(serde_json::json!({ "action": "fork", "cutCursor": "7" }))
            .expect("deserialize fork with cutCursor");
    match with_cursor {
        CodexControlRequest::Fork { cut_cursor } => assert_eq!(cut_cursor, Some("7".to_owned())),
        other => panic!("expected Fork variant, got {other:?}"),
    }

    let without_cursor: CodexControlRequest =
        serde_json::from_value(serde_json::json!({ "action": "fork", "cutCursor": null }))
            .expect("deserialize fork with null cutCursor");
    match without_cursor {
        CodexControlRequest::Fork { cut_cursor } => assert_eq!(cut_cursor, None),
        other => panic!("expected Fork variant, got {other:?}"),
    }
}

#[tokio::test]
async fn codex_session_prompt_unknown_session_maps_not_found() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/codex-wr/activity/codex-sessions/codex:missing/prompt")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(serde_json::json!({ "text": "hi" }).to_string()))
                .expect("prompt request"),
        )
        .await
        .expect("prompt response");
    // provider_error_response maps codex.unknown_session -> 404.
    assert_eq!(response.status(), StatusCode::NOT_FOUND);
    let body = axum::body::to_bytes(response.into_body(), 8 * 1024)
        .await
        .expect("error body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("error JSON");
    assert_eq!(value["code"], "codex.unknown_session");
}

#[tokio::test]
async fn server_scoped_codex_prompt_short_circuits_local_and_forwards_remote() {
    let registry = CodexProviderRegistry::default();
    let connection = spawn_codex_reply_peer(serde_json::json!({ "turn": { "id": "t" } }));
    registry
        .insert_session_for_tests(
            "server-local",
            "codex:scoped",
            WorkRootId::from("codex-wr"),
            "thread-secret-id",
            connection,
            CodexProjector::new(),
        )
        .expect("seed codex session");

    let mut state = app_state();
    state.codex_sessions = registry.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    // LOCAL_SERVER_ID short-circuits to the in-process local handler.
    let local = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/server-local/work-roots/codex-wr/activity/codex-sessions/codex:scoped/prompt")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(serde_json::json!({ "text": "hi" }).to_string()))
                .expect("local prompt request"),
        )
        .await
        .expect("local prompt response");
    assert_eq!(local.status(), StatusCode::OK);

    // A non-local, unlinked server takes the forward branch and is refused.
    let forwarded = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/does-not-exist/work-roots/codex-wr/activity/codex-sessions/codex:scoped/prompt")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(serde_json::json!({ "text": "hi" }).to_string()))
                .expect("forward prompt request"),
        )
        .await
        .expect("forward prompt response");
    assert_eq!(forwarded.status(), StatusCode::NOT_FOUND);
}

#[tokio::test]
async fn work_root_activity_route_merges_codex_session_into_items() {
    if skip_without_git("work_root_activity_route_merges_codex_session_into_items") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-codex-merge");
    let cache_home = temp_fixture_path("work-root-activity-codex-merge-cache");
    fs::create_dir_all(&root).expect("create workRoot");
    init_git_repo(&root);

    let registry = CodexProviderRegistry::default();
    let mut state = app_state_with_activity_cache_home(cache_home.clone());
    state.codex_sessions = registry.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    // Seed a live Codex session for the opened work root.
    let mut projector = CodexProjector::new();
    projector.ingest_line(r#"{"method":"turn/started","params":{"turn":{"id":"turn-secret"}}}"#);
    projector.ingest_line(
        r#"{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"item-secret","text":"codex says hi"}}}"#,
    );
    let connection = spawn_codex_reply_peer(serde_json::json!({}));
    registry
        .insert_session_for_tests(
            "server-local",
            "codex:feedmerge",
            WorkRootId::from(work_root_id.clone()),
            "thread-secret",
            connection,
            projector,
        )
        .expect("seed codex session");

    let (status, body) = fetch_work_root_activity(app, cookie.as_str(), &work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let feed: serde_json::Value = serde_json::from_str(&body).expect("feed JSON");
    let items = feed["items"].as_array().expect("items array");
    let codex_item = items
        .iter()
        .find(|item| item["id"] == "codex:feedmerge")
        .expect("Codex session must appear in the unified feed items");
    assert_eq!(codex_item["kind"], "agent.codex");
    assert_eq!(codex_item["live"], true);
    // Live Codex session ranks first and is the selected item.
    assert_eq!(feed["selectedItemId"], "codex:feedmerge");
    // CONTRACT: Codex rows land in `items`, never the legacy `agents` projection.
    let agents = feed["agents"].as_array().expect("agents array");
    assert!(
        !agents
            .iter()
            .any(|agent| serde_json::to_string(agent).unwrap_or_default().contains("codex:feedmerge")),
        "Codex row must not be forced into the agents projection"
    );
    for forbidden in ["thread-secret", "item-secret", "turn-secret", "threadId", "sessionId"] {
        assert!(!body.contains(forbidden), "feed leaked {forbidden}: {body}");
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}

// ---------------------------------------------------------------------------
// Claude CLI stream-json interactive-session routes (Phase 4)
//
// CONTRACT: mirrors the Codex route-test pattern above. Sessions are seeded
// into a shared `ClaudeProviderRegistry` via `insert_session_for_tests`
// (backed by an in-process scripted stream-json peer) so the write/read
// wiring, request parsing, status-code mapping, LOCAL_SERVER_ID short-circuit
// vs forward branch, and projection privacy are exercised without spawning a
// real `claude` binary.
// ---------------------------------------------------------------------------

/// Spawn an in-process stream-json peer that answers every `user`-typed line
/// with an `assistant` text event followed by a terminal `result` event, and
/// answers `control_request` lines with a matching `control_response`.
/// Returns the client-side `ClaudeConnection`.
fn spawn_claude_reply_peer(assistant_text: &str) -> Arc<ClaudeConnection> {
    use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};

    let (client_side, mut server_side) = tokio::io::duplex(8192);
    let (client_read, client_write) = tokio::io::split(client_side);
    let (connection, _events) =
        ClaudeConnection::from_io(client_read, client_write, Duration::from_secs(5));
    let assistant_text = assistant_text.to_owned();
    tokio::spawn(async move {
        let (server_read, mut server_write) = tokio::io::split(&mut server_side);
        let mut lines = BufReader::new(server_read).lines();
        while let Ok(Some(line)) = lines.next_line().await {
            let Ok(message) = serde_json::from_str::<serde_json::Value>(&line) else {
                continue;
            };
            match message.get("type").and_then(|value| value.as_str()) {
                Some("user") => {
                    let assistant = serde_json::json!({
                        "type": "assistant",
                        "message": {
                            "role": "assistant",
                            "content": [{"type": "text", "text": assistant_text}],
                        },
                    });
                    let result = serde_json::json!({"type": "result", "subtype": "success"});
                    let mut reply = serde_json::to_string(&assistant).expect("serialize assistant");
                    reply.push('\n');
                    reply.push_str(&serde_json::to_string(&result).expect("serialize result"));
                    reply.push('\n');
                    if server_write.write_all(reply.as_bytes()).await.is_err() {
                        break;
                    }
                    let _ = server_write.flush().await;
                }
                Some("control_request") => {
                    let request_id = message
                        .get("request_id")
                        .and_then(|value| value.as_str())
                        .unwrap_or_default()
                        .to_owned();
                    let response = serde_json::json!({
                        "type": "control_response",
                        "response": {"subtype": "success", "request_id": request_id},
                    });
                    let mut reply = serde_json::to_string(&response).expect("serialize control response");
                    reply.push('\n');
                    if server_write.write_all(reply.as_bytes()).await.is_err() {
                        break;
                    }
                    let _ = server_write.flush().await;
                }
                _ => {}
            }
        }
    });
    connection
}

#[tokio::test]
async fn claude_session_prompt_and_transcript_round_trip_local() {
    let registry = ClaudeProviderRegistry::default();
    // Pre-populate the projector so the transcript route has projected content.
    let mut projector = ClaudeProjector::new();
    projector.ingest_line(
        r#"{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello from claude"}]}}"#,
    );
    let connection = spawn_claude_reply_peer("ack");
    registry
        .insert_session_for_tests(
            "server-local",
            "claude:roundtrip",
            WorkRootId::from("claude-wr"),
            "019f5040-secret-session-id",
            PathBuf::from("/tmp/claude-route-test-cwd"),
            connection,
            projector,
        )
        .expect("seed claude session");

    let mut state = app_state();
    state.claude_sessions = registry.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    // POST prompt -> the scripted peer acknowledges with an assistant + result.
    let prompt_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/claude-wr/activity/claude-sessions/claude:roundtrip/prompt")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "text": "do a thing" }).to_string(),
                ))
                .expect("prompt request"),
        )
        .await
        .expect("prompt response");
    assert_eq!(prompt_response.status(), StatusCode::OK);
    let prompt_body = axum::body::to_bytes(prompt_response.into_body(), 16 * 1024)
        .await
        .expect("prompt body bytes");
    let prompt_json: serde_json::Value =
        serde_json::from_slice(&prompt_body).expect("prompt JSON");
    assert_eq!(prompt_json["accepted"], true);

    // GET transcript -> projected blocks; no provider ids/paths.
    let (status, body) = fetch_work_root_activity_path(
        app,
        cookie.as_str(),
        "/api/dashboard/work-roots/claude-wr/activity/claude-sessions/claude:roundtrip/transcript",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let transcript: serde_json::Value = serde_json::from_str(&body).expect("transcript JSON");
    assert_eq!(transcript["activityId"], "claude:roundtrip");
    assert!(body.contains("hello from claude"), "projected block text missing: {body}");
    for forbidden in [
        "019f5040-secret-session-id",
        "claude-route-test-cwd",
        "sessionId",
        "cwd",
    ] {
        assert!(!body.contains(forbidden), "transcript leaked {forbidden}: {body}");
    }
}

/// Phase 4 (`260713-feat-ws-dashboard-agent-chat-real-adapter-wiring`)
/// coverage gap, Claude counterpart to
/// `codex_session_send_receive_multi_poll_e2e`. Drives a real multi-poll
/// `live: true -> live: false` transition through
/// `claude_activity_transcript` by locking the held `Arc<ClaudeSession>`
/// handle's projector between polls, using lines already captured and
/// validated from a real spawned `claude` binary in
/// `ws-dashboard/crates/core/tests/fixtures/claude-cli-turn.ndjson` (see
/// `claude_projection.rs`'s test suite) rather than hand-writing new
/// stream-json — per `ClaudeProjector::ingest_result`
/// (`claude_projection.rs:299-301`), only a `result`-typed event flips
/// `is_turn_active()` to `false`; an `assistant` event flips it to `true`.
#[tokio::test]
async fn claude_session_send_receive_multi_poll_e2e() {
    // Real-capture fixture lines (see module doc comment above): index 0 is
    // `system/init` (ignored, protocol-control), 1 is the first turn's
    // `assistant` "HELLO" text, 3 is that turn's terminal `result`, 4 is a
    // second turn's `system/init`, 5-6 are two `assistant` events (text, then
    // a `tool_use`), 7 is the matching `user` `tool_result`.
    let fixture_lines: Vec<&str> = include_str!("../../core/tests/fixtures/claude-cli-turn.ndjson")
        .lines()
        .filter(|line| !line.trim().is_empty())
        .collect();
    assert!(fixture_lines.len() >= 8, "claude-cli-turn.ndjson fixture shrank under the lines this test indexes");

    let registry = ClaudeProviderRegistry::default();
    // Seed the projector with the fixture's first `assistant` event so the
    // session starts mid-turn (live, one block) right after the prompt was
    // accepted — mirroring the Codex counterpart's `turn/started` pre-seed.
    let mut projector = ClaudeProjector::new();
    projector.ingest_line(fixture_lines[1]);
    let connection = spawn_claude_reply_peer("ack");
    let session = registry
        .insert_session_for_tests(
            "server-local",
            "claude:multipoll",
            WorkRootId::from("claude-wr"),
            "019f5040-secret-session-id",
            PathBuf::from("/tmp/claude-route-multipoll-cwd"),
            connection,
            projector,
        )
        .expect("seed claude session");

    let mut state = app_state();
    state.claude_sessions = registry.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    // POST prompt -> the scripted peer acknowledges with an assistant + result
    // (irrelevant to the projector state driven directly below).
    let prompt_response = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/claude-wr/activity/claude-sessions/claude:multipoll/prompt")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(
                    serde_json::json!({ "text": "do a multi-poll thing" }).to_string(),
                ))
                .expect("prompt request"),
        )
        .await
        .expect("prompt response");
    assert_eq!(prompt_response.status(), StatusCode::OK);

    // Poll #1: the seeded `assistant` event makes the session live with one
    // block, no terminal `result` observed yet.
    let (status, body) = fetch_work_root_activity_path(
        app.clone(),
        cookie.as_str(),
        "/api/dashboard/work-roots/claude-wr/activity/claude-sessions/claude:multipoll/transcript",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let transcript: serde_json::Value = serde_json::from_str(&body).expect("poll #1 transcript JSON");
    assert_eq!(transcript["live"], true, "poll #1: assistant event seeded, no result event ingested yet");
    assert_eq!(
        transcript["blocks"].as_array().expect("poll #1 blocks array").len(),
        1,
        "poll #1: one assistant block ingested"
    );
    assert!(body.contains("HELLO"), "poll #1 missing seeded assistant text: {body}");

    // Between poll #1 and #2: the harness pushes further mid-turn progress
    // (a second assistant text block, then a tool_use block) from the
    // fixture's second captured turn.
    {
        let projector_handle = session.projector();
        let mut projector = projector_handle.lock().await;
        projector.ingest_line(fixture_lines[5]);
        projector.ingest_line(fixture_lines[6]);
    }

    // Poll #2: still live, now has the two additional mid-turn blocks.
    let (status, body) = fetch_work_root_activity_path(
        app.clone(),
        cookie.as_str(),
        "/api/dashboard/work-roots/claude-wr/activity/claude-sessions/claude:multipoll/transcript",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let transcript: serde_json::Value = serde_json::from_str(&body).expect("poll #2 transcript JSON");
    assert_eq!(transcript["live"], true, "poll #2: no result event ingested yet");
    let blocks = transcript["blocks"].as_array().expect("poll #2 blocks array");
    assert_eq!(blocks.len(), 3, "poll #2: two mid-turn blocks ingested since poll #1 (text + tool_use)");

    // Between poll #2 and #3: the harness delivers the matching tool_result
    // (updates the existing tool block in place, adds no new block) then the
    // terminal `result` event that flips `is_turn_active` to false.
    {
        let projector_handle = session.projector();
        let mut projector = projector_handle.lock().await;
        projector.ingest_line(fixture_lines[7]);
        projector.ingest_line(fixture_lines[3]);
    }

    // Poll #3: terminal `result` observed -> live:false, final block count
    // unchanged by the tool_result (it updates the existing tool block).
    let (status, body) = fetch_work_root_activity_path(
        app,
        cookie.as_str(),
        "/api/dashboard/work-roots/claude-wr/activity/claude-sessions/claude:multipoll/transcript",
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    let transcript: serde_json::Value = serde_json::from_str(&body).expect("poll #3 transcript JSON");
    assert_eq!(transcript["live"], false, "poll #3: the fixture's result event flips is_turn_active to false");
    let blocks = transcript["blocks"].as_array().expect("poll #3 blocks array");
    assert_eq!(blocks.len(), 3, "poll #3: tool_result updates the existing tool block, adds no new one");
    for forbidden in [
        "019f5040-secret-session-id",
        "claude-route-multipoll-cwd",
        "sessionId",
        "cwd",
        "a1b2c3d4-1111-4222-8333-444455556666",
        "toolu_01UbL6NLnGbxxSosoEzMaG4a",
    ] {
        assert!(!body.contains(forbidden), "multi-poll transcript leaked {forbidden}: {body}");
    }
}

#[tokio::test]
async fn claude_session_prompt_unknown_session_maps_not_found() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let response = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/work-roots/claude-wr/activity/claude-sessions/claude:missing/prompt")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(serde_json::json!({ "text": "hi" }).to_string()))
                .expect("prompt request"),
        )
        .await
        .expect("prompt response");
    // provider_error_response maps claude.unknown_session -> 404.
    assert_eq!(response.status(), StatusCode::NOT_FOUND);
    let body = axum::body::to_bytes(response.into_body(), 8 * 1024)
        .await
        .expect("error body bytes");
    let value: serde_json::Value = serde_json::from_slice(&body).expect("error JSON");
    assert_eq!(value["code"], "claude.unknown_session");
}

#[tokio::test]
async fn server_scoped_claude_prompt_short_circuits_local_and_forwards_remote() {
    let registry = ClaudeProviderRegistry::default();
    let connection = spawn_claude_reply_peer("ack");
    registry
        .insert_session_for_tests(
            "server-local",
            "claude:scoped",
            WorkRootId::from("claude-wr"),
            "019f5040-secret-session-id",
            PathBuf::from("/tmp/claude-route-test-cwd"),
            connection,
            ClaudeProjector::new(),
        )
        .expect("seed claude session");

    let mut state = app_state();
    state.claude_sessions = registry.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    // LOCAL_SERVER_ID short-circuits to the in-process local handler.
    let local = app
        .clone()
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/server-local/work-roots/claude-wr/activity/claude-sessions/claude:scoped/prompt")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(serde_json::json!({ "text": "hi" }).to_string()))
                .expect("local prompt request"),
        )
        .await
        .expect("local prompt response");
    assert_eq!(local.status(), StatusCode::OK);

    // A non-local, unlinked server takes the forward branch and is refused.
    let forwarded = app
        .oneshot(
            Request::builder()
                .method(Method::POST)
                .uri("/api/dashboard/servers/does-not-exist/work-roots/claude-wr/activity/claude-sessions/claude:scoped/prompt")
                .header(header::COOKIE, cookie.as_str())
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(serde_json::json!({ "text": "hi" }).to_string()))
                .expect("forward prompt request"),
        )
        .await
        .expect("forward prompt response");
    assert_eq!(forwarded.status(), StatusCode::NOT_FOUND);
}

#[tokio::test]
async fn work_root_activity_route_merges_claude_session_into_items() {
    if skip_without_git("work_root_activity_route_merges_claude_session_into_items") {
        return;
    }
    let root = temp_fixture_path("work-root-activity-claude-merge");
    let cache_home = temp_fixture_path("work-root-activity-claude-merge-cache");
    fs::create_dir_all(&root).expect("create workRoot");
    init_git_repo(&root);

    let registry = ClaudeProviderRegistry::default();
    let mut state = app_state_with_activity_cache_home(cache_home.clone());
    state.claude_sessions = registry.clone();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;
    let work_root_id = open_work_root_for_test(app.clone(), cookie.as_str(), &root).await;

    // Seed a live Claude session for the opened work root.
    let mut projector = ClaudeProjector::new();
    projector.ingest_line(
        r#"{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"claude says hi"}]}}"#,
    );
    let connection = spawn_claude_reply_peer("ack");
    registry
        .insert_session_for_tests(
            "server-local",
            "claude:feedmerge",
            WorkRootId::from(work_root_id.clone()),
            "019f5040-secret-session-id",
            PathBuf::from("/tmp/claude-route-test-cwd"),
            connection,
            projector,
        )
        .expect("seed claude session");

    let (status, body) = fetch_work_root_activity(app, cookie.as_str(), &work_root_id).await;
    assert_eq!(status, StatusCode::OK);
    let feed: serde_json::Value = serde_json::from_str(&body).expect("feed JSON");
    let items = feed["items"].as_array().expect("items array");
    let claude_item = items
        .iter()
        .find(|item| item["id"] == "claude:feedmerge")
        .expect("Claude session must appear in the unified feed items");
    assert_eq!(claude_item["kind"], "agent.claude");
    // CONTRACT: Claude rows land in `items`, never the legacy `agents` projection.
    let agents = feed["agents"].as_array().expect("agents array");
    assert!(
        !agents
            .iter()
            .any(|agent| serde_json::to_string(agent).unwrap_or_default().contains("claude:feedmerge")),
        "Claude row must not be forced into the agents projection"
    );
    for forbidden in ["019f5040-secret-session-id", "claude-route-test-cwd", "sessionId"] {
        assert!(!body.contains(forbidden), "feed leaked {forbidden}: {body}");
    }

    remove_static_fixture(&root);
    remove_static_fixture(&cache_home);
}
