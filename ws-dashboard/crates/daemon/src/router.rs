use std::collections::HashMap;
use std::path::{Component, PathBuf};

use axum::extract::{Path as AxumPath, Query, Request, State};
use axum::http::{header, HeaderMap, StatusCode};
use axum::middleware::{from_fn_with_state, Next};
use axum::response::{Html, IntoResponse, Response};
use axum::routing::{get, post};
use axum::Router;
use tokio::fs;

use crate::auth::{OwnerAuthState, PairingOutcome};
use crate::config::ServeConfig;
use crate::resources::dashboard_resources;
use crate::root_picker::{create_empty_directory, list_root_picker, open_work_root};

#[derive(Clone)]
pub struct AppState {
    pub config: ServeConfig,
    pub auth: OwnerAuthState,
}

pub fn build_router(state: AppState) -> Router {
    // CONTRACT: `/pair` is the only unauthenticated browser route.
    // CONTRACT: `/healthz`, `/`, static UI, and future WebSocket upgrade routes
    // are nested behind owner-session authentication.
    let protected = Router::new()
        .route("/healthz", get(healthz))
        .route("/api/dashboard/resources", get(dashboard_resources))
        .route("/api/dashboard/root-picker", get(list_root_picker))
        .route(
            "/api/dashboard/root-picker/directories",
            post(create_empty_directory),
        )
        .route("/api/dashboard/work-roots/open", post(open_work_root))
        .route("/assets/{*asset_path}", get(static_asset))
        .route("/", get(index))
        .fallback(not_found)
        .layer(from_fn_with_state(state.clone(), require_owner_auth));

    Router::new()
        .route("/pair", get(pair))
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
            ([(header::SET_COOKIE, cookie)], "paired\n").into_response()
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
