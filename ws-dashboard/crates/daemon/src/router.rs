use std::collections::HashMap;
use std::path::{Component, PathBuf};
use std::sync::Arc;

use axum::extract::{Path as AxumPath, Query, Request, State};
use axum::http::{header, HeaderMap, StatusCode};
use axum::middleware::{from_fn_with_state, Next};
use axum::response::{Html, IntoResponse, Response};
use axum::routing::{delete, get, post};
use axum::Router;
use tokio::fs;
use tokio::sync::Mutex;

use crate::auth::{OwnerAuthState, PairingOutcome};
use crate::config::ServeConfig;
use crate::document_translation::{
    translate_document, translation_providers, DocumentTranslationService,
};
use crate::events::instance_events;
use crate::git_toolbar::{
    git_branches, git_create_branch, git_fetch, git_pull_ff_only, git_push, git_status,
    git_switch_branch,
};
use crate::git_worktree::{
    git_worktree_add_options, git_worktree_add_preview, git_worktree_add_submit,
};
use crate::persistent_state::DashboardStateStore;
use crate::resources::dashboard_resources;
use crate::root_picker::{
    create_empty_directory, list_root_picker, open_work_root, pin_root_picker_directory,
    remove_workspace, set_work_root_activation, unpin_root_picker_directory,
};
use crate::servers::{
    dashboard_server_resources, dashboard_servers, link_dashboard_server, link_endpoint_server,
    reconnect_dashboard_server_tunnel, remote_link_auth, server_scoped_close_terminal,
    server_scoped_create_empty_directory, server_scoped_document_events,
    server_scoped_git_branches, server_scoped_git_fetch, server_scoped_git_pull_ff_only,
    server_scoped_git_push, server_scoped_git_status, server_scoped_git_switch_branch,
    server_scoped_git_worktree_add_options, server_scoped_git_worktree_add_preview,
    server_scoped_git_worktree_add_submit, server_scoped_open_work_root,
    server_scoped_read_work_root_file, server_scoped_remove_workspace, server_scoped_root_picker,
    server_scoped_root_picker_pins, server_scoped_set_work_root_activation,
    server_scoped_terminal_input, server_scoped_terminal_output, server_scoped_terminal_resize,
    server_scoped_terminals, server_scoped_work_root_activity,
    server_scoped_work_root_activity_events, server_scoped_work_root_activity_transcript,
    server_scoped_work_root_files, server_scoped_write_work_root_file, start_ssh_dashboard_server,
    LinkedServerSessions, LinkedServerTunnels,
};
use crate::terminal::{
    close_terminal, create_terminal, list_terminals, terminal_input, terminal_output,
    terminal_resize, terminal_websocket, TerminalRegistry,
};
use crate::work_root_activity::{
    work_root_activity, work_root_activity_events, work_root_activity_transcript,
    WorkRootActivityProjector,
};
use crate::work_root_files::{
    document_events, list_work_root_files, read_work_root_file, write_work_root_file,
    DocumentEventHub, DocumentWriteLocks, OpenedWorkRoots,
};

#[derive(Clone)]
pub struct AppState {
    pub config: ServeConfig,
    pub auth: OwnerAuthState,
    pub opened_work_roots: OpenedWorkRoots,
    pub dashboard_state: DashboardStateStore,
    pub document_translation: DocumentTranslationService,
    pub terminals: TerminalRegistry,
    pub work_root_activity: WorkRootActivityProjector,
    pub document_events: DocumentEventHub,
    pub document_write_locks: DocumentWriteLocks,
    pub linked_server_sessions: LinkedServerSessions,
    pub linked_server_tunnels: LinkedServerTunnels,
    pub registry_persist_lock: Arc<Mutex<()>>,
}

