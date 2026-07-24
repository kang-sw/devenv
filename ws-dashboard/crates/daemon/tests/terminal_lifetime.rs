// CONTRACT: ticket 260723 Phase 1 binding completion bar item #1 - a live
// terminal must survive a simulated daemon restart and reattach via the
// existing frontend resume-by-id path. This is proven here with two REAL OS
// processes (not in-process `tower::ServiceExt::oneshot` harnesses like
// `routes.rs` uses elsewhere): a first `ws-dashboard serve` daemon process
// that creates a live terminal, a hard `SIGKILL` of that daemon process
// (never a graceful `ctrl_c`/`GracefulShutdown`), and a second independent
// `ws-dashboard serve` daemon process pointed at the same
// `WS_DASHBOARD_STATE_HOME` that must boot-reconcile-adopt the still-running
// detached helper and let the browser-facing HTTP/WebSocket surface resume
// exactly where it left off.

use std::path::PathBuf;
use std::process::Stdio;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

use futures_util::{SinkExt, StreamExt};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, Command};
use tokio::time::timeout;
use tokio_tungstenite::tungstenite::{client::IntoClientRequest, Message as TungsteniteMessage};

const STARTUP_TIMEOUT: Duration = Duration::from_secs(10);
const POLL_TIMEOUT: Duration = Duration::from_secs(8);

static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

fn temp_fixture_path(name: &str) -> PathBuf {
    let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    std::env::temp_dir().join(format!(
        "ws-dashboard-terminal-lifetime-{name}-{}-{unique}",
        std::process::id()
    ))
}

fn echo_marker_command(marker: &str) -> String {
    // CONTRACT: keep this a plain, no-exit echo (unlike
    // `routes.rs::terminal_test_commands`) - this test needs the shell to
    // stay alive across the simulated daemon restart, not exit.
    if cfg!(windows) {
        format!("echo {marker}\r\n")
    } else {
        format!("printf '%s\\n' '{marker}'\n")
    }
}

// CONTRACT: unlike `echo_marker_command`, this schedules the shell to print
// the marker immediately and THEN exit itself after a short delay,
// independent of whether any daemon is attached when the delay elapses.
// This is what lets a test hard-kill the daemon connection *before* the
// shell's exit fires, so the exit genuinely happens during the down window
// with zero IPC connections open - the real production trigger for boot-
// reconcile row 2 (AdoptGrace): a daemon crash/restart while the shell is
// still alive, followed by the shell exiting on its own sometime before the
// next daemon reconnects (as opposed to exiting while a daemon connection is
// still live and witnessing it in real time).
fn delayed_exit_marker_command(marker: &str) -> String {
    if cfg!(windows) {
        format!("echo {marker}\r\nping -n 2 127.0.0.1 >NUL\r\nexit\r\n")
    } else {
        format!("printf '%s\\n' '{marker}'\nsleep 1\nexit\n")
    }
}

// CONTRACT: generous on purpose - this only needs to exceed the shell's own
// scheduled delay (see `delayed_exit_marker_command`'s `sleep 1`) by enough
// margin to stay robust under CPU contention from other tests/daemons
// running concurrently in the same `cargo test` invocation (this test spawns
// four real OS processes across two daemons). A too-short margin risks a
// false failure (daemon #2 observes the shell still `running`), not a false
// pass, so err generous rather than tune this tight.
const DELAYED_EXIT_MARGIN: Duration = Duration::from_secs(4);

struct DaemonProcess {
    child: Child,
    base_url: String,
}

impl DaemonProcess {
    async fn kill_hard(mut self) {
        // CONTRACT: `Child::kill` sends SIGKILL directly - the daemon gets no
        // chance to run its `ctrl_c`-driven graceful shutdown path or any
        // Drop glue. This is the point: only a genuinely detached,
        // independently-surviving helper process passes this test.
        self.child.kill().await.expect("SIGKILL daemon process");
        let _ = self.child.wait().await;
    }
}

