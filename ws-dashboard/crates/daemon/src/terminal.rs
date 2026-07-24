use std::collections::{BTreeSet, HashMap, VecDeque};
use std::path::{Component, Path, PathBuf};
use std::sync::{Arc, Mutex, RwLock};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use axum::extract::{
    ws::{Message, WebSocket, WebSocketUpgrade},
    Path as AxumPath, Query, State,
};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use futures_util::{SinkExt, StreamExt};
use rand::distributions::Alphanumeric;
use rand::{thread_rng, Rng};
use serde::{Deserialize, Serialize};
use tokio::sync::{watch, Mutex as AsyncMutex};
use ws_dashboard_core::WorkRootId;

use crate::router::AppState;
use crate::terminal_helper_ipc::{write_ndjson, NdjsonReader};
use crate::terminal_helper_protocol::{
    DaemonToHelperMessage, HelperToDaemonMessage, TerminalHelperOutputChunk, TerminalHelperStatus,
};
use crate::terminal_ipc_transport::{IpcReadHalf, IpcWriteHalf};
use crate::terminal_reconcile::{classify, IdentityStatus, IpcStatus, ReconcileRow};
use crate::terminal_registry_file::{
    delete_registry_entry, scan_registry_dir, TerminalRegistryEntry,
};
use crate::work_root_files::{resolve_online_available_work_root, WorkRootAccessError};

const MAX_TERMINAL_SESSIONS: usize = 16;
const MAX_OUTPUT_CHUNKS: usize = 1024;
const MAX_INPUT_BYTES: usize = 16 * 1024;
const MIN_COLUMNS: u16 = 1;
const MIN_ROWS: u16 = 1;
const MAX_COLUMNS: u16 = 300;
const MAX_ROWS: u16 = 120;
const DEFAULT_BROWSER_PTY_TERM: &str = "xterm-256color";

