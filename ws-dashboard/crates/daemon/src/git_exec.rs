//! Single seam through which the daemon spawns `git`.
//!
//! Every git invocation in this crate should route through [`capture`] (or a
//! thin per-module wrapper over it) rather than calling `std::process::Command`
//! directly, so that:
//!
//! - every spawn is subject to a bounded wait with a hard kill on expiry
//!   (`WS_DASHBOARD_GIT_TIMEOUT_MS`, default 10s; `0` disables the bound and
//!   waits indefinitely, matching `WS_DASHBOARD_GIT_PROBE_TTL_MS`'s "`0`
//!   disables" reading) instead of the previous unbounded `.output()` call.
//!   The env var is read and parsed once per process, so the budget cannot
//!   silently change mid-run,
//! - stdout/stderr are drained concurrently with the wait so a large-output
//!   child can never deadlock against a full pipe buffer,
//! - every spawn (successful, failed, or timed out) is counted in a shared
//!   [`GitSpawnStats`], broken down by subcommand, and
//! - a genuinely unexpected failure is logged via `tracing::warn!`, while a
//!   call site that already expects routine non-zero exits (e.g. probing
//!   whether a branch has an upstream) can opt out of that log without losing
//!   the counter increment.
//!
//! `GitSpawnStats` is owned explicitly (an `Arc<GitSpawnStats>` living in
//! `AppState`, threaded as `&GitSpawnStats` into call sites) rather than a
//! process-global static, so the many concurrently-running git-spawning tests
//! in `tests/routes.rs` never pollute one another's counters.

use std::collections::BTreeMap;
use std::env;
use std::io::{self, Read};
use std::path::Path;
use std::process::{Command, ExitStatus, Stdio};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{mpsc, Mutex, OnceLock};
use std::time::{Duration, Instant};

/// Result of a successful (zero-exit, non-timed-out) `capture` call.
#[derive(Clone, Debug)]
pub struct GitOutcome {
    pub status: ExitStatus,
    /// Lossy (`from_utf8_lossy`) decode of the child's stdout, which is the
    /// right default for human-facing text output. Call sites that turn stdout
    /// into filesystem paths must use [`GitOutcome::stdout_strict`] instead.
    pub stdout: String,
    pub stderr: String,
    pub elapsed: Duration,
    stdout_valid_utf8: bool,
}

impl GitOutcome {
    /// Strict-UTF-8 view of stdout: `None` when the child's stdout was not
    /// valid UTF-8. Preserves the pre-seam `String::from_utf8(..).ok()?`
    /// semantics for the discovery probes, where a replacement-char-mangled
    /// path is worse than no path at all.
    pub fn stdout_strict(&self) -> Option<&str> {
        self.stdout_valid_utf8.then_some(self.stdout.as_str())
    }
}

/// Decode child output, reporting whether the bytes were valid UTF-8 so
/// [`GitOutcome::stdout_strict`] can offer the strict view without re-scanning
/// a lossy `String` (where a genuine U+FFFD is indistinguishable from a
/// substituted one).
fn decode_output(bytes: &[u8]) -> (String, bool) {
    match std::str::from_utf8(bytes) {
        Ok(text) => (text.to_owned(), true),
        Err(_) => (String::from_utf8_lossy(bytes).into_owned(), false),
    }
}

/// Why a `capture` call did not produce a [`GitOutcome`].
#[derive(Debug)]
pub enum GitFailure {
    /// The process could not be spawned or could not be waited on at all.
    Spawn(io::Error),
    /// The process outlived its budget and was killed.
    Timeout,
    /// The process exited with a non-zero status (`-1` when the exit code is
    /// unavailable, e.g. the process was signal-terminated).
    Status(i32),
}

/// Whether a non-zero exit at a given call site is routine (a probe whose
/// entire purpose is to answer yes/no via exit status) or a genuine failure
/// worth logging.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum GitFailureExpectation {
    /// A non-zero exit is a real problem and should be logged.
    Unexpected,
    /// A non-zero exit is a routine, expected outcome (e.g. no upstream
    /// configured, unborn HEAD, branch does not exist) and must not be
    /// logged, only counted.
    ExpectedNonZero,
}

/// Interned subcommand token, used as the `by_subcommand` breakdown key.
/// `&'static str` cannot serve as that key because it can only be derived
/// from a borrowed `args: &[&str]` at call time, not stored past the call.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub enum GitSubcommand {
    Branch,
    RevParse,
    ForEachRef,
    RevList,
    ShowRef,
    Switch,
    Fetch,
    Push,
    Pull,
    CheckRefFormat,
    Worktree,
    DiffIndex,
    Status,
    /// Everything else, including the test-fixture-only subcommands
    /// (`init`, `config`, `add`, `commit`, `mv`, ...), which don't need
    /// dedicated variants for a production-facing counter.
    Other,
}

