use std::collections::HashMap;
use std::sync::Arc;

use axum::extract::{Path as AxumPath, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::{Deserialize, Serialize};
use tokio::sync::Mutex;
use ws_dashboard_core::{
    ActionHint, DashboardResourcesView, DashboardServersView, InstanceView, ServerConnectionStatus,
    ServerConnectionView, ServerId, ServerKind, ViewState,
};

use crate::auth::BearerAuthToken;
use crate::persistent_state::PersistedLinkedServer;
use crate::resources::local_dashboard_resources_view;
use crate::router::AppState;

const LOCAL_SERVER_ID: &str = "server-local";

#[derive(Clone, Default)]
pub struct LinkedServerSessions {
    tokens: Arc<Mutex<HashMap<ServerId, BearerAuthToken>>>,
}

impl LinkedServerSessions {
    async fn insert(&self, server_id: ServerId, token: BearerAuthToken) {
        self.tokens.lock().await.insert(server_id, token);
    }

    async fn get(&self, server_id: &ServerId) -> Option<BearerAuthToken> {
        self.tokens.lock().await.get(server_id).cloned()
    }

    async fn contains(&self, server_id: &ServerId) -> bool {
        self.tokens.lock().await.contains_key(server_id)
    }
}

pub async fn dashboard_servers(State(state): State<AppState>) -> Json<DashboardServersView> {
    let mut servers = vec![local_server_view()];
    let linked_servers = state.dashboard_state.load_linked_servers().await;
    for server in linked_servers {
        let connected = state.linked_server_sessions.contains(&server.id).await;
        servers.push(linked_server_view(server, connected));
    }
    Json(DashboardServersView { servers })
}

pub async fn remote_link_auth(
    State(state): State<AppState>,
    Json(request): Json<RemoteLinkAuthRequest>,
) -> Response {
    match state.auth.exchange_link_passphrase(&request.passphrase) {
        Some(token) => Json(RemoteLinkAuthResponse {
            bearer_token: token.as_token_string(),
        })
        .into_response(),
        None => server_error(StatusCode::UNAUTHORIZED, "invalid link passphrase"),
    }
}

pub async fn link_dashboard_server(
    State(state): State<AppState>,
    AxumPath(server_id): AxumPath<String>,
    Json(request): Json<RemoteLinkAuthRequest>,
) -> Response {
    if server_id == LOCAL_SERVER_ID {
        return server_error(
            StatusCode::BAD_REQUEST,
            "local server does not require link auth",
        );
    }

    let linked_servers = state.dashboard_state.load_linked_servers().await;
    let Some(server) = linked_servers
        .iter()
        .find(|server| server.id.as_str() == server_id)
    else {
        return server_error(StatusCode::NOT_FOUND, "unknown server");
    };
    let Some(endpoint) = server.endpoint_hint.as_deref() else {
        return server_error(StatusCode::CONFLICT, "linked server tunnel required");
    };

    match request_remote_link_token(endpoint, &request.passphrase).await {
        Ok(token) => {
            state
                .linked_server_sessions
                .insert(server.id.clone(), token)
                .await;
            Json(linked_server_view(server.clone(), true)).into_response()
        }
        Err(LinkAuthError::Rejected) => {
            server_error(StatusCode::UNAUTHORIZED, "link auth rejected")
        }
        Err(LinkAuthError::UnexpectedStatus) => {
            server_error(StatusCode::BAD_GATEWAY, "link auth upstream rejected")
        }
        Err(LinkAuthError::Unavailable) => {
            server_error(StatusCode::BAD_GATEWAY, "linked server unreachable")
        }
    }
}

pub async fn dashboard_server_resources(
    State(state): State<AppState>,
    AxumPath(server_id): AxumPath<String>,
) -> Response {
    if server_id == LOCAL_SERVER_ID {
        return Json(local_dashboard_resources_view(&state).await).into_response();
    }

    let linked_servers = state.dashboard_state.load_linked_servers().await;
    let Some(server) = linked_servers
        .iter()
        .find(|server| server.id.as_str() == server_id)
    else {
        return server_error(StatusCode::NOT_FOUND, "unknown server");
    };

    if let Some(token) = state.linked_server_sessions.get(&server.id).await {
        let Some(endpoint) = server.endpoint_hint.as_deref() else {
            return server_error(StatusCode::CONFLICT, "linked server tunnel required");
        };
        return match request_remote_resources(endpoint, &token).await {
            Ok(view) => Json(rewrite_resources_for_linked_server(view, server)).into_response(),
            Err(ResourceForwardError::Unauthorized) => {
                server_error(StatusCode::UNAUTHORIZED, "linked server auth rejected")
            }
            Err(ResourceForwardError::UnexpectedStatus) => server_error(
                StatusCode::BAD_GATEWAY,
                "linked server resource request failed",
            ),
            Err(ResourceForwardError::Unavailable) => {
                server_error(StatusCode::BAD_GATEWAY, "linked server unreachable")
            }
        };
    }

    server_error(
        StatusCode::CONFLICT,
        linked_server_refusal_message(server_status(server)),
    )
}

fn local_server_view() -> ServerConnectionView {
    ServerConnectionView {
        id: ServerId::from(LOCAL_SERVER_ID),
        label: "Local ws dashboard".to_owned(),
        kind: ServerKind::Local,
        status: ServerConnectionStatus::Connected,
        state: ViewState {
            status: "connected".to_owned(),
            loading: false,
            stale: false,
            error: None,
        },
        actions: vec![
            ActionHint {
                id: "refresh".to_owned(),
                label: "Refresh".to_owned(),
                enabled: true,
            },
            ActionHint {
                id: "openRoot".to_owned(),
                label: "Open root".to_owned(),
                enabled: true,
            },
        ],
    }
}

fn linked_server_view(server: PersistedLinkedServer, connected: bool) -> ServerConnectionView {
    let status = if connected {
        ServerConnectionStatus::Connected
    } else {
        server_status(&server)
    };
    ServerConnectionView {
        id: server.id,
        label: server.label,
        kind: server.kind,
        status,
        state: ViewState {
            status: status_text(status).to_owned(),
            loading: false,
            stale: false,
            error: None,
        },
        actions: linked_server_actions(status),
    }
}

fn server_status(server: &PersistedLinkedServer) -> ServerConnectionStatus {
    if server.endpoint_hint.is_some() {
        ServerConnectionStatus::AuthRequired
    } else {
        ServerConnectionStatus::TunnelRequired
    }
}

fn linked_server_actions(status: ServerConnectionStatus) -> Vec<ActionHint> {
    match status {
        ServerConnectionStatus::Connected => vec![ActionHint {
            id: "refresh".to_owned(),
            label: "Refresh".to_owned(),
            enabled: true,
        }],
        ServerConnectionStatus::AuthRequired => vec![ActionHint {
            id: "enterPassphrase".to_owned(),
            label: "Enter passphrase".to_owned(),
            enabled: true,
        }],
        ServerConnectionStatus::TunnelRequired => vec![ActionHint {
            id: "reconnectTunnel".to_owned(),
            label: "Reconnect tunnel".to_owned(),
            enabled: true,
        }],
        _ => Vec::new(),
    }
}

fn linked_server_refusal_message(status: ServerConnectionStatus) -> &'static str {
    match status {
        ServerConnectionStatus::AuthRequired => "linked server auth required",
        ServerConnectionStatus::TunnelRequired => "linked server tunnel required",
        ServerConnectionStatus::Unreachable => "linked server unreachable",
        ServerConnectionStatus::Starting => "linked server starting",
        ServerConnectionStatus::StaleEndpoint => "linked server endpoint stale",
        ServerConnectionStatus::Connected => "linked server forwarding unavailable",
    }
}

