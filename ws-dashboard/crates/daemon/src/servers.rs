use std::collections::HashMap;
use std::net::TcpListener;
use std::process::{Child, Command, Stdio};
use std::sync::Arc;
use std::thread;
use std::time::Duration;

use axum::extract::{Path as AxumPath, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use reqwest::Url;
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
const SSH_TUNNEL_STARTUP_GRACE: Duration = Duration::from_millis(150);

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

#[derive(Clone)]
pub struct LinkedServerTunnels {
    tunnels: Arc<Mutex<HashMap<ServerId, ManagedTunnel>>>,
    launcher: TunnelLauncher,
}

impl Default for LinkedServerTunnels {
    fn default() -> Self {
        Self::system()
    }
}

impl LinkedServerTunnels {
    pub fn system() -> Self {
        Self {
            tunnels: Arc::new(Mutex::new(HashMap::new())),
            launcher: TunnelLauncher::System,
        }
    }

    pub fn record_only_for_tests() -> Self {
        Self {
            tunnels: Arc::new(Mutex::new(HashMap::new())),
            launcher: TunnelLauncher::RecordOnly,
        }
    }

    async fn connect(
        &self,
        server_id: ServerId,
        ssh_target: String,
        remote_endpoint: String,
        local_port: Option<u16>,
    ) -> Result<String, TunnelConnectError> {
        let remote_port = remote_loopback_port(&remote_endpoint)?;
        let local_port = match local_port {
            Some(0) | None => allocate_loopback_port()?,
            Some(port) => port,
        };
        let endpoint = format!("http://127.0.0.1:{local_port}");
        let child = match self.launcher {
            TunnelLauncher::System => {
                let child = tokio::task::spawn_blocking(move || {
                    start_system_ssh_tunnel(&ssh_target, local_port, remote_port)
                })
                .await
                .map_err(|_| TunnelConnectError::Failed)?
                .map_err(|_| TunnelConnectError::Failed)?;
                Some(child)
            }
            TunnelLauncher::RecordOnly => None,
        };

        self.tunnels
            .lock()
            .await
            .insert(server_id, ManagedTunnel { child });
        Ok(endpoint)
    }

    async fn contains(&self, server_id: &ServerId) -> bool {
        self.tunnels.lock().await.contains_key(server_id)
    }
}

#[derive(Clone, Copy)]
enum TunnelLauncher {
    System,
    RecordOnly,
}

struct ManagedTunnel {
    child: Option<Child>,
}

impl Drop for ManagedTunnel {
    fn drop(&mut self) {
        if let Some(child) = &mut self.child {
            let _ = child.kill();
        }
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum TunnelConnectError {
    InvalidEndpoint,
    Failed,
}

pub async fn dashboard_servers(State(state): State<AppState>) -> Json<DashboardServersView> {
    let mut servers = vec![local_server_view()];
    let linked_servers = state.dashboard_state.load_linked_servers().await;
    for server in linked_servers {
        let connected = state.linked_server_sessions.contains(&server.id).await;
        let tunnel_active = state.linked_server_tunnels.contains(&server.id).await;
        servers.push(linked_server_view(server, connected, tunnel_active));
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
    if server.remote_endpoint_hint.is_some()
        && !state.linked_server_tunnels.contains(&server.id).await
    {
        return server_error(StatusCode::CONFLICT, "linked server tunnel required");
    }

    match request_remote_link_token(endpoint, &request.passphrase).await {
        Ok(token) => {
            state
                .linked_server_sessions
                .insert(server.id.clone(), token)
                .await;
            Json(linked_server_view(server.clone(), true, true)).into_response()
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

pub async fn start_ssh_dashboard_server(
    State(state): State<AppState>,
    Json(request): Json<SshServerTunnelRequest>,
) -> Response {
    let Some((mut server, local_port)) = linked_server_from_tunnel_request(request) else {
        return server_error(StatusCode::BAD_REQUEST, "invalid linked server request");
    };
    if server.id.as_str() == LOCAL_SERVER_ID {
        return server_error(StatusCode::BAD_REQUEST, "local server cannot use SSH start");
    }
    let Some(ssh_target) = server.ssh_target.clone() else {
        return server_error(StatusCode::BAD_REQUEST, "missing SSH target");
    };
    let Some(remote_endpoint) = server.remote_endpoint_hint.clone() else {
        return server_error(StatusCode::BAD_REQUEST, "missing remote endpoint");
    };

    match state
        .linked_server_tunnels
        .connect(server.id.clone(), ssh_target, remote_endpoint, local_port)
        .await
    {
        Ok(endpoint) => {
            server.endpoint_hint = Some(endpoint);
            if let Err(error) = persist_linked_server(&state, server.clone()).await {
                return server_error(StatusCode::INTERNAL_SERVER_ERROR, error);
            }
            Json(linked_server_view(server, false, true)).into_response()
        }
        Err(TunnelConnectError::InvalidEndpoint) => {
            server_error(StatusCode::BAD_REQUEST, "invalid remote endpoint")
        }
        Err(TunnelConnectError::Failed) => {
            server_error(StatusCode::BAD_GATEWAY, "ssh tunnel failed")
        }
    }
}

pub async fn reconnect_dashboard_server_tunnel(
    State(state): State<AppState>,
    AxumPath(server_id): AxumPath<String>,
) -> Response {
    let linked_servers = state.dashboard_state.load_linked_servers().await;
    let Some(mut server) = linked_servers
        .into_iter()
        .find(|server| server.id.as_str() == server_id)
    else {
        return server_error(StatusCode::NOT_FOUND, "unknown server");
    };
    let Some(ssh_target) = server.ssh_target.clone() else {
        return server_error(StatusCode::CONFLICT, "linked server missing SSH target");
    };
    let Some(remote_endpoint) = server.remote_endpoint_hint.clone() else {
        return server_error(
            StatusCode::CONFLICT,
            "linked server missing remote endpoint",
        );
    };

    match state
        .linked_server_tunnels
        .connect(server.id.clone(), ssh_target, remote_endpoint, None)
        .await
    {
        Ok(endpoint) => {
            server.endpoint_hint = Some(endpoint);
            if let Err(error) = persist_linked_server(&state, server.clone()).await {
                return server_error(StatusCode::INTERNAL_SERVER_ERROR, error);
            }
            let connected = state.linked_server_sessions.contains(&server.id).await;
            Json(linked_server_view(server, connected, true)).into_response()
        }
        Err(TunnelConnectError::InvalidEndpoint) => {
            server_error(StatusCode::BAD_REQUEST, "invalid remote endpoint")
        }
        Err(TunnelConnectError::Failed) => {
            server_error(StatusCode::BAD_GATEWAY, "ssh tunnel failed")
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
        linked_server_refusal_message(server_status(
            server,
            state.linked_server_tunnels.contains(&server.id).await,
        )),
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

fn linked_server_view(
    server: PersistedLinkedServer,
    connected: bool,
    tunnel_active: bool,
) -> ServerConnectionView {
    let status = if connected {
        ServerConnectionStatus::Connected
    } else {
        server_status(&server, tunnel_active)
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

fn server_status(server: &PersistedLinkedServer, tunnel_active: bool) -> ServerConnectionStatus {
    if server.remote_endpoint_hint.is_some() && server.ssh_target.is_some() && !tunnel_active {
        ServerConnectionStatus::TunnelRequired
    } else if server.endpoint_hint.is_some() {
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

async fn persist_linked_server(
    state: &AppState,
    server: PersistedLinkedServer,
) -> Result<(), String> {
    let mut servers = state.dashboard_state.load_linked_servers().await;
    servers.retain(|candidate| candidate.id != server.id);
    servers.push(server);
    state.dashboard_state.persist_linked_servers(servers).await
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
pub struct SshServerTunnelRequest {
    server_id: String,
    label: String,
    ssh_target: String,
    remote_endpoint: String,
    #[serde(default)]
    local_port: Option<u16>,
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

fn linked_server_from_tunnel_request(
    request: SshServerTunnelRequest,
) -> Option<(PersistedLinkedServer, Option<u16>)> {
    let server_id = request.server_id.trim();
    let label = request.label.trim();
    let ssh_target = request.ssh_target.trim();
    let remote_endpoint = request.remote_endpoint.trim();
    if server_id.is_empty()
        || label.is_empty()
        || ssh_target.is_empty()
        || remote_endpoint.is_empty()
    {
        return None;
    }

    Some((
        PersistedLinkedServer {
            id: ServerId::from(server_id.to_owned()),
            label: label.to_owned(),
            kind: ServerKind::SshRemote,
            ssh_target: Some(ssh_target.to_owned()),
            endpoint_hint: None,
            remote_endpoint_hint: Some(remote_endpoint.to_owned()),
        },
        request.local_port,
    ))
}

fn remote_loopback_port(endpoint: &str) -> Result<u16, TunnelConnectError> {
    let url = Url::parse(endpoint).map_err(|_| TunnelConnectError::InvalidEndpoint)?;
    if url.scheme() != "http" {
        return Err(TunnelConnectError::InvalidEndpoint);
    }
    let Some(host) = url.host_str() else {
        return Err(TunnelConnectError::InvalidEndpoint);
    };
    if host != "127.0.0.1" && host != "localhost" {
        return Err(TunnelConnectError::InvalidEndpoint);
    }
    url.port().ok_or(TunnelConnectError::InvalidEndpoint)
}

fn allocate_loopback_port() -> Result<u16, TunnelConnectError> {
    TcpListener::bind(("127.0.0.1", 0))
        .and_then(|listener| listener.local_addr())
        .map(|addr| addr.port())
        .map_err(|_| TunnelConnectError::Failed)
}

fn start_system_ssh_tunnel(
    ssh_target: &str,
    local_port: u16,
    remote_port: u16,
) -> Result<Child, std::io::Error> {
    let ssh_bin = std::env::var("WS_DASHBOARD_SSH_BIN").unwrap_or_else(|_| "ssh".to_owned());
    let forward = format!("127.0.0.1:{local_port}:127.0.0.1:{remote_port}");
    let mut child = Command::new(ssh_bin)
        .arg("-N")
        .arg("-L")
        .arg(forward)
        .arg("-o")
        .arg("ExitOnForwardFailure=yes")
        .arg("-o")
        .arg("BatchMode=yes")
        .arg(ssh_target)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()?;
    thread::sleep(SSH_TUNNEL_STARTUP_GRACE);
    if child.try_wait()?.is_some() {
        return Err(std::io::Error::new(
            std::io::ErrorKind::Other,
            "ssh tunnel exited during startup",
        ));
    }
    Ok(child)
}
