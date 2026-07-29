use std::collections::{BTreeSet, HashMap, HashSet, VecDeque};
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
use tokio::io::AsyncWriteExt;
use tokio::sync::{watch, Mutex as AsyncMutex};
use tokio::time::interval;
use ws_dashboard_core::WorkRootId;

use crate::agent_attention::AttentionHub;
use crate::router::AppState;
use crate::terminal_helper_ipc::{write_ndjson, NdjsonReader};
use crate::terminal_helper_protocol::{
    DaemonToHelperMessage, HelperToDaemonMessage, TerminalHelperOutputChunk, TerminalHelperStatus,
};
use crate::terminal_ipc_transport::{IpcReadHalf, IpcWriteHalf};
use crate::terminal_reconcile::{
    boot_identity_verified, classify, probe_authorizes_reclaim, probe_verdict_from_report,
    IdentityStatus, IpcStatus, ProbeVerdict, ReconcileRow, UNATTACHED_GRACE,
};
use crate::terminal_registry_file::{
    delete_registry_entry, read_registry_entry, registry_entry_path, scan_registry_dir,
    TerminalRegistryEntry,
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

// CONTRACT (260729 review finding F10): how long the liveness probe waits for
// the NEXT line from a helper that has already accepted its connection.
// Deliberately separate from the connect budget above, and applied per
// message rather than as one deadline over the whole exchange - see
// `probe_helper_reachability` for why a shared connect-sized deadline
// SIGKILLs a healthy helper with a full scrollback.
//
// Sized against the HELPER's own bound, not against a connect: a helper drops
// a probe connection after `terminal_helper_process::PROBE_CONNECTION_TIMEOUT`
// (5s) of not being asked, so once this daemon has gone that long without a
// line, no amount of extra patience can produce an answer - the peer has
// already abandoned the connection or is genuinely wedged, which is exactly
// the case the `Unanswered` verdict is for. Equal to it rather than larger,
// so the two constants cannot drift into a window where the daemon is still
// hopefully waiting on a connection the helper has already closed;
// `probe_response_idle_timeout_is_sized_against_the_helpers_own_bound` pins
// the relationship.
pub(crate) const PROBE_RESPONSE_IDLE_TIMEOUT: Duration =
    crate::terminal_helper_process::PROBE_CONNECTION_TIMEOUT;

// CONTRACT (260726 Phase 1 correctness review I1): the registry-dir backstop
// must give the primary graceful self-exit path (helper observes the IPC
// EOF `close_ipc_connection` produces -> `kill_shell_if_running` ->
// `delete_registry_entry` -> unlink socket, typically sub-millisecond) a
// real chance to run before the backstop's own SIGKILL. Without this, the
// backstop can preempt sub-fix 3(a)'s designed teardown in the very same
// tick that produced the eviction - and can even SIGKILL a helper whose
// shell is still alive (an IPC-error-only eviction via `mark_ipc_closed`).
// Ids evicted by `sweep_evict_expired`/`insert`'s lazy eviction are held out
// of the backstop's kill-eligible set for this long: generous relative to
// any realistic self-exit latency, but well under the 30s grace window, so
// a helper that is genuinely stuck still gets caught by a later sweep tick.
const EVICTION_BACKSTOP_GRACE: Duration = Duration::from_secs(30);

// CONTRACT (260726 Phase 1 correctness review I2): `connect_timeout` alone
// is exactly the budget a slow-but-successful `TerminalSession::spawn` is
// allowed to consume before its registry entry is even inserted into
// `sessions` - and the production registry is constructed with
// `DEFAULT_RECONCILE_CONNECT_TIMEOUT` (400ms), not the 3s the ticket/plan
// assumed, so a stale-enough gate of exactly `connect_timeout` leaves zero
// slack for the scheduling gap between `spawn` returning `Ok` and
// `insert()` actually taking the write lock. This adds a fixed floor on top
// of whatever `connect_timeout` is configured to - generous relative to any
// realistic insert-path scheduling delay, small relative to the 10s sweep
// interval and 30s grace window.
const STALE_ENTRY_SWEEP_MARGIN: Duration = Duration::from_secs(2);

// CONTRACT (260726 Phase 3): the browser-facing terminal WS has no
// server-initiated liveness probe, so a half-open connection (e.g. the
// client's machine sleeps/loses network without a clean TCP close) is
// otherwise invisible to `terminal_socket_task` - the loop only reacts to
// inbound frames, and a half-open peer never sends one. This is distinct
// from the `DAEMON_GRACE_WINDOW_MS` attach-grace contract above: that grace
// window governs eviction of the `Exited` *session* from the registry; this
// heartbeat only detects and tears down a dead *browser* connection on a
// session that otherwise still admits attach. A ~2-3 missed-beat tolerance
// keeps this well clear of ordinary scheduling jitter.
//
// Coverage scope (review cycle-1 correctness finding): this only detects an
// *idle* half-open peer - no inbound activity while the `select!` loop is
// between sends, including a Pong reply to our own Ping. `tokio::select!`
// does not poll other branches while parked inside an already-selected
// branch's own `.await` (e.g. `send_output_backfill`'s write blocking on a
// full socket send buffer against a dead peer), so a peer that goes dead
// mid-write is not caught by this heartbeat; that shape still falls back to
// the kernel's TCP retransmit timeout. The ticket's finding scopes
// explicitly to the idle case, so this is an intentional scope limit, not a
// gap in this mechanism.
const WS_HEARTBEAT_INTERVAL: Duration = Duration::from_secs(15);
const WS_HEARTBEAT_IDLE_TIMEOUT: Duration = Duration::from_secs(45);

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
    // CONTRACT (260725 Phase 4): terminal_id -> callback token, kept in
    // lockstep with `sessions` at the same four choke points
    // (`insert`/`insert_unchecked`/`remove`/`remove_for_work_roots`) - see
    // each method's own comment. NEVER logged, NEVER serialized into
    // `TerminalRegistryEntry` or any HTTP response; the only reader is
    // `post_terminal_turn_state`'s token check.
    tokens: Arc<RwLock<HashMap<String, String>>>,
    // CONTRACT (260725 Phase 4 review cycle 1, finding A - CLOSES the
    // concurrent-spawn GC race, does not merely shrink it): `sessions` only
    // gains a terminal's id at `insert`/`insert_unchecked`, which lands AFTER
    // `TerminalSession::spawn` has already created `agent-profiles/<id>/` on
    // disk (`profile.json` write, and for a hooked profile the token and
    // `callback.json` writes too) and completed a real process
    // spawn plus an IPC handshake - a real interval on the order of tens to
    // hundreds of milliseconds during which the directory exists but the id
    // is in neither `sessions` nor (before this field existed) anywhere the
    // sweep could see. `mark_profile_pending`/`clear_profile_pending` bound
    // that exact interval: `spawn` marks the id pending BEFORE creating the
    // directory, and `insert`/`insert_unchecked` clear it AFTER the id is
    // already visible in `sessions` (never before - see each method's own
    // ordering comment). `spawn`'s own failure paths after marking (helper
    // spawn failure, handshake timeout) also clear it, since those paths
    // never reach `insert` and would otherwise leak the mark forever. Because
    // `live_terminal_ids()` (the sweep's sole liveness source) unions this
    // set with `sessions`' keys, at every instant from directory-creation to
    // session-insertion the id is a member of AT LEAST ONE of the two sets -
    // there is no gap for a sweep's `read_dir` to land in. Snapshotting
    // liveness after `read_dir` instead of before (the reviewer's original
    // suggestion) was rejected: it only narrows the window to "created after
    // the post-listing snapshot", which is still open for exactly the same
    // spawn-plus-handshake duration: it does not name a boundary the window
    // cannot cross, whereas the mark/clear pair here corresponds to real code
    // events (directory creation, session insertion) that fully bracket it.
    pending_profile_ids: Arc<RwLock<HashSet<String>>>,
    helper_binary: PathBuf,
    registry_dir: PathBuf,
    connect_timeout: Duration,
    // CONTRACT (260726 Phase 1 correctness review I1): ids evicted by
    // `sweep_evict_expired`/`insert`'s lazy eviction, timestamped so
    // `sweep_registry_backstop` can hold them out of its kill-eligible set
    // for `EVICTION_BACKSTOP_GRACE` - see that constant's CONTRACT.
    recently_evicted: Arc<Mutex<HashMap<String, Instant>>>,
    // CONTRACT (260725 Phase 4): the daemon state dir, threaded in from
    // `persistent_state::default_state_dir()` at construction rather than
    // recomputed at every call site that needs it (spawn's hook-config
    // branch, boot-reconcile's adopt arm, the GC sweep) - single source of
    // truth for where `terminal-tokens/` and `agent-profiles/` live.
    state_dir: Option<PathBuf>,
    // CONTRACT (260725 Phase 4, ticket "Ephemeral port"): the daemon's own
    // bound base URL, threaded in from the SAME `bound_addr`-derived string
    // `server.rs` already builds at bind time (BEFORE `boot_reconcile`
    // runs) - never re-read from `bound-base-url.json`, which would
    // reintroduce the rejected multi-daemon-steal shape (see
    // `agent_callback.rs`'s CONTRACT on `write_bound_base_url`).
    base_url: String,
    // CONTRACT (260725 Phase 5): shared Arc-backed attention state. UNLIKE
    // `state_dir`/`base_url` above, this is NOT threaded in as a `new()`
    // constructor parameter - every construction site starts with an empty
    // hub (`AttentionHub::default()`), so there is nothing meaningful for a
    // caller to supply. What a caller building `AppState` DOES need is a
    // CLONE of this SAME instance (never a fresh, disconnected
    // `AttentionHub::default()`) so `AppState.attention`'s route handlers and
    // this registry's `remove`/`remove_for_work_roots` choke points
    // (`forget`, below) observe each other's writes - `attention()` is the
    // one accessor that hands out that shared clone.
    attention: AttentionHub,
}

