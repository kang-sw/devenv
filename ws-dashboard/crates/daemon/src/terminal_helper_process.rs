// The detached per-terminal helper's own runtime. Invoked via the hidden
// `ws-dashboard terminal-helper ...` re-exec target (see `cli.rs`); by the
// time `run_terminal_helper` starts, the daemon-side spawn path
// (`terminal_platform::spawn_detached`) has already run `setsid()` + a
// double fork, so this process is already a session leader detached from
// the daemon's controlling terminal and process tree.
//
// Ownership split (ticket-pinned): this process owns the PTY master/child,
// the reader/writer threads, and the bounded output ring - it is the
// AUTHORITATIVE source of terminal state, not the daemon. The daemon is a
// thin proxy that mirrors what this process pushes over IPC.

use std::collections::VecDeque;
use std::io::{Read, Write};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{mpsc as std_mpsc, Arc, Mutex};
use std::thread;
use std::time::{Duration, Instant};

use portable_pty::{native_pty_system, Child, CommandBuilder, MasterPty, PtySize};
use tokio::sync::Notify;

use crate::cli::TerminalHelperArgs;
use crate::terminal_helper_ipc::{write_ndjson, NdjsonReader};
use crate::terminal_helper_protocol::{
    DaemonToHelperMessage, HelperToDaemonMessage, TerminalHelperOutputChunk, TerminalHelperStatus,
};
use crate::terminal_ipc_transport::{BoxedIpcStream, IpcListener};
use crate::terminal_registry_file::{delete_registry_entry, write_registry_entry, TerminalRegistryEntry};

const MAX_OUTPUT_CHUNKS: usize = 1024;
const GRACE_WINDOW: Duration = Duration::from_secs(30);
const IDLE_ACCEPT_POLL: Duration = Duration::from_secs(2);

struct RingState {
    output: VecDeque<TerminalHelperOutputChunk>,
    next_sequence: u64,
    status: TerminalHelperStatus,
}

impl RingState {
    fn new() -> Self {
        Self {
            output: VecDeque::new(),
            next_sequence: 1,
            status: TerminalHelperStatus::Running,
        }
    }

    fn append(&mut self, data: String) {
        if data.is_empty() {
            return;
        }
        let sequence = self.next_sequence;
        self.next_sequence += 1;
        self.output.push_back(TerminalHelperOutputChunk { sequence, data });
        while self.output.len() > MAX_OUTPUT_CHUNKS {
            self.output.pop_front();
        }
    }

    fn backfill_after(&self, after: u64) -> Vec<TerminalHelperOutputChunk> {
        self.output
            .iter()
            .filter(|chunk| chunk.sequence > after)
            .cloned()
            .collect()
    }
}

enum WriterCommand {
    Write(Vec<u8>),
}

struct SharedState {
    pid: u32,
    start_time: u64,
    ring: Mutex<RingState>,
    notify: Notify,
    child: Mutex<Option<Box<dyn Child + Send + Sync>>>,
    master: Mutex<Option<Box<dyn MasterPty + Send>>>,
    writer_tx: Mutex<Option<std_mpsc::Sender<WriterCommand>>>,
    shell_started: AtomicBool,
    exited_at: Mutex<Option<Instant>>,
}

impl SharedState {
    fn write_input(&self, data: &[u8]) {
        if let Some(tx) = self.writer_tx.lock().expect("writer_tx lock poisoned").as_ref() {
            let _ = tx.send(WriterCommand::Write(data.to_vec()));
        }
    }

    fn resize(&self, columns: u16, rows: u16) {
        if let Some(master) = self.master.lock().expect("master lock poisoned").as_mut() {
            let _ = master.resize(PtySize {
                rows,
                cols: columns,
                pixel_width: 0,
                pixel_height: 0,
            });
        }
    }

    fn kill_shell_if_running(&self) {
        if let Some(mut child) = self.child.lock().expect("child lock poisoned").take() {
            let _ = child.kill();
            let _ = child.wait();
        }
        self.master.lock().expect("master lock poisoned").take();
        self.writer_tx.lock().expect("writer_tx lock poisoned").take();
    }

    fn append_output(&self, data: String) {
        {
            let mut ring = self.ring.lock().expect("ring lock poisoned");
            ring.append(data);
        }
        self.notify.notify_one();
    }

    fn transition(&self, status: TerminalHelperStatus) {
        let mut ring = self.ring.lock().expect("ring lock poisoned");
        if ring.status == TerminalHelperStatus::Running {
            ring.status = status;
            drop(ring);
            *self.exited_at.lock().expect("exited_at lock poisoned") = Some(Instant::now());
            self.master.lock().expect("master lock poisoned").take();
            self.writer_tx.lock().expect("writer_tx lock poisoned").take();
        }
        self.notify.notify_one();
    }

    fn exited_at(&self) -> Option<Instant> {
        *self.exited_at.lock().expect("exited_at lock poisoned")
    }

    fn status_and_next_sequence(&self) -> (TerminalHelperStatus, u64) {
        let ring = self.ring.lock().expect("ring lock poisoned");
        (ring.status, ring.next_sequence)
    }
}

