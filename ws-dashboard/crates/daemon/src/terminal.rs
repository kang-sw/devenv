use std::collections::{HashMap, VecDeque};
use std::io::{Read, Write};
use std::path::{Component, Path, PathBuf};
use std::sync::{mpsc, Arc, Mutex, RwLock};
use std::thread;
use std::time::{SystemTime, UNIX_EPOCH};

use axum::extract::{
    ws::{Message, WebSocket, WebSocketUpgrade},
    Path as AxumPath, Query, State,
};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use futures_util::{SinkExt, StreamExt};
use portable_pty::{native_pty_system, Child, CommandBuilder, MasterPty, PtySize};
use rand::distributions::Alphanumeric;
use rand::{thread_rng, Rng};
use serde::{Deserialize, Serialize};
use tokio::sync::watch;
use ws_dashboard_core::WorkRootId;

use crate::router::AppState;
use crate::work_root_files::{resolve_online_available_work_root, WorkRootAccessError};

const MAX_TERMINAL_SESSIONS: usize = 16;
const MAX_OUTPUT_CHUNKS: usize = 1024;
const MAX_INPUT_BYTES: usize = 16 * 1024;
const MIN_COLUMNS: u16 = 1;
const MIN_ROWS: u16 = 1;
const MAX_COLUMNS: u16 = 300;
const MAX_ROWS: u16 = 120;
const DEFAULT_BROWSER_PTY_TERM: &str = "xterm-256color";

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

#[derive(Clone, Default)]
pub struct TerminalRegistry {
    sessions: Arc<RwLock<HashMap<String, Arc<TerminalSession>>>>,
}