impl Default for TerminalRegistry {
    fn default() -> Self {
        Self::new(
            default_helper_binary(),
            default_registry_dir(),
            DEFAULT_CONNECT_TIMEOUT,
            crate::persistent_state::default_state_dir(),
            // CONTRACT: no real bound address exists outside `server.rs`'s
            // `run_with_shutdown` - the only production call site always
            // supplies the real bound base URL instead of this fallback.
            // An empty string degrades gracefully: `write_callback_target`
            // would write an unusable (but never dangerous) `baseUrl`, and
            // nothing constructs a `TerminalRegistry` via `Default` today
            // (see plan Codebase Findings).
            String::new(),
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
    pub fn new(
        helper_binary: PathBuf,
        registry_dir: PathBuf,
        connect_timeout: Duration,
        state_dir: Option<PathBuf>,
        base_url: String,
    ) -> Self {
        Self {
            sessions: Arc::new(RwLock::new(HashMap::new())),
            tokens: Arc::new(RwLock::new(HashMap::new())),
            pending_profile_ids: Arc::new(RwLock::new(HashSet::new())),
            helper_binary,
            registry_dir,
            connect_timeout,
            recently_evicted: Arc::new(Mutex::new(HashMap::new())),
            state_dir,
            base_url,
            attention: AttentionHub::default(),
        }
    }

    // CONTRACT (260725 Phase 5): the ONE way a caller building `AppState`
    // obtains the SAME attention hub this registry's `remove`/
    // `remove_for_work_roots` choke points clean up via `forget` - never
    // construct a separate `AttentionHub::default()` for `AppState.attention`,
    // or the registry's forget-on-close would silently write into a hub no
    // route handler ever reads from.
    pub fn attention(&self) -> AttentionHub {
        self.attention.clone()
    }

    // CONTRACT (ticket "Boot reconcile policy" / server.rs wiring): must run
    // to completion BEFORE `build_router`/`axum::serve` starts accepting
    // connections - callers must await this before constructing `AppState`.
    // Scans `registry_dir` for `<termid>.json` entries left behind by
    // helpers that survived a prior daemon's exit, applies the 6-row table
    // (`terminal_reconcile::classify`) per entry, and returns a registry
    // pre-populated with every adopted (row 1/2) session.
    //
    // CONTRACT (260725 Phase 4, ORDERING IS LOAD-BEARING): the `server.rs`
    // GC sweep must be spawned strictly AFTER this call is awaited to
    // completion - see `agent_profile_gc.rs`'s module CONTRACT for why a
    // sweep racing ahead of this line would delete the profile directory of
    // every helper this call is about to adopt.
    pub async fn boot_reconcile(
        helper_binary: PathBuf,
        registry_dir: PathBuf,
        connect_timeout: Duration,
        state_dir: Option<PathBuf>,
        base_url: String,
    ) -> Self {
        let registry = Self::new(
            helper_binary,
            registry_dir.clone(),
            connect_timeout,
            state_dir,
            base_url,
        );
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
        let identity = identity_status(entry.pid, entry.start_time, entry.boot_id.as_deref());
        if !matches!(identity, IdentityStatus::VerifiedOurs) {
            delete_registry_entry(&self.registry_dir, &entry.terminal_id);
            return;
        }

        let outcome = connect_and_handshake(&entry.socket_path, self.connect_timeout).await;
        let (ipc_status, connected) = match outcome {
            HandshakeOutcome::Connected(connected) => {
                let status = if connected.status == TerminalStatus::Running {
                    IpcStatus::ReachableShellAlive
                } else {
                    IpcStatus::ReachableShellExited
                };
                (status, Some(connected))
            }
            HandshakeOutcome::NoListener => (IpcStatus::NoListener, None),
            HandshakeOutcome::ConnectedButSilent => (IpcStatus::ConnectedButSilent, None),
        };

        // CONTRACT (260729): the probe is only opened for the one signal that
        // cannot decide on its own. It is deliberately NOT part of the
        // adoption path above - the adopt rows already have handshake-proven
        // reachability, and putting anything extra on that wire would break
        // the one case that works today for every pre-upgrade helper.
        let probe = match ipc_status {
            IpcStatus::ConnectedButSilent => Some(
                probe_helper(
                    &entry.socket_path,
                    entry.supports_liveness_probe,
                    self.connect_timeout,
                )
                .await,
            ),
            _ => None,
        };

        match classify(identity, ipc_status, probe) {
            ReconcileRow::AdoptLive | ReconcileRow::AdoptGrace => {
                let connected =
                    *connected.expect("adopt rows are only reachable with a live connection");
                let callback_token = self.recover_callback_token(&entry.terminal_id);
                let profile_id = self.recover_profile_id(&entry.terminal_id);
                let session = TerminalSession::from_connection(
                    entry.terminal_id.clone(),
                    WorkRootId::from(entry.work_root_id.clone()),
                    entry.title.clone(),
                    entry.cwd_hint.clone(),
                    entry.created_at_ms,
                    connected,
                    entry.columns,
                    entry.rows,
                    // CONTRACT (260726 profile-provenance fix): the profile
                    // id is recovered from the daemon-owned sidecar
                    // `agent-profiles/<terminal_id>/profile.json`, NOT from
                    // `TerminalRegistryEntry` - that file is written by the
                    // helper process (`terminal_helper_process.rs`) and still
                    // carries no profile field (hard constraint, see
                    // `terminal_registry_file.rs::TerminalRegistryEntry`).
                    // The recovered id is echoed verbatim: this field is
                    // provenance (what spawned this terminal), not a live
                    // capability claim, so it is deliberately NOT re-resolved
                    // through `agent_profile_registry::resolve` - doing so
                    // would erase provenance for a still-running terminal the
                    // moment its profile is renamed or retired.
                    //
                    // The recovery still yields `None` whenever no readable
                    // sidecar survives to this point; the known ways to get
                    // there are listed on `TerminalSession::profile_id`'s
                    // CONTRACT (known cases, not a closed set). All of them
                    // degrade to the pre-fix observable behavior
                    // (`profileId: null`), never to a wrong id. See
                    // `recover_profile_id`.
                    //
                    // CONTRACT (260725 Phase 4, callback token): recovered by
                    // its own separate path from `terminal-tokens/` plus
                    // `callback.json` - see `recover_callback_token`. The two
                    // files stay separate on purpose (a hookless profile has
                    // provenance but must never get a token); do not merge
                    // the two recoveries into one file read.
                    profile_id,
                    callback_token,
                );
                self.insert_unchecked(session);
            }
            ReconcileRow::KillVerified => {
                kill_verified_and_delete_entry(
                    &self.registry_dir,
                    &entry.terminal_id,
                    entry.pid,
                    entry.start_time,
                    entry.boot_id.as_deref(),
                )
                .await;
            }
            // CONTRACT (260729): entry untouched, process untouched -
            // deliberately NOT folded into the drop rows below. Deleting the
            // entry here would spare the helper and simultaneously make it
            // unadoptable by the daemon that actually owns it, turning a live
            // terminal into a permanent orphan holding a PTY and a shell.
            ReconcileRow::Leave => {
                tracing::info!(
                    terminal_id = %entry.terminal_id,
                    pid = entry.pid,
                    supports_liveness_probe = entry.supports_liveness_probe,
                    "leaving a reachable helper alone at boot reconcile: it answered that it is \
                     in use (or predates the liveness probe and cannot answer at all), so this \
                     daemon has no positive evidence it is unreachable"
                );
            }
            ReconcileRow::DropNoSuchProcess
            | ReconcileRow::DropPidReused
            | ReconcileRow::DropUnverifiableBoot => {
                unreachable!("identity already verified above; classify cannot return this row")
            }
        }
    }

    // CONTRACT (260726 profile-provenance fix): reads back the sidecar
    // `TerminalSession::spawn` wrote for ANY resolved profile - hooked or
    // hookless - so a session re-adopted after a daemon restart reports the
    // profile it was spawned with instead of `null`. Deliberately tolerant in
    // both directions: no `state_dir` and a missing or malformed sidecar all
    // return `None`, which is exactly the pre-fix observable behavior rather
    // than a new failure mode. No re-validation against
    // `agent_profile_registry::resolve` - see the adopt arm's CONTRACT for
    // why the recorded id is echoed verbatim.
    fn recover_profile_id(&self, terminal_id: &str) -> Option<String> {
        let state_dir = self.state_dir.as_deref()?;
        let profile_dir = state_dir.join("agent-profiles").join(terminal_id);
        crate::agent_profile_store::read_profile_id(&profile_dir)
    }

    // CONTRACT (260725 Phase 4): the callback token is recovered by a
    // DIFFERENT file from `profile_id`'s sidecar (`recover_profile_id`
    // above), and the two must not be merged - the presence of
    // `callback.json` is load-bearing on its own, below. The token was
    // written once, at fresh spawn, to
    // `terminal-tokens/<terminal_id>.json` and never rotates for this
    // terminal's lifetime (one token per terminal, generated once, valid
    // until close - Design Answer 1). The presence of
    // `agent-profiles/<terminal_id>/callback.json` on disk is what
    // distinguishes "this terminal was spawned with hooks" (recover its
    // token, rewrite its callback target with the fresh `base_url`) from a
    // terminal that never had a token to recover, where `None` is not a loss:
    // a plain shell (no profile dir at all) or a resolved HOOKLESS profile,
    // which since the 260726 fix DOES have a profile dir holding
    // `profile.json` but still, deliberately, no `callback.json` and no
    // token. Testing for the file rather than the directory is what keeps
    // those two apart. An unresolved
    // `state_dir` also falls through to `None`, same as the fresh-spawn
    // path's own degrade.
    fn recover_callback_token(&self, terminal_id: &str) -> Option<String> {
        let state_dir = self.state_dir.as_deref()?;
        let profile_dir = state_dir.join("agent-profiles").join(terminal_id);
        let callback_path = crate::agent_callback::callback_path(&profile_dir);
        if !callback_path.exists() {
            return None;
        }
        let token = crate::agent_token_store::read_token(state_dir, terminal_id)?;
        if let Err(error) = crate::agent_callback::write_callback_target(
            &profile_dir,
            &self.base_url,
            terminal_id,
            &token,
        ) {
            tracing::error!(
                terminal_id = %terminal_id,
                %error,
                "failed to rewrite callback target on boot-reconcile adopt; a stale base URL \
                 may remain until the next successful rewrite"
            );
        }
        Some(token)
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

    // CONTRACT (260725 Phase 4): the ONLY reader of `self.tokens` - backs
    // `post_terminal_turn_state`'s auth check. Never logged, never echoed
    // into a response.
    fn token_for(&self, terminal_id: &str) -> Option<String> {
        self.tokens
            .read()
            .expect("terminal registry lock poisoned")
            .get(terminal_id)
            .cloned()
    }

    // CONTRACT (260725 Phase 4, GC sweep liveness source): every terminal id
    // currently in `self.sessions`, regardless of status - "keys off
    // TERMINAL liveness", not `TerminalSession::is_live()`'s strict
    // Running-only check, since a config may legitimately outlive an agent
    // that exited inside a surviving (or grace-window) terminal - UNIONED
    // with `self.pending_profile_ids`, the in-flight-spawn set that closes
    // the concurrent-spawn GC race (review cycle 1, finding A; see the field
    // doc comment on `pending_profile_ids` for why a union of the two sets,
    // rather than either alone, is what fully brackets the
    // directory-creation-to-session-insertion interval with no gap). See
    // `agent_profile_gc.rs`'s module CONTRACT for the ordering guarantee this
    // depends on at its one production call site (`server.rs`).
    pub(crate) fn live_terminal_ids(&self) -> HashSet<String> {
        let mut ids: HashSet<String> = self
            .sessions
            .read()
            .expect("terminal registry lock poisoned")
            .keys()
            .cloned()
            .collect();
        ids.extend(
            self.pending_profile_ids
                .read()
                .expect("terminal registry lock poisoned")
                .iter()
                .cloned(),
        );
        ids
    }

    // CONTRACT (260725 Phase 4 review cycle 1, finding A; hoisted by the
    // 260726 profile-provenance fix): called by `TerminalSession::spawn`
    // BEFORE it creates `agent-profiles/<id>/` on disk - i.e. before the
    // first `write_profile`/`write_token`/`write_callback_target` call, for
    // ANY resolved profile rather than only a hooked one - and this ordering
    // is what makes the id visible to `live_terminal_ids()` before the
    // directory a sweep could race against even exists.
    fn mark_profile_pending(&self, terminal_id: &str) {
        self.pending_profile_ids
            .write()
            .expect("terminal registry lock poisoned")
            .insert(terminal_id.to_owned());
    }

    // CONTRACT (260725 Phase 4 review cycle 1, finding A): the counterpart to
    // `mark_profile_pending`. Two call shapes, both safe to call
    // unconditionally (a missing key is a harmless no-op, so a plain-shell
    // spawn that never marked pending in the first place costs nothing
    // here): (1) `insert`/`insert_unchecked` call this AFTER the id is
    // already present in `sessions` - never before, or a sweep could
    // observe the id in neither set for an instant; (2) `TerminalSession::
    // spawn`'s own failure paths after marking (helper spawn failure,
    // handshake timeout) call this directly, since a `spawn` that returns
    // `Err` never reaches `insert` and would otherwise leak the mark
    // forever - the directory such a failure may have already created
    // becomes an ordinary orphan for the next sweep to reclaim, same as the
    // pre-existing MAX_TERMINAL_SESSIONS-rejection case.
    fn clear_profile_pending(&self, terminal_id: &str) {
        self.pending_profile_ids
            .write()
            .expect("terminal registry lock poisoned")
            .remove(terminal_id);
    }

    // CONTRACT (260726 Phase 1 correctness review I3, grace authority): this
    // lazy eviction is a session-removal path in its own right (it is the
    // one the enumerating comment on `sessions_write_lock_sites_are_
    // enumerated` calls the "FIFTH session-removal path"), and it must key
    // off `admits_attach()`, exactly like the periodic sweep's own eviction
    // step (`sweep_evict_expired`) - `is_live()` alone would drop a session
    // still inside its 30s attach grace the instant an unrelated
    // `create_terminal` call happens to land, discarding the grace window's
    // whole purpose. Worse, since a session dropped here leaves `sessions`
    // entirely, the periodic sweep could never find it again to close its
    // IPC connection - the exact fd/parked-helper leak 260726 Phase 1
    // targets. Every session evicted here is therefore handed to the same
    // `evict_and_close` path the periodic sweep uses, which is the ONE place
    // an eviction discharges its two removal obligations (`forget_token` +
    // `attention.forget`) and arranges its `EVICTION_BACKSTOP_GRACE`
    // protection before closing IPC - never merely dropped out of the map.
    async fn insert(&self, session: Arc<TerminalSession>) -> Result<(), TerminalError> {
        // CONTRACT (260725 Phase 4 review cycle 1, finding A): capture the id
        // before `session` is (possibly) moved into `sessions` below, so
        // `clear_profile_pending` can run on every exit path - including the
        // cap-rejection `Err` path, where the directory `spawn` may already
        // have created simply becomes an ordinary orphan for the next sweep.
        let terminal_id = session.id.clone();
        let (evicted, result) = {
            let mut sessions = self
                .sessions
                .write()
                .expect("terminal registry lock poisoned");
            let mut evicted = Vec::new();
            sessions.retain(|_, existing| {
                if existing.admits_attach() {
                    true
                } else {
                    evicted.push(existing.clone());
                    false
                }
            });
            let result = if sessions.len() >= MAX_TERMINAL_SESSIONS {
                Err(TerminalError::BadRequest("too many terminal sessions"))
            } else {
                self.remember_token(&session);
                sessions.insert(terminal_id.clone(), session);
                Ok(())
            };
            (evicted, result)
        };
        // CONTRACT (load-bearing ORDER, finding A): clear the pending mark
        // only AFTER the id is visible in `sessions` (the write guard above
        // is scoped to the block that just ended, publishing the insert to
        // any concurrent reader before this call), never before - see
        // `pending_profile_ids`'s field doc for why the union, not this
        // ordering alone, closes the race, and why getting this ordering
        // backwards would reopen it.
        self.clear_profile_pending(&terminal_id);
        self.evict_and_close(evicted).await;
        result
    }

    // Boot-reconcile-only insertion path: adopted sessions must all land in
    // the registry before the cap is evaluated against any *new*
    // `create_terminal` call (see `boot_reconcile`'s doc comment) - applying
    // `insert`'s cap check here could evict a legitimately-adopted live
    // session for no better reason than scan order.
    fn insert_unchecked(&self, session: Arc<TerminalSession>) {
        let terminal_id = session.id.clone();
        self.remember_token(&session);
        self.sessions
            .write()
            .expect("terminal registry lock poisoned")
            .insert(terminal_id.clone(), session);
        // See `insert`'s identical ordering CONTRACT: clear only after the
        // id is visible in `sessions`. Boot-reconcile-adopted sessions never
        // went through `TerminalSession::spawn`'s pending mark, so this is a
        // harmless no-op for them; kept here for symmetry and so a future
        // adopt-path change that DID mark pending would not need to
        // rediscover this requirement.
        self.clear_profile_pending(&terminal_id);
    }

    // CONTRACT (260725 Phase 4): the ONE place `self.tokens` gains an entry
    // - both `insert` and `insert_unchecked` call this so the in-memory
    // token map and the session map never drift apart. A session with no
    // `callback_token` (plain shell, or hook materialization that failed
    // before a token was generated) simply adds nothing here.
    fn remember_token(&self, session: &Arc<TerminalSession>) {
        if let Some(token) = session.callback_token.clone() {
            self.tokens
                .write()
                .expect("terminal registry lock poisoned")
                .insert(session.id.clone(), token);
        }
    }

    // CONTRACT (260725 Phase 4): deletes the matching `self.tokens` entry
    // AND best-effort deletes the on-disk token file - the other of the
    // four lockstep choke points (see `remember_token` for the insert side).
    fn forget_token(&self, terminal_id: &str) {
        let had_token = self
            .tokens
            .write()
            .expect("terminal registry lock poisoned")
            .remove(terminal_id)
            .is_some();
        if had_token {
            if let Some(state_dir) = self.state_dir.as_deref() {
                crate::agent_token_store::delete_token(state_dir, terminal_id);
            }
        }
    }

    fn remove(&self, terminal_id: &str) -> Option<Arc<TerminalSession>> {
        let removed = self
            .sessions
            .write()
            .expect("terminal registry lock poisoned")
            .remove(terminal_id);
        self.forget_token(terminal_id);
        // CONTRACT (260725 Phase 5): mirrors `forget_token` exactly - the
        // other lockstep choke point a closed terminal's attention snapshot
        // entry must be forgotten at, or a reconnect's snapshot would show a
        // phantom terminal after close.
        self.attention.forget(terminal_id);
        removed
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
        drop(sessions);
        for session in &removed {
            self.forget_token(&session.id);
            // CONTRACT (260725 Phase 5): same forget-on-removal rule as
            // `remove` above - a workRoot/workspace removal must forget every
            // one of its terminals' attention entries, not just their tokens.
            self.attention.forget(&session.id);
        }
        removed
    }

    // CONTRACT (same kill obligation as `remove_for_work_roots`): drains the
    // ENTIRE registry and returns every removed session so the caller can
    // `terminate()` each. Backs the "kill all terminals" teardown - a detached
    // helper keeps running orphaned unless explicitly killed, so the map drain
    // alone is not enough.
    // CONTRACT (260727 Phase 3): full parity with `remove`/
    // `remove_for_work_roots` - a kill-all sweep forgets BOTH obligations for
    // every drained session, at the same two lockstep choke points: the
    // callback token (`forget_token`, which also deletes the on-disk
    // `terminal-tokens/<id>.json`, or a revoked terminal's token would keep
    // authenticating turn-state callbacks after the terminal is gone) and the
    // attention entry (`attention.forget`, or a reconnect's snapshot would
    // show a phantom terminal after the sweep).
    // The `sessions` write guard is scoped to the drain and released BEFORE
    // the forget loop, mirroring `remove_for_work_roots`'s explicit
    // `drop(sessions)`. This is a lock-HOLD-DURATION rule, not deadlock
    // avoidance: `self.sessions` and `self.tokens` are distinct `RwLock`s and
    // no path acquires them in the reverse order today, but `forget_token`
    // does synchronous disk I/O per session, so running it under the sessions
    // write lock would stall every other terminal request (list/create/attach/
    // close) for the whole sweep, proportional to terminal count. The
    // `sessions` -> `tokens` order this drop-then-loop follows is not a new
    // invariant it establishes: `insert` already holds the `sessions` write
    // guard (acquired, dropped after `remember_token` returns) while calling
    // `remember_token`, which itself takes `tokens.write()` - so both locks
    // are already held simultaneously on the registry's hottest path.
    // `drain_all` simply follows that same established order; what its
    // drop-then-loop shape actually buys is hold duration, not a deadlock
    // invariant that does not exist.
    pub fn drain_all(&self) -> Vec<Arc<TerminalSession>> {
        let drained: Vec<Arc<TerminalSession>> = self
            .sessions
            .write()
            .expect("terminal registry lock poisoned")
            .drain()
            .map(|(_, session)| session)
            .collect();
        for session in &drained {
            self.forget_token(&session.id);
            self.attention.forget(&session.id);
        }
        drained
    }

    // CONTRACT (260726 Phase 1 sub-fix 3, grace authority): the 30s
    // attach-grace window (`admits_attach()`) is the ONLY correct eviction
    // gate for the periodic sweep - `!is_live()` alone would evict the
    // instant a shell exits, before its grace window even starts, which is
    // exactly what the grace window exists to prevent. Mirrors
    // `remove_for_work_roots`'s retain-and-collect shape: returns every
    // evicted session so `sweep_once` can close each one's IPC connection
    // (the eviction step itself does not touch IPC - that would require
    // `.await`ing under the write lock).
    // CONTRACT (260727 Phase 3 removal obligations): this is a
    // session-removal path, so its return value MUST be handed to
    // `evict_and_close` - that is where an eviction discharges
    // `forget_token` and `attention.forget` for every id it dropped. This
    // function deliberately does NOT discharge them itself: `insert`'s lazy
    // eviction removes sessions through an identical `retain`, and routing
    // both through `evict_and_close` keeps exactly ONE implementation of the
    // eviction discharge instead of two that could drift. Calling this
    // without the paired `evict_and_close` leaks a phantom `AttentionHub`
    // entry into every future snapshot for the daemon's lifetime and leaves
    // `terminal-tokens/<id>.json` on disk.
    pub(crate) fn sweep_evict_expired(&self) -> Vec<Arc<TerminalSession>> {
        let mut sessions = self
            .sessions
            .write()
            .expect("terminal registry lock poisoned");
        let mut evicted = Vec::new();
        sessions.retain(|_, session| {
            if session.admits_attach() {
                true
            } else {
                evicted.push(session.clone());
                false
            }
        });
        evicted
    }

    // Shared by the periodic sweep's own eviction step and `insert`'s lazy
    // eviction (260726 Phase 1 correctness review I1/I3): records each
    // evicted session's id BEFORE closing its IPC connection, so
    // `sweep_registry_backstop` can hold it out of its kill-eligible set for
    // `EVICTION_BACKSTOP_GRACE` (see that constant's CONTRACT) - giving the
    // helper's own graceful self-exit the primary chance to run instead of
    // being preempted by a same/next-tick SIGKILL.
    // CONTRACT (260727 Phase 3 removal obligations): this is ALSO the single
    // discharge choke point for BOTH eviction paths. Eviction removes a
    // session from `sessions` exactly like `remove`/`remove_for_work_roots`/
    // `drain_all` do, so it owes the same two forgets, at the same lockstep
    // pair: `forget_token` (which also unlinks `terminal-tokens/<id>.json`,
    // or a revoked terminal's token would keep authenticating turn-state
    // callbacks after the terminal is gone) and `attention.forget` (or the
    // evicted id leaks into every future `attentionSnapshot` for the
    // daemon's lifetime - nothing else ever removes it, because an eviction
    // never calls `remove`). Both run BEFORE the IPC teardown below and
    // outside every `sessions` write guard (each caller's guard is already
    // released by the time it calls this), mirroring `drain_all`'s
    // drop-then-forget hold-duration rule: `forget_token` does synchronous
    // disk I/O per session.
    async fn evict_and_close(&self, evicted: Vec<Arc<TerminalSession>>) {
        if evicted.is_empty() {
            return;
        }
        {
            let mut recently_evicted = self
                .recently_evicted
                .lock()
                .expect("terminal registry lock poisoned");
            let now = Instant::now();
            for session in &evicted {
                recently_evicted.insert(session.id.clone(), now);
            }
        }
        for session in &evicted {
            self.forget_token(&session.id);
            self.attention.forget(&session.id);
        }
        for session in evicted {
            session.close_ipc_connection().await;
        }
    }

    // CONTRACT (260726 Phase 1 sub-fix 3, registry-dir backstop): re-scans
    // `registry_dir` for `<id>.json`/`.sock` entries with no corresponding
    // LIVE session in this registry's own map - catching helpers whose
    // handshake-failure cleanup (sub-fix 1) never ran (e.g. a hard daemon
    // crash mid-`create_terminal`, or a crash before boot-reconcile ever saw
    // the entry). Deliberately never calls `connect_and_handshake`: unlike
    // `reconcile_entry`, a runtime sweep only ever kills, it never adopts -
    // attempting a handshake here would silently reintroduce an
    // adopt-shaped code path the ticket explicitly forbids for the sweep.
    //
    // CONTRACT REWRITTEN (260729 helper liveness probe). The "never adopts"
    // property above STANDS, and `probe_helper` preserves it: a probe writes
    // no `HandshakeAck` and can adopt nothing. What this CONTRACT used to
    // also claim - "never calls `connect_and_handshake`" read as "never asks
    // anything at all" - was the defect. With no check whatsoever, the sole
    // kill predicate was "absent from THIS daemon's session map + identity
    // verified", and another daemon's healthy, in-use helper satisfies both
    // by construction: it is absent from a session map it was never in, and
    // its pid/start-time/boot-id all genuinely match, because they are real.
    // The boot-identity gate cannot help - two concurrent daemons are on the
    // same boot, so the boot id matches by construction. This was the
    // fastest and broadest destructive path in the tree.
    //
    // The sweep now PROBES before it signals. It kills only on positive
    // evidence of unreachability: no listener behind the socket, or (only
    // for a helper whose entry declares `supportsLivenessProbe`) a probe
    // that went unanswered or reported nobody attached for longer than
    // `UNATTACHED_GRACE`. A helper that answers "attached", a helper
    // unattached for less than the grace (its daemon is restarting), and a
    // helper that predates the probe entirely are all LEFT ALONE - entry and
    // process both, since deleting the entry of a surviving helper makes it
    // unadoptable and is worse than the kill it replaces.
    // The liveness set is `live_terminal_ids()` (`sessions` UNIONED with
    // `pending_profile_ids`), never `sessions.keys()` alone: an id that
    // `TerminalSession::spawn` has marked pending but not yet inserted is
    // genuinely live, and naming it here makes in-flight-`create_terminal`
    // coverage EXPLICIT rather than leaving it to be incidentally covered by
    // the independent age gate below. The two guards are deliberately not
    // redundant - the age gate bounds how young an entry may be, the live
    // set bounds which ids exist at all, and only the union of the two
    // brackets the whole directory-creation-to-session-insertion interval
    // (see `pending_profile_ids`'s field doc).
    // An entry younger than `connect_timeout + STALE_ENTRY_SWEEP_MARGIN`
    // (`registry_entry_is_stale_enough_for_sweep`) is always skipped - it
    // may simply be mid-`create_terminal`, not yet inserted into `sessions`
    // (review I2). An entry whose id was evicted within
    // `EVICTION_BACKSTOP_GRACE` is also always skipped (review I1) - it is
    // still `<id>.json`-stale by construction (its entry predates eviction),
    // but its helper is expected to self-exit on its own within
    // milliseconds of the eviction step's `close_ipc_connection` call, and
    // that graceful path must win the race, not this backstop's SIGKILL.
    pub(crate) async fn sweep_registry_backstop(&self) {
        let scan_dir = self.registry_dir.clone();
        let entries = tokio::task::spawn_blocking(move || scan_registry_dir(&scan_dir))
            .await
            .unwrap_or_default();
        if entries.is_empty() {
            return;
        }

        let live_ids = self.live_terminal_ids();
        let recently_evicted_ids: std::collections::HashSet<String> = {
            let mut recently_evicted = self
                .recently_evicted
                .lock()
                .expect("terminal registry lock poisoned");
            let now = Instant::now();
            recently_evicted
                .retain(|_, evicted_at| now.duration_since(*evicted_at) < EVICTION_BACKSTOP_GRACE);
            recently_evicted.keys().cloned().collect()
        };
        let now = now_ms();

        for entry in entries {
            if live_ids.contains(&entry.terminal_id) || recently_evicted_ids.contains(&entry.terminal_id)
            {
                continue;
            }
            if !registry_entry_is_stale_enough_for_sweep(
                now,
                entry.created_at_ms,
                self.connect_timeout,
            ) {
                continue;
            }
            match identity_status(entry.pid, entry.start_time, entry.boot_id.as_deref()) {
                IdentityStatus::VerifiedOurs => {
                    // CONTRACT (260729): identity says "this pid really is the
                    // helper this entry describes". It says NOTHING about
                    // whether that helper is dead - which is the whole reason
                    // this sweep used to destroy another daemon's live
                    // terminals. Ask the helper before signalling it.
                    let probe = probe_helper(
                        &entry.socket_path,
                        entry.supports_liveness_probe,
                        self.connect_timeout,
                    )
                    .await;
                    if !probe_authorizes_reclaim(probe) {
                        // Leave: entry AND process untouched. Falling through
                        // to the delete below would spare the helper and
                        // orphan it in the same breath.
                        tracing::debug!(
                            terminal_id = %entry.terminal_id,
                            pid = entry.pid,
                            ?probe,
                            "registry backstop leaving a reachable helper alone"
                        );
                        continue;
                    }
                    kill_verified_and_delete_entry(
                        &self.registry_dir,
                        &entry.terminal_id,
                        entry.pid,
                        entry.start_time,
                        entry.boot_id.as_deref(),
                    )
                    .await;
                }
                IdentityStatus::NoSuchProcess
                | IdentityStatus::PidReused
                | IdentityStatus::UnverifiableBoot => {
                    // Same never-kill invariant as `reconcile_entry`: an
                    // unverified identity is dropped only, never signaled.
                    // `UnverifiableBoot` matters most HERE: this sweep re-runs
                    // every 10s at runtime, so a stale cross-boot entry that
                    // survived a hard reboot would otherwise be re-evaluated
                    // thousands of times a day, each one a fresh chance for
                    // the recorded pid+tick-offset pair to coincide with some
                    // unrelated process on the new boot.
                    delete_registry_entry(&self.registry_dir, &entry.terminal_id);
                }
            }
        }
    }

    // CONTRACT (260729 helper liveness probe, step 4): `agent_profile_gc` is
    // NOT one of the kill sites - it never calls `kill_verified` - but it
    // destroys `agent-profiles/<id>/` and `terminal-tokens/<id>.json` off the
    // SAME wrong signal, `live_terminal_ids()` read directly. Fixing
    // `classify` and the sweep does not reach it: another daemon's terminals
    // stay absent from this daemon's session map even after they correctly
    // survive Sites A and B, so without this gate the helper processes live
    // and their state is deleted anyway - which is the original symptom this
    // ticket is named after.
    //
    // It has no socket path of its own (profile directories are named by bare
    // terminal id), so the registry entry is what supplies one.
    //
    // THE NO-ENTRY RULE IS LOAD-BEARING IN THE OPPOSITE DIRECTION: no
    // registry entry at all means no helper, so reclaim. Omitting it would
    // stop the GC reclaiming genuine orphans - a leak, not a safety margin.
    pub(crate) async fn profile_gc_may_reclaim(&self, terminal_id: &str) -> bool {
        let registry_dir = self.registry_dir.clone();
        let id = terminal_id.to_owned();
        let entry = tokio::task::spawn_blocking(move || read_registry_entry(&registry_dir, &id))
            .await
            .unwrap_or(None);
        let Some(entry) = entry else {
            return true;
        };
        probe_authorizes_reclaim(
            probe_helper(
                &entry.socket_path,
                entry.supports_liveness_probe,
                self.connect_timeout,
            )
            .await,
        )
    }

    // Single entry point the periodic reaper task drives: evict every
    // session whose attach grace has expired and close its IPC connection,
    // then sweep the registry directory for orphaned entries no live
    // session accounts for. Order matters: the in-memory eviction (cheap,
    // synchronous under the write lock) runs first, then IPC teardown
    // (async), then the registry-dir backstop (a full directory scan) -
    // cheapest/most-common work first. The Unix shell-zombie reap
    // (sub-fix 3b) needs no daemon-side action here: it lives entirely in
    // the helper process's own `SharedState::transition`, triggered once the
    // helper observes the IPC EOF this function's `close_ipc_connection`
    // call produces.
    pub(crate) async fn sweep_once(&self) {
        let evicted = self.sweep_evict_expired();
        self.evict_and_close(evicted).await;
        self.sweep_registry_backstop().await;
    }
}

// CONTRACT (boot-identity gate): the boot check runs BEFORE the start-time
// comparison and before `/proc` (or its per-platform equivalent) is touched
// at all. On a boot-relative platform a `start_time` from another boot - or
// one with no boot qualifier recorded - is not a weaker match, it is not a
// match at all: the very same tick offset names a different instant on the
// new boot, so comparing it can manufacture a `VerifiedOurs` for an
// arbitrary unrelated process. Ordering the check first is what keeps
// "unverifiable" from ever being able to fall through into `VerifiedOurs`.
// See `terminal_reconcile::boot_identity_verified` for the per-platform
// semantics and `terminal_platform.rs`'s file-header CONTRACT for why only
// Linux is boot-relative.
fn identity_status(pid: u32, start_time: u64, recorded_boot_id: Option<&str>) -> IdentityStatus {
    if !boot_identity_verified(
        crate::terminal_platform::START_TIME_IS_BOOT_RELATIVE,
        recorded_boot_id,
        crate::terminal_platform::boot_identity().as_deref(),
    ) {
        return IdentityStatus::UnverifiableBoot;
    }
    match crate::terminal_platform::process_start_time(pid) {
        Some(observed) if observed == start_time => IdentityStatus::VerifiedOurs,
        Some(_) => IdentityStatus::PidReused,
        None => IdentityStatus::NoSuchProcess,
    }
}

// CONTRACT: any daemon-initiated kill of a registry entry MUST route through
// `terminal_platform::kill_verified` (never a bare pid kill) - shared by
// `reconcile_entry`'s `KillVerified` row, `TerminalSession::spawn`'s
// handshake-failure cleanup (sub-fix 1), and the periodic sweep's
// registry-dir backstop (sub-fix 3), so the kill+delete sequence has exactly
// one implementation. Mirrors `reconcile_entry`'s original inline shape:
// verified kill first (best-effort - an already-gone process makes this a
// harmless no-op), then unconditionally prune both registry files.
//
// CONTRACT (boot-identity gate, choke point): because this is the ONE place
// a registry entry's pid is ever signaled, the boot gate is re-applied here
// rather than trusted from the caller. Two of the three callers
// (`reconcile_entry`, `sweep_registry_backstop`) already ran `identity_status`
// and cannot reach this with an unverifiable boot; the third
// (`TerminalSession::spawn`'s handshake-failure cleanup) reads its entry
// straight off disk and never calls `identity_status` at all, so without this
// re-check that path would be the last remaining way a stale cross-boot
// `start_time` could authorize a SIGKILL. On mismatch the entry is still
// pruned - drop-only is the outcome, exactly as in `classify`'s
// `DropUnverifiableBoot` row - only the signal is withheld.
async fn kill_verified_and_delete_entry(
    registry_dir: &Path,
    terminal_id: &str,
    pid: u32,
    start_time: u64,
    recorded_boot_id: Option<&str>,
) {
    if boot_identity_verified(
        crate::terminal_platform::START_TIME_IS_BOOT_RELATIVE,
        recorded_boot_id,
        crate::terminal_platform::boot_identity().as_deref(),
    ) {
        let _ = tokio::task::spawn_blocking(move || {
            crate::terminal_platform::kill_verified(pid, start_time)
        })
        .await;
    } else {
        tracing::warn!(
            terminal_id,
            pid,
            "refusing to signal a registry entry whose boot identity does not match this boot - \
             its start time is not comparable, so the pid may belong to an unrelated process; \
             dropping the entry only"
        );
    }
    delete_registry_entry(registry_dir, terminal_id);
}

// Pure predicate for the periodic sweep's registry-dir backstop age gate,
// split out so the "must never touch an entry younger than the connect/
// handshake timeout" contract (an open/mid-`create_terminal` registry-dir
// entry) is unit-testable without a real filesystem scan, process, or async
// driver. CONTRACT (260726 Phase 1 correctness review I2): the threshold is
// `connect_timeout + STALE_ENTRY_SWEEP_MARGIN`, not `connect_timeout` alone
// - see that constant's CONTRACT for why a bare `connect_timeout` gate has
// zero slack over the in-flight `create_terminal` window.
fn registry_entry_is_stale_enough_for_sweep(
    now_ms: u64,
    created_at_ms: u64,
    connect_timeout: Duration,
) -> bool {
    let threshold_ms = connect_timeout.as_millis() as u64 + STALE_ENTRY_SWEEP_MARGIN.as_millis() as u64;
    now_ms.saturating_sub(created_at_ms) > threshold_ms
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

/// CONTRACT (260729): replaces `connect_and_handshake`'s former bare
/// `Option`. The old return type folded six distinct failure paths into one
/// `None`, and `reconcile_entry` then folded that into
/// `IpcStatus::Unreachable` and killed - so "the socket file is gone" and
/// "connected fine, the helper is busy serving another daemon" were the same
/// value. The `io::Error` that distinguishes them was available at the
/// connect site all along and simply discarded.
enum HandshakeOutcome {
    Connected(Box<HandshakeConnection>),
    /// `connect` never succeeded before the deadline: positive absence.
    NoListener,
    /// The connection was accepted, but the handshake did not arrive (or was
    /// not what the protocol requires). Undecidable on its own - see
    /// `IpcStatus::ConnectedButSilent`.
    ConnectedButSilent,
}

// CONTRACT (260729): the adoption sequence this function drives is the one
// path every helper alive across the upgrade depends on, and it must stay
// byte-identical: it writes ONLY `HandshakeAck` and reads ONLY `Handshake`
// followed by `Status`/`Exit`. Nothing about the probe belongs in here -
// the probe is a separate connection (`probe_helper`), only ever opened
// after this function has already reported `ConnectedButSilent`, and only
// against a helper whose registry entry declares it can answer.
async fn connect_and_handshake(socket_path: &Path, timeout: Duration) -> HandshakeOutcome {
    let deadline = Instant::now() + timeout;
    let stream = loop {
        match crate::terminal_ipc_transport::connect(socket_path).await {
            Ok(stream) => break stream,
            Err(error) => {
                if Instant::now() >= deadline {
                    // The error is no longer discarded: its kind is the
                    // whole difference between "nothing is listening here,
                    // reclaim it" and every other outcome.
                    tracing::debug!(
                        socket_path = %socket_path.display(),
                        kind = ?error.kind(),
                        %error,
                        "helper IPC connect never succeeded within the budget; treating the \
                         listener as absent"
                    );
                    return HandshakeOutcome::NoListener;
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
        _ => return HandshakeOutcome::ConnectedButSilent,
    };
    let HelperToDaemonMessage::Handshake { pid, start_time } = handshake else {
        return HandshakeOutcome::ConnectedButSilent;
    };

    let remaining = deadline.saturating_duration_since(Instant::now());
    let status_message = match tokio::time::timeout(remaining, reader.read_message::<HelperToDaemonMessage>()).await {
        Ok(Ok(Some(message))) => message,
        _ => return HandshakeOutcome::ConnectedButSilent,
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
        _ => return HandshakeOutcome::ConnectedButSilent,
    };

    if write_ndjson(&mut write_half, &DaemonToHelperMessage::HandshakeAck)
        .await
        .is_err()
    {
        return HandshakeOutcome::ConnectedButSilent;
    }

    HandshakeOutcome::Connected(Box::new(HandshakeConnection {
        reader,
        writer: write_half,
        pid,
        start_time,
        status,
        next_sequence,
    }))
}

// CONTRACT (260729 helper liveness probe, daemon side): asks a helper whether
// anyone is attached to it, on a connection of its own. Used by every site
// that has to decide about a helper it has NOT handshaked with - the
// boot-reconcile `ConnectedButSilent` row, the periodic sweep, and the
// profile GC.
//
// HARD GATE, and the reason `supports_liveness_probe` is a parameter rather
// than something read inside: `LivenessProbe` is NEVER put on the wire unless
// the helper's own registry entry declares it can parse it. A helper that
// predates the variant turns it into an `io::Error`, propagates it, and its
// exit path then SIGKILLs the user's shell and erases its registry entry -
// strictly worse than the daemon-side kill this ticket removes, because it
// leaves nothing behind to diagnose with.
//
// CONTRACT (260729 review finding F9-gate): for such a helper this function
// makes NO SOCKET CONTACT AT ALL - it does not even connect. The earlier
// version connected first (to tell "no listener" from "listening but old")
// and dropped the stream, which was itself destructive: a helper that
// predates the probe also predates `is_peer_gone`, so it accepts the
// connection, writes its `Handshake` to an already-closed peer, gets EPIPE,
// propagates it with `?`, and `run_terminal_helper`'s exit path SIGKILLs the
// user's shell. The sweep repeated that every 10s. The compatibility argument
// was about BYTES; the connection is what kills.
//
// Losing that connect loses no decision, and the equivalence is exact: the
// only two callers that reach a capability-absent entry are the sweep and the
// GC. The sweep only probes after `identity_status` returned `VerifiedOurs`,
// which means a live process whose start time matches the entry - so the
// helper process demonstrably exists and `Unsupported` -> `Leave` is the
// ticket's mandated outcome regardless of what a connect would have said; a
// dead helper never reaches the probe at all (`NoSuchProcess`/`PidReused`/
// `UnverifiableBoot` are drop-only, and never kill). `reconcile_entry` is
// unaffected either way: it only probes for `ConnectedButSilent`, which
// already proves a listener is there. The GC does not check identity, so a
// capability-absent entry whose process is already dead now survives one
// extra GC pass - bounded, because the sweep deletes that entry within its
// own 10s period and a missing entry means "reclaim" by the no-entry rule.
//
// The accepted cost is stated in the ticket: pre-upgrade helpers are spared
// unconditionally, with no expiry. Do NOT close it with a version-based kill
// ("old, therefore dead" is the same unsound inference in a new costume).
//
// This is NOT an adopt: no `HandshakeAck` is ever written here, so the sweep
// keeps its "never adopts" property while gaining the check it lacked.
pub(crate) async fn probe_helper(
    socket_path: &Path,
    supports_liveness_probe: bool,
    timeout: Duration,
) -> ProbeVerdict {
    if !supports_liveness_probe {
        return ProbeVerdict::Unsupported;
    }
    match probe_helper_reachability(socket_path, timeout).await {
        Some(verdict) => verdict,
        // No listener at all is positive absence, which the shared predicate
        // spells as "reclaim". Expressed through `Unanswered` so
        // `probe_authorizes_reclaim` stays the single decision surface.
        None => ProbeVerdict::Unanswered,
    }
}

/// `None` when there is no listener behind `socket_path` at all. Only ever
/// called for a helper whose registry entry declares the probe capability -
/// see `probe_helper`, which is the gate.
async fn probe_helper_reachability(socket_path: &Path, timeout: Duration) -> Option<ProbeVerdict> {
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
    if write_ndjson(&mut write_half, &DaemonToHelperMessage::LivenessProbe)
        .await
        .is_err()
    {
        return Some(ProbeVerdict::Unanswered);
    }

    let grace_ms = UNATTACHED_GRACE.as_millis() as u64;
    loop {
        // CONTRACT (260729 review finding F10): a PER-MESSAGE idle timeout,
        // deliberately NOT the connect deadline and deliberately NOT one
        // deadline shared across the whole exchange. Both alternatives kill
        // live helpers. An UNATTACHED helper answers from its ordinary
        // session dispatch, which first flushes the entire retained ring -
        // up to `MAX_OUTPUT_CHUNKS` (1024) chunks, each its own write+flush -
        // so a single deadline sized for a connect (400ms,
        // `DEFAULT_RECONCILE_CONNECT_TIMEOUT`) is exhausted by scrollback
        // alone, and the verdict falls through to `Unanswered`, i.e. a
        // SIGKILL for a helper whose only fault was having a full ring.
        // Restarting the clock per message makes the bound "the peer went
        // quiet", which is the thing actually being measured, and no ring
        // size can exhaust it.
        match tokio::time::timeout(
            PROBE_RESPONSE_IDLE_TIMEOUT,
            reader.read_message::<HelperToDaemonMessage>(),
        )
        .await
        {
            Ok(Ok(Some(HelperToDaemonMessage::LivenessProbeResponse {
                attached,
                unattached_for_ms,
            }))) => {
                return Some(probe_verdict_from_report(
                    attached,
                    unattached_for_ms,
                    grace_ms,
                ))
            }
            // An UNATTACHED helper answers a probe from inside its ordinary
            // session dispatch (there is no attached session for the
            // concurrent probe arm to run alongside), so the reply is
            // preceded by that path's `Handshake`/`Status` and whatever the
            // retained ring flushes. Skip past them rather than mistaking
            // them for a non-answer - this is the leg that makes a genuine
            // orphan reclaimable at all.
            Ok(Ok(Some(_))) => continue,
            _ => return Some(ProbeVerdict::Unanswered),
        }
    }
}

pub struct TerminalSession {
    id: String,
    work_root_id: WorkRootId,
    title: String,
    cwd_hint: Option<String>,
    created_at_ms: u64,
    // CONTRACT (260725 Phase 2, browser spawn profile; 260726 restart
    // provenance): provenance only - which registry profile (if any) produced
    // this session. NOT persisted to `TerminalRegistryEntry` (hard
    // constraint), but it DOES survive a daemon restart: it is written at
    // spawn to the daemon-owned sidecar
    // `agent-profiles/<terminal_id>/profile.json` and read back by
    // `recover_profile_id` in `reconcile_entry`'s adopt arm. The general
    // condition for `None` after adoption is "no readable sidecar at adopt
    // time"; the known ways to reach it are a terminal spawned before the
    // sidecar existed (no backfill, self-clears within one restart), a daemon
    // with no resolvable `state_dir` (which wrote none at spawn), a
    // `write_profile` that failed at spawn (logged as an error there, which
    // already says the terminal will report a null profile id), and a sidecar
    // missing or malformed at read time. Treat that as the known set, not a
    // closed one; every case degrades to the pre-fix `profileId: null`.
    profile_id: Option<String>,
    // CONTRACT (260725 Phase 4): mirrors `profile_id`'s shape (provenance
    // slot, not persisted to `TerminalRegistryEntry` - hard constraint) and,
    // since the 260726 fix, its recoverability too - but through a DIFFERENT
    // file: this one is read back from `terminal-tokens/<terminal_id>.json`
    // gated on `callback.json`'s presence, not from `profile.json`. The two
    // must stay separate: a hookless profile has provenance but must never be
    // handed a credential. `None` for a plain shell terminal (`hook_config: None`
    // at spawn) or when token/callback materialization failed. The only
    // reader is `TerminalRegistry::remember_token`; this field itself is
    // never logged, never serialized (no `Serialize` derive reads it), and
    // never forwarded into helper argv.
    callback_token: Option<String>,
    pid: u32,
    start_time: u64,
    write_half: Arc<AsyncMutex<IpcWriteHalf>>,
    inner: Mutex<TerminalSessionInner>,
    output_signal: watch::Sender<u64>,
    // CONTRACT (260726 Phase 1 sub-fix 3a): the reader task's `JoinHandle`
    // was previously discarded (fire-and-forget `tokio::spawn`), so nothing
    // could ever cancel it from outside - the exact mechanism that let a
    // swept-but-still-connected session's IPC reader linger. Populated once,
    // in `from_connection`, right after the reader task is spawned; taken
    // and aborted by `close_ipc_connection`.
    reader_task: Mutex<Option<tokio::task::JoinHandle<()>>>,
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
    // CONTRACT (260725 Phase 2, browser spawn profile; 260726 restart
    // provenance): read-only echo of which registry profile produced this
    // session, `null` for the unchanged default-shell path. An adopted
    // (post-restart) session reports its spawn profile like any other, except
    // when no readable sidecar survived to adopt time - see
    // `TerminalSession::profile_id`'s CONTRACT for the known ways that
    // happens.
    profile_id: Option<String>,
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
    // CONTRACT (260725 Phase 2, browser spawn profile): opaque id into
    // `agent_profile_registry`. Absent (the common case) keeps today's
    // shell-spawn behavior byte for byte - see
    // `resolve_create_command`'s `None` branch. `#[serde(default)]` so an
    // older/unaware client body (no `profileId` key) still deserializes.
    #[serde(default)]
    profile_id: Option<String>,
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
    let (command, env_overlay, scrub, hook_config) =
        match resolve_create_command(request.profile_id.as_deref()) {
            Ok(resolved) => resolved,
            Err(error) => return error.into_response(),
        };

    match TerminalSession::spawn(
        &state.terminals,
        &state.terminals.helper_binary,
        &state.terminals.registry_dir,
        state.terminals.connect_timeout,
        state.terminals.state_dir.as_deref(),
        &state.terminals.base_url,
        work_root_id,
        root_path,
        request.title.unwrap_or_else(|| "Terminal".to_owned()),
        columns,
        rows,
        request.cwd_hint,
        command,
        env_overlay,
        // CONTRACT (260725 Phase 2): provenance is the request's own
        // (already-validated-by-`resolve_create_command`) profile id, not a
        // second registry lookup - `resolve_create_command` already proved
        // this id resolves or this call site would have returned early
        // above.
        request.profile_id,
        scrub,
        hook_config,
    )
    .await
    {
        Ok(session) => {
            let view = session.view();
            match state.terminals.insert(session.clone()).await {
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

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct TerminalTurnStateRequest {
    token: String,
    // CONTRACT: deliberately a raw `String`, not `agent_turn_state::TurnState`
    // directly - see that module's own CONTRACT for why parsing this field
    // is deferred until AFTER the token check below, rather than left to
    // axum's `Json` extractor (which would reject an unrecognized value
    // before this handler body - and therefore the token check - ever runs).
    state: String,
}

// CONTRACT (ticket "the route pair" / router.rs wiring): registered in the
// OUTER router chain, structurally outside `require_owner_auth` - see
// `router.rs::build_router`'s CONTRACT comment. This is the one HTTP route in
// this crate authorized by a per-terminal opaque token instead of the owner
// session cookie: the caller is a vendor CLI hook firing from inside a
// spawned agent terminal (`terminal-notify`, `terminal_notify.rs`), never a
// browser, and it never receives - and could not present - an owner session
// cookie.
//
// CONTRACT (Design Answer 3): an unknown `terminal_id` and a wrong token
// return the EXACT SAME response (`terminal_error(UNAUTHORIZED,
// "unauthorized")`) - never let a caller distinguish "no such terminal" from
// "wrong token" by status code or body. The turn-state value is only parsed
// (and only rejected as `BAD_REQUEST` on an unrecognized value) AFTER this
// check passes, so a probing caller with no valid token can never even learn
// whether `state`'s value would have been acceptable.
//
// CONTRACT (260725 Phase 5): after the token and state checks below pass,
// this handler resolves the session's `work_root_id` and calls
// `AttentionHub::record_and_publish`, which both updates the snapshot map
// `agent_attention::attention_events` serves on connect AND broadcasts an
// `attention` SSE frame to already-subscribed streams. Unlike Phase 4, the
// parsed value no longer dead-ends here. An unknown `terminal_id` at this
// point (the `state.terminals.get` lookup below) is a defensive no-op, not a
// panic - `token_for` already proved the terminal is known, but this method
// re-reads `sessions` under a separate lock, so treat a race as harmless
// rather than assume it cannot happen.
pub async fn post_terminal_turn_state(
    State(state): State<AppState>,
    AxumPath(terminal_id): AxumPath<String>,
    Json(request): Json<TerminalTurnStateRequest>,
) -> Response {
    let Some(expected_token) = state.terminals.token_for(&terminal_id) else {
        return terminal_error(StatusCode::UNAUTHORIZED, "unauthorized");
    };
    if !crate::agent_turn_state::tokens_match(&expected_token, &request.token) {
        return terminal_error(StatusCode::UNAUTHORIZED, "unauthorized");
    }
    let Some(turn_state) = crate::agent_turn_state::parse_turn_state(&request.state) else {
        return terminal_error(StatusCode::BAD_REQUEST, "invalid turn state");
    };
    if let Some(session) = state.terminals.get(&terminal_id) {
        state
            .attention
            .record_and_publish(terminal_id, session.work_root_id.clone(), turn_state);
    }
    StatusCode::NO_CONTENT.into_response()
}

// CONTRACT (260726 Phase 1, hop-1 env-plan guard): hop 1's env decision is
// expressed as a VALUE computed by this pure function, so a test can assert
// on the decision itself instead of trying to read it back off a built
// `std::process::Command` - which is impossible, because std exposes no
// public API distinguishing "no env method was ever called" from
// "env_clear() called with nothing re-added" (see the application site in
// `build_helper_command` and the CONTRACT in
// `helper_spawn_default_no_command_matches_existing_arg_shape`). The
// previous guard could only sniff `Command`'s unix `Debug` string, which
// made it fragile and, on the default path, effectively unfalsifiable.
// `ClearAndSet` is keyed strictly off `command.is_some()`, mirroring
// `build_helper_command`'s own argv branch on the same `command` reference.
#[derive(Debug, Eq, PartialEq)]
enum HelperEnvPlan {
    /// No explicit command: hop 1 inherits the daemon's environment
    /// untouched - neither `.env()` nor `.env_clear()` is called.
    InheritHost,
    /// Explicit command: hop 1 clears the environment and re-adds exactly
    /// these scrubbed pairs.
    ClearAndSet(Vec<(std::ffi::OsString, std::ffi::OsString)>),
}

fn helper_env_plan(
    command: Option<&(String, Vec<String>)>,
    scrub: Option<&crate::agent_env_profile::EnvScrubProfile>,
    host_env: impl IntoIterator<Item = (std::ffi::OsString, std::ffi::OsString)>,
) -> HelperEnvPlan {
    match command {
        None => HelperEnvPlan::InheritHost,
        Some(_) => {
            // Defensive, non-panicking fallback for a direct (unit-test)
            // caller passing `command = Some(..)` with `scrub = None`. This
            // is NOT a second live resolution: the only production caller,
            // `build_helper_command`, resolves the fallback once at its own
            // top and always passes `Some` here, so both consumers (this
            // scrub and the `--scrub-marker` argv loop) provably read the
            // same resolved profile - see that function's CONTRACT.
            let scrub = scrub.unwrap_or(&crate::agent_env_profile::CLAUDE);
            HelperEnvPlan::ClearAndSet(crate::agent_env_profile::scrub_env_os(host_env, scrub))
        }
    }
}

// CONTRACT (260725 Phase 1, pty-agent spawn-seam argv/env scrub; extended
// Phase 2, browser spawn profile): pure builder extracted from
// `TerminalSession::spawn` so the "default (no explicit command) path is
// byte-for-byte unchanged" contract is testable without spawning a real
// process. When `command` is `None`, this function must build the exact
// same arg chain/stdio as before this phase and must call neither `.env()`
// nor `.env_clear()` - that is hop 1's half of the regression guard. When
// `command` is `Some`, this hop scrubs `host_env` against `scrub` (the
// resolved profile's own deny-list - Phase 2 no longer hardcodes `CLAUDE`
// here, see `resolve_create_command`) as defense-in-depth: hop 2 (the
// helper's own shell spawn) does its own independent scrub of its inherited
// env, but that inherited env is seeded from hop 1's env at process-spawn
// time, so a hop-1 regression would otherwise leave the helper's base env
// dirty even if hop 2's own scrub step were correct - see plan Codebase
// Findings. `scrub`'s fallback is resolved once at the top of this function
// (260726 Phase 1) but is only ever CONSUMED on the `command.is_some()`
// path, so its value stays inert on the default path; callers still pass
// `None` there for the same reason `command` is `None` - one resolved
// profile, one paired scrub list, never independently defaulted.
#[allow(clippy::too_many_arguments)]
fn build_helper_command(
    helper_binary: &Path,
    registry_dir: &Path,
    terminal_id: &str,
    work_root_id: &str,
    spawn_cwd: &Path,
    title: &str,
    columns: u16,
    rows: u16,
    cwd_hint: Option<&str>,
    socket_path: &Path,
    command: Option<&(String, Vec<String>)>,
    env_overlay: &[(String, String)],
    scrub: Option<&crate::agent_env_profile::EnvScrubProfile>,
    host_env: impl IntoIterator<Item = (std::ffi::OsString, std::ffi::OsString)>,
) -> std::process::Command {
    // CONTRACT (260725 Phase 2; hoisted 260726 Phase 1): `scrub` must be
    // `Some` whenever `command` is `Some` - `resolve_create_command` returns
    // them paired, one resolved profile producing both. Falling back to
    // `CLAUDE` (the strictest list this codebase knows) rather than
    // panicking or silently skipping the scrub keeps a defensive
    // caller-error path safe instead of leaving a dirty env - see the hop-1
    // defense-in-depth CONTRACT above. Resolved HERE, exactly once per call,
    // and unconditionally on `command`, so the two consumers below - the
    // `--scrub-marker` argv loop (which threads markers to hop 2) and the
    // `helper_env_plan` application site (which scrubs hop 1's own env) -
    // provably read the SAME profile and cannot default independently.
    let scrub = scrub.unwrap_or(&crate::agent_env_profile::CLAUDE);
    let mut helper_command = std::process::Command::new(helper_binary);
    helper_command
        .arg("terminal-helper")
        .arg("--registry-dir")
        .arg(registry_dir)
        .arg("--terminal-id")
        .arg(terminal_id)
        .arg("--work-root-id")
        .arg(work_root_id)
        .arg("--cwd")
        .arg(spawn_cwd)
        .arg("--title")
        .arg(title)
        .arg("--columns")
        .arg(columns.to_string())
        .arg("--rows")
        .arg(rows.to_string())
        .arg("--socket-path")
        .arg(socket_path);
    if let Some(hint) = cwd_hint {
        helper_command.arg("--cwd-hint").arg(hint);
    }
    if let Some((program, args)) = command {
        helper_command.arg("--command").arg(program);
        for arg in args {
            helper_command.arg("--command-arg").arg(arg);
        }
        // CONTRACT (review cycle 1, finding C3): `--env-overlay` values land
        // verbatim in this argv, which is world-readable via `ps` - see the
        // full CONTRACT on `TerminalHelperArgs::env_overlay` in `cli.rs`.
        // Never route a secret (in particular, the parent ticket's Phase 4
        // callback token) through this loop.
        for (key, value) in env_overlay {
            helper_command.arg("--env-overlay").arg(format!("{key}={value}"));
        }
        // CONTRACT (review cycle 1, finding C1): thread this SAME resolved
        // `scrub` list to hop 2 via `--scrub-marker`, so the helper's own
        // shell spawn (`terminal_helper_process.rs::apply_scrub_and_overlay`)
        // scrubs the profile's actual markers instead of independently
        // hardcoding `CLAUDE`. Without this, a profile whose markers are not
        // a subset of `CLAUDE`'s would be scrubbed at hop 1 and NOT at hop
        // 2 - the hop that actually seeds the PTY child's env.
        for marker in scrub.markers {
            helper_command.arg("--scrub-marker").arg(*marker);
        }
    }
    // CONTRACT (260726 Phase 1, hop-1 env-plan guard): the guarded surface is
    // this plan VALUE, not the `Command` built below - `std::process::Command`
    // exposes no public API to tell "no env method ever called" apart from
    // "env_clear() called with nothing re-added" (both report an empty
    // `get_envs()` iterator), so a Debug-string sniff was the only prior
    // observable and it is fragile (see the unix secondary detector in
    // `helper_spawn_default_no_command_matches_existing_arg_shape` and its own
    // CONTRACT). Note `command.is_some()` is evaluated TWICE in this function -
    // once by the `if let Some((program, args))` argv branch above (which owns
    // --command/--command-arg/--env-overlay/--scrub-marker) and once by
    // `helper_env_plan` below - but both read the SAME `command` reference, so
    // they cannot diverge. KNOWN RESIDUAL: an `env_clear()` written directly
    // into this function outside this one application site is invisible to
    // this guard (and to std's public API) on every platform; the hardened
    // unix secondary detector in that test is the only thing that can still
    // catch that specific case.
    match helper_env_plan(command, Some(scrub), host_env) {
        HelperEnvPlan::InheritHost => {}
        HelperEnvPlan::ClearAndSet(env) => {
            helper_command.env_clear().envs(env);
        }
    }
    helper_command
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null());
    helper_command
}

// CONTRACT (review cycle 1, finding C2): `env_overlay` is only meaningful
// (and only ever applied, at hop 2) when an explicit `command` is present -
// see `TerminalHelperArgs::env_overlay`'s CONTRACT in `cli.rs`. Hop 2
// enforces this at the clap boundary (`requires = "command"`); hop 1
// constructs the helper's argv directly rather than parsing it, so it needs
// its own explicit guard rather than relying on clap. Extracted as a small
// pure predicate so it is unit-testable without exercising the rest of
// `spawn`'s async setup (registry dir, socket path, handshake, ...).
fn validate_command_env_overlay_pairing(
    command: &Option<(String, Vec<String>)>,
    env_overlay: &[(String, String)],
) -> Result<(), TerminalError> {
    if command.is_none() && !env_overlay.is_empty() {
        return Err(TerminalError::BadRequest(
            "env_overlay requires an explicit command",
        ));
    }
    Ok(())
}

// CONTRACT (260725 Phase 2, browser spawn profile): pure resolver extracted
// from `create_terminal` so the "absent `profile_id` is a literal
// no-branch-taken path, not a branch that happens to compute the same
// values" contract is unit-testable without spawning a real process or
// going through the HTTP handler - mirrors this file's existing
// `validate_command_env_overlay_pairing`/`build_helper_command` pure-seam
// pattern (Phase 1 Result, finding 4: the equivalent env-overlay guard was
// silently vacuous until it got its own dedicated, mutation-caught test).
// `command` and `scrub` are always returned paired (`None`/`None` or
// `Some`/`Some`) - never independently defaulted - so `build_helper_command`
// never has to guess which scrub list belongs to an explicit command. An
// unknown `profile_id` is a `TerminalError::BadRequest`, mirroring the
// existing `validate_size` early-return shape.
fn resolve_create_command(
    profile_id: Option<&str>,
) -> Result<
    (
        Option<(String, Vec<String>)>,
        Vec<(String, String)>,
        Option<&'static crate::agent_env_profile::EnvScrubProfile>,
        Option<crate::agent_profile_registry::HookConfigShape>,
    ),
    TerminalError,
> {
    let Some(id) = profile_id else {
        return Ok((None, Vec::new(), None, None));
    };
    let profile = crate::agent_profile_registry::resolve(id)
        .ok_or(TerminalError::BadRequest("unknown terminal profile"))?;
    let command = Some((
        profile.command.to_owned(),
        profile.args.iter().map(|arg| (*arg).to_owned()).collect(),
    ));
    // Phase 2 does not populate env_overlay - no secret/value needs to
    // travel yet (Phase 4 owns the callback token and is explicitly barred
    // from `--env-overlay` regardless of this seam).
    Ok((command, Vec::new(), Some(profile.scrub), profile.hook_config))
}

/// Tears down every terminal on this daemon, helper processes included: drains
/// the whole registry and `terminate()`s each session (graceful IPC shutdown +
/// verified-PID kill, which also collapses each helper's kill-on-close job and
/// its child shell). This is the deliberate, UI-native counterpart to a blanket
/// `taskkill /IM` - unlike a daemon shutdown, it does NOT preserve terminals.
/// Global teardown, so it bypasses the per-terminal work-root access check that
/// `close_terminal` applies. Returns the number of terminals closed.
pub async fn close_all_terminals(State(state): State<AppState>) -> Response {
    let sessions = state.terminals.drain_all();
    let closed = sessions.len();
    for session in sessions {
        session.terminate().await;
    }
    Json(serde_json::json!({ "closed": closed })).into_response()
}

impl TerminalSession {
    #[allow(clippy::too_many_arguments)]
    async fn spawn(
        registry: &TerminalRegistry,
        helper_binary: &Path,
        registry_dir: &Path,
        connect_timeout: Duration,
        state_dir: Option<&Path>,
        base_url: &str,
        work_root_id: WorkRootId,
        root_path: PathBuf,
        title: String,
        columns: u16,
        rows: u16,
        cwd_hint: Option<String>,
        command: Option<(String, Vec<String>)>,
        env_overlay: Vec<(String, String)>,
        profile_id: Option<String>,
        scrub: Option<&'static crate::agent_env_profile::EnvScrubProfile>,
        hook_config: Option<crate::agent_profile_registry::HookConfigShape>,
    ) -> Result<Arc<Self>, TerminalError> {
        validate_command_env_overlay_pairing(&command, &env_overlay)?;
        let (spawn_cwd, normalized_cwd_hint) = resolve_terminal_cwd(&root_path, cwd_hint)?;
        let id = opaque_terminal_id();
        let socket_path = registry_dir.join(format!("{id}.sock"));

        // CONTRACT (260725 Phase 3 step 2): the terminal id above is the
        // FIRST point in this spawn path where a per-terminal id exists, and
        // this is the last point before `build_helper_command` builds the
        // helper argv - hook-config materialization needs the id (for
        // `agent-profiles/<terminal_id>/`) and must land its `--settings`
        // path into `command`'s args before that call, so it happens here,
        // not inside `resolve_create_command` (called before any id exists)
        // and not inside `build_helper_command` (a pure argv builder with no
        // filesystem access, deliberately kept that way).
        let mut command = command;
        let mut callback_token: Option<String> = None;
        // CONTRACT (260726 profile-provenance fix, GATE IS LOAD-BEARING):
        // this branch is gated on `profile_id.is_some()`, NOT
        // `hook_config.is_some()`. `resolve_create_command` (this file) is
        // the only producer of both values and returns `hook_config` only
        // for a profile it already resolved, so `hook_config.is_some()`
        // strictly implies `profile_id.is_some()` but not the reverse: a
        // resolved HOOKLESS profile (`dummy-echo`, `hook_config: None`)
        // reaches this line with a `Some` profile id and must still get a
        // `profile.json` sidecar - that is the whole point of the fix.
        // Widening the gate is therefore what makes the pending-mark hoist
        // below mandatory rather than cosmetic.
        if let Some(resolved_profile_id) = profile_id.as_deref() {
            // FIX (review cycle 1, finding E): a `None` state dir used to
            // fall back to `std::env::temp_dir()`, landing an EXECUTED
            // command line (`settings.json`'s hook `command` string) under
            // the predictable, world-writable `/tmp/agent-profiles/` - a
            // local attacker who pre-creates/owns that path could replace
            // the file with one whose command runs as the daemon's user.
            // Degrading to a hookless spawn costs nothing (turn-attention
            // signaling is best-effort UX, not correctness-critical - see
            // the materialization-failure branch below, which already
            // degrades the same way) and removes the exposure entirely.
            match state_dir {
                Some(state_dir) => {
                    let profile_dir = state_dir.join("agent-profiles").join(&id);
                    // CONTRACT (260725 Phase 4): `agent-profiles/<terminal_id>/`
                    // is created here and reclaimed only by the GC sweep
                    // (`agent_profile_gc.rs`), never on terminal close - the
                    // sweep is the sole cleanup path, driven by terminal
                    // liveness rather than a close-time hook here.
                    //
                    // CONTRACT (260725 Phase 4 review cycle 1, finding A -
                    // LOAD-BEARING, closes the concurrent-spawn GC race):
                    // mark this id pending in the registry BEFORE the first
                    // byte of `agent-profiles/<id>/` is created below (the
                    // upcoming `write_profile`/`write_token`/
                    // `write_callback_target` calls are exactly what
                    // `create_dir_all`s it). From this line
                    // until either `insert`/`insert_unchecked` clears the
                    // mark (success) or one of this function's own later
                    // `?`/early-return failure paths clears it directly
                    // (helper spawn failure, handshake timeout), the id is a
                    // member of `live_terminal_ids()` even though it is not
                    // yet - and may never be - in `sessions`. See
                    // `pending_profile_ids`'s field doc on `TerminalRegistry`
                    // for the full argument that this brackets the race with
                    // no gap, unlike shrinking the sweep's snapshot window.
                    //
                    // CONTRACT (260726 profile-provenance fix, WHY THIS LINE
                    // MOVED OUT OF THE HOOK-CONFIG BRANCH): this mark used to
                    // live inside the nested `hook_config.is_some()` branch
                    // below, which was sound only while that branch was the
                    // ONLY creator of `agent-profiles/<id>/`. The sidecar
                    // write below now creates that directory for a hookless
                    // profile too, i.e. on a path the old mark did not cover,
                    // so leaving the mark nested would reopen exactly the
                    // concurrent-spawn GC race `pending_profile_ids` exists
                    // to close. It must stay ahead of EVERY directory-creating
                    // call in this block, hooked or hookless.
                    registry.mark_profile_pending(&id);
                    // Provenance sidecar for ANY resolved profile (hooked or
                    // hookless) - read back by `recover_profile_id` on
                    // boot-reconcile adopt. A write failure degrades to a
                    // logged error and the spawn continues: a terminal must
                    // never fail to start over provenance metadata, and the
                    // observable result of the degrade is exactly today's
                    // pre-fix behavior (`profileId: null` after a restart).
                    if let Err(error) =
                        crate::agent_profile_store::write_profile(&profile_dir, resolved_profile_id)
                    {
                        tracing::error!(
                            terminal_id = %id,
                            %error,
                            "failed to write the spawn-profile provenance sidecar; this \
                             terminal will report a null profile id if it is re-adopted after \
                             a daemon restart"
                        );
                    }
                    // Everything below is unchanged hook-config work, now
                    // nested one level deeper: it stays gated on
                    // `hook_config.is_some()` so a hookless profile still
                    // mints no callback token and materializes no
                    // `settings.json` - only the sidecar above is new for it.
                    if let (Some(hook_config), Some((_, args))) = (hook_config, command.as_mut()) {
                        // CONTRACT (260725 Phase 4, token generation and write
                        // order - load-bearing): the token and `callback.json`
                        // are written BEFORE `materialize_hook_config` below, so
                        // the vendor `settings.json` this call produces always
                        // points its `--callback` argv at a file that already
                        // exists (even if empty/stale from a write failure) by
                        // the time the spawned process can possibly fire a
                        // hook. Generated once per fresh spawn, never rotated
                        // (Design Answer 1) - `reconcile_entry`'s adopt arm
                        // recovers this SAME token on restart rather than
                        // regenerating it.
                        let token = generate_callback_token();
                        let write_result =
                            crate::agent_token_store::write_token(state_dir, &id, &token).and_then(
                                |()| {
                                    crate::agent_callback::write_callback_target(
                                        &profile_dir,
                                        base_url,
                                        &id,
                                        &token,
                                    )
                                },
                            );
                        match write_result {
                            Ok(()) => callback_token = Some(token),
                            Err(error) => tracing::error!(
                                terminal_id = %id,
                                %error,
                                "failed to write callback token or target; turn-state hooks for \
                                 this terminal will not authenticate"
                            ),
                        }

                        let callback_path = crate::agent_callback::callback_path(&profile_dir);
                        match crate::agent_hook_config::materialize_hook_config(
                            &profile_dir,
                            &hook_config,
                            &default_helper_binary(),
                            &callback_path,
                        ) {
                            Ok(settings_path) => {
                                args.push("--settings".to_owned());
                                args.push(settings_path.display().to_string());
                            }
                            Err(error) => tracing::error!(
                                terminal_id = %id,
                                %error,
                                "failed to materialize agent hook config; spawning without hooks"
                            ),
                        }
                    }
                }
                // CONTRACT (`crates/daemon/tests/agent_hook_missing_state_dir.rs`
                // asserts the literal substring "no persistent state directory
                // resolved" against real daemon stdout): keep that phrase
                // intact when rewording this warning.
                None => tracing::warn!(
                    terminal_id = %id,
                    "no persistent state directory resolved; spawning without agent hooks \
                     and without recording spawn-profile provenance, rather than materializing \
                     an executed command line under a predictable, world-writable temp path"
                ),
            }
        }

        let command_to_spawn = build_helper_command(
            helper_binary,
            registry_dir,
            &id,
            work_root_id.as_str(),
            &spawn_cwd,
            &title,
            columns,
            rows,
            normalized_cwd_hint.as_deref(),
            &socket_path,
            command.as_ref(),
            &env_overlay,
            scrub,
            std::env::vars_os(),
        );

        let spawn_outcome =
            tokio::task::spawn_blocking(move || crate::terminal_platform::spawn_detached(command_to_spawn))
                .await;
        // CONTRACT (260725 Phase 4 review cycle 1, finding A): this function
        // returns `Err` below without ever reaching `insert`/
        // `insert_unchecked`, so if `mark_profile_pending` ran above, this
        // is the ONE place that ever will clear it for this attempt - an
        // unconditional call is correct (and a harmless no-op) whether or
        // not this spawn's `hook_config` branch actually marked pending.
        if spawn_outcome.is_err() || matches!(spawn_outcome, Ok(Err(_))) {
            registry.clear_profile_pending(&id);
        }
        spawn_outcome
            .map_err(|_| TerminalError::BadRequest("terminal spawn failed"))?
            .map_err(|_| TerminalError::BadRequest("terminal spawn failed"))?;

        // CONTRACT (260729, Site C unchanged): this cleanup path is
        // deliberately NOT probe-gated. It kills a helper this daemon just
        // spawned under a freshly generated id, so it cannot be another
        // daemon's busy helper and ownership is not in question - the
        // misjudgment this ticket fixes does not exist here. The only change
        // is the shape of the value being matched: every non-`Connected`
        // outcome takes exactly the branch the former `None` did.
        let HandshakeOutcome::Connected(connected) =
            connect_and_handshake(&socket_path, connect_timeout).await
        else {
            // CONTRACT (260725 Phase 4 review cycle 1, finding A): same
            // reasoning as the spawn-failure branch above - this function
            // returns `Err` below without ever reaching `insert`, so this is
            // the only place left that will clear a pending mark for this
            // attempt.
            registry.clear_profile_pending(&id);
            // CONTRACT (260725 Phase 1, fail-loudly finding): the helper is
            // spawned with all three standard streams to `/dev/null`
            // (above) and dispatches before `logging::init` in `main.rs`, so
            // an `Err` from the helper's own process (e.g. a failed
            // self-identity lookup in `run_terminal_helper`) is otherwise
            // completely invisible - nothing about the generic "terminal
            // spawn failed" response below distinguishes "helper crashed
            // before writing its registry entry" from "helper wrote the
            // entry but the daemon could not connect/handshake in time".
            // Checking for the registry entry file here, from the
            // daemon side, is the cheapest way to surface that distinction
            // without risking two OS processes (daemon + helper) racing to
            // write the same rolling log file. Deliberately a plain
            // existence check rather than a `read_registry_entry(...)
            // .is_some()` reuse of the read below: a file that exists but
            // fails to parse must still log "wrote an entry", not "never
            // wrote one".
            if registry_entry_path(registry_dir, &id).exists() {
                tracing::error!(
                    terminal_id = %id,
                    socket_path = %socket_path.display(),
                    "terminal helper wrote a registry entry but the daemon could not connect \
                     or complete the handshake before the connect timeout"
                );
            } else {
                tracing::error!(
                    terminal_id = %id,
                    "terminal helper never wrote a registry entry before the connect timeout - \
                     likely failed during startup (e.g. self-identity lookup) before reaching \
                     write_registry_entry"
                );
            }
            // CONTRACT (260726 Phase 1 sub-fix 1): a helper that was
            // spawned but never completed the handshake within budget must
            // not leak - read back the registry entry it may have already
            // durably written (see `write_registry_entry`'s ordering
            // CONTRACT in `terminal_helper_process.rs`: the entry lands
            // BEFORE the shell is ever spawned) and, if present, kill it
            // through a verified handle. Either way, prune both registry
            // files - covers both "the helper wrote its entry but never
            // handshaked" and "the helper never even wrote the file" edges.
            let registry_dir_owned = registry_dir.to_path_buf();
            let id_for_read = id.clone();
            let entry = tokio::task::spawn_blocking(move || {
                read_registry_entry(&registry_dir_owned, &id_for_read)
            })
            .await
            .unwrap_or(None);
            match entry {
                Some(entry) => {
                    kill_verified_and_delete_entry(
                        registry_dir,
                        &id,
                        entry.pid,
                        entry.start_time,
                        entry.boot_id.as_deref(),
                    )
                    .await;
                }
                None => delete_registry_entry(registry_dir, &id),
            }
            return Err(TerminalError::BadRequest("terminal spawn failed"));
        };

        Ok(Self::from_connection(
            id,
            work_root_id,
            title,
            normalized_cwd_hint,
            now_ms(),
            *connected,
            columns,
            rows,
            profile_id,
            callback_token,
        ))
    }

    #[allow(clippy::too_many_arguments)]
    fn from_connection(
        id: String,
        work_root_id: WorkRootId,
        title: String,
        cwd_hint: Option<String>,
        created_at_ms: u64,
        connected: HandshakeConnection,
        columns: u16,
        rows: u16,
        profile_id: Option<String>,
        callback_token: Option<String>,
    ) -> Arc<Self> {
        let grace_until_ms = (connected.status != TerminalStatus::Running)
            .then(|| now_ms() + DAEMON_GRACE_WINDOW_MS);
        let session = Arc::new(Self {
            id,
            work_root_id,
            title,
            cwd_hint,
            created_at_ms,
            profile_id,
            callback_token,
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
            reader_task: Mutex::new(None),
        });
        let reader_task = spawn_ipc_reader_task(session.clone(), connected.reader);
        *session
            .reader_task
            .lock()
            .expect("terminal session lock poisoned") = Some(reader_task);
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
            profile_id: self.profile_id.clone(),
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
    // visible/attachable even though `is_live()` is false. `write_input`/
    // `resize` deliberately keep the strict Running-only `is_live()` check;
    // the WS upgrade gate, the work-root listing, and BOTH eviction
    // `retain`s (`insert`'s lazy path and the periodic sweep's
    // `sweep_evict_expired` - review I3) use this relaxed predicate instead,
    // since the 30s attach grace is the binding authority for when a
    // session may be dropped/disconnected, not merely for when it may still
    // be attached to.
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
    //
    // CONTRACT (boot-identity gate, deliberately NOT applied here): unlike
    // the registry-entry kill path (`kill_verified_and_delete_entry`), this
    // one's `pid`/`start_time` do not come from a file that can outlive a
    // reboot - they come from a `HandshakeConnection` this daemon process
    // completed over live IPC (`from_connection`, the only constructor of a
    // `TerminalSession`). A completed handshake is positive proof the helper
    // was running in THIS boot, so the recorded start time is by construction
    // same-boot and comparable. Adding a boot gate here would guard nothing;
    // do not "unify" the two paths on that basis.
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

    // CONTRACT (260726 Phase 1 sub-fix 3a): the periodic sweep's eviction
    // step must actually close the daemon<->helper IPC connection once
    // `admits_attach()` goes false, not merely drop the daemon-side
    // `Arc<TerminalSession>` out of the registry map - `IpcReadHalf`/
    // `IpcWriteHalf` are `tokio::io::split()` halves sharing the underlying
    // stream's state, so dropping only one half does not close the
    // underlying socket. Both halves must be torn down: abort the reader
    // task (owns the read half) AND `.shutdown()` the write half. The write
    // shutdown half-closes the transport, which is what makes the helper's
    // own reader observe EOF and fall into its
    // `if shared.exited_at().is_some() { break; }` self-exit check - no
    // new helper-side logic is needed for this daemon-initiated-disconnect
    // path. An already-broken pipe on `.shutdown()` is expected/harmless
    // (discarded, matching the existing `let _ = write_ndjson(...)` style
    // used elsewhere in this file).
    async fn close_ipc_connection(&self) {
        let reader_task = self
            .reader_task
            .lock()
            .expect("terminal session lock poisoned")
            .take();
        if let Some(reader_task) = reader_task {
            reader_task.abort();
        }
        let mut writer = self.write_half.lock().await;
        let _ = writer.shutdown().await;
    }
}

fn spawn_ipc_reader_task(
    session: Arc<TerminalSession>,
    mut reader: NdjsonReader<IpcReadHalf>,
) -> tokio::task::JoinHandle<()> {
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
                Ok(Some(HelperToDaemonMessage::LivenessProbeResponse { .. })) => {
                    // CONTRACT (260729): unreachable on an ATTACHED
                    // connection - the probe is request-gated and this
                    // session connection never sends `LivenessProbe` (probes
                    // always open their own connection, see `probe_helper`).
                    // Ignored rather than treated as a protocol desync
                    // precisely so a stray one can never take this session's
                    // IPC down and trigger the eviction/kill path.
                }
                Ok(None) | Err(_) => {
                    session.mark_ipc_closed();
                    break;
                }
            }
        }
    })
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

    let mut heartbeat = interval(WS_HEARTBEAT_INTERVAL);
    // Default `MissedTickBehavior::Burst` would fire every accumulated tick
    // back-to-back after the loop is blocked longer than one interval (slow
    // backfill, contended runtime), emitting several Pings in immediate
    // succession. `Delay` keeps this to one liveness probe per interval,
    // matching the stated one-probe-per-15s intent.
    heartbeat.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
    heartbeat.tick().await; // first tick fires immediately; consume it so the interval starts counting from "now"
    let mut last_activity = Instant::now();

    loop {
        tokio::select! {
            maybe_message = receiver.next() => {
                let Some(Ok(message)) = maybe_message else { break; };
                last_activity = Instant::now();
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
            _ = heartbeat.tick() => {
                if last_activity.elapsed() > WS_HEARTBEAT_IDLE_TIMEOUT {
                    // Half-open peer: no inbound frame (including a Pong
                    // reply to our own Ping) within the idle timeout.
                    break;
                }
                if sender.send(Message::Ping(Vec::new().into())).await.is_err() {
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

    // ---------------------------------------------------------------
    // Source-scan helpers (260727 Phase 1, Invariants 1-3 below): these
    // count textual call sites in this file's own compiled-out source, not
    // runtime behavior. They exist because `std::process::Command`'s
    // `env_clear()` and `self.tokens`'s access pattern both have a real
    // observability gap that no runtime assertion can close: see
    // `ai-docs/mental-model/ws-web-dashboard/terminal.md:64` for the
    // concrete `env_clear()`/`get_envs()` case (an empty iterator either
    // way), and `token_for`'s own CONTRACT above for the symmetric
    // `self.tokens` argument - an extra reader of that map is runtime-
    // indistinguishable from `token_for` reading it once more. A
    // VALUE-level test - like `helper_env_plan`'s own assertion in
    // `helper_spawn_default_no_command_matches_existing_arg_shape` - cannot
    // see "a second call site was written somewhere else in this file";
    // these scans are the SECONDARY guard for exactly that residual, never
    // a replacement for a behavioral proof.
    // ---------------------------------------------------------------

    /// This file's own source (`include_str!` of itself, resolved relative
    /// to this file with no `CARGO_MANIFEST_DIR`/`file!()` plumbing needed -
    /// same self-referential pattern `events.rs`/`mock.rs` use for their own
    /// sibling fixtures) with every `#[cfg(test)]`-gated span excised and
    /// every comment-only line then dropped.
    ///
    /// Excise rule: walk lines; a line that is EXACTLY `#[cfg(test)]` at
    /// column 0 drops itself and every following line up to and including
    /// the next line that is EXACTLY `}` at column 0 - the shape all of
    /// this file's `#[cfg(test)]` markers use (this module, plus the
    /// test-only fixture functions below it). Comment-stripping (any line
    /// whose trimmed form starts with `//`) runs AFTER excision, on the
    /// surviving lines only.
    ///
    /// ASSUMPTION: every column-0 `#[cfg(test)]` in this file annotates a
    /// braced item. A future `#[cfg(test)]` on a non-braced item (e.g.
    /// `#[cfg(test)] use ...;` or a bare `const`) has no column-0 `}` of its
    /// own, so this rule would skip forward to the NEXT unrelated column-0
    /// `}` and over-excise real production code in between. Symptom: the
    /// scan counts below drop unexpectedly (never silently pass) because
    /// the excised span swallowed a counted call site - recognizable
    /// immediately as a scan-helper bug, not a production regression, if
    /// this comment is read first.
    fn production_text() -> String {
        const SOURCE: &str = include_str!("terminal.rs");
        let mut lines = SOURCE.lines();
        let mut kept = Vec::new();
        while let Some(line) = lines.next() {
            if line == "#[cfg(test)]" {
                for gated in lines.by_ref() {
                    if gated == "}" {
                        break;
                    }
                }
                continue;
            }
            kept.push(line);
        }
        kept.retain(|line| !line.trim_start().starts_with("//"));
        kept.join("\n")
    }

    /// `text` with every whitespace character removed. Callers must only
    /// ever apply this to the OUTPUT of `production_text()` - flattening the
    /// raw, comment-bearing source FIRST would collapse a backticked
    /// identifier inside a CONTRACT comment (e.g. `` `self.tokens` ``)
    /// against a real field access and inflate a scan's count. Comment-strip
    /// first, flatten second - never the reverse.
    fn flattened(text: &str) -> String {
        text.chars().filter(|c| !c.is_whitespace()).collect()
    }

    // Invariant 1 (260727 Phase 1): the PRIMARY guard for hop 1's env
    // decision is the VALUE-level `helper_env_plan` assertion in
    // `helper_spawn_default_no_command_matches_existing_arg_shape` above -
    // this scan is the SECONDARY guard for the one residual a value
    // assertion cannot see: an `env_clear()` written directly into
    // `build_helper_command` outside its single plan-application arm (see
    // that function's own CONTRACT). Comment-stripped but deliberately NOT
    // flattened - `.env_clear(` never spans a line break in this file today.
    #[test]
    fn terminal_rs_has_exactly_one_production_env_clear() {
        let count = production_text().matches(".env_clear(").count();
        assert_eq!(
            count, 1,
            "expected exactly one production `.env_clear(` call site (in \
             `build_helper_command`'s `HelperEnvPlan::ClearAndSet` arm); \
             found {count} - if this moved, update `build_helper_command`'s \
             CONTRACT and this count together"
        );
    }

    // Invariant 2, structural half (260727 Phase 1; recounted at the
    // 260728 sweep/eviction merge): counts *methods that take the `sessions`
    // write lock* - a DIFFERENT count from the CONTRACT comment on `insert`
    // above, which numbers its own eviction retain the "FIFTH
    // session-removal path" (that phrasing counts session-REMOVAL paths, not
    // write-lock call sites; do not conflate the two when grepping "FIFTH" -
    // they are different numberings for different questions). Today's SIX
    // sites and each one's discharge status:
    //   1. `insert_unchecked` - adds only, owes nothing to tokens or
    //      attention.
    //   2. `insert`'s own eviction `retain` - removes, and discharges token
    //      AND attention in full via the `evict_and_close` it awaits on
    //      every path (see obligation note below).
    //   3. `remove` - discharges token AND attention in full.
    //   4. `remove_for_work_roots` - discharges token AND attention in full.
    //   5. `drain_all` - discharges token AND attention in full. It arrived
    //      with the 260727 Phase 2 ws-dashboard-dev merge discharging
    //      NEITHER obligation; 260727 Phase 3 brought it to parity.
    //   6. `sweep_evict_expired` - removes, and discharges token AND
    //      attention in full via the `evict_and_close` its sole caller
    //      `sweep_once` immediately awaits (see obligation note below).
    // Obligation note for sites 2 and 6: both eviction `retain`s are
    // deliberately NOT self-discharging. They hand their evicted set to the
    // shared `evict_and_close`, which is the ONE place an eviction calls
    // `forget_token` + `attention.forget` - one implementation for two
    // structurally identical `retain`s, rather than two that could drift.
    // Site 2's earlier attention-only discharge (with the callback-token
    // half left as deferred debt) was closed by this same merge; there is no
    // remaining eviction site that discharges only one of the two.
    // Every one of the six calls, and any new one, must keep BOTH forgets on
    // its removal path - an id that leaves `sessions` without them leaves a
    // phantom `AttentionHub` entry in every future snapshot for the daemon's
    // lifetime and leaks `terminal-tokens/<id>.json`.
    #[test]
    fn sessions_write_lock_sites_are_enumerated() {
        let count = flattened(&production_text())
            .matches("self.sessions.write()")
            .count();
        assert_eq!(
            count, 6,
            "expected exactly 6 textual occurrences of `self.sessions.write()` \
             (one per write-lock call site: insert_unchecked, insert, remove, \
             remove_for_work_roots, drain_all, sweep_evict_expired); found \
             {count} - if a write-lock call site was added, removed, or its \
             discharge behavior changed, update both the enumerating CONTRACT \
             comment above and this expected count together"
        );
    }

    // Invariant 3 (260727 Phase 1): `self.tokens` access is confined to
    // three choke points - `token_for` (the ONLY reader, per its own
    // CONTRACT above), and `remember_token`/`forget_token` (the two
    // writers). No behavioral check exists for this because an extra reader
    // is runtime-indistinguishable from `token_for` reading the map once
    // more (the same observability gap named in this module's own doc
    // comment for `env_clear`); this scan is the only guard.
    #[test]
    fn tokens_map_access_is_confined_to_its_choke_points() {
        // All three counts are computed and checked up front, and every
        // mismatch is collected before the single `assert!` below - never
        // three independent `assert_eq!` calls, which would short-circuit on
        // the first mismatch and hide whichever of these three logically
        // dependent counts moved alongside it (see the 260727 Phase 1
        // mutation log in the Phase 1 plan Result for an observed instance).
        let text = flattened(&production_text());
        let total = text.matches(".tokens").count();
        let reads = text.matches("self.tokens.read()").count();
        let writes = text.matches("self.tokens.write()").count();

        let mut mismatches = Vec::new();
        if total != 3 {
            mismatches.push(format!(
                "`.tokens` appeared {total} times, expected 3 (one read in \
                 `token_for`, two writes in `remember_token`/`forget_token`)"
            ));
        }
        if reads != 1 {
            mismatches.push(format!(
                "`self.tokens.read()` appeared {reads} times, expected 1 \
                 (`token_for`, the ONLY reader per its own CONTRACT)"
            ));
        }
        if writes != 2 {
            mismatches.push(format!(
                "`self.tokens.write()` appeared {writes} times, expected 2 \
                 (`remember_token`, `forget_token`)"
            ));
        }
        assert!(
            mismatches.is_empty(),
            "self.tokens access moved off its enumerated choke points - if \
             this is legitimate, update this test's enumerating doc comment \
             and the expected counts together:\n{}",
            mismatches.join("\n")
        );
    }

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
            profile_id: None,
            callback_token: None,
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
            reader_task: Mutex::new(None),
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

    // CONTRACT (260726 Phase 1 sub-fix 3, grace authority): the periodic
    // sweep's eviction step must key strictly off `admits_attach()`, never
    // `!is_live()` alone - a session that has exited but is still inside its
    // 30s attach grace must survive a sweep tick exactly as a still-`Running`
    // session does; only a session whose grace has genuinely elapsed may be
    // evicted.
    #[tokio::test]
    async fn sweep_evict_expired_never_evicts_before_admits_attach_goes_false() {
        let registry = TerminalRegistry::new(
            PathBuf::from("/nonexistent-unused-helper-binary"),
            std::env::temp_dir().join(format!("ws-dashboard-sweep-evict-{}", now_ms())),
            Duration::from_millis(200),
            None,
            String::new(),
        );

        let still_running = Arc::new(fake_terminal_session().await);

        let in_grace = Arc::new(fake_terminal_session().await);
        {
            let mut inner = in_grace.inner.lock().expect("terminal session lock poisoned");
            inner.status = TerminalStatus::Exited;
            inner.grace_until_ms = Some(now_ms() + 60_000);
        }

        let grace_expired = Arc::new(fake_terminal_session().await);
        {
            let mut inner = grace_expired
                .inner
                .lock()
                .expect("terminal session lock poisoned");
            inner.status = TerminalStatus::Exited;
            inner.grace_until_ms = Some(0); // already elapsed
        }

        registry.insert_unchecked(still_running.clone());
        registry.insert_unchecked(in_grace.clone());
        registry.insert_unchecked(grace_expired.clone());

        let evicted = registry.sweep_evict_expired();

        assert_eq!(evicted.len(), 1, "exactly the grace-expired session must be evicted");
        assert_eq!(evicted[0].id, grace_expired.id);
        assert!(
            registry.get(&still_running.id).is_some(),
            "a still-Running session must never be evicted"
        );
        assert!(
            registry.get(&in_grace.id).is_some(),
            "a session still inside its attach grace must never be evicted"
        );
        assert!(
            registry.get(&grace_expired.id).is_none(),
            "a session past its attach grace must be evicted"
        );
    }

    // CONTRACT (260726 Phase 1 correctness review I2): production
    // `connect_timeout` is 400ms (`DEFAULT_RECONCILE_CONNECT_TIMEOUT`), not
    // the 3s the ticket/plan assumed - the gate must be
    // `connect_timeout + STALE_ENTRY_SWEEP_MARGIN`, with real slack over the
    // in-flight `create_terminal` window, not exactly `connect_timeout`
    // (which is precisely the budget a slow-but-successful handshake is
    // allowed to consume, leaving zero room for `insert()`'s own scheduling
    // delay after that).
    #[test]
    fn registry_entry_is_stale_enough_for_sweep_excludes_entries_younger_than_connect_timeout_plus_margin(
    ) {
        let connect_timeout = Duration::from_millis(400);
        let threshold_ms =
            connect_timeout.as_millis() as u64 + STALE_ENTRY_SWEEP_MARGIN.as_millis() as u64;

        assert!(
            !registry_entry_is_stale_enough_for_sweep(10_000, 10_000 - 700, connect_timeout),
            "an entry well younger than connect_timeout + margin (mid-create) must never be swept"
        );
        assert!(
            !registry_entry_is_stale_enough_for_sweep(
                10_000,
                10_000 - threshold_ms,
                connect_timeout
            ),
            "an entry exactly at the connect-timeout+margin boundary must not yet be swept"
        );
        assert!(
            registry_entry_is_stale_enough_for_sweep(
                10_000 + 1,
                10_000 - threshold_ms,
                connect_timeout
            ),
            "an entry older than connect_timeout + margin must be eligible for the sweep's backstop"
        );
    }

    // CONTRACT (260726 Phase 1 sub-fix 1 / sub-fix 3): the kill+delete
    // sequence every daemon-initiated kill of a registry entry MUST route
    // through is exercised here against a REAL process (unlike the pure
    // `classify`/`registry_entry_is_stale_enough_for_sweep` unit tests
    // above) - this proves the shared helper both `TerminalSession::spawn`'s
    // handshake-failure cleanup and the sweep's registry-dir backstop reuse
    // actually terminates a live process and prunes both registry files, not
    // merely that it calls the right functions in the right order.
    #[cfg(unix)]
    #[tokio::test]
    async fn kill_verified_and_delete_entry_kills_a_real_process_and_prunes_both_registry_files() {
        let registry_dir = std::env::temp_dir().join(format!(
            "ws-dashboard-kill-and-delete-{}-{}",
            std::process::id(),
            now_ms()
        ));
        let mut real_process = std::process::Command::new("sleep")
            .arg("30")
            .spawn()
            .expect("spawn real process to stand in for an orphaned helper");
        let pid = real_process.id();
        let start_time = crate::terminal_platform::process_start_time(pid)
            .expect("real process must report a start time");

        let entry = TerminalRegistryEntry {
            terminal_id: "term_kill_and_delete".to_owned(),
            work_root_id: "root-kill".to_owned(),
            pid,
            start_time,
            boot_id: crate::terminal_platform::boot_identity(),
            supports_liveness_probe: true,
            socket_path: registry_dir.join("term_kill_and_delete.sock"),
            created_at_ms: now_ms(),
            title: "Kill And Delete".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        };
        crate::terminal_registry_file::write_registry_entry(&registry_dir, &entry)
            .expect("write entry");
        std::fs::write(&entry.socket_path, b"stand-in socket file")
            .expect("write stand-in socket file");

        kill_verified_and_delete_entry(
            &registry_dir,
            &entry.terminal_id,
            entry.pid,
            entry.start_time,
            entry.boot_id.as_deref(),
        )
        .await;

        let mut exited = false;
        for _ in 0..50 {
            if real_process
                .try_wait()
                .expect("poll killed process")
                .is_some()
            {
                exited = true;
                break;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
        assert!(
            exited,
            "the shared kill+delete helper must actually terminate the orphaned process"
        );
        assert!(
            scan_registry_dir(&registry_dir).is_empty(),
            "the registry .json entry must be pruned"
        );
        assert!(
            !entry.socket_path.exists(),
            "the stray .sock file must be pruned alongside the .json entry"
        );

        let _ = real_process.wait();
        let _ = std::fs::remove_dir_all(&registry_dir);
    }

    // CONTRACT (260726 Phase 1 sub-fix 1): a helper that spawns successfully
    // (proving `spawn_detached` itself did not fail) but never completes the
    // handshake within `connect_timeout` must not leave `TerminalSession::
    // spawn` panicking or hanging - it must surface as a spawn error, having
    // attempted the read-then-cleanup path. `/bin/true` stands in for the
    // helper binary: it ignores every argument and exits almost instantly,
    // so it never binds the IPC socket or writes a registry entry, which
    // deliberately exercises the "helper never even wrote the file" edge
    // (see `kill_verified_and_delete_entry`'s doc comment above for the
    // sibling "kill a still-running orphan" edge). It also exits fast enough
    // that nothing is left running afterward, so this test has no process to
    // clean up.
    #[cfg(unix)]
    #[tokio::test]
    async fn spawn_returns_an_error_and_cleans_up_when_the_helper_never_completes_the_handshake() {
        let registry_dir = std::env::temp_dir().join(format!(
            "ws-dashboard-spawn-handshake-timeout-{}-{}",
            std::process::id(),
            now_ms()
        ));

        let registry = TerminalRegistry::new(
            PathBuf::from("/bin/true"),
            registry_dir.clone(),
            Duration::from_millis(80),
            None,
            String::new(),
        );
        let result = TerminalSession::spawn(
            &registry,
            Path::new("/bin/true"),
            &registry_dir,
            Duration::from_millis(80),
            None,
            "",
            WorkRootId::from("fake-work-root".to_owned()),
            std::env::temp_dir(),
            "Handshake Timeout".to_owned(),
            default_columns(),
            default_rows(),
            None,
            None,
            Vec::new(),
            None,
            None,
            None,
        )
        .await;

        assert!(
            matches!(result, Err(TerminalError::BadRequest(_))),
            "a helper that never completes the handshake must surface as a spawn error, not a panic or a hang"
        );
        assert!(
            scan_registry_dir(&registry_dir).is_empty(),
            "no registry entry should be left behind for a helper that never wrote one"
        );

        let _ = std::fs::remove_dir_all(&registry_dir);
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
            boot_id: crate::terminal_platform::boot_identity(),
            supports_liveness_probe: true,
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
            None,
            String::new(),
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
            boot_id: crate::terminal_platform::boot_identity(),
            supports_liveness_probe: true,
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
            None,
            String::new(),
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

    // CONTRACT (260726 Phase 1 correctness review I3): `insert`'s lazy
    // eviction is a second, independent code path from the periodic sweep's
    // `sweep_evict_expired` - it must honor the exact same grace-authority
    // gate (`admits_attach()`, never `!is_live()` alone) and must actually
    // close the IPC connection of anything it does evict, not merely drop
    // the `Arc` out of the map (which would silently defeat the periodic
    // sweep too, since a session dropped here can never be found by it
    // again).
    #[tokio::test]
    async fn insert_never_evicts_a_session_inside_its_attach_grace_and_closes_what_it_does_evict() {
        use tokio::io::AsyncReadExt;

        let registry = TerminalRegistry::new(
            PathBuf::from("/nonexistent-unused-helper-binary"),
            std::env::temp_dir().join(format!("ws-dashboard-insert-evict-{}", now_ms())),
            Duration::from_millis(200),
            None,
            String::new(),
        );

        let in_grace = Arc::new(fake_terminal_session().await);
        {
            let mut inner = in_grace.inner.lock().expect("terminal session lock poisoned");
            inner.status = TerminalStatus::Exited;
            inner.grace_until_ms = Some(now_ms() + 60_000);
        }

        // Built inline (not via `fake_terminal_session`) so the duplex
        // peer half can be retained here and used to observe EOF - direct
        // proof that `insert`'s lazy eviction actually closed the session's
        // IPC connection, not merely dropped it out of the map.
        let (mut peer, local) = tokio::io::duplex(4096);
        let (_read_half, write_half) = crate::terminal_ipc_transport::split(
            Box::new(local) as crate::terminal_ipc_transport::BoxedIpcStream
        );
        let grace_expired = Arc::new(TerminalSession {
            id: opaque_terminal_id(),
            work_root_id: WorkRootId::from("fake-work-root".to_owned()),
            title: "fake".to_owned(),
            cwd_hint: None,
            created_at_ms: now_ms(),
            profile_id: None,
            callback_token: None,
            pid: std::process::id(),
            start_time: 0,
            write_half: Arc::new(AsyncMutex::new(write_half)),
            inner: Mutex::new(TerminalSessionInner {
                status: TerminalStatus::Exited,
                columns: default_columns(),
                rows: default_rows(),
                output: VecDeque::new(),
                next_sequence: 1,
                grace_until_ms: Some(0), // already elapsed
            }),
            output_signal: watch::channel(0).0,
            reader_task: Mutex::new(None),
        });

        registry.insert_unchecked(in_grace.clone());
        registry.insert_unchecked(grace_expired.clone());

        let newcomer = Arc::new(fake_terminal_session().await);
        registry
            .insert(newcomer.clone())
            .await
            .expect("insert must succeed under the session cap");

        assert!(
            registry.get(&in_grace.id).is_some(),
            "a session still inside its attach grace must survive an unrelated insert"
        );
        assert!(
            registry.get(&grace_expired.id).is_none(),
            "a session past its attach grace must be evicted by insert's own lazy path"
        );
        assert!(
            registry.get(&newcomer.id).is_some(),
            "the newly inserted session itself must land in the registry"
        );

        let mut buf = [0u8; 1];
        let read = tokio::time::timeout(Duration::from_secs(1), peer.read(&mut buf))
            .await
            .expect("peer read must not hang")
            .expect("peer read must not error");
        assert_eq!(
            read, 0,
            "insert's lazy eviction must shut down the write half, observable as EOF on the peer"
        );
    }

    // CONTRACT (260726 Phase 1 correctness review I1): `sweep_registry_
    // backstop` must not SIGKILL the helper of a session `sweep_evict_
    // expired` evicted in the SAME tick - the just-evicted id's `<id>.json`
    // is, by construction, already old enough to look "orphaned" to the
    // backstop's own age gate the instant it is checked, even though the
    // helper's own graceful self-exit (triggered by the eviction step's
    // `close_ipc_connection`) has had zero time to run yet.
    #[cfg(unix)]
    #[tokio::test]
    async fn sweep_once_never_kills_the_helper_of_a_session_it_just_evicted_in_the_same_tick() {
        let registry_dir = std::env::temp_dir().join(format!(
            "ws-dashboard-sweep-i1-{}-{}",
            std::process::id(),
            now_ms()
        ));
        let registry = TerminalRegistry::new(
            PathBuf::from("/nonexistent-unused-helper-binary"),
            registry_dir.clone(),
            Duration::from_millis(50),
            None,
            String::new(),
        );

        let mut real_process = std::process::Command::new("sleep")
            .arg("30")
            .spawn()
            .expect("spawn real process to stand in for a still-running helper");
        let pid = real_process.id();
        let start_time = crate::terminal_platform::process_start_time(pid)
            .expect("real process must report a start time");

        let terminal_id = "term_sweep_i1".to_owned();
        let entry = TerminalRegistryEntry {
            terminal_id: terminal_id.clone(),
            work_root_id: "root-sweep-i1".to_owned(),
            pid,
            start_time,
            boot_id: crate::terminal_platform::boot_identity(),
            supports_liveness_probe: true,
            socket_path: registry_dir.join(format!("{terminal_id}.sock")),
            // Old enough to already exceed connect_timeout + margin the
            // instant the backstop checks it - exactly what makes the I1
            // race dangerous without the fix.
            created_at_ms: now_ms().saturating_sub(60_000),
            title: "Sweep I1".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        };
        crate::terminal_registry_file::write_registry_entry(&registry_dir, &entry)
            .expect("write orphan-shaped entry for the still-live session");

        let mut session = fake_terminal_session().await;
        session.id = terminal_id.clone();
        {
            let mut inner = session.inner.lock().expect("terminal session lock poisoned");
            inner.status = TerminalStatus::Exited;
            inner.grace_until_ms = Some(0); // already elapsed - eligible this tick
        }
        registry.insert_unchecked(Arc::new(session));

        registry.sweep_once().await;

        assert!(
            real_process
                .try_wait()
                .expect("poll standing-in process")
                .is_none(),
            "the just-evicted session's helper must not be SIGKILLed in the same sweep tick"
        );
        assert!(
            scan_registry_dir(&registry_dir)
                .iter()
                .any(|found| found.terminal_id == terminal_id),
            "the just-evicted session's registry entry must not be deleted by the same-tick backstop"
        );
        assert!(
            registry.get(&terminal_id).is_none(),
            "the session must still be evicted from the live map by sweep_evict_expired itself"
        );

        let _ = real_process.kill();
        let _ = real_process.wait();
        let _ = std::fs::remove_dir_all(&registry_dir);
    }

    // CONTRACT (260726 Phase 1 test-partition finding #3): `sweep_registry_
    // backstop`'s actual scan-and-dispatch logic (live-id exclusion,
    // VerifiedOurs-vs-unverified dispatch) had no direct test - only its
    // pure age-gate predicate did. This drives the real function end to end
    // against a populated registry directory: a genuinely orphaned
    // verified-identity entry must be killed and pruned, an entry whose
    // identity does not verify must be pruned WITHOUT ever signaling
    // anything, and an entry that still has a live in-memory session must be
    // left completely untouched even though its `<id>.json` looks equally
    // stale on disk.
    #[cfg(unix)]
    #[tokio::test]
    async fn sweep_registry_backstop_kills_verified_orphan_drops_unverified_and_skips_live_entries() {
        let registry_dir = std::env::temp_dir().join(format!(
            "ws-dashboard-sweep-backstop-{}-{}",
            std::process::id(),
            now_ms()
        ));
        let registry = TerminalRegistry::new(
            PathBuf::from("/nonexistent-unused-helper-binary"),
            registry_dir.clone(),
            Duration::from_millis(50),
            None,
            String::new(),
        );
        let stale_created_at_ms = now_ms().saturating_sub(60_000);

        // (a) A genuine orphan: a real process, verified identity, no live
        // session anywhere - must be killed and pruned.
        let mut orphan_process = std::process::Command::new("sleep")
            .arg("30")
            .spawn()
            .expect("spawn real process to stand in for a genuinely orphaned helper");
        let orphan_pid = orphan_process.id();
        let orphan_start_time = crate::terminal_platform::process_start_time(orphan_pid)
            .expect("real process must report a start time");
        let orphan_entry = TerminalRegistryEntry {
            terminal_id: "term_backstop_orphan".to_owned(),
            work_root_id: "root-backstop".to_owned(),
            pid: orphan_pid,
            start_time: orphan_start_time,
            boot_id: crate::terminal_platform::boot_identity(),
            supports_liveness_probe: true,
            socket_path: registry_dir.join("term_backstop_orphan.sock"),
            created_at_ms: stale_created_at_ms,
            title: "Orphan".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        };
        crate::terminal_registry_file::write_registry_entry(&registry_dir, &orphan_entry)
            .expect("write orphan entry");

        // (b) An unverified identity (no such process) - must be pruned
        // without ever attempting a kill.
        let unverified_entry = TerminalRegistryEntry {
            terminal_id: "term_backstop_unverified".to_owned(),
            work_root_id: "root-backstop".to_owned(),
            pid: 0x7fff_fffe,
            start_time: 123,
            boot_id: crate::terminal_platform::boot_identity(),
            supports_liveness_probe: true,
            socket_path: registry_dir.join("term_backstop_unverified.sock"),
            created_at_ms: stale_created_at_ms,
            title: "Unverified".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        };
        crate::terminal_registry_file::write_registry_entry(&registry_dir, &unverified_entry)
            .expect("write unverified entry");

        // (c) A stale-looking entry that DOES have a live in-memory session
        // - must be skipped entirely (live-id exclusion), even though its
        // age alone would otherwise make it eligible.
        let mut live_session = fake_terminal_session().await;
        live_session.id = "term_backstop_live".to_owned();
        let live_session = Arc::new(live_session);
        registry.insert_unchecked(live_session.clone());
        let live_entry = TerminalRegistryEntry {
            terminal_id: "term_backstop_live".to_owned(),
            work_root_id: "root-backstop".to_owned(),
            pid: orphan_pid,
            start_time: orphan_start_time,
            boot_id: crate::terminal_platform::boot_identity(),
            supports_liveness_probe: true,
            socket_path: registry_dir.join("term_backstop_live.sock"),
            created_at_ms: stale_created_at_ms,
            title: "Live".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        };
        crate::terminal_registry_file::write_registry_entry(&registry_dir, &live_entry)
            .expect("write live-session-shadowed entry");

        registry.sweep_registry_backstop().await;

        let mut exited = false;
        for _ in 0..50 {
            if orphan_process
                .try_wait()
                .expect("poll orphan process")
                .is_some()
            {
                exited = true;
                break;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
        assert!(exited, "a verified-identity orphan must actually be killed");

        let remaining: std::collections::HashSet<String> = scan_registry_dir(&registry_dir)
            .into_iter()
            .map(|entry| entry.terminal_id)
            .collect();
        assert!(
            !remaining.contains("term_backstop_orphan"),
            "the killed orphan's registry entry must be pruned"
        );
        assert!(
            !remaining.contains("term_backstop_unverified"),
            "an unverified-identity entry must still be pruned"
        );
        assert!(
            remaining.contains("term_backstop_live"),
            "an entry with a live in-memory session must be left untouched by the backstop"
        );

        let _ = std::fs::remove_dir_all(&registry_dir);
    }

    // CONTRACT (boot-identity gate): the headline hazard this guard exists
    // for, reproduced end-to-end against a REAL live process. A hard reboot
    // SIGKILLs helpers before their delete-on-exit cleanup runs, so stale
    // `<id>.json` entries survive it. On a boot-relative platform (Linux)
    // the recorded `start_time` is ticks-since-boot, so an unrelated process
    // on the NEW boot can hold the recorded pid AND report the very same
    // tick offset - `process_start_time` then agrees and, before this fix,
    // `identity_status` returned `VerifiedOurs` and the entry was SIGKILLed.
    //
    // This test constructs exactly that coincidence deterministically rather
    // than waiting for it: `pid`/`start_time` are read from a real live
    // process so the start-time comparison is GUARANTEED to match, and only
    // the recorded boot identity differs. Revert the boot check in
    // `identity_status` (or in `kill_verified_and_delete_entry`) and this
    // test kills `sleep 30` and fails. It is also the runtime-frequency case:
    // `sweep_registry_backstop` re-runs this decision every sweep tick, not
    // once per daemon start.
    #[cfg(unix)]
    #[tokio::test]
    async fn sweep_never_kills_a_matching_pid_and_start_time_recorded_under_a_different_boot() {
        if !crate::terminal_platform::START_TIME_IS_BOOT_RELATIVE {
            // macOS/Windows record an ABSOLUTE start time, so there is no
            // cross-boot re-minting hazard and the gate is inert by design
            // (see `terminal_platform.rs`'s file-header CONTRACT). Asserting
            // "must not kill" there would assert the opposite of those
            // platforms' intended behavior.
            return;
        }
        let registry_dir = std::env::temp_dir().join(format!(
            "ws-dashboard-sweep-foreign-boot-{}-{}",
            std::process::id(),
            now_ms()
        ));
        let registry = TerminalRegistry::new(
            PathBuf::from("/nonexistent-unused-helper-binary"),
            registry_dir.clone(),
            Duration::from_millis(50),
            None,
            String::new(),
        );

        let mut bystander = std::process::Command::new("sleep")
            .arg("30")
            .spawn()
            .expect("spawn the unrelated process a stale cross-boot entry would mis-target");
        let pid = bystander.id();
        let start_time = crate::terminal_platform::process_start_time(pid)
            .expect("bystander must report a start time");

        let entry = TerminalRegistryEntry {
            terminal_id: "term_foreign_boot".to_owned(),
            work_root_id: "root-foreign-boot".to_owned(),
            // Real pid + real start time: the start-time comparison alone
            // WILL verify. The boot identity is the only thing standing
            // between this entry and a SIGKILL of an arbitrary process.
            pid,
            start_time,
            boot_id: Some("00000000-0000-0000-0000-000000000000".to_owned()),
            supports_liveness_probe: true,
            socket_path: registry_dir.join("term_foreign_boot.sock"),
            created_at_ms: now_ms().saturating_sub(60_000),
            title: "Foreign Boot".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        };
        crate::terminal_registry_file::write_registry_entry(&registry_dir, &entry)
            .expect("write cross-boot entry");

        assert_eq!(
            identity_status(entry.pid, entry.start_time, entry.boot_id.as_deref()),
            IdentityStatus::UnverifiableBoot,
            "a pid+start-time pair recorded under a different boot must classify as \
             unverifiable, never as VerifiedOurs"
        );

        registry.sweep_registry_backstop().await;

        // Give a mis-fired SIGKILL every chance to land before asserting the
        // absence - a bare post-sweep poll could pass simply by racing it.
        tokio::time::sleep(Duration::from_millis(200)).await;
        assert!(
            bystander
                .try_wait()
                .expect("poll the bystander process")
                .is_none(),
            "a stale entry from a previous boot must NEVER SIGKILL the unrelated process now \
             holding its pid, even when the recorded start time coincidentally matches"
        );
        assert!(
            scan_registry_dir(&registry_dir).is_empty(),
            "the stale cross-boot entry must still be pruned - the outcome is drop-only, not \
             leave-it-alone"
        );

        let _ = bystander.kill();
        let _ = bystander.wait();
        let _ = std::fs::remove_dir_all(&registry_dir);
    }

    // CONTRACT (boot-identity gate, backward compatibility): the same hazard
    // as the test above, but for an entry written BEFORE `bootId` existed -
    // `boot_id: None`. On a boot-relative platform that entry is
    // unverifiable, NOT trusted: a boot-relative start time with no boot
    // qualifier is exactly the value a reboot can silently re-mint. Reverting
    // the `None` leg of `boot_identity_verified` (e.g. to "absent means
    // trust the pid" for compatibility) kills the bystander here.
    #[cfg(unix)]
    #[tokio::test]
    async fn a_legacy_entry_with_no_recorded_boot_identity_is_dropped_not_killed() {
        if !crate::terminal_platform::START_TIME_IS_BOOT_RELATIVE {
            return;
        }
        let registry_dir = std::env::temp_dir().join(format!(
            "ws-dashboard-sweep-legacy-boot-{}-{}",
            std::process::id(),
            now_ms()
        ));
        let registry = TerminalRegistry::new(
            PathBuf::from("/nonexistent-unused-helper-binary"),
            registry_dir.clone(),
            Duration::from_millis(50),
            None,
            String::new(),
        );

        let mut bystander = std::process::Command::new("sleep")
            .arg("30")
            .spawn()
            .expect("spawn the unrelated process a legacy entry would mis-target");
        let pid = bystander.id();
        let start_time = crate::terminal_platform::process_start_time(pid)
            .expect("bystander must report a start time");

        let entry = TerminalRegistryEntry {
            terminal_id: "term_legacy_boot".to_owned(),
            work_root_id: "root-legacy-boot".to_owned(),
            pid,
            start_time,
            boot_id: None,
            supports_liveness_probe: true,
            socket_path: registry_dir.join("term_legacy_boot.sock"),
            created_at_ms: now_ms().saturating_sub(60_000),
            title: "Legacy Boot".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        };
        crate::terminal_registry_file::write_registry_entry(&registry_dir, &entry)
            .expect("write legacy entry");

        registry.sweep_registry_backstop().await;

        tokio::time::sleep(Duration::from_millis(200)).await;
        assert!(
            bystander
                .try_wait()
                .expect("poll the bystander process")
                .is_none(),
            "an entry with no recorded boot identity must never authorize a kill on a \
             boot-relative platform"
        );
        assert!(
            scan_registry_dir(&registry_dir).is_empty(),
            "the legacy entry must still be pruned"
        );

        let _ = bystander.kill();
        let _ = bystander.wait();
        let _ = std::fs::remove_dir_all(&registry_dir);
    }

    // CONTRACT (boot-identity gate, choke point): `kill_verified_and_delete_
    // entry` re-applies the gate itself rather than trusting callers, because
    // `TerminalSession::spawn`'s handshake-failure cleanup reaches it without
    // ever calling `identity_status`. Driving the helper DIRECTLY is what
    // distinguishes this from the sweep tests above - remove only the
    // in-function check and the sweep tests still pass while this one kills
    // the bystander.
    #[cfg(unix)]
    #[tokio::test]
    async fn kill_verified_and_delete_entry_withholds_the_signal_on_a_foreign_boot_identity() {
        if !crate::terminal_platform::START_TIME_IS_BOOT_RELATIVE {
            return;
        }
        let registry_dir = std::env::temp_dir().join(format!(
            "ws-dashboard-kill-foreign-boot-{}-{}",
            std::process::id(),
            now_ms()
        ));
        let mut bystander = std::process::Command::new("sleep")
            .arg("30")
            .spawn()
            .expect("spawn bystander process");
        let pid = bystander.id();
        let start_time = crate::terminal_platform::process_start_time(pid)
            .expect("bystander must report a start time");

        let entry = TerminalRegistryEntry {
            terminal_id: "term_choke_point".to_owned(),
            work_root_id: "root-choke-point".to_owned(),
            pid,
            start_time,
            boot_id: Some("00000000-0000-0000-0000-000000000000".to_owned()),
            supports_liveness_probe: true,
            socket_path: registry_dir.join("term_choke_point.sock"),
            created_at_ms: now_ms(),
            title: "Choke Point".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        };
        crate::terminal_registry_file::write_registry_entry(&registry_dir, &entry)
            .expect("write entry");
        std::fs::write(&entry.socket_path, b"stand-in socket file")
            .expect("write stand-in socket file");

        kill_verified_and_delete_entry(
            &registry_dir,
            &entry.terminal_id,
            entry.pid,
            entry.start_time,
            entry.boot_id.as_deref(),
        )
        .await;

        tokio::time::sleep(Duration::from_millis(200)).await;
        assert!(
            bystander
                .try_wait()
                .expect("poll the bystander process")
                .is_none(),
            "the shared kill+delete choke point must withhold the signal when the entry's boot \
             identity does not match this boot"
        );
        assert!(
            scan_registry_dir(&registry_dir).is_empty(),
            "the entry must still be pruned even though the signal was withheld"
        );
        assert!(
            !entry.socket_path.exists(),
            "the stray .sock file must be pruned alongside the .json entry"
        );

        let _ = bystander.kill();
        let _ = bystander.wait();
        let _ = std::fs::remove_dir_all(&registry_dir);
    }

    // CONTRACT (260726 Phase 1 test-partition finding #2): the only
    // previously tested edges of `TerminalSession::spawn`'s handshake-
    // failure cleanup were "helper never even wrote its registry file"
    // (`/bin/true`, above) and the shared `kill_verified_and_delete_entry`
    // helper called directly (bypassing `spawn` entirely). Neither drives
    // the actual `Some(entry) =>` arm INSIDE `spawn` itself - a bug
    // swapping `entry.pid`/`entry.start_time`, or the wrong `id`/
    // `registry_dir` variable, would pass both. This uses a tiny POSIX
    // shell script as `helper_binary` that deterministically reproduces the
    // ticket's headline orphan scenario: it durably writes its OWN
    // `<id>.json` registry entry (with its own real pid/start-time, read
    // back from `/proc/self/stat` before writing anything) and a stand-in
    // `.sock` file, then blocks forever without ever accepting a
    // connection - so `connect_and_handshake` is guaranteed to exhaust
    // `connect_timeout` and return `None`, driving `spawn`'s `Some(entry)`
    // arm against the entry it actually reads back off disk. The script
    // also drops its own pid/start-time into a side-channel file (outside
    // `registry_dir`, so it survives the entry's deletion) so this test can
    // independently confirm the real process was actually killed - proving
    // `entry.pid`/`entry.start_time` were threaded through correctly rather
    // than merely proving "no panic, files deleted".
    #[cfg(unix)]
    #[tokio::test]
    async fn spawn_kills_the_verified_orphan_when_the_helper_writes_its_entry_but_never_handshakes() {
        let registry_dir = std::env::temp_dir().join(format!(
            "ws-dashboard-spawn-some-entry-{}-{}",
            std::process::id(),
            now_ms()
        ));
        let script_path = std::env::temp_dir().join(format!(
            "ws-dashboard-never-handshake-helper-{}-{}.sh",
            std::process::id(),
            now_ms()
        ));
        let identity_path = std::env::temp_dir().join(format!(
            "ws-dashboard-never-handshake-identity-{}-{}",
            std::process::id(),
            now_ms()
        ));

        let script = format!(
            r#"#!/bin/sh
set -eu
registry_dir=""
terminal_id=""
socket_path=""
while [ $# -gt 0 ]; do
  case "$1" in
    --registry-dir) registry_dir="$2"; shift 2 ;;
    --terminal-id) terminal_id="$2"; shift 2 ;;
    --socket-path) socket_path="$2"; shift 2 ;;
    --*) shift 2 ;;
    *) shift ;;
  esac
done
mkdir -p "$registry_dir"
after_comm=$(sed 's/.*)//' /proc/$$/stat)
start_time=$(printf '%s' "$after_comm" | awk '{{print $20}}')
printf '%s %s' "$$" "$start_time" > {identity_path}
# Boot identity, mirroring `terminal_platform::boot_identity()`: the daemon
# refuses to signal an entry whose recorded boot does not match this one, so
# this fixture must record the real current boot id or the test would stop
# exercising the kill path at all (it would silently become a drop-only test).
if [ -r /proc/sys/kernel/random/boot_id ]; then
  boot_id="\"$(cat /proc/sys/kernel/random/boot_id)\""
else
  boot_id=null
fi
cat > "$registry_dir/$terminal_id.json.tmp" <<JSON
{{"terminalId":"$terminal_id","workRootId":"fake-work-root","pid":$$,"startTime":$start_time,"bootId":$boot_id,"socketPath":"$socket_path","createdAtMs":0,"title":"never-handshakes","cwdHint":null,"columns":80,"rows":24}}
JSON
mv "$registry_dir/$terminal_id.json.tmp" "$registry_dir/$terminal_id.json"
touch "$socket_path"
exec sleep 30
"#,
            identity_path = identity_path.display(),
        );
        std::fs::write(&script_path, script).expect("write never-handshake helper script");
        {
            use std::os::unix::fs::PermissionsExt;
            std::fs::set_permissions(&script_path, std::fs::Permissions::from_mode(0o700))
                .expect("make never-handshake helper script executable");
        }

        let registry = TerminalRegistry::new(
            script_path.clone(),
            registry_dir.clone(),
            Duration::from_millis(300),
            None,
            String::new(),
        );
        let result = TerminalSession::spawn(
            &registry,
            &script_path,
            &registry_dir,
            Duration::from_millis(300),
            None,
            "",
            WorkRootId::from("fake-work-root".to_owned()),
            std::env::temp_dir(),
            "Some Entry Orphan".to_owned(),
            default_columns(),
            default_rows(),
            None,
            None,
            Vec::new(),
            None,
            None,
            None,
        )
        .await;

        assert!(
            matches!(result, Err(TerminalError::BadRequest(_))),
            "a helper that wrote its entry but never handshaked must still surface as a spawn error"
        );
        assert!(
            scan_registry_dir(&registry_dir).is_empty(),
            "the Some(entry) arm must prune the registry entry it read back"
        );

        let identity =
            std::fs::read_to_string(&identity_path).expect("read the script's identity side-channel");
        let mut parts = identity.split_whitespace();
        let script_pid: u32 = parts
            .next()
            .expect("identity file must contain a pid")
            .parse()
            .expect("identity pid must be numeric");
        let script_start_time: u64 = parts
            .next()
            .expect("identity file must contain a start time")
            .parse()
            .expect("identity start time must be numeric");

        let mut killed = false;
        for _ in 0..50 {
            if crate::terminal_platform::process_start_time(script_pid) != Some(script_start_time) {
                killed = true;
                break;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
        assert!(
            killed,
            "spawn's Some(entry) arm must actually kill the real orphaned process, proving \
             entry.pid/entry.start_time were threaded through correctly"
        );

        let _ = std::fs::remove_file(&script_path);
        let _ = std::fs::remove_file(&identity_path);
        let _ = std::fs::remove_dir_all(&registry_dir);
    }

    #[test]
    fn validate_command_env_overlay_pairing_rejects_overlay_without_command() {
        assert!(matches!(
            validate_command_env_overlay_pairing(&None, &[("FOO".to_owned(), "bar".to_owned())]),
            Err(TerminalError::BadRequest(_))
        ));
    }

    #[test]
    fn validate_command_env_overlay_pairing_allows_overlay_with_command() {
        assert!(validate_command_env_overlay_pairing(
            &Some(("agent-cli".to_owned(), Vec::new())),
            &[("FOO".to_owned(), "bar".to_owned())],
        )
        .is_ok());
    }

    #[test]
    fn validate_command_env_overlay_pairing_allows_no_command_and_no_overlay() {
        assert!(validate_command_env_overlay_pairing(&None, &[]).is_ok());
    }

    #[test]
    fn helper_spawn_default_no_command_matches_existing_arg_shape() {
        let command = build_helper_command(
            Path::new("/usr/local/bin/ws-dashboard"),
            Path::new("/tmp/registry"),
            "term_abc",
            "wr1",
            Path::new("/tmp/cwd"),
            "title",
            80,
            24,
            None,
            Path::new("/tmp/term_abc.sock"),
            None,
            &[],
            None,
            Vec::<(std::ffi::OsString, std::ffi::OsString)>::new(),
        );

        assert!(
            command.get_envs().next().is_none(),
            "default path must not call .env()/.env_clear() at all"
        );
        // PRIMARY GUARD (260726 Phase 1): the falsifiable, platform-neutral
        // assertion this test previously lacked - it asserts on the env plan
        // VALUE, which discriminates, rather than on a built `Command`, which
        // cannot (see the CONTRACT below).
        assert_eq!(
            helper_env_plan(
                None,
                None,
                Vec::<(std::ffi::OsString, std::ffi::OsString)>::new()
            ),
            HelperEnvPlan::InheritHost,
            "default (no-command) path must plan to inherit the host env untouched"
        );
        // CONTRACT (review cycle 1, finding T1): `get_envs()` alone cannot
        // distinguish "no env method ever called" from "env_clear() called
        // with nothing re-added" - both report zero explicit entries,
        // because `std::process::Command` has no public API exposing
        // whether `clear()` ran (verified empirically against this
        // toolchain's std - see review finding T1). On unix, `Command`'s
        // `Debug` impl renders as a shell `env` invocation and DOES encode
        // the clear flag (`env -i ...` vs a bare quoted program), which
        // closes the gap there. There is no known equivalent public signal
        // on Windows (`Command`'s windows `Debug` impl only prints
        // program+args), so that platform's default-path env_clear()
        // regression remains a named, accepted limitation of this guard
        // rather than a fixed gap.
        #[cfg(unix)]
        {
            let debug = format!("{command:?}");
            assert!(
                !debug.contains("env -i"),
                "default path's Debug rendering must not contain an env_clear() \
                 marker (env -i): {debug:?}"
            );
            // Positive control for the secondary detector itself: proves the
            // "env -i" substring check is actually live on this toolchain's
            // `Debug` impl, rather than a check that would pass against any
            // string at all.
            let mut cleared = std::process::Command::new("/usr/bin/true");
            cleared.env_clear();
            let cleared_debug = format!("{cleared:?}");
            assert!(
                cleared_debug.contains("env -i"),
                "positive control: a deliberately env_clear()ed Command's Debug \
                 must contain env -i, or this secondary detector cannot fire: \
                 {cleared_debug:?}"
            );
        }
        let args: Vec<String> = command
            .get_args()
            .map(|arg| arg.to_string_lossy().into_owned())
            .collect();
        assert!(!args.iter().any(|arg| arg == "--command"));
        assert!(!args.iter().any(|arg| arg == "--command-arg"));
        assert!(!args.iter().any(|arg| arg == "--env-overlay"));
    }

    #[test]
    fn helper_spawn_with_command_scrubs_claude_markers_and_forwards_argv() {
        let mut host_env: Vec<(std::ffi::OsString, std::ffi::OsString)> = crate::agent_env_profile::CLAUDE
            .markers
            .iter()
            .map(|marker| {
                (
                    std::ffi::OsString::from(*marker),
                    std::ffi::OsString::from("marker-value"),
                )
            })
            .collect();
        host_env.push((
            std::ffi::OsString::from("PATH"),
            std::ffi::OsString::from("/usr/bin:/bin"),
        ));
        // T2: a second, arbitrary non-marker key that no plausible
        // hand-rolled allowlist would think to include - closes the gap
        // where a narrow allowlist that happens to enumerate PATH would
        // otherwise still pass this test.
        host_env.push((
            std::ffi::OsString::from("SOME_OTHER_VAR"),
            std::ffi::OsString::from("keep-me"),
        ));

        let command = build_helper_command(
            Path::new("/usr/local/bin/ws-dashboard"),
            Path::new("/tmp/registry"),
            "term_abc",
            "wr1",
            Path::new("/tmp/cwd"),
            "title",
            80,
            24,
            None,
            Path::new("/tmp/term_abc.sock"),
            Some(&("agent-cli".to_owned(), vec!["--flag".to_owned()])),
            &[("BASE_URL".to_owned(), "http://x".to_owned())],
            Some(&crate::agent_env_profile::CLAUDE),
            host_env,
        );

        let envs: HashMap<String, Option<String>> = command
            .get_envs()
            .map(|(key, value)| {
                (
                    key.to_string_lossy().into_owned(),
                    value.map(|value| value.to_string_lossy().into_owned()),
                )
            })
            .collect();
        for marker in crate::agent_env_profile::CLAUDE.markers {
            assert!(!envs.contains_key(*marker), "marker {marker} must be scrubbed");
        }
        assert_eq!(envs.get("PATH").cloned().flatten().as_deref(), Some("/usr/bin:/bin"));
        assert_eq!(
            envs.get("SOME_OTHER_VAR").cloned().flatten().as_deref(),
            Some("keep-me"),
            "deny-list, not allowlist - an arbitrary non-marker key must survive too (T2)"
        );

        let args: Vec<String> = command
            .get_args()
            .map(|arg| arg.to_string_lossy().into_owned())
            .collect();
        let command_pos = args.iter().position(|arg| arg == "--command").expect("--command present");
        assert_eq!(args[command_pos + 1], "agent-cli");
        let command_arg_pos = args
            .iter()
            .position(|arg| arg == "--command-arg")
            .expect("--command-arg present");
        assert_eq!(args[command_arg_pos + 1], "--flag");
        let env_overlay_pos = args
            .iter()
            .position(|arg| arg == "--env-overlay")
            .expect("--env-overlay present");
        assert_eq!(args[env_overlay_pos + 1], "BASE_URL=http://x");
    }

    // CONTRACT (260726 Phase 1, non-vacuity): the other half of the env-plan
    // guard. The default-path assertion above only proves `helper_env_plan`
    // returns `InheritHost` for `None`; a function that returned
    // `InheritHost` unconditionally would satisfy it. This test proves the
    // plan value DISCRIMINATES - an explicit command yields `ClearAndSet`
    // carrying the scrubbed pairs.
    #[test]
    fn helper_env_plan_with_command_scrubs_claude_markers_and_preserves_others() {
        let mut host_env: Vec<(std::ffi::OsString, std::ffi::OsString)> = crate::agent_env_profile::CLAUDE
            .markers
            .iter()
            .map(|marker| {
                (
                    std::ffi::OsString::from(*marker),
                    std::ffi::OsString::from("marker-value"),
                )
            })
            .collect();
        host_env.push((
            std::ffi::OsString::from("PATH"),
            std::ffi::OsString::from("/usr/bin:/bin"),
        ));
        host_env.push((
            std::ffi::OsString::from("HOME"),
            std::ffi::OsString::from("/home/example"),
        ));

        let plan = helper_env_plan(
            Some(&("agent-cli".to_owned(), Vec::new())),
            Some(&crate::agent_env_profile::CLAUDE),
            host_env,
        );

        let HelperEnvPlan::ClearAndSet(pairs) = plan else {
            panic!("an explicit command must plan to clear and set, got {plan:?}");
        };
        let envs: HashMap<String, String> = pairs
            .into_iter()
            .map(|(key, value)| {
                (
                    key.to_string_lossy().into_owned(),
                    value.to_string_lossy().into_owned(),
                )
            })
            .collect();
        for marker in crate::agent_env_profile::CLAUDE.markers {
            assert!(
                !envs.contains_key(*marker),
                "marker {marker} must be scrubbed out of the env plan"
            );
        }
        assert_eq!(
            envs.get("PATH").map(String::as_str),
            Some("/usr/bin:/bin"),
            "an ordinary shell var must survive the plan - deny-list, not allowlist"
        );
        assert_eq!(envs.get("HOME").map(String::as_str), Some("/home/example"));
    }

    // CONTRACT (260725 Phase 2, non-vacuity): proves `build_helper_command`
    // actually applies the CALLER-SUPPLIED `scrub` profile rather than a
    // profile-blind hardcoded `CLAUDE` - the sibling test above passes
    // `Some(&CLAUDE)` explicitly, which alone cannot distinguish "the scrub
    // parameter is real" from "CLAUDE stayed hardcoded and the parameter is
    // dead". This test passes a DIFFERENT synthetic scrub profile whose
    // marker does not appear in CLAUDE's list, and asserts that marker (not
    // a CLAUDE marker) is what gets stripped.
    #[test]
    fn helper_spawn_with_command_uses_the_supplied_scrub_profile_not_a_hardcoded_one() {
        const SYNTHETIC: crate::agent_env_profile::EnvScrubProfile =
            crate::agent_env_profile::EnvScrubProfile {
                name: "synthetic-test-profile",
                markers: &["SYNTHETIC_MARKER_ONLY"],
            };
        let host_env: Vec<(std::ffi::OsString, std::ffi::OsString)> = vec![
            (
                std::ffi::OsString::from("SYNTHETIC_MARKER_ONLY"),
                std::ffi::OsString::from("scrub-me"),
            ),
            (
                std::ffi::OsString::from("CLAUDECODE"),
                std::ffi::OsString::from("marker-value"),
            ),
            (
                std::ffi::OsString::from("PATH"),
                std::ffi::OsString::from("/usr/bin:/bin"),
            ),
        ];

        let command = build_helper_command(
            Path::new("/usr/local/bin/ws-dashboard"),
            Path::new("/tmp/registry"),
            "term_abc",
            "wr1",
            Path::new("/tmp/cwd"),
            "title",
            80,
            24,
            None,
            Path::new("/tmp/term_abc.sock"),
            Some(&("agent-cli".to_owned(), Vec::new())),
            &[],
            Some(&SYNTHETIC),
            host_env,
        );

        let envs: HashMap<String, Option<String>> = command
            .get_envs()
            .map(|(key, value)| {
                (
                    key.to_string_lossy().into_owned(),
                    value.map(|value| value.to_string_lossy().into_owned()),
                )
            })
            .collect();
        assert!(
            !envs.contains_key("SYNTHETIC_MARKER_ONLY"),
            "the supplied synthetic profile's own marker must be scrubbed"
        );
        assert!(
            envs.contains_key("CLAUDECODE"),
            "a hardcoded-CLAUDE regression would scrub this even though the \
             supplied profile never lists it - CLAUDECODE surviving proves \
             the caller-supplied profile is what actually ran, not CLAUDE"
        );
        assert_eq!(envs.get("PATH").cloned().flatten().as_deref(), Some("/usr/bin:/bin"));

        // C1 fix: hop 1 must forward the SYNTHETIC profile's own marker to
        // hop 2 via `--scrub-marker`, not silently keep it hop-1-only.
        let args: Vec<String> = command
            .get_args()
            .map(|arg| arg.to_string_lossy().into_owned())
            .collect();
        let scrub_marker_pos = args
            .iter()
            .position(|arg| arg == "--scrub-marker")
            .expect("--scrub-marker present");
        assert_eq!(args[scrub_marker_pos + 1], "SYNTHETIC_MARKER_ONLY");
    }

    #[test]
    fn resolve_create_command_with_no_profile_id_takes_the_no_branch_path() {
        let (command, env_overlay, scrub, hook_config) = resolve_create_command(None)
            .expect("absent profile_id must never fail resolution");
        assert_eq!(command, None, "absent profile_id must not resolve to a default command");
        assert!(env_overlay.is_empty());
        assert!(scrub.is_none(), "absent profile_id must not resolve to a default scrub profile");
        assert!(hook_config.is_none(), "absent profile_id must not resolve to a default hook config");
    }

    #[test]
    fn resolve_create_command_with_claude_resolves_command_and_claude_scrub() {
        let (command, env_overlay, scrub, hook_config) = resolve_create_command(Some("claude"))
            .expect("the claude profile must resolve");
        let (program, args) = command.expect("a resolved profile must produce a command");
        assert_eq!(program, "claude");
        assert!(args.is_empty());
        assert!(env_overlay.is_empty());
        assert_eq!(
            scrub.expect("a resolved profile must produce a scrub list").name,
            "claude"
        );
        assert!(hook_config.is_some(), "the claude profile must resolve a hook config");
    }

    #[test]
    fn resolve_create_command_with_dummy_echo_resolves_no_hook_config() {
        let (command, _env_overlay, _scrub, hook_config) = resolve_create_command(Some("dummy-echo"))
            .expect("the dummy-echo profile must resolve");
        assert!(command.is_some());
        assert!(hook_config.is_none(), "the test-only profile must not carry hooks");
    }

    #[test]
    fn resolve_create_command_rejects_an_unknown_profile_id() {
        assert!(matches!(
            resolve_create_command(Some("not-a-real-profile")),
            Err(TerminalError::BadRequest(_))
        ));
    }

    // CONTRACT (260725 Phase 5): `remove`/`remove_for_work_roots` must forget
    // a closed terminal's attention entry at the SAME two choke points that
    // already forget its callback token - see each method's own CONTRACT.
    // These tests reach `registry.attention` directly (private field, same
    // crate module) rather than through an HTTP route, so they exercise the
    // choke point in isolation from `post_terminal_turn_state`'s own tests
    // (`tests/routes.rs`).
    #[tokio::test]
    async fn remove_forgets_the_attention_entry() {
        let registry = TerminalRegistry::default();
        insert_fake_live_session_for_test(&registry, "term_forget_on_remove").await;
        registry.attention.record_and_publish(
            "term_forget_on_remove".to_owned(),
            WorkRootId::from("fake-work-root".to_owned()),
            crate::agent_turn_state::TurnState::Working,
        );
        assert_eq!(registry.attention.snapshot().len(), 1);

        registry.remove("term_forget_on_remove");

        assert!(
            registry.attention.snapshot().is_empty(),
            "remove must forget the attention entry, not just the session and token"
        );
    }

    // CONTRACT (260727 Phase 3): `drain_all` is the THIRD choke point subject
    // to the same forget-on-removal rule - a kill-all sweep must forget every
    // drained terminal's attention entry, not merely empty the sessions map.
    // Two sessions, not one: a forget loop that only handled the first
    // drained session (e.g. a `.take(1)`-shaped bug) would still pass this
    // test if it seeded and drained just one session, so this test seeds TWO
    // and asserts both are gone. The pre-drain `len() == 2` assertion is the
    // non-vacuity control: without it, a fixture that never published an
    // entry would satisfy the post-drain `is_empty()` for entirely the wrong
    // reason. Attention-only, so the token-free
    // `insert_fake_live_session_for_test` is the right seeder and
    // `TerminalRegistry::default()` is safe here (no on-disk assertion - see
    // `remove_forgets_the_callback_token` for the temp-state-dir form).
    #[tokio::test]
    async fn drain_all_forgets_the_attention_entry() {
        let registry = TerminalRegistry::default();
        insert_fake_live_session_for_test(&registry, "term_forget_on_drain_all_a").await;
        insert_fake_live_session_for_test(&registry, "term_forget_on_drain_all_b").await;
        registry.attention.record_and_publish(
            "term_forget_on_drain_all_a".to_owned(),
            WorkRootId::from("fake-work-root".to_owned()),
            crate::agent_turn_state::TurnState::Working,
        );
        registry.attention.record_and_publish(
            "term_forget_on_drain_all_b".to_owned(),
            WorkRootId::from("fake-work-root".to_owned()),
            crate::agent_turn_state::TurnState::Working,
        );
        assert_eq!(
            registry.attention.snapshot().len(),
            2,
            "non-vacuity control: both attention entries must be PRESENT \
             before the drain, or the post-drain assertion proves nothing"
        );

        let drained = registry.drain_all();

        assert_eq!(
            drained.len(),
            2,
            "drain_all must return every drained session, not just some of them"
        );
        assert!(
            registry.attention.snapshot().is_empty(),
            "drain_all must forget the attention entry of EVERY drained \
             session, not just the first, and not just empty the sessions map"
        );
    }

    #[tokio::test]
    async fn remove_for_work_roots_forgets_the_attention_entry() {
        let registry = TerminalRegistry::default();
        let work_root_id = WorkRootId::from("fake-work-root".to_owned());
        insert_fake_live_session_for_test(&registry, "term_forget_on_workroot_removal").await;
        registry.attention.record_and_publish(
            "term_forget_on_workroot_removal".to_owned(),
            work_root_id.clone(),
            crate::agent_turn_state::TurnState::Ready,
        );
        assert_eq!(registry.attention.snapshot().len(), 1);

        let work_root_ids = BTreeSet::from([work_root_id]);
        registry.remove_for_work_roots(&work_root_ids);

        assert!(
            registry.attention.snapshot().is_empty(),
            "remove_for_work_roots must forget every removed session's attention entry"
        );
    }

    // CONTRACT (260725 Phase 5 review cycle 1, finding A; gate corrected at
    // the 260728 sweep/eviction merge): `insert`'s own opening
    // `sessions.retain` is a FIFTH session-removal path, distinct from
    // `remove`/`remove_for_work_roots` above - this test watches THAT
    // specific path (not just any removal), since the finding was that a
    // removal path was silently missed even though the other two were wired
    // correctly. The retain gates on `!admits_attach()`, NOT `!is_live()`
    // (see `TerminalSession::admits_attach`'s own CONTRACT for why), so
    // marking the fixture merely `Exited` is no longer enough to make it
    // eviction-eligible - a just-exited session is still inside its 30s
    // attach grace. The fixture therefore expires the grace window too;
    // dropping that step silently turns this test vacuous (nothing is
    // evicted, and the post-insert snapshot is empty for the wrong reason).
    #[tokio::test]
    async fn insert_forgets_the_attention_entry_of_a_session_its_own_eviction_retain_drops() {
        let registry = TerminalRegistry::default();
        insert_fake_live_session_for_test(&registry, "term_evicted_by_insert").await;
        registry.attention.record_and_publish(
            "term_evicted_by_insert".to_owned(),
            WorkRootId::from("fake-work-root".to_owned()),
            crate::agent_turn_state::TurnState::Ready,
        );
        assert_eq!(registry.attention.snapshot().len(), 1);

        // Mark the fake session not-live, mirroring a helper that exited
        // without the browser ever issuing a `DELETE` for it - so the NEXT
        // `insert` call's eviction `retain` (not `remove`, not
        // `remove_for_work_roots`) is what drops it from `sessions`.
        {
            let sessions = registry
                .sessions
                .read()
                .expect("terminal registry lock poisoned");
            let evicted = sessions
                .get("term_evicted_by_insert")
                .expect("fake session must be present before eviction");
            evicted.apply_helper_status(TerminalStatus::Exited, 1);
            // Expire the attach grace `apply_helper_status` just opened, so
            // `admits_attach()` - the retain's actual gate - reads false.
            evicted
                .inner
                .lock()
                .expect("terminal session lock poisoned")
                .grace_until_ms = Some(0);
        }

        // A fresh, unrelated, real `insert` (not `insert_unchecked` -
        // `insert_unchecked` skips the eviction retain entirely and would
        // not exercise this path) is what runs the retain under test.
        let trigger_session = Arc::new(fake_terminal_session().await);
        registry
            .insert(trigger_session)
            .await
            .expect("inserting an unrelated live session must succeed");

        // Non-vacuity guard: the assertion below is only meaningful if the
        // retain actually dropped the session. Without this, a fixture that
        // stopped being eviction-eligible (see the gate note above) would
        // pass the snapshot check for free.
        assert!(
            registry.get("term_evicted_by_insert").is_none(),
            "the fixture must actually have been evicted by insert's retain, or the attention \
             assertion below proves nothing"
        );
        assert!(
            registry.attention.snapshot().is_empty(),
            "insert's eviction retain must forget the evicted session's attention entry, not \
             just remove it from `sessions` - otherwise a helper that exits without a browser \
             DELETE leaks its last state into every future snapshot for the daemon's lifetime"
        );
    }

    // CONTRACT (260728 sweep/eviction merge): the callback-token half of the
    // eviction gap the test above covers the attention half of. Both
    // eviction paths (`insert`'s lazy retain and `sweep_evict_expired`)
    // discharge through the shared `evict_and_close`, so exercising either
    // one proves the shared discharge; this drives `sweep_evict_expired` so
    // the two tests between them cover both entry points into it. Before
    // this merge, eviction forgot attention but NOT the token - an evicted
    // terminal's credential kept authenticating turn-state callbacks and its
    // `terminal-tokens/<id>.json` stayed on disk for the daemon's lifetime
    // (the deviation `260728-bug-dashboard-terminal-eviction-leaks-callback-token`
    // named). Built on the token-bearing fixture and an explicit temp
    // `state_dir` for the same two reasons `remove_forgets_the_callback_token`
    // documents above.
    #[tokio::test]
    async fn eviction_forgets_the_callback_token_and_the_attention_entry() {
        let unique = format!("{}-{}", std::process::id(), now_ms());
        let base =
            std::env::temp_dir().join(format!("ws-dashboard-callback-token-evict-{unique}"));
        let state_dir = base.join("state");
        let registry_dir = base.join("terminals");
        std::fs::create_dir_all(&state_dir).expect("create temp state dir fixture");

        let registry = TerminalRegistry::new(
            default_helper_binary(),
            registry_dir,
            DEFAULT_CONNECT_TIMEOUT,
            Some(state_dir.clone()),
            String::new(),
        );
        let terminal_id = "term_forget_token_on_eviction";
        let token = "fake-callback-token-evict";
        insert_fake_live_session_with_token_for_test(&registry, terminal_id, token).await;
        crate::agent_token_store::write_token(&state_dir, terminal_id, token)
            .expect("write fake on-disk token file");
        registry.attention.record_and_publish(
            terminal_id.to_owned(),
            WorkRootId::from("fake-work-root".to_owned()),
            crate::agent_turn_state::TurnState::Ready,
        );

        assert_eq!(
            registry.token_for(terminal_id),
            Some(token.to_owned()),
            "token must resolve before eviction or this test proves nothing"
        );
        assert!(
            crate::agent_token_store::token_store_path(&state_dir, terminal_id).exists(),
            "on-disk token file must exist before eviction or this test proves nothing"
        );
        assert_eq!(registry.attention.snapshot().len(), 1);

        // Drive the session past `admits_attach()` (exited AND out of its
        // attach grace) so the sweep's retain is what removes it.
        {
            let sessions = registry
                .sessions
                .read()
                .expect("terminal registry lock poisoned");
            let session = sessions
                .get(terminal_id)
                .expect("fixture session must be present before eviction");
            session.apply_helper_status(TerminalStatus::Exited, 1);
            session
                .inner
                .lock()
                .expect("terminal session lock poisoned")
                .grace_until_ms = Some(0);
        }

        let evicted = registry.sweep_evict_expired();
        assert_eq!(
            evicted.len(),
            1,
            "the fixture must actually have been evicted, or the assertions below prove nothing"
        );
        registry.evict_and_close(evicted).await;

        assert!(
            registry.token_for(terminal_id).is_none(),
            "eviction must forget the in-memory callback token, not just drop the session"
        );
        assert!(
            !crate::agent_token_store::token_store_path(&state_dir, terminal_id).exists(),
            "eviction must best-effort delete the on-disk token file"
        );
        assert!(
            registry.attention.snapshot().is_empty(),
            "eviction must forget the attention entry too - both obligations, not one"
        );

        let _ = std::fs::remove_dir_all(&base);
    }

    // CONTRACT (260725 Phase 4, callback-token counterpart to the
    // `*_forgets_the_attention_entry` trio above): `remove`/
    // `remove_for_work_roots` must forget a closed terminal's callback token
    // at the same two choke points they already forget its attention entry
    // at - see each method's own CONTRACT. Built via the token-bearing
    // `insert_fake_live_session_with_token_for_test`, NOT the shared
    // `insert_fake_live_session_for_test` used above: that shared helper
    // hard-codes `callback_token: None`, which `remember_token` no-ops on,
    // so a token test built on it would pass its post-removal assertion
    // vacuously. Constructed via `TerminalRegistry::new` with an explicit
    // temp `state_dir`, NOT `TerminalRegistry::default()`: `Default`
    // resolves the real `default_state_dir()`, and `forget_token`'s
    // best-effort `agent_token_store::delete_token` would then create/delete
    // a real file under the developer's actual `terminal-tokens/` on every
    // `cargo test` run and pass silently (mirrors
    // `spawn_marks_the_profile_pending_before_writing_the_first_sidecar_byte`'s
    // temp-dir shape below).
    #[tokio::test]
    async fn remove_forgets_the_callback_token() {
        let unique = format!("{}-{}", std::process::id(), now_ms());
        let base =
            std::env::temp_dir().join(format!("ws-dashboard-callback-token-remove-{unique}"));
        let state_dir = base.join("state");
        let registry_dir = base.join("terminals");
        std::fs::create_dir_all(&state_dir).expect("create temp state dir fixture");

        let registry = TerminalRegistry::new(
            default_helper_binary(),
            registry_dir,
            DEFAULT_CONNECT_TIMEOUT,
            Some(state_dir.clone()),
            String::new(),
        );
        let terminal_id = "term_forget_token_on_remove";
        let token = "fake-callback-token-remove";
        insert_fake_live_session_with_token_for_test(&registry, terminal_id, token).await;
        crate::agent_token_store::write_token(&state_dir, terminal_id, token)
            .expect("write fake on-disk token file");

        // Non-vacuity guard: without this pre-removal assertion, a helper
        // that silently never seeded a token (e.g. a fixture regression)
        // would still pass the post-removal `is_none()` check for free.
        assert_eq!(
            registry.token_for(terminal_id),
            Some(token.to_owned()),
            "token must resolve before removal or this test proves nothing"
        );
        assert!(
            crate::agent_token_store::token_store_path(&state_dir, terminal_id).exists(),
            "on-disk token file must exist before removal or this test proves nothing"
        );

        registry.remove(terminal_id);

        assert!(
            registry.token_for(terminal_id).is_none(),
            "remove must forget the in-memory callback token, not just the session"
        );
        assert!(
            !crate::agent_token_store::token_store_path(&state_dir, terminal_id).exists(),
            "remove must best-effort delete the on-disk token file"
        );

        let _ = std::fs::remove_dir_all(&base);
    }

    #[tokio::test]
    async fn remove_for_work_roots_forgets_the_callback_token() {
        let unique = format!("{}-{}", std::process::id(), now_ms());
        let base = std::env::temp_dir().join(format!(
            "ws-dashboard-callback-token-remove-for-work-roots-{unique}"
        ));
        let state_dir = base.join("state");
        let registry_dir = base.join("terminals");
        std::fs::create_dir_all(&state_dir).expect("create temp state dir fixture");

        let registry = TerminalRegistry::new(
            default_helper_binary(),
            registry_dir,
            DEFAULT_CONNECT_TIMEOUT,
            Some(state_dir.clone()),
            String::new(),
        );
        let terminal_id = "term_forget_token_on_workroot_removal";
        let token = "fake-callback-token-workroot-removal";
        insert_fake_live_session_with_token_for_test(&registry, terminal_id, token).await;
        crate::agent_token_store::write_token(&state_dir, terminal_id, token)
            .expect("write fake on-disk token file");

        assert_eq!(
            registry.token_for(terminal_id),
            Some(token.to_owned()),
            "token must resolve before removal or this test proves nothing"
        );
        assert!(
            crate::agent_token_store::token_store_path(&state_dir, terminal_id).exists(),
            "on-disk token file must exist before removal or this test proves nothing"
        );

        // Same `work_root_id` `insert_fake_live_session_with_token_for_test`
        // inserts under ("fake-work-root", mirroring
        // `insert_fake_live_session_for_test`'s own hard-coded value).
        let work_root_ids = BTreeSet::from([WorkRootId::from("fake-work-root".to_owned())]);
        registry.remove_for_work_roots(&work_root_ids);

        assert!(
            registry.token_for(terminal_id).is_none(),
            "remove_for_work_roots must forget the in-memory callback token, not just the \
             session"
        );
        assert!(
            !crate::agent_token_store::token_store_path(&state_dir, terminal_id).exists(),
            "remove_for_work_roots must best-effort delete the on-disk token file"
        );

        let _ = std::fs::remove_dir_all(&base);
    }

    #[test]
    fn attention_handle_returned_by_app_state_construction_shares_the_registrys_own_hub() {
        // CONTRACT: proves the `attention()` accessor hands out a clone of
        // the SAME hub, not a fresh disconnected one - a write through one
        // handle must be visible through the other.
        let registry = TerminalRegistry::default();
        let app_state_attention = registry.attention();

        app_state_attention.record_and_publish(
            "term_shared".to_owned(),
            WorkRootId::from("fake-work-root".to_owned()),
            crate::agent_turn_state::TurnState::Idle,
        );

        assert_eq!(
            registry.attention.snapshot().len(),
            1,
            "a write through the accessor's returned handle must be visible through the \
             registry's own internal handle"
        );
    }

    // CONTRACT (260726 profile-provenance fix, ORDERING PROOF for the hoisted
    // `mark_profile_pending` - deliberately NOT a timing poll):
    //
    // What has to hold is that `spawn` marks the id pending BEFORE the first
    // byte of `agent-profiles/<id>/` exists, now that a HOOKLESS profile
    // (`dummy-echo`) creates that directory too. A poll that races the spawn
    // and hopes to observe the in-between state proves little, so this test
    // removes the race instead: `mark_profile_pending` takes the WRITE lock
    // on `pending_profile_ids`, so a READ guard held by this test parks the
    // spawn task ON that exact line. While parked it cannot execute a single
    // statement past the mark - including `write_profile`.
    //
    // The single load-bearing assertion is `!profile_root.exists()`. Its
    // premise is that the task has entered `spawn` (it signalled from inside
    // itself, and everything between that signal and the mark is
    // straight-line synchronous code with no `.await`, so it cannot be parked
    // anywhere else) and is therefore parked on `mark_profile_pending`'s
    // `.write()`, which cannot return while this test holds a read guard. A
    // build that wrote the sidecar before marking would have written it from
    // the same parked position, so the absent directory is what discriminates.
    // Releasing the guard then lets the same spawn finish the write, so the
    // negative half cannot be vacuous for want of a reachable write.
    //
    // The `pending_guard.is_empty()` assertion below is NOT independent
    // evidence of ordering - it is entailed by holding the read guard, since
    // no writer can publish into the set meanwhile. It is kept as a cheap
    // tripwire: it fires only if some future refactor lets the pending mark
    // become visible without taking that write lock, which is exactly the
    // premise the paragraph above rests on.
    //
    // Honest residual: this pins WHERE the task is parked, not the wall-clock
    // instant it got there, so the window below still has to be generous
    // enough for a mis-ordered build to have performed its write. That is the
    // one assumption left, and it is the assumption a mutation run checks.
    //
    // The helper binary is deliberately nonexistent: `spawn_detached` fails
    // with ENOENT well AFTER the sidecar write, so no real helper process is
    // ever created and this test asserts nothing about `spawn`'s return.
    #[cfg(unix)]
    #[allow(clippy::await_holding_lock)]
    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn spawn_marks_the_profile_pending_before_writing_the_first_sidecar_byte() {
        let unique = format!("{}-{}", std::process::id(), now_ms());
        let base = std::env::temp_dir().join(format!("ws-dashboard-profile-pending-order-{unique}"));
        let state_dir = base.join("state");
        let registry_dir = base.join("terminals");
        let root_path = base.join("root");
        std::fs::create_dir_all(&root_path).expect("create work root fixture");

        let registry = TerminalRegistry::new(
            PathBuf::from("/nonexistent-unused-helper-binary"),
            registry_dir.clone(),
            Duration::from_millis(200),
            Some(state_dir.clone()),
            String::new(),
        );
        let profile_root = state_dir.join("agent-profiles");

        let pending_guard = registry
            .pending_profile_ids
            .read()
            .expect("terminal registry lock poisoned");

        let (started_tx, started_rx) = tokio::sync::oneshot::channel::<()>();
        let task_registry = registry.clone();
        let task_registry_dir = registry_dir.clone();
        let task_state_dir = state_dir.clone();
        let task_root_path = root_path.clone();
        let spawn_task = tokio::spawn(async move {
            let (command, env_overlay, scrub, hook_config) =
                resolve_create_command(Some("dummy-echo")).expect("dummy-echo must resolve");
            assert!(
                hook_config.is_none(),
                "this test's whole point is the HOOKLESS path; dummy-echo must stay hookless"
            );
            started_tx
                .send(())
                .expect("ordering-proof start signal receiver alive");
            let _ = TerminalSession::spawn(
                &task_registry,
                Path::new("/nonexistent-unused-helper-binary"),
                &task_registry_dir,
                Duration::from_millis(200),
                Some(task_state_dir.as_path()),
                "",
                WorkRootId::from("root-ordering-proof".to_owned()),
                task_root_path,
                "Ordering proof".to_owned(),
                80,
                24,
                None,
                command,
                env_overlay,
                Some("dummy-echo".to_owned()),
                scrub,
                hook_config,
            )
            .await;
        });

        started_rx.await.expect("ordering-proof start signal");
        tokio::time::sleep(Duration::from_millis(750)).await;

        assert!(
            pending_guard.is_empty(),
            "the spawn task must still be parked ON mark_profile_pending's write lock; a \
             non-empty set here would mean it got past the mark while this read guard is held, \
             invalidating the rest of this proof"
        );
        assert!(
            !profile_root.exists(),
            "mark_profile_pending must run BEFORE the first byte of agent-profiles/<id>/ for a \
             hookless profile - found {profile_root:?} while the mark itself is provably still \
             blocked"
        );

        drop(pending_guard);
        spawn_task.await.expect("ordering-proof spawn task");

        // The negative half above is only meaningful if the write is reachable
        // at all once the mark is unblocked.
        let written: Vec<PathBuf> = std::fs::read_dir(&profile_root)
            .expect("agent-profiles must exist once the mark is released")
            .filter_map(|entry| entry.ok().map(|entry| entry.path()))
            .collect();
        assert_eq!(
            written.len(),
            1,
            "exactly one hookless profile directory must have been created: {written:?}"
        );
        assert_eq!(
            crate::agent_profile_store::read_profile_id(&written[0]).as_deref(),
            Some("dummy-echo")
        );
        assert!(
            !written[0].join("callback.json").exists(),
            "a hookless profile must never get a callback target"
        );
        assert!(
            registry
                .pending_profile_ids
                .read()
                .expect("terminal registry lock poisoned")
                .is_empty(),
            "spawn's own failure path must clear the pending mark it set"
        );

        let _ = std::fs::remove_dir_all(&base);
    }

    // ================================================================
    // 260729 helper liveness probe - "kill only on positive evidence of
    // unreachability".
    //
    // FIXTURE FIDELITY NOTE, stated once for every test below. A helper is
    // two things the daemon looks at separately: an OS PROCESS (identified by
    // `pid`/`start_time` in its registry entry, which is what
    // `kill_verified` signals) and an IPC LISTENER at `socket_path` (which is
    // what reachability is decided from). These fixtures supply the two
    // independently - a real `sleep 30` child for the process, so "was it
    // actually signalled?" is a real observation rather than a mock's
    // recorded call, and an in-process scripted listener for the socket, so
    // each leg of the predicate can be produced deterministically instead of
    // waited for. Neither half is a stub of the code under test: the daemon
    // runs its real `connect_and_handshake`, real `probe_helper`, real
    // `classify`, real sweep and real GC against them.
    //
    // The scripted listener deliberately never completes a handshake in the
    // probe-answering variants. That is exactly the wire signature this
    // ticket is about - `serve_connections` serves one session at a time, so
    // a helper busy with another daemon accepts the connection and answers no
    // handshake - and it is also what keeps the probe, rather than an adopt,
    // the thing being tested at both Site A and Site B.
    // ================================================================

    #[cfg(unix)]
    #[derive(Clone, Copy, Debug)]
    enum FakeHelperBehaviour {
        /// Accepts and never says anything at all. Models a helper that
        /// PREDATES the probe while it is busy: connect succeeds, the
        /// handshake never arrives, and nothing can answer a probe.
        SilentForever,
        /// Accepts, never handshakes, but answers `LivenessProbe` with this
        /// self-report - the signature of a probe-capable helper whose single
        /// session belongs to somebody else.
        AnswersProbe {
            attached: bool,
            unattached_for_ms: Option<u64>,
        },
        /// Completes a full adoption handshake (`Handshake` + `Status`) and
        /// then keeps recording. Used to pin the adoption sequence.
        Adoptable,
        /// Writes `chunks` `Output` lines spaced `interval_ms` apart before
        /// answering anything, then answers `LivenessProbe`. Models the path
        /// a REAL unattached helper answers a probe from
        /// (`handle_connection`): the entire retained ring - up to
        /// `MAX_OUTPUT_CHUNKS` (1024) chunks, each its own write+flush - is
        /// flushed ahead of the answer. Scripted with explicit spacing so the
        /// daemon-side timeout semantics can be asserted deterministically
        /// instead of raced against a real machine's I/O speed.
        FloodsThenAnswersProbe { chunks: u64, interval_ms: u64 },
    }

    #[cfg(unix)]
    struct FakeHelper {
        /// Every NDJSON line the DAEMON wrote, in order. The upgrade-safety
        /// assertions are made against this: a `livenessProbe` line reaching
        /// a helper that never declared the capability is the failure that
        /// SIGKILLs the user's shell.
        received: Arc<Mutex<Vec<String>>>,
        accept_task: tokio::task::JoinHandle<()>,
    }

    #[cfg(unix)]
    impl FakeHelper {
        fn received(&self) -> Vec<String> {
            self.received.lock().expect("fake helper lock poisoned").clone()
        }
    }

    #[cfg(unix)]
    impl Drop for FakeHelper {
        fn drop(&mut self) {
            self.accept_task.abort();
        }
    }

    #[cfg(unix)]
    fn spawn_fake_helper(socket_path: &Path, behaviour: FakeHelperBehaviour) -> FakeHelper {
        // Bound synchronously, before this function returns, so a daemon path
        // that connects immediately afterwards can never race the bind and
        // see a spurious `NoListener`.
        let mut listener = crate::terminal_ipc_transport::IpcListener::bind(socket_path)
            .expect("bind fake helper listener");
        let received = Arc::new(Mutex::new(Vec::new()));
        let received_for_task = received.clone();
        let accept_task = tokio::spawn(async move {
            loop {
                let Ok(stream) = listener.accept().await else {
                    return;
                };
                let received = received_for_task.clone();
                tokio::spawn(async move {
                    serve_fake_helper_connection(stream, behaviour, received).await;
                });
            }
        });
        FakeHelper {
            received,
            accept_task,
        }
    }

    #[cfg(unix)]
    async fn serve_fake_helper_connection(
        stream: crate::terminal_ipc_transport::BoxedIpcStream,
        behaviour: FakeHelperBehaviour,
        received: Arc<Mutex<Vec<String>>>,
    ) {
        use tokio::io::AsyncBufReadExt;

        let (read_half, mut write_half) = crate::terminal_ipc_transport::split(stream);
        if matches!(behaviour, FakeHelperBehaviour::Adoptable) {
            let _ = write_ndjson(
                &mut write_half,
                &HelperToDaemonMessage::Handshake {
                    // Identity is not what these tests exercise; the daemon
                    // verifies identity from the registry entry's pid against
                    // the real `sleep` child, not from this message.
                    pid: std::process::id(),
                    start_time: 0,
                },
            )
            .await;
            let _ = write_ndjson(
                &mut write_half,
                &HelperToDaemonMessage::Status {
                    status: TerminalHelperStatus::Running,
                    next_sequence: 1,
                },
            )
            .await;
        }

        if let FakeHelperBehaviour::FloodsThenAnswersProbe {
            chunks,
            interval_ms,
        } = behaviour
        {
            for sequence in 1..=chunks {
                let _ = write_ndjson(
                    &mut write_half,
                    &HelperToDaemonMessage::Output(TerminalHelperOutputChunk {
                        sequence,
                        data: "x".repeat(64),
                    }),
                )
                .await;
                tokio::time::sleep(Duration::from_millis(interval_ms)).await;
            }
        }

        // What this fixture will report if asked. Kept as data rather than
        // matched inline so a new probe-answering behaviour cannot silently
        // stop answering.
        let probe_report = match behaviour {
            FakeHelperBehaviour::AnswersProbe {
                attached,
                unattached_for_ms,
            } => Some((attached, unattached_for_ms)),
            // A helper that just finished flushing a ring is by definition
            // unattached, and freshly so.
            FakeHelperBehaviour::FloodsThenAnswersProbe { .. } => Some((false, Some(50))),
            FakeHelperBehaviour::SilentForever | FakeHelperBehaviour::Adoptable => None,
        };

        // Raw lines, not decoded messages: the point of `received` is to
        // record what the daemon actually put on the wire, including anything
        // this fixture's own types would not understand.
        let mut lines = tokio::io::BufReader::new(read_half).lines();
        while let Ok(Some(line)) = lines.next_line().await {
            if line.trim().is_empty() {
                continue;
            }
            received
                .lock()
                .expect("fake helper lock poisoned")
                .push(line.clone());
            let Ok(message) = serde_json::from_str::<DaemonToHelperMessage>(&line) else {
                continue;
            };
            if let (DaemonToHelperMessage::LivenessProbe, Some((attached, unattached_for_ms))) =
                (&message, probe_report)
            {
                let _ = write_ndjson(
                    &mut write_half,
                    &HelperToDaemonMessage::LivenessProbeResponse {
                        attached,
                        unattached_for_ms,
                    },
                )
                .await;
            }
        }
    }

    /// One helper's worth of on-disk state under a single shared state dir:
    /// a real process to signal, a registry entry, a profile directory and a
    /// token - i.e. everything the four destructive paths can destroy.
    #[cfg(unix)]
    struct ProbeFixture {
        state_dir: PathBuf,
        registry_dir: PathBuf,
        terminal_id: String,
        process: std::process::Child,
        helper: Option<FakeHelper>,
    }

    #[cfg(unix)]
    impl ProbeFixture {
        /// `listener` = `None` means no listener behind the socket at all
        /// (positive absence). `declare_probe_capability = false` writes the
        /// entry BY HAND without the field, so the assertion is about the
        /// absent-field decode rather than about a default the writer
        /// happened to supply.
        async fn new(
            label: &str,
            listener: Option<FakeHelperBehaviour>,
            declare_probe_capability: bool,
        ) -> Self {
            // Deliberately terse: `socket_path` below is bound as a real
            // Unix domain socket, whose sun_path is capped at ~108 bytes.
            // The usual `ws-dashboard-<test>-<pid>-<nanos>` temp-dir shape
            // used elsewhere in this file overruns that once the
            // `terminals/<terminal_id>.sock` tail is appended.
            static FIXTURE_COUNTER: std::sync::atomic::AtomicU64 =
                std::sync::atomic::AtomicU64::new(0);
            let unique = FIXTURE_COUNTER.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
            let state_dir = std::env::temp_dir().join(format!(
                "wsd-{label}-{}-{unique}",
                std::process::id()
            ));
            let registry_dir = state_dir.join("terminals");
            std::fs::create_dir_all(&registry_dir).expect("create registry dir");
            let terminal_id = format!("term_{label}");
            let socket_path = registry_dir.join(format!("{terminal_id}.sock"));

            let process = std::process::Command::new("sleep")
                .arg("30")
                .spawn()
                .expect("spawn the real process a kill would have to terminate");
            let pid = process.id();
            let start_time = crate::terminal_platform::process_start_time(pid)
                .expect("real process must report a start time");

            if declare_probe_capability {
                let entry = TerminalRegistryEntry {
                    terminal_id: terminal_id.clone(),
                    work_root_id: "root-probe".to_owned(),
                    pid,
                    start_time,
                    boot_id: crate::terminal_platform::boot_identity(),
                    supports_liveness_probe: true,
                    socket_path: socket_path.clone(),
                    // Comfortably past `connect_timeout + STALE_ENTRY_SWEEP_
                    // MARGIN`, so the sweep's age gate can never be what
                    // spares this entry.
                    created_at_ms: now_ms().saturating_sub(60_000),
                    title: "Probe".to_owned(),
                    cwd_hint: None,
                    columns: 80,
                    rows: 24,
                };
                crate::terminal_registry_file::write_registry_entry(&registry_dir, &entry)
                    .expect("write registry entry");
            } else {
                // Hand-written, in the exact shape a pre-probe helper left on
                // disk: `supportsLivenessProbe` is ABSENT, not `false`.
                let boot_id = match crate::terminal_platform::boot_identity() {
                    Some(boot_id) => format!("{boot_id:?}"),
                    None => "null".to_owned(),
                };
                let raw = format!(
                    r#"{{"terminalId":"{terminal_id}","workRootId":"root-probe","pid":{pid},"startTime":{start_time},"bootId":{boot_id},"socketPath":"{socket}","createdAtMs":{created},"title":"Probe","cwdHint":null,"columns":80,"rows":24}}"#,
                    socket = socket_path.display(),
                    created = now_ms().saturating_sub(60_000),
                );
                std::fs::write(
                    crate::terminal_registry_file::registry_entry_path(&registry_dir, &terminal_id),
                    raw,
                )
                .expect("write legacy registry entry");
                assert!(
                    !crate::terminal_registry_file::read_registry_entry(
                        &registry_dir,
                        &terminal_id
                    )
                    .expect("hand-written legacy entry must parse")
                    .supports_liveness_probe,
                    "fixture precondition: the hand-written entry must decode as \
                     capability-absent, or this stops testing the upgrade case"
                );
            }

            let profile_dir = state_dir.join("agent-profiles").join(&terminal_id);
            std::fs::create_dir_all(&profile_dir).expect("create profile dir");
            std::fs::write(profile_dir.join("settings.json"), "{}").expect("write settings.json");
            crate::agent_token_store::write_token(&state_dir, &terminal_id, "secret")
                .expect("write token");

            let helper = listener.map(|behaviour| spawn_fake_helper(&socket_path, behaviour));

            Self {
                state_dir,
                registry_dir,
                terminal_id,
                process,
                helper,
            }
        }

        /// A second daemon over the SAME state dir - the concurrent-daemon
        /// shape this ticket exists for. Nothing partitions the directory;
        /// this registry sees the other daemon's entries in full.
        fn second_registry(&self) -> TerminalRegistry {
            TerminalRegistry::new(
                PathBuf::from("/nonexistent-unused-helper-binary"),
                self.registry_dir.clone(),
                Duration::from_millis(150),
                Some(self.state_dir.clone()),
                String::new(),
            )
        }

        async fn second_registry_boot_reconcile(&self) -> TerminalRegistry {
            TerminalRegistry::boot_reconcile(
                PathBuf::from("/nonexistent-unused-helper-binary"),
                self.registry_dir.clone(),
                Duration::from_millis(150),
                Some(self.state_dir.clone()),
                String::new(),
            )
            .await
        }

        async fn run_profile_gc(&self, registry: &TerminalRegistry) {
            crate::agent_profile_gc::sweep_agent_profiles(
                &self.state_dir,
                registry,
                &mut crate::notify_failure::NotifyFailureWatch::default(),
                crate::server::AGENT_PROFILE_GC_SWEEP_PERIOD,
            )
            .await;
        }

        /// Gives any mis-fired SIGKILL every chance to land before an
        /// absence is asserted - a bare poll could otherwise pass by racing
        /// it.
        async fn process_is_alive(&mut self) -> bool {
            tokio::time::sleep(Duration::from_millis(200)).await;
            self.process.try_wait().expect("poll fixture process").is_none()
        }

        async fn process_was_killed(&mut self) -> bool {
            for _ in 0..50 {
                if self.process.try_wait().expect("poll fixture process").is_some() {
                    return true;
                }
                tokio::time::sleep(Duration::from_millis(20)).await;
            }
            false
        }

        fn entry_exists(&self) -> bool {
            crate::terminal_registry_file::registry_entry_path(
                &self.registry_dir,
                &self.terminal_id,
            )
            .exists()
        }

        fn profile_exists(&self) -> bool {
            self.state_dir
                .join("agent-profiles")
                .join(&self.terminal_id)
                .exists()
        }

        fn token_exists(&self) -> bool {
            crate::agent_token_store::read_token(&self.state_dir, &self.terminal_id).is_some()
        }

        fn cleanup(mut self) {
            let _ = self.process.kill();
            let _ = self.process.wait();
            let _ = std::fs::remove_dir_all(&self.state_dir);
        }
    }

    // CONTRACT (260729, the headline case): a helper attached to a SECOND
    // daemon must survive this daemon's `boot_reconcile` AND its 10s sweep,
    // with its registry entry intact. Both halves are load-bearing and fail
    // independently: sparing the process while deleting the entry passes a
    // process-only assertion and still destroys the terminal, because the
    // owning daemon can no longer adopt it at its next `boot_reconcile`.
    //
    // Before this ticket BOTH paths killed it. `reconcile_entry` folded
    // "connected, helper busy" into `IpcStatus::Unreachable` and `classify`
    // mapped that straight to `KillVerified`; the sweep had no liveness check
    // at all. The boot-identity gate cannot help - both daemons are on the
    // same boot, so the boot id matches by construction.
    #[cfg(unix)]
    #[tokio::test]
    async fn a_helper_attached_to_another_daemon_survives_boot_reconcile_and_the_sweep() {
        let mut fixture = ProbeFixture::new(
            "attached",
            Some(FakeHelperBehaviour::AnswersProbe {
                attached: true,
                unattached_for_ms: None,
            }),
            true,
        )
        .await;

        let registry = fixture.second_registry_boot_reconcile().await;
        assert!(
            registry.get(&fixture.terminal_id).is_none(),
            "a busy helper is not adoptable by this daemon - leaving it alone must not be \
             confused with adopting it"
        );
        assert!(
            fixture.entry_exists(),
            "boot reconcile must not delete the registry entry of a helper that answered it is \
             in use - deleting it makes the surviving helper unadoptable by its real owner"
        );

        registry.sweep_registry_backstop().await;

        assert!(
            fixture.process_is_alive().await,
            "neither boot reconcile nor the backstop sweep may signal a helper that reports \
             itself attached to another daemon"
        );
        assert!(
            fixture.entry_exists(),
            "the backstop sweep must leave the entry alone too, not merely withhold the signal"
        );

        fixture.cleanup();
    }

    // CONTRACT (260729, step 4): a SEPARATE assertion from the one above, and
    // it fails independently. `agent_profile_gc` is not a kill site and never
    // routes through `classify`, so steps 1-3 do not reach it: it reads
    // `live_terminal_ids()` directly, and another daemon's terminals are
    // absent from this daemon's session map no matter how healthy they are.
    // Without the probe gate the helper survives and its state is deleted
    // anyway - the original symptom this ticket is named after.
    #[cfg(unix)]
    #[tokio::test]
    async fn the_profile_gc_spares_a_helper_that_is_attached_to_another_daemon() {
        let fixture = ProbeFixture::new(
            "attached-gc",
            Some(FakeHelperBehaviour::AnswersProbe {
                attached: true,
                unattached_for_ms: None,
            }),
            true,
        )
        .await;

        let registry = fixture.second_registry();
        fixture.run_profile_gc(&registry).await;

        assert!(
            fixture.profile_exists(),
            "agent-profiles/<id>/ must survive a GC run by a daemon that does not own the \
             terminal but can see that its helper is in use"
        );
        assert!(
            fixture.token_exists(),
            "terminal-tokens/<id>.json must survive too - losing it makes the terminal's \
             callback auth unrecoverable across a restart"
        );

        fixture.cleanup();
    }

    // CONTRACT (260729, the predicate leg easiest to get wrong): a helper
    // that ANSWERS but reports unattached for LESS than the grace is a helper
    // whose own daemon is restarting - possibly still inside its
    // `boot_reconcile`, before it has re-adopted anything. Every path must
    // leave it alone. Without this case a two-way predicate ("answers or
    // not") satisfies every other bullet in this file while reintroducing
    // this ticket's bug in its narrower form: another daemon's 10s sweep
    // reaping live terminals during their owner's restart.
    #[cfg(unix)]
    #[tokio::test]
    async fn a_helper_whose_daemon_is_restarting_survives_every_path() {
        let mut fixture = ProbeFixture::new(
            "restarting",
            Some(FakeHelperBehaviour::AnswersProbe {
                attached: false,
                // Just disconnected: far inside the grace.
                unattached_for_ms: Some(50),
            }),
            true,
        )
        .await;

        let registry = fixture.second_registry_boot_reconcile().await;
        registry.sweep_registry_backstop().await;
        fixture.run_profile_gc(&registry).await;

        assert!(
            fixture.process_is_alive().await,
            "a helper unattached for less than the grace must never be signalled - its daemon \
             is coming back for it"
        );
        assert!(fixture.entry_exists(), "its registry entry must survive too");
        assert!(
            fixture.profile_exists(),
            "its profile directory must survive the GC"
        );
        assert!(fixture.token_exists(), "its token must survive the GC");

        fixture.cleanup();
    }

    // NON-VACUITY 1: positive absence still kills, on all three paths.
    // Without this, "spare everything" would pass every test above.
    #[cfg(unix)]
    #[tokio::test]
    async fn a_helper_with_no_listener_behind_it_is_still_reclaimed_everywhere() {
        // Three independent fixtures: the first two paths DELETE the entry,
        // so one fixture cannot be driven through all three.
        let mut boot = ProbeFixture::new("no-listener-boot", None, true).await;
        let registry = boot.second_registry_boot_reconcile().await;
        assert!(
            boot.process_was_killed().await,
            "boot reconcile must still kill a verified-ours helper with nothing listening"
        );
        assert!(
            !boot.entry_exists(),
            "and must still prune its registry entry"
        );
        drop(registry);
        boot.cleanup();

        let mut sweep = ProbeFixture::new("no-listener-sweep", None, true).await;
        let registry = sweep.second_registry();
        registry.sweep_registry_backstop().await;
        assert!(
            sweep.process_was_killed().await,
            "the backstop sweep must still kill a verified-ours helper with nothing listening"
        );
        assert!(!sweep.entry_exists(), "and must still prune its entry");
        sweep.cleanup();

        let gc = ProbeFixture::new("no-listener-gc", None, true).await;
        let registry = gc.second_registry();
        gc.run_profile_gc(&registry).await;
        assert!(
            !gc.profile_exists(),
            "the GC must still reclaim the profile directory of a helper with no listener"
        );
        assert!(!gc.token_exists(), "and its token");
        gc.cleanup();
    }

    // NON-VACUITY 2, and the one an earlier draft of this phase was missing.
    // A REAL orphan - live shell, no daemon coming back - keeps its listener
    // bound and sits in its idle accept loop forever, so it ANSWERS the
    // probe. Under the naive "kill only when the probe does not answer" rule
    // it becomes unreclaimable and its shell and PTY leak permanently and
    // silently. Without this case that exact regression passes the suite by
    // construction, because the orphan it leaks is precisely the one that
    // answers.
    #[cfg(unix)]
    #[tokio::test]
    async fn a_helper_that_answers_but_is_unattached_past_the_grace_is_still_reclaimed() {
        let past_grace = Some(UNATTACHED_GRACE.as_millis() as u64 + 60_000);

        let mut sweep = ProbeFixture::new(
            "past-grace-sweep",
            Some(FakeHelperBehaviour::AnswersProbe {
                attached: false,
                unattached_for_ms: past_grace,
            }),
            true,
        )
        .await;
        let registry = sweep.second_registry();
        registry.sweep_registry_backstop().await;
        assert!(
            sweep.process_was_killed().await,
            "a helper that answers the probe but has been unattached past the grace is a real \
             orphan and MUST still be reclaimed - sparing it leaks its shell and PTY forever"
        );
        assert!(!sweep.entry_exists(), "and its entry must be pruned");
        sweep.cleanup();

        let gc = ProbeFixture::new(
            "past-grace-gc",
            Some(FakeHelperBehaviour::AnswersProbe {
                attached: false,
                unattached_for_ms: past_grace,
            }),
            true,
        )
        .await;
        let registry = gc.second_registry();
        gc.run_profile_gc(&registry).await;
        assert!(
            !gc.profile_exists(),
            "the GC must still reclaim a past-grace orphan's profile directory"
        );
        assert!(!gc.token_exists(), "and its token");
        gc.cleanup();
    }

    // CONTRACT (260729, THE UPGRADE CASE - the one the user is standing in).
    // Helpers outlive their daemon by design, so a daemon carrying the probe
    // WILL meet helpers spawned by a binary that has never heard of it. Such
    // a helper has no concurrent accept arm: while it is attached, its
    // listener queues the probe and it answers nothing - which is the "no
    // answer" leg, i.e. kill. Left unhandled, the fix reproduces the exact
    // defect it was written to remove, for every helper alive across the
    // upgrade.
    //
    // Two independent failure modes are asserted here:
    //   1. the helper must survive all three paths with entry, profile and
    //      token intact, because "connected but silent" is undecidable for a
    //      peer that cannot speak the probe;
    //   2. NOTHING may be put on its wire. This is not hygiene - `read_message`
    //      turns an unknown variant into an `io::Error`, the helper's read
    //      site propagates it, and `run_terminal_helper`'s exit path then runs
    //      `kill_shell_if_running()` + `delete_registry_entry()`. Sending the
    //      probe here would SIGKILL the user's shell and erase the evidence,
    //      which is strictly worse than the daemon-side kill this ticket
    //      exists to remove.
    #[cfg(unix)]
    #[tokio::test]
    async fn a_helper_that_predates_the_probe_is_left_alone_and_never_sent_one() {
        let mut fixture = ProbeFixture::new(
            "pre-upgrade",
            Some(FakeHelperBehaviour::SilentForever),
            // The registry entry is hand-written WITHOUT the capability field.
            false,
        )
        .await;

        let registry = fixture.second_registry_boot_reconcile().await;
        registry.sweep_registry_backstop().await;
        fixture.run_profile_gc(&registry).await;

        assert!(
            fixture.process_is_alive().await,
            "a helper whose entry declares no probe capability must never be signalled for \
             staying silent - kill only on positive absence"
        );
        assert!(fixture.entry_exists(), "its registry entry must survive");
        assert!(fixture.profile_exists(), "its profile directory must survive");
        assert!(fixture.token_exists(), "its token must survive");

        let received = fixture
            .helper
            .as_ref()
            .expect("fixture has a listener")
            .received();
        assert!(
            received.is_empty(),
            "NOTHING may be written to a helper that predates the probe - it would parse the \
             unknown variant as a transport failure and SIGKILL the user's shell. Got: \
             {received:?}"
        );

        fixture.cleanup();
    }

    // CONTRACT (260729): the adoption sequence is the one path every live
    // helper depends on across the upgrade, and it is the one an implementer
    // is most likely to "tidy" while adding the probe. Adopting must put
    // NOTHING on the wire beyond `HandshakeAck`, and must complete against a
    // helper that sends nothing beyond `Handshake` + `Status`.
    #[cfg(unix)]
    #[tokio::test]
    async fn adopting_a_helper_still_puts_only_a_handshake_ack_on_the_wire() {
        let mut fixture =
            ProbeFixture::new("adopt", Some(FakeHelperBehaviour::Adoptable), true).await;

        let registry = fixture.second_registry_boot_reconcile().await;

        assert!(
            registry.get(&fixture.terminal_id).is_some(),
            "a helper that completes the pre-probe handshake sequence must still be adopted"
        );

        // The adopted session's reader task holds the connection open, so
        // give any extra write this path might have gained a chance to land
        // before asserting its absence.
        tokio::time::sleep(Duration::from_millis(200)).await;
        let received = fixture
            .helper
            .as_ref()
            .expect("fixture has a listener")
            .received();
        assert_eq!(
            received,
            vec![r#"{"type":"handshakeAck"}"#.to_owned()],
            "the adoption sequence must stay byte-identical: exactly one HandshakeAck, nothing \
             else - no probe, no version negotiation, no backfill request"
        );
        assert!(
            fixture.process_is_alive().await,
            "an adopted helper is obviously never signalled"
        );

        fixture.cleanup();
    }

    // Direct coverage of the socket-level probe, independent of any kill
    // site: the capability gate must be enforced HERE, at the one function
    // that writes `LivenessProbe`, so a future caller cannot reintroduce the
    // unguarded send by forgetting to check the entry first.
    #[cfg(unix)]
    #[tokio::test]
    async fn probe_helper_writes_nothing_to_a_peer_that_does_not_declare_the_capability() {
        let socket_path = std::env::temp_dir().join(format!(
            "ws-dashboard-probe-gate-{}-{}.sock",
            std::process::id(),
            now_ms()
        ));
        let _ = std::fs::remove_file(&socket_path);
        let helper = spawn_fake_helper(&socket_path, FakeHelperBehaviour::SilentForever);

        let verdict = probe_helper(&socket_path, false, Duration::from_millis(150)).await;

        assert_eq!(
            verdict,
            ProbeVerdict::Unsupported,
            "a connected peer that declares no capability is undecidable, not unreachable"
        );
        assert!(
            !probe_authorizes_reclaim(verdict),
            "and undecidable must never authorize reclaim"
        );
        assert!(
            helper.received().is_empty(),
            "probe_helper must connect but write NOTHING when the capability is not declared"
        );

        // Non-vacuity for the gate: the same listener, probed WITH the
        // capability declared, does receive the request - so the emptiness
        // above is the gate working, not the fixture failing to record.
        let verdict = probe_helper(&socket_path, true, Duration::from_millis(150)).await;
        assert_eq!(
            verdict,
            ProbeVerdict::Unanswered,
            "a capable-but-silent peer is the wedged case, which does authorize reclaim"
        );
        assert!(probe_authorizes_reclaim(verdict));
        assert!(
            helper
                .received()
                .iter()
                .any(|line| line.contains("livenessProbe")),
            "the probe must actually be sent once the entry declares the capability: {:?}",
            helper.received()
        );

        drop(helper);
        let _ = std::fs::remove_file(&socket_path);
    }

    // Positive absence, at the same seam: nothing bound behind the path.
    #[cfg(unix)]
    #[tokio::test]
    async fn probe_helper_reports_a_reclaimable_verdict_when_nothing_is_listening() {
        let socket_path = std::env::temp_dir().join(format!(
            "ws-dashboard-probe-absent-{}-{}.sock",
            std::process::id(),
            now_ms()
        ));
        let _ = std::fs::remove_file(&socket_path);

        let verdict = probe_helper(&socket_path, true, Duration::from_millis(80)).await;
        assert!(
            probe_authorizes_reclaim(verdict),
            "no listener is positive absence and must authorize reclaim for a helper that \
             declares the probe capability (got {verdict:?})"
        );
    }

    // CONTRACT (260729 review finding F9-gate): the capability gate is
    // evaluated BEFORE the connect, so a capability-absent entry produces
    // `Unsupported` even with nothing bound behind its socket path - the
    // daemon makes no socket contact at all with such a helper. This
    // deliberately WIDENS the "pre-upgrade helpers are spared" consequence
    // the ticket already accepted, from "spared while attached" to "spared
    // unconditionally", and it is asserted rather than left implicit because
    // it is the observable difference from the previous behaviour (which
    // connected first and reclaimed on a failed connect).
    //
    // Nothing is leaked by it: the sweep only probes after `identity_status`
    // returned `VerifiedOurs`, i.e. the helper process is provably alive, and
    // a capability-absent entry whose process has died is deleted by the
    // sweep's own non-`VerifiedOurs` arm without ever consulting the probe.
    #[cfg(unix)]
    #[tokio::test]
    async fn a_capability_absent_entry_is_never_probed_even_when_nothing_is_listening() {
        let socket_path = std::env::temp_dir().join(format!(
            "ws-dashboard-probe-legacy-absent-{}-{}.sock",
            std::process::id(),
            now_ms()
        ));
        let _ = std::fs::remove_file(&socket_path);

        let started = Instant::now();
        let verdict = probe_helper(&socket_path, false, Duration::from_millis(800)).await;

        assert_eq!(
            verdict,
            ProbeVerdict::Unsupported,
            "a helper that predates the probe must be undecidable, never positively absent - \
             connecting to it is what SIGKILLs its shell"
        );
        assert!(
            !probe_authorizes_reclaim(verdict),
            "and undecidable must never authorize reclaim"
        );
        assert!(
            started.elapsed() < Duration::from_millis(400),
            "the gate must short-circuit BEFORE the connect loop's retry budget is spent - \
             taking the full budget means a connect was attempted (took {:?})",
            started.elapsed()
        );
    }

    // CONTRACT (260729 review finding F10, the per-message half): the probe's
    // patience is bounded by "the peer went quiet", not by "the exchange has
    // taken too long". A helper answers a probe from `handle_connection`
    // whenever nobody is attached, and that path flushes the ENTIRE retained
    // ring first - up to `MAX_OUTPUT_CHUNKS` (1024) chunks, each its own
    // write+flush. Under one shared deadline a big scrollback is enough to
    // spend the whole budget, and the verdict falls through to `Unanswered`,
    // i.e. `KillVerified`: a live helper killed for being slow.
    //
    // The fixture drips for far longer than any single-deadline budget while
    // never going quiet for as long as the idle timeout, so it separates the
    // two policies exactly: restore a shared deadline and this goes red.
    #[cfg(unix)]
    #[tokio::test]
    async fn a_slow_ring_flush_cannot_exhaust_the_probe_timeout() {
        let socket_path = std::env::temp_dir().join(format!(
            "ws-dashboard-probe-flood-{}-{}.sock",
            std::process::id(),
            now_ms()
        ));
        let _ = std::fs::remove_file(&socket_path);
        let helper = spawn_fake_helper(
            &socket_path,
            FakeHelperBehaviour::FloodsThenAnswersProbe {
                chunks: 12,
                interval_ms: 120,
            },
        );

        let started = Instant::now();
        // The connect budget, deliberately left at a realistic value: the
        // point is that it does NOT bound the answer.
        let verdict = probe_helper(&socket_path, true, Duration::from_millis(150)).await;
        let elapsed = started.elapsed();

        assert_eq!(
            verdict,
            ProbeVerdict::UnattachedWithinGrace,
            "a helper that keeps talking must be read to its answer, not timed out mid-ring"
        );
        assert!(
            !probe_authorizes_reclaim(verdict),
            "and it must not be reclaimed"
        );
        assert!(
            elapsed > Duration::from_millis(1_000),
            "the answer must genuinely have arrived long after the connect budget - otherwise \
             this test would pass under a shared deadline too (elapsed {elapsed:?})"
        );

        drop(helper);
        let _ = std::fs::remove_file(&socket_path);
    }

    // The same scenario against a REAL helper, end to end: a real
    // `run_terminal_helper`, a real shell, a real retained ring larger than a
    // socket buffer, probed with the REAL production connect budget the sweep
    // uses. This is the integration-level guard for F10 - "a live helper with
    // scrollback is answered, not reclaimed".
    //
    // Honest scope note: this test is NOT the mutation instrument for the
    // timeout policy. On an unloaded machine a real helper flushes even a
    // multi-megabyte ring faster than the 400ms budget, so restoring the
    // shared deadline leaves it green; that is a statement about this
    // machine's speed, not about the policy being safe. The deterministic
    // instrument is `a_slow_ring_flush_cannot_exhaust_the_probe_timeout`
    // above, which scripts the timing instead of racing it.
    #[cfg(unix)]
    #[tokio::test]
    async fn a_real_helper_with_a_retained_ring_answers_a_probe_without_being_reclaimed() {
        use crate::terminal_helper_process::real_helper_test_support::RealHelper;

        // Deliberately more than a socket buffer's worth (~200KB on Linux):
        // a ring that fits entirely in the kernel buffer is flushed without
        // the daemon's reads ever having to wait, which would make this test
        // pass under a shared deadline too and prove nothing.
        let helper = RealHelper::spawn(
            "ring",
            "head -c 1500000 /dev/zero | tr '\\0' A; sleep 30",
        )
        .await;

        // Attach so the shell spawns and the ring fills, then leave: an
        // unattached helper with a live shell is exactly what the sweep meets
        // during another daemon's restart.
        let mut session = helper.attach().await;
        let mut chunks = 0;
        while chunks < 60 {
            if let crate::terminal_helper_protocol::HelperToDaemonMessage::Output(_) =
                session.read().await
            {
                chunks += 1;
            }
        }
        drop(session);
        // Let the helper observe the EOF and re-park in its own `accept()`,
        // so the probe below lands on the ring-flushing `handle_connection`
        // path rather than on the concurrent probe arm.
        tokio::time::sleep(Duration::from_millis(250)).await;

        let verdict = probe_helper(
            &helper.socket_path,
            true,
            DEFAULT_RECONCILE_CONNECT_TIMEOUT,
        )
        .await;

        assert_eq!(
            verdict,
            ProbeVerdict::UnattachedWithinGrace,
            "a real helper flushing a real ring must still be heard out"
        );
        assert!(
            !probe_authorizes_reclaim(verdict),
            "and must not be reclaimed for having scrollback"
        );
        assert!(
            helper.entry_exists(),
            "probing must not disturb the helper's registry entry"
        );

        helper.shutdown().await;
    }

    // Pins the F10 constant to the helper's own bound rather than to a
    // number: if either side is retuned without the other, the probe either
    // waits on a connection the helper has already closed, or gives up on a
    // helper that is still willing to answer.
    #[test]
    fn probe_response_idle_timeout_is_sized_against_the_helpers_own_bound() {
        assert_eq!(
            PROBE_RESPONSE_IDLE_TIMEOUT,
            crate::terminal_helper_process::PROBE_CONNECTION_TIMEOUT,
            "the probe's per-message idle timeout is defined by how long a helper will hold a \
             probe connection open, not by the connect budget"
        );
        assert!(
            PROBE_RESPONSE_IDLE_TIMEOUT > DEFAULT_RECONCILE_CONNECT_TIMEOUT,
            "sharing the connect budget is the F10 defect: a full retained ring is flushed \
             before the probe answer, and 400ms of it is enough to make a healthy helper look \
             unanswered"
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

// CONTRACT (260725 Phase 4, callback token): a longer, unprefixed sibling of
// `opaque_terminal_id` above - same crate/distribution, deliberately
// distinct purpose. `opaque_terminal_id` is displayed and logged freely (it
// names a terminal, not a secret); this token is a credential and must NEVER
// be displayed, logged, or forwarded through argv (hard constraint). 32
// alphanumeric characters (~190 bits of entropy over a 62-symbol alphabet)
// is far past what a per-terminal, non-rotating, POST-body credential needs.
const CALLBACK_TOKEN_LENGTH: usize = 32;

fn generate_callback_token() -> String {
    thread_rng()
        .sample_iter(&Alphanumeric)
        .take(CALLBACK_TOKEN_LENGTH)
        .map(char::from)
        .collect()
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

// CONTRACT (260725 Phase 4, cross-module test seam): `TerminalRegistry`'s
// `insert_unchecked` and `TerminalSession`'s fields all stay private to this
// module - this is the one deliberate exception, gated `#[cfg(test)]` so it
// never ships in a production build, letting `agent_profile_gc.rs`'s own
// tests seed a registry with a live session id without needing a real
// spawned helper process (this crate has no lighter-weight way to construct
// a `TerminalSession` - see `fake_terminal_session` above, which this
// mirrors, for the same real-but-unattached-IPC-duplex shape).
#[cfg(test)]
pub(crate) async fn insert_fake_live_session_for_test(registry: &TerminalRegistry, terminal_id: &str) {
    let (_peer, local) = tokio::io::duplex(4096);
    let (_read_half, write_half) =
        crate::terminal_ipc_transport::split(Box::new(local) as crate::terminal_ipc_transport::BoxedIpcStream);
    let session = Arc::new(TerminalSession {
        id: terminal_id.to_owned(),
        work_root_id: WorkRootId::from("fake-work-root".to_owned()),
        title: "fake".to_owned(),
        cwd_hint: None,
        created_at_ms: now_ms(),
        profile_id: None,
        callback_token: None,
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
        reader_task: Mutex::new(None),
    });
    registry.insert_unchecked(session);
}

// CONTRACT (260727 Phase 1, token-bearing sibling): identical to
// `insert_fake_live_session_for_test` above except `callback_token` is
// `Some(token)` instead of hard-coded `None`. That hard-coded `None` is
// exactly why the shared helper above cannot seed a token
// (`remember_token` no-ops when `session.callback_token` is `None`), and
// the helper is shared with `agent_profile_gc.rs`'s own tests, so changing
// its signature is a cross-module edit outside this phase's scope. Callers
// that need a token-bearing fake session (the `*_forgets_the_callback_token`
// tests above) use this sibling instead; do not merge the two or touch the
// existing helper's call sites.
#[cfg(test)]
pub(crate) async fn insert_fake_live_session_with_token_for_test(
    registry: &TerminalRegistry,
    terminal_id: &str,
    token: &str,
) {
    let (_peer, local) = tokio::io::duplex(4096);
    let (_read_half, write_half) =
        crate::terminal_ipc_transport::split(Box::new(local) as crate::terminal_ipc_transport::BoxedIpcStream);
    let session = Arc::new(TerminalSession {
        id: terminal_id.to_owned(),
        work_root_id: WorkRootId::from("fake-work-root".to_owned()),
        title: "fake".to_owned(),
        cwd_hint: None,
        created_at_ms: now_ms(),
        profile_id: None,
        callback_token: Some(token.to_owned()),
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
        reader_task: Mutex::new(None),
    });
    registry.insert_unchecked(session);
}

// CONTRACT (260725 Phase 4 review cycle 1, finding A): test-only hook so
// `agent_profile_gc.rs`'s concurrent-spawn regression test can reproduce the
// exact ordering `TerminalSession::spawn` performs in production - mark an
// id pending WITHOUT ever inserting a session for it - without needing a
// real process spawn and IPC handshake in the test itself. Mirrors
// `insert_fake_live_session_for_test`'s existing role for the "already
// live" case.
#[cfg(test)]
pub(crate) fn mark_profile_pending_for_test(registry: &TerminalRegistry, terminal_id: &str) {
    registry.mark_profile_pending(terminal_id);
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