pub async fn run_terminal_helper(args: TerminalHelperArgs) -> anyhow::Result<()> {
    let pid = std::process::id();
    let start_time = crate::terminal_platform::process_start_time(pid).unwrap_or(0);

    let entry = TerminalRegistryEntry {
        terminal_id: args.terminal_id.clone(),
        work_root_id: args.work_root_id.clone(),
        pid,
        start_time,
        socket_path: args.socket_path.clone(),
        created_at_ms: now_ms(),
        title: args.title.clone(),
        cwd_hint: args.cwd_hint.clone(),
        columns: args.columns,
        rows: args.rows,
    };
    // CONTRACT (ticket "Registry-write ordering"): the entry is durably
    // written BEFORE the IPC listener even binds, and the shell is spawned
    // only after a daemon has connected AND handshaked (see
    // `handle_connection`) - this closes the orphan-leak window where a
    // helper could be running a live shell nothing durably tracks yet.
    write_registry_entry(&args.registry_dir, &entry)?;

    let _ = std::fs::remove_file(&args.socket_path);
    let mut listener = IpcListener::bind(&args.socket_path)?;

    let shared = Arc::new(SharedState {
        pid,
        start_time,
        ring: Mutex::new(RingState::new()),
        notify: Notify::new(),
        child: Mutex::new(None),
        master: Mutex::new(None),
        writer_tx: Mutex::new(None),
        shell_started: AtomicBool::new(false),
        exited_at: Mutex::new(None),
    });

    let result = serve_connections(&args, &mut listener, &shared).await;

    shared.kill_shell_if_running();
    delete_registry_entry(&args.registry_dir, &args.terminal_id);
    let _ = std::fs::remove_file(&args.socket_path);
    result
}

async fn serve_connections(
    args: &TerminalHelperArgs,
    listener: &mut IpcListener,
    shared: &Arc<SharedState>,
) -> anyhow::Result<()> {
    loop {
        let wait = match shared.exited_at() {
            Some(exited_at) => match GRACE_WINDOW.checked_sub(exited_at.elapsed()) {
                Some(remaining) => remaining,
                None => break,
            },
            None => IDLE_ACCEPT_POLL,
        };
        match tokio::time::timeout(wait, listener.accept()).await {
            Ok(Ok(stream)) => {
                let keep_serving = handle_connection(args, stream, shared).await?;
                if !keep_serving {
                    break;
                }
                // One reattach after the shell has exited is the grace
                // window's whole purpose - deliver the exit + trailing
                // output once, then self-exit rather than lingering for the
                // rest of the 30s.
                if shared.exited_at().is_some() {
                    break;
                }
            }
            Ok(Err(error)) => return Err(error.into()),
            Err(_elapsed) => continue,
        }
    }
    Ok(())
}