pub fn build_router(state: AppState) -> Router {
    // CONTRACT: `/pair` and daemon-to-daemon link auth stay outside the
    // protected browser router.
    // CONTRACT: `/healthz`, `/`, static UI, and WebSocket upgrade routes are
    // nested behind one central auth boundary when owner auth is enabled. The
    // loopback-only no-auth debug profile omits that layer for the whole
    // protected router so handlers remain oblivious to serving mode.
    let protected = Router::new()
        .route("/healthz", get(healthz))
        .route("/api/dashboard/resources", get(dashboard_resources))
        .route("/api/dashboard/servers", get(dashboard_servers))
        .route(
            "/api/dashboard/servers/ssh/start",
            post(start_ssh_dashboard_server),
        )
        .route("/api/dashboard/servers/link", post(link_endpoint_server))
        .route(
            "/api/dashboard/servers/{server_route}/resources",
            get(dashboard_server_resources),
        )
        .route(
            "/api/dashboard/servers/{server_route}/link-auth",
            post(link_dashboard_server),
        )
        .route(
            "/api/dashboard/servers/{server_route}/tunnel/reconnect",
            post(reconnect_dashboard_server_tunnel),
        )
        .route(
            "/api/dashboard/servers/{server_route}/root-picker",
            get(server_scoped_root_picker),
        )
        .route(
            "/api/dashboard/servers/{server_route}/root-picker/directories",
            post(server_scoped_create_empty_directory),
        )
        .route(
            "/api/dashboard/servers/{server_route}/root-picker/pins",
            post(server_scoped_root_picker_pins).delete(server_scoped_root_picker_pins),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/open",
            post(server_scoped_open_work_root),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/activation",
            post(server_scoped_set_work_root_activation),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/files",
            get(server_scoped_work_root_files),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/files/read",
            get(server_scoped_read_work_root_file),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/files/write",
            post(server_scoped_write_work_root_file),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/documents/events",
            get(server_scoped_document_events),
        )
        .route(
            "/api/dashboard/servers/{server_route}/workspaces/{workspace_id}",
            delete(server_scoped_remove_workspace),
        )
        .route(
            "/api/dashboard/servers/{server_route}/workspaces/{workspace_id}/git-worktree-add/options",
            get(server_scoped_git_worktree_add_options),
        )
        .route(
            "/api/dashboard/servers/{server_route}/workspaces/{workspace_id}/git-worktree-add/preview",
            post(server_scoped_git_worktree_add_preview),
        )
        .route(
            "/api/dashboard/servers/{server_route}/workspaces/{workspace_id}/git-worktree-add",
            post(server_scoped_git_worktree_add_submit),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/activity",
            get(server_scoped_work_root_activity),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/activity/items/{activity_id}/transcript",
            get(server_scoped_work_root_activity_transcript),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/activity/events",
            get(server_scoped_work_root_activity_events),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/git/status",
            get(server_scoped_git_status),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/git/branches",
            get(server_scoped_git_branches).post(server_scoped_git_branches),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/git/switch-branch",
            post(server_scoped_git_switch_branch),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/git/fetch",
            post(server_scoped_git_fetch),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/git/push",
            post(server_scoped_git_push),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/git/pull-ff-only",
            post(server_scoped_git_pull_ff_only),
        )
        .route(
            "/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/terminals",
            get(server_scoped_terminals).post(server_scoped_terminals),
        )
        .route(
            "/api/dashboard/servers/{server_route}/terminals/{terminal_id}/output",
            get(server_scoped_terminal_output),
        )
        .route(
            "/api/dashboard/servers/{server_route}/terminals/{terminal_id}/input",
            post(server_scoped_terminal_input),
        )
        .route(
            "/api/dashboard/servers/{server_route}/terminals/{terminal_id}/resize",
            post(server_scoped_terminal_resize),
        )
        .route(
            "/api/dashboard/servers/{server_route}/terminals/{terminal_id}",
            delete(server_scoped_close_terminal),
        )
        .route(
            "/api/dashboard/document-translation/providers",
            get(translation_providers),
        )
        .route(
            "/api/dashboard/document-translation/translate",
            post(translate_document),
        )
        .route(
            "/api/dashboard/instance-events/{stream_id}",
            get(instance_events),
        )
        .route("/api/dashboard/root-picker", get(list_root_picker))
        .route(
            "/api/dashboard/root-picker/directories",
            post(create_empty_directory),
        )
        .route(
            "/api/dashboard/root-picker/pins",
            post(pin_root_picker_directory).delete(unpin_root_picker_directory),
        )
        .route("/api/dashboard/work-roots/open", post(open_work_root))
        .route(
            "/api/dashboard/workspaces/{workspace_id}",
            delete(remove_workspace),
        )
        .route(
            "/api/dashboard/workspaces/{workspace_id}/git-worktree-add/options",
            get(git_worktree_add_options),
        )
        .route(
            "/api/dashboard/workspaces/{workspace_id}/git-worktree-add/preview",
            post(git_worktree_add_preview),
        )
        .route(
            "/api/dashboard/workspaces/{workspace_id}/git-worktree-add",
            post(git_worktree_add_submit),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/activation",
            post(set_work_root_activation),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/git/status",
            get(git_status),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/git/branches",
            get(git_branches).post(git_create_branch),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/git/switch-branch",
            post(git_switch_branch),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/git/fetch",
            post(git_fetch),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/git/push",
            post(git_push),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/git/pull-ff-only",
            post(git_pull_ff_only),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/terminals",
            get(list_terminals).post(create_terminal),
        )
        .route(
            "/api/dashboard/terminals/{terminal_id}/output",
            get(terminal_output),
        )
        .route(
            "/api/dashboard/terminals/{terminal_id}/input",
            post(terminal_input),
        )
        .route(
            "/api/dashboard/terminals/{terminal_id}/resize",
            post(terminal_resize),
        )
        .route(
            "/api/dashboard/terminals/{terminal_id}/socket",
            get(terminal_websocket),
        )
        .route(
            "/api/dashboard/terminals/{terminal_id}",
            axum::routing::delete(close_terminal),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/files",
            get(list_work_root_files),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/files/read",
            get(read_work_root_file),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/files/write",
            post(write_work_root_file),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/documents/events",
            get(document_events),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/activity",
            get(work_root_activity),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/activity/items/{activity_id}/transcript",
            get(work_root_activity_transcript),
        )
        .route(
            "/api/dashboard/work-roots/{work_root_id}/activity/events",
            get(work_root_activity_events),
        )
        .route("/assets/{*asset_path}", get(static_asset))
        .route("/servers", get(index))
        .route("/servers/{*app_path}", get(index))
        .route("/", get(index))
        .fallback(not_found);

    let protected = if state.config.owner_auth_enabled {
        protected.layer(from_fn_with_state(state.clone(), require_owner_auth))
    } else {
        protected
    };

    Router::new()
        .route("/pair", get(pair))
        .route("/api/dashboard/link-auth", post(remote_link_auth))
        .merge(protected)
        .with_state(state)
}

