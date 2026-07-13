//! Codex `app-server` read/write adapter (Phase 2 of
//! `260620-feat-ws-dashboard-agent-client-activity-sources`).
//!
//! This module owns the async side of the Codex adapter: a JSON-RPC-over-stdio
//! duplex transport over a `codex app-server --stdio` child process, a
//! server-scoped session registry, the pre-spawn ws/wsflow plugin-presence
//! gate, and the `AgentClientProvider` bridge. The pure event->transcript
//! projection lives in `ws-dashboard-core::codex_projection` and is fed from
//! here.
//!
//! CONTRACT (Finding B): framing is newline-delimited JSON in both directions
//! (NOT LSP `Content-Length`). No `jsonrpc`/`jsonrpsee`/`tokio-util` crate is
//! added; `AsyncBufReadExt::lines()` + `serde_json` cover the confirmed NDJSON
//! framing with zero dependency delta.
//!
//! CONTRACT (browser identity): provider `thread.id`/`sessionId`, turn ids,
//! item ids, `thread.path`, `codexHome`, `installationId`, and raw event JSON
//! stay daemon-private. Only the dashboard-owned `activityId` and
//! projected/derived Activity content cross the browser boundary.

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::{Arc, Mutex as StdMutex, RwLock, Weak};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use serde::Deserialize;
use serde_json::{json, Value};
use tokio::io::{AsyncBufReadExt, AsyncRead, AsyncWrite, AsyncWriteExt, BufReader};
use tokio::process::{Child, Command};
use tokio::sync::{mpsc, oneshot, Mutex as AsyncMutex};
use ws_dashboard_core::activity::{
    ActivityItem, ActivitySourceDisplay, ActivityTranscript, ActivityTranscriptAvailability,
};
use ws_dashboard_core::agent_client_provider::{
    AgentClientCapabilities, AgentClientInitializeRequest, AgentClientInitializeResult,
    AgentClientInterruptRequest, AgentClientProvider, AgentClientProviderError,
    AgentClientProviderMetadata, AgentClientPromptSendRequest, AgentClientPromptSendResult,
    AgentClientSessionCreateRequest, AgentClientSessionCreateResult, AgentClientSessionListRequest,
    AgentClientSessionListResult, AgentClientSessionResumeRequest,
    AgentClientTranscriptBackfillRequest, AgentClientTranscriptBackfillResult,
};
use ws_dashboard_core::codex_projection::{project_fork_turns, CodexProjector};
use ws_dashboard_core::WorkRootId;

/// Dashboard-owned provider discriminator (never the raw binary name).
pub const CODEX_PROVIDER: &str = "codex";
/// `ActivityItem`/source `kind` for a Codex interactive session.
pub const CODEX_ACTIVITY_KIND: &str = "agent.codex";
const CODEX_ACTIVITY_ID_PREFIX: &str = "codex:";

const DEFAULT_REQUEST_TIMEOUT: Duration = Duration::from_secs(120);
const MAX_CODEX_SESSIONS: usize = 16;
const MAX_STDERR_LINES: usize = 256;

// ---------------------------------------------------------------------------
// Message classification (three-way, per Finding A)
// ---------------------------------------------------------------------------

/// A JSON-RPC error object.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct JsonRpcError {
    pub code: i64,
    pub message: String,
}

/// One classified incoming JSON-RPC line. The reader loop MUST distinguish
/// responses to our own requests (`id`, `result`/`error`, no `method`) from
/// fire-and-forget notifications (`method`, no `id`) and server-initiated
/// requests such as approvals (`method` AND `id`).
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum CodexIncoming {
    Response {
        id: i64,
        result: Result<Value, JsonRpcError>,
    },
    Notification {
        method: String,
        params: Value,
    },
    ServerRequest {
        id: Value,
        method: String,
        params: Value,
    },
    Malformed,
}

/// Classify a single NDJSON line into the three-way protocol space.
pub fn classify_incoming(line: &str) -> CodexIncoming {
    let Ok(value) = serde_json::from_str::<Value>(line) else {
        return CodexIncoming::Malformed;
    };
    let method = value.get("method").and_then(Value::as_str);
    let id = value.get("id");
    match (method, id) {
        (Some(method), Some(id)) => CodexIncoming::ServerRequest {
            id: id.clone(),
            method: method.to_owned(),
            params: value.get("params").cloned().unwrap_or(Value::Null),
        },
        (Some(method), None) => CodexIncoming::Notification {
            method: method.to_owned(),
            params: value.get("params").cloned().unwrap_or(Value::Null),
        },
        (None, Some(id)) => {
            let Some(id) = id.as_i64() else {
                return CodexIncoming::Malformed;
            };
            if let Some(error) = value.get("error") {
                CodexIncoming::Response {
                    id,
                    result: Err(JsonRpcError {
                        code: error.get("code").and_then(Value::as_i64).unwrap_or(0),
                        message: error
                            .get("message")
                            .and_then(Value::as_str)
                            .unwrap_or("codex error")
                            .to_owned(),
                    }),
                }
            } else {
                CodexIncoming::Response {
                    id,
                    result: Ok(value.get("result").cloned().unwrap_or(Value::Null)),
                }
            }
        }
        (None, None) => CodexIncoming::Malformed,
    }
}

// ---------------------------------------------------------------------------
// Transport
// ---------------------------------------------------------------------------

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum CodexTransportError {
    Io(String),
    Timeout,
    Closed,
    Rpc { code: i64, message: String },
    SteerTurnMismatch,
}

impl CodexTransportError {
    fn provider_code(&self) -> &'static str {
        match self {
            CodexTransportError::Io(_) => "codex.io",
            CodexTransportError::Timeout => "codex.timeout",
            CodexTransportError::Closed => "codex.closed",
            CodexTransportError::Rpc { .. } => "codex.rpc",
            CodexTransportError::SteerTurnMismatch => "codex.steer_turn_mismatch",
        }
    }

    /// Bounded, provider-neutral message. Never carries raw transport payloads.
    fn provider_message(&self) -> String {
        match self {
            CodexTransportError::Io(_) => "codex transport io error".to_owned(),
            CodexTransportError::Timeout => "codex request timed out".to_owned(),
            CodexTransportError::Closed => "codex session closed".to_owned(),
            CodexTransportError::Rpc { code, .. } => format!("codex rpc error {code}"),
            CodexTransportError::SteerTurnMismatch => {
                "steer target turn is no longer active".to_owned()
            }
        }
    }
}

impl From<CodexTransportError> for AgentClientProviderError {
    fn from(error: CodexTransportError) -> Self {
        AgentClientProviderError {
            code: error.provider_code().to_owned(),
            message: error.provider_message(),
        }
    }
}

#[derive(Default)]
struct TurnState {
    active_turn_id: Option<String>,
}

type PendingMap = Arc<StdMutex<HashMap<i64, oneshot::Sender<Result<Value, JsonRpcError>>>>>;