/// Serves a single accepted connection until it closes. Returns `Ok(true)`
/// to keep accepting further connections, `Ok(false)` when the daemon
/// requested a full graceful shutdown (helper should stop entirely).
async fn handle_connection(
    args: &TerminalHelperArgs,
    stream: BoxedIpcStream,
    shared: &Arc<SharedState>,
) -> anyhow::Result<bool> {
    let (read_half, write_half) = crate::terminal_ipc_transport::split(stream);
    let mut reader = NdjsonReader::new(read_half);
    let write_half = Arc::new(tokio::sync::Mutex::new(write_half));

    write_ndjson(
        &mut *write_half.lock().await,
        &HelperToDaemonMessage::Handshake {
            pid: shared.pid,
            start_time: shared.start_time,
        },
    )
    .await?;

    let (status, next_sequence) = shared.status_and_next_sequence();
    let mut last_sent_status = status;
    let initial_message = if status == TerminalHelperStatus::Running {
        HelperToDaemonMessage::Status { status, next_sequence }
    } else {
        HelperToDaemonMessage::Exit { status, next_sequence }
    };
    write_ndjson(&mut *write_half.lock().await, &initial_message).await?;

    // A fresh connection has nothing "already sent" from this process's own
    // point of view; re-pushing the full retained ring here doubles as the
    // grace-reattach/boot-reconcile-adopt backfill mechanism without a
    // dedicated request/response round trip for the common case.
    let mut last_sent_sequence: u64 = 0;

    loop {
        tokio::select! {
            incoming = reader.read_message::<DaemonToHelperMessage>() => {
                match incoming? {
                    None => return Ok(true), // daemon disconnected; outer loop decides what's next
                    Some(DaemonToHelperMessage::HandshakeAck) => {
                        if shared
                            .shell_started
                            .compare_exchange(false, true, Ordering::SeqCst, Ordering::SeqCst)
                            .is_ok()
                        {
                            if let Err(error) = spawn_shell(args, shared.clone()) {
                                tracing::error!(%error, "terminal helper failed to spawn shell");
                                shared.transition(TerminalHelperStatus::Error);
                            }
                        }
                    }
                    Some(DaemonToHelperMessage::Input { data }) => shared.write_input(data.as_bytes()),
                    Some(DaemonToHelperMessage::Resize { columns, rows }) => {
                        // The PTY resize ioctl is a syscall; keep it off
                        // this task's async runtime thread the same way the
                        // pre-decouple daemon offloaded it (see
                        // `terminal.rs`'s old `terminal_resize` handler).
                        let shared_for_resize = shared.clone();
                        let _ = tokio::task::spawn_blocking(move || {
                            shared_for_resize.resize(columns, rows)
                        })
                        .await;
                    }
                    Some(DaemonToHelperMessage::RequestBackfill { request_id, after }) => {
                        let (chunks, next_sequence, status) = {
                            let ring = shared.ring.lock().expect("ring lock poisoned");
                            (ring.backfill_after(after), ring.next_sequence, ring.status)
                        };
                        write_ndjson(
                            &mut *write_half.lock().await,
                            &HelperToDaemonMessage::BackfillResponse {
                                request_id,
                                chunks,
                                next_sequence,
                                status,
                            },
                        )
                        .await?;
                    }
                    Some(DaemonToHelperMessage::GracefulShutdown) => {
                        shared.kill_shell_if_running();
                        return Ok(false);
                    }
                }
            }
            () = shared.notify.notified() => {
                let (pending, status_now, next_sequence) = {
                    let ring = shared.ring.lock().expect("ring lock poisoned");
                    let pending: Vec<_> = ring
                        .output
                        .iter()
                        .filter(|chunk| chunk.sequence > last_sent_sequence)
                        .cloned()
                        .collect();
                    (pending, ring.status, ring.next_sequence)
                };
                for chunk in pending {
                    last_sent_sequence = last_sent_sequence.max(chunk.sequence);
                    write_ndjson(&mut *write_half.lock().await, &HelperToDaemonMessage::Output(chunk)).await?;
                }
                if status_now != last_sent_status {
                    last_sent_status = status_now;
                    let message = if status_now == TerminalHelperStatus::Running {
                        HelperToDaemonMessage::Status { status: status_now, next_sequence }
                    } else {
                        HelperToDaemonMessage::Exit { status: status_now, next_sequence }
                    };
                    write_ndjson(&mut *write_half.lock().await, &message).await?;
                }
            }
        }
    }
}

fn spawn_shell(args: &TerminalHelperArgs, shared: Arc<SharedState>) -> anyhow::Result<()> {
    let pty_system = native_pty_system();
    let pair = pty_system.openpty(PtySize {
        rows: args.rows,
        cols: args.columns,
        pixel_width: 0,
        pixel_height: 0,
    })?;
    let mut command = CommandBuilder::new(crate::terminal::default_shell());
    command.cwd(&args.cwd);
    command.env(
        "TERM",
        crate::terminal::browser_pty_term(|key| {
            std::env::var_os(key).map(|value| value.to_string_lossy().into_owned())
        }),
    );
    let child = pair.slave.spawn_command(command)?;
    drop(pair.slave);
    let reader = pair.master.try_clone_reader()?;
    let writer = pair.master.take_writer()?;

    *shared.child.lock().expect("child lock poisoned") = Some(child);
    *shared.master.lock().expect("master lock poisoned") = Some(pair.master);
    *shared.writer_tx.lock().expect("writer_tx lock poisoned") = Some(spawn_writer_thread(writer));
    spawn_reader_thread(shared, reader);
    Ok(())
}

// Mirrors `terminal.rs`'s `spawn_writer_thread`/`run_writer_thread` shutdown-
// ordering contract (ticket 260723-bug-dashboard-terminal-blocking-pty-write
// -thread-starvation): writes never block an async task, and a stalled
// `write_all` is unblocked by the child dying (`kill_shell_if_running`),
// never by this thread on its own.
fn spawn_writer_thread(writer: Box<dyn Write + Send>) -> std_mpsc::Sender<WriterCommand> {
    let (tx, rx) = std_mpsc::channel::<WriterCommand>();
    thread::spawn(move || run_writer_thread(rx, writer));
    tx
}

fn run_writer_thread(rx: std_mpsc::Receiver<WriterCommand>, mut writer: Box<dyn Write + Send>) {
    while let Ok(WriterCommand::Write(data)) = rx.recv() {
        if writer.write_all(&data).and_then(|()| writer.flush()).is_err() {
            break;
        }
    }
}

fn spawn_reader_thread(shared: Arc<SharedState>, mut reader: Box<dyn Read + Send>) {
    thread::spawn(move || {
        let mut buffer = [0_u8; 4096];
        loop {
            match reader.read(&mut buffer) {
                Ok(0) => break,
                Ok(n) => shared.append_output(String::from_utf8_lossy(&buffer[..n]).into_owned()),
                Err(_) => {
                    shared.transition(TerminalHelperStatus::Error);
                    return;
                }
            }
        }
        shared.transition(TerminalHelperStatus::Exited);
    });
}

fn now_ms() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|duration| duration.as_millis() as u64)
        .unwrap_or(0)
}
