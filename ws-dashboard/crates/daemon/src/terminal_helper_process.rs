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
use crate::terminal_helper_ipc::{write_ndjson, NdjsonReader, WriteFault};
use crate::terminal_helper_protocol::{
    DaemonToHelperMessage, HelperToDaemonMessage, TerminalHelperOutputChunk, TerminalHelperStatus,
};
use crate::terminal_ipc_transport::{BoxedIpcStream, IpcListener};
use crate::terminal_registry_file::{delete_registry_entry, write_registry_entry, TerminalRegistryEntry};

const MAX_OUTPUT_CHUNKS: usize = 1024;
const GRACE_WINDOW: Duration = Duration::from_secs(30);
const IDLE_ACCEPT_POLL: Duration = Duration::from_secs(2);
// CONTRACT (260726 Phase 1 sub-fix 2): bounds how long this helper waits for
// its FIRST successful handshake before self-exiting, independent of
// whatever the daemon is doing (covers "daemon crashes/never connects before
// this helper's first accept"). Comfortably above the daemon's own
// `DEFAULT_CONNECT_TIMEOUT`/`DEFAULT_RECONCILE_CONNECT_TIMEOUT` connect
// budgets (`terminal.rs`) to leave margin for retry/backoff; an
// executor-adjustable constant, not a hard ticket number. Only gates the
// pre-handshake wait - once `shell_started` flips true, the existing
// `IDLE_ACCEPT_POLL` idle-reconnect behavior is unchanged.
const NO_HANDSHAKE_TIMEOUT: Duration = Duration::from_secs(10);
// CONTRACT (260729 helper liveness probe): bounds how long a CONCURRENT
// probe connection may occupy a task before it is dropped. A probe is
// strictly request/response - one line in, one line out - so anything slower
// than this is a peer that connected and then said nothing, which is not the
// helper's problem to wait on. Deliberately generous relative to a local
// socket round trip and irrelevant to the attached session's latency, since
// probe connections are served on their own tasks and never block
// `handle_connection`.
pub(crate) const PROBE_CONNECTION_TIMEOUT: Duration = Duration::from_secs(5);
// CONTRACT (260729 review finding F12): the concurrent probe arm spawns a
// task per accepted probe connection, and nothing about a stranger's connect
// rate is under this helper's control - an unbounded `tokio::spawn` there is
// a task-growth channel any local process can drive. This caps the number of
// in-flight probe tasks.
//
// The bound is applied as BACKPRESSURE, never as a drop: at capacity the
// accept arm simply stops accepting, so the surplus connection waits in the
// listener backlog until a slot frees instead of being accepted and closed.
// That distinction is load-bearing - a dropped probe connection reads
// daemon-side as `ProbeVerdict::Unanswered`, which authorizes a SIGKILL, so
// "shed load" here would mean "manufacture the exact false-death signal this
// ticket exists to remove". Each probe task is one line in, one line out and
// is hard-bounded by `PROBE_CONNECTION_TIMEOUT`, so a queue drains in
// microseconds under any non-pathological load.
const MAX_CONCURRENT_PROBE_CONNECTIONS: usize = 8;
// CONTRACT (260729 review round 3, finding B): `accept()` failing is not
// evidence that this helper should die. EMFILE/ENFILE (someone else on this
// machine exhausted the fd table), ECONNABORTED (a peer that connected and
// vanished before the accept completed) and the Windows named-pipe
// connect failures are all PEER- or environment-influenced and all transient,
// yet every one of them used to unwind into `run_terminal_helper`'s
// `kill_shell_if_running()`. The listener errors; the shell does not.
//
// So every accept failure is retried with backoff, and only a listener that
// has failed CONSECUTIVELY for this whole budget is treated as permanently
// broken. There is deliberately no per-ErrorKind "is this transient?" test:
// that is the same enumerate-the-kinds mistake finding A removed from the read
// path, and the safe default here is "retry", not "die".
const ACCEPT_RETRY_BACKOFF_START: Duration = Duration::from_millis(20);
const ACCEPT_RETRY_BACKOFF_MAX: Duration = Duration::from_secs(1);
// 20ms doubling to a 1s ceiling reaches the ceiling after 6 retries, so this
// budget spans roughly a minute of UNBROKEN failure. Sized to outlast any
// transient fd-table or backlog pressure while still bounding a listener that
// is genuinely unusable (a closed/invalid fd fails instantly and forever). A
// success anywhere in the run resets the counter, so intermittent failures
// never accumulate to it.
const MAX_CONSECUTIVE_ACCEPT_FAILURES: u32 = 64;

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
    // CONTRACT (260729 helper liveness probe): when the LAST daemon
    // disconnected, or `Some(process start)` if none ever attached. `None`
    // means a daemon is attached right now.
    //
    // This is genuinely new state, not a rename of `exited_at`: `exited_at`
    // is set ONLY when the shell exits (`transition`), and the read loop's
    // daemon-disconnect arm previously stored nothing at all. Without it the
    // probe has no duration to report and the daemon-side predicate collapses
    // back to two-way ("answers or not"), which cannot tell a healthy helper
    // whose daemon is restarting from a real orphan whose daemon is never
    // coming back.
    //
    // Only an ATTACHED connection (one that received `HandshakeAck`) moves
    // this: probe connections deliberately never touch it, or a daemon
    // probing an orphan every 10s would keep resetting its unattached clock
    // and the orphan would become unreclaimable.
    unattached_since: Mutex<Option<Instant>>,
    // CONTRACT (260723 Phase-1 review finding I2, Windows Job-Object
    // wiring): must be kept alive for the helper's whole lifetime (see
    // `terminal_platform::windows::create_kill_on_close_job`'s doc comment)
    // - dropping it closes the Job Object handle, which tears down every
    // process still assigned to it. Populated once, in `spawn_shell`.
    // Windows-only: Unix gets the equivalent guarantee from `setsid()` +
    // PTY-master-close instead (see `terminal_platform::unix`).
    #[cfg(windows)]
    job: Mutex<Option<std::os::windows::io::OwnedHandle>>,
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
        // Stamp the ring `Terminated` BEFORE killing the child so a
        // concurrent exit observer that wakes on child death finds a
        // non-`Running` ring and its own `transition(Exited)` becomes a
        // genuine no-op (see `transition`'s `Running`-only guard) instead of
        // racing an `Exited` status over this intentional, daemon-initiated
        // kill. On Windows the #[cfg(windows)] handle-wait reaper
        // (`spawn_shell`) is exactly such an observer; the reader thread's
        // PTY-EOF `transition(Exited)` is another. Only the status flag is
        // set here - the PTY master and writer channel are deliberately torn
        // down AFTER the child is killed below, because only child death
        // reliably unblocks a writer thread stuck on a full OS pipe buffer
        // (mental-model "Common Mistakes"); tearing them down first would
        // reintroduce that starvation.
        {
            let mut ring = self.ring.lock().expect("ring lock poisoned");
            if ring.status == TerminalHelperStatus::Running {
                ring.status = TerminalHelperStatus::Terminated;
            }
        }
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
            // CONTRACT (260726 Phase 1 sub-fix 3b, Unix zombie fix): a
            // self-detected exit (PTY-EOF reader thread on Unix; this same
            // path on Windows too, where it is a harmless idempotent extra
            // `wait()` alongside the handle-wait reaper) never previously
            // reaped `self.child` - only `kill_shell_if_running` did. Take
            // and `wait()` it here (no `.kill()`: the child already exited
            // on its own, unlike the daemon-initiated kill path) so every
            // self-detected-exit path is reaped uniformly, not just the
            // daemon-initiated one.
            if let Some(mut child) = self.child.lock().expect("child lock poisoned").take() {
                let _ = child.wait();
            }
        }
        self.notify.notify_one();
    }

    fn exited_at(&self) -> Option<Instant> {
        *self.exited_at.lock().expect("exited_at lock poisoned")
    }

    /// Called exactly once per connection, on the `HandshakeAck` that makes
    /// that connection an ATTACHED session. Never called from the probe path.
    fn mark_attached(&self) {
        *self
            .unattached_since
            .lock()
            .expect("unattached_since lock poisoned") = None;
    }

    /// Called when an attached connection ends, by whatever route (clean
    /// disconnect, transport error, graceful shutdown) - see
    /// `AttachmentGuard`, which is what makes "by whatever route" true.
    fn mark_unattached(&self) {
        *self
            .unattached_since
            .lock()
            .expect("unattached_since lock poisoned") = Some(Instant::now());
    }

    /// `(attached, unattached_for_ms)` exactly as the probe response carries
    /// them. `attached` is authoritative; the duration is `None` while
    /// attached because there is nothing to measure.
    fn liveness_report(&self) -> (bool, Option<u64>) {
        match *self
            .unattached_since
            .lock()
            .expect("unattached_since lock poisoned")
        {
            None => (true, None),
            Some(since) => (false, Some(since.elapsed().as_millis() as u64)),
        }
    }

    fn status_and_next_sequence(&self) -> (TerminalHelperStatus, u64) {
        let ring = self.ring.lock().expect("ring lock poisoned");
        (ring.status, ring.next_sequence)
    }
}

/// CONTRACT (260729 review round 3, finding F - THE TEST SEAM): which of the
/// three helper-side behaviours this ticket added are actually present in a
/// running helper.
///
/// Why this exists at all: every real-helper test in the tree runs the CURRENT
/// binary, which by construction has all the fixes. The population this
/// ticket's compatibility claims are ABOUT - helpers spawned by the previous
/// binary, still alive, still holding a user's shell - was therefore untested
/// by construction. That is the same fixture-is-not-the-population blind spot
/// the first review round found, one layer up.
///
/// `CURRENT` is what this binary is; nothing in production ever constructs
/// anything else. `LEGACY` is test-only and reproduces a pre-upgrade helper
/// FAITHFULLY, in all three respects at once, so a test can assert what
/// actually happens to one rather than what we hope happens.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) struct HelperShape {
    /// `true`: a fault attributable to the peer's connection ends only that
    /// connection (finding A). `false`: it propagates, and
    /// `run_terminal_helper`'s exit path SIGKILLs the shell - the pre-upgrade
    /// behaviour.
    peer_faults_end_only_the_connection: bool,
    /// `true`: probes are answerable while a session is attached. `false`: the
    /// accept loop awaits the session inline, so a probe sits unread in the
    /// backlog - the pre-upgrade behaviour, and the reason a busy helper
    /// looked dead.
    concurrent_probe_arm: bool,
    /// What goes into `<registry_dir>/<id>.json` as `supportsLivenessProbe`.
    /// `false` is what a pre-upgrade entry decodes to via `#[serde(default)]`.
    declares_liveness_probe: bool,
}

impl HelperShape {
    /// What this binary is.
    pub(crate) const CURRENT: Self = Self {
        peer_faults_end_only_the_connection: true,
        concurrent_probe_arm: true,
        declares_liveness_probe: true,
    };

    /// A helper as it existed before this ticket landed: the at-risk
    /// population. Test-only - production has exactly one shape.
    #[cfg(test)]
    pub(crate) const LEGACY: Self = Self {
        peer_faults_end_only_the_connection: false,
        concurrent_probe_arm: false,
        declares_liveness_probe: false,
    };
}

// Pure builder for the entry `run_terminal_helper` durably writes before it
// binds its listener. Extracted so the two facts a daemon reads off disk
// BEFORE it ever puts a byte on this helper's wire - the boot identity and
// (260729) the liveness-probe capability - are unit-testable without spawning
// a helper process.
fn startup_registry_entry(
    args: &TerminalHelperArgs,
    pid: u32,
    start_time: u64,
    shape: HelperShape,
) -> TerminalRegistryEntry {
    TerminalRegistryEntry {
        terminal_id: args.terminal_id.clone(),
        work_root_id: args.work_root_id.clone(),
        pid,
        start_time,
        // CONTRACT (boot-identity gate): recorded in the same breath as
        // `start_time`, because on a boot-relative platform the two are only
        // meaningful together. Deliberately NOT fatal when it comes back
        // `None` (unlike `start_time`, which hard-errors in the caller): the
        // daemon side treats a missing boot id as unverifiable, i.e. this
        // helper becomes un-reapable rather than the terminal failing to
        // start at all. Degrading a rare kernel/procfs oddity into "this one
        // helper must self-exit on its own timers" is the conservative trade;
        // a hard error would take down terminal creation outright.
        boot_id: crate::terminal_platform::boot_identity(),
        // CONTRACT (260729 helper liveness probe): this binary answers
        // `DaemonToHelperMessage::LivenessProbe`, so say so HERE, where every
        // daemon-side kill site can read it before connecting. A helper that
        // does not declare it is never sent a probe (which would SIGKILL its
        // shell) and is never reaped merely for staying silent - see
        // `TerminalRegistryEntry::supports_liveness_probe`.
        supports_liveness_probe: shape.declares_liveness_probe,
        socket_path: args.socket_path.clone(),
        created_at_ms: now_ms(),
        title: args.title.clone(),
        cwd_hint: args.cwd_hint.clone(),
        columns: args.columns,
        rows: args.rows,
    }
}

pub async fn run_terminal_helper(args: TerminalHelperArgs) -> anyhow::Result<()> {
    run_terminal_helper_shaped(args, HelperShape::CURRENT).await
}

pub(crate) async fn run_terminal_helper_shaped(
    args: TerminalHelperArgs,
    shape: HelperShape,
) -> anyhow::Result<()> {
    let pid = std::process::id();
    let start_time = crate::terminal_platform::process_start_time(pid).ok_or_else(|| {
        anyhow::anyhow!("failed to read own process start time for identity registration (pid {pid})")
    })?;

    let entry = startup_registry_entry(&args, pid, start_time, shape);
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
        // A helper starts life unattached: the clock runs from process
        // start, so a helper whose daemon never connects at all still
        // reports a growing unattached duration rather than a bare "not
        // attached" with nothing to compare against.
        unattached_since: Mutex::new(Some(Instant::now())),
        #[cfg(windows)]
        job: Mutex::new(None),
    });

    let result = serve_connections(&args, &mut listener, &shared, shape).await;

    shared.kill_shell_if_running();
    delete_registry_entry(&args.registry_dir, &args.terminal_id);
    let _ = std::fs::remove_file(&args.socket_path);
    result
}