/// One live JSON-RPC-over-stdio duplex. Owns the write half behind an async
/// mutex, correlates responses through a pending map, and fans notifications
/// out to a channel for the projector.
pub struct CodexConnection {
    writer: AsyncMutex<Box<dyn AsyncWrite + Unpin + Send>>,
    next_id: AtomicI64,
    pending: PendingMap,
    turn_state: AsyncMutex<TurnState>,
    request_timeout: Duration,
    stderr_tail: Arc<StdMutex<Vec<String>>>,
    // Held to keep the child alive and kill it on drop (kill_on_drop). Never
    // read directly; RAII ownership is the point.
    #[allow(dead_code)]
    child: StdMutex<Option<Child>>,
}

impl CodexConnection {
    /// Build a connection from arbitrary async byte streams. Used by tests with
    /// an in-process NDJSON peer and by `spawn` with a real child process.
    /// Returns the connection plus the notification receiver a session drains
    /// into its projector.
    pub fn from_io<R, W>(
        reader: R,
        writer: W,
        request_timeout: Duration,
    ) -> (Arc<Self>, mpsc::UnboundedReceiver<(String, Value)>)
    where
        R: AsyncRead + Unpin + Send + 'static,
        W: AsyncWrite + Unpin + Send + 'static,
    {
        let pending: PendingMap = Arc::new(StdMutex::new(HashMap::new()));
        let connection = Arc::new(Self {
            writer: AsyncMutex::new(Box::new(writer)),
            next_id: AtomicI64::new(1),
            pending: pending.clone(),
            turn_state: AsyncMutex::new(TurnState::default()),
            request_timeout,
            stderr_tail: Arc::new(StdMutex::new(Vec::new())),
            child: StdMutex::new(None),
        });
        let (notif_tx, notif_rx) = mpsc::unbounded_channel();
        // CONTRACT: the reader holds only a Weak reference. A strong Arc here
        // would form a cycle (connection owns the Child; the reader lives until
        // stdout EOF; the child only exits when killed on Child drop) that
        // would leak the child process. With a Weak, dropping the session/
        // registry drops the connection, which drops the Child, which
        // kill_on_drop terminates, which closes stdout and ends the reader.
        tokio::spawn(reader_loop(
            reader,
            pending,
            notif_tx,
            Arc::downgrade(&connection),
        ));
        (connection, notif_rx)
    }

    fn set_child(&self, child: Child) {
        *self.child.lock().expect("codex child lock poisoned") = Some(child);
    }

    async fn write_message(&self, message: &Value) -> Result<(), CodexTransportError> {
        let mut line = serde_json::to_string(message).map_err(|error| {
            CodexTransportError::Io(format!("serialize codex request: {error}"))
        })?;
        line.push('\n');
        let mut writer = self.writer.lock().await;
        writer
            .write_all(line.as_bytes())
            .await
            .map_err(|error| CodexTransportError::Io(error.to_string()))?;
        writer
            .flush()
            .await
            .map_err(|error| CodexTransportError::Io(error.to_string()))?;
        Ok(())
    }

    /// Fire-and-forget notification (e.g. `initialized`).
    pub async fn notify(&self, method: &str, params: Value) -> Result<(), CodexTransportError> {
        self.write_message(&json!({
            "jsonrpc": "2.0",
            "method": method,
            "params": params,
        }))
        .await
    }

    /// Send a request and await its correlated response with a timeout. The
    /// pending entry is registered before the write so a fast response cannot
    /// race the registration.
    pub async fn request(
        &self,
        method: &str,
        params: Value,
    ) -> Result<Value, CodexTransportError> {
        let id = self.next_id.fetch_add(1, Ordering::SeqCst);
        let (tx, rx) = oneshot::channel();
        self.pending
            .lock()
            .expect("codex pending lock poisoned")
            .insert(id, tx);

        if let Err(error) = self
            .write_message(&json!({
                "jsonrpc": "2.0",
                "id": id,
                "method": method,
                "params": params,
            }))
            .await
        {
            self.pending
                .lock()
                .expect("codex pending lock poisoned")
                .remove(&id);
            return Err(error);
        }

        match tokio::time::timeout(self.request_timeout, rx).await {
            Ok(Ok(Ok(value))) => Ok(value),
            Ok(Ok(Err(rpc))) => Err(CodexTransportError::Rpc {
                code: rpc.code,
                message: rpc.message,
            }),
            Ok(Err(_)) => Err(CodexTransportError::Closed),
            Err(_) => {
                self.pending
                    .lock()
                    .expect("codex pending lock poisoned")
                    .remove(&id);
                Err(CodexTransportError::Timeout)
            }
        }
    }

    /// `turn/steer`, gated on `expected_turn_id` matching the tracked in-flight
    /// turn. The check-and-send is serialized under the turn-state lock so a
    /// concurrent steer cannot interleave; the lock is released before awaiting
    /// the response to avoid deadlocking the reader (which needs the same lock
    /// to record turn completion).
    pub async fn steer(
        &self,
        thread_id: &str,
        expected_turn_id: &str,
        input: Value,
    ) -> Result<Value, CodexTransportError> {
        {
            let state = self.turn_state.lock().await;
            if state.active_turn_id.as_deref() != Some(expected_turn_id) {
                return Err(CodexTransportError::SteerTurnMismatch);
            }
        }
        self.request(
            "turn/steer",
            json!({
                "threadId": thread_id,
                "expectedTurnId": expected_turn_id,
                "input": input,
            }),
        )
        .await
    }

    /// Currently tracked in-flight turn id (daemon-private; test/introspection).
    pub async fn active_turn_id(&self) -> Option<String> {
        self.turn_state.lock().await.active_turn_id.clone()
    }

    async fn set_active_turn(&self, turn_id: Option<String>) {
        self.turn_state.lock().await.active_turn_id = turn_id;
    }
}