impl GitSubcommand {
    /// Find the real subcommand token, skipping any leading `--`-prefixed
    /// flags (e.g. `--no-optional-locks` precedes `diff-index`/`status` in
    /// `changes_for_path`).
    pub fn from_args(args: &[&str]) -> Self {
        args.iter()
            .find(|arg| !arg.starts_with("--"))
            .map(|token| Self::from_token(token))
            .unwrap_or(Self::Other)
    }

    fn from_token(token: &str) -> Self {
        match token {
            "branch" => Self::Branch,
            "rev-parse" => Self::RevParse,
            "for-each-ref" => Self::ForEachRef,
            "rev-list" => Self::RevList,
            "show-ref" => Self::ShowRef,
            "switch" => Self::Switch,
            "fetch" => Self::Fetch,
            "push" => Self::Push,
            "pull" => Self::Pull,
            "check-ref-format" => Self::CheckRefFormat,
            "worktree" => Self::Worktree,
            "diff-index" => Self::DiffIndex,
            "status" => Self::Status,
            _ => Self::Other,
        }
    }

    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Branch => "branch",
            Self::RevParse => "rev-parse",
            Self::ForEachRef => "for-each-ref",
            Self::RevList => "rev-list",
            Self::ShowRef => "show-ref",
            Self::Switch => "switch",
            Self::Fetch => "fetch",
            Self::Push => "push",
            Self::Pull => "pull",
            Self::CheckRefFormat => "check-ref-format",
            Self::Worktree => "worktree",
            Self::DiffIndex => "diff-index",
            Self::Status => "status",
            Self::Other => "other",
        }
    }
}

/// A point-in-time read of [`GitSpawnStats`], cheap to serialize.
#[derive(Clone, Debug, Default)]
pub struct GitSpawnStatsSnapshot {
    pub total: u64,
    pub timeouts: u64,
    pub failures: u64,
    pub by_subcommand: BTreeMap<GitSubcommand, u64>,
    pub uptime_ms: u64,
}

/// Process-wide-shape but explicitly-owned counters for every `git` spawn
/// routed through [`capture`]. Never a process-global static: held as
/// `Arc<GitSpawnStats>` in `AppState` and threaded as `&GitSpawnStats` into
/// call sites, so tests each get their own isolated instance.
#[derive(Debug)]
pub struct GitSpawnStats {
    total: AtomicU64,
    timeouts: AtomicU64,
    failures: AtomicU64,
    by_subcommand: Mutex<BTreeMap<GitSubcommand, u64>>,
    started_at: Instant,
}

impl Default for GitSpawnStats {
    fn default() -> Self {
        Self {
            total: AtomicU64::new(0),
            timeouts: AtomicU64::new(0),
            failures: AtomicU64::new(0),
            by_subcommand: Mutex::new(BTreeMap::new()),
            started_at: Instant::now(),
        }
    }
}

impl GitSpawnStats {
    pub fn new() -> Self {
        Self::default()
    }

    fn record_spawn(&self, subcommand: GitSubcommand) {
        self.total.fetch_add(1, Ordering::Relaxed);
        let mut by_subcommand = self
            .by_subcommand
            .lock()
            .expect("git spawn stats subcommand map lock poisoned");
        *by_subcommand.entry(subcommand).or_insert(0) += 1;
    }

    fn record_timeout(&self) {
        self.timeouts.fetch_add(1, Ordering::Relaxed);
        self.failures.fetch_add(1, Ordering::Relaxed);
    }

    fn record_failure(&self) {
        self.failures.fetch_add(1, Ordering::Relaxed);
    }

    pub fn snapshot(&self) -> GitSpawnStatsSnapshot {
        let by_subcommand = self
            .by_subcommand
            .lock()
            .expect("git spawn stats subcommand map lock poisoned")
            .clone();
        GitSpawnStatsSnapshot {
            total: self.total.load(Ordering::Relaxed),
            timeouts: self.timeouts.load(Ordering::Relaxed),
            failures: self.failures.load(Ordering::Relaxed),
            by_subcommand,
            uptime_ms: self.started_at.elapsed().as_millis() as u64,
        }
    }
}

const DEFAULT_GIT_TIMEOUT_MS: u64 = 10_000;

/// Upper clamp for a configured budget. `Instant + Duration` panics on
/// overflow and `WS_DASHBOARD_GIT_TIMEOUT_MS` accepts any `u64` millis, so an
/// absurd value must degrade to "effectively never" instead of panicking the
/// calling `spawn_blocking` thread.
const MAX_GIT_TIMEOUT: Duration = Duration::from_secs(24 * 60 * 60);

