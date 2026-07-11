//! HTTP routes for Claude CLI interactive sessions (Phase 4 write path).
//!
//! These are the local (`server-local`) handlers; the server-scoped wrappers
//! in `servers.rs` short-circuit here for `LOCAL_SERVER_ID` and forward
//! otherwise, matching the `codex_routes.rs` pattern so routing keys by
//! `serverId` (ticket Constraints #L550-L561).
//!
//! CONTRACT: only the dashboard-owned `activityId` and projected Activity
//! content cross the browser boundary. Provider session ids and cwd paths
//! never appear in a response body (see `claude_cli` privacy test).

use std::path::PathBuf;
use std::sync::Arc;

use axum::extract::{Path as AxumPath, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::{Deserialize, Serialize};
use ws_dashboard_core::agent_client_provider::{
    AgentClientInterruptRequest, AgentClientProvider, AgentClientProviderError,
    AgentClientPromptSendRequest, AgentClientSessionCreateRequest, AgentClientSessionListRequest,
};
use ws_dashboard_core::WorkRootId;

use crate::claude_cli::{
    claude_activity_transcript, ClaudeCliProvider, ClaudeProviderRegistry, ClaudeSpawnConfig,
    ClaudeWorkRootResolver,
};
use crate::router::AppState;
use crate::work_root_files::resolve_online_available_work_root;

/// The local server id (matches `servers.rs::LOCAL_SERVER_ID`).
pub const LOCAL_SERVER_ID: &str = "server-local";

struct AppStateResolver {
    state: AppState,
}

impl ClaudeWorkRootResolver for AppStateResolver {
    fn resolve_cwd(&self, work_root_id: &WorkRootId) -> Result<PathBuf, AgentClientProviderError> {
        resolve_online_available_work_root(&self.state, work_root_id).map_err(|error| {
            AgentClientProviderError {
                code: "claude.work_root".to_owned(),
                message: error.message().to_owned(),
            }
        })
    }
}

fn local_provider(state: &AppState, registry: ClaudeProviderRegistry) -> ClaudeCliProvider {
    ClaudeCliProvider::new(
        ClaudeSpawnConfig::new(LOCAL_SERVER_ID),
        registry,
        Arc::new(AppStateResolver {
            state: state.clone(),
        }),
    )
}

fn provider_error_response(error: AgentClientProviderError) -> Response {
    let status = match error.code.as_str() {
        "claude.plugin_gate" => StatusCode::FAILED_DEPENDENCY,
        "claude.work_root" => StatusCode::CONFLICT,
        "claude.unknown_session" => StatusCode::NOT_FOUND,
        "claude.too_many_sessions" => StatusCode::TOO_MANY_REQUESTS,
        "claude.timeout" => StatusCode::GATEWAY_TIMEOUT,
        _ => StatusCode::BAD_GATEWAY,
    };
    (status, Json(error)).into_response()
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateClaudeSessionRequest {
    #[serde(default)]
    pub initial_prompt: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ClaudePromptRequest {
    pub text: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ClaudeControlResponse {
    pub applied: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub data: Option<serde_json::Value>,
}

pub async fn create_claude_session(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Json(request): Json<CreateClaudeSessionRequest>,
) -> Response {
    let provider = local_provider(&state, state.claude_sessions.clone());
    match provider
        .create_session(AgentClientSessionCreateRequest {
            work_root_id: WorkRootId::from(work_root_id),
            initial_prompt: request.initial_prompt,
        })
        .await
    {
        Ok(result) => (StatusCode::CREATED, Json(result)).into_response(),
        Err(error) => provider_error_response(error),
    }
}

pub async fn list_claude_sessions(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    let provider = local_provider(&state, state.claude_sessions.clone());
    match provider
        .list_sessions(AgentClientSessionListRequest {
            work_root_id: WorkRootId::from(work_root_id),
        })
        .await
    {
        Ok(result) => Json(result).into_response(),
        Err(error) => provider_error_response(error),
    }
}

pub async fn claude_session_prompt(
    State(state): State<AppState>,
    AxumPath((_work_root_id, activity_id)): AxumPath<(String, String)>,
    Json(request): Json<ClaudePromptRequest>,
) -> Response {
    let provider = local_provider(&state, state.claude_sessions.clone());
    match provider
        .send_prompt(AgentClientPromptSendRequest {
            activity_id,
            text: request.text,
        })
        .await
    {
        Ok(result) => Json(result).into_response(),
        Err(error) => provider_error_response(error),
    }
}

pub async fn claude_session_interrupt(
    State(state): State<AppState>,
    AxumPath((_work_root_id, activity_id)): AxumPath<(String, String)>,
) -> Response {
    let provider = local_provider(&state, state.claude_sessions.clone());
    match provider
        .interrupt(AgentClientInterruptRequest { activity_id })
        .await
    {
        Ok(()) => StatusCode::NO_CONTENT.into_response(),
        Err(error) => provider_error_response(error),
    }
}

pub async fn claude_session_transcript(
    State(state): State<AppState>,
    AxumPath((work_root_id, activity_id)): AxumPath<(String, String)>,
) -> Response {
    // CONTRACT: a transcript read is not input (plan lifecycle: sessions are
    // "transparently respawned ... on the next input"), so this handler must
    // NOT call `resume_session`/`ensure_live` — that would respawn a child
    // for every poll of a killed/idle session. The in-memory projector
    // already holds the full accumulated transcript (Finding B lifecycle
    // consequence 1), so existence-check the session in the registry only,
    // without touching its connection.
    let Some(session) = state
        .claude_sessions
        .session_for(LOCAL_SERVER_ID, &activity_id)
    else {
        return provider_error_response(AgentClientProviderError {
            code: "claude.unknown_session".to_owned(),
            message: "unknown Claude session".to_owned(),
        });
    };
    let transcript = claude_activity_transcript(&session, WorkRootId::from(work_root_id)).await;
    Json(transcript).into_response()
}