async fn reader_loop<R>(
    reader: R,
    pending: PendingMap,
    notifications: mpsc::UnboundedSender<(String, Value)>,
    connection: Weak<CodexConnection>,
) where
    R: AsyncRead + Unpin + Send + 'static,
{
    let mut lines = BufReader::new(reader).lines();
    loop {
        match lines.next_line().await {
            Ok(Some(line)) => {
                if line.trim().is_empty() {
                    continue;
                }
                match classify_incoming(&line) {
                    CodexIncoming::Response { id, result } => {
                        if let Some(sender) = pending
                            .lock()
                            .expect("codex pending lock poisoned")
                            .remove(&id)
                        {
                            let _ = sender.send(result);
                        }
                    }
                    CodexIncoming::Notification { method, params } => {
                        match method.as_str() {
                            "turn/started" => {
                                let turn_id = params
                                    .get("turn")
                                    .and_then(|turn| turn.get("id"))
                                    .and_then(Value::as_str)
                                    .map(str::to_owned);
                                if let Some(connection) = connection.upgrade() {
                                    connection.set_active_turn(turn_id).await;
                                }
                            }
                            "turn/completed" | "turn/failed" | "turn/aborted"
                            | "turn/interrupted" => {
                                if let Some(connection) = connection.upgrade() {
                                    connection.set_active_turn(None).await;
                                }
                            }
                            _ => {}
                        }
                        let _ = notifications.send((method, params));
                    }
                    CodexIncoming::ServerRequest { id, method, .. } => {
                        // CONTRACT: server-initiated requests (approvals) must
                        // be answered so the turn does not hang. Interactive
                        // approval relay is out of Phase 2 scope (we spawn with
                        // approvalPolicy=never); respond with a JSON-RPC error
                        // so no approval is implicitly granted.
                        if let Some(connection) = connection.upgrade() {
                            let _ = connection
                                .write_message(&json!({
                                    "jsonrpc": "2.0",
                                    "id": id,
                                    "error": {
                                        "code": -32601,
                                        "message": format!(
                                            "approval handling not enabled for {method}"
                                        ),
                                    },
                                }))
                                .await;
                        }
                    }
                    CodexIncoming::Malformed => {
                        // A malformed server line degrades silently at the
                        // transport layer; the projector surfaces a bounded
                        // diagnostic if it ever sees one via the notification
                        // path.
                    }
                }
            }
            Ok(None) => break,
            Err(_) => break,
        }
    }
    // EOF/error: fail all outstanding requests so callers do not hang.
    let mut pending = pending.lock().expect("codex pending lock poisoned");
    for (_, sender) in pending.drain() {
        let _ = sender.send(Err(JsonRpcError {
            code: 0,
            message: "codex connection closed".to_owned(),
        }));
    }
}

async fn drain_stderr<R>(reader: R, tail: Arc<StdMutex<Vec<String>>>)
where
    R: AsyncRead + Unpin + Send + 'static,
{
    let mut lines = BufReader::new(reader).lines();
    while let Ok(Some(line)) = lines.next_line().await {
        let mut tail = tail.lock().expect("codex stderr lock poisoned");
        tail.push(line);
        let overflow = tail.len().saturating_sub(MAX_STDERR_LINES);
        if overflow > 0 {
            tail.drain(0..overflow);
        }
    }
}

// ---------------------------------------------------------------------------
// Plugin-presence spawn gate (Finding C)
// ---------------------------------------------------------------------------

#[derive(Debug, Deserialize)]
struct PluginListOutput {
    #[serde(default)]
    installed: Vec<PluginListEntry>,
}

#[derive(Debug, Deserialize)]
struct PluginListEntry {
    #[serde(default)]
    name: String,
    #[serde(default)]
    installed: bool,
    #[serde(default)]
    enabled: bool,
}

/// Refusal returned when the ws/wsflow plugin-presence precondition fails.
/// Carries install guidance, not a silent degrade.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PluginGateRefusal {
    pub message: String,
}

impl From<PluginGateRefusal> for AgentClientProviderError {
    fn from(refusal: PluginGateRefusal) -> Self {
        AgentClientProviderError {
            code: "codex.plugin_gate".to_owned(),
            message: refusal.message,
        }
    }
}

const PLUGIN_GATE_GUIDANCE: &str = "Codex session refused: neither the `ws` nor `wsflow` plugin \
is installed and enabled for this project. Install and enable it (e.g. `codex plugin install \
wsflow`) before starting a dashboard-driven Codex session.";

/// Evaluate the pre-spawn plugin gate against `codex plugin list --json`
/// output. Passes iff some `installed[]` entry named `ws` or `wsflow` is both
/// installed and enabled.
pub fn evaluate_plugin_gate(plugin_list_json: &str) -> Result<(), PluginGateRefusal> {
    let parsed = serde_json::from_str::<PluginListOutput>(plugin_list_json).map_err(|_| {
        PluginGateRefusal {
            message: format!("{PLUGIN_GATE_GUIDANCE} (could not read plugin list)"),
        }
    })?;
    let satisfied = parsed.installed.iter().any(|entry| {
        matches!(entry.name.as_str(), "ws" | "wsflow") && entry.installed && entry.enabled
    });
    if satisfied {
        Ok(())
    } else {
        Err(PluginGateRefusal {
            message: PLUGIN_GATE_GUIDANCE.to_owned(),
        })
    }
}

/// Run `codex plugin list --json` and evaluate the gate. This is the correct
/// pre-spawn surface (no live app-server required), NOT the app-server
/// `plugin/list` RPC.
pub async fn check_plugin_gate(codex_bin: &str) -> Result<(), PluginGateRefusal> {
    let output = Command::new(codex_bin)
        .args(["plugin", "list", "--json"])
        .output()
        .await
        .map_err(|error| PluginGateRefusal {
            message: format!("{PLUGIN_GATE_GUIDANCE} (could not run codex: {error})"),
        })?;
    if !output.status.success() {
        return Err(PluginGateRefusal {
            message: format!("{PLUGIN_GATE_GUIDANCE} (codex plugin list failed)"),
        });
    }
    evaluate_plugin_gate(&String::from_utf8_lossy(&output.stdout))
}

// ---------------------------------------------------------------------------
// Session registry (server-scoped, mirroring TerminalRegistry lifecycle)
// ---------------------------------------------------------------------------

#[derive(Clone, Debug, Eq, Hash, PartialEq)]
struct CodexSessionKey {
    // CONTRACT (ticket #L550-L561): key by serverId, not workRootId alone, so
    // routing matches terminal.rs / servers.rs.
    server_id: String,
    activity_id: String,
}

/// A live Codex session: the transport, its private thread id, the accumulating
/// projector, and the owning work root/server for lifecycle cleanup.
pub struct CodexSession {
    activity_id: String,
    server_id: String,
    work_root_id: WorkRootId,
    thread_id: String,
    connection: Arc<CodexConnection>,
    projector: Arc<AsyncMutex<CodexProjector>>,
    // Retained for future recency ordering in the Activity feed merge.
    #[allow(dead_code)]
    created_at_ms: u64,
}

impl CodexSession {
    fn is_live(&self) -> bool {
        // A session with a still-open connection is considered live. The
        // reader loop drops pending senders on EOF, but the struct persists
        // until removed; treat presence as liveness for the cap/cleanup model.
        Arc::strong_count(&self.connection) > 0
    }

    pub fn activity_id(&self) -> &str {
        &self.activity_id
    }

    pub fn projector(&self) -> Arc<AsyncMutex<CodexProjector>> {
        self.projector.clone()
    }
}

#[derive(Clone, Default)]
pub struct CodexProviderRegistry {
    sessions: Arc<RwLock<HashMap<CodexSessionKey, Arc<CodexSession>>>>,
}

impl CodexProviderRegistry {
    fn key(server_id: &str, activity_id: &str) -> CodexSessionKey {
        CodexSessionKey {
            server_id: server_id.to_owned(),
            activity_id: activity_id.to_owned(),
        }
    }

