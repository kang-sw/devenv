//! Claude CLI stream-json duplex adapter (Phase 4 of
//! `260620-feat-ws-dashboard-agent-client-activity-sources`).
//!
//! This module owns the async side of the Claude adapter: a duplex transport
//! over a `claude -p --input-format stream-json --output-format stream-json`
//! child process, a server-scoped session registry, the pre-spawn ws/wsflow
//! plugin-presence gate, and the `AgentClientProvider` bridge. The pure
//! event->transcript projection lives in `ws-dashboard-core::claude_projection`
//! and is fed from here.
//!
//! CONTRACT (Finding A): Claude output is *typed events*, NOT JSON-RPC
//! responses/notifications like Codex. Every line is classified by its
//! top-level `type`. Framing is newline-delimited JSON in both directions; no
//! new crate is added (`AsyncBufReadExt::lines()` + `serde_json` cover it,
//! same as `codex_app_server`).
//!
//! CONTRACT (Finding A4/A5): permission interception in plain stream-json does
//! NOT use a CLI->client `control_request`; that only fires for the SDK's
//! `can_use_tool` callback, which this phase does not adopt. `PreToolUse`
//! hooks (Finding D) are the interception mechanism. `control_request`/
//! `control_response` IS real and is used here only for the client->CLI
//! `interrupt` round trip.
//!
//! CONTRACT (Finding B): `--resume` is scoped to the spawn cwd; a session's
//! spawn cwd is pinned for the session's lifetime and every respawn reuses it
//! verbatim. Resuming from a different cwd fails hard with no fallback.
//!
//! CONTRACT (browser identity): provider `session_id`, `transcript_path`,
//! `cwd`, and raw event JSON stay daemon-private. Only the dashboard-owned
//! `activityId` and projected/derived Activity content cross the browser
//! boundary.

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex as StdMutex, RwLock};
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
    AgentClientSessionListResult, AgentClientSessionResumeRequest, AgentClientSessionSummary,
    AgentClientTranscriptBackfillRequest, AgentClientTranscriptBackfillResult,
};
use ws_dashboard_core::claude_projection::ClaudeProjector;
use ws_dashboard_core::WorkRootId;

/// Dashboard-owned provider discriminator (never the raw binary name).
pub const CLAUDE_PROVIDER: &str = "claude";
/// `ActivityItem`/source `kind` for a Claude interactive session.
pub const CLAUDE_ACTIVITY_KIND: &str = "agent.claude";
const CLAUDE_ACTIVITY_ID_PREFIX: &str = "claude:";

const DEFAULT_REQUEST_TIMEOUT: Duration = Duration::from_secs(120);
const MAX_CLAUDE_SESSIONS: usize = 16;
const MAX_STDERR_LINES: usize = 256;

// ---------------------------------------------------------------------------
// Transport
// ---------------------------------------------------------------------------

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum ClaudeTransportError {
    Io(String),
    Timeout,
    Closed,
    TurnError(String),
}

impl ClaudeTransportError {
    fn provider_code(&self) -> &'static str {
        match self {
            ClaudeTransportError::Io(_) => "claude.io",
            ClaudeTransportError::Timeout => "claude.timeout",
            ClaudeTransportError::Closed => "claude.closed",
            ClaudeTransportError::TurnError(_) => "claude.turn_error",
        }
    }

    /// Bounded, provider-neutral message. Never carries raw transport
    /// payloads, paths, or session ids.
    fn provider_message(&self) -> String {
        match self {
            ClaudeTransportError::Io(_) => "claude transport io error".to_owned(),
            ClaudeTransportError::Timeout => "claude request timed out".to_owned(),
            ClaudeTransportError::Closed => "claude session closed".to_owned(),
            ClaudeTransportError::TurnError(subtype) => format!("claude turn error: {subtype}"),
        }
    }
}

impl From<ClaudeTransportError> for AgentClientProviderError {
    fn from(error: ClaudeTransportError) -> Self {
        AgentClientProviderError {
            code: error.provider_code().to_owned(),
            message: error.provider_message(),
        }
    }
}

type PendingControlMap = Arc<StdMutex<HashMap<String, oneshot::Sender<Value>>>>;
type PendingTurn = Arc<StdMutex<Option<oneshot::Sender<Result<Value, ClaudeTransportError>>>>>;

/// One live stream-json duplex. Owns the write half behind an async mutex,
/// correlates `control_response`s by `request_id`, resolves the in-flight
/// turn on the next `result` event, and fans every classified event out to a
/// channel for the projector.
pub struct ClaudeConnection {
    writer: AsyncMutex<Box<dyn AsyncWrite + Unpin + Send>>,
    next_control_id: AtomicU64,
    pending_control: PendingControlMap,
    pending_turn: PendingTurn,
    // CONTRACT: only one turn is ever in flight per Claude session (the
    // duplex is turn-sequential, not per-request-id-correlated like Codex),
    // so sending a prompt must be serialized under this lock.
    send_lock: AsyncMutex<()>,
    request_timeout: Duration,
    stderr_tail: Arc<StdMutex<Vec<String>>>,
    closed: Arc<AtomicBool>,
    // Held to keep the child alive and kill it on drop (kill_on_drop). Never
    // read directly; RAII ownership is the point.
    #[allow(dead_code)]
    child: StdMutex<Option<Child>>,
}

