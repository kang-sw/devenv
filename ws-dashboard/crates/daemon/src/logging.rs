use std::path::{Path, PathBuf};

use tracing_appender::non_blocking::WorkerGuard;
use tracing_appender::rolling::{RollingFileAppender, Rotation};
use tracing_subscriber::prelude::*;
use tracing_subscriber::EnvFilter;

use crate::persistent_state;

// CONTRACT: Rotated files land at "<prefix>.<date>", e.g. "daemon.log.2026-07-16" —
// see tracing-appender's `filename_prefix` semantics.
const LOG_DIR_NAME: &str = "logs";
const LOG_FILE_PREFIX: &str = "daemon.log";
const MAX_LOG_FILES: usize = 14;

/// Installs the daemon's tracing subscriber: an always-on stderr `fmt` layer
/// plus a default-on rolling-file `fmt` layer, composed under one
/// `tracing_subscriber::registry()`.
///
/// CONTRACT: Daemon startup installs structured logging suitable for server
/// and request lifecycle diagnostics without exposing auth secrets. The
/// stderr layer must keep working unchanged (`dev.sh run` foreground output)
/// even when the file sink cannot be opened — file-sink failures are
/// fail-soft (warn on stderr, continue stderr-only) and must never panic or
/// abort startup.
///
/// `log_file_override` mirrors the `serve` subcommand's `--log-file <path>`
/// flag; when `None`, the file sink resolves to the daemon's persistent
/// state directory (`<state_dir>/logs/daemon.log`).
///
/// Returns the file sink's `WorkerGuard` when the file layer is active. The
/// caller MUST hold this guard for the process lifetime — dropping it early
/// silently stops the non-blocking writer's flush and can drop buffered
/// lines. Returns `Ok(None)` when logging is stderr-only (fail-soft path).
pub fn init(
    filter: &str,
    log_file_override: Option<PathBuf>,
) -> anyhow::Result<Option<WorkerGuard>> {
    let stderr_filter = EnvFilter::try_new(filter)?;
    let stderr_layer = tracing_subscriber::fmt::layer().with_filter(stderr_filter);

    let file_sink = resolve_log_target(log_file_override).and_then(|(dir, prefix)| {
        match build_file_appender(&dir, &prefix) {
            Ok(appender) => {
                let file_filter = match EnvFilter::try_new(filter) {
                    Ok(file_filter) => file_filter,
                    Err(err) => {
                        eprintln!(
                            "ws-dashboard: failed to build file log filter, continuing stderr-only: {err}"
                        );
                        return None;
                    }
                };
                let (non_blocking, guard) = tracing_appender::non_blocking(appender);
                let file_layer = tracing_subscriber::fmt::layer()
                    .with_ansi(false)
                    .with_writer(non_blocking)
                    .with_filter(file_filter);
                Some((file_layer, guard))
            }
            Err(err) => {
                eprintln!(
                    "ws-dashboard: failed to open log file under {}, continuing stderr-only: {err}",
                    dir.display()
                );
                None
            }
        }
    });

    // NOTE: `try_init()`'s result is intentionally ignored (matching the
    // prior single-sink `init`), since a global subscriber can only be
    // installed once per process; repeat calls (e.g. across unit tests in
    // the same test binary) must not turn into hard errors here.
    match file_sink {
        Some((file_layer, guard)) => {
            let _ = tracing_subscriber::registry()
                .with(stderr_layer)
                .with(file_layer)
                .try_init();
            Ok(Some(guard))
        }
        None => {
            let _ = tracing_subscriber::registry().with(stderr_layer).try_init();
            Ok(None)
        }
    }
}

/// Resolves the (directory, filename_prefix) pair the rolling file appender
/// should target, given an optional `--log-file` override. Returns `None`
/// when no target can be resolved at all (override absent and no persistent
/// state directory available) — the fail-soft path skips the file layer
/// entirely in that case.
fn resolve_log_target(log_file_override: Option<PathBuf>) -> Option<(PathBuf, String)> {
    match log_file_override {
        Some(path) => {
            let dir = path
                .parent()
                .filter(|parent| !parent.as_os_str().is_empty())
                .map(Path::to_path_buf)
                .unwrap_or_else(|| PathBuf::from("."));
            let prefix = path
                .file_name()
                .map(|name| name.to_string_lossy().into_owned())
                .unwrap_or_else(|| LOG_FILE_PREFIX.to_string());
            Some((dir, prefix))
        }
        None => persistent_state::default_state_dir()
            .map(|state_dir| (state_dir.join(LOG_DIR_NAME), LOG_FILE_PREFIX.to_string())),
    }
}

fn build_file_appender(
    dir: &Path,
    filename_prefix: &str,
) -> Result<RollingFileAppender, tracing_appender::rolling::InitError> {
    RollingFileAppender::builder()
        .rotation(Rotation::DAILY)
        .filename_prefix(filename_prefix)
        .max_log_files(MAX_LOG_FILES)
        .build(dir)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU64, Ordering};
    use std::time::{SystemTime, UNIX_EPOCH};

    static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

    fn temp_path(label: &str) -> PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        let nanos = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system time after epoch")
            .as_nanos();
        std::env::temp_dir().join(format!("ws-dashboard-logging-{label}-{nanos}-{unique}"))
    }

    #[test]
    fn init_falls_back_to_stderr_only_when_log_dir_cannot_be_created() {
        // A regular file blocks any path nested under it from ever becoming
        // a directory: `fs::create_dir_all` on such a path fails with
        // "not a directory", exercising the fail-soft branch.
        let blocking_file = temp_path("blocker");
        std::fs::write(&blocking_file, b"not a directory").expect("create blocking file");

        let unreachable_log_file = blocking_file.join("subdir").join("daemon.log");

        let result = init("info", Some(unreachable_log_file));

        let _ = std::fs::remove_file(&blocking_file);

        let guard = result.expect("init must not error on file-sink failure");
        assert!(
            guard.is_none(),
            "fail-soft path must report no active file-sink guard"
        );
    }
}
