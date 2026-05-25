use std::collections::HashMap;
use std::net::TcpListener;
use std::pin::Pin;
use std::process::{Child, Command, Stdio};
use std::sync::Arc;
use std::thread;
use std::time::Duration;

use axum::body::{Body, Bytes};
use axum::extract::{OriginalUri, Path as AxumPath, Query, State};
use axum::http::{header, HeaderMap, HeaderValue, Method, StatusCode};
use axum::response::{IntoResponse, Response};
use axum::Json;
use futures_util::Stream;
use reqwest::Url;
use serde::{Deserialize, Serialize};
use tokio::io::{AsyncRead, AsyncReadExt};
use tokio::sync::Mutex;
use ws_dashboard_core::{
    ActionHint, DashboardResourcesView, DashboardServersView, InstanceView, ServerConnectionStatus,
    ServerConnectionView, ServerId, ServerKind, ViewState,
};

use crate::auth::BearerAuthToken;
use crate::persistent_state::PersistedLinkedServer;
use crate::resources::local_dashboard_resources_view;
use crate::root_picker::{
    create_empty_directory, list_root_picker, open_work_root, pin_root_picker_directory,
    set_work_root_activation, unpin_root_picker_directory, CreateEmptyDirectoryRequest,
    OpenWorkRootRequest, RootPickerPinRequest, SetWorkRootActivationRequest,
};
use crate::router::AppState;
use crate::work_root_files::{
    document_events, list_work_root_files, read_work_root_file, write_work_root_file,
    WorkRootFileListQuery, WorkRootFileWriteRequest,
};