impl ClaudeConnection {
    /// Build a connection from arbitrary async byte streams. Used by tests
    /// with an in-process NDJSON peer and by `spawn` with a real child
    /// process. Returns the connection plus the raw-event receiver a session
    /// drains into its projector.
    pub fn from_io<R, W>(
        reader: R,
        writer: W,
        request_timeout: Duration,
    ) -> (Arc<Self>, mpsc::UnboundedReceiver<Value>)
    where
        R: AsyncRead + Unpin + Send + 'static,
        W: AsyncWrite + Unpin + Send + 'static,
    {
        let pending_control: PendingControlMap = Arc::new(StdMutex::new(HashMap::new()));
        let pending_turn: PendingTurn = Arc::new(StdMutex::new(None));
        let closed = Arc::new(AtomicBool::new(false));
        let connection = Arc::new(Self {
            writer: AsyncMutex::new(Box::new(writer)),
            next_control_id: AtomicU64::new(1),
            pending_control: pending_control.clone(),
            pending_turn: pending_turn.clone(),
            send_lock: AsyncMutex::new(()),
            request_timeout,
            stderr_tail: Arc::new(StdMutex::new(Vec::new())),
            closed: closed.clone(),
            child: StdMutex::new(None),
        });
        let (event_tx, event_rx) = mpsc::unbounded_channel();
        tokio::spawn(reader_loop(reader, pending_control, pending_turn, event_tx, closed));
        (connection, event_rx)
    }

    fn set_child(&self, child: Child) {
        *self.child.lock().expect("claude child lock poisoned") = Some(child);
    }

    pub fn is_closed(&self) -> bool {
        self.closed.load(Ordering::SeqCst)
    }

    async fn write_line(&self, message: &Value) -> Result<(), ClaudeTransportError> {
        let mut line = serde_json::to_string(message)
            .map_err(|error| ClaudeTransportError::Io(format!("serialize claude message: {error}")))?;
        line.push('\n');
        let mut writer = self.writer.lock().await;
        writer
            .write_all(line.as_bytes())
            .await
            .map_err(|error| ClaudeTransportError::Io(error.to_string()))?;
        writer
            .flush()
            .await
            .map_err(|error| ClaudeTransportError::Io(error.to_string()))?;
        Ok(())
    }

    /// Send one user-message turn and await the terminal `result` event.
    /// CONTRACT: a turn is "write one user-message line -> drain events until
    /// the next `result` line"; there is no per-request id correlation
    /// (Finding A/E), so turns are serialized under `send_lock`.
    pub async fn send_user_message(&self, text: &str) -> Result<Value, ClaudeTransportError> {
        let _guard = self.send_lock.lock().await;
        if self.is_closed() {
            return Err(ClaudeTransportError::Closed);
        }
        let (tx, rx) = oneshot::channel();
        *self.pending_turn.lock().expect("claude turn lock poisoned") = Some(tx);

        let message = json!({
            "type": "user",
            "message": { "role": "user", "content": [{ "type": "text", "text": text }] },
        });
        if let Err(error) = self.write_line(&message).await {
            self.pending_turn.lock().expect("claude turn lock poisoned").take();
            return Err(error);
        }

        match tokio::time::timeout(self.request_timeout, rx).await {
            Ok(Ok(result)) => result,
            Ok(Err(_)) => Err(ClaudeTransportError::Closed),
            Err(_) => {
                self.pending_turn.lock().expect("claude turn lock poisoned").take();
                // CONTRACT: the CLI turn is still running past our client-side
                // deadline. Turns are serialized under `send_lock` precisely
                // so a later `send_prompt` never writes a new user-message
                // line into a still-active turn; simply abandoning the
                // pending turn here (without stopping the CLI) would let the
                // next `send_user_message` call release this guard and
                // interleave into the live stream. Issue an interrupt
                // (best-effort) before releasing `send_lock` so the CLI turn
                // is actually stopped first.
                let _ = self.interrupt().await;
                Err(ClaudeTransportError::Timeout)
            }
        }
    }

    /// Client->CLI `interrupt` (Finding A5): writes a `control_request` and
    /// awaits the matching `control_response` by `request_id`.
    pub async fn interrupt(&self) -> Result<(), ClaudeTransportError> {
        if self.is_closed() {
            return Err(ClaudeTransportError::Closed);
        }
        let request_id = format!("ws-ctl-{}", self.next_control_id.fetch_add(1, Ordering::SeqCst));
        let (tx, rx) = oneshot::channel();
        self.pending_control
            .lock()
            .expect("claude control lock poisoned")
            .insert(request_id.clone(), tx);

        let message = json!({
            "type": "control_request",
            "request_id": request_id,
            "request": { "subtype": "interrupt" },
        });
        if let Err(error) = self.write_line(&message).await {
            self.pending_control
                .lock()
                .expect("claude control lock poisoned")
                .remove(&request_id);
            return Err(error);
        }

        match tokio::time::timeout(self.request_timeout, rx).await {
            Ok(Ok(_response)) => Ok(()),
            Ok(Err(_)) => Err(ClaudeTransportError::Closed),
            Err(_) => {
                self.pending_control
                    .lock()
                    .expect("claude control lock poisoned")
                    .remove(&request_id);
                Err(ClaudeTransportError::Timeout)
            }
        }
    }
}

async fn reader_loop<R>(
    reader: R,
    pending_control: PendingControlMap,
    pending_turn: PendingTurn,
    events: mpsc::UnboundedSender<Value>,
    closed: Arc<AtomicBool>,
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
                let Ok(value) = serde_json::from_str::<Value>(&line) else {
                    // A malformed server line degrades silently at the
                    // transport layer; the projector surfaces a bounded
                    // diagnostic if it ever sees a malformed value via the
                    // event path (it does not, since this line never
                    // forwards); no transcript effect either way.
                    continue;
                };
                let event_type = value.get("type").and_then(Value::as_str).unwrap_or("");
                match event_type {
                    "control_response" => {
                        let request_id = value
                            .get("response")
                            .and_then(|response| response.get("request_id"))
                            .and_then(Value::as_str);
                        if let Some(request_id) = request_id {
                            if let Some(sender) = pending_control
                                .lock()
                                .expect("claude control lock poisoned")
                                .remove(request_id)
                            {
                                let _ = sender.send(value.clone());
                            }
                        }
                    }
                    "result" => {
                        // CONTRACT: a `result` with `subtype:"error_during_execution"`
                        // (interrupt or in-turn error, Finding A5/B) is a
                        // failed/interrupted turn, not a successful one —
                        // surface it as a `TurnError` so `send_prompt` never
                        // reports `accepted:true` for it.
                        let subtype = value.get("subtype").and_then(Value::as_str).unwrap_or("");
                        if let Some(sender) =
                            pending_turn.lock().expect("claude turn lock poisoned").take()
                        {
                            let outcome = if subtype == "error_during_execution" {
                                Err(ClaudeTransportError::TurnError(subtype.to_owned()))
                            } else {
                                Ok(value.clone())
                            };
                            let _ = sender.send(outcome);
                        }
                        let _ = events.send(value);
                    }
                    _ => {
                        let _ = events.send(value);
                    }
                }
            }
            Ok(None) => break,
            Err(_) => break,
        }
    }
    // EOF/error: mark closed and fail all outstanding awaits so callers never
    // hang against a dead child.
    closed.store(true, Ordering::SeqCst);
    if let Some(sender) = pending_turn.lock().expect("claude turn lock poisoned").take() {
        let _ = sender.send(Err(ClaudeTransportError::Closed));
    }
    let mut pending_control = pending_control.lock().expect("claude control lock poisoned");
    for (_, sender) in pending_control.drain() {
        let _ = sender.send(json!({"type": "control_response", "response": {"subtype": "closed"}}));
    }
}