impl TerminalRegistry {
    fn list_for_work_root(&self, work_root_id: &WorkRootId) -> Vec<TerminalSessionView> {
        self.sessions
            .read()
            .expect("terminal registry lock poisoned")
            .values()
            .filter(|session| &session.work_root_id == work_root_id && session.is_live())
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

    fn remove(&self, terminal_id: &str) -> Option<Arc<TerminalSession>> {
        self.sessions
            .write()
            .expect("terminal registry lock poisoned")
            .remove(terminal_id)
    }

    pub fn remove_for_work_roots(
        &self,
        work_root_ids: &std::collections::BTreeSet<WorkRootId>,
    ) -> usize {
        let mut sessions = self
            .sessions
            .write()
            .expect("terminal registry lock poisoned");
        let before = sessions.len();
        sessions.retain(|_, session| !work_root_ids.contains(&session.work_root_id));
        before - sessions.len()
    }
}

struct TerminalSession {
    id: String,
    work_root_id: WorkRootId,
    title: String,
    cwd_hint: Option<String>,
    created_at_ms: u64,
    inner: Mutex<TerminalSessionInner>,
    output_signal: watch::Sender<u64>,
}

struct TerminalSessionInner {
    status: TerminalStatus,
    columns: u16,
    rows: u16,
    output: VecDeque<TerminalOutputChunk>,
    next_sequence: u64,
    writer_tx: Option<mpsc::Sender<TerminalWriterCommand>>,
    master: Option<Box<dyn MasterPty + Send>>,
    child: Option<Box<dyn Child + Send + Sync>>,
}

// CONTRACT: PTY writes must never block a Tokio worker thread (see ticket
// 260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation). Each
// live terminal session owns exactly one dedicated blocking OS thread that
// serializes writes to the PTY master; `write_input` only ever performs a
// non-blocking `mpsc::Sender::send` handoff onto this thread, never a direct
// blocking `write_all`.
enum TerminalWriterCommand {
    Write(Vec<u8>),
}

// Detached, unjoined thread (mirrors `spawn_reader`'s style below): a stalled
// `write_all` on a full OS pipe buffer is unblocked by the *child process*
// dying (see `terminate`/`mark_error`/`mark_exited` ordering), not by
// anything this thread does on its own, so there is nothing useful to join.
fn spawn_writer_thread(writer: Box<dyn Write + Send>) -> mpsc::Sender<TerminalWriterCommand> {
    let (tx, rx) = mpsc::channel::<TerminalWriterCommand>();
    thread::spawn(move || run_writer_thread(rx, writer));
    tx
}

// Pulled out of `spawn_writer_thread` so tests can drive the loop on a
// `thread::Builder` handle they retain and `.join()` (safe in test code,
// since no session mutex is held across the join) to assert a failing write
// stops the loop cleanly without panicking, without giving production code
// a joinable handle to misuse.
fn run_writer_thread(rx: mpsc::Receiver<TerminalWriterCommand>, mut writer: Box<dyn Write + Send>) {
    while let Ok(TerminalWriterCommand::Write(data)) = rx.recv() {
        if writer.write_all(&data).and_then(|()| writer.flush()).is_err() {
            // A write failure correlates with process death; the reader
            // thread already observes that independently and reports it via
            // `mark_error`/`mark_exited`, so this thread just stops quietly.
            break;
        }
    }
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
        work_root_id,
        root_path,
        request.title.unwrap_or_else(|| "Terminal".to_owned()),
        columns,
        rows,
        request.cwd_hint,
    ) {
        Ok(session) => {
            let view = session.view();
            match state.terminals.insert(session.clone()) {
                Ok(()) => Json(view).into_response(),
                Err(error) => {
                    session.terminate();
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
    match session.write_input(request.data.as_bytes()) {
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
    // `resize()`'s body stays synchronous (it must return the
    // `TerminalSessionView` for the JSON response), but the blocking
    // `master.resize()` syscall itself must not run on a Tokio worker
    // thread - offload the whole call via `spawn_blocking`, mirroring the
    // existing idiom in `git_toolbar.rs`/`root_picker.rs`/`resources.rs`.
    match tokio::task::spawn_blocking(move || session.resize(columns, rows)).await {
        Ok(Ok(view)) => Json(view).into_response(),
        Ok(Err(error)) => error.into_response(),
        Err(_) => terminal_error(StatusCode::INTERNAL_SERVER_ERROR, "terminal resize failed"),
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
    // TerminalRegistry::get confirms a live session; terminal_socket_task owns
    // output backfill, resize/input frames, and close propagation.
    let Some(session) = state.terminals.get(&terminal_id) else {
        return terminal_error(StatusCode::NOT_FOUND, "unknown terminal");
    };
    if let Err(error) = resolve_online_available_work_root(&state, &session.work_root_id) {
        return terminal_access_error(error);
    }
    if !session.is_live() {
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
    session.terminate();
    StatusCode::NO_CONTENT.into_response()
}

impl TerminalSession {
    fn spawn(
        work_root_id: WorkRootId,
        root_path: PathBuf,
        title: String,
        columns: u16,
        rows: u16,
        cwd_hint: Option<String>,
    ) -> Result<Arc<Self>, TerminalError> {
        let (spawn_cwd, normalized_cwd_hint) = resolve_terminal_cwd(&root_path, cwd_hint)?;
        let pty_system = native_pty_system();
        let pair = pty_system
            .openpty(PtySize {
                rows,
                cols: columns,
                pixel_width: 0,
                pixel_height: 0,
            })
            .map_err(|_| TerminalError::BadRequest("terminal spawn failed"))?;
        let mut command = CommandBuilder::new(default_shell());
        command.cwd(spawn_cwd);
        command.env(
            "TERM",
            browser_pty_term(|key| {
                std::env::var_os(key).map(|value| value.to_string_lossy().into_owned())
            }),
        );
        let child = pair
            .slave
            .spawn_command(command)
            .map_err(|_| TerminalError::BadRequest("terminal spawn failed"))?;
        drop(pair.slave);
        let reader = pair
            .master
            .try_clone_reader()
            .map_err(|_| TerminalError::BadRequest("terminal spawn failed"))?;
        let writer = pair
            .master
            .take_writer()
            .map_err(|_| TerminalError::BadRequest("terminal spawn failed"))?;
        let session = Arc::new(Self {
            id: opaque_terminal_id(),
            work_root_id,
            title,
            cwd_hint: normalized_cwd_hint,
            created_at_ms: now_ms(),
            inner: Mutex::new(TerminalSessionInner {
                status: TerminalStatus::Running,
                columns,
                rows,
                output: VecDeque::new(),
                next_sequence: 1,
                writer_tx: Some(spawn_writer_thread(writer)),
                master: Some(pair.master),
                child: Some(child),
            }),
            output_signal: watch::channel(0).0,
        });
        spawn_reader(session.clone(), reader);
        Ok(session)
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

    // CONTRACT: this replaces a `filter(|c| c.sequence > after)` scan with
    // direct index arithmetic. It is only valid because `append_output`
    // (see below) maintains a gapless, strictly-contiguous `sequence`
    // numbering (each push consumes exactly one `next_sequence` value) and
    // only ever evicts from the front (`pop_front`, never mid-deque
    // removal). If either invariant changes, this shortcut must be
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

    fn write_input(&self, input: &[u8]) -> Result<(), TerminalError> {
        if input.len() > MAX_INPUT_BYTES {
            return Err(TerminalError::BadRequest("terminal input too large"));
        }
        // Fast path stays synchronous and cheap: an already-closed terminal
        // must still return `Gone` immediately, without touching the writer
        // channel. Only the actual PTY write is handed off.
        let inner = self.inner.lock().expect("terminal session lock poisoned");
        if inner.status != TerminalStatus::Running {
            return Err(TerminalError::Gone("terminal is closed"));
        }
        let Some(writer_tx) = inner.writer_tx.as_ref() else {
            return Err(TerminalError::Gone("terminal is closed"));
        };
        // Non-blocking handoff to the dedicated writer thread (see
        // `spawn_writer_thread`). `send` on an unbounded `mpsc` channel never
        // blocks the caller. A send error means the writer thread already
        // exited (e.g. after a prior write failure) - best-effort, not a
        // synchronous error; the reader thread's `mark_error`/`mark_exited`
        // is the authoritative signal for that case, surfaced asynchronously
        // via `output_signal`/`is_live`.
        let _ = writer_tx.send(TerminalWriterCommand::Write(input.to_vec()));
        Ok(())
    }

    fn resize(&self, columns: u16, rows: u16) -> Result<TerminalSessionView, TerminalError> {
        let mut inner = self.inner.lock().expect("terminal session lock poisoned");
        if inner.status != TerminalStatus::Running {
            return Err(TerminalError::Gone("terminal is closed"));
        }
        let Some(master) = inner.master.as_mut() else {
            return Err(TerminalError::Gone("terminal is closed"));
        };
        master
            .resize(PtySize {
                rows,
                cols: columns,
                pixel_width: 0,
                pixel_height: 0,
            })
            .map_err(|_| TerminalError::BadRequest("terminal resize failed"))?;
        inner.columns = columns;
        inner.rows = rows;
        drop(inner);
        Ok(self.view())
    }

    fn terminate(&self) {
        let next_sequence = {
            let mut inner = self.inner.lock().expect("terminal session lock poisoned");
            inner.status = TerminalStatus::Terminated;
            // Kill/wait the child (and drop the master) BEFORE dropping the
            // writer channel: this is what actually unblocks a `write_all`
            // stuck on a full OS pipe buffer inside the writer thread.
            // Dropping the channel sender first does nothing for an
            // already-blocked syscall - see shutdown-ordering constraint.
            if let Some(mut child) = inner.child.take() {
                let _ = child.kill();
                let _ = child.wait();
            }
            inner.master = None;
            inner.writer_tx = None;
            inner.next_sequence
        };
        let _ = self.output_signal.send(next_sequence);
    }

    fn append_output(&self, data: String) {
        let next_sequence = {
            let mut inner = self.inner.lock().expect("terminal session lock poisoned");
            if data.is_empty() {
                return;
            }
            let sequence = inner.next_sequence;
            inner.next_sequence += 1;
            inner.output.push_back(TerminalOutputChunk {
                sequence,
                data,
                stream: "pty".to_owned(),
            });
            while inner.output.len() > MAX_OUTPUT_CHUNKS {
                inner.output.pop_front();
            }
            inner.next_sequence
        };
        let _ = self.output_signal.send(next_sequence);
    }

    fn mark_error(&self) {
        let next_sequence = {
            let mut inner = self.inner.lock().expect("terminal session lock poisoned");
            if inner.status == TerminalStatus::Running {
                inner.status = TerminalStatus::Error;
                // Same shutdown ordering as `terminate`: unblock a stalled
                // writer-thread write before dropping the channel/master.
                if let Some(mut child) = inner.child.take() {
                    let _ = child.kill();
                }
                inner.master = None;
                inner.writer_tx = None;
            }
            inner.next_sequence
        };
        let _ = self.output_signal.send(next_sequence);
    }

    fn mark_exited(&self) {
        let next_sequence = {
            let mut inner = self.inner.lock().expect("terminal session lock poisoned");
            if inner.status == TerminalStatus::Running {
                inner.status = TerminalStatus::Exited;
                // Same shutdown ordering as `terminate`: unblock a stalled
                // writer-thread write before dropping the channel/master.
                if let Some(mut child) = inner.child.take() {
                    let _ = child.wait();
                }
                inner.master = None;
                inner.writer_tx = None;
            }
            inner.next_sequence
        };
        let _ = self.output_signal.send(next_sequence);
    }
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
                        if session.write_input(&bytes).is_err() {
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
                if !session.is_live() {
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
        TerminalWebSocketClientMessage::Input { data } => session.write_input(data.as_bytes()),
        TerminalWebSocketClientMessage::Resize { columns, rows } => {
            let (columns, rows) = validate_size(columns, rows)
                .map_err(|_| TerminalError::BadRequest("invalid terminal size"))?;
            // Same rationale as the HTTP `terminal_resize` handler: offload
            // the blocking `master.resize()` call so it never runs on this
            // Tokio worker.
            tokio::task::spawn_blocking(move || session.resize(columns, rows))
                .await
                .map_err(|_| TerminalError::BadRequest("terminal resize failed"))?
                .map(|_| ())
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

fn spawn_reader(session: Arc<TerminalSession>, mut reader: Box<dyn Read + Send>) {
    thread::spawn(move || {
        let mut buffer = [0_u8; 4096];
        loop {
            match reader.read(&mut buffer) {
                Ok(0) => break,
                Ok(n) => session.append_output(String::from_utf8_lossy(&buffer[..n]).into_owned()),
                Err(_) => {
                    session.mark_error();
                    return;
                }
            }
        }
        session.mark_exited();
    });
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

fn default_shell() -> PathBuf {
    #[cfg(windows)]
    {
        select_terminal_shell(TerminalPlatform::Windows, |key| std::env::var_os(key)).program
    }
    #[cfg(not(windows))]
    {
        select_terminal_shell(TerminalPlatform::Unix, |key| std::env::var_os(key)).program
    }
}

fn browser_pty_term(env: impl Fn(&str) -> Option<String>) -> String {
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

    // Builds a TerminalSession without spawning a real PTY, so the ring
    // buffer eviction / truncation-detection contract can be exercised
    // deterministically and fast, independent of the environment's PTY
    // availability (see Phase 4 plan: no PTY-based e2e coverage needed for
    // this backend-only cursor/eviction logic).
    fn fake_terminal_session() -> TerminalSession {
        TerminalSession {
            id: opaque_terminal_id(),
            work_root_id: WorkRootId::from("fake-work-root".to_owned()),
            title: "fake".to_owned(),
            cwd_hint: None,
            created_at_ms: now_ms(),
            inner: Mutex::new(TerminalSessionInner {
                status: TerminalStatus::Running,
                columns: default_columns(),
                rows: default_rows(),
                output: VecDeque::new(),
                next_sequence: 1,
                writer_tx: None,
                master: None,
                child: None,
            }),
            output_signal: watch::channel(0).0,
        }
    }

    #[test]
    fn is_range_truncated_never_fires_on_fresh_after_zero_attach() {
        let session = fake_terminal_session();
        for _ in 0..(MAX_OUTPUT_CHUNKS + 200) {
            session.append_output("x".to_owned());
        }
        // A fresh pane always requests after=0 ("send me everything you
        // have"), even against a terminal that has already evicted far more
        // than MAX_OUTPUT_CHUNKS chunks - that must never be reported as a
        // gap, since the client never observed the evicted data in the first
        // place.
        assert!(!session.is_range_truncated(0));
    }

    #[test]
    fn is_range_truncated_fires_only_for_a_genuine_resume_past_eviction() {
        let session = fake_terminal_session();
        for _ in 0..(MAX_OUTPUT_CHUNKS + 200) {
            session.append_output("x".to_owned());
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
    #[test]
    fn plan_output_backfill_computes_truncation_from_requested_cursor_not_advanced_cursor() {
        let session = fake_terminal_session();
        for _ in 0..(MAX_OUTPUT_CHUNKS + 200) {
            session.append_output("x".to_owned());
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
    #[test]
    fn output_after_index_arithmetic_matches_old_filter_semantics_across_eviction() {
        let session = fake_terminal_session();
        for _ in 0..(MAX_OUTPUT_CHUNKS + 200) {
            session.append_output("x".to_owned());
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

    // Test-only `Write` impl that forwards each write's bytes over a plain
    // `mpsc` channel the test polls with `recv_timeout`, so the assertion
    // exercises the real writer thread's ordering guarantee without a real
    // PTY and without a flaky sleep-based race.
    struct RecordingWriter {
        tx: mpsc::Sender<Vec<u8>>,
    }

    impl Write for RecordingWriter {
        fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
            let _ = self.tx.send(buf.to_vec());
            Ok(buf.len())
        }

        fn flush(&mut self) -> std::io::Result<()> {
            Ok(())
        }
    }

    // Test-only `Write` impl that always fails, used to prove the writer
    // thread's loop exits cleanly (no panic, no unwrap on the write error)
    // instead of looping forever or crashing the thread abnormally.
    struct FailingWriter;

    impl Write for FailingWriter {
        fn write(&mut self, _buf: &[u8]) -> std::io::Result<usize> {
            Err(std::io::Error::new(std::io::ErrorKind::BrokenPipe, "boom"))
        }

        fn flush(&mut self) -> std::io::Result<()> {
            Ok(())
        }
    }

    #[test]
    fn write_input_delivers_chunks_in_order_over_the_writer_thread() {
        let (record_tx, record_rx) = mpsc::channel::<Vec<u8>>();
        let writer_tx = spawn_writer_thread(Box::new(RecordingWriter { tx: record_tx }));
        let session = fake_terminal_session();
        {
            let mut inner = session.inner.lock().expect("terminal session lock poisoned");
            inner.writer_tx = Some(writer_tx);
        }

        session.write_input(b"first").expect("write first");
        session.write_input(b"second").expect("write second");
        session.write_input(b"third").expect("write third");

        let timeout = std::time::Duration::from_secs(2);
        assert_eq!(
            record_rx.recv_timeout(timeout).expect("first chunk delivered"),
            b"first"
        );
        assert_eq!(
            record_rx.recv_timeout(timeout).expect("second chunk delivered"),
            b"second"
        );
        assert_eq!(
            record_rx.recv_timeout(timeout).expect("third chunk delivered"),
            b"third"
        );
    }

    #[test]
    fn writer_thread_loop_stops_without_panicking_on_write_error() {
        let (tx, rx) = mpsc::channel::<TerminalWriterCommand>();
        // Unlike `spawn_writer_thread` (detached, never joined in
        // production - see shutdown-ordering constraint), the test retains
        // a `JoinHandle` via `run_writer_thread` directly so it can assert
        // the loop returns normally (no panic) instead of only inferring it
        // from a timing-sensitive absence of a crash.
        let handle = thread::spawn(move || run_writer_thread(rx, Box::new(FailingWriter)));

        tx.send(TerminalWriterCommand::Write(b"doomed".to_vec()))
            .expect("channel receiver still alive before the failing write");
        drop(tx);

        handle
            .join()
            .expect("writer thread must not panic when the underlying write fails");
    }

    #[test]
    fn write_input_returns_gone_synchronously_after_terminate_without_touching_channel() {
        let session = fake_terminal_session();
        let (record_tx, record_rx) = mpsc::channel::<Vec<u8>>();
        let writer_tx = spawn_writer_thread(Box::new(RecordingWriter { tx: record_tx }));
        {
            let mut inner = session.inner.lock().expect("terminal session lock poisoned");
            inner.writer_tx = Some(writer_tx);
        }

        session.terminate();

        assert!(matches!(
            session.write_input(b"too-late"),
            Err(TerminalError::Gone("terminal is closed"))
        ));
        // The synchronous `status != Running` fast-path must short-circuit
        // before ever reaching the channel: nothing should have been
        // forwarded to the (now-dropped) writer thread.
        assert!(record_rx.try_recv().is_err());
    }

    #[test]
    fn write_input_returns_gone_synchronously_after_mark_error_without_touching_channel() {
        let session = fake_terminal_session();
        let (record_tx, record_rx) = mpsc::channel::<Vec<u8>>();
        let writer_tx = spawn_writer_thread(Box::new(RecordingWriter { tx: record_tx }));
        {
            let mut inner = session.inner.lock().expect("terminal session lock poisoned");
            inner.writer_tx = Some(writer_tx);
        }

        session.mark_error();

        assert!(matches!(
            session.write_input(b"too-late"),
            Err(TerminalError::Gone("terminal is closed"))
        ));
        assert!(record_rx.try_recv().is_err());
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
