// The `ws-dashboard terminal-notify` hidden subcommand's implementation
// (260725 Phase 3 step 3): resolves the `--callback` file via
// `agent_callback::resolve_callback_target` and POSTs the turn state to the
// daemon's callback route.
//
// CONTRACT (deliberate design decision, empirically verified - see the
// phase's implementation report for the full method and raw evidence): this
// command is invoked in two very different contexts - a developer running
// it BY HAND to debug the pipeline, and a vendor CLI's hook runner firing it
// on EVERY turn boundary inside a user's live agent session. There is no
// reliable signal (argv shape, env, or otherwise) that distinguishes the two
// invocations, so ONE behavior must serve both.
//
// A real-PTY measurement (drive the compiled binary as a Claude hook,
// pointed at a deliberately-missing `callback.json` - the exact
// Phase-3-only-vs-Phase-4 gap) proved that a non-zero exit WITH stderr
// output makes Claude Code surface a visible `<Event> hook error` /
// `Failed with non-blocking status code: <stderr text>` line in the
// interactive transcript, PLUS a persistent "Stop hook error occurred"
// status-line indicator - on every UserPromptSubmit and every Stop, for as
// long as the callback file is absent. That is unacceptable per-turn noise
// the user cannot act on, so this module does NOT propagate a non-zero exit
// or print anything to stdout/stderr on failure. Every failure is instead
// appended to a dedicated log file under the daemon's state dir
// (`logs/terminal-notify.log`, sibling to the daemon's own `logs/`
// directory) - loud in a place a developer or the daemon operator can find,
// quiet in the user's live terminal - and the process still exits 0. A
// developer debugging by hand loses an immediate stderr message but keeps
// two ways to notice failure: the log file, and (for a genuine pipeline
// break) the visible absence of the turn-state effect they expected.
use std::io::Write as _;
use std::path::PathBuf;
use std::time::Duration;

use crate::agent_callback;
use crate::cli::TerminalNotifyArgs;

// FIX (review cycle 1, finding B): a bare `reqwest::Client::new()` has no
// connect or request timeout, so `deliver` would await `send()` forever. That
// is not hypothetical for this design: the whole point of the ephemeral-port
// mechanism is that the daemon's port changes across restarts, so a stale
// `callback.json` can point at a port a DIFFERENT process has since bound -
// the connect succeeds and then never responds. Left unbounded, this hook
// fire would stall the vendor CLI's turn loop until ITS OWN hook timeout
// fires, which is exactly the per-turn UX cost the module CONTRACT above
// exists to avoid. Values chosen: `CONNECT_TIMEOUT` is generous for a
// same-host TCP connect (normally sub-millisecond, even over `--bind-mode
// public` on a slow network) while still being far short of "noticeable
// mid-turn"; `REQUEST_TIMEOUT` bounds the whole request (connect + send +
// response) so a target that accepts the connection and then hangs cannot
// block indefinitely either.
const CONNECT_TIMEOUT: Duration = Duration::from_millis(750);
const REQUEST_TIMEOUT: Duration = Duration::from_secs(2);

pub async fn run_terminal_notify(args: TerminalNotifyArgs) -> anyhow::Result<()> {
    if let Err(error) = deliver(&args).await {
        log_failure(&args, &error);
    }
    Ok(())
}

async fn deliver(args: &TerminalNotifyArgs) -> Result<(), String> {
    let target = agent_callback::resolve_callback_target(&args.callback)
        .map_err(|error| format!("resolving callback target: {error}"))?;

    let (Some(terminal_id), Some(token)) = (target.terminal_id, target.token) else {
        return Err(format!(
            "callback file at {} has no terminalId/token - Phase 4 has not populated this \
             callback target yet",
            args.callback.display()
        ));
    };

    let base_url = target.base_url.trim_end_matches('/');
    let url = format!("{base_url}/api/dashboard/terminals/{terminal_id}/turn-state");
    let client = reqwest::Client::builder()
        .connect_timeout(CONNECT_TIMEOUT)
        .timeout(REQUEST_TIMEOUT)
        .build()
        .map_err(|error| format!("building HTTP client: {error}"))?;
    let response = client
        .post(&url)
        .json(&serde_json::json!({ "token": token, "state": args.state.as_str() }))
        .send()
        .await
        .map_err(|error| format!("POST to {url} failed: {error}"))?;

    if !response.status().is_success() {
        return Err(format!("POST to {url} returned HTTP {}", response.status()));
    }
    Ok(())
}

// CONTRACT (review cycle 1, finding C): rotated files land at
// "<prefix>.<date>" (mirrors `logging.rs`'s own comment on `LOG_FILE_PREFIX`),
// e.g. "terminal-notify.log.2026-07-26".
const LOG_FILE_PREFIX: &str = "terminal-notify.log";

fn log_dir() -> Option<PathBuf> {
    Some(crate::persistent_state::default_state_dir()?.join("logs"))
}