async fn drain_stderr<R>(reader: R, tail: Arc<StdMutex<Vec<String>>>)
where
    R: AsyncRead + Unpin + Send + 'static,
{
    let mut lines = BufReader::new(reader).lines();
    while let Ok(Some(line)) = lines.next_line().await {
        let mut tail = tail.lock().expect("claude stderr lock poisoned");
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

/// One entry of `claude plugin list --json`'s flat array. CONTRACT: this
/// shape differs from Codex's `{installed:[{name,installed,enabled}]}`
/// nested shape (Finding C), so it needs its own parser, not a reuse of
/// `codex_app_server::evaluate_plugin_gate`.
#[derive(Debug, Deserialize)]
struct ClaudePluginListEntry {
    // `id` is `<name>@<marketplace>`.
    #[serde(default)]
    id: String,
    #[serde(default)]
    enabled: bool,
}

/// Refusal returned when the ws/wsflow plugin-presence precondition fails.
/// Carries install guidance, not a silent degrade.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ClaudePluginGateRefusal {
    pub message: String,
}

impl From<ClaudePluginGateRefusal> for AgentClientProviderError {
    fn from(refusal: ClaudePluginGateRefusal) -> Self {
        AgentClientProviderError {
            code: "claude.plugin_gate".to_owned(),
            message: refusal.message,
        }
    }
}

const PLUGIN_GATE_GUIDANCE: &str = "Claude session refused: neither the `ws` nor `wsflow` plugin \
is installed and enabled for this project. Install and enable it (e.g. `claude plugin install \
wsflow`) before starting a dashboard-driven Claude session.";

/// Evaluate the pre-spawn plugin gate against `claude plugin list --json`
/// output: a flat array of `{id, enabled, ...}`. Passes iff some entry's `id`
/// name-part (before `@marketplace`) is `ws` or `wsflow` and `enabled` is
/// true.
pub fn evaluate_claude_plugin_gate(plugin_list_json: &str) -> Result<(), ClaudePluginGateRefusal> {
    let parsed = serde_json::from_str::<Vec<ClaudePluginListEntry>>(plugin_list_json).map_err(|_| {
        ClaudePluginGateRefusal {
            message: format!("{PLUGIN_GATE_GUIDANCE} (could not read plugin list)"),
        }
    })?;
    let satisfied = parsed.iter().any(|entry| {
        let name = entry.id.split('@').next().unwrap_or("");
        matches!(name, "ws" | "wsflow") && entry.enabled
    });
    if satisfied {
        Ok(())
    } else {
        Err(ClaudePluginGateRefusal {
            message: PLUGIN_GATE_GUIDANCE.to_owned(),
        })
    }
}

/// Run `claude plugin list --json` and evaluate the gate. No-session CLI
/// surface (no `claude` conversation is started to check this).
///
/// CONTRACT: `claude plugin list --json` reports per-scope (user/project)
/// enablement (Finding C), so this MUST run with `current_dir(cwd)` pinned to
/// the target work-root — otherwise project-scoped enablement is judged
/// against the daemon's own process cwd instead of the project actually being
/// gated, and the gate can pass or refuse for the wrong project.
pub async fn check_claude_plugin_gate(
    claude_bin: &str,
    cwd: &std::path::Path,
) -> Result<(), ClaudePluginGateRefusal> {
    let output = Command::new(claude_bin)
        .args(["plugin", "list", "--json"])
        .current_dir(cwd)
        .output()
        .await
        .map_err(|error| ClaudePluginGateRefusal {
            message: format!("{PLUGIN_GATE_GUIDANCE} (could not run claude: {error})"),
        })?;
    if !output.status.success() {
        return Err(ClaudePluginGateRefusal {
            message: format!("{PLUGIN_GATE_GUIDANCE} (claude plugin list failed)"),
        });
    }
    evaluate_claude_plugin_gate(&String::from_utf8_lossy(&output.stdout))
}

// ---------------------------------------------------------------------------
// PreToolUse hook config injection (Finding D)
// ---------------------------------------------------------------------------

/// Build the `--settings` JSON injected at spawn to register a `PreToolUse`
/// hook. CONTRACT (Finding D): injected via `--settings`, never written into
/// the user's project `.claude/settings.json`.
///
/// For this phase's non-interactive default path (and tests), the hook
/// denies every tool call by default with a bounded, non-leaking reason. The
/// live interactive approval relay (a dashboard-owned executable that calls
/// back to the daemon's loopback control surface, correlated by
/// `session_id`+`tool_use_id`) is the follow-up
/// `260711-idea-dashboard-agent-facing-mcp-control-surface` integration; wiring
/// a real callback executable here is out of this phase's scope.
pub fn default_deny_hook_settings() -> String {
    let decision = json!({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": "ws-dashboard: interactive approval relay not configured for this session",
        }
    });
    // `command` is a shell command string (Claude Code hook contract), not a
    // provider-private path; the decision payload is a fixed literal with no
    // session/tool/path data, so nothing session-specific leaks here.
    let command = format!("echo {}", shell_single_quote(&decision.to_string()));
    let settings = json!({
        "hooks": {
            "PreToolUse": [
                {
                    "matcher": "*",
                    "hooks": [
                        { "type": "command", "command": command }
                    ]
                }
            ]
        }
    });
    settings.to_string()
}

