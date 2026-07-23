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