/// CONTRACT (260729 review round 3, finding B): consecutive-`accept()`-failure
/// state, shared by both accept sites (`serve_connections`' idle loop and
/// `serve_session`'s concurrent probe arm) so a listener that fails in one of
/// them cannot have its budget silently reset by the other.
struct AcceptFailures {
    consecutive: u32,
}

impl AcceptFailures {
    fn new() -> Self {
        Self { consecutive: 0 }
    }

    fn reset(&mut self) {
        self.consecutive = 0;
    }

    /// Records one failure. `Some(delay)` means "back off this long and keep
    /// accepting"; `None` means the listener has failed for the whole budget
    /// and is treated as permanently broken.
    fn record(&mut self, error: &std::io::Error) -> Option<Duration> {
        self.consecutive += 1;
        if self.consecutive >= MAX_CONSECUTIVE_ACCEPT_FAILURES {
            return None;
        }
        let delay = ACCEPT_RETRY_BACKOFF_START
            .saturating_mul(1u32 << (self.consecutive - 1).min(16))
            .min(ACCEPT_RETRY_BACKOFF_MAX);
        tracing::debug!(
            %error,
            consecutive = self.consecutive,
            backoff_ms = delay.as_millis() as u64,
            "terminal helper accept() failed; backing off and continuing (the shell is untouched)"
        );
        Some(delay)
    }
}

async fn serve_connections(
    args: &TerminalHelperArgs,
    listener: &mut IpcListener,
    shared: &Arc<SharedState>,
    shape: HelperShape,
) -> anyhow::Result<()> {
    // CONTRACT (sub-fix 2): captured once, at helper-process start, so
    // `NO_HANDSHAKE_TIMEOUT` below bounds the wait for this helper's very
    // first successful handshake - not merely the current accept-loop
    // iteration.
    let started_at = Instant::now();
    // Created once per helper, not per session: probe tasks spawned during
    // one session can outlive it (they are bounded by
    // `PROBE_CONNECTION_TIMEOUT`, not by the session), so a per-session
    // semaphore would let the true in-flight count exceed the cap across a
    // reconnect.
    let probe_slots = Arc::new(tokio::sync::Semaphore::new(MAX_CONCURRENT_PROBE_CONNECTIONS));
    let mut accept_failures = AcceptFailures::new();
    loop {
        let wait = match shared.exited_at() {
            Some(exited_at) => match GRACE_WINDOW.checked_sub(exited_at.elapsed()) {
                Some(remaining) => remaining,
                None => break,
            },
            // `shell_started` (flipped exactly once, on the first
            // `HandshakeAck`) doubles as "has a handshake ever completed".
            // Once true, a connection has succeeded at least once and the
            // shell may still be legitimately running with no current
            // connection - keep the existing unconditional idle-reconnect
            // poll. Before that, bound the wait by how long this helper has
            // been alive without ever completing a handshake; elapsing this
            // budget `break`s the accept loop the same way grace exhaustion
            // above already does, self-exiting the helper.
            None if shared.shell_started.load(Ordering::SeqCst) => IDLE_ACCEPT_POLL,
            None => match NO_HANDSHAKE_TIMEOUT.checked_sub(started_at.elapsed()) {
                Some(remaining) => remaining,
                None => break,
            },
        };
        match tokio::time::timeout(wait, listener.accept()).await {
            Ok(Ok(stream)) => {
                accept_failures.reset();
                match serve_session(
                    args,
                    stream,
                    shared,
                    listener,
                    &probe_slots,
                    &mut accept_failures,
                    shape,
                )
                .await?
                {
                    ConnectionOutcome::Shutdown => break,
                    // One reattach after the shell has exited is the grace
                    // window's whole purpose - deliver the exit + trailing
                    // output once, then self-exit rather than lingering for
                    // the rest of the 30s.
                    //
                    // CONTRACT (260729 helper liveness probe): gated on
                    // `attached`, i.e. on a connection that actually sent
                    // `HandshakeAck`. A daemon that merely PROBED this helper
                    // (which reaches `handle_connection` when no session is
                    // attached, since there is no session future to run the
                    // concurrent arm alongside) must not consume the one
                    // grace-window reattach its real owner is coming back
                    // for - that would let another daemon's 10s sweep end a
                    // terminal it is forbidden from killing.
                    ConnectionOutcome::Disconnected { attached } => {
                        if attached && shared.exited_at().is_some() {
                            break;
                        }
                    }
                }
            }
            // CONTRACT (finding B): an `accept()` failure ends the ACCEPT, not
            // the helper. Returning here ran `kill_shell_if_running()` for an
            // EMFILE somebody else on the machine caused.
            Ok(Err(error)) => match accept_failures.record(&error) {
                Some(delay) => {
                    tokio::time::sleep(delay).await;
                    continue;
                }
                None => return Err(error.into()),
            },
            Err(_elapsed) => continue,
        }
    }
    Ok(())
}

/// How a single accepted session connection ended.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum ConnectionOutcome {
    /// The daemon went away (clean disconnect). `attached` records whether
    /// this connection ever became a real session, i.e. whether it sent
    /// `HandshakeAck` - a probe-only connection did not.
    Disconnected { attached: bool },
    /// The daemon requested a full graceful shutdown; the helper stops.
    Shutdown,
}

// CONTRACT (260729 helper liveness probe, the load-bearing half): a new
// message kind ALONE would not have worked. `serve_connections` awaits the
// session inline and never polls `accept()` while a session is attached, so
// a probe would sit unread in the listener backlog - which is exactly
// today's failure, where "connect succeeded, nobody answered" is read as
// death. This function adds the concurrent accept arm that makes the probe
// answerable while a session is attached.
//
// FORBIDDEN, and the reason this is not simply "spawn every connection":
// the probe arm must never dispatch `handle_connection`. Doing so would make
// it a second full attach path (shell spawn, input, resize, backfill,
// graceful shutdown) and break the one-session-at-a-time guarantee the rest
// of this module is built on. Probe-only accept, probe-only response - the
// spawned task is `serve_probe_connection`, which cannot spawn a shell,
// cannot flip `shell_started`, and cannot touch `unattached_since`.
// KNOWN LIMITATION (260729 review finding F11), stated here rather than left
// to be rediscovered: every connection accepted by this arm is served as a
// PROBE and nothing else. A daemon that connects intending to ADOPT while a
// session is still attached therefore gets a peer that never sends
// `Handshake`, so its `connect_and_handshake` reports `ConnectedButSilent`
// and that boot's `boot_reconcile` does not adopt the terminal (it leaves it
// alone - the entry, the process and its state all survive; only the
// adoption is missed, and the next daemon start retries it).
//
// Trigger, precisely: daemon B connects while daemon A's session is genuinely
// still attached, AND A disconnects inside B's connect/handshake budget. If A
// has already disconnected when B connects, `biased` below makes the finished
// session future win the select, so `serve_connections` re-parks in its own
// `accept()` and B's connection is served as a full session as before.
//
// Not fixed here because the fix is disproportionate to the exposure: an
// accepted connection cannot be promoted to a session without making this a
// second attach path, which the ticket forbids by name, and handing it back
// to `serve_connections` cannot help either - the daemon's whole
// connect+handshake budget (400ms) elapses long before this arm's
// `PROBE_CONNECTION_TIMEOUT` (5s) would release it. The pre-ticket behaviour
// in the same window was not "adopts reliably" but "queues, times out, then
// EPIPEs the helper into SIGKILLing the user's shell" (see `is_peer_gone`).
async fn serve_session(
    args: &TerminalHelperArgs,
    stream: BoxedIpcStream,
    shared: &Arc<SharedState>,
    listener: &mut IpcListener,
    probe_slots: &Arc<tokio::sync::Semaphore>,
    accept_failures: &mut AcceptFailures,
    shape: HelperShape,
) -> anyhow::Result<ConnectionOutcome> {
    let mut session = std::pin::pin!(handle_connection(args, stream, shared, shape));
    // The pre-upgrade shape (test-only, see `HelperShape`): await the session
    // inline with no concurrent accept arm at all, which is exactly why a busy
    // helper could not answer a probe and therefore looked dead.
    if !shape.concurrent_probe_arm {
        return session.await;
    }
    loop {
        // Take a probe slot BEFORE accepting (finding F12). While all slots
        // are held this select has only the session arm, so the listener is
        // not drained and surplus probe connections stay queued in the
        // backlog rather than being accepted and dropped - see
        // `MAX_CONCURRENT_PROBE_CONNECTIONS` for why dropping would be
        // actively harmful. `acquire_owned` resolves immediately whenever a
        // slot is free, so the uncontended path is unchanged.
        let permit = tokio::select! {
            biased;
            outcome = &mut session => return outcome,
            permit = probe_slots.clone().acquire_owned() => {
                permit.expect("probe slot semaphore is never closed")
            }
        };
        tokio::select! {
            // Biased so an available session event is always taken first:
            // the attached daemon's traffic must never lose priority to a
            // stream of probes.
            biased;
            outcome = &mut session => return outcome,
            accepted = listener.accept() => match accepted {
                Ok(probe_stream) => {
                    accept_failures.reset();
                    tokio::spawn(serve_probe_connection(shared.clone(), probe_stream, permit));
                }
                // CONTRACT (finding B): identical rule to the idle accept
                // loop's - a failed accept must not reach
                // `kill_shell_if_running()`. The backoff is awaited inside a
                // select with the session so an accept storm cannot starve the
                // attached daemon's traffic.
                Err(error) => {
                    drop(permit);
                    let Some(delay) = accept_failures.record(&error) else {
                        return Err(error.into());
                    };
                    tokio::select! {
                        biased;
                        outcome = &mut session => return outcome,
                        () = tokio::time::sleep(delay) => {}
                    }
                }
            },
        }
    }
}

// The whole of the probe-only accept arm's behaviour. Reads at most one
// `LivenessProbe` (bounded by `PROBE_CONNECTION_TIMEOUT`), answers it, and
// drops the connection.
//
// CONTRACT: strictly request-gated - nothing is written until a
// `LivenessProbe` has been read, so a helper never emits
// `LivenessProbeResponse` to a daemon that did not ask for one (an old
// daemon receiving an unknown variant would escalate it to a kill).
// Deliberately swallows every error rather than propagating with `?`: a
// malformed or half-open probe connection is a stranger's problem, and
// letting it reach `run_terminal_helper`'s exit path would kill the user's
// shell over a stray byte from a process that is not even attached.
async fn serve_probe_connection(
    shared: Arc<SharedState>,
    stream: BoxedIpcStream,
    // Held for exactly as long as this task runs; dropping it is what lets
    // `serve_session` accept the next probe connection (finding F12).
    _slot: tokio::sync::OwnedSemaphorePermit,
) {
    let (read_half, mut write_half) = crate::terminal_ipc_transport::split(stream);
    let mut reader = NdjsonReader::new(read_half);
    let _ = tokio::time::timeout(PROBE_CONNECTION_TIMEOUT, async move {
        loop {
            match reader.read_message::<DaemonToHelperMessage>().await {
                Ok(Some(DaemonToHelperMessage::LivenessProbe)) => {
                    let (attached, unattached_for_ms) = shared.liveness_report();
                    let _ = write_ndjson(
                        &mut write_half,
                        &HelperToDaemonMessage::LivenessProbeResponse {
                            attached,
                            unattached_for_ms,
                        },
                    )
                    .await;
                    return;
                }
                // Probe-only: a session message arriving on a concurrent
                // connection is ignored, never acted on. Acting on it here
                // is precisely the "second attach path" this arm forbids.
                Ok(Some(_)) => continue,
                Ok(None) | Err(_) => return,
            }
        }
    })
    .await;
}

// CONTRACT (260729 review finding F9, WIDENED by review round 3 finding A -
// and this is the invariant the whole ticket rests on):
//
//   NO ERROR ATTRIBUTABLE TO THE PEER'S CONNECTION MAY END THE HELPER PROCESS
//   OR KILL ITS SHELL. IT ENDS THE CONNECTION; THE HELPER RETURNS TO ITS
//   ACCEPT LOOP.
//
// Before this ticket, every write in `handle_connection` propagated with `?`,
// and `run_terminal_helper` runs `kill_shell_if_running()` +
// `delete_registry_entry()` on ANY return from `serve_connections` - so a
// daemon that connected and dropped without reading (which is precisely what a
// reachability check does) made the helper SIGKILL the user's shell and erase
// its own registry entry.
//
// Round 2 fixed that for a hand-written list of I/O `ErrorKind`s and left
// everything else fatal. That was still wrong, and the list was wrong in the
// most common case of all: a daemon SIGKILLed mid-`write_all` leaves a
// TRUNCATED final line, `Lines::next_line()` returns those trailing bytes as
// `Some(line)` at EOF, and the resulting decode failure was classified fatal.
// So the ordinary crash of the ordinary peer killed the user's shell. Invalid
// UTF-8 in that same tail (`ErrorKind::InvalidData`), an unknown variant tag
// from a NEWER daemon, `ErrorKind::WriteZero` (what `write_all` actually
// returns on a short write) and Windows `ERROR_OPERATION_ABORTED` (995) were
// all fatal for the same reason: an allow-list of kinds cannot be shown to be
// complete, and every kind missing from it is a dead shell.
//
// The classification is therefore BY SOURCE, in the transport itself
// (`terminal_helper_ipc::PeerFault` / `WriteFault`), not by kind here. There
// is nothing left to enumerate: reading can only fail because of the peer, and
// writing fails either because of the peer's connection (`WriteFault::Peer`,
// every kind) or because THIS process could not serialize its own message
// (`WriteFault::Serialize`), which stays fatal because no reconnect can fix it.
//
// This also makes the unknown-variant defence STRUCTURAL. The ticket's
// Decisions made `supportsLivenessProbe` the defence against a new variant
// reaching a helper that predates it; after this change that flag is
// defence-in-depth (it still stops the probe being SENT, which matters for the
// helpers that predate this fix and have no such guarantee), and the guarantee
// for every helper from this build onwards is here: an unparseable line, from
// any cause, drops the connection and nothing else.

