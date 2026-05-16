use std::collections::{HashMap, VecDeque};
use std::io::{Read, Write};
use std::path::PathBuf;
use std::sync::{Arc, Mutex, RwLock};
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

const MAX_TERMINAL_SESSIONS: usize = 16;
const MAX_OUTPUT_CHUNKS: usize = 1024;
const MAX_INPUT_BYTES: usize = 16 * 1024;
const MIN_COLUMNS: u16 = 1;
const MIN_ROWS: u16 = 1;
const MAX_COLUMNS: u16 = 300;
const MAX_ROWS: u16 = 120;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum TerminalPlatform {
    Unix,
    Windows,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum TerminalShellSource {
    ShellEnv,
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
    // CONTRACT: Shell selection must be explicit and testable for Unix and
    // Windows without relying on compile-time cfg branches inside tests.
    // HINT: Unix uses SHELL then /bin/sh; Windows uses COMSPEC then cmd.exe.
    let (key, source, fallback) = match platform {
        TerminalPlatform::Unix => ("SHELL", TerminalShellSource::ShellEnv, "/bin/sh"),
        TerminalPlatform::Windows => ("COMSPEC", TerminalShellSource::ComspecEnv, "cmd.exe"),
    };

    if let Some(program) = env(key).filter(|value| !value.is_empty()) {
        return TerminalShellSelection {
            platform,
            program: PathBuf::from(program),
            source,
        };
    }

    TerminalShellSelection {
        platform,
        program: PathBuf::from(fallback),
        source: TerminalShellSource::Fallback,
    }
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
}

struct TerminalSession {
    id: String,
    work_root_id: WorkRootId,
    title: String,
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
    writer: Option<Box<dyn Write + Send>>,
    master: Option<Box<dyn MasterPty + Send>>,
    child: Option<Box<dyn Child + Send + Sync>>,
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
    },
    Exit {
        #[serde(rename = "terminalId")]
        terminal_id: String,
        status: TerminalStatus,
        #[serde(rename = "nextSequence")]
        next_sequence: u64,
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
    after: u64,
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
    let Some(root_path) = state.opened_work_roots.resolve(&work_root_id) else {
        return terminal_error(StatusCode::NOT_FOUND, "unknown workRoot");
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
    if state.opened_work_roots.resolve(&work_root_id).is_none() {
        return terminal_error(StatusCode::NOT_FOUND, "unknown workRoot");
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
    let Ok((columns, rows)) = validate_size(request.columns, request.rows) else {
        return terminal_error(StatusCode::BAD_REQUEST, "invalid terminal size");
    };
    match session.resize(columns, rows) {
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
    // TerminalRegistry::get confirms a live session; terminal_socket_task owns
    // output backfill, resize/input frames, and close propagation.
    let Some(session) = state.terminals.get(&terminal_id) else {
        return terminal_error(StatusCode::NOT_FOUND, "unknown terminal");
    };
    if !session.is_live() {
        return terminal_error(StatusCode::GONE, "terminal is closed");
    }
    upgrade
        .on_upgrade(move |socket| terminal_socket_task(session, socket, query.after))
        .into_response()
}

pub async fn close_terminal(
    State(state): State<AppState>,
    AxumPath(terminal_id): AxumPath<String>,
) -> Response {
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
    ) -> Result<Arc<Self>, TerminalError> {
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
        command.cwd(root_path);
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
            created_at_ms: now_ms(),
            inner: Mutex::new(TerminalSessionInner {
                status: TerminalStatus::Running,
                columns,
                rows,
                output: VecDeque::new(),
                next_sequence: 1,
                writer: Some(writer),
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

    fn output_after(&self, after: u64) -> TerminalOutputView {
        let inner = self.inner.lock().expect("terminal session lock poisoned");
        TerminalOutputView {
            terminal_id: self.id.clone(),
            status: inner.status,
            next_sequence: inner.next_sequence,
            chunks: inner
                .output
                .iter()
                .filter(|chunk| chunk.sequence > after)
                .cloned()
                .collect(),
        }
    }

    fn write_input(&self, input: &[u8]) -> Result<(), TerminalError> {
        if input.len() > MAX_INPUT_BYTES {
            return Err(TerminalError::BadRequest("terminal input too large"));
        }
        let mut inner = self.inner.lock().expect("terminal session lock poisoned");
        if inner.status != TerminalStatus::Running {
            return Err(TerminalError::Gone("terminal is closed"));
        }
        let Some(writer) = inner.writer.as_mut() else {
            return Err(TerminalError::Gone("terminal is closed"));
        };
        writer
            .write_all(input)
            .and_then(|()| writer.flush())
            .map_err(|_| TerminalError::Gone("terminal is closed"))
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
            inner.writer = None;
            inner.master = None;
            if let Some(mut child) = inner.child.take() {
                let _ = child.kill();
                let _ = child.wait();
            }
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
                inner.writer = None;
                inner.master = None;
                if let Some(mut child) = inner.child.take() {
                    let _ = child.kill();
                }
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
                inner.writer = None;
                inner.master = None;
                if let Some(mut child) = inner.child.take() {
                    let _ = child.wait();
                }
            }
            inner.next_sequence
        };
        let _ = self.output_signal.send(next_sequence);
    }
}

async fn terminal_socket_task(session: Arc<TerminalSession>, socket: WebSocket, after: u64) {
    let (mut sender, mut receiver) = socket.split();
    let mut output_signal = session.output_signal.subscribe();
    let mut cursor = after;

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
                        let Ok(message) = serde_json::from_str::<TerminalWebSocketClientMessage>(&text) else {
                            break;
                        };
                        if handle_terminal_socket_client_message(&session, message).is_err() {
                            let _ = send_terminal_socket_status(&session, &mut sender, false).await;
                            break;
                        }
                    }
                    Message::Binary(bytes) => {
                        if session.write_input(&bytes).is_err() {
                            let _ = send_terminal_socket_status(&session, &mut sender, false).await;
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

fn handle_terminal_socket_client_message(
    session: &TerminalSession,
    message: TerminalWebSocketClientMessage,
) -> Result<(), TerminalError> {
    match message {
        TerminalWebSocketClientMessage::Input { data } => session.write_input(data.as_bytes()),
        TerminalWebSocketClientMessage::Resize { columns, rows } => {
            let (columns, rows) = validate_size(columns, rows)
                .map_err(|_| TerminalError::BadRequest("invalid terminal size"))?;
            session.resize(columns, rows).map(|_| ())
        }
    }
}

async fn send_output_backfill(
    session: &TerminalSession,
    sender: &mut futures_util::stream::SplitSink<WebSocket, Message>,
    cursor: &mut u64,
) -> Result<(), ()> {
    let output = session.output_after(*cursor);
    for chunk in output.chunks {
        *cursor = (*cursor).max(chunk.sequence);
        send_socket_json(
            sender,
            &TerminalWebSocketServerMessage::Output {
                terminal_id: session.id.clone(),
                chunk,
            },
        )
        .await?;
    }
    send_terminal_socket_status(session, sender, !session.is_live()).await
}

async fn send_terminal_socket_status(
    session: &TerminalSession,
    sender: &mut futures_util::stream::SplitSink<WebSocket, Message>,
    exit: bool,
) -> Result<(), ()> {
    let output = session.output_after(u64::MAX);
    let message = if exit {
        TerminalWebSocketServerMessage::Exit {
            terminal_id: session.id.clone(),
            status: output.status,
            next_sequence: output.next_sequence,
        }
    } else {
        TerminalWebSocketServerMessage::Status {
            terminal_id: session.id.clone(),
            status: output.status,
            next_sequence: output.next_sequence,
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

fn terminal_error(status: StatusCode, error: impl Into<String>) -> Response {
    (
        status,
        Json(TerminalErrorView {
            error: error.into(),
        }),
    )
        .into_response()
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

#[cfg(test)]
mod terminal_portability_skeleton_tests {
    use super::*;

    #[test]
    fn terminal_shell_selection_contract_targets() {
        // CONTRACT: Fill executable assertions for SHELL, COMSPEC, Unix
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
            select_terminal_shell(TerminalPlatform::Windows, windows_env),
            TerminalShellSelection {
                platform: TerminalPlatform::Windows,
                program: PathBuf::from(r"C:\Windows\System32\cmd.exe"),
                source: TerminalShellSource::ComspecEnv,
            }
        );

        assert_eq!(
            select_terminal_shell(TerminalPlatform::Unix, |_| None).program,
            PathBuf::from("/bin/sh")
        );
        assert_eq!(
            select_terminal_shell(TerminalPlatform::Windows, |_| None).program,
            PathBuf::from("cmd.exe")
        );
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