    fn insert(&self, session: Arc<CodexSession>) -> Result<(), AgentClientProviderError> {
        let mut sessions = self.sessions.write().expect("codex registry lock poisoned");
        sessions.retain(|_, session| session.is_live());
        if sessions.len() >= MAX_CODEX_SESSIONS {
            return Err(AgentClientProviderError {
                code: "codex.too_many_sessions".to_owned(),
                message: "too many active Codex sessions".to_owned(),
            });
        }
        sessions.insert(
            Self::key(&session.server_id, &session.activity_id),
            session,
        );
        Ok(())
    }

    fn get(&self, server_id: &str, activity_id: &str) -> Option<Arc<CodexSession>> {
        self.sessions
            .read()
            .expect("codex registry lock poisoned")
            .get(&Self::key(server_id, activity_id))
            .cloned()
    }

    /// Public lookup for route handlers that need to project a live session's
    /// transcript. Returns the session behind an `Arc` so the caller can await
    /// its projector lock without holding the registry lock.
    pub fn session_for(&self, server_id: &str, activity_id: &str) -> Option<Arc<CodexSession>> {
        self.get(server_id, activity_id)
    }

    /// Session summaries for a work root under a server, for `list_sessions`.
    fn list_for_work_root(
        &self,
        server_id: &str,
        work_root_id: &WorkRootId,
    ) -> Vec<Arc<CodexSession>> {
        self.sessions
            .read()
            .expect("codex registry lock poisoned")
            .values()
            .filter(|session| {
                session.server_id == server_id
                    && &session.work_root_id == work_root_id
                    && session.is_live()
            })
            .cloned()
            .collect()
    }

    /// Remove a single session by key, returning it if present. Dropping the
    /// returned `Arc` (and any other holders) releases the connection, whose
    /// `Child` is killed on drop. Used to roll back a partially-created session
    /// whose initial turn failed so no orphaned child leaks against the cap.
    fn remove(&self, server_id: &str, activity_id: &str) -> Option<Arc<CodexSession>> {
        self.sessions
            .write()
            .expect("codex registry lock poisoned")
            .remove(&Self::key(server_id, activity_id))
    }

    /// Drop sessions for closed work roots (mirrors
    /// `TerminalRegistry::remove_for_work_roots`).
    pub fn remove_for_work_roots(
        &self,
        work_root_ids: &std::collections::BTreeSet<WorkRootId>,
    ) -> usize {
        let mut sessions = self.sessions.write().expect("codex registry lock poisoned");
        let before = sessions.len();
        sessions.retain(|_, session| !work_root_ids.contains(&session.work_root_id));
        before - sessions.len()
    }

    /// Test-support: insert a session backed by an arbitrary connection and a
    /// pre-populated projector so route/integration tests can drive the
    /// session-lookup and projection paths through the real HTTP handlers
    /// without spawning a real `codex app-server`. Mirrors the crate's other
    /// `*_for_tests` constructors.
    pub fn insert_session_for_tests(
        &self,
        server_id: impl Into<String>,
        activity_id: impl Into<String>,
        work_root_id: WorkRootId,
        thread_id: impl Into<String>,
        connection: Arc<CodexConnection>,
        projector: CodexProjector,
    ) -> Result<Arc<CodexSession>, AgentClientProviderError> {
        let session = Arc::new(CodexSession {
            activity_id: activity_id.into(),
            server_id: server_id.into(),
            work_root_id,
            thread_id: thread_id.into(),
            connection,
            projector: Arc::new(AsyncMutex::new(projector)),
            created_at_ms: now_ms(),
        });
        let handle = session.clone();
        self.insert(session)?;
        Ok(handle)
    }
}

// ---------------------------------------------------------------------------
// AgentClientProvider bridge
// ---------------------------------------------------------------------------

/// Resolves an opaque `WorkRootId` to a spawn cwd. The daemon implements this
/// over `AppState`; tests use a stub so the provider stays unit-testable.
pub trait CodexWorkRootResolver: Send + Sync {
    fn resolve_cwd(&self, work_root_id: &WorkRootId) -> Result<PathBuf, AgentClientProviderError>;
}

#[derive(Clone)]
pub struct CodexSpawnConfig {
    pub codex_bin: String,
    pub server_id: String,
    pub request_timeout: Duration,
}

impl CodexSpawnConfig {
    pub fn new(server_id: impl Into<String>) -> Self {
        Self {
            codex_bin: "codex".to_owned(),
            server_id: server_id.into(),
            request_timeout: DEFAULT_REQUEST_TIMEOUT,
        }
    }
}

/// Concrete `AgentClientProvider` backed by a `codex app-server` child process.
pub struct CodexAppServerProvider {
    config: CodexSpawnConfig,
    registry: CodexProviderRegistry,
    resolver: Arc<dyn CodexWorkRootResolver>,
}

impl CodexAppServerProvider {
    pub fn new(
        config: CodexSpawnConfig,
        registry: CodexProviderRegistry,
        resolver: Arc<dyn CodexWorkRootResolver>,
    ) -> Self {
        Self {
            config,
            registry,
            resolver,
        }
    }

    fn capabilities() -> AgentClientCapabilities {
        AgentClientCapabilities {
            compact: true,
            steer: true,
            goal: true,
            // rewind maps to the deprecated thread/rollback RPC; do not build UI
            // on a sunsetting surface (Finding D).
            rewind: false,
            fork: true,
            skills: true,
        }
    }

    /// Spawn a child `codex app-server --stdio`, run the handshake, and return
    /// the live connection plus notification receiver.
    async fn spawn_connection(
        &self,
        cwd: &PathBuf,
    ) -> Result<
        (Arc<CodexConnection>, mpsc::UnboundedReceiver<(String, Value)>),
        CodexTransportError,
    > {
        let mut child = Command::new(&self.config.codex_bin)
            .args(["app-server", "--stdio"])
            .current_dir(cwd)
            .stdin(std::process::Stdio::piped())
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .kill_on_drop(true)
            .spawn()
            .map_err(|error| CodexTransportError::Io(error.to_string()))?;

        let stdin = child
            .stdin
            .take()
            .ok_or_else(|| CodexTransportError::Io("codex stdin unavailable".to_owned()))?;
        let stdout = child
            .stdout
            .take()
            .ok_or_else(|| CodexTransportError::Io("codex stdout unavailable".to_owned()))?;
        let stderr = child.stderr.take();

        let (connection, notifications) =
            CodexConnection::from_io(stdout, stdin, self.config.request_timeout);
        if let Some(stderr) = stderr {
            tokio::spawn(drain_stderr(stderr, connection.stderr_tail.clone()));
        }
        connection.set_child(child);

        // Handshake: initialize -> await result -> initialized notification.
        connection
            .request(
                "initialize",
                json!({
                    "clientInfo": {
                        "name": "ws-dashboard",
                        "version": env!("CARGO_PKG_VERSION"),
                    }
                }),
            )
            .await?;
        connection.notify("initialized", json!({})).await?;

        Ok((connection, notifications))
    }
}

fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|elapsed| elapsed.as_millis() as u64)
        .unwrap_or(0)
}

fn new_activity_id() -> String {
    // Dashboard-owned id; not derived from any provider id.
    format!("{CODEX_ACTIVITY_ID_PREFIX}{:x}", now_ms())
}

/// Build the browser-facing source display for a Codex session. Carries the
/// dashboard-owned discriminator and display-only fields, never provider ids.
pub fn codex_source_display(label: &str) -> ActivitySourceDisplay {
    ActivitySourceDisplay {
        kind: CODEX_ACTIVITY_KIND.to_owned(),
        label: label.to_owned(),
        backend: Some(CODEX_PROVIDER.to_owned()),
        harness: Some(CODEX_PROVIDER.to_owned()),
        tier: Some("core".to_owned()),
        model: None,
    }
}

/// Project a live Codex session into a browser-facing `ActivityItem` for the
/// `ActivityFeed.items` merge. CONTRACT: this must be merged into
/// `ActivityFeed.items`, never `agents`, and must omit provider ids/paths.
pub async fn codex_activity_item(session: &CodexSession) -> ActivityItem {
    let projector = session.projector.lock().await;
    let live = projector.is_turn_active();
    let diagnostics = projector.diagnostics().to_vec();
    let has_transcript = !projector.transcript_blocks().is_empty();
    let status = if live { "running" } else { "idle" }.to_owned();
    let mut metadata = std::collections::BTreeMap::new();
    if let Some(usage) = projector.usage() {
        if let Ok(usage) = serde_json::to_value(usage) {
            metadata.insert("usage".to_owned(), usage);
        }
    }
    ActivityItem {
        id: session.activity_id.clone(),
        kind: CODEX_ACTIVITY_KIND.to_owned(),
        label: "Codex session".to_owned(),
        status,
        live,
        attention: !diagnostics.is_empty(),
        started_at: None,
        updated_at: None,
        finished_at: None,
        source: codex_source_display("Codex session"),
        transcript: ActivityTranscriptAvailability {
            status: if has_transcript { "available" } else { "empty" }.to_owned(),
            available: has_transcript,
            cursor: has_transcript.then(|| "0".to_owned()),
        },
        diagnostics,
        metadata,
    }
}

/// Project a live Codex session's transcript for the browser. CONTRACT: only
/// projected blocks + dashboard-owned `activityId` cross; the private thread id
/// is collapsed to nothing.
pub async fn codex_activity_transcript(
    session: &CodexSession,
    work_root_id: WorkRootId,
) -> ActivityTranscript {
    let projector = session.projector.lock().await;
    let blocks = projector.transcript_blocks();
    let live = projector.is_turn_active();
    let diagnostics = projector.diagnostics().to_vec();
    let degraded = projector.degraded();
    ActivityTranscript {
        work_root_id,
        activity_id: session.activity_id.clone(),
        status: if degraded { "degraded" } else { "available" }.to_owned(),
        source_status: if degraded { "degraded" } else { "ok" }.to_owned(),
        live,
        source: codex_source_display("Codex session"),
        blocks,
        next_cursor: None,
        has_more: false,
        diagnostics,
    }
}

const MAX_SKILLS_PROJECTED: usize = 256;
const MAX_SKILL_FIELD_LEN: usize = 280;

/// Project a raw Codex `skills/list` response into a bounded, path-free display
/// shape. CONTRACT: only per-skill display `name`/`title`/`description` cross
/// the browser boundary; provider `source`/`cwd`/filesystem paths and every
/// other field are dropped, and the entry count is capped.
fn project_skills_list(raw: &Value) -> Value {
    let entries = raw
        .get("data")
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[]);
    let skills = entries
        .iter()
        .take(MAX_SKILLS_PROJECTED)
        .map(|entry| {
            let mut projected = serde_json::Map::new();
            for field in ["name", "title", "description"] {
                if let Some(text) = entry.get(field).and_then(Value::as_str) {
                    projected.insert(
                        field.to_owned(),
                        Value::String(bound_display(text, MAX_SKILL_FIELD_LEN)),
                    );
                }
            }
            Value::Object(projected)
        })
        .collect::<Vec<_>>();
    json!({ "count": skills.len(), "skills": skills })
}

/// Bounded, char-boundary-safe truncation for projected display strings.
fn bound_display(text: &str, max: usize) -> String {
    if text.len() <= max {
        return text.to_owned();
    }
    let mut end = max;
    while end > 0 && !text.is_char_boundary(end) {
        end -= 1;
    }
    let mut bounded = text[..end].to_owned();
    bounded.push('…');
    bounded
}

/// Collect browser-facing Codex `ActivityItem`s for the sessions of one work
/// root under a server, for merging into the unified `ActivityFeed.items`.
/// CONTRACT: these rows belong in `items`, never the legacy `agents`
/// projection, and carry no provider ids/paths (see `codex_activity_item`).
pub async fn codex_activity_items(
    registry: &CodexProviderRegistry,
    server_id: &str,
    work_root_id: &WorkRootId,
) -> Vec<ActivityItem> {
    let sessions = registry.list_for_work_root(server_id, work_root_id);
    let mut items = Vec::with_capacity(sessions.len());
    for session in sessions {
        items.push(codex_activity_item(&session).await);
    }
    items
}

/// Spawn the projector-updating task: drain notifications into the session's
/// projector under its lock.
fn spawn_projector_pump(
    projector: Arc<AsyncMutex<CodexProjector>>,
    mut notifications: mpsc::UnboundedReceiver<(String, Value)>,
) {
    tokio::spawn(async move {
        while let Some((method, params)) = notifications.recv().await {
            let message = json!({ "method": method, "params": params });
            let mut projector = projector.lock().await;
            projector.ingest_value(&message);
        }
    });
}

impl AgentClientProvider for CodexAppServerProvider {
    async fn initialize(
        &self,
        _request: AgentClientInitializeRequest,
    ) -> Result<AgentClientInitializeResult, AgentClientProviderError> {
        // Pre-spawn plugin-presence gate.
        check_plugin_gate(&self.config.codex_bin).await?;
        Ok(AgentClientInitializeResult {
            metadata: AgentClientProviderMetadata {
                provider: CODEX_PROVIDER.to_owned(),
                version: None,
                capabilities: Self::capabilities(),
            },
        })
    }