async fn pair(
    State(state): State<AppState>,
    Query(query): Query<HashMap<String, String>>,
) -> Response {
    let Some(token) = query.get("token") else {
        return (StatusCode::BAD_REQUEST, "missing pairing token\n").into_response();
    };

    match state.auth.consume_pairing_token(token) {
        PairingOutcome::Paired => {
            let cookie = state.auth.issue_session_cookie().as_set_cookie_header();
            (
                StatusCode::SEE_OTHER,
                [
                    (header::SET_COOKIE, cookie),
                    (header::LOCATION, "/".to_owned()),
                ],
            )
                .into_response()
        }
        PairingOutcome::Invalid => {
            (StatusCode::UNAUTHORIZED, "invalid pairing token\n").into_response()
        }
        PairingOutcome::AlreadyUsed => {
            (StatusCode::GONE, "pairing token already used\n").into_response()
        }
        PairingOutcome::Expired => {
            (StatusCode::UNAUTHORIZED, "pairing token expired\n").into_response()
        }
    }
}

async fn require_owner_auth(
    State(state): State<AppState>,
    headers: HeaderMap,
    request: Request,
    next: Next,
) -> Response {
    let auth_result = match headers
        .get(header::UPGRADE)
        .and_then(|value| value.to_str().ok())
    {
        Some(value) if value.eq_ignore_ascii_case("websocket") => {
            state.auth.authenticate_websocket_upgrade(&headers)
        }
        _ => state.auth.authenticate_browser_entrypoint(&headers),
    };

    if let Err(rejection) = auth_result {
        return rejection.status_code().into_response();
    }

    next.run(request).await
}

async fn healthz() -> Response {
    (StatusCode::OK, "ok\n").into_response()
}

async fn index(State(state): State<AppState>) -> Response {
    if let Some(static_dir) = state.config.static_dir.as_deref() {
        return serve_static_file(static_dir.join("index.html"), "text/html; charset=utf-8").await;
    }

    Html("<!doctype html><title>ws dashboard</title><main>ws dashboard daemon</main>\n")
        .into_response()
}

async fn static_asset(
    State(state): State<AppState>,
    AxumPath(asset_path): AxumPath<String>,
) -> Response {
    let Some(static_dir) = state.config.static_dir.as_deref() else {
        return not_found().await;
    };

    let Some(asset_path) = safe_relative_path(&asset_path) else {
        return StatusCode::FORBIDDEN.into_response();
    };

    serve_static_file(
        static_dir.join("assets").join(&asset_path),
        content_type_for_asset(&asset_path),
    )
    .await
}

async fn not_found() -> Response {
    StatusCode::NOT_FOUND.into_response()
}

async fn serve_static_file(path: PathBuf, content_type: &'static str) -> Response {
    match fs::read(path).await {
        Ok(body) => ([(header::CONTENT_TYPE, content_type)], body).into_response(),
        Err(_) => StatusCode::NOT_FOUND.into_response(),
    }
}

fn safe_relative_path(path: &str) -> Option<PathBuf> {
    let mut safe = PathBuf::new();
    for component in PathBuf::from(path).components() {
        match component {
            Component::Normal(part) => safe.push(part),
            _ => return None,
        }
    }

    (!safe.as_os_str().is_empty()).then_some(safe)
}

fn content_type_for_asset(path: &PathBuf) -> &'static str {
    match path.extension().and_then(|extension| extension.to_str()) {
        Some("css") => "text/css; charset=utf-8",
        Some("js") => "application/javascript; charset=utf-8",
        Some("svg") => "image/svg+xml",
        _ => "application/octet-stream",
    }
}