// CONTRACT: best-effort, fire-and-forget. A logging failure (unwritable
// disk, missing state dir) must never itself produce stdout/stderr output or
// change the process's exit code - see the module CONTRACT above.
//
// FIX (review cycle 1, finding C, correctness + fit): this used to be a
// bespoke `OpenOptions::new().create(true).append(true)` writer with no size
// cap and no rotation, living in the SAME `logs/` directory the daemon's own
// appender rotates and prunes (`logging.rs`, `Rotation::DAILY` +
// `MAX_LOG_FILES`) - but under a filename that pruner's prefix match never
// reclaims. During the Phase-3-only window every hook fire fails (no
// `callback.json` exists until Phase 4), so the old file grew unbounded for
// as long as any agent terminal ran. Reusing `logging::build_file_appender`
// gives this file the SAME bounded rotation policy instead of a second,
// uncapped one.
fn log_failure(args: &TerminalNotifyArgs, error: &str) {
    let Some(dir) = log_dir() else { return };
    // CONTRACT: `RollingFileAppender::builder().build(dir)` prunes old
    // rotated files as part of `build()` by `fs::read_dir`-ing `dir` BEFORE
    // it ever creates that directory itself - if `dir` does not already
    // exist (e.g. this is the very first hook fire against a fresh state
    // dir, before the daemon's own file-log sink has created `logs/`),
    // `tracing_appender` prints "Error reading the log directory/files: ..."
    // directly to this process's real stderr from inside the library,
    // independent of any `tracing` subscriber - which would silently break
    // this module's load-bearing stdio-silence CONTRACT (discovered by
    // running this fix's own verification test, not by static reading).
    // Pre-creating the directory ourselves (mirrors the ORIGINAL bespoke
    // writer's own `create_dir_all` call, which this fix must not drop)
    // avoids that internal diagnostic on the common path.
    if std::fs::create_dir_all(&dir).is_err() {
        return;
    }
    let Ok(mut appender) = crate::logging::build_file_appender(&dir, LOG_FILE_PREFIX) else {
        return;
    };
    let line = format!(
        "{} terminal-notify state={} callback={} error={error}\n",
        now_ms(),
        args.state.as_str(),
        args.callback.display(),
    );
    let _ = appender.write_all(line.as_bytes());
}

fn now_ms() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|duration| duration.as_millis() as u64)
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::cli::TurnStateArg;
    use std::sync::atomic::{AtomicU64, Ordering};

    static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

    fn temp_state_home(label: &str) -> PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        std::env::temp_dir().join(format!(
            "ws-dashboard-terminal-notify-log-{label}-{}-{unique}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time after epoch")
                .as_nanos()
        ))
    }

    // The rolling appender names files "<prefix>.<date>" (e.g.
    // "terminal-notify.log.2026-07-26"), not the bare prefix, so tests read
    // back whichever single file under `logs/` starts with the prefix rather
    // than asserting an exact dateless filename (mirrors `logging.rs`'s own
    // test pattern for the same rotation scheme).
    fn read_rotated_log(state_home: &std::path::Path) -> String {
        let log_dir = state_home.join("logs");
        let entries: Vec<_> = std::fs::read_dir(&log_dir)
            .expect("read logs dir")
            .filter_map(|entry| entry.ok())
            .filter(|entry| {
                entry
                    .file_name()
                    .to_string_lossy()
                    .starts_with(LOG_FILE_PREFIX)
            })
            .collect();
        assert_eq!(
            entries.len(),
            1,
            "expected exactly one {LOG_FILE_PREFIX}.<date> file in {log_dir:?}, found {entries:?}"
        );
        std::fs::read_to_string(entries[0].path()).expect("read rotated terminal-notify log")
    }

    // CONTRACT: mutates the process-global `WS_DASHBOARD_STATE_HOME` env var
    // that `persistent_state::default_state_dir` reads, so - per that
    // module's own documented contract - this test holds `ENV_LOCK` for its
    // whole body to avoid racing any sibling test in this crate that reads
    // or mutates the same var.
    #[test]
    fn log_failure_appends_a_line_naming_the_state_callback_path_and_error() {
        let _env_lock = crate::persistent_state::ENV_LOCK
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        let saved = std::env::var_os("WS_DASHBOARD_STATE_HOME");

        let state_home = temp_state_home("append");
        std::env::set_var("WS_DASHBOARD_STATE_HOME", &state_home);

        let args = TerminalNotifyArgs {
            callback: PathBuf::from("/tmp/does-not-matter/callback.json"),
            state: TurnStateArg::Ready,
        };
        log_failure(&args, "resolving callback target: callback file not found");

        let log_contents = read_rotated_log(&state_home);
        assert!(log_contents.contains("state=ready"));
        assert!(log_contents.contains("callback=/tmp/does-not-matter/callback.json"));
        assert!(log_contents.contains("callback file not found"));

        match saved {
            Some(value) => std::env::set_var("WS_DASHBOARD_STATE_HOME", value),
            None => std::env::remove_var("WS_DASHBOARD_STATE_HOME"),
        }
        let _ = std::fs::remove_dir_all(&state_home);
    }

    #[test]
    fn log_failure_appends_multiple_lines_across_calls() {
        let _env_lock = crate::persistent_state::ENV_LOCK
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        let saved = std::env::var_os("WS_DASHBOARD_STATE_HOME");

        let state_home = temp_state_home("multi");
        std::env::set_var("WS_DASHBOARD_STATE_HOME", &state_home);

        let args = TerminalNotifyArgs {
            callback: PathBuf::from("/tmp/callback.json"),
            state: TurnStateArg::Working,
        };
        log_failure(&args, "first failure");
        log_failure(&args, "second failure");

        let log_contents = read_rotated_log(&state_home);
        assert_eq!(log_contents.lines().count(), 2, "each call must append, not overwrite");
        assert!(log_contents.contains("first failure"));
        assert!(log_contents.contains("second failure"));

        match saved {
            Some(value) => std::env::set_var("WS_DASHBOARD_STATE_HOME", value),
            None => std::env::remove_var("WS_DASHBOARD_STATE_HOME"),
        }
        let _ = std::fs::remove_dir_all(&state_home);
    }
}