/// First poll interval of the bounded wait. Deliberately far below the fastest
/// observed poll-path git run (`branch --show-current` ~1.4ms) so the common
/// case returns essentially at child exit.
const INITIAL_POLL_INTERVAL: Duration = Duration::from_micros(250);

/// Ceiling for the poll interval, so a long-running `fetch` costs at most one
/// wake-up per 5ms instead of a busy spin.
const MAX_POLL_INTERVAL: Duration = Duration::from_millis(5);

static GIT_TIMEOUT: OnceLock<Duration> = OnceLock::new();

/// `WS_DASHBOARD_GIT_TIMEOUT_MS`-driven default budget for `capture`, default
/// 10s, with `0` meaning "no timeout / wait indefinitely" (the same reading
/// `WS_DASHBOARD_GIT_PROBE_TTL_MS` gives `0`). Read and parsed once per
/// process, mirroring `discovery.rs`'s `git_probe_ttl_from_env`, which is read
/// once into `GitProbeCache::default`.
pub fn git_timeout_from_env() -> Duration {
    *GIT_TIMEOUT.get_or_init(|| {
        let millis = env::var("WS_DASHBOARD_GIT_TIMEOUT_MS")
            .ok()
            .and_then(|raw| raw.trim().parse::<u64>().ok())
            .unwrap_or(DEFAULT_GIT_TIMEOUT_MS);
        Duration::from_millis(millis)
    })
}

/// Deadline for a bounded wait started at `now`, or `None` for an unbounded
/// wait (`budget` of zero, i.e. `WS_DASHBOARD_GIT_TIMEOUT_MS=0`).
fn deadline_for(now: Instant, budget: Duration) -> Option<Instant> {
    if budget.is_zero() {
        return None;
    }
    Some(
        now.checked_add(budget)
            .or_else(|| now.checked_add(MAX_GIT_TIMEOUT))
            .unwrap_or(now),
    )
}

/// Geometric backoff for the `try_wait` poll loop.
///
/// A fixed quantum here is a latency bug, not a tuning choice: every
/// poll-path git run measures 1.4-6.0ms, so a 10ms floor would add a full
/// quantum to each of `status_for_path`'s 4-5 sequential spawns and turn
/// `/git/status` from ~12-16ms into 40-50ms. The previous `.output()` returned
/// at child exit and added nothing, and this seam is not allowed to change
/// observable git behavior.
fn next_poll_interval(previous: Duration) -> Duration {
    previous.saturating_mul(2).min(MAX_POLL_INTERVAL)
}

/// Result of waiting for one reader thread to deliver its collected bytes.
enum ReaderCollect {
    /// The reader reached EOF and handed over everything it read.
    Collected(Vec<u8>),
    /// The budget expired before EOF, which means some process other than the
    /// direct child still holds a write end of the pipe.
    Expired,
}

/// Bounded receive of one reader thread's output.
///
/// This is why the readers deliver through a channel instead of a
/// `JoinHandle`: `join()` cannot be bounded, and `read_to_end` returns only
/// once *every* write end of the pipe is closed, so a child that exited while
/// a descendant kept the inherited pipes would block an un-bounded join
/// forever - on the success path, where the poll loop's deadline is no longer
/// in play.
///
/// `deadline == None` (`WS_DASHBOARD_GIT_TIMEOUT_MS=0`) blocks: the operator
/// explicitly opted out of bounding. A disconnected channel means the reader
/// thread panicked before sending, treated as empty output to match the
/// previous `join().unwrap_or_default()`.
fn collect_reader(rx: &mpsc::Receiver<Vec<u8>>, deadline: Option<Instant>) -> ReaderCollect {
    let Some(deadline) = deadline else {
        return ReaderCollect::Collected(rx.recv().unwrap_or_default());
    };
    match rx.recv_timeout(deadline.saturating_duration_since(Instant::now())) {
        Ok(bytes) => ReaderCollect::Collected(bytes),
        Err(mpsc::RecvTimeoutError::Disconnected) => ReaderCollect::Collected(Vec::new()),
        Err(mpsc::RecvTimeoutError::Timeout) => ReaderCollect::Expired,
    }
}