fn shell_single_quote(text: &str) -> String {
    format!("'{}'", text.replace('\'', "'\\''"))
}

/// `--permission-mode bypassPermissions` opt-in, only ever selected on
/// explicit per-session human opt-in (never the default path). Rejects the
/// SDK `--permission-prompt-tool`/`control_request` permission path (Finding
/// A4): this crosses no such flag.
pub const CLAUDE_BYPASS_PERMISSION_MODE_FLAG: &str = "bypassPermissions";

// ---------------------------------------------------------------------------
// uuid v4 generation (no new crate: `rand` is already a workspace dependency)
// ---------------------------------------------------------------------------

fn new_session_id() -> String {
    let mut bytes = [0u8; 16];
    rand::RngCore::fill_bytes(&mut rand::rngs::OsRng, &mut bytes);
    bytes[6] = (bytes[6] & 0x0f) | 0x40; // version 4
    bytes[8] = (bytes[8] & 0x3f) | 0x80; // variant 10
    format!(
        "{:02x}{:02x}{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}{:02x}{:02x}{:02x}{:02x}",
        bytes[0], bytes[1], bytes[2], bytes[3],
        bytes[4], bytes[5],
        bytes[6], bytes[7],
        bytes[8], bytes[9],
        bytes[10], bytes[11], bytes[12], bytes[13], bytes[14], bytes[15]
    )
}

// ---------------------------------------------------------------------------
// Session registry (server-scoped, mirroring CodexProviderRegistry)
// ---------------------------------------------------------------------------

#[derive(Clone, Debug, Eq, Hash, PartialEq)]
struct ClaudeSessionKey {
    server_id: String,
    activity_id: String,
}

/// A live Claude session: the transport, its private session id + spawn cwd
/// (the resume pin, Finding B), the accumulating projector, and the owning
/// work root/server for lifecycle cleanup.
///
/// CONTRACT (Finding B lifecycle consequence 1): the projector and session
/// metadata survive child respawn; only `connection` is replaced.
pub struct ClaudeSession {
    activity_id: String,
    server_id: String,
    work_root_id: WorkRootId,
    // Daemon-private: never serialized into any browser-facing payload.
    session_id: String,
    cwd: PathBuf,
    connection: AsyncMutex<Arc<ClaudeConnection>>,
    projector: Arc<AsyncMutex<ClaudeProjector>>,
    #[allow(dead_code)]
    created_at_ms: u64,
}

impl ClaudeSession {
    fn is_live(&self) -> bool {
        // Presence-based liveness for the cap/cleanup model, mirroring
        // `CodexSession::is_live`; explicit removal (work-root close) is the
        // real eviction path, not process exit (which triggers a lazy
        // respawn-on-next-use instead, see Finding B).
        true
    }

    pub fn activity_id(&self) -> &str {
        &self.activity_id
    }

    pub fn projector(&self) -> Arc<AsyncMutex<ClaudeProjector>> {
        self.projector.clone()
    }
}

#[derive(Clone, Default)]
pub struct ClaudeProviderRegistry {
    sessions: Arc<RwLock<HashMap<ClaudeSessionKey, Arc<ClaudeSession>>>>,
}

impl ClaudeProviderRegistry {
    fn key(server_id: &str, activity_id: &str) -> ClaudeSessionKey {
        ClaudeSessionKey {
            server_id: server_id.to_owned(),
            activity_id: activity_id.to_owned(),
        }
    }

    fn insert(&self, session: Arc<ClaudeSession>) -> Result<(), AgentClientProviderError> {
        let mut sessions = self.sessions.write().expect("claude registry lock poisoned");
        sessions.retain(|_, session| session.is_live());
        if sessions.len() >= MAX_CLAUDE_SESSIONS {
            return Err(AgentClientProviderError {
                code: "claude.too_many_sessions".to_owned(),
                message: "too many active Claude sessions".to_owned(),
            });
        }
        sessions.insert(Self::key(&session.server_id, &session.activity_id), session);
        Ok(())
    }

    fn get(&self, server_id: &str, activity_id: &str) -> Option<Arc<ClaudeSession>> {
        self.sessions
            .read()
            .expect("claude registry lock poisoned")
            .get(&Self::key(server_id, activity_id))
            .cloned()
    }

    /// Public lookup for route handlers that need to project a live
    /// session's transcript.
    pub fn session_for(&self, server_id: &str, activity_id: &str) -> Option<Arc<ClaudeSession>> {
        self.get(server_id, activity_id)
    }

