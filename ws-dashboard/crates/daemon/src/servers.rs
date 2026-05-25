use axum::extract::{Path as AxumPath, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::Serialize;
use ws_dashboard_core::{
    ActionHint, DashboardServersView, ServerConnectionStatus, ServerConnectionView, ServerId,
    ServerKind, ViewState,
};

use crate::persistent_state::PersistedLinkedServer;
use crate::resources::local_dashboard_resources_view;
use crate::router::AppState;

const LOCAL_SERVER_ID: &str = "server-local";

pub async fn dashboard_servers(State(state): State<AppState>) -> Json<DashboardServersView> {
    let mut servers = vec![local_server_view()];
    servers.extend(
        state
            .dashboard_state
            .load_linked_servers()
            .await
            .into_iter()
            .map(linked_server_view),
    );
    Json(DashboardServersView { servers })
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

fn linked_server_view(server: PersistedLinkedServer) -> ServerConnectionView {
    let status = server_status(&server);
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