/// Spawn `git -C <root> <args>`, bounded by `budget` (a zero `budget` means
/// unbounded, see [`git_timeout_from_env`]).
///
/// Drains stdout/stderr concurrently with the wait (one reader thread per
/// pipe, reading to EOF) so a child that fills a pipe buffer (~64 KB) before
/// exiting can never deadlock the wait loop. On expiry the child is killed and
/// reaped and the reader threads are left *detached and uncollected* (see the
/// comment at that site) before returning `GitFailure::Timeout`, so the call
/// never outlives `budget` by more than one kill-and-reap.
///
/// The bound covers *both* halves uniformly: waiting for the exit and then
/// collecting the readers' output. A child that exits within `budget` but
/// leaves a descendant holding the inherited pipes also yields
/// `GitFailure::Timeout` rather than a possibly-truncated success.
///
/// Every call increments `stats.total` and the per-subcommand counter,
/// regardless of outcome. `stats.timeouts`/`stats.failures` are incremented
/// on `Timeout`; `stats.failures` alone on `Spawn`/`Status`. `Spawn`/`Timeout`
/// are always logged via `tracing::warn!`; `Status` (non-zero exit) is logged
/// only when `expect == GitFailureExpectation::Unexpected`.
pub fn capture(
    stats: &GitSpawnStats,
    root: &Path,
    args: &[&str],
    expect: GitFailureExpectation,
    budget: Duration,
) -> Result<GitOutcome, GitFailure> {
    capture_with_program("git", stats, root, args, expect, budget)
}