    fn list_for_work_root(&self, server_id: &str, work_root_id: &WorkRootId) -> Vec<Arc<ClaudeSession>> {
        self.sessions
            .read()
            .expect("claude registry lock poisoned")
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
    /// `Child` is killed on drop.
    fn remove(&self, server_id: &str, activity_id: &str) -> Option<Arc<ClaudeSession>> {
        self.sessions
            .write()
            .expect("claude registry lock poisoned")
            .remove(&Self::key(server_id, activity_id))
    }

    /// Drop sessions for closed work roots (mirrors
    /// `CodexProviderRegistry::remove_for_work_roots`).
    pub fn remove_for_work_roots(&self, work_root_ids: &std::collections::BTreeSet<WorkRootId>) -> usize {
        let mut sessions = self.sessions.write().expect("claude registry lock poisoned");
        let before = sessions.len();
        sessions.retain(|_, session| !work_root_ids.contains(&session.work_root_id));
        before - sessions.len()
    }

    /// Test-support: insert a session backed by an arbitrary connection and a
    /// pre-populated projector, mirroring
    /// `CodexProviderRegistry::insert_session_for_tests`.
    #[allow(clippy::too_many_arguments)]
    pub fn insert_session_for_tests(
        &self,
        server_id: impl Into<String>,
        activity_id: impl Into<String>,
        work_root_id: WorkRootId,
        session_id: impl Into<String>,
        cwd: PathBuf,
        connection: Arc<ClaudeConnection>,
        projector: ClaudeProjector,
    ) -> Result<Arc<ClaudeSession>, AgentClientProviderError> {
        let session = Arc::new(ClaudeSession {
            activity_id: activity_id.into(),
            server_id: server_id.into(),
            work_root_id,
            session_id: session_id.into(),
            cwd,
            connection: AsyncMutex::new(connection),
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
pub trait ClaudeWorkRootResolver: Send + Sync {
    fn resolve_cwd(&self, work_root_id: &WorkRootId) -> Result<PathBuf, AgentClientProviderError>;
}

#[derive(Clone)]
pub struct ClaudeSpawnConfig {
    pub claude_bin: String,
    pub server_id: String,
    pub request_timeout: Duration,
}

impl ClaudeSpawnConfig {
    pub fn new(server_id: impl Into<String>) -> Self {
        Self {
            claude_bin: "claude".to_owned(),
            server_id: server_id.into(),
            request_timeout: DEFAULT_REQUEST_TIMEOUT,
        }
    }
}

/// Concrete `AgentClientProvider` backed by a `claude -p --input-format
/// stream-json --output-format stream-json` child process.
pub struct ClaudeCliProvider {
    config: ClaudeSpawnConfig,
    registry: ClaudeProviderRegistry,
    resolver: Arc<dyn ClaudeWorkRootResolver>,
}

impl ClaudeCliProvider {
    pub fn new(
        config: ClaudeSpawnConfig,
        registry: ClaudeProviderRegistry,
        resolver: Arc<dyn ClaudeWorkRootResolver>,
    ) -> Self {
        Self {
            config,
            registry,
            resolver,
        }
    }

    fn capabilities() -> AgentClientCapabilities {
        // Cross-Harness Feature Matrix (plan #L38-L42): Claude is Unavailable
        // for compact/steer/goal, Hack-tier (out of Phase 4) for rewind/fork,
        // and split Passthrough(plugin)+Overlay(fs scan) for skills, of which
        // only the plugin-presence half lands with the core adapter.
        AgentClientCapabilities {
            compact: false,
            steer: false,
            goal: false,
            rewind: false,
            fork: false,
            skills: true,
        }
    }

    /// Spawn a child `claude` stream-json process, pinned to `cwd` (Finding
    /// B: the resume key). `resume_session_id`, when set, adds `--resume
    /// <id>`; otherwise a fresh `--session-id <id>` is passed so the daemon
    /// controls the id from creation.
    async fn spawn_connection(
        &self,
        cwd: &PathBuf,
        session_id: &str,
        resume: bool,
    ) -> Result<(Arc<ClaudeConnection>, mpsc::UnboundedReceiver<Value>), ClaudeTransportError> {
        let settings = default_deny_hook_settings();
        let mut command = Command::new(&self.config.claude_bin);
        command
            .args([
                "-p",
                "--input-format",
                "stream-json",
                "--output-format",
                "stream-json",
                "--verbose",
                "--settings",
            ])
            .arg(&settings);
        if resume {
            command.args(["--resume", session_id]);
        } else {
            command.args(["--session-id", session_id]);
        }
        let mut child = command
            .current_dir(cwd)
            .stdin(std::process::Stdio::piped())
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .kill_on_drop(true)
            .spawn()
            .map_err(|error| ClaudeTransportError::Io(error.to_string()))?;

        let stdin = child
            .stdin
            .take()
            .ok_or_else(|| ClaudeTransportError::Io("claude stdin unavailable".to_owned()))?;
        let stdout = child
            .stdout
            .take()
            .ok_or_else(|| ClaudeTransportError::Io("claude stdout unavailable".to_owned()))?;
        let stderr = child.stderr.take();

        let (connection, events) = ClaudeConnection::from_io(stdout, stdin, self.config.request_timeout);
        if let Some(stderr) = stderr {
            tokio::spawn(drain_stderr(stderr, connection.stderr_tail.clone()));
        }
        connection.set_child(child);

        Ok((connection, events))
    }

    /// Ensure `session`'s connection is live, lazily respawning with
    /// `--resume <session_id>` from the pinned spawn cwd if the previous
    /// child has exited (Finding B: kill-and-respawn-via-`--resume`). The
    /// projector is preserved across the respawn (Finding B lifecycle
    /// consequence 1): resume does not replay prior turns to stdout, so
    /// accumulated transcript state must survive the child process.
    async fn ensure_live(&self, session: &Arc<ClaudeSession>) -> Result<(), AgentClientProviderError> {
        let mut connection_guard = session.connection.lock().await;
        if !connection_guard.is_closed() {
            return Ok(());
        }
        let (connection, events) = self
            .spawn_connection(&session.cwd, &session.session_id, true)
            .await?;
        spawn_projector_pump(session.projector.clone(), events);
        *connection_guard = connection;
        Ok(())
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
    format!("{CLAUDE_ACTIVITY_ID_PREFIX}{:x}", now_ms())
}

/// Build the browser-facing source display for a Claude session. Carries the
/// dashboard-owned discriminator and display-only fields, never provider ids.
pub fn claude_source_display(label: &str) -> ActivitySourceDisplay {
    ActivitySourceDisplay {
        kind: CLAUDE_ACTIVITY_KIND.to_owned(),
        label: label.to_owned(),
        backend: Some(CLAUDE_PROVIDER.to_owned()),
        harness: Some(CLAUDE_PROVIDER.to_owned()),
        tier: Some("core".to_owned()),
        model: None,
    }
}

/// Project a live Claude session into a browser-facing `ActivityItem` for the
/// `ActivityFeed.items` merge. CONTRACT: this must be merged into
/// `ActivityFeed.items`, never `agents`, and must omit provider ids/paths.
pub async fn claude_activity_item(session: &ClaudeSession) -> ActivityItem {
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
        kind: CLAUDE_ACTIVITY_KIND.to_owned(),
        label: "Claude session".to_owned(),
        status,
        live,
        attention: !diagnostics.is_empty(),
        started_at: None,
        updated_at: None,
        finished_at: None,
        source: claude_source_display("Claude session"),
        transcript: ActivityTranscriptAvailability {
            status: if has_transcript { "available" } else { "empty" }.to_owned(),
            available: has_transcript,
            cursor: has_transcript.then(|| "0".to_owned()),
        },
        diagnostics,
        metadata,
    }
}

/// Project a live Claude session's transcript for the browser. CONTRACT: only
/// projected blocks + dashboard-owned `activityId` cross; the private session
/// id and cwd collapse to nothing.
pub async fn claude_activity_transcript(
    session: &ClaudeSession,
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
        source: claude_source_display("Claude session"),
        blocks,
        next_cursor: None,
        has_more: false,
        diagnostics,
    }
}

/// Collect browser-facing Claude `ActivityItem`s for the sessions of one work
/// root under a server, for merging into the unified `ActivityFeed.items`.
/// CONTRACT: these rows belong in `items`, never the legacy `agents`
/// projection, and carry no provider ids/paths (see `claude_activity_item`).
pub async fn claude_activity_items(
    registry: &ClaudeProviderRegistry,
    server_id: &str,
    work_root_id: &WorkRootId,
) -> Vec<ActivityItem> {
    let sessions = registry.list_for_work_root(server_id, work_root_id);
    let mut items = Vec::with_capacity(sessions.len());
    for session in sessions {
        items.push(claude_activity_item(&session).await);
    }
    items
}

/// Spawn the projector-updating task: drain raw events into the session's
/// projector under its lock.
fn spawn_projector_pump(
    projector: Arc<AsyncMutex<ClaudeProjector>>,
    mut events: mpsc::UnboundedReceiver<Value>,
) {
    tokio::spawn(async move {
        while let Some(event) = events.recv().await {
            let mut projector = projector.lock().await;
            projector.ingest_value(&event);
        }
    });
}

impl AgentClientProvider for ClaudeCliProvider {
    async fn initialize(
        &self,
        request: AgentClientInitializeRequest,
    ) -> Result<AgentClientInitializeResult, AgentClientProviderError> {
        // CONTRACT: resolve the target work-root cwd BEFORE gating so the
        // gate is judged against the right project (Finding C project-scoped
        // enablement), not the daemon's own process cwd.
        let cwd = self.resolver.resolve_cwd(&request.work_root_id)?;
        check_claude_plugin_gate(&self.config.claude_bin, &cwd).await?;
        Ok(AgentClientInitializeResult {
            metadata: AgentClientProviderMetadata {
                provider: CLAUDE_PROVIDER.to_owned(),
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
            summaries.push(AgentClientSessionSummary {
                activity_id: session.activity_id.clone(),
                label: "Claude session".to_owned(),
                status: if projector.is_turn_active() {
                    "running"
                } else {
                    "idle"
                }
                .to_owned(),
                updated_at: None,
            });
        }
        Ok(AgentClientSessionListResult { sessions: summaries })
    }

    async fn create_session(
        &self,
        request: AgentClientSessionCreateRequest,
    ) -> Result<AgentClientSessionCreateResult, AgentClientProviderError> {
        // CONTRACT: resolve the work-root cwd BEFORE gating (Finding C
        // project-scoped enablement) so `claude plugin list --json` is
        // evaluated against the actual target project, then enforce the
        // plugin gate before spawning any process.
        let cwd = self.resolver.resolve_cwd(&request.work_root_id)?;
        check_claude_plugin_gate(&self.config.claude_bin, &cwd).await?;

        let session_id = new_session_id();
        let (connection, events) = self.spawn_connection(&cwd, &session_id, false).await?;

        let projector = Arc::new(AsyncMutex::new(ClaudeProjector::new()));
        spawn_projector_pump(projector.clone(), events);

        let activity_id = new_activity_id();
        let session = Arc::new(ClaudeSession {
            activity_id: activity_id.clone(),
            server_id: self.config.server_id.clone(),
            work_root_id: request.work_root_id.clone(),
            session_id: session_id.clone(),
            cwd,
            connection: AsyncMutex::new(connection.clone()),
            projector: projector.clone(),
            created_at_ms: now_ms(),
        });
        self.registry.insert(session)?;

        if let Some(prompt) = request.initial_prompt.filter(|prompt| !prompt.is_empty()) {
            if let Err(error) = connection.send_user_message(&prompt).await {
                // CONTRACT: the initial-turn failure must not leave an
                // orphaned session + live child registered against
                // MAX_CLAUDE_SESSIONS. Roll the registry insert back;
                // dropping the session (and the local connection Arc on
                // return) kills the child on drop.
                self.registry.remove(&self.config.server_id, &activity_id);
                return Err(error.into());
            }
        }

        Ok(AgentClientSessionCreateResult { activity_id })
    }

    async fn resume_session(
        &self,
        request: AgentClientSessionResumeRequest,
    ) -> Result<(), AgentClientProviderError> {
        let session = self.session(&request.activity_id)?;
        self.ensure_live(&session).await
    }

    async fn send_prompt(
        &self,
        request: AgentClientPromptSendRequest,
    ) -> Result<AgentClientPromptSendResult, AgentClientProviderError> {
        let session = self.session(&request.activity_id)?;
        self.ensure_live(&session).await?;
        let connection = session.connection.lock().await.clone();
        connection.send_user_message(&request.text).await?;
        Ok(AgentClientPromptSendResult { accepted: true })
    }

    async fn interrupt(
        &self,
        request: AgentClientInterruptRequest,
    ) -> Result<(), AgentClientProviderError> {
        let session = self.session(&request.activity_id)?;
        let connection = session.connection.lock().await.clone();
        connection.interrupt().await?;
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

impl ClaudeCliProvider {
    fn session(&self, activity_id: &str) -> Result<Arc<ClaudeSession>, AgentClientProviderError> {
        self.registry
            .get(&self.config.server_id, activity_id)
            .ok_or_else(|| AgentClientProviderError {
                code: "claude.unknown_session".to_owned(),
                message: "unknown Claude session".to_owned(),
            })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::io::{duplex, AsyncBufReadExt, AsyncWriteExt, BufReader};

    #[tokio::test]
    async fn send_user_message_resolves_on_next_result() {
        let (client_side, mut server_side) = duplex(4096);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, mut events) =
            ClaudeConnection::from_io(client_read, client_write, Duration::from_secs(5));

        let peer = tokio::spawn(async move {
            let (server_read, mut server_write) = tokio::io::split(&mut server_side);
            let mut lines = BufReader::new(server_read).lines();
            let line = lines.next_line().await.unwrap().unwrap();
            let request: Value = serde_json::from_str(&line).unwrap();
            assert_eq!(request["type"], "user");
            let assistant = "{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"hi\"}]}}\n";
            server_write.write_all(assistant.as_bytes()).await.unwrap();
            let result = "{\"type\":\"result\",\"subtype\":\"success\"}\n";
            server_write.write_all(result.as_bytes()).await.unwrap();
            server_write.flush().await.unwrap();
            tokio::time::sleep(Duration::from_millis(100)).await;
        });

        let result = connection.send_user_message("hello").await.unwrap();
        assert_eq!(result["subtype"], "success");
        // Both the assistant event and the result event fan out to the
        // projector event channel.
        let first = events.recv().await.unwrap();
        assert_eq!(first["type"], "assistant");
        let second = events.recv().await.unwrap();
        assert_eq!(second["type"], "result");
        peer.await.unwrap();
    }

    #[tokio::test]
    async fn interrupt_writes_control_request_and_resolves_on_matching_response() {
        let (client_side, mut server_side) = duplex(4096);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, _events) =
            ClaudeConnection::from_io(client_read, client_write, Duration::from_secs(5));

        let peer = tokio::spawn(async move {
            let (server_read, mut server_write) = tokio::io::split(&mut server_side);
            let mut lines = BufReader::new(server_read).lines();
            let line = lines.next_line().await.unwrap().unwrap();
            let request: Value = serde_json::from_str(&line).unwrap();
            assert_eq!(request["type"], "control_request");
            assert_eq!(request["request"]["subtype"], "interrupt");
            let request_id = request["request_id"].as_str().unwrap().to_owned();
            let response = json!({
                "type": "control_response",
                "response": {"subtype": "success", "request_id": request_id, "response": {"still_queued": []}},
            });
            server_write
                .write_all(format!("{response}\n").as_bytes())
                .await
                .unwrap();
            server_write.flush().await.unwrap();
            tokio::time::sleep(Duration::from_millis(100)).await;
        });

        connection.interrupt().await.unwrap();
        peer.await.unwrap();
    }

    #[tokio::test]
    async fn request_times_out_when_peer_silent() {
        let (client_side, _server_side) = duplex(4096);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, _events) =
            ClaudeConnection::from_io(client_read, client_write, Duration::from_millis(80));
        let error = connection.send_user_message("hello").await.unwrap_err();
        assert_eq!(error, ClaudeTransportError::Timeout);
    }

    #[tokio::test]
    async fn eof_fails_outstanding_turn_and_marks_closed() {
        let (client_side, server_side) = duplex(4096);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, _events) =
            ClaudeConnection::from_io(client_read, client_write, Duration::from_secs(5));

        // Spawn so the write actually executes (and lands) before the peer
        // closes; a lazily-polled future would not have written anything
        // yet, turning this into a synchronous broken-pipe write error
        // instead of the intended reader-observes-EOF path.
        let connection_for_send = connection.clone();
        let send_task = tokio::spawn(async move { connection_for_send.send_user_message("hello").await });
        tokio::time::sleep(Duration::from_millis(20)).await;
        drop(server_side); // close the peer, forcing EOF on the reader.

        let error = send_task.await.unwrap().unwrap_err();
        assert_eq!(error, ClaudeTransportError::Closed);
        // Give the reader loop a beat to observe EOF and set the flag.
        tokio::time::sleep(Duration::from_millis(50)).await;
        assert!(connection.is_closed());
    }

    #[test]
    fn plugin_gate_passes_for_flat_array_with_matching_enabled_entry() {
        let json = r#"[{"id":"ws@kang-sw-devenv","enabled":true},{"id":"other@x","enabled":true}]"#;
        assert!(evaluate_claude_plugin_gate(json).is_ok());
    }

    #[test]
    fn plugin_gate_refuses_when_neither_ws_nor_wsflow_present() {
        let json = r#"[{"id":"other@x","enabled":true}]"#;
        let refusal = evaluate_claude_plugin_gate(json).unwrap_err();
        assert!(refusal.message.contains("ws"));
        assert!(refusal.message.contains("wsflow"));
    }

    #[test]
    fn plugin_gate_refuses_when_sole_match_disabled() {
        let json = r#"[{"id":"wsflow@kang-sw-devenv","enabled":false}]"#;
        assert!(evaluate_claude_plugin_gate(json).is_err());
    }

    #[test]
    fn plugin_gate_refusal_becomes_provider_error() {
        let refusal = evaluate_claude_plugin_gate("not json").unwrap_err();
        let error: AgentClientProviderError = refusal.into();
        assert_eq!(error.code, "claude.plugin_gate");
    }

    #[test]
    fn default_deny_hook_settings_registers_pre_tool_use_deny_and_has_no_session_data() {
        let settings = default_deny_hook_settings();
        let parsed: Value = serde_json::from_str(&settings).expect("settings is valid json");
        assert_eq!(
            parsed["hooks"]["PreToolUse"][0]["hooks"][0]["type"],
            "command"
        );
        assert!(!settings.contains("session_id"));
        assert!(!settings.contains("/home"));
    }

    #[tokio::test]
    async fn registry_remove_frees_session_slot() {
        let registry = ClaudeProviderRegistry::default();
        let (client_side, _server_side) = duplex(1024);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, _events) =
            ClaudeConnection::from_io(client_read, client_write, Duration::from_secs(1));
        registry
            .insert_session_for_tests(
                "server-local",
                "claude:slot",
                WorkRootId::from("wr-slot"),
                "session-private",
                PathBuf::from("/tmp"),
                connection,
                ClaudeProjector::new(),
            )
            .expect("seed session");
        assert!(registry.session_for("server-local", "claude:slot").is_some());
        assert!(registry.remove("server-local", "claude:slot").is_some());
        assert!(registry.session_for("server-local", "claude:slot").is_none());
        assert!(registry.remove("server-local", "claude:slot").is_none());
    }

    fn test_session(projector: ClaudeProjector) -> Arc<ClaudeSession> {
        let (client_side, _server_side) = duplex(1024);
        let (client_read, client_write) = tokio::io::split(client_side);
        let (connection, _events) =
            ClaudeConnection::from_io(client_read, client_write, Duration::from_secs(1));
        // Deliberately private, leak-prone values that must NOT reach the
        // browser payload.
        Arc::new(ClaudeSession {
            activity_id: "claude:abc123".to_owned(),
            server_id: "server-local".to_owned(),
            work_root_id: WorkRootId::from("root-local-xyz"),
            session_id: "019f5040-secret-session-id".to_owned(),
            cwd: PathBuf::from("/home/swkang/.claude/projects/secret-cwd"),
            connection: AsyncMutex::new(connection),
            projector: Arc::new(AsyncMutex::new(projector)),
            created_at_ms: 0,
        })
    }

    #[tokio::test]
    async fn browser_payloads_omit_provider_ids_and_paths() {
        let mut projector = ClaudeProjector::new();
        projector.ingest_value(&json!({
            "type": "assistant",
            "message": {"role": "assistant", "content": [{"type": "text", "text": "hello"}]},
        }));
        // Inject a leak-prone unknown event carrying a private session path.
        projector.ingest_value(&json!({
            "type": "mysteryLeak",
            "session_id": "019f5040-secret-session-id",
            "transcript_path": "/home/swkang/.claude/projects/secret-cwd/rollout.jsonl",
        }));
        let session = test_session(projector);

        let item = claude_activity_item(&session).await;
        let transcript = claude_activity_transcript(&session, WorkRootId::from("root-local-xyz")).await;
        let body = serde_json::to_string(&(item, transcript)).expect("serialize claude payloads");

        for forbidden in [
            "019f5040-secret-session-id",
            "/home/swkang/.claude",
            "rollout.jsonl",
            "secret-cwd",
            "sessionId",
            "session_id",
        ] {
            assert!(!body.contains(forbidden), "claude payload leaked {forbidden}: {body}");
        }
        // But dashboard-owned identity and projected content DO cross.
        assert!(body.contains("claude:abc123"));
        assert!(body.contains("agent.claude"));
    }

    struct FixedCwdResolver(PathBuf);
    impl ClaudeWorkRootResolver for FixedCwdResolver {
        fn resolve_cwd(&self, _work_root_id: &WorkRootId) -> Result<PathBuf, AgentClientProviderError> {
            Ok(self.0.clone())
        }
    }

    #[tokio::test]
    async fn spawn_refuses_when_plugin_gate_fails() {
        // A binary that does not exist fails the plugin-gate `output()` call,
        // which must surface as `claude.plugin_gate`, never a panic or a
        // spawned child.
        let cwd = std::env::current_dir().unwrap();
        let registry = ClaudeProviderRegistry::default();
        let provider = ClaudeCliProvider::new(
            ClaudeSpawnConfig {
                claude_bin: "definitely-not-a-real-claude-binary".to_owned(),
                server_id: "server-local".to_owned(),
                request_timeout: Duration::from_secs(1),
            },
            registry,
            Arc::new(FixedCwdResolver(cwd)),
        );
        let error = provider
            .create_session(AgentClientSessionCreateRequest {
                work_root_id: WorkRootId::from("wr-gate"),
                initial_prompt: None,
            })
            .await
            .unwrap_err();
        assert_eq!(error.code, "claude.plugin_gate");
    }

    // Manual WSL smoke path (gated on the installed, authenticated claude
    // binary). Run with:
    //   cargo test -p ws-dashboard-daemon claude_end_to_end_smoke -- --ignored --nocapture
    // Drives a real `claude -p --input-format stream-json --output-format
    // stream-json`: plugin gate -> spawn -> a trivial prompt turn and asserts
    // the projector captured an assistant block. The child is killed on drop.
    #[tokio::test]
    #[ignore = "requires a real, authenticated claude binary"]
    async fn claude_end_to_end_smoke() {
        let cwd = std::env::current_dir().unwrap();
        let registry = ClaudeProviderRegistry::default();
        let provider = ClaudeCliProvider::new(
            ClaudeSpawnConfig::new("server-local"),
            registry.clone(),
            Arc::new(FixedCwdResolver(cwd)),
        );

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
                    "reply with exactly the single word PONG and nothing else".to_owned(),
                ),
            })
            .await
            .expect("create session + initial turn");

        tokio::time::sleep(Duration::from_secs(15)).await;

        let session = registry
            .session_for("server-local", &created.activity_id)
            .expect("session present");
        let transcript = claude_activity_transcript(&session, WorkRootId::from("smoke")).await;
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