const LOCAL_SERVER_ID: &str = "server-local";
const SSH_TUNNEL_STARTUP_GRACE: Duration = Duration::from_millis(150);
const SSH_STARTUP_CAPTURE_TIMEOUT: Duration = Duration::from_secs(20);
const SSH_STARTUP_CAPTURE_BYTE_LIMIT: usize = 64 * 1024;
const OWNER_PAIRING_PREFIX: &str = "ws-dashboard owner pairing URL:";
const LINK_PASSPHRASE_PREFIX: &str = "ws-dashboard remote link passphrase:";

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

    async fn remove(&self, server_id: &ServerId) {
        self.tokens.lock().await.remove(server_id);
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

    async fn capture_remote_startup(
        &self,
        ssh_target: &str,
        startup_command: &str,
    ) -> Result<RemoteStartupCapture, StartupCaptureError> {
        match self.launcher {
            TunnelLauncher::System => {
                capture_system_remote_startup(ssh_target, startup_command).await
            }
            TunnelLauncher::RecordOnly => parse_remote_startup_metadata(startup_command),
        }
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

#[derive(Clone, Debug, Eq, PartialEq)]
struct RemoteStartupCapture {
    remote_endpoint: String,
    link_passphrase: Option<String>,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum StartupCaptureError {
    Failed,
    MissingEndpoint,
    InvalidEndpoint,
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

pub async fn link_endpoint_server(
    State(state): State<AppState>,
    Json(request): Json<EndpointLinkedServerRequest>,
) -> Response {
    let Some(server) = linked_server_from_endpoint_request(&request) else {
        return server_error(StatusCode::BAD_REQUEST, "invalid linked server request");
    };
    if server.id.as_str() == LOCAL_SERVER_ID {
        return server_error(
            StatusCode::BAD_REQUEST,
            "local server cannot be linked as a remote endpoint",
        );
    }
    let Some(endpoint) = server.endpoint_hint.as_deref() else {
        return server_error(StatusCode::BAD_REQUEST, "invalid endpoint");
    };

    let passphrase = request.passphrase.as_deref().map(str::trim).unwrap_or("");
    let auth_result = request_remote_link_token(endpoint, passphrase).await;
    let status = match auth_result {
        Ok(token) => {
            state
                .linked_server_sessions
                .insert(server.id.clone(), token)
                .await;
            ServerConnectionStatus::Connected
        }
        Err(LinkAuthError::Rejected) => {
            state.linked_server_sessions.remove(&server.id).await;
            ServerConnectionStatus::AuthRequired
        }
        Err(LinkAuthError::UnexpectedStatus) => {
            return server_error(
                StatusCode::BAD_GATEWAY,
                "linked server endpoint is incompatible",
            );
        }
        Err(LinkAuthError::Unavailable) => {
            return server_error(StatusCode::BAD_GATEWAY, "linked server unreachable");
        }
    };

    if let Err(error) = persist_linked_server(&state, server.clone()).await {
        return server_error(StatusCode::INTERNAL_SERVER_ERROR, error);
    }
    Json(linked_server_view_with_status(server, status)).into_response()
}

pub async fn start_ssh_dashboard_server(
    State(state): State<AppState>,
    Json(request): Json<SshServerTunnelRequest>,
) -> Response {
    let Some(mut request) = linked_server_from_tunnel_request(request) else {
        return server_error(StatusCode::BAD_REQUEST, "invalid linked server request");
    };
    if request.server.id.as_str() == LOCAL_SERVER_ID {
        return server_error(StatusCode::BAD_REQUEST, "local server cannot use SSH start");
    }
    let Some(ssh_target) = request.server.ssh_target.clone() else {
        return server_error(StatusCode::BAD_REQUEST, "missing SSH target");
    };
    let mut link_passphrase = None;
    let remote_endpoint = match request.server.remote_endpoint_hint.clone() {
        Some(endpoint) => endpoint,
        None => {
            let Some(startup_command) = request.startup_command.as_deref() else {
                return server_error(StatusCode::BAD_REQUEST, "missing remote endpoint");
            };
            match state
                .linked_server_tunnels
                .capture_remote_startup(&ssh_target, startup_command)
                .await
            {
                Ok(capture) => {
                    link_passphrase = capture.link_passphrase;
                    request.server.remote_endpoint_hint = Some(capture.remote_endpoint.clone());
                    capture.remote_endpoint
                }
                Err(StartupCaptureError::MissingEndpoint) => {
                    return server_error(
                        StatusCode::BAD_GATEWAY,
                        "remote startup endpoint missing",
                    );
                }
                Err(StartupCaptureError::InvalidEndpoint) => {
                    return server_error(
                        StatusCode::BAD_GATEWAY,
                        "remote startup endpoint invalid",
                    );
                }
                Err(StartupCaptureError::Failed) => {
                    return server_error(StatusCode::BAD_GATEWAY, "remote startup failed");
                }
            }
        }
    };

    match state
        .linked_server_tunnels
        .connect(
            request.server.id.clone(),
            ssh_target,
            remote_endpoint,
            request.local_port,
        )
        .await
    {
        Ok(endpoint) => {
            request.server.endpoint_hint = Some(endpoint.clone());
            state
                .linked_server_sessions
                .remove(&request.server.id)
                .await;
            if let Some(passphrase) = link_passphrase {
                if let Ok(token) =
                    request_remote_link_token_with_retry(&endpoint, &passphrase).await
                {
                    state
                        .linked_server_sessions
                        .insert(request.server.id.clone(), token)
                        .await;
                }
            }
            if let Err(error) = persist_linked_server(&state, request.server.clone()).await {
                return server_error(StatusCode::INTERNAL_SERVER_ERROR, error);
            }
            let connected = state
                .linked_server_sessions
                .contains(&request.server.id)
                .await;
            Json(linked_server_view(request.server, connected, true)).into_response()
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

#[derive(Clone, Debug)]
struct ServerScopedForwardOperation {
    method: Method,
    legacy_path: String,
    rewrite_resources: bool,
}

impl ServerScopedForwardOperation {
    fn root_picker(uri: OriginalUri) -> Self {
        Self {
            method: Method::GET,
            legacy_path: legacy_path_with_query("/api/dashboard/root-picker", &uri),
            rewrite_resources: false,
        }
    }

    fn create_directory() -> Self {
        Self {
            method: Method::POST,
            legacy_path: "/api/dashboard/root-picker/directories".to_owned(),
            rewrite_resources: false,
        }
    }

    fn pins(method: Method) -> Option<Self> {
        matches!(method, Method::POST | Method::DELETE).then(|| Self {
            method,
            legacy_path: "/api/dashboard/root-picker/pins".to_owned(),
            rewrite_resources: false,
        })
    }

    fn open_work_root() -> Self {
        Self {
            method: Method::POST,
            legacy_path: "/api/dashboard/work-roots/open".to_owned(),
            rewrite_resources: true,
        }
    }

    fn set_activation(work_root_id: &str) -> Self {
        Self {
            method: Method::POST,
            legacy_path: format!("/api/dashboard/work-roots/{work_root_id}/activation"),
            rewrite_resources: true,
        }
    }

    fn work_root_files(work_root_id: &str, uri: OriginalUri) -> Self {
        Self {
            method: Method::GET,
            legacy_path: legacy_path_with_query(
                &format!("/api/dashboard/work-roots/{work_root_id}/files"),
                &uri,
            ),
            rewrite_resources: false,
        }
    }

    fn read_work_root_file(work_root_id: &str, uri: OriginalUri) -> Self {
        Self {
            method: Method::GET,
            legacy_path: legacy_path_with_query(
                &format!("/api/dashboard/work-roots/{work_root_id}/files/read"),
                &uri,
            ),
            rewrite_resources: false,
        }
    }

    fn write_work_root_file(work_root_id: &str) -> Self {
        Self {
            method: Method::POST,
            legacy_path: format!("/api/dashboard/work-roots/{work_root_id}/files/write"),
            rewrite_resources: false,
        }
    }

    fn document_events(work_root_id: &str) -> Self {
        Self {
            method: Method::GET,
            legacy_path: format!("/api/dashboard/work-roots/{work_root_id}/documents/events"),
            rewrite_resources: false,
        }
    }
}

enum ServerScopedResolution {
    Local,
    Linked {
        server: PersistedLinkedServer,
        endpoint: String,
        token: BearerAuthToken,
    },
    Refusal {
        status: StatusCode,
        message: &'static str,
    },
}

pub async fn server_scoped_root_picker(
    State(state): State<AppState>,
    AxumPath(server_id): AxumPath<String>,
    Query(query): Query<HashMap<String, String>>,
    uri: OriginalUri,
) -> Response {
    let operation = ServerScopedForwardOperation::root_picker(uri);
    if server_id == LOCAL_SERVER_ID {
        return list_root_picker(State(state), Query(query)).await;
    }
    forward_server_scoped_operation(state, server_id, operation, HeaderMap::new(), Bytes::new())
        .await
}

pub async fn server_scoped_create_empty_directory(
    State(state): State<AppState>,
    AxumPath(server_id): AxumPath<String>,
    headers: HeaderMap,
    body: Bytes,
) -> Response {
    let operation = ServerScopedForwardOperation::create_directory();
    if server_id == LOCAL_SERVER_ID {
        return match serde_json::from_slice::<CreateEmptyDirectoryRequest>(&body) {
            Ok(request) => create_empty_directory(Json(request)).await,
            Err(_) => server_error(StatusCode::BAD_REQUEST, "invalid JSON body"),
        };
    }
    forward_server_scoped_operation(state, server_id, operation, headers, body).await
}

pub async fn server_scoped_root_picker_pins(
    State(state): State<AppState>,
    AxumPath(server_id): AxumPath<String>,
    method: Method,
    headers: HeaderMap,
    body: Bytes,
) -> Response {
    let Some(operation) = ServerScopedForwardOperation::pins(method.clone()) else {
        return server_error(
            StatusCode::METHOD_NOT_ALLOWED,
            "unsupported server-scoped operation",
        );
    };
    if server_id == LOCAL_SERVER_ID {
        let request = match serde_json::from_slice::<RootPickerPinRequest>(&body) {
            Ok(request) => request,
            Err(_) => return server_error(StatusCode::BAD_REQUEST, "invalid JSON body"),
        };
        return if method == Method::POST {
            pin_root_picker_directory(State(state), Json(request)).await
        } else {
            unpin_root_picker_directory(State(state), Json(request)).await
        };
    }
    forward_server_scoped_operation(state, server_id, operation, headers, body).await
}

pub async fn server_scoped_open_work_root(
    State(state): State<AppState>,
    AxumPath(server_id): AxumPath<String>,
    headers: HeaderMap,
    body: Bytes,
) -> Response {
    let operation = ServerScopedForwardOperation::open_work_root();
    if server_id == LOCAL_SERVER_ID {
        return match serde_json::from_slice::<OpenWorkRootRequest>(&body) {
            Ok(request) => open_work_root(State(state), Json(request)).await,
            Err(_) => server_error(StatusCode::BAD_REQUEST, "invalid JSON body"),
        };
    }
    forward_server_scoped_operation(state, server_id, operation, headers, body).await
}

pub async fn server_scoped_set_work_root_activation(
    State(state): State<AppState>,
    AxumPath((server_id, work_root_id)): AxumPath<(String, String)>,
    headers: HeaderMap,
    body: Bytes,
) -> Response {
    let operation = ServerScopedForwardOperation::set_activation(&work_root_id);
    if server_id == LOCAL_SERVER_ID {
        return match serde_json::from_slice::<SetWorkRootActivationRequest>(&body) {
            Ok(request) => {
                set_work_root_activation(State(state), AxumPath(work_root_id), Json(request)).await
            }
            Err(_) => server_error(StatusCode::BAD_REQUEST, "invalid JSON body"),
        };
    }
    forward_server_scoped_operation(state, server_id, operation, headers, body).await
}

pub async fn server_scoped_work_root_files(
    State(state): State<AppState>,
    AxumPath((server_id, work_root_id)): AxumPath<(String, String)>,
    Query(query): Query<WorkRootFileListQuery>,
    uri: OriginalUri,
) -> Response {
    let operation = ServerScopedForwardOperation::work_root_files(&work_root_id, uri);
    if server_id == LOCAL_SERVER_ID {
        return list_work_root_files(State(state), AxumPath(work_root_id), Query(query)).await;
    }
    forward_server_scoped_operation(state, server_id, operation, HeaderMap::new(), Bytes::new())
        .await
}

pub async fn server_scoped_read_work_root_file(
    State(state): State<AppState>,
    AxumPath((server_id, work_root_id)): AxumPath<(String, String)>,
    Query(query): Query<WorkRootFileListQuery>,
    uri: OriginalUri,
) -> Response {
    let operation = ServerScopedForwardOperation::read_work_root_file(&work_root_id, uri);
    if server_id == LOCAL_SERVER_ID {
        return read_work_root_file(State(state), AxumPath(work_root_id), Query(query)).await;
    }
    forward_server_scoped_operation(state, server_id, operation, HeaderMap::new(), Bytes::new())
        .await
}

pub async fn server_scoped_write_work_root_file(
    State(state): State<AppState>,
    AxumPath((server_id, work_root_id)): AxumPath<(String, String)>,
    headers: HeaderMap,
    body: Bytes,
) -> Response {
    let operation = ServerScopedForwardOperation::write_work_root_file(&work_root_id);
    if server_id == LOCAL_SERVER_ID {
        return match serde_json::from_slice::<WorkRootFileWriteRequest>(&body) {
            Ok(request) => {
                write_work_root_file(State(state), AxumPath(work_root_id), Json(request)).await
            }
            Err(_) => server_error(StatusCode::BAD_REQUEST, "invalid JSON body"),
        };
    }
    forward_server_scoped_operation(state, server_id, operation, headers, body).await
}

pub async fn server_scoped_document_events(
    State(state): State<AppState>,
    AxumPath((server_id, work_root_id)): AxumPath<(String, String)>,
) -> Response {
    let operation = ServerScopedForwardOperation::document_events(&work_root_id);
    if server_id == LOCAL_SERVER_ID {
        return document_events(State(state), AxumPath(work_root_id)).await;
    }
    forward_server_scoped_document_events(state, server_id, operation).await
}

async fn forward_server_scoped_operation(
    state: AppState,
    server_id: String,
    operation: ServerScopedForwardOperation,
    headers: HeaderMap,
    body: Bytes,
) -> Response {
    match resolve_server_scoped_forwarding(&state, &server_id).await {
        ServerScopedResolution::Local => server_error(
            StatusCode::INTERNAL_SERVER_ERROR,
            "server-local dispatch should be handled in-process",
        ),
        ServerScopedResolution::Refusal { status, message } => server_error(status, message),
        ServerScopedResolution::Linked {
            server,
            endpoint,
            token,
        } => match request_remote_dashboard_operation(&endpoint, &token, &operation, headers, body)
            .await
        {
            Ok(response) => response.into_response_for(&operation, &server),
            Err(ForwardOperationError::Unavailable) => {
                server_error(StatusCode::BAD_GATEWAY, "linked server unreachable")
            }
        },
    }
}

async fn forward_server_scoped_document_events(
    state: AppState,
    server_id: String,
    operation: ServerScopedForwardOperation,
) -> Response {
    match resolve_server_scoped_forwarding(&state, &server_id).await {
        ServerScopedResolution::Local => server_error(
            StatusCode::INTERNAL_SERVER_ERROR,
            "server-local dispatch should be handled in-process",
        ),
        ServerScopedResolution::Refusal { status, message } => server_error(status, message),
        ServerScopedResolution::Linked {
            endpoint, token, ..
        } => match request_remote_document_events(&endpoint, &token, &operation).await {
            Ok(RemoteDocumentEventsResponse::Stream { content_type, body }) => {
                let mut response = Body::from_stream(body).into_response();
                response
                    .headers_mut()
                    .insert(header::CONTENT_TYPE, content_type);
                response
            }
            Ok(RemoteDocumentEventsResponse::UpstreamError(response)) => {
                response.into_plain_response()
            }
            Ok(RemoteDocumentEventsResponse::InvalidStream) => server_error(
                StatusCode::BAD_GATEWAY,
                "linked server document events stream unavailable",
            ),
            Err(ForwardOperationError::Unavailable) => {
                server_error(StatusCode::BAD_GATEWAY, "linked server unreachable")
            }
        },
    }
}

async fn resolve_server_scoped_forwarding(
    state: &AppState,
    server_id: &str,
) -> ServerScopedResolution {
    if server_id == LOCAL_SERVER_ID {
        return ServerScopedResolution::Local;
    }

    let linked_servers = state.dashboard_state.load_linked_servers().await;
    let Some(server) = linked_servers
        .into_iter()
        .find(|server| server.id.as_str() == server_id)
    else {
        return ServerScopedResolution::Refusal {
            status: StatusCode::NOT_FOUND,
            message: "unknown server",
        };
    };

    let Some(token) = state.linked_server_sessions.get(&server.id).await else {
        let status = server_status(
            &server,
            state.linked_server_tunnels.contains(&server.id).await,
        );
        return ServerScopedResolution::Refusal {
            status: StatusCode::CONFLICT,
            message: linked_server_refusal_message(status),
        };
    };

    let Some(endpoint) = server.endpoint_hint.clone() else {
        return ServerScopedResolution::Refusal {
            status: StatusCode::CONFLICT,
            message: "linked server tunnel required",
        };
    };

    ServerScopedResolution::Linked {
        server,
        endpoint,
        token,
    }
}

struct ForwardedDashboardResponse {
    status: StatusCode,
    content_type: Option<HeaderValue>,
    opened_work_root_id: Option<HeaderValue>,
    body: Bytes,
}

impl ForwardedDashboardResponse {
    fn into_response_for(
        self,
        operation: &ServerScopedForwardOperation,
        server: &PersistedLinkedServer,
    ) -> Response {
        if self.status.is_success() && operation.rewrite_resources {
            if let Ok(view) = serde_json::from_slice::<DashboardResourcesView>(&self.body) {
                let mut response =
                    Json(rewrite_resources_for_linked_server(view, server)).into_response();
                *response.status_mut() = self.status;
                if let Some(value) = self.opened_work_root_id {
                    response
                        .headers_mut()
                        .insert("x-ws-dashboard-opened-work-root-id", value);
                }
                return response;
            }
        }

        self.into_plain_response()
    }

    fn into_plain_response(self) -> Response {
        let mut response = (self.status, Body::from(self.body)).into_response();
        if let Some(value) = self.content_type {
            response.headers_mut().insert(header::CONTENT_TYPE, value);
        }
        if let Some(value) = self.opened_work_root_id {
            response
                .headers_mut()
                .insert("x-ws-dashboard-opened-work-root-id", value);
        }
        response
    }
}

#[derive(Debug, Eq, PartialEq)]
enum ForwardOperationError {
    Unavailable,
}

type RemoteDocumentEventStream = Pin<Box<dyn Stream<Item = Result<Bytes, reqwest::Error>> + Send>>;

enum RemoteDocumentEventsResponse {
    Stream {
        content_type: HeaderValue,
        body: RemoteDocumentEventStream,
    },
    UpstreamError(ForwardedDashboardResponse),
    InvalidStream,
}

async fn request_remote_document_events(
    endpoint: &str,
    token: &BearerAuthToken,
    operation: &ServerScopedForwardOperation,
) -> Result<RemoteDocumentEventsResponse, ForwardOperationError> {
    let response = reqwest::Client::new()
        .get(remote_url(endpoint, &operation.legacy_path))
        .header(
            header::AUTHORIZATION.as_str(),
            token.as_authorization_header(),
        )
        .header(header::ACCEPT.as_str(), "text/event-stream")
        .send()
        .await
        .map_err(|_| ForwardOperationError::Unavailable)?;
    let status = response.status();
    let content_type = response.headers().get(header::CONTENT_TYPE).cloned();

    if !status.is_success() {
        let body = response
            .bytes()
            .await
            .map_err(|_| ForwardOperationError::Unavailable)?;
        return Ok(RemoteDocumentEventsResponse::UpstreamError(
            ForwardedDashboardResponse {
                status,
                content_type,
                opened_work_root_id: None,
                body,
            },
        ));
    }

    let Some(content_type) = content_type else {
        return Ok(RemoteDocumentEventsResponse::InvalidStream);
    };
    let is_event_stream = content_type
        .to_str()
        .map(|value| value.starts_with("text/event-stream"))
        .unwrap_or(false);
    if !is_event_stream {
        return Ok(RemoteDocumentEventsResponse::InvalidStream);
    }

    Ok(RemoteDocumentEventsResponse::Stream {
        content_type,
        body: Box::pin(response.bytes_stream()),
    })
}

async fn request_remote_dashboard_operation(
    endpoint: &str,
    token: &BearerAuthToken,
    operation: &ServerScopedForwardOperation,
    headers: HeaderMap,
    body: Bytes,
) -> Result<ForwardedDashboardResponse, ForwardOperationError> {
    let mut request = reqwest::Client::new()
        .request(
            operation.method.clone(),
            remote_url(endpoint, &operation.legacy_path),
        )
        .header(
            header::AUTHORIZATION.as_str(),
            token.as_authorization_header(),
        );
    if let Some(content_type) = headers.get(header::CONTENT_TYPE) {
        request = request.header(header::CONTENT_TYPE.as_str(), content_type.clone());
    }
    if !body.is_empty() {
        request = request.body(body.clone());
    }

    let response = request
        .send()
        .await
        .map_err(|_| ForwardOperationError::Unavailable)?;
    let status = response.status();
    let content_type = response.headers().get(header::CONTENT_TYPE).cloned();
    let opened_work_root_id = response
        .headers()
        .get("x-ws-dashboard-opened-work-root-id")
        .cloned();
    let body = response
        .bytes()
        .await
        .map_err(|_| ForwardOperationError::Unavailable)?;
    Ok(ForwardedDashboardResponse {
        status,
        content_type,
        opened_work_root_id,
        body,
    })
}

fn legacy_path_with_query(path: &str, uri: &OriginalUri) -> String {
    match uri.query() {
        Some(query) if !query.is_empty() => format!("{path}?{query}"),
        _ => path.to_owned(),
    }
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
    linked_server_view_with_status(server, status)
}

fn linked_server_view_with_status(
    server: PersistedLinkedServer,
    status: ServerConnectionStatus,
) -> ServerConnectionView {
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
    #[serde(default)]
    remote_endpoint: Option<String>,
    #[serde(default)]
    startup_command: Option<String>,
    #[serde(default)]
    local_port: Option<u16>,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct EndpointLinkedServerRequest {
    server_id: String,
    label: String,
    endpoint: String,
    #[serde(default)]
    passphrase: Option<String>,
}

struct NormalizedSshServerTunnelRequest {
    server: PersistedLinkedServer,
    startup_command: Option<String>,
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

async fn request_remote_link_token_with_retry(
    endpoint: &str,
    passphrase: &str,
) -> Result<BearerAuthToken, LinkAuthError> {
    let mut last_error = LinkAuthError::Unavailable;
    for _ in 0..20 {
        match request_remote_link_token(endpoint, passphrase).await {
            Ok(token) => return Ok(token),
            Err(LinkAuthError::Unavailable) => {
                last_error = LinkAuthError::Unavailable;
                tokio::time::sleep(Duration::from_millis(100)).await;
            }
            Err(error) => return Err(error),
        }
    }
    Err(last_error)
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

fn trim_optional(value: Option<String>) -> Option<String> {
    value.and_then(|value| {
        let trimmed = value.trim();
        (!trimmed.is_empty()).then(|| trimmed.to_owned())
    })
}

fn linked_server_from_tunnel_request(
    request: SshServerTunnelRequest,
) -> Option<NormalizedSshServerTunnelRequest> {
    let server_id = request.server_id.trim();
    let label = request.label.trim();
    let ssh_target = request.ssh_target.trim();
    if server_id.is_empty() || label.is_empty() || ssh_target.is_empty() {
        return None;
    }
    let remote_endpoint = trim_optional(request.remote_endpoint);
    let startup_command = trim_optional(request.startup_command);
    if remote_endpoint.is_none() && startup_command.is_none() {
        return None;
    }

    Some(NormalizedSshServerTunnelRequest {
        server: PersistedLinkedServer {
            id: ServerId::from(server_id.to_owned()),
            label: label.to_owned(),
            kind: ServerKind::SshRemote,
            ssh_target: Some(ssh_target.to_owned()),
            endpoint_hint: None,
            remote_endpoint_hint: remote_endpoint,
        },
        startup_command,
        local_port: request.local_port,
    })
}

fn linked_server_from_endpoint_request(
    request: &EndpointLinkedServerRequest,
) -> Option<PersistedLinkedServer> {
    let server_id = request.server_id.trim();
    let label = request.label.trim();
    let endpoint = normalize_dashboard_endpoint(&request.endpoint)?;
    if server_id.is_empty() || label.is_empty() {
        return None;
    }

    Some(PersistedLinkedServer {
        id: ServerId::from(server_id.to_owned()),
        label: label.to_owned(),
        kind: ServerKind::Manual,
        ssh_target: None,
        endpoint_hint: Some(endpoint),
        remote_endpoint_hint: None,
    })
}

fn normalize_dashboard_endpoint(endpoint: &str) -> Option<String> {
    let trimmed = endpoint.trim();
    if trimmed.is_empty() {
        return None;
    }
    let mut url = Url::parse(trimmed).ok()?;
    match url.scheme() {
        "http" | "https" => {}
        _ => return None,
    }
    url.host_str()?;
    if url.query().is_some() || url.fragment().is_some() {
        return None;
    }
    let normalized_path = url.path().trim_end_matches('/').to_owned();
    url.set_path(&normalized_path);
    Some(url.to_string().trim_end_matches('/').to_owned())
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

async fn capture_system_remote_startup(
    ssh_target: &str,
    startup_command: &str,
) -> Result<RemoteStartupCapture, StartupCaptureError> {
    let ssh_bin = std::env::var("WS_DASHBOARD_SSH_BIN").unwrap_or_else(|_| "ssh".to_owned());
    let mut command = tokio::process::Command::new(ssh_bin);
    command
        .arg("-o")
        .arg("BatchMode=yes")
        .arg(ssh_target)
        .arg(startup_command)
        .kill_on_drop(true)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let mut child = command.spawn().map_err(|_| StartupCaptureError::Failed)?;
    let stdout = child.stdout.take().ok_or(StartupCaptureError::Failed)?;
    let stderr = child.stderr.take().ok_or(StartupCaptureError::Failed)?;

    let (stdout, stderr, _status) = tokio::time::timeout(SSH_STARTUP_CAPTURE_TIMEOUT, async {
        let stdout = read_bounded_startup_output(stdout);
        let stderr = read_bounded_startup_output(stderr);
        let status = child.wait();
        tokio::join!(stdout, stderr, status)
    })
    .await
    .map_err(|_| StartupCaptureError::Failed)?;
    let stdout = stdout?;
    let stderr = stderr?;
    _status.map_err(|_| StartupCaptureError::Failed)?;

    let raw = format!(
        "{}\n{}",
        String::from_utf8_lossy(&stdout),
        String::from_utf8_lossy(&stderr)
    );
    parse_remote_startup_metadata(&raw)
}

async fn read_bounded_startup_output<R>(mut reader: R) -> Result<Vec<u8>, StartupCaptureError>
where
    R: AsyncRead + Unpin,
{
    let mut output = Vec::new();
    let mut chunk = [0_u8; 4096];
    loop {
        let read = reader
            .read(&mut chunk)
            .await
            .map_err(|_| StartupCaptureError::Failed)?;
        if read == 0 {
            return Ok(output);
        }
        if output.len() + read > SSH_STARTUP_CAPTURE_BYTE_LIMIT {
            return Err(StartupCaptureError::Failed);
        }
        output.extend_from_slice(&chunk[..read]);
    }
}

fn parse_remote_startup_metadata(
    output: &str,
) -> Result<RemoteStartupCapture, StartupCaptureError> {
    let mut pairing_url = None;
    let mut link_passphrase = None;
    for line in output.lines() {
        if let Some(index) = line.find(OWNER_PAIRING_PREFIX) {
            let value = &line[index + OWNER_PAIRING_PREFIX.len()..];
            pairing_url = Some(value.trim().to_owned());
        }
        if let Some(index) = line.find(LINK_PASSPHRASE_PREFIX) {
            let value = &line[index + LINK_PASSPHRASE_PREFIX.len()..];
            let value = value.trim();
            if !value.is_empty() {
                link_passphrase = Some(value.to_owned());
            }
        }
    }
    let Some(pairing_url) = pairing_url else {
        return Err(StartupCaptureError::MissingEndpoint);
    };
    let url = Url::parse(&pairing_url).map_err(|_| StartupCaptureError::InvalidEndpoint)?;
    if url.scheme() != "http" {
        return Err(StartupCaptureError::InvalidEndpoint);
    }
    let Some(host) = url.host_str() else {
        return Err(StartupCaptureError::InvalidEndpoint);
    };
    if host != "127.0.0.1" && host != "localhost" {
        return Err(StartupCaptureError::InvalidEndpoint);
    }
    let Some(port) = url.port() else {
        return Err(StartupCaptureError::InvalidEndpoint);
    };
    Ok(RemoteStartupCapture {
        remote_endpoint: format!("http://127.0.0.1:{port}"),
        link_passphrase,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn remote_startup_parser_accepts_plain_startup_lines() {
        let parsed = parse_remote_startup_metadata(
            "ws-dashboard owner pairing URL: http://127.0.0.1:49170/pair?token=redacted\nws-dashboard remote link passphrase: secret\n",
        )
        .expect("parse startup output");

        assert_eq!(parsed.remote_endpoint, "http://127.0.0.1:49170");
        assert_eq!(parsed.link_passphrase.as_deref(), Some("secret"));
    }

    #[test]
    fn remote_startup_parser_accepts_powershell_native_stderr_prefix() {
        let parsed = parse_remote_startup_metadata(
            "ws-dashboard.exe : ws-dashboard owner pairing URL: http://127.0.0.1:60437/pair?token=redacted\nws-dashboard remote link passphrase: secret\n",
        )
        .expect("parse PowerShell-wrapped startup output");

        assert_eq!(parsed.remote_endpoint, "http://127.0.0.1:60437");
        assert_eq!(parsed.link_passphrase.as_deref(), Some("secret"));
    }
}
