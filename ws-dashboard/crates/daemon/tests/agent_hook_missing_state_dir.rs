// CONTRACT (review cycle 1, finding E): `TerminalSession::spawn`
// (`terminal.rs`) used to fall back to `std::env::temp_dir()` when
// `persistent_state::default_state_dir()` resolved to `None`, landing a file
// whose content is an EXECUTED command line
// (`agent-profiles/<terminal_id>/settings.json`) under the predictable,
// world-writable `/tmp/agent-profiles/`. The fix degrades to a hookless spawn
// instead - this proves that degradation actually happens (not merely that
// the code compiles), through the real CLI/HTTP boundary rather than a unit
// test of an isolated helper, mirroring this ticket's existing
// `env!("CARGO_BIN_EXE_ws-dashboard")` real-subprocess pattern
// (`terminal_lifetime.rs`, `tests/server.rs`).
//
// The daemon subprocess here has EVERY state-dir-resolving env var removed
// (`WS_DASHBOARD_STATE_FILE`, `WS_DASHBOARD_STATE_HOME`, `XDG_STATE_HOME`,
// `HOME`, and `LOCALAPPDATA` for completeness on Windows), which -
// `persistent_state::default_state_file`'s documented resolution order - is
// exactly the combination that makes `default_state_dir()` return `None`
// (mirrors `persistent_state.rs`'s own
// `default_state_file_falls_back_to_local_app_data_on_windows` test, which
// removes the same four non-Windows vars to reach that same `None` state
// before layering the Windows-only fallback on top). This is
// subprocess-scoped (`Command::env_remove`), so it never touches this test
// process's own environment.

use std::path::PathBuf;
use std::process::Stdio;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, Command};
use tokio::time::timeout;

static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

fn temp_fixture_path(name: &str) -> PathBuf {
    let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    #[cfg(target_os = "macos")]
    let base = PathBuf::from("/tmp");
    #[cfg(not(target_os = "macos"))]
    let base = std::env::temp_dir();
    base.join(format!(
        "ws-dashboard-agent-hook-missing-state-dir-{name}-{}-{unique}",
        std::process::id()
    ))
}

struct DaemonProcess {
    child: Child,
    base_url: String,
    // CONTRACT: verified empirically while writing this test (not assumed) -
    // `logging::init`'s `tracing_subscriber::fmt::layer()` stderr layer uses
    // that layer's DEFAULT writer, which is `std::io::stdout`, not
    // `std::io::stderr` - so `tracing::info!`/`warn!`/`error!` events (the
    // hookless-degrade warning included) land on the daemon's STDOUT, while
    // the plain `eprintln!` readiness lines (pairing URL, no-auth debug URL,
    // link passphrase) land on STDERR. Both streams are captured here.
    stdout_lines: Arc<Mutex<Vec<String>>>,
}

impl DaemonProcess {
    async fn kill(mut self) -> Vec<String> {
        self.child.kill().await.expect("kill daemon subprocess");
        let _ = self.child.wait().await;
        // Give the drain tasks a brief window to flush any trailing lines
        // (the hook-materialization warning is logged synchronously during
        // `create_terminal`'s handling, well before this point, so this is
        // just cleanup margin, not load-bearing timing).
        tokio::time::sleep(Duration::from_millis(50)).await;
        self.stdout_lines.lock().expect("stdout lines lock").clone()
    }
}

async fn spawn_daemon_without_any_state_dir() -> DaemonProcess {
    let mut child = Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))
        .arg("serve")
        .arg("--no-auth")
        .arg("--port")
        .arg("0")
        .arg("--bind-mode")
        .arg("local")
        .env_remove("WS_DASHBOARD_STATE_FILE")
        .env_remove("WS_DASHBOARD_STATE_HOME")
        .env_remove("XDG_STATE_HOME")
        .env_remove("HOME")
        .env_remove("LOCALAPPDATA")
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true)
        .spawn()
        .expect("spawn real ws-dashboard daemon subprocess with no state-dir env vars");

    let stderr = child.stderr.take().expect("daemon stderr pipe");
    let mut stderr_lines = BufReader::new(stderr).lines();

    let base_url = timeout(Duration::from_secs(10), async {
        loop {
            let line = stderr_lines
                .next_line()
                .await
                .expect("read daemon stderr")
                .expect("daemon exited before printing its no-auth debug URL");
            if let Some(url) = line.strip_prefix("ws-dashboard no-auth debug mode active: ") {
                break url.trim().trim_end_matches('/').to_owned();
            }
        }
    })
    .await
    .expect("daemon subprocess must print its no-auth debug URL before the timeout");

    // Keep draining stderr in the background so its pipe never fills up and
    // blocks the daemon - nothing else in this test reads from it.
    tokio::spawn(async move { while let Ok(Some(_)) = stderr_lines.next_line().await {} });

    // Drain stdout in the background too, buffering every line so this test
    // can later assert on the hookless-degrade warning - unlike the other
    // integration tests' fire-and-forget stdout handling (`Stdio::null()`),
    // this test's whole point is observing a `tracing::warn!` line.
    let stdout = child.stdout.take().expect("daemon stdout pipe");
    let mut stdout_lines = BufReader::new(stdout).lines();
    let stdout_lines_buf = Arc::new(Mutex::new(Vec::new()));
    let collected = stdout_lines_buf.clone();
    tokio::spawn(async move {
        while let Ok(Some(line)) = stdout_lines.next_line().await {
            collected.lock().expect("stdout lines lock").push(line);
        }
    });

    DaemonProcess {
        child,
        base_url,
        stdout_lines: stdout_lines_buf,
    }
}