    async fn list_sessions(
        &self,
        request: AgentClientSessionListRequest,
    ) -> Result<AgentClientSessionListResult, AgentClientProviderError> {
        let sessions = self
            .registry
            .list_for_work_root(&self.config.server_id, &request.work_root_id);
        let mut summaries = Vec::with_capacity(sessions.len());
        for session in sessions {
            let projector = session.projector.lock().await;
            summaries.push(ws_dashboard_core::agent_client_provider::AgentClientSessionSummary {
                activity_id: session.activity_id.clone(),
                label: "Codex session".to_owned(),
                status: if projector.is_turn_active() {
                    "running"
                } else {
                    "idle"
                }
                .to_owned(),
                updated_at: None,
            });
        }
        Ok(AgentClientSessionListResult {
            sessions: summaries,
        })
    }

    async fn create_session(
        &self,
        request: AgentClientSessionCreateRequest,
    ) -> Result<AgentClientSessionCreateResult, AgentClientProviderError> {
        // Enforce the plugin gate before spawning any process.
        check_plugin_gate(&self.config.codex_bin).await?;
        let cwd = self.resolver.resolve_cwd(&request.work_root_id)?;

        let (connection, notifications) = self.spawn_connection(&cwd).await?;

        let thread_result = connection
            .request(
                "thread/start",
                json!({
                    "cwd": cwd.to_string_lossy(),
                    "approvalPolicy": "never",
                }),
            )
            .await?;
        let thread_id = thread_result
            .get("thread")
            .and_then(|thread| thread.get("id"))
            .and_then(Value::as_str)
            .ok_or_else(|| AgentClientProviderError {
                code: "codex.thread_start".to_owned(),
                message: "codex thread id missing".to_owned(),
            })?
            .to_owned();

        let projector = Arc::new(AsyncMutex::new(CodexProjector::new()));
        spawn_projector_pump(projector.clone(), notifications);

        let activity_id = new_activity_id();
        let session = Arc::new(CodexSession {
            activity_id: activity_id.clone(),
            server_id: self.config.server_id.clone(),
            work_root_id: request.work_root_id.clone(),
            thread_id: thread_id.clone(),
            connection: connection.clone(),
            projector: projector.clone(),
            created_at_ms: now_ms(),
        });
        self.registry.insert(session)?;

        if let Some(prompt) = request.initial_prompt.filter(|prompt| !prompt.is_empty()) {
            {
                let mut projector = projector.lock().await;
                projector.suppress_local_prompt(prompt.clone());
            }
            if let Err(error) = connection
                .request(
                    "turn/start",
                    json!({
                        "threadId": thread_id,
                        "input": [{ "type": "text", "text": prompt }],
                    }),
                )
                .await
            {
                // CONTRACT: the initial-turn failure must not leave an orphaned
                // session + live child registered against MAX_CODEX_SESSIONS.
                // Roll the registry insert back; dropping the session (and the
                // local connection Arc on return) kills the child on drop.
                self.registry
                    .remove(&self.config.server_id, &activity_id);
                return Err(error.into());
            }
        }

        Ok(AgentClientSessionCreateResult { activity_id })
    }

    async fn resume_session(
        &self,
        request: AgentClientSessionResumeRequest,
    ) -> Result<(), AgentClientProviderError> {
        self.session(&request.activity_id).map(|_| ())
    }

    async fn send_prompt(
        &self,
        request: AgentClientPromptSendRequest,
    ) -> Result<AgentClientPromptSendResult, AgentClientProviderError> {
        let session = self.session(&request.activity_id)?;
        {
            let mut projector = session.projector.lock().await;
            projector.suppress_local_prompt(request.text.clone());
        }
        session
            .connection
            .request(
                "turn/start",
                json!({
                    "threadId": session.thread_id,
                    "input": [{ "type": "text", "text": request.text }],
                }),
            )
            .await?;
        Ok(AgentClientPromptSendResult { accepted: true })
    }

    async fn interrupt(
        &self,
        request: AgentClientInterruptRequest,
    ) -> Result<(), AgentClientProviderError> {
        let session = self.session(&request.activity_id)?;
        session
            .connection
            .request(
                "turn/interrupt",
                json!({ "threadId": session.thread_id }),
            )
            .await?;
        Ok(())
    }

    async fn backfill_transcript(
        &self,
        request: AgentClientTranscriptBackfillRequest,
    ) -> Result<AgentClientTranscriptBackfillResult, AgentClientProviderError> {
        let session = self.session(&request.activity_id)?;
        let projector = session.projector.lock().await;
        Ok(AgentClientTranscriptBackfillResult {
            blocks: projector.transcript_blocks(),
            next_cursor: None,
            has_more: false,
        })
    }
}

impl CodexAppServerProvider {
    fn session(&self, activity_id: &str) -> Result<Arc<CodexSession>, AgentClientProviderError> {
        self.registry
            .get(&self.config.server_id, activity_id)
            .ok_or_else(|| AgentClientProviderError {
                code: "codex.unknown_session".to_owned(),
                message: "unknown Codex session".to_owned(),
            })
    }

    /// Codex-native `skills/list` control (Passthrough capability). Bounded
    /// display data only. CONTRACT: the raw `skills/list` response may carry
    /// per-skill filesystem sources, cwds, and other provider-private fields;
    /// only projected, path-free display names/descriptions cross the browser
    /// boundary (see `project_skills_list`).
    pub async fn skills_list(
        &self,
        activity_id: &str,
    ) -> Result<Value, AgentClientProviderError> {
        let session = self.session(activity_id)?;
        let raw = session
            .connection
            .request("skills/list", json!({}))
            .await?;
        Ok(project_skills_list(&raw))
    }

    /// Codex-native `thread/compact/start` control.
    pub async fn compact(&self, activity_id: &str) -> Result<(), AgentClientProviderError> {
        let session = self.session(activity_id)?;
        session
            .connection
            .request(
                "thread/compact/start",
                json!({ "threadId": session.thread_id }),
            )
            .await?;
        Ok(())
    }

    /// Codex-native `turn/steer` control, gated on the active turn id.
    pub async fn steer(
        &self,
        activity_id: &str,
        text: &str,
    ) -> Result<(), AgentClientProviderError> {
        let session = self.session(activity_id)?;
        let expected = session
            .connection
            .active_turn_id()
            .await
            .ok_or(CodexTransportError::SteerTurnMismatch)?;
        session
            .connection
            .steer(
                &session.thread_id,
                &expected,
                json!([{ "type": "text", "text": text }]),
            )
            .await?;
        Ok(())
    }