/// `capture`'s implementation, parameterized over the program name so unit
/// tests can inject a non-`git` long-running or large-output program without
/// depending on any particular `git` behavior. `capture` always calls this
/// with `"git"`.
fn capture_with_program(
    program: &str,
    stats: &GitSpawnStats,
    root: &Path,
    args: &[&str],
    expect: GitFailureExpectation,
    budget: Duration,
) -> Result<GitOutcome, GitFailure> {
    let subcommand = GitSubcommand::from_args(args);
    stats.record_spawn(subcommand);
    let start = Instant::now();

    let mut command = Command::new(program);
    // Only the real `git` invocation gets `-C <root>`; the test-injected
    // program is not `git` and may not understand that flag. This preserves
    // the exact prior `git -C <root> <args>` shape for production callers.
    if program == "git" {
        command.arg("-C").arg(root);
    }
    command
        .args(args)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());

    let mut child = match command.spawn() {
        Ok(child) => child,
        Err(error) => {
            stats.record_failure();
            tracing::warn!(
                subcommand = subcommand.as_str(),
                error = %error,
                "git spawn failed",
            );
            return Err(GitFailure::Spawn(error));
        }
    };

    let mut stdout_pipe = child.stdout.take().expect("child stdout is piped");
    let mut stderr_pipe = child.stderr.take().expect("child stderr is piped");
    // The readers are detached on every path and deliver their bytes through a
    // channel rather than through `JoinHandle::join`, so that *collecting* the
    // output is bounded by the same deadline as waiting for the exit (see
    // `collect_reader`). Each closure captures only its own owned pipe handle
    // and `Sender`, never `stats`, `child`, or any borrow of this frame, so a
    // reader that outlives this call cannot touch freed state.
    let (stdout_tx, stdout_rx) = mpsc::channel();
    let (stderr_tx, stderr_rx) = mpsc::channel();
    std::thread::spawn(move || {
        let mut buf = Vec::new();
        let _ = stdout_pipe.read_to_end(&mut buf);
        let _ = stdout_tx.send(buf);
    });
    std::thread::spawn(move || {
        let mut buf = Vec::new();
        let _ = stderr_pipe.read_to_end(&mut buf);
        let _ = stderr_tx.send(buf);
    });

    // Measured from `start`, not from here, so the budget the caller gets and
    // the `elapsed_ms` the timeout warning reports agree.
    let deadline = deadline_for(start, budget);
    let mut poll_interval = INITIAL_POLL_INTERVAL;
    let status = loop {
        match child.try_wait() {
            Ok(Some(status)) => break Some(status),
            Ok(None) => {
                let mut sleep_for = poll_interval;
                if let Some(deadline) = deadline {
                    let now = Instant::now();
                    if now >= deadline {
                        break None;
                    }
                    // Never overshoot the budget by up to a poll interval.
                    sleep_for = sleep_for.min(deadline - now);
                }
                std::thread::sleep(sleep_for);
                poll_interval = next_poll_interval(poll_interval);
            }
            Err(error) => {
                let _ = child.kill();
                let _ = child.wait();
                // The readers are left to finish on their own and their output
                // is never collected, for the same reason as the timeout path
                // below: a grandchild may still hold the pipes open.
                stats.record_failure();
                tracing::warn!(
                    subcommand = subcommand.as_str(),
                    error = %error,
                    "git wait failed",
                );
                return Err(GitFailure::Spawn(error));
            }
        }
    };

    let Some(status) = status else {
        // Budget expired. Kill and reap the DIRECT child only: `wait()`
        // returns promptly even while a grandchild is still alive.
        //
        // DELIBERATE THREAD LEAK: the reader threads are left running and
        // their output is never collected on this path. `read_to_end` returns
        // only once EVERY write end of the pipe is closed, and `kill()`
        // signals the direct child alone - so `fetch`/`push`/`pull` (which
        // exec `ssh` and credential helpers with inherited stdout/stderr) and
        // `switch` (post-checkout hooks) would keep a waiting `capture`
        // blocked long past `budget`, making the "bounded wait" unbounded for
        // exactly the network operations the bound exists for. Each timeout
        // therefore leaks two threads plus their two pipe read handles until
        // the pipe finally closes on its own; the cost is bounded by timeout
        // frequency, and the captured output is discarded on this path anyway.
        //
        // Rejected alternative: waiting for the readers with a grace period.
        // Any grace long enough to be useful can still push the total wait
        // past `budget`, which is the defect this avoids.
        let _ = child.kill();
        let _ = child.wait();
        stats.record_timeout();
        let elapsed_ms = start.elapsed().as_millis() as u64;
        tracing::warn!(
            subcommand = subcommand.as_str(),
            elapsed_ms,
            "git command timed out",
        );
        return Err(GitFailure::Timeout);
    };

    // The child exited, but collecting its output is bounded by the SAME
    // deadline: `read_to_end` reaches EOF only when every write end of the pipe
    // is closed, so a descendant that inherited stdout/stderr and outlived the
    // direct child (`ssh` and credential helpers under `fetch`/`push`/`pull`,
    // post-checkout hooks under `switch`) keeps the readers blocked with no
    // deadline left in play. Bounding only the exit wait made "no `capture`
    // call outlives `budget`" false on the most common path.
    let collected = match collect_reader(&stdout_rx, deadline) {
        ReaderCollect::Collected(stdout_bytes) => match collect_reader(&stderr_rx, deadline) {
            ReaderCollect::Collected(stderr_bytes) => Some((stdout_bytes, stderr_bytes)),
            ReaderCollect::Expired => None,
        },
        ReaderCollect::Expired => None,
    };
    let Some((stdout_bytes, stderr_bytes)) = collected else {
        // TRADE-OFF: a git invocation that completed successfully but whose
        // output could not be read within the budget reports `Timeout` rather
        // than returning possibly-truncated stdout. Truncated-but-successful is
        // the more dangerous outcome: callers like `for-each-ref` and
        // `worktree list --porcelain` parse stdout and would treat a short read
        // as a complete answer.
        //
        // `kill()` is a no-op here (the child was already reaped by
        // `try_wait`); it is kept so this path cannot leave a live direct child
        // behind if the exit detection ever changes. The readers are left
        // running and uncollected exactly as on the expiry path above.
        let _ = child.kill();
        stats.record_timeout();
        let elapsed_ms = start.elapsed().as_millis() as u64;
        tracing::warn!(
            subcommand = subcommand.as_str(),
            elapsed_ms,
            // Distinct from the "git command timed out" message above so a
            // descendant holding the pipes is diagnosable apart from a
            // genuinely slow git.
            "git command exited but its output was still held open by a descendant process",
        );
        return Err(GitFailure::Timeout);
    };
    let elapsed = start.elapsed();
    let (stdout, stdout_valid_utf8) = decode_output(&stdout_bytes);
    let (stderr, _) = decode_output(&stderr_bytes);

    if !status.success() {
        stats.record_failure();
        let code = status.code().unwrap_or(-1);
        if expect == GitFailureExpectation::Unexpected {
            tracing::warn!(
                subcommand = subcommand.as_str(),
                code,
                stderr = %truncate(&stderr, 512),
                elapsed_ms = elapsed.as_millis() as u64,
                "git command exited non-zero",
            );
        }
        return Err(GitFailure::Status(code));
    }

    Ok(GitOutcome {
        status,
        stdout,
        stderr,
        elapsed,
        stdout_valid_utf8,
    })
}