async fn open_work_root(client: &reqwest::Client, base_url: &str, root: &std::path::Path) -> String {
    let response = client
        .post(format!("{base_url}/api/dashboard/work-roots/open"))
        .json(&serde_json::json!({ "path": root.display().to_string() }))
        .send()
        .await
        .expect("open work root request");
    assert_eq!(response.status(), reqwest::StatusCode::OK, "open work root");
    response
        .headers()
        .get("x-ws-dashboard-opened-work-root-id")
        .and_then(|value| value.to_str().ok())
        .expect("opened work root id header")
        .to_owned()
}

#[tokio::test]
async fn terminal_spawn_degrades_to_hookless_when_no_state_dir_resolves() {
    let daemon = spawn_daemon_without_any_state_dir().await;
    let client = reqwest::Client::new();

    let work_root = temp_fixture_path("root");
    std::fs::create_dir_all(&work_root).expect("create work root dir");
    let work_root_id = open_work_root(&client, &daemon.base_url, &work_root).await;

    // profileId "claude" is the ONLY registered profile with a non-`None`
    // `hook_config` (`agent_profile_registry.rs`) - this is what makes the
    // hook-materialization branch in `TerminalSession::spawn` run at all. The
    // "claude" binary itself does not need to exist: the daemon's registry
    // entry write and IPC handshake with its own detached helper subprocess
    // (always our own binary) complete BEFORE the helper ever attempts to
    // spawn the wrapped "claude" program
    // (`terminal_helper_process.rs::run_terminal_helper`'s own ordering
    // CONTRACT), so `create_terminal`'s HTTP response does not depend on
    // "claude" being resolvable on PATH.
    let response = client
        .post(format!(
            "{}/api/dashboard/work-roots/{work_root_id}/terminals",
            daemon.base_url
        ))
        .json(&serde_json::json!({
            "columns": 80,
            "rows": 24,
            "title": "hookless-degrade test",
            "profileId": "claude",
        }))
        .send()
        .await
        .expect("create terminal request");
    assert_eq!(response.status(), reqwest::StatusCode::OK, "create terminal");
    let created: serde_json::Value = response.json().await.expect("create terminal JSON");
    let terminal_id = created["terminalId"]
        .as_str()
        .expect("terminal id")
        .to_owned();

    let stdout_lines = daemon.kill().await;

    // Evidence 1: the fallback this fix removed used to land
    // `<temp_dir>/agent-profiles/<terminal_id>/settings.json` - proving that
    // exact path does NOT exist for THIS terminal id is airtight regardless
    // of unrelated garbage from other test runs sharing the same system temp
    // directory, since the id is a fresh 18-char random suffix per spawn.
    let fallback_profile_dir = std::env::temp_dir()
        .join("agent-profiles")
        .join(&terminal_id);
    assert!(
        !fallback_profile_dir.exists(),
        "hook materialization must not fall back to the world-writable temp \
         path when no state dir resolves; found {fallback_profile_dir:?}"
    );

    // Evidence 2: the degrade path is not merely "did nothing observable" -
    // it logs a warning naming the terminal id, proving the branch actually
    // ran rather than being skipped for an unrelated reason.
    let joined_stdout = stdout_lines.join("\n");
    assert!(
        joined_stdout.contains("no persistent state directory resolved")
            && joined_stdout.contains(&terminal_id),
        "expected a hookless-degrade warning naming terminal id {terminal_id}, got stdout:\n{joined_stdout}"
    );

    let _ = std::fs::remove_dir_all(&work_root);
}