fn status_text(status: ServerConnectionStatus) -> &'static str {
    match status {
        ServerConnectionStatus::Connected => "connected",
        ServerConnectionStatus::AuthRequired => "authRequired",
        ServerConnectionStatus::Unreachable => "unreachable",
        ServerConnectionStatus::Starting => "starting",
        ServerConnectionStatus::StaleEndpoint => "staleEndpoint",
        ServerConnectionStatus::TunnelRequired => "tunnelRequired",
    }
}

fn server_error(status: StatusCode, error: impl Into<String>) -> Response {
    #[derive(Serialize)]
    #[serde(rename_all = "camelCase")]
    struct ServerError {
        error: String,
    }

    (
        status,
        Json(ServerError {
            error: error.into(),
        }),
    )
        .into_response()
}

fn rewrite_resources_for_linked_server(
    mut view: DashboardResourcesView,
    server: &PersistedLinkedServer,
) -> DashboardResourcesView {
    view.server.id = server.id.clone();
    view.server.label = server.label.clone();
    for workspace in &mut view.workspaces {
        for work_root in &mut workspace.work_roots {
            work_root.resource_path.server_id = server.id.clone();
            for instance in &mut work_root.main_instances {
                rewrite_instance_server_id(instance, &server.id);
            }
        }
    }
    view
}

