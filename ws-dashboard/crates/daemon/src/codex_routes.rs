//! HTTP routes for Codex app-server interactive sessions (Phase 2 write path).
//!
//! These are the local (`server-local`) handlers; the server-scoped wrappers in
//! `servers.rs` short-circuit here for `LOCAL_SERVER_ID` and forward otherwise,
//! matching the terminal/activity route pattern so routing keys by `serverId`
//! (ticket Constraints #L550-L561).
//!
//! CONTRACT: only the dashboard-owned `activityId` and projected Activity
//! content cross the browser boundary. Provider thread/turn ids and session
//! paths never appear in a response body (see `codex_app_server` privacy test).

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
    AgentClientSessionResumeRequest,
};
use ws_dashboard_core::WorkRootId;

use crate::codex_app_server::{
    codex_activity_transcript, CodexAppServerProvider, CodexProviderRegistry, CodexSpawnConfig,
    CodexWorkRootResolver,
};
use crate::router::AppState;
use crate::work_root_files::resolve_online_available_work_root;

/// The local server id (matches `servers.rs::LOCAL_SERVER_ID`).
pub const LOCAL_SERVER_ID: &str = "server-local";

struct AppStateResolver {
    state: AppState,
}

impl CodexWorkRootResolver for AppStateResolver {
    fn resolve_cwd(&self, work_root_id: &WorkRootId) -> Result<PathBuf, AgentClientProviderError> {
        resolve_online_available_work_root(&self.state, work_root_id).map_err(|error| {
            AgentClientProviderError {
                code: "codex.work_root".to_owned(),
                message: error.message().to_owned(),
            }
        })
    }
}

fn local_provider(state: &AppState, registry: CodexProviderRegistry) -> CodexAppServerProvider {
    CodexAppServerProvider::new(
        CodexSpawnConfig::new(LOCAL_SERVER_ID),
        registry,
        Arc::new(AppStateResolver {
            state: state.clone(),
        }),
    )
}

fn provider_error_response(error: AgentClientProviderError) -> Response {
    let status = match error.code.as_str() {
        "codex.plugin_gate" => StatusCode::FAILED_DEPENDENCY,
        "codex.work_root" => StatusCode::CONFLICT,
        "codex.unknown_session" => StatusCode::NOT_FOUND,
        "codex.too_many_sessions" => StatusCode::TOO_MANY_REQUESTS,
        "codex.steer_turn_mismatch" => StatusCode::CONFLICT,
        "codex.timeout" => StatusCode::GATEWAY_TIMEOUT,
        _ => StatusCode::BAD_GATEWAY,
    };
    (status, Json(error)).into_response()
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateCodexSessionRequest {
    #[serde(default)]
    pub initial_prompt: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CodexPromptRequest {
    pub text: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", tag = "action")]
pub enum CodexControlRequest {
    Compact,
    Steer { text: String },
    Skills,
    // `goal`/`rewind` deliberately unwired this phase: `capabilities()`
    // reports `goal: true` but no adapter method/route backs it yet, and
    // `rewind` stays scaffolded-disabled per the 260711 Phase 3 precedent
    // (see plan `260713`... Phase 3 Out of Scope).
    Fork {
        // CONTRACT: `#[serde(rename_all = "camelCase")]` on the enum only
        // renames the `action` tag values (`fork`), not fields nested inside
        // a struct variant — verified via a local repro against this
        // project's pinned serde 1.0.228 (serde's own docs are explicit that
        // container `rename_all` on an enum governs variant names, not their
        // fields; `rename_all_fields` is the container attribute for the
        // latter). Without this per-field rename the wire's `cutCursor` key
        // silently fails to populate `cut_cursor` (defaults to `None`)
        // instead of erroring, since the field is optional.
        #[serde(rename = "cutCursor", default)]
        cut_cursor: Option<String>,
    },
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CodexControlResponse {
    pub applied: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub data: Option<serde_json::Value>,
}

pub async fn create_codex_session(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Json(request): Json<CreateCodexSessionRequest>,
) -> Response {
    let provider = local_provider(&state, state.codex_sessions.clone());
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

pub async fn list_codex_sessions(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    let provider = local_provider(&state, state.codex_sessions.clone());
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

pub async fn codex_session_prompt(
    State(state): State<AppState>,
    AxumPath((_work_root_id, activity_id)): AxumPath<(String, String)>,
    Json(request): Json<CodexPromptRequest>,
) -> Response {
    let provider = local_provider(&state, state.codex_sessions.clone());
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

pub async fn codex_session_interrupt(
    State(state): State<AppState>,
    AxumPath((_work_root_id, activity_id)): AxumPath<(String, String)>,
) -> Response {
    let provider = local_provider(&state, state.codex_sessions.clone());
    match provider
        .interrupt(AgentClientInterruptRequest { activity_id })
        .await
    {
        Ok(()) => StatusCode::NO_CONTENT.into_response(),
        Err(error) => provider_error_response(error),
    }
}

pub async fn codex_session_control(
    State(state): State<AppState>,
    AxumPath((_work_root_id, activity_id)): AxumPath<(String, String)>,
    Json(request): Json<CodexControlRequest>,
) -> Response {
    let provider = local_provider(&state, state.codex_sessions.clone());
    let result = match request {
        CodexControlRequest::Compact => provider
            .compact(&activity_id)
            .await
            .map(|()| CodexControlResponse {
                applied: true,
                data: None,
            }),
        CodexControlRequest::Steer { text } => provider
            .steer(&activity_id, &text)
            .await
            .map(|()| CodexControlResponse {
                applied: true,
                data: None,
            }),
        CodexControlRequest::Skills => {
            provider
                .skills_list(&activity_id)
                .await
                .map(|data| CodexControlResponse {
                    applied: true,
                    data: Some(data),
                })
        }
        CodexControlRequest::Fork { cut_cursor } => provider
            .fork(&activity_id, cut_cursor.as_deref())
            .await
            .map(|(new_activity_id, echoed_cursor)| CodexControlResponse {
                applied: true,
                data: Some(serde_json::json!({
                    "activityId": new_activity_id,
                    "cutCursor": echoed_cursor,
                })),
            }),
    };
    match result {
        Ok(response) => Json(response).into_response(),
        Err(error) => provider_error_response(error),
    }
}

pub async fn codex_session_transcript(
    State(state): State<AppState>,
    AxumPath((work_root_id, activity_id)): AxumPath<(String, String)>,
) -> Response {
    let provider = local_provider(&state, state.codex_sessions.clone());
    // resume_session validates the session exists without side effects.
    if let Err(error) = provider
        .resume_session(AgentClientSessionResumeRequest {
            activity_id: activity_id.clone(),
        })
        .await
    {
        return provider_error_response(error);
    }
    let Some(session) = state
        .codex_sessions
        .session_for(LOCAL_SERVER_ID, &activity_id)
    else {
        return provider_error_response(AgentClientProviderError {
            code: "codex.unknown_session".to_owned(),
            message: "unknown Codex session".to_owned(),
        });
    };
    let transcript = codex_activity_transcript(&session, WorkRootId::from(work_root_id)).await;
    Json(transcript).into_response()
}