// CONTRACT: the daemon-local grace window is a display/attach-gating
// convenience only; the helper is the authoritative timer (see
// `terminal_helper_process.rs::GRACE_WINDOW`) and self-exits/deletes its
// registry entry independently of whatever the daemon believes here.
const DAEMON_GRACE_WINDOW_MS: u64 = 30_000;
const DEFAULT_CONNECT_TIMEOUT: Duration = Duration::from_millis(3_000);
pub(crate) const DEFAULT_RECONCILE_CONNECT_TIMEOUT: Duration = Duration::from_millis(400);

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum TerminalPlatform {
    Unix,
    Windows,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum TerminalShellSource {
    ShellEnv,
    PwshPath,
    WindowsPowerShellPath,
    ComspecEnv,
    Fallback,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct TerminalShellSelection {
    pub platform: TerminalPlatform,
    pub program: std::path::PathBuf,
    pub source: TerminalShellSource,
}

pub fn select_terminal_shell(
    platform: TerminalPlatform,
    env: impl Fn(&str) -> Option<std::ffi::OsString>,
) -> TerminalShellSelection {
    select_terminal_shell_with_detector(platform, &env, |program| {
        windows_program_on_path(program, &env)
    })
}

fn select_terminal_shell_with_detector<E, D>(
    platform: TerminalPlatform,
    env: &E,
    windows_program_on_path: D,
) -> TerminalShellSelection
where
    E: Fn(&str) -> Option<std::ffi::OsString>,
    D: Fn(&str) -> Option<PathBuf>,
{
    // CONTRACT: Shell selection must be explicit and testable for Unix and
    // Windows without relying on compile-time cfg branches inside tests.
    // HINT: Unix uses SHELL then /bin/sh; Windows prefers PowerShell, then
    // COMSPEC/cmd.exe for compatibility.
    if platform == TerminalPlatform::Unix {
        if let Some(program) = env("SHELL").filter(|value| !value.is_empty()) {
            return TerminalShellSelection {
                platform,
                program: PathBuf::from(program),
                source: TerminalShellSource::ShellEnv,
            };
        }

        return TerminalShellSelection {
            platform,
            program: PathBuf::from("/bin/sh"),
            source: TerminalShellSource::Fallback,
        };
    }

    if let Some(program) = windows_program_on_path("pwsh.exe") {
        return TerminalShellSelection {
            platform,
            program,
            source: TerminalShellSource::PwshPath,
        };
    }

    if let Some(program) = windows_program_on_path("powershell.exe") {
        return TerminalShellSelection {
            platform,
            program,
            source: TerminalShellSource::WindowsPowerShellPath,
        };
    }

    if let Some(program) = env("COMSPEC").filter(|value| !value.is_empty()) {
        return TerminalShellSelection {
            platform,
            program: PathBuf::from(program),
            source: TerminalShellSource::ComspecEnv,
        };
    }

    TerminalShellSelection {
        platform,
        program: PathBuf::from("cmd.exe"),
        source: TerminalShellSource::Fallback,
    }
}

fn windows_program_on_path<E>(program: &str, env: &E) -> Option<PathBuf>
where
    E: Fn(&str) -> Option<std::ffi::OsString>,
{
    env("PATH").and_then(|path| {
        std::env::split_paths(&path)
            .map(|directory| directory.join(program))
            .find(|candidate| candidate.is_file())
    })
}

#[derive(Clone)]
pub struct TerminalRegistry {
    sessions: Arc<RwLock<HashMap<String, Arc<TerminalSession>>>>,
    helper_binary: PathBuf,
    registry_dir: PathBuf,
    connect_timeout: Duration,
}

impl Default for TerminalRegistry {
    fn default() -> Self {
        Self::new(
            default_helper_binary(),
            default_registry_dir(),
            DEFAULT_CONNECT_TIMEOUT,
        )
    }
}

pub(crate) fn default_helper_binary() -> PathBuf {
    std::env::current_exe().unwrap_or_else(|_| PathBuf::from("ws-dashboard"))
}

pub(crate) fn default_registry_dir() -> PathBuf {
    crate::persistent_state::default_state_dir()
        .unwrap_or_else(std::env::temp_dir)
        .join("terminals")
}

impl TerminalRegistry {
    pub fn new(helper_binary: PathBuf, registry_dir: PathBuf, connect_timeout: Duration) -> Self {
        Self {
            sessions: Arc::new(RwLock::new(HashMap::new())),
            helper_binary,
            registry_dir,
            connect_timeout,
        }
    }

    // CONTRACT (ticket "Boot reconcile policy" / server.rs wiring): must run
    // to completion BEFORE `build_router`/`axum::serve` starts accepting
    // connections - callers must await this before constructing `AppState`.
    // Scans `registry_dir` for `<termid>.json` entries left behind by
    // helpers that survived a prior daemon's exit, applies the 6-row table
    // (`terminal_reconcile::classify`) per entry, and returns a registry
    // pre-populated with every adopted (row 1/2) session.
    pub async fn boot_reconcile(
        helper_binary: PathBuf,
        registry_dir: PathBuf,
        connect_timeout: Duration,
    ) -> Self {
        let registry = Self::new(helper_binary, registry_dir.clone(), connect_timeout);
        let scan_dir = registry_dir.clone();
        let entries = tokio::task::spawn_blocking(move || scan_registry_dir(&scan_dir))
            .await
            .unwrap_or_default();

        let mut seen_ids = std::collections::HashSet::new();
        for entry in entries {
            // Duplicate-entry defense: the one-file-per-terminal-id scan
            // shape structurally prevents true duplicates, but keep the
            // guard so a corrupted directory (e.g. hand-edited during
            // debugging) degrades to "first wins" instead of double-adopt.
            if !seen_ids.insert(entry.terminal_id.clone()) {
                continue;
            }
            registry.reconcile_entry(entry).await;
        }
        registry
    }

    async fn reconcile_entry(&self, entry: TerminalRegistryEntry) {
        // Unverified identity NEVER even attempts an IPC connection, let
        // alone a kill - `classify` encodes this short-circuit, but the
        // check is duplicated here explicitly so no connect attempt can
        // slip in before it (see `terminal_reconcile.rs` rows 3/5).
        let identity = identity_status(entry.pid, entry.start_time);
        if !matches!(identity, IdentityStatus::VerifiedOurs) {
            delete_registry_entry(&self.registry_dir, &entry.terminal_id);
            return;
        }

        let connected = connect_and_handshake(&entry.socket_path, self.connect_timeout).await;
        let ipc_status = match &connected {
            Some(connected) if connected.status == TerminalStatus::Running => {
                IpcStatus::ReachableShellAlive
            }
            Some(_) => IpcStatus::ReachableShellExited,
            None => IpcStatus::Unreachable,
        };

        match classify(identity, ipc_status) {
            ReconcileRow::AdoptLive | ReconcileRow::AdoptGrace => {
                let connected =
                    connected.expect("adopt rows are only reachable with a live connection");
                let session = TerminalSession::from_connection(
                    entry.terminal_id.clone(),
                    WorkRootId::from(entry.work_root_id.clone()),
                    entry.title.clone(),
                    entry.cwd_hint.clone(),
                    entry.created_at_ms,
                    connected,
                    entry.columns,
                    entry.rows,
                );
                self.insert_unchecked(session);
            }
            ReconcileRow::KillVerified => {
                let pid = entry.pid;
                let start_time = entry.start_time;
                let _ = tokio::task::spawn_blocking(move || {
                    crate::terminal_platform::kill_verified(pid, start_time)
                })
                .await;
                delete_registry_entry(&self.registry_dir, &entry.terminal_id);
            }
            ReconcileRow::DropNoSuchProcess | ReconcileRow::DropPidReused => {
                unreachable!("identity already verified above; classify cannot return this row")
            }
        }
    }

    fn list_for_work_root(&self, work_root_id: &WorkRootId) -> Vec<TerminalSessionView> {
        self.sessions
            .read()
            .expect("terminal registry lock poisoned")
            .values()
            .filter(|session| &session.work_root_id == work_root_id && session.admits_attach())
            .map(|session| session.view())
            .collect()
    }

    fn get(&self, terminal_id: &str) -> Option<Arc<TerminalSession>> {
        self.sessions
            .read()
            .expect("terminal registry lock poisoned")
            .get(terminal_id)
            .cloned()
    }

    fn insert(&self, session: Arc<TerminalSession>) -> Result<(), TerminalError> {
        let mut sessions = self
            .sessions
            .write()
            .expect("terminal registry lock poisoned");
        sessions.retain(|_, session| session.is_live());
        if sessions.len() >= MAX_TERMINAL_SESSIONS {
            return Err(TerminalError::BadRequest("too many terminal sessions"));
        }
        sessions.insert(session.id.clone(), session);
        Ok(())
    }

    // Boot-reconcile-only insertion path: adopted sessions must all land in
    // the registry before the cap is evaluated against any *new*
    // `create_terminal` call (see `boot_reconcile`'s doc comment) - applying
    // `insert`'s cap check here could evict a legitimately-adopted live
    // session for no better reason than scan order.
    fn insert_unchecked(&self, session: Arc<TerminalSession>) {
        self.sessions
            .write()
            .expect("terminal registry lock poisoned")
            .insert(session.id.clone(), session);
    }

    fn remove(&self, terminal_id: &str) -> Option<Arc<TerminalSession>> {
        self.sessions
            .write()
            .expect("terminal registry lock poisoned")
            .remove(terminal_id)
    }

    // CONTRACT (risk signal, ticket 260723 Phase 1 plan): returns every
    // removed session so callers can explicitly request its kill. Before
    // the PTY lived out-of-process, dropping the last `Arc<TerminalSession>`
    // here implicitly closed the PTY master (SIGHUP) and that was enough -
    // dropping this thin daemon-side proxy now does NOTHING to a detached
    // helper, which would otherwise keep running orphaned forever. Callers
    // MUST kill each returned session (see the three `remove_for_work_roots`
    // call sites in `git_worktree.rs`/`resources.rs`/`root_picker.rs`).
    pub fn remove_for_work_roots(
        &self,
        work_root_ids: &BTreeSet<WorkRootId>,
    ) -> Vec<Arc<TerminalSession>> {
        let mut sessions = self
            .sessions
            .write()
            .expect("terminal registry lock poisoned");
        let mut removed = Vec::new();
        sessions.retain(|_, session| {
            if work_root_ids.contains(&session.work_root_id) {
                removed.push(session.clone());
                false
            } else {
                true
            }
        });
        removed
    }
}

fn identity_status(pid: u32, start_time: u64) -> IdentityStatus {
    match crate::terminal_platform::process_start_time(pid) {
        Some(observed) if observed == start_time => IdentityStatus::VerifiedOurs,
        Some(_) => IdentityStatus::PidReused,
        None => IdentityStatus::NoSuchProcess,
    }
}

/// Result of a successful connect + handshake against a helper's IPC
/// listener: the still-open reader/writer halves plus the identity and
/// initial status the helper reported. Shared by fresh `create_terminal`
/// spawns and boot-reconcile adoption - both need exactly this handshake
/// shape (see `terminal_helper_process.rs::handle_connection`).
struct HandshakeConnection {
    reader: NdjsonReader<IpcReadHalf>,
    writer: IpcWriteHalf,
    pid: u32,
    start_time: u64,
    status: TerminalStatus,
    next_sequence: u64,
}

async fn connect_and_handshake(socket_path: &Path, timeout: Duration) -> Option<HandshakeConnection> {
    let deadline = Instant::now() + timeout;
    let stream = loop {
        match crate::terminal_ipc_transport::connect(socket_path).await {
            Ok(stream) => break stream,
            Err(_) => {
                if Instant::now() >= deadline {
                    return None;
                }
                tokio::time::sleep(Duration::from_millis(20)).await;
            }
        }
    };
    let (read_half, mut write_half) = crate::terminal_ipc_transport::split(stream);
    let mut reader = NdjsonReader::new(read_half);

    let remaining = deadline.saturating_duration_since(Instant::now());
    let handshake = match tokio::time::timeout(remaining, reader.read_message::<HelperToDaemonMessage>()).await {
        Ok(Ok(Some(message))) => message,
        _ => return None,
    };
    let HelperToDaemonMessage::Handshake { pid, start_time } = handshake else {
        return None;
    };

    let remaining = deadline.saturating_duration_since(Instant::now());
    let status_message = match tokio::time::timeout(remaining, reader.read_message::<HelperToDaemonMessage>()).await {
        Ok(Ok(Some(message))) => message,
        _ => return None,
    };
    let (status, next_sequence) = match status_message {
        HelperToDaemonMessage::Status {
            status,
            next_sequence,
        } => (status.into(), next_sequence),
        HelperToDaemonMessage::Exit {
            status,
            next_sequence,
        } => (status.into(), next_sequence),
        _ => return None,
    };

    write_ndjson(&mut write_half, &DaemonToHelperMessage::HandshakeAck)
        .await
        .ok()?;

    Some(HandshakeConnection {
        reader,
        writer: write_half,
        pid,
        start_time,
        status,
        next_sequence,
    })
}

pub struct TerminalSession {
    id: String,
    work_root_id: WorkRootId,
    title: String,
    cwd_hint: Option<String>,
    created_at_ms: u64,
    pid: u32,
    start_time: u64,
    write_half: Arc<AsyncMutex<IpcWriteHalf>>,
    inner: Mutex<TerminalSessionInner>,
    output_signal: watch::Sender<u64>,
}

struct TerminalSessionInner {
    status: TerminalStatus,
    columns: u16,
    rows: u16,
    // CONTRACT: this is a daemon-side *cache*, not the source of truth - the
    // helper owns the authoritative bounded ring (see
    // `terminal_helper_process.rs::RingState`) and pushes every chunk over
    // IPC as it is produced. On (re)connect (fresh create, grace-reattach,
    // or boot-reconcile adopt) the helper unconditionally flushes its whole
    // retained ring BEFORE entering its per-connection select loop (see
    // `handle_connection`'s matching CONTRACT comment in
    // `terminal_helper_process.rs`) - this is a deterministic, one-shot
    // push on every connect, not something that merely happens to fire via
    // a pending `Notify` permit, which is what makes this cache's
    // bootstrap/backfill on adopt reliable even for an already-quiescent
    // shell with no further output after reconnect.
    output: VecDeque<TerminalOutputChunk>,
    next_sequence: u64,
    grace_until_ms: Option<u64>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalSessionView {
    terminal_id: String,
    work_root_id: WorkRootId,
    title: String,
    status: TerminalStatus,
    columns: u16,
    rows: u16,
    created_at_ms: u64,
    cwd_hint: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalOutputView {
    terminal_id: String,
    status: TerminalStatus,
    next_sequence: u64,
    chunks: Vec<TerminalOutputChunk>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalOutputChunk {
    sequence: u64,
    data: String,
    stream: String,
}

// CONTRACT: Terminal WebSocket server frames are the public live terminal
// stream from daemon to browser. Output frames preserve the same ordered PTY
// chunk semantics as the HTTP backfill route; status frames report terminal
// lifecycle changes; exit frames end the live attachment without making the
// browser connection own the daemon process lifecycle.
//
// STABILITY (ticket 260723 Phase 1, Decision A): this type and
// `TerminalWebSocketClientMessage` are the ONLY browser-facing wire types.
// The daemon<->helper protocol (`terminal_helper_protocol.rs`) is a
// deliberately separate type hierarchy; nothing in this phase changes the
// shape or semantics of these two enums or `TerminalSessionView`/
// `TerminalOutputChunk` below.
#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum TerminalWebSocketServerMessage {
    Output {
        #[serde(rename = "terminalId")]
        terminal_id: String,
        chunk: TerminalOutputChunk,
    },
    Status {
        #[serde(rename = "terminalId")]
        terminal_id: String,
        status: TerminalStatus,
        #[serde(rename = "nextSequence")]
        next_sequence: u64,
        truncated: bool,
    },
    Exit {
        #[serde(rename = "terminalId")]
        terminal_id: String,
        status: TerminalStatus,
        #[serde(rename = "nextSequence")]
        next_sequence: u64,
        truncated: bool,
    },
}

// CONTRACT: Terminal WebSocket client frames are the public live browser to
// daemon terminal control stream. Input data is raw terminal data from xterm's
// onData callback; resize uses the existing bounded PTY size contract.
#[derive(Clone, Debug, Eq, PartialEq, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum TerminalWebSocketClientMessage {
    Input { data: String },
    Resize { columns: u16, rows: u16 },
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum TerminalStatus {
    Running,
    Exited,
    Terminated,
    Error,
}

impl From<TerminalHelperStatus> for TerminalStatus {
    fn from(status: TerminalHelperStatus) -> Self {
        match status {
            TerminalHelperStatus::Running => TerminalStatus::Running,
            TerminalHelperStatus::Exited => TerminalStatus::Exited,
            TerminalHelperStatus::Terminated => TerminalStatus::Terminated,
            TerminalHelperStatus::Error => TerminalStatus::Error,
        }
    }
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct CreateTerminalRequest {
    #[serde(default = "default_columns")]
    columns: u16,
    #[serde(default = "default_rows")]
    rows: u16,
    title: Option<String>,
    cwd_hint: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct TerminalInputRequest {
    data: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct TerminalResizeRequest {
    columns: u16,
    rows: u16,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
pub struct TerminalOutputQuery {
    #[serde(default)]
    after: u64,
}

// CONTRACT (260723 Phase 1 batch fallback poll): one HTTP round trip carries
// every fallback-polling pane's cursor instead of one request per terminal.
// `terminal_id` is per-cursor (not per-request) because a single batch
// request already spans every terminal a browser tab is polling for one
// work root's serverRoute.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct TerminalOutputCursor {
    terminal_id: String,
    #[serde(default)]
    after: u64,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct TerminalOutputBatchRequest {
    cursors: Vec<TerminalOutputCursor>,
}

// CONTRACT: unknown or currently-inaccessible (offline/unavailable work root)
// terminal IDs are silently omitted from `results`, never a per-ID error and
// never a whole-batch failure - the same per-terminal auth/work-root gating
// as the single-ID `terminal_output` handler, just non-fatal per cursor.
#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalOutputBatchResponse {
    results: HashMap<String, TerminalOutputView>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
pub struct TerminalWebSocketQuery {
    #[serde(default)]
    pub after: u64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct TerminalErrorView {
    error: String,
}

pub async fn create_terminal(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Json(request): Json<CreateTerminalRequest>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let root_path = match resolve_online_available_work_root(&state, &work_root_id) {
        Ok(root_path) => root_path,
        Err(error) => return terminal_access_error(error),
    };
    let Ok((columns, rows)) = validate_size(request.columns, request.rows) else {
        return terminal_error(StatusCode::BAD_REQUEST, "invalid terminal size");
    };

    match TerminalSession::spawn(
        &state.terminals.helper_binary,
        &state.terminals.registry_dir,
        state.terminals.connect_timeout,
        work_root_id,
        root_path,
        request.title.unwrap_or_else(|| "Terminal".to_owned()),
        columns,
        rows,
        request.cwd_hint,
    )
    .await
    {
        Ok(session) => {
            let view = session.view();
            match state.terminals.insert(session.clone()) {
                Ok(()) => Json(view).into_response(),
                Err(error) => {
                    session.terminate().await;
                    error.into_response()
                }
            }
        }
        Err(error) => error.into_response(),
    }
}

pub async fn list_terminals(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    if let Err(error) = resolve_online_available_work_root(&state, &work_root_id) {
        return terminal_access_error(error);
    }
    Json(state.terminals.list_for_work_root(&work_root_id)).into_response()
}

pub async fn terminal_output(
    State(state): State<AppState>,
    AxumPath(terminal_id): AxumPath<String>,
    Query(query): Query<TerminalOutputQuery>,
) -> Response {
    let Some(session) = state.terminals.get(&terminal_id) else {
        return terminal_error(StatusCode::NOT_FOUND, "unknown terminal");
    };
    if let Err(error) = resolve_online_available_work_root(&state, &session.work_root_id) {
        return terminal_access_error(error);
    }
    Json(session.output_after(query.after)).into_response()
}

// CONTRACT (260723 Phase 1): a batch never fails as a whole - a missing
// registry entry or a per-terminal work-root access error just drops that
// one cursor from `results` and moves on to the next, mirroring
// `terminal_output`'s own per-terminal gating without ever returning
// non-200 for the request as a whole.
pub async fn terminal_output_batch(
    State(state): State<AppState>,
    Json(request): Json<TerminalOutputBatchRequest>,
) -> Response {
    let mut results = HashMap::with_capacity(request.cursors.len());
    for cursor in request.cursors {
        let Some(session) = state.terminals.get(&cursor.terminal_id) else {
            continue;
        };
        if resolve_online_available_work_root(&state, &session.work_root_id).is_err() {
            continue;
        }
        results.insert(cursor.terminal_id, session.output_after(cursor.after));
    }
    Json(TerminalOutputBatchResponse { results }).into_response()
}

pub async fn terminal_input(
    State(state): State<AppState>,
    AxumPath(terminal_id): AxumPath<String>,
    Json(request): Json<TerminalInputRequest>,
) -> Response {
    let Some(session) = state.terminals.get(&terminal_id) else {
        return terminal_error(StatusCode::NOT_FOUND, "unknown terminal");
    };
    if let Err(error) = resolve_online_available_work_root(&state, &session.work_root_id) {
        return terminal_access_error(error);
    }
    match session.write_input(request.data.as_bytes()).await {
        Ok(()) => StatusCode::NO_CONTENT.into_response(),
        Err(error) => error.into_response(),
    }
}

pub async fn terminal_resize(
    State(state): State<AppState>,
    AxumPath(terminal_id): AxumPath<String>,
    Json(request): Json<TerminalResizeRequest>,
) -> Response {
    let Some(session) = state.terminals.get(&terminal_id) else {
        return terminal_error(StatusCode::NOT_FOUND, "unknown terminal");
    };
    if let Err(error) = resolve_online_available_work_root(&state, &session.work_root_id) {
        return terminal_access_error(error);
    }
    let Ok((columns, rows)) = validate_size(request.columns, request.rows) else {
        return terminal_error(StatusCode::BAD_REQUEST, "invalid terminal size");
    };
    match session.resize(columns, rows).await {
        Ok(view) => Json(view).into_response(),
        Err(error) => error.into_response(),
    }
}

pub async fn terminal_websocket(
    State(state): State<AppState>,
    AxumPath(terminal_id): AxumPath<String>,
    Query(query): Query<TerminalWebSocketQuery>,
    upgrade: WebSocketUpgrade,
) -> Response {
    // CONTRACT: This route is nested behind the owner auth and Host/Origin
    // pre-upgrade gate in router.rs. Implementation must reject unknown or
    // closed opaque terminal ids before accepting the WebSocket attachment.
    // The Axum WebSocketUpgrade extractor is accepted only after
    // TerminalRegistry::get confirms a live-or-in-grace session;
    // terminal_socket_task owns output backfill, resize/input frames, and
    // close propagation.
    let Some(session) = state.terminals.get(&terminal_id) else {
        return terminal_error(StatusCode::NOT_FOUND, "unknown terminal");
    };
    if let Err(error) = resolve_online_available_work_root(&state, &session.work_root_id) {
        return terminal_access_error(error);
    }
    if !session.admits_attach() {
        return terminal_error(StatusCode::GONE, "terminal is closed");
    }
    upgrade
        .on_upgrade(move |socket| terminal_socket_task(state, session, socket, query.after))
        .into_response()
}

pub async fn close_terminal(
    State(state): State<AppState>,
    AxumPath(terminal_id): AxumPath<String>,
) -> Response {
    let Some(session) = state.terminals.get(&terminal_id) else {
        return terminal_error(StatusCode::NOT_FOUND, "unknown terminal");
    };
    if let Err(error) = resolve_online_available_work_root(&state, &session.work_root_id) {
        return terminal_access_error(error);
    }
    let Some(session) = state.terminals.remove(&terminal_id) else {
        return terminal_error(StatusCode::NOT_FOUND, "unknown terminal");
    };
    session.terminate().await;
    StatusCode::NO_CONTENT.into_response()
}

impl TerminalSession {
    #[allow(clippy::too_many_arguments)]
    async fn spawn(
        helper_binary: &Path,
        registry_dir: &Path,
        connect_timeout: Duration,
        work_root_id: WorkRootId,
        root_path: PathBuf,
        title: String,
        columns: u16,
        rows: u16,
        cwd_hint: Option<String>,
    ) -> Result<Arc<Self>, TerminalError> {
        let (spawn_cwd, normalized_cwd_hint) = resolve_terminal_cwd(&root_path, cwd_hint)?;
        let id = opaque_terminal_id();
        let socket_path = registry_dir.join(format!("{id}.sock"));

        let mut command = std::process::Command::new(helper_binary);
        command
            .arg("terminal-helper")
            .arg("--registry-dir")
            .arg(registry_dir)
            .arg("--terminal-id")
            .arg(&id)
            .arg("--work-root-id")
            .arg(work_root_id.as_str())
            .arg("--cwd")
            .arg(&spawn_cwd)
            .arg("--title")
            .arg(&title)
            .arg("--columns")
            .arg(columns.to_string())
            .arg("--rows")
            .arg(rows.to_string())
            .arg("--socket-path")
            .arg(&socket_path);
        if let Some(hint) = normalized_cwd_hint.as_deref() {
            command.arg("--cwd-hint").arg(hint);
        }
        command
            .stdin(std::process::Stdio::null())
            .stdout(std::process::Stdio::null())
            .stderr(std::process::Stdio::null());

        tokio::task::spawn_blocking(move || crate::terminal_platform::spawn_detached(command))
            .await
            .map_err(|_| TerminalError::BadRequest("terminal spawn failed"))?
            .map_err(|_| TerminalError::BadRequest("terminal spawn failed"))?;

        let connected = connect_and_handshake(&socket_path, connect_timeout)
            .await
            .ok_or(TerminalError::BadRequest("terminal spawn failed"))?;

        Ok(Self::from_connection(
            id,
            work_root_id,
            title,
            normalized_cwd_hint,
            now_ms(),
            connected,
            columns,
            rows,
        ))
    }

    fn from_connection(
        id: String,
        work_root_id: WorkRootId,
        title: String,
        cwd_hint: Option<String>,
        created_at_ms: u64,
        connected: HandshakeConnection,
        columns: u16,
        rows: u16,
    ) -> Arc<Self> {
        let grace_until_ms = (connected.status != TerminalStatus::Running)
            .then(|| now_ms() + DAEMON_GRACE_WINDOW_MS);
        let session = Arc::new(Self {
            id,
            work_root_id,
            title,
            cwd_hint,
            created_at_ms,
            pid: connected.pid,
            start_time: connected.start_time,
            write_half: Arc::new(AsyncMutex::new(connected.writer)),
            inner: Mutex::new(TerminalSessionInner {
                status: connected.status,
                columns,
                rows,
                output: VecDeque::new(),
                next_sequence: connected.next_sequence,
                grace_until_ms,
            }),
            output_signal: watch::channel(0).0,
        });
        spawn_ipc_reader_task(session.clone(), connected.reader);
        session
    }

    fn view(&self) -> TerminalSessionView {
        let inner = self.inner.lock().expect("terminal session lock poisoned");
        TerminalSessionView {
            terminal_id: self.id.clone(),
            work_root_id: self.work_root_id.clone(),
            title: self.title.clone(),
            status: inner.status,
            columns: inner.columns,
            rows: inner.rows,
            created_at_ms: self.created_at_ms,
            cwd_hint: self.cwd_hint.clone(),
        }
    }

    fn is_live(&self) -> bool {
        matches!(
            self.inner
                .lock()
                .expect("terminal session lock poisoned")
                .status,
            TerminalStatus::Running
        )
    }

    // CONTRACT (grace-reattach, ticket "Boot reconcile policy" row 2): a
    // session that has exited but is still inside its grace window remains
    // visible/attachable even though `is_live()` is false. Every OTHER
    // `is_live()` call site (`write_input`, `resize`, eviction `retain`)
    // deliberately keeps the strict Running-only check - only the WS
    // upgrade gate and the work-root listing use this relaxed predicate.
    fn admits_attach(&self) -> bool {
        let inner = self.inner.lock().expect("terminal session lock poisoned");
        inner.status == TerminalStatus::Running
            || inner
                .grace_until_ms
                .is_some_and(|deadline| now_ms() < deadline)
    }

    // CONTRACT: this replaces a `filter(|c| c.sequence > after)` scan with
    // direct index arithmetic. It is only valid because `append_output_from_
    // helper` (see below) maintains a gapless, strictly-contiguous
    // `sequence` numbering (each push consumes exactly one `next_sequence`
    // value) and only ever evicts from the front (`pop_front`, never mid-
    // deque removal). If either invariant changes, this shortcut must be
    // revisited.
    fn output_after(&self, after: u64) -> TerminalOutputView {
        let inner = self.inner.lock().expect("terminal session lock poisoned");
        let front_seq = inner.output.front().map(|chunk| chunk.sequence);
        let skip = match front_seq {
            Some(front_seq) => after
                .saturating_add(1)
                .saturating_sub(front_seq)
                .min(inner.output.len() as u64) as usize,
            None => 0,
        };
        TerminalOutputView {
            terminal_id: self.id.clone(),
            status: inner.status,
            next_sequence: inner.next_sequence,
            chunks: inner.output.iter().skip(skip).cloned().collect(),
        }
    }

    fn status_and_next_sequence(&self) -> (TerminalStatus, u64) {
        let inner = self.inner.lock().expect("terminal session lock poisoned");
        (inner.status, inner.next_sequence)
    }

    // Reports whether a client resuming from `after` has missed retained
    // history: only meaningful for a genuine resume (`after > 0`; `after ==
    // 0` always means "send me everything you have", never a gap - see
    // Phase 4 plan risk signal), and only true when the oldest retained
    // chunk's sequence is past `after + 1`, i.e. there is a real hole between
    // what the client last saw and what is still retained.
    fn is_range_truncated(&self, after: u64) -> bool {
        let inner = self.inner.lock().expect("terminal session lock poisoned");
        after > 0 && inner.output.front().is_some_and(|chunk| chunk.sequence > after + 1)
    }

    async fn write_input(&self, input: &[u8]) -> Result<(), TerminalError> {
        if input.len() > MAX_INPUT_BYTES {
            return Err(TerminalError::BadRequest("terminal input too large"));
        }
        // Fast path stays cheap: an already-closed terminal must return
        // `Gone` without touching the IPC connection at all.
        if !self.is_live() {
            return Err(TerminalError::Gone("terminal is closed"));
        }
        // The daemon<->helper wire is NDJSON/UTF-8 text (see
        // `terminal_helper_protocol.rs`); the browser contract already
        // types terminal input as UTF-8 `String` (`TerminalInputRequest`,
        // `TerminalWebSocketClientMessage::Input`) for the primary paths.
        // The WS binary-frame path funnels arbitrary bytes through here too
        // - a lossy conversion is a deliberate, documented simplification
        // for this phase rather than adding a base64 wire encoding.
        let data = String::from_utf8_lossy(input).into_owned();
        let mut writer = self.write_half.lock().await;
        let _ = write_ndjson(&mut *writer, &DaemonToHelperMessage::Input { data }).await;
        Ok(())
    }

    async fn resize(&self, columns: u16, rows: u16) -> Result<TerminalSessionView, TerminalError> {
        if !self.is_live() {
            return Err(TerminalError::Gone("terminal is closed"));
        }
        {
            let mut inner = self.inner.lock().expect("terminal session lock poisoned");
            inner.columns = columns;
            inner.rows = rows;
        }
        let mut writer = self.write_half.lock().await;
        let _ = write_ndjson(&mut *writer, &DaemonToHelperMessage::Resize { columns, rows }).await;
        drop(writer);
        Ok(self.view())
    }

    // 2-tier kill (ticket-pinned): prefer a graceful IPC request first (the
    // helper `child.kill()`s its own shell and exits cleanly); ALWAYS follow
    // up with a verified-PID kill after a short delay regardless of whether
    // the graceful write appeared to succeed - a hung-but-still-connected
    // helper can accept the write into its socket buffer without ever
    // processing it, and an already-gone helper simply makes the verified
    // kill a harmless no-op (identity will not verify).
    pub(crate) async fn terminate(&self) {
        let next_sequence = {
            let mut inner = self.inner.lock().expect("terminal session lock poisoned");
            inner.status = TerminalStatus::Terminated;
            inner.next_sequence
        };
        {
            let mut writer = self.write_half.lock().await;
            let _ = write_ndjson(&mut *writer, &DaemonToHelperMessage::GracefulShutdown).await;
        }
        tokio::time::sleep(Duration::from_millis(200)).await;
        let pid = self.pid;
        let start_time = self.start_time;
        let _ = tokio::task::spawn_blocking(move || {
            crate::terminal_platform::kill_verified(pid, start_time)
        })
        .await;
        let _ = self.output_signal.send(next_sequence);
    }

    fn append_output_from_helper(&self, chunk: TerminalHelperOutputChunk) {
        let next_sequence = {
            let mut inner = self.inner.lock().expect("terminal session lock poisoned");
            inner.output.push_back(TerminalOutputChunk {
                sequence: chunk.sequence,
                data: chunk.data,
                stream: "pty".to_owned(),
            });
            while inner.output.len() > MAX_OUTPUT_CHUNKS {
                inner.output.pop_front();
            }
            inner.next_sequence = inner.next_sequence.max(chunk.sequence + 1);
            inner.next_sequence
        };
        let _ = self.output_signal.send(next_sequence);
    }

    fn apply_helper_status(&self, status: TerminalStatus, next_sequence: u64) {
        let seq = {
            let mut inner = self.inner.lock().expect("terminal session lock poisoned");
            inner.status = status;
            inner.next_sequence = inner.next_sequence.max(next_sequence);
            if status != TerminalStatus::Running && inner.grace_until_ms.is_none() {
                inner.grace_until_ms = Some(now_ms() + DAEMON_GRACE_WINDOW_MS);
            }
            inner.next_sequence
        };
        let _ = self.output_signal.send(seq);
    }

    // The IPC connection dropped unexpectedly (helper crashed, or otherwise
    // vanished without a clean `Exit` message) - distinct from a
    // daemon-initiated `terminate()`, which already set `Terminated` before
    // ever touching the connection.
    fn mark_ipc_closed(&self) {
        let seq = {
            let mut inner = self.inner.lock().expect("terminal session lock poisoned");
            if inner.status == TerminalStatus::Running {
                inner.status = TerminalStatus::Error;
                inner.grace_until_ms = Some(now_ms() + DAEMON_GRACE_WINDOW_MS);
            }
            inner.next_sequence
        };
        let _ = self.output_signal.send(seq);
    }
}

fn spawn_ipc_reader_task(
    session: Arc<TerminalSession>,
    mut reader: NdjsonReader<IpcReadHalf>,
) {
    tokio::spawn(async move {
        loop {
            match reader.read_message::<HelperToDaemonMessage>().await {
                Ok(Some(HelperToDaemonMessage::Handshake { .. })) => {
                    // Only meaningful at connect time; `connect_and_handshake`
                    // already consumed the one handshake message this
                    // connection will ever send.
                }
                Ok(Some(HelperToDaemonMessage::Output(chunk))) => {
                    session.append_output_from_helper(chunk);
                }
                Ok(Some(HelperToDaemonMessage::Status {
                    status,
                    next_sequence,
                })) => session.apply_helper_status(status.into(), next_sequence),
                Ok(Some(HelperToDaemonMessage::Exit {
                    status,
                    next_sequence,
                })) => session.apply_helper_status(status.into(), next_sequence),
                Ok(Some(HelperToDaemonMessage::BackfillResponse { .. })) => {
                    // Not consumed in Stage 1 - the push-on-connect
                    // mechanism (see `TerminalSessionInner::output`'s
                    // CONTRACT comment) already covers the adopt/reattach
                    // bootstrap case this would otherwise serve.
                }
                Ok(None) | Err(_) => {
                    session.mark_ipc_closed();
                    break;
                }
            }
        }
    });
}

async fn terminal_socket_task(
    state: AppState,
    session: Arc<TerminalSession>,
    socket: WebSocket,
    after: u64,
) {
    let (mut sender, mut receiver) = socket.split();
    let mut output_signal = session.output_signal.subscribe();
    let mut cursor = after;

    if resolve_online_available_work_root(&state, &session.work_root_id).is_err() {
        return;
    }
    if send_output_backfill(&session, &mut sender, &mut cursor)
        .await
        .is_err()
    {
        return;
    }

    loop {
        tokio::select! {
            maybe_message = receiver.next() => {
                let Some(Ok(message)) = maybe_message else { break; };
                match message {
                    Message::Text(text) => {
                        if resolve_online_available_work_root(&state, &session.work_root_id).is_err() {
                            let _ = send_terminal_socket_status(&session, &mut sender, false, false).await;
                            break;
                        }
                        let Ok(message) = serde_json::from_str::<TerminalWebSocketClientMessage>(&text) else {
                            break;
                        };
                        if handle_terminal_socket_client_message(session.clone(), message).await.is_err() {
                            let _ = send_terminal_socket_status(&session, &mut sender, false, false).await;
                            break;
                        }
                    }
                    Message::Binary(bytes) => {
                        if resolve_online_available_work_root(&state, &session.work_root_id).is_err() {
                            let _ = send_terminal_socket_status(&session, &mut sender, false, false).await;
                            break;
                        }
                        if session.write_input(&bytes).await.is_err() {
                            let _ = send_terminal_socket_status(&session, &mut sender, false, false).await;
                            break;
                        }
                    }
                    Message::Close(_) => break,
                    Message::Ping(payload) => {
                        if sender.send(Message::Pong(payload)).await.is_err() { break; }
                    }
                    Message::Pong(_) => {}
                }
            }
            changed = output_signal.changed() => {
                if changed.is_err() { break; }
                if resolve_online_available_work_root(&state, &session.work_root_id).is_err() {
                    break;
                }
                if send_output_backfill(&session, &mut sender, &mut cursor).await.is_err() {
                    break;
                }
                if !session.admits_attach() {
                    break;
                }
            }
        }
    }
}

async fn handle_terminal_socket_client_message(
    session: Arc<TerminalSession>,
    message: TerminalWebSocketClientMessage,
) -> Result<(), TerminalError> {
    match message {
        TerminalWebSocketClientMessage::Input { data } => session.write_input(data.as_bytes()).await,
        TerminalWebSocketClientMessage::Resize { columns, rows } => {
            let (columns, rows) = validate_size(columns, rows)
                .map_err(|_| TerminalError::BadRequest("invalid terminal size"))?;
            session.resize(columns, rows).await.map(|_| ())
        }
    }
}

// Pulled out of `send_output_backfill` so the requested-cursor-vs-advanced-
// cursor ordering (the primary wiring risk this feature was built around -
// see Phase 4 plan) is exercisable by a plain unit test, without needing a
// live WebSocket sink. `cursor` is advanced by this function exactly the
// same way `send_output_backfill` used to advance it inline; `truncated` is
// always computed from the cursor value as requested at entry, before this
// function's own loop advances `*cursor`.
struct OutputBackfillPlan {
    chunks: Vec<TerminalOutputChunk>,
    truncated: bool,
}

fn plan_output_backfill(session: &TerminalSession, cursor: &mut u64) -> OutputBackfillPlan {
    let requested_after = *cursor;
    let output = session.output_after(requested_after);
    let truncated = session.is_range_truncated(requested_after);
    for chunk in &output.chunks {
        *cursor = (*cursor).max(chunk.sequence);
    }
    OutputBackfillPlan {
        chunks: output.chunks,
        truncated,
    }
}

async fn send_output_backfill(
    session: &TerminalSession,
    sender: &mut futures_util::stream::SplitSink<WebSocket, Message>,
    cursor: &mut u64,
) -> Result<(), ()> {
    let plan = plan_output_backfill(session, cursor);
    for chunk in plan.chunks {
        send_socket_json(
            sender,
            &TerminalWebSocketServerMessage::Output {
                terminal_id: session.id.clone(),
                chunk,
            },
        )
        .await?;
    }
    send_terminal_socket_status(session, sender, !session.is_live(), plan.truncated).await
}

async fn send_terminal_socket_status(
    session: &TerminalSession,
    sender: &mut futures_util::stream::SplitSink<WebSocket, Message>,
    exit: bool,
    truncated: bool,
) -> Result<(), ()> {
    let (status, next_sequence) = session.status_and_next_sequence();
    let message = if exit {
        TerminalWebSocketServerMessage::Exit {
            terminal_id: session.id.clone(),
            status,
            next_sequence,
            truncated,
        }
    } else {
        TerminalWebSocketServerMessage::Status {
            terminal_id: session.id.clone(),
            status,
            next_sequence,
            truncated,
        }
    };
    send_socket_json(sender, &message).await
}

async fn send_socket_json(
    sender: &mut futures_util::stream::SplitSink<WebSocket, Message>,
    message: &TerminalWebSocketServerMessage,
) -> Result<(), ()> {
    let text = serde_json::to_string(message).map_err(|_| ())?;
    sender
        .send(Message::Text(text.into()))
        .await
        .map_err(|_| ())
}

fn validate_size(columns: u16, rows: u16) -> Result<(u16, u16), ()> {
    if (MIN_COLUMNS..=MAX_COLUMNS).contains(&columns) && (MIN_ROWS..=MAX_ROWS).contains(&rows) {
        Ok((columns, rows))
    } else {
        Err(())
    }
}

fn resolve_terminal_cwd(
    root_path: &Path,
    cwd_hint: Option<String>,
) -> Result<(PathBuf, Option<String>), TerminalError> {
    let Some(raw_hint) = cwd_hint else {
        return Ok((root_path.to_path_buf(), None));
    };
    let trimmed = raw_hint.trim();
    if trimmed.is_empty() || trimmed == "." {
        return Ok((root_path.to_path_buf(), None));
    }

    let hint_path = Path::new(trimmed);
    let mut normalized = PathBuf::new();
    for component in hint_path.components() {
        match component {
            Component::Normal(part) => normalized.push(part),
            Component::CurDir => {}
            Component::ParentDir | Component::RootDir | Component::Prefix(_) => {
                return Err(TerminalError::BadRequest("invalid terminal cwd"));
            }
        }
    }

    if normalized.as_os_str().is_empty() {
        return Ok((root_path.to_path_buf(), None));
    }
    let spawn_cwd = root_path.join(&normalized);
    if !spawn_cwd.is_dir() {
        return Err(TerminalError::BadRequest("terminal cwd not found"));
    }
    Ok((
        spawn_cwd,
        Some(normalized.to_string_lossy().replace('\\', "/")),
    ))
}

fn terminal_error(status: StatusCode, error: impl Into<String>) -> Response {
    (
        status,
        Json(TerminalErrorView {
            error: error.into(),
        }),
    )
        .into_response()
}

fn terminal_access_error(error: WorkRootAccessError) -> Response {
    terminal_error(error.status(), error.message())
}

// CONTRACT: called from `terminal_helper_process.rs` (the helper picks its
// own shell) as well as `terminal.rs`'s own tests - it must stay pure/
// testable and must not assume it is running inside the daemon process.
pub(crate) fn default_shell() -> PathBuf {
    #[cfg(windows)]
    {
        select_terminal_shell(TerminalPlatform::Windows, |key| std::env::var_os(key)).program
    }
    #[cfg(not(windows))]
    {
        select_terminal_shell(TerminalPlatform::Unix, |key| std::env::var_os(key)).program
    }
}

pub(crate) fn browser_pty_term(env: impl Fn(&str) -> Option<String>) -> String {
    env("TERM")
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty() && value != "dumb")
        .unwrap_or_else(|| DEFAULT_BROWSER_PTY_TERM.to_owned())
}

#[cfg(test)]
mod terminal_portability_skeleton_tests {
    use super::*;

    #[test]
    fn terminal_shell_selection_contract_targets() {
        // CONTRACT: Fill executable assertions for SHELL, PowerShell, COMSPEC, Unix
        // fallback, Windows fallback, invalid/missing env values where
        // practical, and spawn cwd diagnostics.
        let unix_env = |key: &str| (key == "SHELL").then(|| std::ffi::OsString::from("/bin/zsh"));
        assert_eq!(
            select_terminal_shell(TerminalPlatform::Unix, unix_env),
            TerminalShellSelection {
                platform: TerminalPlatform::Unix,
                program: PathBuf::from("/bin/zsh"),
                source: TerminalShellSource::ShellEnv,
            }
        );

        let windows_env = |key: &str| {
            (key == "COMSPEC").then(|| std::ffi::OsString::from(r"C:\Windows\System32\cmd.exe"))
        };
        assert_eq!(
            select_terminal_shell_with_detector(
                TerminalPlatform::Windows,
                &windows_env,
                |program| {
                    (program == "pwsh.exe")
                        .then(|| PathBuf::from(r"C:\Program Files\PowerShell\7\pwsh.exe"))
                }
            ),
            TerminalShellSelection {
                platform: TerminalPlatform::Windows,
                program: PathBuf::from(r"C:\Program Files\PowerShell\7\pwsh.exe"),
                source: TerminalShellSource::PwshPath,
            }
        );
        assert_eq!(
            select_terminal_shell_with_detector(
                TerminalPlatform::Windows,
                &windows_env,
                |program| {
                    (program == "powershell.exe").then(|| {
                        PathBuf::from(r"C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe")
                    })
                }
            ),
            TerminalShellSelection {
                platform: TerminalPlatform::Windows,
                program: PathBuf::from(
                    r"C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe"
                ),
                source: TerminalShellSource::WindowsPowerShellPath,
            }
        );
        assert_eq!(
            select_terminal_shell_with_detector(TerminalPlatform::Windows, &windows_env, |_| None),
            TerminalShellSelection {
                platform: TerminalPlatform::Windows,
                program: PathBuf::from(r"C:\Windows\System32\cmd.exe"),
                source: TerminalShellSource::ComspecEnv,
            }
        );

        assert_eq!(
            select_terminal_shell(TerminalPlatform::Unix, |_| None),
            TerminalShellSelection {
                platform: TerminalPlatform::Unix,
                program: PathBuf::from("/bin/sh"),
                source: TerminalShellSource::Fallback,
            }
        );
        assert_eq!(
            select_terminal_shell(TerminalPlatform::Windows, |_| None),
            TerminalShellSelection {
                platform: TerminalPlatform::Windows,
                program: PathBuf::from("cmd.exe"),
                source: TerminalShellSource::Fallback,
            }
        );
        assert_eq!(
            select_terminal_shell(TerminalPlatform::Unix, |key| {
                (key == "SHELL").then(std::ffi::OsString::new)
            })
            .source,
            TerminalShellSource::Fallback
        );
        assert_eq!(
            select_terminal_shell(TerminalPlatform::Windows, |key| {
                (key == "COMSPEC").then(std::ffi::OsString::new)
            })
            .source,
            TerminalShellSource::Fallback
        );
    }

    #[test]
    fn terminal_cwd_hint_stays_work_root_relative() {
        let root = std::env::temp_dir().join(format!("ws-terminal-cwd-{}", now_ms()));
        let nested = root.join("nested/child");
        std::fs::create_dir_all(&nested).expect("create nested cwd fixture");

        assert_eq!(
            resolve_terminal_cwd(&root, None).expect("root cwd"),
            (root.clone(), None)
        );
        assert_eq!(
            resolve_terminal_cwd(&root, Some("nested/child".to_owned())).expect("nested cwd"),
            (nested.clone(), Some("nested/child".to_owned()))
        );
        assert!(matches!(
            resolve_terminal_cwd(&root, Some("../outside".to_owned())),
            Err(TerminalError::BadRequest("invalid terminal cwd"))
        ));
        assert!(matches!(
            resolve_terminal_cwd(&root, Some("missing".to_owned())),
            Err(TerminalError::BadRequest("terminal cwd not found"))
        ));

        let _ = std::fs::remove_dir_all(&root);
    }

    #[test]
    fn browser_pty_term_uses_browser_capable_default_for_unusable_parent_term() {
        assert_eq!(browser_pty_term(|_| None), DEFAULT_BROWSER_PTY_TERM);
        assert_eq!(
            browser_pty_term(|key| (key == "TERM").then(|| "".to_owned())),
            DEFAULT_BROWSER_PTY_TERM
        );
        assert_eq!(
            browser_pty_term(|key| (key == "TERM").then(|| "   ".to_owned())),
            DEFAULT_BROWSER_PTY_TERM
        );
        assert_eq!(
            browser_pty_term(|key| (key == "TERM").then(|| "dumb".to_owned())),
            DEFAULT_BROWSER_PTY_TERM
        );
    }

    #[test]
    fn browser_pty_term_preserves_explicit_capable_parent_term() {
        assert_eq!(
            browser_pty_term(|key| (key == "TERM").then(|| "screen-256color".to_owned())),
            "screen-256color"
        );
        assert_eq!(
            browser_pty_term(|key| (key == "TERM").then(|| " xterm-kitty ".to_owned())),
            "xterm-kitty"
        );
    }

    // Builds a TerminalSession without spawning a real helper process, so
    // the ring buffer eviction / truncation-detection contract can be
    // exercised deterministically and fast, independent of a real PTY/IPC
    // round trip. `tokio::io::duplex()` gives `write_half` a real (but
    // unattached-to-any-helper) in-memory duplex half - cross-platform,
    // unlike a Unix socketpair; none of these tests send through it.
    async fn fake_terminal_session() -> TerminalSession {
        let (_peer, local) = tokio::io::duplex(4096);
        let (_read_half, write_half) =
            crate::terminal_ipc_transport::split(Box::new(local) as crate::terminal_ipc_transport::BoxedIpcStream);
        TerminalSession {
            id: opaque_terminal_id(),
            work_root_id: WorkRootId::from("fake-work-root".to_owned()),
            title: "fake".to_owned(),
            cwd_hint: None,
            created_at_ms: now_ms(),
            pid: std::process::id(),
            start_time: 0,
            write_half: Arc::new(AsyncMutex::new(write_half)),
            inner: Mutex::new(TerminalSessionInner {
                status: TerminalStatus::Running,
                columns: default_columns(),
                rows: default_rows(),
                output: VecDeque::new(),
                next_sequence: 1,
                grace_until_ms: None,
            }),
            output_signal: watch::channel(0).0,
        }
    }

    fn push_chunk(session: &TerminalSession, data: &str) {
        session.append_output_from_helper(TerminalHelperOutputChunk {
            sequence: {
                let mut inner = session.inner.lock().expect("terminal session lock poisoned");
                let sequence = inner.next_sequence;
                inner.next_sequence += 1;
                sequence
            },
            data: data.to_owned(),
        });
    }

    #[tokio::test]
    async fn is_range_truncated_never_fires_on_fresh_after_zero_attach() {
        let session = fake_terminal_session().await;
        for _ in 0..(MAX_OUTPUT_CHUNKS + 200) {
            push_chunk(&session, "x");
        }
        // A fresh pane always requests after=0 ("send me everything you
        // have"), even against a terminal that has already evicted far more
        // than MAX_OUTPUT_CHUNKS chunks - that must never be reported as a
        // gap, since the client never observed the evicted data in the first
        // place.
        assert!(!session.is_range_truncated(0));
    }

    #[tokio::test]
    async fn is_range_truncated_fires_only_for_a_genuine_resume_past_eviction() {
        let session = fake_terminal_session().await;
        for _ in 0..(MAX_OUTPUT_CHUNKS + 200) {
            push_chunk(&session, "x");
        }
        let oldest_retained = session
            .inner
            .lock()
            .expect("terminal session lock poisoned")
            .output
            .front()
            .expect("output non-empty after eviction")
            .sequence;

        // Resuming from a cursor at or before the last chunk the client
        // could still have seen contiguously is not a gap...
        assert!(!session.is_range_truncated(oldest_retained - 1));
        // ...but resuming from anything older than that has a real hole
        // between what the client last observed and what remains retained.
        assert!(session.is_range_truncated(oldest_retained - 2));
    }

    // This is the wiring the plan flagged as the primary risk: the
    // truncation check must use the cursor *as requested at entry*, not the
    // cursor after `plan_output_backfill`'s own loop has advanced it to the
    // last sent chunk's sequence. Exercises `plan_output_backfill` itself
    // (the real call site `send_output_backfill` delegates to), not just
    // `is_range_truncated` in isolation, so a regression that moved the
    // `requested_after` capture below the loop (or reused the
    // now-advanced `*cursor`) would fail this test: the post-loop cursor
    // always equals the newest retained chunk's sequence whenever any
    // chunks are sent, and `is_range_truncated` of that value can never be
    // true (the oldest retained chunk can never exceed the newest retained
    // chunk's sequence by more than zero), so the buggy ordering would
    // silently flip this assertion to `false`.
    #[tokio::test]
    async fn plan_output_backfill_computes_truncation_from_requested_cursor_not_advanced_cursor() {
        let session = fake_terminal_session().await;
        for _ in 0..(MAX_OUTPUT_CHUNKS + 200) {
            push_chunk(&session, "x");
        }
        let oldest_retained = session
            .inner
            .lock()
            .expect("terminal session lock poisoned")
            .output
            .front()
            .expect("output non-empty after eviction")
            .sequence;
        let newest_retained = session
            .inner
            .lock()
            .expect("terminal session lock poisoned")
            .output
            .back()
            .expect("output non-empty after eviction")
            .sequence;

        // Genuine resume past eviction: the client's cursor is older than
        // what's still retained, so the backfill loop will send chunks and
        // advance `cursor` all the way up to `newest_retained` - if the
        // truncation check used that advanced value instead of the
        // requested one, it would never see a gap.
        let mut cursor = oldest_retained - 2;
        let plan = plan_output_backfill(&session, &mut cursor);
        assert!(
            plan.truncated,
            "genuine resume past eviction must be reported as truncated"
        );
        assert_eq!(
            cursor, newest_retained,
            "cursor still advances to the newest sent chunk despite the gap"
        );

        // Normal resume, no gap: cursor equals the boundary right before
        // the oldest retained chunk, so nothing was missed.
        let mut cursor = oldest_retained - 1;
        let plan = plan_output_backfill(&session, &mut cursor);
        assert!(
            !plan.truncated,
            "contiguous resume at the retention boundary must not be reported as truncated"
        );
        assert_eq!(cursor, newest_retained);
    }

    // `output_after` was rewritten from a `filter(|c| c.sequence > after)`
    // scan to index arithmetic (see the CONTRACT comment on `output_after`).
    // This proves the new skip-based implementation returns byte-identical
    // `Vec<TerminalOutputChunk>` results to the old filter semantics, across
    // a deque pushed past `MAX_OUTPUT_CHUNKS` (eviction forced), for every
    // representative class of `after` value: before the retained window,
    // both eviction-boundary values, mid-window, both ends of the "no new
    // data" boundary, and near-`u64::MAX`.
    #[tokio::test]
    async fn output_after_index_arithmetic_matches_old_filter_semantics_across_eviction() {
        let session = fake_terminal_session().await;
        for _ in 0..(MAX_OUTPUT_CHUNKS + 200) {
            push_chunk(&session, "x");
        }
        let (front_seq, next_sequence) = {
            let inner = session.inner.lock().expect("terminal session lock poisoned");
            (
                inner
                    .output
                    .front()
                    .expect("output non-empty after eviction")
                    .sequence,
                inner.next_sequence,
            )
        };
        let mid_window = front_seq + (next_sequence - 1 - front_seq) / 2;

        let cases = [
            0,                    // before-window
            front_seq - 1,        // at-boundary: still contiguous, nothing missed
            front_seq,            // at-boundary: first evicted chunk excluded
            mid_window,           // mid-window
            next_sequence - 1,    // at-next_sequence - 1: last valid, empty result
            next_sequence,        // at-next_sequence: no new data, empty result
            u64::MAX - 1,         // near-u64::MAX
            u64::MAX,             // exactly u64::MAX
        ];

        for after in cases {
            let expected: Vec<TerminalOutputChunk> = {
                let inner = session.inner.lock().expect("terminal session lock poisoned");
                inner
                    .output
                    .iter()
                    .filter(|chunk| chunk.sequence > after)
                    .cloned()
                    .collect()
            };
            let actual = session.output_after(after).chunks;
            assert_eq!(
                actual, expected,
                "output_after({after}) mismatched old filter(seq > after) semantics"
            );
        }
    }

    #[tokio::test]
    async fn write_input_returns_gone_synchronously_after_terminal_status_is_not_running() {
        let session = fake_terminal_session().await;
        {
            let mut inner = session.inner.lock().expect("terminal session lock poisoned");
            inner.status = TerminalStatus::Terminated;
        }

        assert!(matches!(
            session.write_input(b"too-late").await,
            Err(TerminalError::Gone("terminal is closed"))
        ));
    }

    #[tokio::test]
    async fn admits_attach_stays_true_through_grace_window_after_exit() {
        let session = fake_terminal_session().await;
        session.apply_helper_status(TerminalStatus::Exited, 1);

        assert!(!session.is_live(), "exited session is not `is_live`");
        assert!(
            session.admits_attach(),
            "exited session inside its grace window must still admit attach"
        );
    }

    #[tokio::test]
    async fn admits_attach_becomes_false_once_grace_window_elapses() {
        let session = fake_terminal_session().await;
        session.apply_helper_status(TerminalStatus::Exited, 1);
        {
            let mut inner = session.inner.lock().expect("terminal session lock poisoned");
            inner.grace_until_ms = Some(0); // already elapsed
        }

        assert!(!session.admits_attach());
    }

    // CONTRACT (260723 Phase 1 binding item #2): the ticket's 6-row
    // boot-reconcile table's "never kill on unverified identity" rows must
    // be exercised end-to-end through the real async
    // `TerminalRegistry::boot_reconcile`, not merely through the pure
    // `terminal_reconcile::classify` unit tests - `reconcile_entry` has its
    // own explicit pre-`classify` short-circuit (see the CONTRACT comment on
    // `reconcile_entry`) that only these tests actually drive.
    //
    // NOTE (260723 Phase-1 review finding M-c, numbering): the two tests
    // below are named after `terminal_reconcile.rs`'s own list order, NOT
    // the ticket's literal row numbers - the first test
    // (`..._when_pid_does_not_exist`) covers the ticket's row 6 (PID gone),
    // and the second (`..._on_pid_reuse`) covers BOTH the ticket's row 3
    // (reachable + identity mismatch) and row 5 (unreachable + identity
    // mismatch), which are provably the same `PidReused` code branch here
    // since identity is checked before IPC is ever consulted - see the
    // matching numbering note atop `terminal_reconcile.rs`.
    #[cfg(unix)]
    #[tokio::test]
    async fn boot_reconcile_drops_entry_without_touching_anything_when_pid_does_not_exist() {
        let registry_dir = std::env::temp_dir().join(format!(
            "ws-dashboard-boot-reconcile-row3-{}-{}",
            std::process::id(),
            now_ms()
        ));
        let entry = TerminalRegistryEntry {
            terminal_id: "term_row3_no_such_process".to_owned(),
            work_root_id: "root-row3".to_owned(),
            // Implausibly high pid: exceeds any realistic /proc/sys/kernel/pid_max
            // (2^22 default ceiling), so no real process can ever hold it.
            pid: 0x7fff_fffe,
            start_time: 123,
            socket_path: registry_dir.join("term_row3_no_such_process.sock"),
            created_at_ms: now_ms(),
            title: "Row 3".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        };
        crate::terminal_registry_file::write_registry_entry(&registry_dir, &entry)
            .expect("write row-3 registry entry");

        let registry = TerminalRegistry::boot_reconcile(
            // Rows 3/5 short-circuit on identity failure before
            // `helper_binary` is ever touched (no spawn happens on this
            // path), so an unused placeholder is deliberate here - unlike
            // the real-process E2E test, this unit test cannot use
            // `CARGO_BIN_EXE_ws-dashboard` anyway (that env var is only
            // compile-time-defined inside integration test/bench targets,
            // not the lib crate's own `#[cfg(test)]` unit tests).
            PathBuf::from("/nonexistent-unused-helper-binary"),
            registry_dir.clone(),
            Duration::from_millis(200),
        )
        .await;

        assert!(
            registry.get(&entry.terminal_id).is_none(),
            "row 3 (NoSuchProcess) must never be adopted into the live registry"
        );
        assert!(
            scan_registry_dir(&registry_dir).is_empty(),
            "row 3 must delete the stale registry entry file"
        );

        let _ = std::fs::remove_dir_all(&registry_dir);
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn boot_reconcile_drops_entry_and_never_kills_a_foreign_process_on_pid_reuse() {
        let registry_dir = std::env::temp_dir().join(format!(
            "ws-dashboard-boot-reconcile-row5-{}-{}",
            std::process::id(),
            now_ms()
        ));
        // A real, unrelated process is alive under `entry.pid`, but the
        // recorded `start_time` deliberately does not match it - simulating
        // the OS having recycled the pid for a different process since the
        // helper that owned this registry entry exited.
        let mut foreign = std::process::Command::new("sleep")
            .arg("5")
            .spawn()
            .expect("spawn foreign process for row-5 pid-reuse simulation");
        let foreign_pid = foreign.id();
        tokio::time::sleep(Duration::from_millis(50)).await;

        let entry = TerminalRegistryEntry {
            terminal_id: "term_row5_pid_reused".to_owned(),
            work_root_id: "root-row5".to_owned(),
            pid: foreign_pid,
            start_time: 1,
            socket_path: registry_dir.join("term_row5_pid_reused.sock"),
            created_at_ms: now_ms(),
            title: "Row 5".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        };
        crate::terminal_registry_file::write_registry_entry(&registry_dir, &entry)
            .expect("write row-5 registry entry");

        let registry = TerminalRegistry::boot_reconcile(
            // Rows 3/5 short-circuit on identity failure before
            // `helper_binary` is ever touched (no spawn happens on this
            // path), so an unused placeholder is deliberate here - unlike
            // the real-process E2E test, this unit test cannot use
            // `CARGO_BIN_EXE_ws-dashboard` anyway (that env var is only
            // compile-time-defined inside integration test/bench targets,
            // not the lib crate's own `#[cfg(test)]` unit tests).
            PathBuf::from("/nonexistent-unused-helper-binary"),
            registry_dir.clone(),
            Duration::from_millis(200),
        )
        .await;

        assert!(
            registry.get(&entry.terminal_id).is_none(),
            "row 5 (PidReused) must never be adopted into the live registry"
        );
        assert!(
            scan_registry_dir(&registry_dir).is_empty(),
            "row 5 must delete the stale registry entry file"
        );
        assert!(
            foreign
                .try_wait()
                .expect("poll foreign process status")
                .is_none(),
            "row 5 must never kill the foreign process merely occupying a reused pid"
        );

        let _ = foreign.kill();
        let _ = foreign.wait();
        let _ = std::fs::remove_dir_all(&registry_dir);
    }
}

fn opaque_terminal_id() -> String {
    let suffix: String = thread_rng()
        .sample_iter(&Alphanumeric)
        .take(18)
        .map(char::from)
        .collect();
    format!("term_{suffix}")
}

fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_millis() as u64)
        .unwrap_or(0)
}

fn default_columns() -> u16 {
    80
}

fn default_rows() -> u16 {
    24
}

#[derive(Debug)]
enum TerminalError {
    BadRequest(&'static str),
    Gone(&'static str),
}

impl TerminalError {
    fn into_response(self) -> Response {
        match self {
            Self::BadRequest(error) => terminal_error(StatusCode::BAD_REQUEST, error),
            Self::Gone(error) => terminal_error(StatusCode::GONE, error),
        }
    }
}
