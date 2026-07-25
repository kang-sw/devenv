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

use crate::agent_callback;
use crate::cli::TerminalNotifyArgs;

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
    let client = reqwest::Client::new();
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

fn log_path() -> Option<PathBuf> {
    Some(
        crate::persistent_state::default_state_dir()?
            .join("logs")
            .join("terminal-notify.log"),
    )
}

// CONTRACT: best-effort, fire-and-forget. A logging failure (unwritable
// disk, missing state dir) must never itself produce stdout/stderr output or
// change the process's exit code - see the module CONTRACT above.
fn log_failure(args: &TerminalNotifyArgs, error: &str) {
    let Some(path) = log_path() else { return };
    let Some(parent) = path.parent() else { return };
    if std::fs::create_dir_all(parent).is_err() {
        return;
    }
    let line = format!(
        "{} terminal-notify state={} callback={} error={error}\n",
        now_ms(),
        args.state.as_str(),
        args.callback.display(),
    );
    if let Ok(mut file) = std::fs::OpenOptions::new().create(true).append(true).open(&path) {
        let _ = file.write_all(line.as_bytes());
    }
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

        let log_contents = std::fs::read_to_string(state_home.join("logs").join("terminal-notify.log"))
            .expect("terminal-notify.log must exist after log_failure");
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

        let log_contents = std::fs::read_to_string(state_home.join("logs").join("terminal-notify.log"))
            .expect("terminal-notify.log must exist");
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