    /// Codex-native fork-from-here: `thread/fork` loaded against the source
    /// session's `thread_id`, cut at the turn `cut_cursor` resolves to (or
    /// the whole thread when `cut_cursor` is `None`). Registers a brand-new
    /// `CodexSession` (own `activity_id`, own `thread_id`, own connection)
    /// rather than reusing the source session's connection: `thread/fork`'s
    /// schema loads the thread from disk by id (the source connection does
    /// not need to be alive), and sharing one connection across two threads
    /// would require demultiplexing notifications by `threadId`, which this
    /// crate's one-connection-per-projector pump does not support. Returns
    /// `(new_activity_id, resolved_last_turn_id)`, where the second element is
    /// the turn id actually passed to `thread/fork` (i.e. `None` when
    /// `cut_cursor` failed to resolve to a known turn, signalling that the
    /// fork is an unfiltered full-thread fork rather than echoing back the
    /// caller's original, possibly-unresolvable `cut_cursor`).
    pub async fn fork(
        &self,
        activity_id: &str,
        cut_cursor: Option<&str>,
    ) -> Result<(String, Option<String>), AgentClientProviderError> {
        let session = self.session(activity_id)?;
        let last_turn_id = match cut_cursor {
            Some(cursor) => session.projector.lock().await.turn_id_for_cursor(cursor),
            None => None,
        };

        // Same hard-spawn-precondition as any new process (mirrors
        // `create_session`).
        check_plugin_gate(&self.config.codex_bin).await?;
        let cwd = self.resolver.resolve_cwd(&session.work_root_id)?;
        let (connection, notifications) = self.spawn_connection(&cwd).await?;

        let fork_result = connection
            .request(
                "thread/fork",
                json!({
                    "threadId": session.thread_id,
                    "lastTurnId": last_turn_id,
                }),
            )
            .await?;
        let thread_value = fork_result.get("thread").cloned().unwrap_or(Value::Null);
        let new_thread_id = thread_value
            .get("id")
            .and_then(Value::as_str)
            .ok_or_else(|| AgentClientProviderError {
                code: "codex.thread_fork".to_owned(),
                message: "codex forked thread id missing".to_owned(),
            })?
            .to_owned();

        // Seed the new projector from the forked thread's inline turns/items
        // so the browser sees the correct cut-point history immediately,
        // rather than an empty transcript for a thread that provider-side
        // already has content.
        let seeded_blocks = project_fork_turns(&thread_value);
        let projector = Arc::new(AsyncMutex::new(CodexProjector::seeded(seeded_blocks)));
        spawn_projector_pump(projector.clone(), notifications);

        let forked_activity_id = new_activity_id();
        let forked_session = Arc::new(CodexSession {
            activity_id: forked_activity_id.clone(),
            server_id: session.server_id.clone(),
            work_root_id: session.work_root_id.clone(),
            thread_id: new_thread_id,
            connection,
            projector,
            created_at_ms: now_ms(),
        });
        self.registry.insert(forked_session)?;

        Ok((forked_activity_id, last_turn_id))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::io::{duplex, AsyncBufReadExt, AsyncWriteExt, BufReader};

    #[test]
    fn classifies_response_notification_and_server_request() {
        match classify_incoming(r#"{"id":3,"result":{"turn":{"id":"t"}}}"#) {
            CodexIncoming::Response { id, result } => {
                assert_eq!(id, 3);
                assert!(result.is_ok());
            }
            other => panic!("expected response, got {other:?}"),
        }
        match classify_incoming(r#"{"id":5,"error":{"code":-32000,"message":"boom"}}"#) {
            CodexIncoming::Response { id, result } => {
                assert_eq!(id, 5);
                assert_eq!(
                    result,
                    Err(JsonRpcError {
                        code: -32000,
                        message: "boom".to_owned()
                    })
                );
            }
            other => panic!("expected error response, got {other:?}"),
        }
        match classify_incoming(r#"{"method":"turn/started","params":{"turn":{"id":"t"}}}"#) {
            CodexIncoming::Notification { method, .. } => assert_eq!(method, "turn/started"),
            other => panic!("expected notification, got {other:?}"),
        }
        match classify_incoming(
            r#"{"id":9,"method":"item/commandExecution/requestApproval","params":{}}"#,
        ) {
            CodexIncoming::ServerRequest { method, .. } => {
                assert_eq!(method, "item/commandExecution/requestApproval");
            }
            other => panic!("expected server request, got {other:?}"),
        }
        assert_eq!(classify_incoming("not json"), CodexIncoming::Malformed);
        assert_eq!(classify_incoming("{}"), CodexIncoming::Malformed);
    }

    #[tokio::test]
    async fn request_response_correlation_resolves() {
        // In-process NDJSON peer: reads one request, replies with a result.
        let (client_side, mut server_side) = duplex(4096);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, _notifications) =
            CodexConnection::from_io(client_read, client_write, Duration::from_secs(5));

        let peer = tokio::spawn(async move {
            let (server_read, mut server_write) = tokio::io::split(&mut server_side);
            let mut lines = BufReader::new(server_read).lines();
            let line = lines.next_line().await.unwrap().unwrap();
            let request: Value = serde_json::from_str(&line).unwrap();
            let id = request["id"].as_i64().unwrap();
            let reply = format!("{{\"id\":{id},\"result\":{{\"ok\":true}}}}\n");
            server_write.write_all(reply.as_bytes()).await.unwrap();
            server_write.flush().await.unwrap();
            // Keep the peer alive so the duplex is not closed prematurely.
            tokio::time::sleep(Duration::from_millis(200)).await;
        });

        let result = connection.request("thread/start", json!({})).await.unwrap();
        assert_eq!(result["ok"], true);
        peer.await.unwrap();
    }