fn rewrite_instance_server_id(instance: &mut InstanceView, server_id: &ServerId) {
    instance.resource_path.server_id = server_id.clone();
    for child in &mut instance.sub_instances {
        rewrite_instance_server_id(child, server_id);
    }
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteLinkAuthRequest {
    passphrase: String,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct RemoteLinkAuthResponse {
    bearer_token: String,
}

#[derive(Debug, Eq, PartialEq)]
enum LinkAuthError {
    Rejected,
    UnexpectedStatus,
    Unavailable,
}

#[derive(Debug, Eq, PartialEq)]
enum ResourceForwardError {
    Unauthorized,
    UnexpectedStatus,
    Unavailable,
}

async fn request_remote_link_token(
    endpoint: &str,
    passphrase: &str,
) -> Result<BearerAuthToken, LinkAuthError> {
    let response = reqwest::Client::new()
        .post(remote_url(endpoint, "/api/dashboard/link-auth"))
        .json(&RemoteLinkAuthRequest {
            passphrase: passphrase.to_owned(),
        })
        .send()
        .await
        .map_err(|_| LinkAuthError::Unavailable)?;
    if response.status() == StatusCode::UNAUTHORIZED {
        return Err(LinkAuthError::Rejected);
    }
    if !response.status().is_success() {
        return Err(LinkAuthError::UnexpectedStatus);
    }
    response
        .json::<RemoteLinkAuthResponse>()
        .await
        .map(|body| BearerAuthToken::from_token_string(body.bearer_token))
        .map_err(|_| LinkAuthError::UnexpectedStatus)
}

async fn request_remote_resources(
    endpoint: &str,
    token: &BearerAuthToken,
) -> Result<DashboardResourcesView, ResourceForwardError> {
    let response = reqwest::Client::new()
        .get(remote_url(endpoint, "/api/dashboard/resources"))
        .header(
            axum::http::header::AUTHORIZATION.as_str(),
            token.as_authorization_header(),
        )
        .send()
        .await
        .map_err(|_| ResourceForwardError::Unavailable)?;
    if response.status() == StatusCode::UNAUTHORIZED {
        return Err(ResourceForwardError::Unauthorized);
    }
    if !response.status().is_success() {
        return Err(ResourceForwardError::UnexpectedStatus);
    }
    response
        .json::<DashboardResourcesView>()
        .await
        .map_err(|_| ResourceForwardError::UnexpectedStatus)
}

fn remote_url(endpoint: &str, path: &str) -> String {
    format!("{}{}", endpoint.trim_end_matches('/'), path)
}
