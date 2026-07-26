//! Single seam through which the daemon spawns `git`.
//!
//! Every git invocation in this crate should route through [`capture`] (or a
//! thin per-module wrapper over it) rather than calling `std::process::Command`
//! directly, so that:
//!
//! - every spawn is subject to a bounded wait with a hard kill on expiry
//!   (`WS_DASHBOARD_GIT_TIMEOUT_MS`, default 10s) instead of the previous
//!   unbounded `.output()` call,
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
use std::sync::Mutex;
use std::time::{Duration, Instant};

/// Result of a successful (zero-exit, non-timed-out) `capture` call.
#[derive(Clone, Debug)]
pub struct GitOutcome {
    pub status: ExitStatus,
    pub stdout: String,
    pub stderr: String,
    pub elapsed: Duration,
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

/// `WS_DASHBOARD_GIT_TIMEOUT_MS`-driven default budget for `capture`, default
/// 10s. Mirrors `discovery.rs`'s `git_probe_ttl_from_env` shape.
pub fn git_timeout_from_env() -> Duration {
    let millis = env::var("WS_DASHBOARD_GIT_TIMEOUT_MS")
        .ok()
        .and_then(|raw| raw.trim().parse::<u64>().ok())
        .unwrap_or(DEFAULT_GIT_TIMEOUT_MS);
    Duration::from_millis(millis)
}

/// Spawn `git -C <root> <args>`, bounded by `budget`.
///
/// Drains stdout/stderr concurrently with the wait (one reader thread per
/// pipe, reading to EOF) so a child that fills a pipe buffer (~64 KB) before
/// exiting can never deadlock the wait loop. On expiry the child is killed
/// and the reader threads are joined before returning `GitFailure::Timeout`.
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
    let stdout_reader = std::thread::spawn(move || {
        let mut buf = Vec::new();
        let _ = stdout_pipe.read_to_end(&mut buf);
        buf
    });
    let stderr_reader = std::thread::spawn(move || {
        let mut buf = Vec::new();
        let _ = stderr_pipe.read_to_end(&mut buf);
        buf
    });

    let deadline = Instant::now() + budget;
    let status = loop {
        match child.try_wait() {
            Ok(Some(status)) => break Some(status),
            Ok(None) => {
                if Instant::now() >= deadline {
                    break None;
                }
                std::thread::sleep(Duration::from_millis(10));
            }
            Err(error) => {
                let _ = child.kill();
                let _ = child.wait();
                let _ = stdout_reader.join();
                let _ = stderr_reader.join();
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
        // Budget expired: kill, reap, and join the readers so the pipes'
        // read ends (and the threads blocked on them) are cleaned up before
        // returning.
        let _ = child.kill();
        let _ = child.wait();
        let _ = stdout_reader.join();
        let _ = stderr_reader.join();
        stats.record_timeout();
        let elapsed_ms = start.elapsed().as_millis() as u64;
        tracing::warn!(
            subcommand = subcommand.as_str(),
            elapsed_ms,
            "git command timed out",
        );
        return Err(GitFailure::Timeout);
    };

    let stdout_bytes = stdout_reader.join().unwrap_or_default();
    let stderr_bytes = stderr_reader.join().unwrap_or_default();
    let elapsed = start.elapsed();
    let stdout = String::from_utf8_lossy(&stdout_bytes).into_owned();
    let stderr = String::from_utf8_lossy(&stderr_bytes).into_owned();

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

    #[test]
    fn capture_kills_and_reports_timeout_for_a_child_that_outlives_its_budget() {
        let stats = GitSpawnStats::default();
        let start = Instant::now();
        // Spawned directly (not via `sh -c`) so the killed process has no
        // grandchild that could keep inheriting the stdout/stderr pipes open
        // after the kill.
        let result = capture_with_program(
            "sleep",
            &stats,
            Path::new("."),
            &["5"],
            GitFailureExpectation::Unexpected,
            Duration::from_millis(200),
        );

        assert!(
            matches!(result, Err(GitFailure::Timeout)),
            "expected Timeout, got {result:?}"
        );
        assert!(
            start.elapsed() < Duration::from_secs(4),
            "capture must kill the child instead of waiting out the full sleep"
        );
        let snapshot = stats.snapshot();
        assert_eq!(snapshot.total, 1);
        assert_eq!(snapshot.timeouts, 1);
        assert_eq!(snapshot.failures, 1);
    }

    #[test]
    fn capture_survives_a_child_emitting_more_than_1mb_of_stdout() {
        let stats = GitSpawnStats::default();
        let result = capture_with_program(
            "sh",
            &stats,
            Path::new("."),
            &["-c", "head -c 2000000 /dev/zero"],
            GitFailureExpectation::Unexpected,
            Duration::from_secs(10),
        );

        let outcome = result.expect("a large-stdout child must succeed, not time out or deadlock");
        assert_eq!(outcome.stdout.len(), 2_000_000);
        let snapshot = stats.snapshot();
        assert_eq!(snapshot.timeouts, 0);
        assert_eq!(snapshot.failures, 0);
    }

    #[test]
    fn expected_non_zero_exit_increments_failures_without_logging() {
        let stats = GitSpawnStats::default();

        let events = count_events(|| {
            let result = capture_with_program(
                "sh",
                &stats,
                Path::new("."),
                &["-c", "exit 1"],
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
                "sh",
                &stats,
                Path::new("."),
                &["-c", "exit 1"],
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

    #[test]
    fn by_subcommand_is_keyed_by_the_token_after_leading_dash_dash_flags() {
        let stats = GitSpawnStats::default();
        let _ = capture_with_program(
            "sh",
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
}