    #[tokio::test]
    async fn request_times_out_when_peer_silent() {
        let (client_side, _server_side) = duplex(4096);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, _notifications) =
            CodexConnection::from_io(client_read, client_write, Duration::from_millis(80));
        let error = connection.request("thread/start", json!({})).await.unwrap_err();
        assert_eq!(error, CodexTransportError::Timeout);
    }

    #[tokio::test]
    async fn steer_rejects_stale_expected_turn_id() {
        let (client_side, _server_side) = duplex(4096);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, _notifications) =
            CodexConnection::from_io(client_read, client_write, Duration::from_secs(5));
        connection.set_active_turn(Some("turn-current".to_owned())).await;
        let error = connection
            .steer("thread-1", "turn-stale", json!([]))
            .await
            .unwrap_err();
        assert_eq!(error, CodexTransportError::SteerTurnMismatch);
    }

    #[test]
    fn plugin_gate_passes_when_wsflow_installed_and_enabled() {
        let json = r#"{"installed":[{"name":"wsflow","installed":true,"enabled":true}]}"#;
        assert!(evaluate_plugin_gate(json).is_ok());
    }

    #[test]
    fn plugin_gate_refuses_with_guidance_when_neither_present() {
        let json = r#"{"installed":[{"name":"other","installed":true,"enabled":true}]}"#;
        let refusal = evaluate_plugin_gate(json).unwrap_err();
        assert!(refusal.message.contains("ws"));
        assert!(refusal.message.contains("wsflow"));
    }

    #[test]
    fn plugin_gate_refuses_when_present_but_disabled() {
        let json = r#"{"installed":[{"name":"ws","installed":true,"enabled":false}]}"#;
        assert!(evaluate_plugin_gate(json).is_err());
    }

    #[test]
    fn plugin_gate_refusal_becomes_provider_error() {
        let refusal = evaluate_plugin_gate("not json").unwrap_err();
        let error: AgentClientProviderError = refusal.into();
        assert_eq!(error.code, "codex.plugin_gate");
    }

    #[test]
    fn skills_list_projection_omits_paths_and_extra_fields() {
        let raw = json!({
            "data": [
                {
                    "name": "do-thing",
                    "description": "does a thing",
                    "source": "/home/x/.codex/skills/do-thing/SKILL.md",
                    "cwd": "/private/host"
                },
                {
                    "name": "other",
                    "title": "Other",
                    "extra": {"secret": "/home/x/.codex/leak.jsonl"}
                }
            ]
        });
        let projected = project_skills_list(&raw);
        assert_eq!(projected["count"], 2);
        let body = serde_json::to_string(&projected).expect("serialize projected skills");
        // Display fields cross.
        assert!(body.contains("do-thing"));
        assert!(body.contains("does a thing"));
        assert!(body.contains("Other"));
        // Provider paths / private fields never cross.
        for forbidden in [
            "/home/x/.codex",
            "SKILL.md",
            "/private/host",
            "leak.jsonl",
            "source",
            "cwd",
            "extra",
            "secret",
        ] {
            assert!(!body.contains(forbidden), "skills projection leaked {forbidden}: {body}");
        }
    }

    #[test]
    fn skills_list_projection_bounds_field_length() {
        let long = "x".repeat(MAX_SKILL_FIELD_LEN + 100);
        let raw = json!({ "data": [{ "name": long }] });
        let projected = project_skills_list(&raw);
        let name = projected["skills"][0]["name"].as_str().expect("name string");
        assert!(name.chars().count() <= MAX_SKILL_FIELD_LEN + 1);
        assert!(name.ends_with('…'));
    }

    #[tokio::test]
    async fn registry_remove_frees_session_slot() {
        let registry = CodexProviderRegistry::default();
        let (client_side, _server_side) = duplex(1024);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, _notifications) =
            CodexConnection::from_io(client_read, client_write, Duration::from_secs(1));
        registry
            .insert_session_for_tests(
                "server-local",
                "codex:slot",
                WorkRootId::from("wr-slot"),
                "thread-private",
                connection,
                CodexProjector::new(),
            )
            .expect("seed session");
        assert!(registry.session_for("server-local", "codex:slot").is_some());
        assert!(registry.remove("server-local", "codex:slot").is_some());
        assert!(registry.session_for("server-local", "codex:slot").is_none());
        assert!(registry.remove("server-local", "codex:slot").is_none());
    }

    fn test_session(projector: CodexProjector) -> Arc<CodexSession> {
        let (client_side, _server_side) = duplex(1024);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, _notifications) =
            CodexConnection::from_io(client_read, client_write, Duration::from_secs(1));
        // Deliberately private, leak-prone values that must NOT reach the
        // browser payload.
        Arc::new(CodexSession {
            activity_id: "codex:abc123".to_owned(),
            server_id: "server-local".to_owned(),
            work_root_id: WorkRootId::from("root-local-xyz"),
            thread_id: "019f5040-secret-thread-and-sessionid".to_owned(),
            connection,
            projector: Arc::new(AsyncMutex::new(projector)),
            created_at_ms: 0,
        })
    }

    #[tokio::test]
    async fn browser_payloads_omit_provider_ids_and_paths() {
        let mut projector = CodexProjector::new();
        projector.ingest_value(&json!({
            "method": "item/completed",
            "params": {"item": {"type": "agentMessage", "id": "msg_secret_item_id", "text": "hello"}}
        }));
        // Inject a leak-prone unknown item carrying a private session path.
        projector.ingest_value(&json!({
            "method": "item/completed",
            "params": {"item": {
                "type": "mysteryLeak",
                "id": "leak_item",
                "path": "/home/swkang/.codex/sessions/2026/07/rollout-secret.jsonl",
                "codexHome": "/home/swkang/.codex"
            }}
        }));
        let session = test_session(projector);

        let item = codex_activity_item(&session).await;
        let transcript =
            codex_activity_transcript(&session, WorkRootId::from("root-local-xyz")).await;
        let body = serde_json::to_string(&(item, transcript)).expect("serialize codex payloads");

        for forbidden in [
            "019f5040-secret-thread-and-sessionid",
            "msg_secret_item_id",
            "leak_item",
            "/home/swkang/.codex",
            "rollout-secret.jsonl",
            "sessionId",
            "threadId",
        ] {
            assert!(!body.contains(forbidden), "codex payload leaked {forbidden}: {body}");
        }
        // But dashboard-owned identity and projected content DO cross.
        assert!(body.contains("codex:abc123"));
        assert!(body.contains("agent.codex"));
    }

    struct FixedCwdResolver(PathBuf);
    impl CodexWorkRootResolver for FixedCwdResolver {
        fn resolve_cwd(
            &self,
            _work_root_id: &WorkRootId,
        ) -> Result<PathBuf, AgentClientProviderError> {
            Ok(self.0.clone())
        }
    }

    // Manual WSL smoke path (gated on the installed codex binary). Run with:
    //   cargo test -p ws-dashboard-daemon codex_end_to_end_smoke -- --ignored --nocapture
    // Drives a real `codex app-server --stdio`: plugin gate -> spawn ->
    // thread/start -> turn/start (trivial prompt) and asserts the projector
    // captured an assistant block. The child is killed on drop.
    #[tokio::test]
    #[ignore = "requires a real, authenticated codex binary"]
    async fn codex_end_to_end_smoke() {
        let cwd = std::env::current_dir().unwrap();
        let registry = CodexProviderRegistry::default();
        let provider = CodexAppServerProvider::new(
            CodexSpawnConfig::new("server-local"),
            registry.clone(),
            Arc::new(FixedCwdResolver(cwd)),
        );

        // Gate must pass (wsflow installed/enabled in this environment).
        provider
            .initialize(AgentClientInitializeRequest {
                work_root_id: WorkRootId::from("smoke"),
            })
            .await
            .expect("plugin gate + initialize");

        let created = provider
            .create_session(AgentClientSessionCreateRequest {
                work_root_id: WorkRootId::from("smoke"),
                initial_prompt: Some(
                    "reply with exactly the single word HELLO and nothing else".to_owned(),
                ),
            })
            .await
            .expect("create session + initial turn");

        // Allow the streamed turn to complete (first token can take several
        // seconds; the spike measured ~7s to first delta).
        tokio::time::sleep(Duration::from_secs(25)).await;

        let session = registry
            .session_for("server-local", &created.activity_id)
            .expect("session present");
        let transcript = codex_activity_transcript(&session, WorkRootId::from("smoke")).await;
        eprintln!("smoke transcript: {transcript:#?}");
        assert!(
            transcript
                .blocks
                .iter()
                .any(|block| block.title.as_deref() == Some("Assistant")),
            "expected an assistant block from the real turn"
        );
    }
}
