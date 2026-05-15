use axum::Router;

use crate::auth::OwnerAuthState;
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
    // HINT: Prefer Axum middleware/layer composition that route tests can call
    // directly without binding a socket.
    let _ = state;
    todo!("build auth-gated dashboard router")
}
