// CONTRACT: proves the `ws-dashboard terminal-notify` hook subcommand
// through the REAL CLI boundary (argv -> file resolution -> HTTP attempt),
// mirroring `terminal_lifetime.rs`'s `env!("CARGO_BIN_EXE_ws-dashboard")`
// real-subprocess pattern rather than calling internal functions directly -
// the CLI boundary itself is what this ticket's Phase 3 step 3 needs
// proving. No real vendor CLI is invoked; this test drives the compiled
// `ws-dashboard` binary directly with a synthetic fixture, and the "server"
// on the receiving end is a throwaway mock listener, never Phase 4's real
// (not-yet-implemented) route.

use std::path::PathBuf;
use std::process::Stdio;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use axum::extract::Path as AxumPath;
use axum::http::StatusCode;
use axum::routing::post;
use axum::{Json, Router};
use serde::Deserialize;
use serde_json::json;
use tokio::net::TcpListener;
use tokio::sync::oneshot;
use tokio::time::timeout;

static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

fn temp_fixture_dir(name: &str) -> PathBuf {
    let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    std::env::temp_dir().join(format!(
        "ws-dashboard-terminal-notify-{name}-{}-{unique}",
        std::process::id()
    ))
}

#[derive(Debug, Deserialize, PartialEq, Eq)]
struct ReceivedTurnState {
    token: String,
    state: String,
}

#[tokio::test]
async fn terminal_notify_resolves_the_callback_file_and_delivers_the_turn_state_post() {
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind mock callback receiver on an OS-assigned ephemeral port");
    let addr = listener.local_addr().expect("mock receiver local addr");

    let (tx, rx) = oneshot::channel::<(String, ReceivedTurnState)>();
    let tx = Arc::new(Mutex::new(Some(tx)));

    let app = Router::new().route(
        "/api/dashboard/terminals/{terminal_id}/turn-state",
        post(
            move |AxumPath(terminal_id): AxumPath<String>, Json(body): Json<ReceivedTurnState>| {
                if let Some(sender) = tx.lock().expect("mock receiver tx lock").take() {
                    let _ = sender.send((terminal_id, body));
                }
                async { StatusCode::OK }
            },
        ),
    );
    let server = tokio::spawn(async move {
        let _ = axum::serve(listener, app).await;
    });

    let fixture_dir = temp_fixture_dir("callback");
    std::fs::create_dir_all(&fixture_dir).expect("create fixture dir");
    let callback_path = fixture_dir.join("callback.json");
    std::fs::write(
        &callback_path,
        json!({
            "baseUrl": format!("http://{addr}"),
            "terminalId": "term_test",
            "token": "test-token",
        })
        .to_string(),
    )
    .expect("write callback fixture");

    let status = tokio::process::Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))
        .arg("terminal-notify")
        .arg("--callback")
        .arg(&callback_path)
        .arg("--state")
        .arg("ready")
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .status()
        .await
        .expect("spawn the real compiled ws-dashboard terminal-notify subprocess");
    assert!(
        status.success(),
        "terminal-notify must exit 0 when the mock receiver accepts the POST"
    );

    let (terminal_id, body) = timeout(Duration::from_secs(5), rx)
        .await
        .expect("mock receiver must observe exactly one POST within the timeout")
        .expect("mock receiver's oneshot sender must not be dropped without sending");
    assert_eq!(terminal_id, "term_test");
    assert_eq!(
        body,
        ReceivedTurnState {
            token: "test-token".to_owned(),
            state: "ready".to_owned(),
        }
    );

    server.abort();
    let _ = std::fs::remove_dir_all(&fixture_dir);
}

// CONTRACT (deliberate, empirically-verified design decision - see
// `terminal_notify.rs`'s module CONTRACT and the phase's implementation
// report): a real-PTY measurement against the compiled binary, run as an
// actual Claude hook pointed at a missing callback file, proved that a
// non-zero exit + stderr text makes Claude Code surface a visible
// "<Event> hook error" banner on every UserPromptSubmit/Stop - unacceptable
// per-turn noise in a user's live agent session, for as long as the
// callback file stays absent (exactly the Phase-3-only-vs-Phase-4 gap this
// ticket cannot close by itself). `terminal-notify` therefore exits 0 and
// prints NOTHING to stdout/stderr even on failure, recording the failure to
// a dedicated log file instead - this test proves both halves: silence on
// the process's own streams, and a non-silent trail in the log file.
#[tokio::test]
async fn terminal_notify_exits_zero_and_stays_silent_on_stdio_when_the_callback_file_is_missing() {
    let state_home = temp_fixture_dir("missing-callback-state-home");
    let fixture_dir = temp_fixture_dir("missing-callback");
    let callback_path = fixture_dir.join("callback.json");

    let output = tokio::process::Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))
        .arg("terminal-notify")
        .arg("--callback")
        .arg(&callback_path)
        .arg("--state")
        .arg("working")
        .env("WS_DASHBOARD_STATE_HOME", &state_home)
        .env_remove("WS_DASHBOARD_STATE_FILE")
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .await
        .expect("spawn terminal-notify against a missing callback file");

    assert!(
        output.status.success(),
        "terminal-notify must exit 0 even when the callback file is missing, so a hook fire \
         does not surface a per-turn error banner in the user's live agent session"
    );
    assert!(
        output.stdout.is_empty(),
        "terminal-notify must print nothing to stdout on failure, got: {}",
        String::from_utf8_lossy(&output.stdout)
    );
    assert!(
        output.stderr.is_empty(),
        "terminal-notify must print nothing to stderr on failure, got: {}",
        String::from_utf8_lossy(&output.stderr)
    );

    let log_contents = std::fs::read_to_string(state_home.join("logs").join("terminal-notify.log"))
        .expect("terminal-notify must append a diagnostic line to logs/terminal-notify.log");
    assert!(log_contents.contains("state=working"));
    assert!(log_contents.contains(&callback_path.display().to_string()));
    assert!(
        log_contents.contains("not found"),
        "expected the log line to name the actual failure, got: {log_contents}"
    );

    let _ = std::fs::remove_dir_all(&fixture_dir);
    let _ = std::fs::remove_dir_all(&state_home);
}