async fn spawn_real_daemon(state_home: &std::path::Path) -> DaemonProcess {
    let mut child = Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))
        .arg("serve")
        .arg("--no-auth")
        .arg("--port")
        .arg("0")
        .arg("--bind-mode")
        .arg("local")
        .env("WS_DASHBOARD_STATE_HOME", state_home)
        .env_remove("WS_DASHBOARD_STATE_FILE")
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::piped())
        .kill_on_drop(true)
        .spawn()
        .expect("spawn real ws-dashboard daemon process");

    let stderr = child.stderr.take().expect("daemon stderr pipe");
    let mut lines = BufReader::new(stderr).lines();
    let base_url = timeout(STARTUP_TIMEOUT, async {
        loop {
            let line = lines
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
    .expect("daemon did not print its startup URL in time");

    // Keep draining stderr in the background so the pipe never fills up and
    // blocks the daemon; nothing else in this test reads from it.
    tokio::spawn(async move { while let Ok(Some(_)) = lines.next_line().await {} });

    DaemonProcess { child, base_url }
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

async fn create_terminal(client: &reqwest::Client, base_url: &str, work_root_id: &str) -> String {
    let response = client
        .post(format!(
            "{base_url}/api/dashboard/work-roots/{work_root_id}/terminals"
        ))
        .json(&serde_json::json!({ "columns": 80, "rows": 24, "title": "lifetime E2E terminal" }))
        .send()
        .await
        .expect("create terminal request");
    assert_eq!(response.status(), reqwest::StatusCode::OK, "create terminal");
    let created: serde_json::Value = response.json().await.expect("create terminal JSON");
    assert_eq!(created["status"], "running");
    created["terminalId"]
        .as_str()
        .expect("terminal id")
        .to_owned()
}

async fn send_input(client: &reqwest::Client, base_url: &str, terminal_id: &str, data: &str) {
    let response = client
        .post(format!("{base_url}/api/dashboard/terminals/{terminal_id}/input"))
        .json(&serde_json::json!({ "data": data }))
        .send()
        .await
        .expect("terminal input request");
    assert_eq!(
        response.status(),
        reqwest::StatusCode::NO_CONTENT,
        "terminal input"
    );
}

/// Polls `.../output?after=<after>` until `needle` shows up in the
/// concatenated chunk text, returning the full text seen and the response's
/// `nextSequence` cursor (for the caller's next `after`/WS reattach cursor).
async fn poll_output_until_contains(
    client: &reqwest::Client,
    base_url: &str,
    terminal_id: &str,
    after: u64,
    needle: &str,
) -> (String, u64) {
    let deadline = tokio::time::Instant::now() + POLL_TIMEOUT;
    loop {
        let response = client
            .get(format!(
                "{base_url}/api/dashboard/terminals/{terminal_id}/output?after={after}"
            ))
            .send()
            .await
            .expect("poll output request");
        assert_eq!(response.status(), reqwest::StatusCode::OK, "poll output");
        let value: serde_json::Value = response.json().await.expect("poll output JSON");
        let chunks = value["chunks"].as_array().expect("chunks array");
        let text: String = chunks
            .iter()
            .filter_map(|chunk| chunk["data"].as_str())
            .collect();
        if text.contains(needle) {
            let next_sequence = value["nextSequence"].as_u64().expect("nextSequence");
            return (text, next_sequence);
        }
        assert!(
            tokio::time::Instant::now() < deadline,
            "output never contained {needle:?}; last seen: {text:?}"
        );
        tokio::time::sleep(Duration::from_millis(75)).await;
    }
}

#[tokio::test]
async fn terminal_survives_simulated_daemon_restart_and_reattaches_by_id() {
    let client = reqwest::Client::new();
    let state_home = temp_fixture_path("state");
    std::fs::create_dir_all(&state_home).expect("create shared state home dir");
    let work_root = temp_fixture_path("root");
    std::fs::create_dir_all(&work_root).expect("create work root dir");

    // --- Daemon instance #1: create a live terminal and prove it is alive.
    let daemon_a = spawn_real_daemon(&state_home).await;
    let work_root_id = open_work_root(&client, &daemon_a.base_url, &work_root).await;
    let terminal_id = create_terminal(&client, &daemon_a.base_url, &work_root_id).await;

    send_input(
        &client,
        &daemon_a.base_url,
        &terminal_id,
        &echo_marker_command("PRE-RESTART-MARKER"),
    )
    .await;
    let (_, cursor_after_daemon_a) = poll_output_until_contains(
        &client,
        &daemon_a.base_url,
        &terminal_id,
        0,
        "PRE-RESTART-MARKER",
    )
    .await;

    // --- Hard-kill daemon #1 without any graceful shutdown. Only a
    // genuinely detached helper process (setsid + double-fork) can survive
    // this - an in-process PTY owned by the daemon would die with it.
    daemon_a.kill_hard().await;

    // --- Daemon instance #2: independent process, same state home. Its
    // boot-reconcile pass must adopt the still-running helper (reconcile row
    // 1: verified identity + reachable-and-alive IPC) before it starts
    // serving HTTP/WebSocket traffic at all.
    let daemon_b = spawn_real_daemon(&state_home).await;

    // The work root registry persisted through the restart (same
    // `WS_DASHBOARD_STATE_HOME`); re-open is what a reconnecting frontend
    // would do on load, and must resolve to the same opaque work root id.
    let reopened_work_root_id = open_work_root(&client, &daemon_b.base_url, &work_root).await;
    assert_eq!(
        reopened_work_root_id, work_root_id,
        "work root id must be stable across a restart"
    );

    // `list_terminals` on the new daemon instance must show the adopted
    // terminal - the existing frontend resume-by-id path depends on this.
    let listed: serde_json::Value = client
        .get(format!(
            "{}/api/dashboard/work-roots/{work_root_id}/terminals",
            daemon_b.base_url
        ))
        .send()
        .await
        .expect("list terminals on daemon #2 request")
        .json()
        .await
        .expect("list terminals on daemon #2 JSON");
    let listed_terminals = listed.as_array().expect("listed terminals array");
    let adopted = listed_terminals
        .iter()
        .find(|entry| entry["terminalId"] == terminal_id)
        .unwrap_or_else(|| panic!("adopted terminal missing from list: {listed_terminals:?}"));
    assert_eq!(adopted["status"], "running", "adopted terminal must be live");

    // Reattach over WebSocket at the exact pre-restart cursor and confirm
    // continuity: no duplicate/missing chunks, i.e. every replayed output
    // chunk's sequence must be `> cursor_after_daemon_a` and strictly
    // increasing.
    let mut request = format!(
        "ws://{}/api/dashboard/terminals/{terminal_id}/socket?after={cursor_after_daemon_a}",
        daemon_b
            .base_url
            .strip_prefix("http://")
            .expect("daemon base url is http")
    )
    .into_client_request()
    .expect("reattach websocket request");
    request
        .headers_mut()
        .insert(axum::http::header::HOST, "127.0.0.1".parse().unwrap());
    let (mut socket, response) = tokio_tungstenite::connect_async(request)
        .await
        .expect("reattach websocket upgrades");
    assert_eq!(
        response.status(),
        reqwest::StatusCode::SWITCHING_PROTOCOLS,
        "reattach websocket upgrade"
    );

    let mut last_sequence: Option<u64> = None;
    let mut saw_status_or_exit = false;
    let deadline = tokio::time::Instant::now() + Duration::from_secs(3);
    while tokio::time::Instant::now() < deadline {
        let Some(message) = tokio::time::timeout(Duration::from_millis(500), socket.next())
            .await
            .unwrap_or(None)
        else {
            continue;
        };
        let message = message.expect("reattach websocket message");
        let TungsteniteMessage::Text(payload) = message else {
            continue;
        };
        let value: serde_json::Value =
            serde_json::from_str(&payload).expect("reattach websocket frame JSON");
        assert_eq!(value["terminalId"], terminal_id);
        match value["type"].as_str() {
            Some("output") => {
                let sequence = value["chunk"]["sequence"].as_u64().expect("chunk sequence");
                assert!(
                    sequence > cursor_after_daemon_a,
                    "reattach must not replay chunks already delivered before restart: \
                     sequence={sequence} cursor={cursor_after_daemon_a}"
                );
                if let Some(previous) = last_sequence {
                    assert!(
                        sequence > previous,
                        "reattach chunk sequences must be strictly increasing: \
                         previous={previous} sequence={sequence}"
                    );
                }
                last_sequence = Some(sequence);
            }
            Some("status") => {
                assert_eq!(value["status"], "running");
                saw_status_or_exit = true;
            }
            _ => {}
        }
    }
    assert!(
        saw_status_or_exit,
        "reattach must deliver at least one status frame confirming the terminal is still running"
    );

    // Prove the reattached IPC pipe still reaches the SAME live shell
    // process (not a freshly spawned one) by writing a second marker through
    // the reattached WebSocket and observing it via the HTTP output route.
    socket
        .send(TungsteniteMessage::Text(
            serde_json::json!({
                "type": "input",
                "data": echo_marker_command("POST-RESTART-MARKER"),
            })
            .to_string()
            .into(),
        ))
        .await
        .expect("send post-restart input over reattached websocket");
    let (_, _) = poll_output_until_contains(
        &client,
        &daemon_b.base_url,
        &terminal_id,
        cursor_after_daemon_a,
        "POST-RESTART-MARKER",
    )
    .await;

    socket.close(None).await.expect("close reattach websocket");

    // Cleanup: explicitly close the terminal (reaps the detached helper +
    // shell) before killing daemon #2, then hard-kill daemon #2 too.
    let close_response = client
        .delete(format!(
            "{}/api/dashboard/terminals/{terminal_id}",
            daemon_b.base_url
        ))
        .send()
        .await
        .expect("close terminal request");
    assert_eq!(
        close_response.status(),
        reqwest::StatusCode::NO_CONTENT,
        "close terminal"
    );

    daemon_b.kill_hard().await;
    let _ = std::fs::remove_dir_all(&state_home);
    let _ = std::fs::remove_dir_all(&work_root);
}

// CONTRACT (260723 Phase 1 review finding I3): reconcile row 2 (AdoptGrace)
// must be exercised through the REAL async `TerminalRegistry::boot_reconcile`
// driver with a genuine helper connection, not merely
// `terminal_reconcile::classify(VerifiedOurs, ReachableShellExited) ==
// AdoptGrace` in isolation - that pure table-lookup test never touches
// `reconcile_entry`, `connect_and_handshake`, or `TerminalSession::
// from_connection`, the exact code that must correctly translate a helper's
// already-exited handshake response into a properly grace-timed adopted
// session across a restart. This also regression-guards the I1 fix (helper
// retained-ring backfill on connect): the marker below is produced entirely
// during the down window, before daemon #2's IPC connection to the helper -
// and before the reattaching WebSocket - ever exist, so nothing will
// re-deliver it to the fresh (empty) daemon-side proxy cache unless the
// helper proactively flushes its retained ring on every (re)connect.
#[tokio::test]
async fn terminal_boot_reconcile_adopts_grace_row_and_delivers_final_output_on_reattach() {
    let client = reqwest::Client::new();
    let state_home = temp_fixture_path("state-row2");
    std::fs::create_dir_all(&state_home).expect("create shared state home dir");
    let work_root = temp_fixture_path("root-row2");
    std::fs::create_dir_all(&work_root).expect("create work root dir");

    // --- Daemon #1: create a live terminal and schedule its shell to print
    // a marker, then exit on its own shortly after.
    let daemon_a = spawn_real_daemon(&state_home).await;
    let work_root_id = open_work_root(&client, &daemon_a.base_url, &work_root).await;
    let terminal_id = create_terminal(&client, &daemon_a.base_url, &work_root_id).await;

    send_input(
        &client,
        &daemon_a.base_url,
        &terminal_id,
        &delayed_exit_marker_command("ROW2-GRACE-MARKER"),
    )
    .await;
    poll_output_until_contains(
        &client,
        &daemon_a.base_url,
        &terminal_id,
        0,
        "ROW2-GRACE-MARKER",
    )
    .await;

    // --- Hard-kill daemon #1 *before* the shell's scheduled exit fires -
    // the shell is still alive at this point (still inside its delay), so
    // its eventual exit genuinely happens with no daemon attached at all,
    // rather than while a daemon connection is still live to witness it.
    daemon_a.kill_hard().await;

    // Give the shell time to run its scheduled exit while no daemon is
    // connected.
    tokio::time::sleep(DELAYED_EXIT_MARGIN).await;

    // --- Daemon #2: an independent process, same state home. Its
    // boot-reconcile pass must classify this entry as row 2 (AdoptGrace: IPC
    // reachable, shell already exited) through the real async driver.
    let daemon_b = spawn_real_daemon(&state_home).await;

    let listed: serde_json::Value = client
        .get(format!(
            "{}/api/dashboard/work-roots/{work_root_id}/terminals",
            daemon_b.base_url
        ))
        .send()
        .await
        .expect("list terminals on daemon #2 request")
        .json()
        .await
        .expect("list terminals on daemon #2 JSON");
    let listed_terminals = listed.as_array().expect("listed terminals array");
    let adopted = listed_terminals
        .iter()
        .find(|entry| entry["terminalId"] == terminal_id)
        .unwrap_or_else(|| panic!("grace-adopted terminal missing from list: {listed_terminals:?}"));
    assert_eq!(
        adopted["status"], "exited",
        "boot-reconcile must adopt row 2 with the helper's real exited status, not `running`"
    );

    // --- One grace reattach: both the WS `GONE` gate and the list filter
    // had to be relaxed for exactly this case (ticket "Boot reconcile
    // policy" row 2). Confirm the reattach succeeds and delivers the marker
    // the shell printed before it exited during the down window, plus the
    // terminal's non-running status/exit frame.
    let mut request = format!(
        "ws://{}/api/dashboard/terminals/{terminal_id}/socket?after=0",
        daemon_b
            .base_url
            .strip_prefix("http://")
            .expect("daemon base url is http")
    )
    .into_client_request()
    .expect("grace reattach websocket request");
    request
        .headers_mut()
        .insert(axum::http::header::HOST, "127.0.0.1".parse().unwrap());
    let (mut socket, response) = tokio_tungstenite::connect_async(request)
        .await
        .expect("grace reattach websocket upgrades");
    assert_eq!(
        response.status(),
        reqwest::StatusCode::SWITCHING_PROTOCOLS,
        "grace reattach websocket upgrade"
    );

    let mut text = String::new();
    let mut saw_non_running_status_or_exit = false;
    let deadline = tokio::time::Instant::now() + Duration::from_secs(5);
    while tokio::time::Instant::now() < deadline {
        let Some(message) = tokio::time::timeout(Duration::from_millis(500), socket.next())
            .await
            .unwrap_or(None)
        else {
            continue;
        };
        let message = message.expect("grace reattach websocket message");
        let TungsteniteMessage::Text(payload) = message else {
            continue;
        };
        let value: serde_json::Value =
            serde_json::from_str(&payload).expect("grace reattach websocket frame JSON");
        assert_eq!(value["terminalId"], terminal_id);
        match value["type"].as_str() {
            Some("output") => {
                text.push_str(value["chunk"]["data"].as_str().expect("output data"));
            }
            Some("status") => {
                if value["status"] != "running" {
                    saw_non_running_status_or_exit = true;
                }
            }
            Some("exit") => {
                saw_non_running_status_or_exit = true;
                break;
            }
            _ => {}
        }
        if text.contains("ROW2-GRACE-MARKER") && saw_non_running_status_or_exit {
            break;
        }
    }
    assert!(
        text.contains("ROW2-GRACE-MARKER"),
        "grace reattach must deliver the buffered marker produced during the down window: {text:?}"
    );
    assert!(
        saw_non_running_status_or_exit,
        "grace reattach must deliver the terminal's non-running status/exit on this one reattach"
    );

    socket
        .close(None)
        .await
        .expect("close grace reattach websocket");

    // Cleanup: explicitly close the terminal before killing daemon #2.
    let close_response = client
        .delete(format!(
            "{}/api/dashboard/terminals/{terminal_id}",
            daemon_b.base_url
        ))
        .send()
        .await
        .expect("close grace-adopted terminal request");
    assert_eq!(
        close_response.status(),
        reqwest::StatusCode::NO_CONTENT,
        "close grace-adopted terminal"
    );

    daemon_b.kill_hard().await;
    let _ = std::fs::remove_dir_all(&state_home);
    let _ = std::fs::remove_dir_all(&work_root);
}

// Leak-safe cleanup guard for the live-EOF test below. The daemon spawns its
// PTY helper detached via `setsid()` + double-fork, so the helper outlives the
// daemon's `kill_on_drop`/SIGKILL and would LEAK on any panic path between
// terminal-create and the shell's normal exit (e.g. a readiness `poll_output_*`
// timeout unwinding before the trailing cleanup ever runs). This guard fires on
// Drop - on BOTH the success path and any unwind - and best-effort reaps a
// still-live helper by the identity the helper itself persisted: its
// `<terminal_id>.json` registry entry (written under `WS_DASHBOARD_STATE_HOME`)
// carries the helper's real `pid` and `startTime`. It verifies the PID's
// `/proc` start-time matches before signalling, so a recycled PID is never
// killed. Then it removes the temp dirs.
//
// CONTRACT: the identity-verified reap (pid + /proc start-time match) closes the
// PID-reuse window entirely. The only residual risk is a panic in the razor-thin
// window after `create_terminal` returns but before the helper has flushed its
// registry `.json`; such an untracked helper is left to the OS, which EOF-exits
// it once its orphaned PTY master is dropped. Fix #1 (the marker handshake)
// makes the panic path rare regardless, so this guard is defense in depth.
struct HelperReaper {
    state_home: PathBuf,
    work_root: PathBuf,
}

impl Drop for HelperReaper {
    fn drop(&mut self) {
        #[cfg(unix)]
        if let Ok(entries) = std::fs::read_dir(&self.state_home) {
            for entry in entries.flatten() {
                let path = entry.path();
                if path.extension().and_then(|extension| extension.to_str()) != Some("json") {
                    continue;
                }
                let Ok(raw) = std::fs::read_to_string(&path) else {
                    continue;
                };
                let Ok(value) = serde_json::from_str::<serde_json::Value>(&raw) else {
                    continue;
                };
                // Only `<terminal_id>.json` registry entries carry `pid`/`startTime`;
                // sibling state files (e.g. `opened-workroots.json`) are skipped here.
                let (Some(pid), Some(start_time)) = (
                    value.get("pid").and_then(serde_json::Value::as_u64),
                    value.get("startTime").and_then(serde_json::Value::as_u64),
                ) else {
                    continue;
                };
                if proc_start_time(pid) != Some(start_time) {
                    // PID gone or recycled for another process - never signal a stranger.
                    continue;
                }
                let _ = std::process::Command::new("kill")
                    .arg("-KILL")
                    .arg(pid.to_string())
                    .status();
            }
        }
        let _ = std::fs::remove_dir_all(&self.state_home);
        let _ = std::fs::remove_dir_all(&self.work_root);
    }
}

// Mirror of `terminal_platform::unix::process_start_time`: `/proc/<pid>/stat`
// field 22 (`starttime`), read after the `)` that closes the (possibly
// space-containing) comm field. `None` if the process is gone.
#[cfg(unix)]
fn proc_start_time(pid: u64) -> Option<u64> {
    let stat = std::fs::read_to_string(format!("/proc/{pid}/stat")).ok()?;
    let after_comm = stat.rsplit_once(')')?.1;
    after_comm.split_whitespace().nth(19)?.parse().ok()
}

// CONTRACT (260724 dead-shell Phase 3, Unix-regression leg): guard the LIVE,
// steady-state PTY-master-EOF exit-detection path. Phase 1 added a
// `#[cfg(windows)]` process-handle "reaper" thread (blocks on
// `WaitForSingleObject`, drives `transition(Exited)` on shell death) plus a
// kill-path reorder; that reaper is compiled OUT on Unix. On Unix, exit
// detection still relies entirely on the pre-existing path: the helper's PTY
// reader sees `read()==Ok(0)` (EOF) when the shell dies and calls
// `transition(Exited)`. This test proves that path was NOT regressed by the
// Phase 1 wiring: with a SINGLE live daemon and a WebSocket attached, a shell
// that exits normally must flip the terminal's status to `exited` over the
// live socket via the reader EOF path - no daemon restart, no boot-reconcile.
//
// It is deliberately the third, distinct case the other two tests do NOT cover:
//   * `terminal_survives_simulated_daemon_restart_and_reattaches_by_id` keeps
//     the shell ALIVE across a restart (survive-restart; it never exits).
//   * `terminal_boot_reconcile_adopts_grace_row_and_delivers_final_output_on_reattach`
//     lets the shell exit during the daemon-DOWN window and discovers `exited`
//     later via BOOT-RECONCILE row 2 (AdoptGrace), not a live witness.
//   * THIS test is the live-witness case: the shell exits while a daemon is up
//     and a socket is attached, so `exited` arrives in real time over the PTY-
//     EOF reader path. We assert specifically on `exited` (not merely
//     "non-running") to pin which path is being guarded.
//
// Readiness before sending `exit` uses the same echo-marker handshake the
// sibling `terminal_survives_...` test uses (send `echo <marker>`, poll the
// terminal's real output until the marker appears): observing the marker in
// output is a DETERMINISTIC proof the interactive shell finished startup and is
// executing input, so the subsequent `exit` is reliably processed rather than
// buffered/re-typed by an async line editor (ZLE) mid-startup. It deliberately
// does NOT use an output-timing/quiet-gap heuristic, which is load-sensitive on
// hosts with a heavy async prompt (e.g. p10k zsh) and can time out under load.
#[tokio::test]
async fn terminal_live_pty_eof_exit_flips_status_to_exited() {
    let client = reqwest::Client::new();
    let state_home = temp_fixture_path("state-live-eof");
    std::fs::create_dir_all(&state_home).expect("create state home dir");
    let work_root = temp_fixture_path("root-live-eof");
    std::fs::create_dir_all(&work_root).expect("create work root dir");

    // Leak-safe cleanup: reaps any still-live detached helper and removes the
    // temp dirs on EVERY exit path, including a panic that unwinds before the
    // trailing normal cleanup. Declared before `daemon` so it drops LAST (after
    // `daemon`'s `kill_on_drop` has taken the daemon down), then reaps the
    // orphaned helper the daemon kill could not reach. See `HelperReaper`.
    let _reaper = HelperReaper {
        state_home: state_home.clone(),
        work_root: work_root.clone(),
    };

    // --- One live daemon; create a terminal and attach a WebSocket to witness
    // its exit in real time.
    let daemon = spawn_real_daemon(&state_home).await;
    let work_root_id = open_work_root(&client, &daemon.base_url, &work_root).await;
    let terminal_id = create_terminal(&client, &daemon.base_url, &work_root_id).await;

    let mut request = format!(
        "ws://{}/api/dashboard/terminals/{terminal_id}/socket?after=0",
        daemon
            .base_url
            .strip_prefix("http://")
            .expect("daemon base url is http")
    )
    .into_client_request()
    .expect("live-eof websocket request");
    request
        .headers_mut()
        .insert(axum::http::header::HOST, "127.0.0.1".parse().unwrap());
    let (mut socket, response) = tokio_tungstenite::connect_async(request)
        .await
        .expect("live-eof websocket upgrades");
    assert_eq!(
        response.status(),
        reqwest::StatusCode::SWITCHING_PROTOCOLS,
        "live-eof websocket upgrade"
    );

    // --- Readiness handshake before sending `exit`. On Unix the default shell
    // here is an interactive shell (e.g. zsh) whose rc / prompt / line-editor
    // (ZLE) load asynchronously; input delivered during that startup window is
    // buffered and re-processed unreliably (bracketed-paste + ZLE re-typing the
    // buffer one keystroke at a time), so an `exit` sent too early can sit
    // unexecuted for seconds. Rather than guess readiness from output timing (a
    // quiet-gap heuristic that is load-sensitive and times out under a heavy
    // async prompt), we use the proven marker handshake the sibling
    // `terminal_survives_...` test relies on: send `echo <marker>` and poll the
    // terminal's ACTUAL output until the marker appears. Seeing the marker is a
    // deterministic proof the shell finished startup and is executing input, so
    // the `exit` that follows is reliably processed. `poll_output_until_contains`
    // panics on its own timeout; the `HelperReaper` guard above makes that panic
    // path leak-safe.
    let ready_marker = "LIVE-EOF-READY-MARKER";
    send_input(
        &client,
        &daemon.base_url,
        &terminal_id,
        &echo_marker_command(ready_marker),
    )
    .await;
    let (_, _) =
        poll_output_until_contains(&client, &daemon.base_url, &terminal_id, 0, ready_marker).await;

    // --- Make the shell exit normally so the PTY master EOFs. Sent over the
    // live WebSocket so the whole exit is witnessed on one live connection.
    let exit_command = if cfg!(windows) { "exit\r\n" } else { "exit\n" };
    socket
        .send(TungsteniteMessage::Text(
            serde_json::json!({ "type": "input", "data": exit_command })
                .to_string()
                .into(),
        ))
        .await
        .expect("send exit input over live websocket");

    // --- Drain frames until we witness the terminal go `exited`. Generous
    // bounded deadline (real OS processes; consistent with the other tests'
    // 3-5s socket-drain windows) so slow shell teardown under concurrent test
    // load never produces a false failure.
    let mut saw_exited = false;
    let deadline = tokio::time::Instant::now() + Duration::from_secs(5);
    while tokio::time::Instant::now() < deadline {
        let message = match tokio::time::timeout(Duration::from_millis(500), socket.next()).await {
            Ok(Some(message)) => message,
            // Stream ended: no further frames will ever arrive, so stop
            // waiting immediately instead of busy-spinning to the deadline.
            Ok(None) => break,
            // Per-poll timeout: keep waiting until the outer deadline.
            Err(_elapsed) => continue,
        };
        let message = message.expect("live-eof websocket message");
        let TungsteniteMessage::Text(payload) = message else {
            continue;
        };
        let value: serde_json::Value =
            serde_json::from_str(&payload).expect("live-eof websocket frame JSON");
        assert_eq!(value["terminalId"], terminal_id);
        match value["type"].as_str() {
            // The PTY-EOF path transitions to `Exited`; assert specifically on
            // that (not merely non-running) so this guards the exact path.
            Some("status") => {
                if value["status"] == "exited" {
                    saw_exited = true;
                    break;
                }
            }
            Some("exit") => {
                // Pin the exited path too: an exit frame carrying e.g.
                // `terminated`/`error` must NOT satisfy this guard (mirror the
                // status arm's ==`exited` check). The exit frame ends the live
                // attachment either way, so break regardless.
                if value["status"] == "exited" {
                    saw_exited = true;
                }
                break;
            }
            _ => {}
        }
    }
    assert!(
        saw_exited,
        "live PTY-EOF exit path must flip status to `exited` over the live socket"
    );

    socket.close(None).await.expect("close live-eof websocket");

    // Cleanup: the shell has already exited, so the terminal row may already
    // be gone; tolerate either NO_CONTENT (still present) or NOT_FOUND (already
    // reaped). Then hard-kill the daemon and drop the temp dirs.
    let close_response = client
        .delete(format!(
            "{}/api/dashboard/terminals/{terminal_id}",
            daemon.base_url
        ))
        .send()
        .await
        .expect("close live-eof terminal request");
    assert!(
        matches!(
            close_response.status(),
            reqwest::StatusCode::NO_CONTENT | reqwest::StatusCode::NOT_FOUND
        ),
        "close live-eof terminal: unexpected status {}",
        close_response.status()
    );

    daemon.kill_hard().await;
    let _ = std::fs::remove_dir_all(&state_home);
    let _ = std::fs::remove_dir_all(&work_root);
}