fn truncate(value: &str, max: usize) -> String {
    if value.len() <= max {
        return value.to_owned();
    }
    let mut cut = max;
    while !value.is_char_boundary(cut) {
        cut -= 1;
    }
    format!("{}...", &value[..cut])
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::AtomicUsize;
    use std::sync::Arc;

    /// Minimal `tracing::Subscriber` that only counts events, so a test can
    /// pin "no warning was emitted" without pulling in a heavier capture
    /// crate. No tracing-capture test helper exists elsewhere in this crate
    /// (checked before adding this).
    struct EventCounter(Arc<AtomicUsize>);

    impl tracing::Subscriber for EventCounter {
        fn enabled(&self, _metadata: &tracing::Metadata<'_>) -> bool {
            true
        }

        fn new_span(&self, _span: &tracing::span::Attributes<'_>) -> tracing::span::Id {
            tracing::span::Id::from_u64(1)
        }

        fn record(&self, _span: &tracing::span::Id, _values: &tracing::span::Record<'_>) {}

        fn record_follows_from(&self, _span: &tracing::span::Id, _follows: &tracing::span::Id) {}

        fn event(&self, _event: &tracing::Event<'_>) {
            self.0.fetch_add(1, Ordering::SeqCst);
        }

        fn enter(&self, _span: &tracing::span::Id) {}

        fn exit(&self, _span: &tracing::span::Id) {}
    }

    fn count_events(run: impl FnOnce()) -> usize {
        let count = Arc::new(AtomicUsize::new(0));
        let subscriber = EventCounter(Arc::clone(&count));
        tracing::subscriber::with_default(subscriber, run);
        count.load(Ordering::SeqCst)
    }

    /// Shared body of the kill-on-timeout pin. Both platform variants below
    /// call it, so the assertion logic is compiled and checked everywhere and
    /// only the choice of long-running program is platform-specific. This pin
    /// is one of the phase's two load-bearing regression pins for the
    /// drain/kill contract, and Windows is the production platform, so it must
    /// not be unix-only.
    fn assert_capture_kills_on_timeout(program: &str, args: &[&str]) {
        let stats = GitSpawnStats::default();
        let start = Instant::now();
        // Spawned directly (never via a shell) so the killed process has no
        // grandchild that could keep inheriting the stdout/stderr pipes open
        // after the kill. `capture` detaches its readers on the timeout path
        // precisely because a grandchild CAN do that; this pin covers the
        // direct-child case.
        let result = capture_with_program(
            program,
            &stats,
            Path::new("."),
            args,
            GitFailureExpectation::Unexpected,
            Duration::from_millis(200),
        );

        assert!(
            matches!(result, Err(GitFailure::Timeout)),
            "expected Timeout, got {result:?}"
        );
        assert!(
            start.elapsed() < Duration::from_secs(4),
            "capture must kill the child instead of waiting out its full runtime"
        );
        let snapshot = stats.snapshot();
        assert_eq!(snapshot.total, 1);
        assert_eq!(snapshot.timeouts, 1);
        assert_eq!(snapshot.failures, 1);
    }

    /// Shared body of the >1 MB-stdout survival pin. Asserts at least 1 MB
    /// (not an exact byte count) so the same assertions can cover both
    /// platforms' emitters, which may differ by a trailing newline.
    fn assert_capture_survives_large_stdout(program: &str, args: &[&str]) {
        let stats = GitSpawnStats::default();
        let result = capture_with_program(
            program,
            &stats,
            Path::new("."),
            args,
            GitFailureExpectation::Unexpected,
            Duration::from_secs(30),
        );

        let outcome = result.expect("a large-stdout child must succeed, not time out or deadlock");
        assert!(
            outcome.stdout.len() >= 1_000_000,
            "expected >1MB of stdout, got {} bytes",
            outcome.stdout.len()
        );
        let snapshot = stats.snapshot();
        assert_eq!(snapshot.timeouts, 0);
        assert_eq!(snapshot.failures, 0);
    }

    #[cfg(unix)]
    #[test]
    fn capture_kills_and_reports_timeout_for_a_child_that_outlives_its_budget() {
        assert_capture_kills_on_timeout("sleep", &["5"]);
    }

    /// Windows counterpart: `ping` is in `System32`, needs no shell, and
    /// `-n 6` against loopback runs ~5s without reading stdin.
    #[cfg(windows)]
    #[test]
    fn capture_kills_and_reports_timeout_for_a_child_that_outlives_its_budget() {
        assert_capture_kills_on_timeout("ping", &["-n", "6", "127.0.0.1"]);
    }

    /// The realistic timeout case the detach exists for: `fetch`/`push`/`pull`
    /// exec transport and credential helpers that inherit stdout/stderr, so
    /// killing the direct child does not close the pipes. Joining the readers
    /// here would block for the grandchild's full lifetime (5s), blowing the
    /// 200ms budget. Unix-only because provoking a grandchild needs a shell;
    /// the fix it pins is platform-independent.
    #[cfg(unix)]
    #[test]
    fn capture_times_out_within_budget_when_a_grandchild_still_holds_the_pipes() {
        let stats = GitSpawnStats::default();
        let start = Instant::now();
        let result = capture_with_program(
            "sh",
            &stats,
            Path::new("."),
            &["-c", "sleep 5 & sleep 5"],
            GitFailureExpectation::Unexpected,
            Duration::from_millis(200),
        );

        assert!(
            matches!(result, Err(GitFailure::Timeout)),
            "expected Timeout, got {result:?}"
        );
        assert!(
            start.elapsed() < Duration::from_secs(3),
            "capture must not wait for a grandchild to release the pipes; took {:?}",
            start.elapsed()
        );
        assert_eq!(stats.snapshot().timeouts, 1);
    }

    /// Success-path twin of the pin above, and the reason output collection is
    /// bounded rather than joined: the direct child exits ZERO within the
    /// budget, but a descendant keeps the inherited pipes open. Joining the
    /// readers here reported `ok` only after the descendant exited (measured
    /// 3002.9ms under a 200ms budget); the bound must hold on every exit path,
    /// not just on expiry. Unix-only because provoking a descendant needs a
    /// shell; the behavior it pins is platform-independent.
    #[cfg(unix)]
    #[test]
    fn capture_times_out_when_a_zero_exit_child_leaves_a_descendant_holding_the_pipes() {
        let stats = GitSpawnStats::default();
        let start = Instant::now();
        let result = capture_with_program(
            "sh",
            &stats,
            Path::new("."),
            &["-c", "sleep 5 & exit 0"],
            GitFailureExpectation::Unexpected,
            Duration::from_millis(200),
        );

        assert!(
            matches!(result, Err(GitFailure::Timeout)),
            "a zero-exit child whose descendant holds the pipes must still be bounded, got {result:?}"
        );
        assert!(
            start.elapsed() < Duration::from_secs(3),
            "capture must not wait for the descendant to release the pipes; took {:?}",
            start.elapsed()
        );
        let snapshot = stats.snapshot();
        assert_eq!(snapshot.timeouts, 1);
        assert_eq!(snapshot.failures, 1);
    }

    #[cfg(unix)]
    #[test]
    fn capture_survives_a_child_emitting_more_than_1mb_of_stdout() {
        assert_capture_survives_large_stdout("sh", &["-c", "head -c 2000000 /dev/zero"]);
    }

    /// Windows counterpart: `[Console]::Out.Write` bypasses PowerShell's
    /// output formatting (which can wrap or pad), so this emits exactly
    /// 2,000,000 ASCII bytes. `powershell.exe` parses standard MSVCRT argument
    /// quoting, so the single quoted `-Command` argument survives `Command`'s
    /// escaping (unlike `cmd /C`).
    #[cfg(windows)]
    #[test]
    fn capture_survives_a_child_emitting_more_than_1mb_of_stdout() {
        assert_capture_survives_large_stdout(
            "powershell",
            &[
                "-NoProfile",
                "-Command",
                "[Console]::Out.Write('x' * 2000000)",
            ],
        );
    }

    /// A shell-free-as-possible way to get a deterministic exit code 1, per
    /// platform. Neither form contains a space inside a single argument, so
    /// `Command`'s argument escaping is not a factor.
    #[cfg(unix)]
    const EXIT_ONE: (&str, &[&str]) = ("sh", &["-c", "exit 1"]);
    #[cfg(windows)]
    const EXIT_ONE: (&str, &[&str]) = ("cmd", &["/C", "exit", "1"]);

    #[test]
    fn expected_non_zero_exit_increments_failures_without_logging() {
        let stats = GitSpawnStats::default();

        let events = count_events(|| {
            let result = capture_with_program(
                EXIT_ONE.0,
                &stats,
                Path::new("."),
                EXIT_ONE.1,
                GitFailureExpectation::ExpectedNonZero,
                Duration::from_secs(5),
            );
            assert!(matches!(result, Err(GitFailure::Status(1))));
        });

        assert_eq!(
            events, 0,
            "an ExpectedNonZero exit must not emit a tracing event"
        );
        let snapshot = stats.snapshot();
        assert_eq!(snapshot.total, 1);
        assert_eq!(snapshot.timeouts, 0);
        assert_eq!(
            snapshot.failures, 1,
            "ExpectedNonZero must still increment the failure counter"
        );
    }

    #[test]
    fn unexpected_non_zero_exit_increments_failures_and_logs() {
        let stats = GitSpawnStats::default();

        let events = count_events(|| {
            let result = capture_with_program(
                EXIT_ONE.0,
                &stats,
                Path::new("."),
                EXIT_ONE.1,
                GitFailureExpectation::Unexpected,
                Duration::from_secs(5),
            );
            assert!(matches!(result, Err(GitFailure::Status(1))));
        });

        assert!(
            events > 0,
            "an Unexpected non-zero exit must emit a tracing warning"
        );
        assert_eq!(stats.snapshot().failures, 1);
    }

    /// The unbounded branch of `collect_reader`
    /// (`WS_DASHBOARD_GIT_TIMEOUT_MS=0`): with no deadline the output is
    /// collected with a plain blocking receive, so a zero budget must return at
    /// child exit and must never report a timeout. Portable: reuses `EXIT_ONE`.
    #[test]
    fn a_zero_budget_capture_collects_output_at_child_exit_without_timing_out() {
        let stats = GitSpawnStats::default();

        let result = capture_with_program(
            EXIT_ONE.0,
            &stats,
            Path::new("."),
            EXIT_ONE.1,
            GitFailureExpectation::ExpectedNonZero,
            Duration::ZERO,
        );

        assert!(
            matches!(result, Err(GitFailure::Status(1))),
            "an unbounded budget must still observe the real exit status, got {result:?}"
        );
        assert_eq!(
            stats.snapshot().timeouts,
            0,
            "a zero budget opts out of bounding and must never report a timeout"
        );
    }

    /// Portable on purpose: `total` and `by_subcommand` are recorded before
    /// the spawn is attempted, so this pin needs no real program on either
    /// platform.
    #[test]
    fn by_subcommand_is_keyed_by_the_token_after_leading_dash_dash_flags() {
        let stats = GitSpawnStats::default();
        let _ = capture_with_program(
            "ws-dashboard-git-exec-no-such-program",
            &stats,
            Path::new("."),
            &["--no-optional-locks", "diff-index", "-M"],
            GitFailureExpectation::Unexpected,
            Duration::from_secs(5),
        );

        let snapshot = stats.snapshot();
        assert_eq!(
            snapshot.by_subcommand.get(&GitSubcommand::DiffIndex),
            Some(&1),
            "the leading --flag must be skipped when finding the subcommand token"
        );
    }

    #[test]
    fn subcommand_from_args_defaults_to_other_for_unknown_or_flag_only_args() {
        assert_eq!(GitSubcommand::from_args(&["init", "-q"]), GitSubcommand::Other);
        assert_eq!(GitSubcommand::from_args(&["--foo"]), GitSubcommand::Other);
        assert_eq!(GitSubcommand::from_args(&[]), GitSubcommand::Other);
        assert_eq!(
            GitSubcommand::from_args(&["--no-optional-locks", "status"]),
            GitSubcommand::Status
        );
    }

    /// Pins the backoff *schedule*, not wall-clock latency (which would flake).
    /// The first interval must stay well under the ~1.4ms fastest poll-path git
    /// run, or every poll-path spawn pays a sleep it did not previously pay.
    #[test]
    fn poll_interval_doubles_from_a_sub_millisecond_start_and_caps_at_five_ms() {
        let mut schedule = vec![INITIAL_POLL_INTERVAL];
        for _ in 0..6 {
            let previous = *schedule.last().expect("non-empty");
            schedule.push(next_poll_interval(previous));
        }

        assert_eq!(
            schedule,
            vec![
                Duration::from_micros(250),
                Duration::from_micros(500),
                Duration::from_micros(1_000),
                Duration::from_micros(2_000),
                Duration::from_micros(4_000),
                Duration::from_millis(5),
                Duration::from_millis(5),
            ]
        );
        assert!(INITIAL_POLL_INTERVAL < Duration::from_millis(1));
        assert_eq!(next_poll_interval(Duration::MAX), MAX_POLL_INTERVAL);
    }

    #[test]
    fn zero_budget_waits_forever_and_an_absurd_budget_clamps_instead_of_panicking() {
        let now = Instant::now();

        assert_eq!(
            deadline_for(now, Duration::ZERO),
            None,
            "WS_DASHBOARD_GIT_TIMEOUT_MS=0 must mean no timeout, matching WS_DASHBOARD_GIT_PROBE_TTL_MS"
        );
        assert_eq!(
            deadline_for(now, Duration::from_millis(50)),
            now.checked_add(Duration::from_millis(50))
        );

        let clamped = deadline_for(now, Duration::MAX).expect("a non-zero budget has a deadline");
        assert!(clamped > now);
        assert_eq!(
            clamped,
            now.checked_add(MAX_GIT_TIMEOUT)
                .expect("a one-day deadline is representable")
        );
    }

    #[test]
    fn stdout_strict_rejects_invalid_utf8_while_stdout_stays_lossy() {
        let (text, valid) = decode_output(b"refs/heads/main\n");
        assert!(valid);
        assert_eq!(text, "refs/heads/main\n");

        let (lossy, valid) = decode_output(b"/tmp/\xffbroken");
        assert!(!valid, "invalid UTF-8 must be reported, not silently mangled");
        assert!(lossy.contains('\u{fffd}'));

        let outcome = GitOutcome {
            status: ExitStatus::default(),
            stdout: lossy,
            stderr: String::new(),
            elapsed: Duration::ZERO,
            stdout_valid_utf8: valid,
        };
        assert_eq!(
            outcome.stdout_strict(),
            None,
            "path-consuming call sites must see None, as they did before the seam"
        );
    }
}
