use std::collections::HashMap;

use axum::extract::{Query, Request, State};
use axum::http::{header, HeaderMap, StatusCode};
use axum::middleware::{from_fn_with_state, Next};
use axum::response::{Html, IntoResponse, Response};
use axum::routing::get;
use axum::Router;

use crate::auth::{OwnerAuthState, PairingOutcome};
use crate::config::ServeConfig;

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
    if let Err(status) = state.auth.authenticate_headers(&headers) {
        return status.into_response();
    }

    next.run(request).await
}

async fn healthz() -> Response {
    (StatusCode::OK, "ok\n").into_response()
}

async fn index() -> Response {
    Html("<!doctype html><title>ws dashboard</title><main>ws dashboard daemon</main>\n")
        .into_response()
}

async fn not_found() -> Response {
    StatusCode::NOT_FOUND.into_response()
}