/// The whole of the write-side classification, extracted only so it is
/// directly assertable. There is nothing to enumerate: the discriminant is
/// which SOURCE produced the fault, so no `ErrorKind` can be "missing" from it.
/// Reads need no equivalent - every read fault is a peer fault by
/// construction, which is `PeerFault`'s reason for existing.
fn write_fault_ends_only_the_connection(fault: &WriteFault) -> bool {
    match fault {
        WriteFault::Peer(_) => true,
        WriteFault::Serialize(_) => false,
    }
}

/// Restarts the unattached clock when an attached connection ends, on every
/// exit route out of `handle_connection` including the `?` ones. See that
/// function's own comment for why this is a guard and not a call site.
struct AttachmentGuard<'a> {
    shared: &'a Arc<SharedState>,
    attached: bool,
}

impl Drop for AttachmentGuard<'_> {
    fn drop(&mut self) {
        if self.attached {
            self.shared.mark_unattached();
        }
    }
}

/// Serves a single accepted connection until it closes.
async fn handle_connection(
    args: &TerminalHelperArgs,
    stream: BoxedIpcStream,
    shared: &Arc<SharedState>,
    shape: HelperShape,
) -> anyhow::Result<ConnectionOutcome> {
    // CONTRACT (260729 helper liveness probe): the unattached clock must
    // restart when an attached connection ends BY WHATEVER ROUTE - clean
    // disconnect, transport error propagated with `?`, or graceful shutdown.
    // A guard rather than a line at each `return` is what makes that true;
    // the `?`s below are exactly the routes a hand-placed call would miss,
    // and a missed one leaves the helper reporting "attached" forever, which
    // makes it unreclaimable by every daemon-side path.
    let mut attachment = AttachmentGuard {
        shared,
        attached: false,
    };

    let (read_half, write_half) = crate::terminal_ipc_transport::split(stream);
    let mut reader = NdjsonReader::new(read_half);
    let write_half = Arc::new(tokio::sync::Mutex::new(write_half));

    // CONTRACT (260729 review finding F9, widened by round 3 finding A): the
    // ONLY way a write reaches the daemon peer from this function. A
    // `WriteFault::Peer` - ANY I/O kind, since the source is what classifies
    // it - returns the same `Disconnected` outcome the EOF arm below returns:
    // the connection ends, `serve_connections` goes back to accepting, and the
    // shell lives. Only `WriteFault::Serialize` (this process failing to
    // serialize its own message) still propagates. Do not reintroduce a bare
    // `?` on any write here: `run_terminal_helper` turns any `Err` out of
    // `serve_connections` into `kill_shell_if_running()` +
    // `delete_registry_entry()`.
    macro_rules! write_to_daemon {
        ($message:expr) => {
            match write_ndjson(&mut *write_half.lock().await, $message).await {
                Ok(()) => {}
                Err(fault)
                    if shape.peer_faults_end_only_the_connection
                        && write_fault_ends_only_the_connection(&fault) =>
                {
                    tracing::debug!(
                        %fault,
                        "terminal helper's daemon peer went away mid-write; ending this \
                         connection and returning to the accept loop (the shell is untouched)"
                    );
                    return Ok(ConnectionOutcome::Disconnected {
                        attached: attachment.attached,
                    });
                }
                Err(error) => return Err(error.into()),
            }
        };
    }

    write_to_daemon!(&HelperToDaemonMessage::Handshake {
        pid: shared.pid,
        start_time: shared.start_time,
    });

    let (status, next_sequence) = shared.status_and_next_sequence();
    let mut last_sent_status = status;
    let initial_message = if status == TerminalHelperStatus::Running {
        HelperToDaemonMessage::Status { status, next_sequence }
    } else {
        HelperToDaemonMessage::Exit { status, next_sequence }
    };
    write_to_daemon!(&initial_message);

    // CONTRACT (260723 Phase-1 review finding I1 - cross-restart backfill
    // gap): a fresh connection has nothing "already sent" from this
    // process's own point of view, so unconditionally flush the entire
    // retained ring here, BEFORE entering the select loop below. This is
    // what actually guarantees the grace-reattach/boot-reconcile-adopt
    // backfill: an earlier version of this function relied solely on the
    // `shared.notify.notified()` arm inside the loop to re-push the ring,
    // which only fires when a `Notify` permit already happens to be
    // pending - a QUIESCENT adopted shell (resume cursor behind the
    // helper's current sequence, but no further output produced after
    // reconnect) would never trigger it, silently dropping the backfill
    // (and `is_range_truncated` on the daemon side would see an empty
    // proxy ring and report no truncation either - a silent loss with no
    // flag). Flushing once, unconditionally, here removes that race: every
    // (re)connect - fresh spawn, grace-reattach, or boot-reconcile adopt -
    // always re-delivers whatever the ring still holds, whether or not new
    // output ever arrives afterward.
    let mut last_sent_sequence: u64 = 0;
    let initial_backfill = {
        let ring = shared.ring.lock().expect("ring lock poisoned");
        ring.backfill_after(last_sent_sequence)
    };
    for chunk in initial_backfill {
        last_sent_sequence = last_sent_sequence.max(chunk.sequence);
        write_to_daemon!(&HelperToDaemonMessage::Output(chunk));
    }

    loop {
        tokio::select! {
            incoming = reader.read_message::<DaemonToHelperMessage>() => {
                // Round 3 finding A: EVERY read failure is a `PeerFault` by
                // construction - a reset mid-read, a truncated final line from
                // a SIGKILLed daemon, invalid UTF-8 in that same tail, or a
                // variant tag from a newer daemon. All of them end this
                // connection and nothing more. The desync is still surfaced
                // (logged, connection dropped); what is gone is the escalation
                // from "the peer misspoke" to "SIGKILL the user's shell".
                let incoming = match incoming {
                    Ok(incoming) => incoming,
                    Err(fault) if shape.peer_faults_end_only_the_connection => {
                        tracing::debug!(
                            %fault,
                            "terminal helper's daemon peer faulted mid-read; ending this \
                             connection and returning to the accept loop (the shell is untouched)"
                        );
                        return Ok(ConnectionOutcome::Disconnected {
                            attached: attachment.attached,
                        });
                    }
                    Err(fault) => return Err(fault.into()),
                };
                match incoming {
                    // daemon disconnected; outer loop decides what's next
                    None => {
                        return Ok(ConnectionOutcome::Disconnected {
                            attached: attachment.attached,
                        })
                    }
                    Some(DaemonToHelperMessage::HandshakeAck) => {
                        // CONTRACT (260729): `HandshakeAck` - not merely
                        // accepting a connection - is what makes this an
                        // ATTACHED session. A probe connection never sends
                        // one, so probing can never reset an orphan's
                        // unattached clock.
                        if !attachment.attached {
                            attachment.attached = true;
                            shared.mark_attached();
                        }
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
                        write_to_daemon!(&HelperToDaemonMessage::BackfillResponse {
                            request_id,
                            chunks,
                            next_sequence,
                            status,
                        });
                    }
                    // CONTRACT (260729 helper liveness probe): answered here
                    // too, not only on the concurrent probe arm. When NO
                    // session is attached the helper is parked in
                    // `serve_connections`' own `accept()`, so a probe lands
                    // as an ordinary connection and reaches this dispatch -
                    // and that is exactly the case the "unattached past the
                    // grace" leg of the daemon-side predicate depends on
                    // being answerable. Answering costs nothing here: it
                    // never spawns a shell and never marks this connection
                    // attached, so it does not disturb `unattached_since`.
                    // FIDELITY SEAM (260729 review round 3, finding F): a
                    // pre-upgrade helper's `DaemonToHelperMessage` has no
                    // `LivenessProbe` variant at all, so the line does not
                    // decode - it is an unknown-variant fault, and on that
                    // build an unknown-variant fault SIGKILLs the shell. Model
                    // it here rather than letting a legacy-shaped fixture
                    // cheerfully answer a probe it could never have parsed;
                    // the daemon-side capability gate exists precisely because
                    // this is what a probe does to that population.
                    Some(DaemonToHelperMessage::LivenessProbe)
                        if !shape.declares_liveness_probe =>
                    {
                        if shape.peer_faults_end_only_the_connection {
                            return Ok(ConnectionOutcome::Disconnected {
                                attached: attachment.attached,
                            });
                        }
                        return Err(anyhow::anyhow!(
                            "unknown message variant (helper predates the liveness probe)"
                        ));
                    }
                    Some(DaemonToHelperMessage::LivenessProbe) => {
                        let (attached, unattached_for_ms) = shared.liveness_report();
                        write_to_daemon!(&HelperToDaemonMessage::LivenessProbeResponse {
                            attached,
                            unattached_for_ms,
                        });
                    }
                    Some(DaemonToHelperMessage::GracefulShutdown) => {
                        shared.kill_shell_if_running();
                        return Ok(ConnectionOutcome::Shutdown);
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
                    write_to_daemon!(&HelperToDaemonMessage::Output(chunk));
                }
                if status_now != last_sent_status {
                    last_sent_status = status_now;
                    let message = if status_now == TerminalHelperStatus::Running {
                        HelperToDaemonMessage::Status { status: status_now, next_sequence }
                    } else {
                        HelperToDaemonMessage::Exit { status: status_now, next_sequence }
                    };
                    write_to_daemon!(&message);
                }
            }
        }
    }
}

// CONTRACT (260725 Phase 1, pty-agent spawn-seam argv/env scrub): pure
// builder extracted from `spawn_shell` so both the "default (no explicit
// command) path is byte-for-byte unchanged" and the "explicit command scrubs
// Claude markers + applies overlay" contracts are testable without spawning
// a real process. Hop 1 (the daemon's helper spawn,
// `terminal.rs::build_helper_command`) independently scrubs too, since this
// hop's inherited env is itself inherited wholesale from hop 1; this hop's
// own scrub is not merely redundant with that, because it is the only hop
// that actually determines what a `command = Some(..)` spawn's process
// sees, so it must scrub regardless of hop 1's outcome.
//
// Review cycle 1, finding I1/C1: this used to seed a scrubbed copy of the
// host env via `command.env_clear()` + repopulate. On Windows,
// `CommandBuilder::new()`'s base env (`get_base_env`) additionally merges a
// registry-refreshed system+user `PATH` on top of the process's own
// inherited env; `env_clear()` destroyed that merge and the repopulation
// loop only restored the (stale) inherited copy underneath it, so an
// agent-profile terminal would resolve its program against a narrower
// `PATH` than an ordinary shell terminal from the same helper binary. Unix
// is unaffected (its `as_command()` conversion re-derives `SHELL`/`HOME`
// regardless), but the bug is real on Windows. Per-marker
// `command.env_remove(marker)` (see `apply_scrub_and_overlay` below)
// removes exactly the scrub markers from whatever `CommandBuilder::new()`
// already built - registry merge included - and touches nothing else, so it
// preserves the whole base-env construction on every platform. There is
// therefore no `host_env` parameter here anymore: the real base env comes
// from `CommandBuilder::new()` itself, not an injected copy.
fn build_shell_command(args: &TerminalHelperArgs, term: String) -> CommandBuilder {
    let mut command = match &args.command {
        None => CommandBuilder::new(crate::terminal::default_shell()),
        Some(program) => {
            let mut command = CommandBuilder::new(program);
            command.args(&args.command_args);
            apply_scrub_and_overlay(&mut command, &args.scrub_marker, &args.env_overlay);
            command
        }
    };
    command.cwd(&args.cwd);
    command.env("TERM", term);
    command
}

// CONTRACT (review cycle 1, finding T3 - LEAD DECISION, since neither the
// ticket nor the plan settles overlay-vs-scrub precedence explicitly): the
// scrub wins. An overlay pair keyed to one of `markers` must NOT resurrect
// it - this is a security-adjacent deny-list, and silent resurrection is
// exactly the failure it exists to prevent. Finding C3 separately flags
// `--env-overlay` as the argv channel a later phase is most likely to reach
// for by accident, which makes this precedence load-bearing rather than
// cosmetic. A colliding overlay key is dropped with a warning rather than
// applied or hard-failed: nothing populates `env_overlay` yet in this phase
// (Phase 2 is the first real caller), so a hard error here would add
// fallible-signature plumbing through `build_shell_command`/`spawn_shell` to
// protect a path with no live caller; a log-and-drop keeps the invariant
// enforced while staying cheap at this seam, and Phase 2+ callers get a
// visible signal if they ever hit this collision by mistake.
//
// CONTRACT (review cycle 1, finding C1): `markers` is the resolved
// profile's OWN scrub list, threaded from hop 1
// (`terminal.rs::build_helper_command`) via the repeated `--scrub-marker`
// argv flag - this used to be hardcoded to `agent_env_profile::CLAUDE.markers`
// unconditionally, which contradicted the header CONTRACT above ("this hop
// ... must scrub regardless of hop 1's outcome") for any profile whose
// markers are not a subset of `CLAUDE`'s. Both hops now honour the same
// list.
fn apply_scrub_and_overlay(command: &mut CommandBuilder, markers: &[String], overlay: &[(String, String)]) {
    for marker in markers {
        command.env_remove(marker);
    }
    for (key, value) in overlay {
        if markers.iter().any(|marker| marker == key) {
            tracing::warn!(
                %key,
                "env-overlay key matches a scrubbed marker; the scrub wins, overlay value dropped"
            );
            continue;
        }
        command.env(key, value);
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
    let command = build_shell_command(
        args,
        crate::terminal::browser_pty_term(|key| {
            std::env::var_os(key).map(|value| value.to_string_lossy().into_owned())
        }),
    );
    let child = pair.slave.spawn_command(command)?;
    drop(pair.slave);

    // CONTRACT (260723 Phase-1 review finding I2, Windows Job-Object
    // wiring): assign the newly spawned shell into a fresh helper-owned
    // kill-on-close Job Object so the verified-PID kill fallback tier
    // (`terminal_platform::kill_verified` against an IPC-unreachable
    // helper, e.g. boot-reconcile row 4) reliably takes the shell subtree
    // down too, not just the helper itself - `TerminateProcess` alone does
    // not tear down child processes on Windows, so without this wiring a
    // hard-killed helper would orphan its shell indefinitely. No-op on
    // Unix, which already gets that guarantee from `setsid()` + PTY-
    // master-close (see `terminal_platform::unix`).
    #[cfg(windows)]
    {
        if let Some(raw_handle) = child.as_raw_handle() {
            match crate::terminal_platform::windows::create_kill_on_close_job() {
                Ok(job) => {
                    if let Err(error) =
                        crate::terminal_platform::windows::assign_into_job(&job, raw_handle)
                    {
                        tracing::warn!(
                            %error,
                            "failed to assign terminal shell into kill-on-close job object"
                        );
                    }
                    *shared.job.lock().expect("job lock poisoned") = Some(job);
                }
                Err(error) => {
                    tracing::warn!(
                        %error,
                        "failed to create kill-on-close job object for terminal shell"
                    );
                }
            }

            // Event-driven exit detection that does NOT depend on PTY EOF.
            // On Windows a shell can die while ConPTY keeps the PTY master
            // pipe open (conhost holding it), so the reader thread's
            // EOF-triggered `transition(Exited)` (`spawn_reader_thread`) may
            // never fire and the terminal would wrongly stay `Running`
            // forever. Duplicate the shell's process handle into a handle we
            // own (distinct from the `Child`'s, which `kill_shell_if_running`
            // closes) and hand it to a detached reaper thread that blocks on
            // it and drives the same exit path on wake.
            match crate::terminal_platform::windows::duplicate_process_handle(raw_handle) {
                Ok(handle) => spawn_process_exit_reaper(shared.clone(), handle),
                Err(error) => {
                    tracing::warn!(
                        %error,
                        "failed to duplicate terminal shell handle for exit reaper; \
                         falling back to PTY-EOF-only exit detection"
                    );
                }
            }
        } else {
            tracing::warn!(
                "terminal shell child exposed no raw handle; skipping job-object assignment \
                 and exit reaper (PTY-EOF-only exit detection)"
            );
        }
    }

    let reader = pair.master.try_clone_reader()?;
    let writer = pair.master.take_writer()?;

    *shared.child.lock().expect("child lock poisoned") = Some(child);
    *shared.master.lock().expect("master lock poisoned") = Some(pair.master);
    *shared.writer_tx.lock().expect("writer_tx lock poisoned") = Some(spawn_writer_thread(writer));
    // Detached, never joined in production - matches `spawn_process_exit_reaper`'s
    // style below. The returned `JoinHandle` exists purely so tests can join it
    // deterministically instead of polling `status_and_next_sequence()`.
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

// CONTRACT (260724-bug-dashboard-terminal-utf8-multibyte-read-boundary
// -corruption): a raw `read()` from the PTY has no notion of codepoint
// boundaries - the OS can (and, for wide multi-byte glyphs like CJK or
// emoji, routinely does) split a single UTF-8 sequence across two `read()`
// calls. Decoding each read independently via `String::from_utf8_lossy`
// therefore corrupts every such boundary into U+FFFD even though the bytes
// are perfectly valid once reassembled. `carry` holds the tail of the
// previous read that was a genuinely incomplete-but-valid-so-far sequence
// (bounded to <=3 bytes - the longest a valid UTF-8 lead byte's encoding
// can run is 4 bytes total, so at most 3 can be pending) and is prepended
// to the next read's bytes before decoding. A truly malformed byte (not a
// split) is NOT carried - it is replaced with exactly one U+FFFD and
// decoding resumes past it, which is what prevents an unrecoverable byte
// from wedging the carry-over forever.
fn spawn_reader_thread(shared: Arc<SharedState>, mut reader: Box<dyn Read + Send>) -> thread::JoinHandle<()> {
    thread::spawn(move || {
        let mut scratch = [0_u8; 4096];
        let mut carry: Vec<u8> = Vec::new();
        loop {
            match reader.read(&mut scratch) {
                Ok(0) => {
                    // Genuine EOF: any still-pending carry is a sequence
                    // truncated at the true end of the stream (not a
                    // read-boundary split awaiting more bytes, since there
                    // are no more bytes) - degrade it to lossy replacement
                    // rather than dropping it silently.
                    if !carry.is_empty() {
                        shared.append_output(String::from_utf8_lossy(&carry).into_owned());
                        carry.clear();
                    }
                    break;
                }
                Ok(n) => {
                    let mut combined = std::mem::take(&mut carry);
                    combined.extend_from_slice(&scratch[..n]);
                    let mut remaining: &[u8] = &combined;
                    loop {
                        match std::str::from_utf8(remaining) {
                            Ok(valid) => {
                                shared.append_output(valid.to_owned());
                                break;
                            }
                            Err(error) => {
                                let valid_up_to = error.valid_up_to();
                                if valid_up_to > 0 {
                                    shared.append_output(
                                        std::str::from_utf8(&remaining[..valid_up_to])
                                            .expect("bytes before valid_up_to are always valid UTF-8")
                                            .to_owned(),
                                    );
                                }
                                match error.error_len() {
                                    None => {
                                        // Incomplete trailing sequence at the
                                        // end of this read's bytes: a
                                        // genuine read-boundary split. Carry
                                        // it forward for the next read
                                        // instead of guessing.
                                        carry = remaining[valid_up_to..].to_vec();
                                        break;
                                    }
                                    Some(malformed_len) => {
                                        // A genuinely malformed span (not a
                                        // split): emit exactly one U+FFFD for
                                        // it, then keep decoding the
                                        // remainder of THIS SAME read - do
                                        // not carry the malformed bytes.
                                        let malformed_end = valid_up_to + malformed_len;
                                        shared.append_output(
                                            String::from_utf8_lossy(&remaining[valid_up_to..malformed_end])
                                                .into_owned(),
                                        );
                                        remaining = &remaining[malformed_end..];
                                        if remaining.is_empty() {
                                            break;
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
                Err(_) => {
                    shared.transition(TerminalHelperStatus::Error);
                    return;
                }
            }
        }
        shared.transition(TerminalHelperStatus::Exited);
    })
}

// Windows-only, event-driven shell-exit detection independent of PTY EOF.
//
// The reader thread (`spawn_reader_thread`) is the ONLY exit trigger on
// Unix, keyed off PTY master EOF. On Windows/ConPTY a shell process can
// terminate while conhost keeps the master pipe open, so that EOF may never
// arrive and the terminal would remain `Running` forever - the reported
// zombie-pane bug. This reaper closes that gap at the source: it owns an
// independent `DuplicateHandle` copy of the shell process handle (NOT the
// borrowed `Child::as_raw_handle`, which `kill_shell_if_running` closes when
// it drops the `Child`) and blocks on it in the kernel (zero idle CPU, zero
// poll latency). The instant the shell process exits it drives the SAME
// `SharedState::transition(Exited)` the PTY-EOF reader uses, so the existing
// `Exit` IPC -> `apply_helper_status` -> WebSocket exit-frame pipeline is
// reused unchanged.
//
// Windows-only by design: a second Unix `waitpid`-based reaper would race
// and steal the reap from `portable_pty`'s own `Child::wait()` in
// `kill_shell_if_running`, whereas Windows process handles admit many
// concurrent independent waiters. Gated at the call site (`spawn_shell`'s
// `#[cfg(windows)]` block).
//
// Detached and never joined. On a daemon-initiated kill the ring is already
// `Terminated` (`kill_shell_if_running` stamps it before `child.kill()`), so
// this `transition(Exited)` is a genuine no-op; on a spontaneous shell death
// it is the authoritative exit signal. Exactly one shell is ever spawned per
// helper (`shell_started` compare_exchange), so exactly one reaper exists,
// and every helper-exit path runs `kill_shell_if_running`, which kills the
// shell and unblocks this wait - so neither the thread nor the duplicated
// handle leaks.
#[cfg(windows)]
fn spawn_process_exit_reaper(shared: Arc<SharedState>, handle: std::os::windows::io::OwnedHandle) {
    thread::spawn(move || {
        crate::terminal_platform::windows::wait_for_process_exit(&handle);
        shared.transition(TerminalHelperStatus::Exited);
    });
}

fn now_ms() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|duration| duration.as_millis() as u64)
        .unwrap_or(0)
}

// CONTRACT (260723 Phase-1 review finding M-b): `RingState` is the helper's
// own *authoritative* output ring (the ticket-pinned source of truth for
// scrollback, distinct from the daemon-side mirror cache in `terminal.rs`,
// which already has thorough eviction/truncation coverage). It is pure,
// synchronous, dependency-free logic, so it is cheap to unit-test directly
// here rather than only indirectly through a real PTY/IPC round trip.
#[cfg(test)]
mod ring_state_tests {
    use super::*;

    #[test]
    fn append_assigns_gapless_sequence_numbers_starting_at_one() {
        let mut ring = RingState::new();
        ring.append("a".to_owned());
        ring.append("b".to_owned());
        ring.append("c".to_owned());
        let chunks = ring.backfill_after(0);
        assert_eq!(
            chunks.iter().map(|chunk| chunk.sequence).collect::<Vec<_>>(),
            vec![1, 2, 3]
        );
        assert_eq!(ring.next_sequence, 4);
    }

    #[test]
    fn append_ignores_empty_chunks_without_consuming_a_sequence_number() {
        let mut ring = RingState::new();
        ring.append("a".to_owned());
        ring.append(String::new());
        ring.append("b".to_owned());
        let chunks = ring.backfill_after(0);
        assert_eq!(chunks.len(), 2, "empty chunk must not be retained: {chunks:?}");
        assert_eq!(
            chunks.iter().map(|chunk| chunk.sequence).collect::<Vec<_>>(),
            vec![1, 2],
            "skipping the empty append must not create a sequence gap"
        );
    }

    #[test]
    fn backfill_after_returns_only_chunks_strictly_newer_than_the_cursor() {
        let mut ring = RingState::new();
        for label in ["a", "b", "c", "d"] {
            ring.append(label.to_owned());
        }
        assert_eq!(ring.backfill_after(4).len(), 0, "nothing newer than the latest sequence");
        assert_eq!(
            ring.backfill_after(2)
                .into_iter()
                .map(|chunk| chunk.data)
                .collect::<Vec<_>>(),
            vec!["c".to_owned(), "d".to_owned()]
        );
        assert_eq!(ring.backfill_after(0).len(), 4, "after=0 means the full retained ring");
    }

    #[test]
    fn append_evicts_from_the_front_once_past_max_output_chunks() {
        let mut ring = RingState::new();
        for _ in 0..(MAX_OUTPUT_CHUNKS + 250) {
            ring.append("x".to_owned());
        }
        assert_eq!(
            ring.output.len(),
            MAX_OUTPUT_CHUNKS,
            "ring must never retain more than MAX_OUTPUT_CHUNKS chunks"
        );
        let oldest_retained = ring.output.front().expect("ring non-empty").sequence;
        let newest_retained = ring.output.back().expect("ring non-empty").sequence;
        assert_eq!(
            oldest_retained,
            (MAX_OUTPUT_CHUNKS + 250) as u64 - MAX_OUTPUT_CHUNKS as u64 + 1,
            "eviction must only ever drop from the front, in append order"
        );
        assert_eq!(newest_retained, (MAX_OUTPUT_CHUNKS + 250) as u64);
    }

    #[test]
    fn backfill_after_a_cursor_predating_eviction_silently_returns_only_what_remains() {
        // `RingState` itself has no truncation-detection concept (that lives
        // daemon-side, see `terminal.rs::is_range_truncated`) - this pins
        // down exactly what it DOES return for a stale cursor: everything
        // still retained, nothing more, nothing panics.
        let mut ring = RingState::new();
        for _ in 0..(MAX_OUTPUT_CHUNKS + 250) {
            ring.append("x".to_owned());
        }
        let oldest_retained = ring.output.front().expect("ring non-empty").sequence;
        let chunks = ring.backfill_after(1);
        assert_eq!(chunks.len(), MAX_OUTPUT_CHUNKS);
        assert_eq!(chunks.first().expect("non-empty").sequence, oldest_retained);
    }
}

// Regression coverage for the "make the guard real" invariant: the kill path
// stamps the ring `Terminated` before `child.kill()` so a racing exit
// observer (the #[cfg(windows)] handle-wait reaper, or the PTY-EOF reader)
// waking on the daemon-initiated kill runs `transition(Exited)` as a genuine
// no-op instead of overwriting the intentional `Terminated`. Both the guard
// and the stamp are cross-platform pure state logic (no OS/PTY dependency),
// so they are exercised directly here, NOT windows-gated.
#[cfg(test)]
mod kill_path_guard_tests {
    use super::*;
    use portable_pty::{ChildKiller, ExitStatus};

    /// Minimal `Child` stand-in that records whether `wait()` was called,
    /// without wiring a real PTY child - `transition`'s reap guard is pure
    /// state logic (which method got called), not a real-process concern
    /// (that is covered end-to-end by the real-process `terminal_lifetime`
    /// integration tests).
    #[derive(Debug)]
    struct RecordingChild {
        wait_called: Arc<AtomicBool>,
    }

    impl Child for RecordingChild {
        fn try_wait(&mut self) -> std::io::Result<Option<ExitStatus>> {
            Ok(Some(ExitStatus::with_exit_code(0)))
        }
        fn wait(&mut self) -> std::io::Result<ExitStatus> {
            self.wait_called.store(true, Ordering::SeqCst);
            Ok(ExitStatus::with_exit_code(0))
        }
        fn process_id(&self) -> Option<u32> {
            None
        }
        #[cfg(windows)]
        fn as_raw_handle(&self) -> Option<std::os::windows::io::RawHandle> {
            None
        }
    }

    impl ChildKiller for RecordingChild {
        fn kill(&mut self) -> std::io::Result<()> {
            Ok(())
        }
        fn clone_killer(&self) -> Box<dyn ChildKiller + Send + Sync> {
            Box::new(RecordingChild {
                wait_called: self.wait_called.clone(),
            })
        }
    }

    fn shared_state_for_test() -> SharedState {
        SharedState {
            pid: 0,
            start_time: 0,
            ring: Mutex::new(RingState::new()),
            notify: Notify::new(),
            child: Mutex::new(None),
            master: Mutex::new(None),
            writer_tx: Mutex::new(None),
            shell_started: AtomicBool::new(false),
            exited_at: Mutex::new(None),
            unattached_since: Mutex::new(Some(Instant::now())),
            #[cfg(windows)]
            job: Mutex::new(None),
        }
    }

    #[test]
    fn transition_is_a_no_op_once_the_ring_has_left_running() {
        let shared = shared_state_for_test();
        // Drive the ring out of `Running` directly (isolating this test to
        // `transition`'s guard, independent of how the kill path gets there).
        shared.ring.lock().expect("ring lock poisoned").status = TerminalHelperStatus::Terminated;

        // A racing exit observer waking after the daemon-initiated kill must
        // NOT downgrade the intentional `Terminated` back to `Exited`.
        shared.transition(TerminalHelperStatus::Exited);

        assert_eq!(
            shared.status_and_next_sequence().0,
            TerminalHelperStatus::Terminated,
            "transition(Exited) must be a genuine no-op once the ring is non-Running \
             - removing transition's Running-only guard would fail this"
        );
    }

    #[test]
    fn transition_from_running_still_reaches_exited() {
        // Negative control: the guard is specifically the non-`Running` gate,
        // not a blanket freeze - a `Running` ring DOES transition, so the
        // no-op above is genuinely conditional on prior state, not a
        // transition() that never mutates.
        let shared = shared_state_for_test();
        assert_eq!(
            shared.status_and_next_sequence().0,
            TerminalHelperStatus::Running,
            "fresh ring starts Running"
        );

        shared.transition(TerminalHelperStatus::Exited);

        assert_eq!(
            shared.status_and_next_sequence().0,
            TerminalHelperStatus::Exited,
            "a Running ring must still transition to Exited"
        );
    }

    #[test]
    fn kill_shell_if_running_stamps_terminated() {
        // The load-bearing half of the kill-path reorder that is testable on
        // Linux: `kill_shell_if_running` must leave the ring `Terminated`
        // (not `Running`, not `Exited`). Removing the pre-kill `Terminated`
        // stamp would leave the ring `Running` here and fail this assertion.
        // (The child is `None` in this unit test - wiring a real PTY child in
        // is disproportionate; the before/after-`child.kill()` write-unblock
        // ordering is a separate writer-starvation concern, not the guard
        // invariant under review, and combined with the two tests above this
        // fully covers the reaper's "Exited becomes a no-op" reliance.)
        let shared = shared_state_for_test();

        shared.kill_shell_if_running();

        assert_eq!(
            shared.status_and_next_sequence().0,
            TerminalHelperStatus::Terminated,
            "kill path must stamp Terminated so a racing exit observer's Exited is a no-op"
        );
    }

    // Regression coverage for 260726 Phase 1 sub-fix 3b: a self-detected
    // exit (the PTY-EOF reader thread calling `transition(Exited)` directly,
    // WITHOUT going through `kill_shell_if_running` first) must still reap
    // the child via `wait()` - otherwise the shell becomes a zombie once its
    // parent (this helper) never collects its exit status. Distinct from
    // `kill_shell_if_running_stamps_terminated` above, which covers the
    // daemon-initiated kill path's own child reap (`child.kill()` +
    // `child.wait()`) - this test isolates the OTHER path into `transition`.
    #[test]
    fn transition_from_running_reaps_the_child_via_wait_without_a_prior_kill() {
        let shared = shared_state_for_test();
        let wait_called = Arc::new(AtomicBool::new(false));
        *shared.child.lock().expect("child lock poisoned") = Some(Box::new(RecordingChild {
            wait_called: wait_called.clone(),
        }));

        shared.transition(TerminalHelperStatus::Exited);

        assert!(
            wait_called.load(Ordering::SeqCst),
            "a self-detected Running -> Exited transition must reap the child via wait(), \
             not merely drop it (that would leave a zombie)"
        );
        assert!(
            shared.child.lock().expect("child lock poisoned").is_none(),
            "the child slot must be taken once reaped"
        );
    }
}

// Regression coverage for 260724-bug-dashboard-terminal-utf8-multibyte-read
// -boundary-corruption: `spawn_reader_thread`'s carry-over decode loop must
// reassemble a UTF-8 sequence that a raw `read()` split across two chunks,
// must not hang/panic when a sequence is genuinely truncated at EOF, and
// must recover (not wedge) from a single malformed byte sandwiched between
// valid spans in one read. Self-contained like `kill_path_guard_tests` -
// duplicates `shared_state_for_test()` rather than widening that module's
// private helper's visibility.
#[cfg(test)]
mod reader_thread_utf8_tests {
    use super::*;
    use std::io;

    fn shared_state_for_test() -> SharedState {
        SharedState {
            pid: 0,
            start_time: 0,
            ring: Mutex::new(RingState::new()),
            notify: Notify::new(),
            child: Mutex::new(None),
            master: Mutex::new(None),
            writer_tx: Mutex::new(None),
            shell_started: AtomicBool::new(false),
            exited_at: Mutex::new(None),
            unattached_since: Mutex::new(Some(Instant::now())),
            #[cfg(windows)]
            job: Mutex::new(None),
        }
    }

    /// A fake `Read` that returns a scripted sequence of chunks, then `Ok(0)`
    /// (EOF) once the script is exhausted - enough to drive all three
    /// required carry-over cases (mid-codepoint split, EOF truncation,
    /// malformed interior byte) without a real PTY.
    struct ScriptedReader {
        chunks: VecDeque<Vec<u8>>,
    }

    impl ScriptedReader {
        fn new(chunks: Vec<Vec<u8>>) -> Self {
            Self { chunks: chunks.into_iter().collect() }
        }
    }

    impl Read for ScriptedReader {
        fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
            match self.chunks.pop_front() {
                Some(chunk) => {
                    assert!(chunk.len() <= buf.len(), "test chunk must fit the scratch buffer");
                    buf[..chunk.len()].copy_from_slice(&chunk);
                    Ok(chunk.len())
                }
                None => Ok(0),
            }
        }
    }

    fn backfill_text(shared: &SharedState) -> String {
        shared
            .ring
            .lock()
            .expect("ring lock poisoned")
            .backfill_after(0)
            .into_iter()
            .map(|chunk| chunk.data)
            .collect()
    }

    #[test]
    fn split_mid_codepoint_reassembles_exactly() {
        // "안녕" (3-byte codepoints each) + "🎉" (4-byte codepoint) = 10
        // bytes total. Split after byte 8 - inside the emoji's 4-byte
        // sequence (2 bytes present, 2 pending) - so the first read ends
        // mid-codepoint exactly like a real PTY read boundary can.
        let original = "안녕🎉";
        let bytes = original.as_bytes();
        assert_eq!(bytes.len(), 10, "fixture assumption: 3 + 3 + 4 bytes");
        let chunk1 = bytes[0..8].to_vec();
        let chunk2 = bytes[8..10].to_vec();

        let shared = Arc::new(shared_state_for_test());
        let handle = spawn_reader_thread(shared.clone(), Box::new(ScriptedReader::new(vec![chunk1, chunk2])));
        handle.join().expect("reader thread must not panic on a mid-codepoint split");

        let reassembled = backfill_text(&shared);
        assert_eq!(reassembled, original, "split-then-carried bytes must reassemble byte-for-byte");
        assert!(
            !reassembled.contains('\u{FFFD}'),
            "a genuine read-boundary split must never surface a replacement character: {reassembled:?}"
        );
    }

    #[test]
    fn eof_truncated_tail_degrades_to_lossy_without_hanging() {
        // "A" + the first 2 of "🎉"'s 4 bytes, then EOF with no further
        // chunk - a sequence that is truncated at the TRUE end of the
        // stream, not merely split across reads.
        //
        // NOTE: this is a no-hang/no-wedge guard, NOT a carry-over
        // regression guard - a truly EOF-truncated sequence is lossily
        // replaced by the OLD per-read `from_utf8_lossy` implementation
        // too, so this test's assertions pass unchanged against the old
        // code. `split_codepoint_across_reads_then_clean_eof_reassembles_
        // without_replacement` below is the test that actually fails
        // against the old implementation for the "split, then EOF" shape.
        let emoji_bytes = "🎉".as_bytes();
        let mut chunk = b"A".to_vec();
        chunk.extend_from_slice(&emoji_bytes[..2]);

        let shared = Arc::new(shared_state_for_test());
        let handle = spawn_reader_thread(shared.clone(), Box::new(ScriptedReader::new(vec![chunk])));
        // A hang here would fail via the test harness's own timeout - no
        // extra polling/timeout plumbing needed.
        handle.join().expect("reader thread must not panic on EOF truncation");

        let reassembled = backfill_text(&shared);
        assert!(reassembled.starts_with('A'), "the valid prefix before the truncated tail must survive: {reassembled:?}");
        assert!(
            reassembled.contains('\u{FFFD}'),
            "a sequence truncated at true EOF must degrade to lossy replacement: {reassembled:?}"
        );
    }

    #[test]
    fn split_codepoint_across_reads_then_clean_eof_reassembles_without_replacement() {
        // "X" + "한" (3-byte codepoint) + "Y", with the FIRST read cutting
        // "한" after only 2 of its 3 bytes and the SECOND read supplying
        // the completing byte plus the trailing "Y", followed immediately
        // by a clean EOF (no further chunk). This is the carry-then-
        // complete-at-EOF shape: unlike the truncation test above, the
        // sequence genuinely finishes, so the OLD per-read
        // `from_utf8_lossy` implementation (which sees only "X" + 2
        // dangling lead bytes in read 1, then a lone continuation byte +
        // "Y" in read 2) corrupts this into replacement characters even
        // though every byte is present and valid once reassembled -
        // reverting the carry-over would fail this test's exact-equality
        // assertion.
        let original = "X한Y";
        let bytes = original.as_bytes();
        assert_eq!(bytes.len(), 5, "fixture assumption: 1 + 3 + 1 bytes");
        let chunk1 = bytes[0..3].to_vec(); // "X" + first 2 bytes of "한"
        let chunk2 = bytes[3..5].to_vec(); // completing byte of "한" + "Y"

        let shared = Arc::new(shared_state_for_test());
        let handle = spawn_reader_thread(shared.clone(), Box::new(ScriptedReader::new(vec![chunk1, chunk2])));
        handle
            .join()
            .expect("reader thread must not panic on a split-then-clean-EOF codepoint");

        let reassembled = backfill_text(&shared);
        assert_eq!(
            reassembled, original,
            "a codepoint split across reads must reassemble exactly once the completing read \
             arrives, even when that read is immediately followed by a clean EOF: {reassembled:?}"
        );
        assert!(
            !reassembled.contains('\u{FFFD}'),
            "carry survives to a real completion here - there must be no replacement character: {reassembled:?}"
        );
    }

    #[test]
    fn malformed_interior_byte_yields_exactly_one_replacement_and_does_not_wedge() {
        // A valid multi-byte codepoint ("한", 3 bytes) straddles the read
        // boundary (read 1 ends after 2 of its bytes), read 2 supplies the
        // completing byte, then a single invalid lead byte (0xFF is never
        // valid UTF-8, so `error_len() == Some(1)`), then more valid text -
        // all still within read 2. This exercises BOTH the carry-then-
        // complete path AND the malformed-span-does-not-wedge path in one
        // shot: the OLD per-read `from_utf8_lossy` implementation sees "한"
        // as two dangling/lone bytes across two independent reads and
        // produces several replacement characters instead of exactly one,
        // so the exact-string assertion below fails against the old code.
        let mut chunk1 = b"before".to_vec();
        chunk1.extend_from_slice(&"한".as_bytes()[..2]); // incomplete: 2 of 3 bytes

        let mut chunk2 = vec!["한".as_bytes()[2]]; // completing byte of "한"
        chunk2.push(0xFF); // malformed, not a split - error_len() == Some(1)
        chunk2.extend_from_slice(b"after");

        let shared = Arc::new(shared_state_for_test());
        let handle = spawn_reader_thread(shared.clone(), Box::new(ScriptedReader::new(vec![chunk1, chunk2])));
        handle.join().expect("reader thread must not panic on a malformed interior byte");

        let reassembled = backfill_text(&shared);
        let expected = "before한\u{FFFD}after";
        assert_eq!(
            reassembled, expected,
            "the straddling codepoint must reassemble intact, exactly one U+FFFD must stand in \
             for the malformed byte, and no text may be dropped, duplicated, or reordered: \
             {reassembled:?}"
        );
        assert_eq!(
            reassembled.matches('\u{FFFD}').count(),
            1,
            "exactly one replacement character for the single malformed byte: {reassembled:?}"
        );
    }
}

#[cfg(test)]
mod spawn_shell_command_tests {
    use super::*;

    fn args_fixture() -> TerminalHelperArgs {
        TerminalHelperArgs {
            registry_dir: std::path::PathBuf::from("/tmp/registry"),
            terminal_id: "term_abc".to_owned(),
            work_root_id: "wr1".to_owned(),
            cwd: std::path::PathBuf::from("/tmp/cwd"),
            cwd_hint: None,
            title: "title".to_owned(),
            columns: 80,
            rows: 24,
            socket_path: std::path::PathBuf::from("/tmp/term_abc.sock"),
            command: None,
            command_args: Vec::new(),
            env_overlay: Vec::new(),
            scrub_marker: Vec::new(),
        }
    }

    #[test]
    fn spawn_shell_default_no_command_matches_existing_behaviour() {
        let args = args_fixture();
        let command = build_shell_command(&args, "xterm-256color".to_owned());

        assert_eq!(
            command.get_argv().first(),
            Some(&std::ffi::OsString::from(crate::terminal::default_shell()))
        );
        assert_eq!(
            command.get_cwd().map(|cwd| cwd.to_string_lossy().into_owned()),
            Some(args.cwd.to_string_lossy().into_owned())
        );
        let extra_env: Vec<&str> = command.iter_extra_env_as_str().map(|(key, _value)| key).collect();
        assert_eq!(
            extra_env,
            vec!["TERM"],
            "no env manipulation beyond TERM must have run on the default path: {extra_env:?}"
        );
        // CONTRACT (review cycle 1, finding T1): `iter_extra_env_as_str()`
        // alone cannot distinguish "no env method ever called" from
        // "env_clear() called with nothing re-added but TERM" -
        // `env_clear()` wipes the ENTIRE internal env map, base-flagged
        // entries included, but `iter_extra_env_as_str()` only ever reports
        // non-base-flagged entries either way, so both cases produce the
        // identical `["TERM"]` result above (verified empirically against
        // portable-pty 0.8.1 - see review finding T1). A known real
        // base-env value (`PATH`, present in any dev/CI process) surviving
        // is the actual proof: an accidental `env_clear()` on this branch
        // wipes it along with everything else; a correct no-manipulation
        // default path leaves it untouched.
        assert!(
            command.get_env("PATH").is_some(),
            "default path must preserve the real base env (e.g. PATH); its absence means \
             something cleared the base env on this branch"
        );
    }

    #[test]
    fn spawn_shell_explicit_command_forwards_argv_cwd_term_and_non_marker_overlay() {
        let mut args = args_fixture();
        args.command = Some("printf".to_owned());
        args.command_args = vec!["hi".to_owned()];
        args.env_overlay = vec![("FOO".to_owned(), "bar".to_owned())];

        let command = build_shell_command(&args, "xterm-256color".to_owned());

        assert_eq!(
            command.get_argv(),
            &vec![std::ffi::OsString::from("printf"), std::ffi::OsString::from("hi")]
        );
        assert_eq!(
            command.get_cwd().map(|cwd| cwd.to_string_lossy().into_owned()),
            Some(args.cwd.to_string_lossy().into_owned())
        );
        assert_eq!(
            command.get_env("TERM").map(|value| value.to_string_lossy().into_owned()),
            Some("xterm-256color".to_owned())
        );
        assert_eq!(
            command.get_env("FOO").map(|value| value.to_string_lossy().into_owned()),
            Some("bar".to_owned())
        );
    }

    #[test]
    fn apply_scrub_and_overlay_removes_markers_preserves_unrelated_vars_and_scrub_wins_over_colliding_overlay(
    ) {
        // Markers are seeded as explicit `.env()` entries rather than via an
        // injected "host env" iterable: `CommandBuilder::new()`'s base env
        // is seeded from this test process's own real environment (which
        // does not carry Claude markers), and per-marker `env_remove`
        // behaves identically against base-flagged and explicit entries
        // (both are stored in the same internal map, see
        // `portable_pty::cmdbuilder::EnvEntry`), so this exercises the same
        // removal codepath `build_shell_command`'s `Some` branch runs
        // against a real dirty inherited env.
        let markers: Vec<String> = crate::agent_env_profile::CLAUDE
            .markers
            .iter()
            .map(|marker| (*marker).to_owned())
            .collect();
        let mut command = CommandBuilder::new("printf");
        for marker in &markers {
            command.env(marker, "marker-value");
        }
        command.env("PATH", "/usr/bin:/bin");
        // T2: an arbitrary non-marker key that no plausible hand-rolled
        // allowlist would think to include - closes the gap where a narrow
        // allowlist that happens to enumerate PATH would otherwise still
        // pass this test.
        command.env("SOME_OTHER_VAR", "keep-me");

        // T3 (LEAD DECISION: scrub wins): one overlay pair targets a scrub
        // marker directly, alongside an ordinary non-conflicting pair.
        let overlay = vec![
            ("FOO".to_owned(), "bar".to_owned()),
            ("CLAUDECODE".to_owned(), "resurrected".to_owned()),
        ];
        apply_scrub_and_overlay(&mut command, &markers, &overlay);

        for marker in &markers {
            assert_eq!(
                command.get_env(marker),
                None,
                "marker {marker} must stay scrubbed even when the overlay tries to set it"
            );
        }
        assert_eq!(
            command.get_env("PATH").map(|value| value.to_string_lossy().into_owned()),
            Some("/usr/bin:/bin".to_owned()),
            "deny-list, not allowlist - PATH must survive"
        );
        assert_eq!(
            command.get_env("SOME_OTHER_VAR").map(|value| value.to_string_lossy().into_owned()),
            Some("keep-me".to_owned()),
            "deny-list, not allowlist - an arbitrary non-marker key must survive too (T2)"
        );
        assert_eq!(
            command.get_env("FOO").map(|value| value.to_string_lossy().into_owned()),
            Some("bar".to_owned())
        );
    }

    // CONTRACT (review cycle 1, finding C1, non-vacuity): mirrors
    // `terminal.rs`'s
    // `helper_spawn_with_command_uses_the_supplied_scrub_profile_not_a_hardcoded_one`
    // at hop 2. Proves `apply_scrub_and_overlay` (the function
    // `build_shell_command`'s `Some` branch calls) scrubs the
    // CALLER-SUPPLIED `markers` list rather than a profile-blind hardcoded
    // `CLAUDE` - a synthetic marker absent from `CLAUDE`'s list is scrubbed,
    // while a real Claude marker (not in the synthetic list) survives,
    // which a hardcoded-`CLAUDE` regression would get backwards.
    #[test]
    fn apply_scrub_and_overlay_uses_the_supplied_marker_list_not_a_hardcoded_claude_one() {
        let synthetic_markers = vec!["SYNTHETIC_MARKER_ONLY".to_owned()];
        let mut command = CommandBuilder::new("printf");
        command.env("SYNTHETIC_MARKER_ONLY", "scrub-me");
        command.env("CLAUDECODE", "marker-value");
        command.env("PATH", "/usr/bin:/bin");

        apply_scrub_and_overlay(&mut command, &synthetic_markers, &[]);

        assert_eq!(
            command.get_env("SYNTHETIC_MARKER_ONLY"),
            None,
            "the supplied synthetic profile's own marker must be scrubbed at hop 2"
        );
        assert!(
            command.get_env("CLAUDECODE").is_some(),
            "a hardcoded-CLAUDE regression would scrub this even though the \
             supplied profile never lists it - CLAUDECODE surviving proves \
             the caller-supplied marker list is what actually ran, not CLAUDE"
        );
        assert_eq!(
            command.get_env("PATH").map(|value| value.to_string_lossy().into_owned()),
            Some("/usr/bin:/bin".to_owned())
        );
    }
}

// Regression coverage for 260729 (helper liveness probe). These drive the
// REAL `serve_connections` accept loop over a REAL Unix domain socket, not a
// scripted stand-in: the load-bearing claim of this phase is that a probe is
// answerable WHILE A SESSION IS ATTACHED, and nothing short of running the
// actual loop can show that. A new message kind alone would not have been
// enough - the loop awaited `handle_connection` inline and never polled
// `accept()` while a session was attached, so a probe would have sat unread
// in the backlog, which is precisely today's failure.
#[cfg(all(test, unix))]
mod liveness_probe_tests {
    use super::*;
    use crate::terminal_helper_ipc::write_ndjson;
    use std::path::PathBuf;

    fn shared_state_for_test() -> SharedState {
        SharedState {
            pid: 0,
            start_time: 0,
            ring: Mutex::new(RingState::new()),
            notify: Notify::new(),
            child: Mutex::new(None),
            master: Mutex::new(None),
            writer_tx: Mutex::new(None),
            shell_started: AtomicBool::new(false),
            exited_at: Mutex::new(None),
            unattached_since: Mutex::new(Some(Instant::now())),
            #[cfg(windows)]
            job: Mutex::new(None),
        }
    }

    /// Short by necessity: a Unix domain socket's sun_path is capped at ~108
    /// bytes, and these paths are bound for real.
    fn socket_path_for_test(label: &str) -> PathBuf {
        static COUNTER: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);
        let unique = COUNTER.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        std::env::temp_dir().join(format!("wsd-hp-{label}-{}-{unique}.sock", std::process::id()))
    }

    fn args_for_test(socket_path: &std::path::Path) -> TerminalHelperArgs {
        TerminalHelperArgs {
            registry_dir: std::env::temp_dir().join("wsd-hp-registry-unused"),
            terminal_id: "term_probe".to_owned(),
            work_root_id: "wr1".to_owned(),
            cwd: std::env::temp_dir(),
            cwd_hint: None,
            title: "probe".to_owned(),
            columns: 80,
            rows: 24,
            socket_path: socket_path.to_path_buf(),
            command: None,
            command_args: Vec::new(),
            env_overlay: Vec::new(),
            scrub_marker: Vec::new(),
        }
    }

    /// Opens a daemon-side connection, sends one `LivenessProbe`, and returns
    /// the helper's report. Skips past anything the helper writes first: an
    /// UNATTACHED helper answers from inside its ordinary session dispatch,
    /// which opens with `Handshake` + `Status`, exactly as the daemon-side
    /// `probe_helper` has to cope with.
    async fn probe(socket_path: &std::path::Path) -> (bool, Option<u64>) {
        let stream = crate::terminal_ipc_transport::connect(socket_path)
            .await
            .expect("probe connect");
        let (read_half, mut write_half) = crate::terminal_ipc_transport::split(stream);
        let mut reader = NdjsonReader::new(read_half);
        write_ndjson(&mut write_half, &DaemonToHelperMessage::LivenessProbe)
            .await
            .expect("write probe");
        loop {
            let message = tokio::time::timeout(
                Duration::from_secs(5),
                reader.read_message::<HelperToDaemonMessage>(),
            )
            .await
            .expect("probe must be answered without hanging")
            .expect("probe read must not error")
            .expect("probe must be answered before EOF");
            if let HelperToDaemonMessage::LivenessProbeResponse {
                attached,
                unattached_for_ms,
            } = message
            {
                return (attached, unattached_for_ms);
            }
        }
    }

    // The load-bearing test of the whole helper-side change: a probe arriving
    // while a session is attached must be answered. Revert the concurrent
    // accept arm (`serve_session`) and this hangs on `probe`'s own timeout,
    // because the probe connection sits in the listener backlog until the
    // attached daemon disconnects - which is the exact behaviour that made a
    // healthy, busy helper look dead.
    #[tokio::test]
    async fn a_probe_is_answered_while_a_session_is_attached() {
        let socket_path = socket_path_for_test("attached");
        let _ = std::fs::remove_file(&socket_path);
        let mut listener = IpcListener::bind(&socket_path).expect("bind helper listener");
        let shared = Arc::new(shared_state_for_test());
        // Pre-flip `shell_started` so the `HandshakeAck` below marks this
        // connection attached WITHOUT spawning a real shell - the attach
        // bookkeeping is what is under test here, not the PTY.
        shared.shell_started.store(true, Ordering::SeqCst);
        let args = args_for_test(&socket_path);

        let serve_shared = shared.clone();
        let serve = tokio::spawn(async move {
            let _ = serve_connections(&args, &mut listener, &serve_shared, HelperShape::CURRENT).await;
        });

        // Connection 1: a daemon attaches and STAYS attached.
        let stream = crate::terminal_ipc_transport::connect(&socket_path)
            .await
            .expect("session connect");
        let (read_half, mut session_write) = crate::terminal_ipc_transport::split(stream);
        let mut session_reader = NdjsonReader::new(read_half);
        for _ in 0..2 {
            session_reader
                .read_message::<HelperToDaemonMessage>()
                .await
                .expect("handshake read")
                .expect("handshake present");
        }
        write_ndjson(&mut session_write, &DaemonToHelperMessage::HandshakeAck)
            .await
            .expect("write handshake ack");
        // Let the helper process the ack before probing, so `attached: true`
        // is a real observation rather than a race.
        for _ in 0..100 {
            if shared.liveness_report().0 {
                break;
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }

        // Connection 2, CONCURRENT with the still-open session above.
        let (attached, unattached_for_ms) = probe(&socket_path).await;
        assert!(
            attached,
            "a helper whose session is held by another daemon must answer that it is IN USE - \
             this is the fact that stops a second daemon killing it"
        );
        assert_eq!(
            unattached_for_ms, None,
            "there is no unattached duration to report while attached"
        );

        // And the session it was serving must be undisturbed by the probe.
        drop(session_write);
        drop(session_reader);
        serve.abort();
        let _ = std::fs::remove_file(&socket_path);
    }

    // The other half of the three-way predicate, and the leg a naive
    // "kill only when the probe does not answer" rule leaks forever: an
    // UNATTACHED helper still answers, and must report how long it has been
    // unattached so the daemon can tell a restart from a real orphan.
    #[tokio::test]
    async fn a_probe_on_an_unattached_helper_reports_its_unattached_duration() {
        let socket_path = socket_path_for_test("idle");
        let _ = std::fs::remove_file(&socket_path);
        let mut listener = IpcListener::bind(&socket_path).expect("bind helper listener");
        let shared = Arc::new(shared_state_for_test());
        shared.shell_started.store(true, Ordering::SeqCst);
        let args = args_for_test(&socket_path);

        let serve_shared = shared.clone();
        let serve = tokio::spawn(async move {
            let _ = serve_connections(&args, &mut listener, &serve_shared, HelperShape::CURRENT).await;
        });

        tokio::time::sleep(Duration::from_millis(60)).await;
        let (attached, unattached_for_ms) = probe(&socket_path).await;

        assert!(!attached, "nobody has attached to this helper");
        let elapsed = unattached_for_ms.expect("an unattached helper must report a duration");
        assert!(
            elapsed >= 50,
            "the duration must be measured from the helper's own clock, not reported as zero: \
             {elapsed}ms"
        );

        serve.abort();
        let _ = std::fs::remove_file(&socket_path);
    }

    // CONTRACT (260729): a PROBE must never reset the unattached clock. If it
    // did, a daemon probing an orphan every 10s would keep the orphan
    // permanently within grace and it would become unreclaimable - the
    // opposite failure to the one this ticket fixes, and a silent one.
    #[tokio::test]
    async fn probing_never_resets_the_unattached_clock() {
        let socket_path = socket_path_for_test("noreset");
        let _ = std::fs::remove_file(&socket_path);
        let mut listener = IpcListener::bind(&socket_path).expect("bind helper listener");
        let shared = Arc::new(shared_state_for_test());
        shared.shell_started.store(true, Ordering::SeqCst);
        let args = args_for_test(&socket_path);

        let serve_shared = shared.clone();
        let serve = tokio::spawn(async move {
            let _ = serve_connections(&args, &mut listener, &serve_shared, HelperShape::CURRENT).await;
        });

        tokio::time::sleep(Duration::from_millis(60)).await;
        let (_, first) = probe(&socket_path).await;
        tokio::time::sleep(Duration::from_millis(60)).await;
        let (_, second) = probe(&socket_path).await;

        let first = first.expect("first probe reports a duration");
        let second = second.expect("second probe reports a duration");
        assert!(
            second > first,
            "the unattached clock must keep running across probes ({first}ms then {second}ms) - \
             a probe that resets it makes every orphan permanently unreclaimable"
        );

        serve.abort();
        let _ = std::fs::remove_file(&socket_path);
    }

    // CONTRACT (260729): the clock RESTARTS when an attached daemon goes
    // away. `exited_at` cannot serve this purpose - it is set only when the
    // SHELL exits - and before this ticket the daemon-disconnect arm of the
    // read loop stored nothing at all, which is why the predicate had no
    // duration to work with.
    #[tokio::test]
    async fn the_unattached_clock_restarts_when_an_attached_daemon_disconnects() {
        let shared = Arc::new(shared_state_for_test());
        assert!(
            !shared.liveness_report().0,
            "a helper starts life unattached"
        );

        shared.mark_attached();
        assert_eq!(
            shared.liveness_report(),
            (true, None),
            "an attached helper reports no unattached duration"
        );

        shared.mark_unattached();
        let (attached, elapsed) = shared.liveness_report();
        assert!(!attached, "the helper is unattached again once its daemon left");
        assert!(
            elapsed.is_some(),
            "and the clock must be running from the disconnect, not left at None"
        );
    }

    // CONTRACT (260729): the capability declaration is what every daemon-side
    // kill site reads BEFORE it puts a byte on this helper's wire. Reverting
    // it to `false`/absent silently disables the entire three-way predicate:
    // every kill site would then treat this helper as one that predates the
    // probe and leave it alone forever, including when it is a genuine
    // orphan.
    #[test]
    fn the_startup_registry_entry_declares_the_liveness_probe_capability() {
        let args = args_for_test(std::path::Path::new("/tmp/wsd-hp-decl.sock"));
        let entry = startup_registry_entry(&args, 4242, 99, HelperShape::CURRENT);

        assert!(
            entry.supports_liveness_probe,
            "this binary answers LivenessProbe, so its registry entry must say so - otherwise \
             no daemon will ever probe it and it can never be reclaimed"
        );
        assert_eq!(entry.pid, 4242);
        assert_eq!(entry.start_time, 99);
        assert_eq!(entry.terminal_id, args.terminal_id);
        assert_eq!(entry.socket_path, args.socket_path);
    }
}

// Regression coverage for 260729 review findings F9/F10, driven by a REAL
// helper: `run_terminal_helper` itself, with its real registry write, real
// listener, real PTY and a real shell child. The existing
// `liveness_probe_tests` above drive `serve_connections` against a
// hand-built `SharedState` with `shell_started` pre-flipped and no PTY at
// all, which is exactly why they cannot see these defects - there is no
// shell for a mis-fired `kill_shell_if_running()` to kill, no registry entry
// for the exit path to delete, and every fixture write is `let _ = ...`, so
// a write error is unobservable by construction.
#[cfg(all(test, unix))]
pub(crate) mod real_helper_test_support {
    use super::*;
    use crate::terminal_helper_ipc::write_ndjson;
    use crate::terminal_ipc_transport::{IpcReadHalf, IpcWriteHalf};
    use std::path::PathBuf;

    /// A live `run_terminal_helper` task plus the on-disk state it owns.
    pub(crate) struct RealHelper {
        pub(crate) state_dir: PathBuf,
        pub(crate) registry_dir: PathBuf,
        pub(crate) terminal_id: String,
        pub(crate) socket_path: PathBuf,
        task: tokio::task::JoinHandle<anyhow::Result<()>>,
    }

    /// A daemon-side connection to a `RealHelper`, split the way the daemon
    /// splits it.
    pub(crate) struct DaemonConnection {
        pub(crate) reader: NdjsonReader<IpcReadHalf>,
        pub(crate) writer: IpcWriteHalf,
    }

    impl DaemonConnection {
        pub(crate) async fn read(&mut self) -> HelperToDaemonMessage {
            tokio::time::timeout(
                Duration::from_secs(10),
                self.reader.read_message::<HelperToDaemonMessage>(),
            )
            .await
            .expect("helper must answer without hanging")
            .expect("helper read must not error")
            .expect("helper must not EOF mid-conversation")
        }
    }

    impl RealHelper {
        /// Spawns the real helper with `sh -c <shell_command>` as its shell.
        /// Returns once the helper has durably written its registry entry and
        /// bound its listener, so no caller can race the bind.
        pub(crate) async fn spawn(label: &str, shell_command: &str) -> Self {
            Self::spawn_shaped(label, shell_command, HelperShape::CURRENT).await
        }

        /// CONTRACT (260729 review round 3, finding F): the same real helper,
        /// spawned in PRE-UPGRADE shape - no peer-fault downgrade, no
        /// concurrent probe arm, and `supportsLivenessProbe` absent from its
        /// registry entry. This is the population every compatibility claim in
        /// this ticket is about, and before this seam existed it was untested
        /// by construction: every real-helper test ran the current binary,
        /// which already has the fixes.
        pub(crate) async fn spawn_legacy(label: &str, shell_command: &str) -> Self {
            Self::spawn_shaped(label, shell_command, HelperShape::LEGACY).await
        }

        pub(crate) async fn spawn_shaped(
            label: &str,
            shell_command: &str,
            shape: HelperShape,
        ) -> Self {
            // Short by necessity: `socket_path` is bound as a real Unix
            // domain socket (sun_path caps at ~108 bytes).
            static COUNTER: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);
            let unique = COUNTER.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
            let state_dir = std::env::temp_dir()
                .join(format!("wsd-rh-{label}-{}-{unique}", std::process::id()));
            let registry_dir = state_dir.join("terminals");
            std::fs::create_dir_all(&registry_dir).expect("create registry dir");
            let terminal_id = format!("t_{label}");
            let socket_path = registry_dir.join(format!("{terminal_id}.sock"));

            let args = TerminalHelperArgs {
                registry_dir: registry_dir.clone(),
                terminal_id: terminal_id.clone(),
                work_root_id: "wr-real".to_owned(),
                cwd: std::env::temp_dir(),
                cwd_hint: None,
                title: "real helper".to_owned(),
                columns: 80,
                rows: 24,
                socket_path: socket_path.clone(),
                command: Some("sh".to_owned()),
                command_args: vec!["-c".to_owned(), shell_command.to_owned()],
                env_overlay: Vec::new(),
                scrub_marker: Vec::new(),
            };
            let task = tokio::spawn(run_terminal_helper_shaped(args, shape));

            let helper = Self {
                state_dir,
                registry_dir,
                terminal_id,
                socket_path,
                task,
            };
            for _ in 0..200 {
                if helper.entry_exists() && helper.socket_path.exists() {
                    return helper;
                }
                tokio::time::sleep(Duration::from_millis(10)).await;
            }
            panic!("real helper never wrote its registry entry and bound its socket");
        }

        pub(crate) fn entry_exists(&self) -> bool {
            crate::terminal_registry_file::registry_entry_path(&self.registry_dir, &self.terminal_id)
                .exists()
        }

        pub(crate) fn is_running(&self) -> bool {
            !self.task.is_finished()
        }

        pub(crate) fn entry(&self) -> Option<TerminalRegistryEntry> {
            crate::terminal_registry_file::read_registry_entry(&self.registry_dir, &self.terminal_id)
        }

        /// Waits for the helper task to finish, up to a bounded budget.
        /// `true` means it exited - which for a legacy-shaped helper means its
        /// exit path ran `kill_shell_if_running()` + `delete_registry_entry()`.
        pub(crate) async fn wait_for_exit(&self) -> bool {
            for _ in 0..200 {
                if !self.is_running() {
                    return true;
                }
                tokio::time::sleep(Duration::from_millis(25)).await;
            }
            false
        }

        pub(crate) async fn connect(&self) -> DaemonConnection {
            let stream = crate::terminal_ipc_transport::connect(&self.socket_path)
                .await
                .expect("connect to real helper");
            let (read_half, writer) = crate::terminal_ipc_transport::split(stream);
            DaemonConnection {
                reader: NdjsonReader::new(read_half),
                writer,
            }
        }

        /// Full adoption sequence: `Handshake` + `Status` in, `HandshakeAck`
        /// out - which is what spawns the shell.
        pub(crate) async fn attach(&self) -> DaemonConnection {
            let mut connection = self.connect().await;
            assert!(
                matches!(connection.read().await, HelperToDaemonMessage::Handshake { .. }),
                "a real helper opens every connection with its Handshake"
            );
            let status = connection.read().await;
            assert!(
                matches!(status, HelperToDaemonMessage::Status { .. }),
                "and follows it with a Status: {status:?}"
            );
            write_ndjson(&mut connection.writer, &DaemonToHelperMessage::HandshakeAck)
                .await
                .expect("write handshake ack");
            connection
        }

        /// Connects and drops immediately without reading a byte - exactly
        /// what a daemon-side reachability check used to do to a helper whose
        /// registry entry declared no probe capability.
        pub(crate) async fn connect_and_drop(&self) {
            let stream = crate::terminal_ipc_transport::connect(&self.socket_path)
                .await
                .expect("connect to real helper");
            drop(stream);
        }

        /// Ends the helper the way a daemon does, so the real exit path runs
        /// and the shell child is reaped rather than leaked into the test
        /// process.
        pub(crate) async fn shutdown(self) {
            if self.is_running() {
                if let Ok(stream) = crate::terminal_ipc_transport::connect(&self.socket_path).await {
                    let (_read_half, mut writer) = crate::terminal_ipc_transport::split(stream);
                    let _ = write_ndjson(&mut writer, &DaemonToHelperMessage::GracefulShutdown).await;
                }
            }
            let _ = tokio::time::timeout(Duration::from_secs(5), self.task).await;
            let _ = std::fs::remove_dir_all(&self.state_dir);
        }
    }

    /// A shell that keeps producing output forever, so "is the shell still
    /// alive?" can be answered by observing NEW output rather than by
    /// inferring it from the helper process still existing.
    pub(crate) const TICKING_SHELL: &str =
        "i=0; while true; do echo TICK$i; i=$((i+1)); sleep 0.1; done";

    /// Reads until an `Output` chunk with a sequence strictly greater than
    /// `after` arrives, returning its sequence. Panics on timeout, which is
    /// the assertion: no new output means the shell is gone.
    pub(crate) async fn read_output_past(connection: &mut DaemonConnection, after: u64) -> u64 {
        let deadline = Instant::now() + Duration::from_secs(10);
        loop {
            assert!(Instant::now() < deadline, "no new output arrived from the shell");
            if let HelperToDaemonMessage::Output(chunk) = connection.read().await {
                if chunk.sequence > after {
                    return chunk.sequence;
                }
            }
        }
    }
}

#[cfg(all(test, unix))]
mod real_helper_peer_loss_tests {
    use super::real_helper_test_support::*;
    use super::*;

    // CONTRACT (260729 review finding F9-root, and the invariant the whole
    // ticket rests on): A PEER GOING AWAY MUST NEVER KILL THE SHELL.
    //
    // This is the exact shape of the silent destruction the review found: a
    // daemon connects to decide whether the helper is reachable and drops the
    // connection without reading. The helper accepts, writes its `Handshake`
    // into an already-closed peer, gets EPIPE - and before this fix that `?`
    // unwound through `serve_connections` into `run_terminal_helper`'s exit
    // path, which runs `kill_shell_if_running()` + `delete_registry_entry()`.
    // The user's shell died and the entry that would have let anyone diagnose
    // it was erased in the same breath. The periodic sweep did this every 10s.
    //
    // Fails against bff07caf: there the reconnect below cannot even connect,
    // because the helper has already exited and unlinked its socket.
    #[tokio::test]
    async fn a_real_helper_survives_a_daemon_that_connects_and_drops_without_reading() {
        let helper = RealHelper::spawn("drop", TICKING_SHELL).await;

        // Attach once so the shell actually spawns, then leave - the helper
        // is now unattached with a live shell, which is precisely the state
        // the sweep finds another daemon's restarting terminal in.
        let mut session = helper.attach().await;
        let seen = read_output_past(&mut session, 0).await;
        drop(session);

        // The reachability check, three times over, exactly as the 10s sweep
        // would repeat it.
        for _ in 0..3 {
            helper.connect_and_drop().await;
            tokio::time::sleep(Duration::from_millis(50)).await;
        }

        assert!(
            helper.is_running(),
            "a helper must not exit because a peer connected and left"
        );
        assert!(
            helper.entry_exists(),
            "and it must not erase its own registry entry - without it the helper's real owner \
             can never adopt it again, so sparing the process alone still destroys the terminal"
        );

        // The shell itself, not merely the helper process: NEW output past
        // the sequence seen before the drops proves `kill_shell_if_running()`
        // never ran.
        let mut reattached = helper.attach().await;
        let fresh = read_output_past(&mut reattached, seen).await;
        assert!(
            fresh > seen,
            "the shell must still be producing output after the peer-gone writes"
        );
        drop(reattached);

        helper.shutdown().await;
    }

    // The same invariant stated as behaviour rather than as survival: after a
    // peer-gone write the helper must be back in its ACCEPT LOOP, able to
    // serve the next daemon in full. "Did not die" and "can still be adopted"
    // are separate properties - a helper that survived but stopped accepting
    // would pass the test above and still be lost to its owner.
    #[tokio::test]
    async fn a_peer_gone_write_returns_the_real_helper_to_its_accept_loop() {
        let helper = RealHelper::spawn("accept", TICKING_SHELL).await;
        let session = helper.attach().await;
        drop(session);

        for _ in 0..5 {
            helper.connect_and_drop().await;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;

        // A FULL adoption sequence, not merely a successful connect: the
        // helper must still write `Handshake` + `Status` and accept a
        // `HandshakeAck`.
        let mut session = helper.attach().await;
        let _ = read_output_past(&mut session, 0).await;
        drop(session);

        assert!(helper.is_running());
        assert!(helper.entry_exists());
        helper.shutdown().await;
    }

    // REPLACES `only_peer_gone_io_errors_are_downgraded`, which was wrong on
    // its face: it PINNED an allow-list of five `ErrorKind`s as intentional,
    // and asserted that `InvalidData` (a truncated line's invalid UTF-8) and
    // `Other` (any decode failure) stay fatal. Those are the two most common
    // shapes a SIGKILLed daemon leaves behind. It also guarded only the
    // too-WIDE direction - it could tell you the list had grown, never that
    // the list was short a kind, which is the failure that kills a shell.
    //
    // The replacement is built so it CAN catch a missing case, in two layers:
    // - the classification itself is by SOURCE, so there is no list to be
    //   short of (asserted directly below, including against `ErrorKind`s
    //   nobody enumerated);
    // - and the behavioural tests that follow drive REAL faults through a REAL
    //   helper and assert the shell survives, so a regression is caught by the
    //   outcome that actually matters rather than by a list membership.
    #[test]
    fn every_peer_connection_fault_ends_only_the_connection() {
        use crate::terminal_helper_ipc::PeerFault;
        use std::io::{Error, ErrorKind};

        // Deliberately includes the kinds the old allow-list rejected
        // (`InvalidData`, `Other`), the ones it never considered (`WriteZero`,
        // which is what `write_all` actually returns on a short write), and
        // kinds that have nothing to do with sockets at all - because the
        // point is that the KIND is not consulted.
        for kind in [
            ErrorKind::BrokenPipe,
            ErrorKind::ConnectionReset,
            ErrorKind::ConnectionAborted,
            ErrorKind::NotConnected,
            ErrorKind::UnexpectedEof,
            ErrorKind::InvalidData,
            ErrorKind::Other,
            ErrorKind::WriteZero,
            ErrorKind::TimedOut,
            ErrorKind::Interrupted,
            ErrorKind::PermissionDenied,
            ErrorKind::OutOfMemory,
            ErrorKind::InvalidInput,
            ErrorKind::NotFound,
        ] {
            assert!(
                write_fault_ends_only_the_connection(&WriteFault::Peer(Error::new(kind, "peer"))),
                "{kind:?} arrived on the PEER's connection, so it must end that connection and \
                 nothing else - an allow-list that omits it kills the user's shell"
            );
        }
        // Windows ERROR_OPERATION_ABORTED (995), which no ErrorKind folds
        // predictably and which the raw-code list did not carry either.
        assert!(
            write_fault_ends_only_the_connection(&WriteFault::Peer(Error::from_raw_os_error(995))),
            "a raw OS error on the peer's connection is still the peer's"
        );

        // Non-vacuity: the classification must still separate this process's
        // OWN failures, or it is a no-op with the same behaviour as deleting
        // the check.
        let serialize_failure =
            serde_json::to_string(&std::collections::BTreeMap::from([((1u8, 2u8), 3u8)]))
                .expect_err("a tuple-keyed map has no JSON form");
        assert!(
            !write_fault_ends_only_the_connection(&WriteFault::Serialize(serialize_failure)),
            "failing to serialize our OWN message is not the peer's doing and stays fatal"
        );

        // And the read side has no classification at all to get wrong: every
        // variant of `PeerFault` is, by its type, the peer's.
        let faults = [
            PeerFault::Io(Error::new(ErrorKind::Other, "io")),
            PeerFault::InvalidUtf8(Error::new(ErrorKind::InvalidData, "utf8")),
            PeerFault::Malformed(
                serde_json::from_str::<serde_json::Value>("{bad").expect_err("malformed"),
            ),
        ];
        for fault in &faults {
            assert!(
                matches!(
                    fault,
                    PeerFault::Io(_) | PeerFault::InvalidUtf8(_) | PeerFault::Malformed(_)
                ),
                "reading can only fail because of the peer: {fault:?}"
            );
        }
    }

    // The behavioural half, and the one that would have caught round 2's gap:
    // a REAL helper, an ATTACHED session, and the exact byte sequences a
    // SIGKILLed or newer daemon leaves on the wire. Each of these was fatal
    // after round 2 - the shell died and the registry entry was erased.
    #[tokio::test]
    async fn a_real_helper_survives_every_malformed_thing_a_peer_can_put_on_the_wire() {
        use tokio::io::AsyncWriteExt;

        // (label, bytes) - the four shapes named in review round 3.
        let payloads: [(&str, &[u8]); 4] = [
            // A daemon SIGKILLed mid-`write_all`: a truncated final line with
            // no newline, returned by `Lines::next_line()` at EOF. THE common
            // crash signature.
            ("truncated", br#"{"type":"input","data":"abc"#),
            // The same cut landing mid-codepoint.
            ("invalid-utf8", &[0x7b, 0xf0, 0x9f, 0x92]),
            // A complete but undecodable line.
            ("malformed", b"{not json at all}\n"),
            // A NEWER daemon sending a variant this build has never heard of.
            ("unknown-variant", b"{\"type\":\"somethingFromTheFuture\"}\n"),
        ];

        for (label, payload) in payloads {
            let helper = RealHelper::spawn(label, TICKING_SHELL).await;
            let mut session = helper.attach().await;
            let seen = read_output_past(&mut session, 0).await;

            // Same connection the session is on, so the fault lands in
            // `handle_connection`'s own read loop rather than on a probe task.
            session
                .writer
                .write_all(payload)
                .await
                .expect("write the malformed payload");
            session.writer.flush().await.expect("flush");
            drop(session);
            tokio::time::sleep(Duration::from_millis(100)).await;

            assert!(
                helper.is_running(),
                "[{label}] a peer's malformed line must end the CONNECTION, not the helper"
            );
            assert!(
                helper.entry_exists(),
                "[{label}] and must not erase the registry entry its owner adopts through"
            );

            // The shell itself: NEW output past the pre-fault sequence proves
            // `kill_shell_if_running()` never ran, and a full re-adoption
            // proves the helper went back to its accept loop.
            let mut reattached = helper.attach().await;
            let fresh = read_output_past(&mut reattached, seen).await;
            assert!(
                fresh > seen,
                "[{label}] the user's shell must still be producing output"
            );
            drop(reattached);
            helper.shutdown().await;
        }
    }

    // CONTRACT (260729 review round 3, finding F - the seam's fidelity, and
    // the reason it is worth having): this asserts what actually happens to a
    // PRE-UPGRADE helper, which is the population every compatibility claim in
    // this ticket is about and which no test in the tree could reach before.
    //
    // The assertions here are deliberately assertions of DAMAGE. They are the
    // instrument, not the goal: they are what makes the daemon-side "never
    // make contact with a capability-absent entry" rule verifiable as
    // load-bearing rather than merely stated. If a future change makes the
    // daemon connect to such a helper again, the daemon-side tests that lean
    // on this fixture go red instead of silently killing a user's shell.
    #[tokio::test]
    async fn a_legacy_shaped_helper_really_is_destroyed_by_what_a_daemon_used_to_do() {
        // 1. The on-disk declaration, which is what every daemon-side kill
        //    site reads before it puts a byte on the wire.
        let helper = RealHelper::spawn_legacy("legacy-entry", TICKING_SHELL).await;
        let entry = helper.entry().expect("a legacy helper still writes its entry");
        assert!(
            !entry.supports_liveness_probe,
            "a pre-upgrade entry has no capability field; absent decodes to false"
        );

        // 2. A bare connect-and-drop - exactly what a daemon-side reachability
        //    check does - kills it. The helper writes its `Handshake` into an
        //    already-closed peer, the EPIPE propagates, and
        //    `run_terminal_helper`'s exit path runs
        //    `kill_shell_if_running()` + `delete_registry_entry()`.
        let session = helper.attach().await;
        drop(session);
        helper.connect_and_drop().await;

        assert!(
            helper.wait_for_exit().await,
            "a pre-upgrade helper DOES die when a peer connects and leaves - if this ever \
             stops being true, the fixture has stopped modelling the at-risk population"
        );
        assert!(
            !helper.entry_exists(),
            "and it erases the entry that would have let anyone diagnose it"
        );
        helper.shutdown().await;

        // 3. And a probe is worse still: an unknown variant on that build is a
        //    decode failure, and a decode failure there is fatal. This is why
        //    `probe_helper` refuses to send one without the capability flag.
        let helper = RealHelper::spawn_legacy("legacy-probe", TICKING_SHELL).await;
        let mut session = helper.attach().await;
        let _ = read_output_past(&mut session, 0).await;
        write_ndjson(&mut session.writer, &DaemonToHelperMessage::LivenessProbe)
            .await
            .expect("write probe");
        assert!(
            helper.wait_for_exit().await,
            "sending the probe to a helper that predates it SIGKILLs the user's shell - \
             the capability gate is the only thing standing between the two"
        );
        drop(session);
        helper.shutdown().await;
    }

    // The other half of the legacy shape: with no concurrent accept arm, a
    // probe arriving while a session is attached is never even read - it sits
    // in the backlog. This is the "healthy, busy helper looks dead" signature
    // the whole ticket exists for, reproduced against a real helper rather
    // than argued from source.
    #[tokio::test]
    async fn a_legacy_shaped_helper_cannot_answer_anything_while_a_session_is_attached() {
        let helper = RealHelper::spawn_legacy("legacy-busy", TICKING_SHELL).await;
        let mut session = helper.attach().await;
        let _ = read_output_past(&mut session, 0).await;

        // A second connection, as a second daemon's `connect_and_handshake`
        // would open it. It connects (the listener is bound) and hears
        // nothing.
        let stream = crate::terminal_ipc_transport::connect(&helper.socket_path)
            .await
            .expect("a busy helper still ACCEPTS - that is the trap");
        let (read_half, _write_half) = crate::terminal_ipc_transport::split(stream);
        let mut reader = NdjsonReader::new(read_half);
        let silence = tokio::time::timeout(
            Duration::from_millis(500),
            reader.read_message::<HelperToDaemonMessage>(),
        )
        .await;
        assert!(
            silence.is_err(),
            "a busy pre-upgrade helper answers nothing at all - 'connected but silent' is a \
             HEALTHY signature, which is why it must never authorize a kill on its own"
        );

        // And it is still perfectly healthy underneath.
        assert!(helper.is_running());
        assert!(helper.entry_exists());
        drop(session);
        helper.shutdown().await;
    }

    // CONTRACT (260729 review round 3, finding B): an `accept()` failure ends
    // the accept, not the helper. Driven through the classifier rather than by
    // exhausting the machine's fd table, which no test may do.
    #[test]
    fn accept_failures_back_off_and_only_a_sustained_run_is_fatal() {
        use std::io::{Error, ErrorKind};

        let mut failures = AcceptFailures::new();
        // The peer-influenced kinds named in the finding, plus an arbitrary
        // one: none of them is consulted, exactly as in finding A.
        for kind in [
            ErrorKind::Other, // EMFILE/ENFILE have no stable ErrorKind
            ErrorKind::ConnectionAborted,
            ErrorKind::Interrupted,
            ErrorKind::PermissionDenied,
        ] {
            assert!(
                failures.record(&Error::new(kind, "accept")).is_some(),
                "{kind:?} on accept() must back off and continue, never kill the shell"
            );
        }
        // Backoff grows and is capped.
        let mut last = Duration::ZERO;
        for _ in 0..40 {
            let delay = failures
                .record(&Error::new(ErrorKind::Other, "accept"))
                .expect("still under budget");
            assert!(delay >= last || delay == ACCEPT_RETRY_BACKOFF_MAX);
            assert!(delay <= ACCEPT_RETRY_BACKOFF_MAX, "backoff must be capped");
            last = delay;
        }
        assert_eq!(last, ACCEPT_RETRY_BACKOFF_MAX);

        // Non-vacuity in both directions: a success resets the run, and only
        // an unbroken run reaches the fatal budget.
        failures.reset();
        for index in 1..MAX_CONSECUTIVE_ACCEPT_FAILURES {
            assert!(
                failures.record(&Error::new(ErrorKind::Other, "accept")).is_some(),
                "failure {index} of {MAX_CONSECUTIVE_ACCEPT_FAILURES} must not be fatal"
            );
        }
        assert!(
            failures.record(&Error::new(ErrorKind::Other, "accept")).is_none(),
            "a listener that has failed for the entire budget is permanently broken"
        );
    }

    // CONTRACT (260729 review finding F12): the probe arm's spawn is bounded,
    // and the bound is BACKPRESSURE rather than shedding - every probe still
    // gets answered. Dropping surplus connections would be worse than an
    // unbounded spawn: daemon-side, a dropped probe reads as
    // `ProbeVerdict::Unanswered`, which authorizes a SIGKILL.
    #[tokio::test]
    async fn every_probe_is_answered_even_past_the_concurrent_probe_bound() {
        use crate::terminal_helper_ipc::write_ndjson;

        let helper = RealHelper::spawn("bound", TICKING_SHELL).await;
        // Hold a session open so every probe below is served by the
        // CONCURRENT arm (the bounded one), not by `handle_connection`.
        let session = helper.attach().await;

        let probes = MAX_CONCURRENT_PROBE_CONNECTIONS * 3;
        let mut tasks = Vec::new();
        for _ in 0..probes {
            let socket_path = helper.socket_path.clone();
            tasks.push(tokio::spawn(async move {
                let stream = crate::terminal_ipc_transport::connect(&socket_path)
                    .await
                    .expect("probe connect");
                let (read_half, mut writer) = crate::terminal_ipc_transport::split(stream);
                let mut reader = NdjsonReader::new(read_half);
                write_ndjson(&mut writer, &DaemonToHelperMessage::LivenessProbe)
                    .await
                    .expect("write probe");
                loop {
                    let message = tokio::time::timeout(
                        Duration::from_secs(10),
                        reader.read_message::<HelperToDaemonMessage>(),
                    )
                    .await
                    .expect("a queued probe must still be answered, never dropped")
                    .expect("probe read")
                    .expect("probe must be answered before EOF");
                    if let HelperToDaemonMessage::LivenessProbeResponse { attached, .. } = message {
                        return attached;
                    }
                }
            }));
        }
        for task in tasks {
            assert!(
                task.await.expect("probe task"),
                "every probe must report the session as attached"
            );
        }

        drop(session);
        helper.shutdown().await;
    }
}
