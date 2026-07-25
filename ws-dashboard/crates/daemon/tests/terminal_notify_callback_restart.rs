// CONTRACT (260725 Phase 4): the ticket is explicit that the restart-survival
// verification bullet must cover the CALLBACK URL, not merely token
// survival - "after a daemon restart on a DIFFERENT port with a live helper
// still running, a callback from that helper must still arrive." This test
// proves that literally: two REAL OS daemon processes (mirrors
// `terminal_lifetime.rs`'s own two-real-process restart harness), a hard
// `SIGKILL` between them, and a genuine HTTP POST to the turn-state route on
// daemon #2's NEW ephemeral port using the SAME token daemon #1 minted.
//
// This same test also doubles as the GC-sweep ORDERING regression: daemon
// #2's `agent-profiles/<terminal_id>/` directory (and its `callback.json`)
// must survive its own boot, because the terminal is adopted by
// `boot_reconcile` before the sweep ever runs against it - see
// `agent_profile_gc.rs`'s module CONTRACT and `server.rs`'s wiring comment.
//
// UNLIKE `terminal_lifetime.rs`, this harness runs with OWNER AUTH ENABLED
// (no `--no-auth`) per the ticket's own verification requirement, so it
// scrapes the "owner pairing URL" startup line and performs a real pairing
// exchange to obtain a session cookie before touching any protected route.

use std::path::PathBuf;
use std::process::Stdio;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, Command};
use tokio::time::timeout;

const STARTUP_TIMEOUT: Duration = Duration::from_secs(10);

static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

// CONTRACT (macOS Unix-domain-socket path-length ceiling - same root cause
// documented in `routes.rs::terminal_registry_temp_dir` and
// `terminal_lifetime.rs::temp_fixture_path`): the terminal helper's `.sock`
// path is derived from `WS_DASHBOARD_STATE_HOME`, so this fixture uses the
// same short-base-on-macOS scheme those two files already established.
fn temp_fixture_path(name: &str) -> PathBuf {
    let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    #[cfg(target_os = "macos")]
    let base = PathBuf::from("/tmp");
    #[cfg(not(target_os = "macos"))]
    let base = std::env::temp_dir();
    base.join(format!(
        "ws-dashboard-turn-state-restart-{name}-{}-{unique}",
        std::process::id()
    ))
}

struct DaemonProcess {
    child: Child,
    base_url: String,
    pairing_url: String,
}

impl DaemonProcess {
    async fn kill_hard(mut self) {
        // CONTRACT: SIGKILL, no graceful shutdown - only a genuinely
        // detached, independently-surviving helper process passes this test.
        self.child.kill().await.expect("SIGKILL daemon process");
        let _ = self.child.wait().await;
    }
}

// DOGFOOD FINDING (260725 Phase 4, surfaced by this test, not by argument):
// `agent_profile_registry::CLAUDE_PROFILE` (`command: "claude"`) is the ONLY
// registered profile carrying a `hook_config` - `dummy-echo` deliberately has
// none - so a token/`callback.json` restart-survival test has no choice but
// to spawn a terminal with `profileId: "claude"`. The plan's own Codebase
// Findings cites Phase 2's finding that `create_terminal`'s HTTP response
// completes before the helper attempts to spawn the resolved command, and
// concludes "using the unavailable `claude` binary here costs nothing." That
// holds for a plain create-and-observe-the-response test, but this is a
// RESTART test: `terminal_helper_process.rs::spawn_shell` failing (`claude`
// not found) transitions the shell to `TerminalHelperStatus::Error`
// (`SharedState::transition`, which sets `exited_at`), and
// `serve_connections`'s loop self-exits the WHOLE HELPER PROCESS as soon as
// the daemon's IPC connection next drops AND `exited_at` is already
// `Some` - which is exactly what a hard `SIGKILL` of daemon #1 triggers.
// Verified empirically: the first version of this test, without this
// stand-in, failed with the OLD (daemon #1) base URL still in
// `callback.json` after "restart" - the helper had already self-exited
// before daemon #2 ever started, so there was nothing left to adopt. A
// stand-in `claude` executable that just sleeps, placed ahead of the real
// `PATH` for daemon #1's own process (whose env `TerminalSession::spawn`
// forwards into the helper's env-scrubbed argv), keeps the shell `Running`
// through the SIGKILL - mirroring, for THIS test's purposes, the same role
// `agent_profile_registry.rs`'s own `dummy-echo` profile plays for its
// callers.
fn install_fake_claude_binary(bin_dir: &std::path::Path) {
    std::fs::create_dir_all(bin_dir).expect("create fake claude bin dir");
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let path = bin_dir.join("claude");
        std::fs::write(&path, "#!/bin/sh\nsleep 60\n").expect("write fake claude script");
        let mut perms = std::fs::metadata(&path)
            .expect("fake claude script metadata")
            .permissions();
        perms.set_mode(0o755);
        std::fs::set_permissions(&path, perms).expect("chmod fake claude script executable");
    }
    #[cfg(windows)]
    {
        let path = bin_dir.join("claude.bat");
        std::fs::write(&path, "@echo off\r\nping -n 61 127.0.0.1 > NUL\r\n")
            .expect("write fake claude batch file");
    }
}

fn path_with_prefix(extra_dir: &std::path::Path) -> std::ffi::OsString {
    let existing = std::env::var_os("PATH").unwrap_or_default();
    std::env::join_paths(std::iter::once(extra_dir.to_path_buf()).chain(std::env::split_paths(&existing)))
        .expect("join PATH with fake claude bin dir prefix")
}

async fn spawn_real_daemon(
    state_home: &std::path::Path,
    fake_claude_bin_dir: &std::path::Path,
) -> DaemonProcess {
    let mut child = Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))
        .arg("serve")
        .arg("--port")
        .arg("0")
        .arg("--bind-mode")
        .arg("local")
        .env("WS_DASHBOARD_STATE_HOME", state_home)
        .env_remove("WS_DASHBOARD_STATE_FILE")
        // See `install_fake_claude_binary`'s CONTRACT above: this is what
        // makes the spawned `claude`-profile terminal's underlying shell
        // resolve to a stand-in that stays alive, instead of erroring out
        // and self-exiting the helper.
        .env("PATH", path_with_prefix(fake_claude_bin_dir))
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::piped())
        .kill_on_drop(true)
        .spawn()
        .expect("spawn real ws-dashboard daemon process");

    let stderr = child.stderr.take().expect("daemon stderr pipe");
    let mut lines = BufReader::new(stderr).lines();
    let pairing_url = timeout(STARTUP_TIMEOUT, async {
        loop {
            let line = lines
                .next_line()
                .await
                .expect("read daemon stderr")
                .expect("daemon exited before printing its owner pairing URL");
            if let Some(url) = line.strip_prefix("ws-dashboard owner pairing URL: ") {
                break url.trim().to_owned();
            }
        }
    })
    .await
    .expect("daemon did not print its owner pairing URL in time");

    // Keep draining stderr in the background so the pipe never fills up and
    // blocks the daemon; nothing else in this test reads from it.
    tokio::spawn(async move { while let Ok(Some(_)) = lines.next_line().await {} });

    let base_url = base_url_from_pairing_url(&pairing_url);
    DaemonProcess {
        child,
        base_url,
        pairing_url,
    }
}

// `pairing_url` is `http://<host>:<port>/pair?token=<token>` - strip the
// `/pair` path and query to recover the bare base URL every other route in
// this test needs.
fn base_url_from_pairing_url(pairing_url: &str) -> String {
    let without_query = pairing_url.split('?').next().expect("pairing url has a path");
    without_query
        .strip_suffix("/pair")
        .expect("pairing url ends in /pair")
        .to_owned()
}

// CONTRACT: built with NO automatic redirect-following - the pairing route
// responds `303 See Other` with a `Set-Cookie` header on the FIRST response;
// following the redirect would risk observing headers from the SECOND
// (post-redirect) response instead, which does not carry the cookie the same
// way. Every other request in this test is a direct-action endpoint (no
// redirects expected), so disabling redirects is harmless everywhere else.
fn http_client() -> reqwest::Client {
    reqwest::Client::builder()
        .redirect(reqwest::redirect::Policy::none())
        .build()
        .expect("build reqwest client")
}

async fn pair_and_get_cookie(client: &reqwest::Client, pairing_url: &str) -> String {
    let response = client
        .get(pairing_url)
        .send()
        .await
        .expect("pair request");
    assert_eq!(
        response.status(),
        reqwest::StatusCode::SEE_OTHER,
        "pairing exchange must succeed"
    );
    response
        .headers()
        .get(reqwest::header::SET_COOKIE)
        .expect("owner session cookie")
        .to_str()
        .expect("cookie header is ASCII")
        .split(';')
        .next()
        .expect("cookie pair")
        .to_owned()
}

async fn open_work_root(
    client: &reqwest::Client,
    base_url: &str,
    cookie: &str,
    root: &std::path::Path,
) -> String {
    let response = client
        .post(format!("{base_url}/api/dashboard/work-roots/open"))
        .header(reqwest::header::COOKIE, cookie)
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

// CONTRACT: `profileId: "claude"` resolves to a real argv with a hook config
// (see `agent_profile_registry.rs`) even though the `claude` binary itself is
// not expected to exist on the test machine - `create_terminal`'s HTTP
// response completes before the helper ever attempts to spawn the resolved
// command (Phase 2's own finding, reused verbatim by this restart harness
// per the plan's Codebase Findings), so this costs nothing and avoids a real
// vendor-binary dependency.
async fn create_terminal_with_profile(
    client: &reqwest::Client,
    base_url: &str,
    cookie: &str,
    work_root_id: &str,
) -> String {
    let response = client
        .post(format!(
            "{base_url}/api/dashboard/work-roots/{work_root_id}/terminals"
        ))
        .header(reqwest::header::COOKIE, cookie)
        .json(&serde_json::json!({
            "columns": 80,
            "rows": 24,
            "title": "restart callback test terminal",
            "profileId": "claude",
        }))
        .send()
        .await
        .expect("create terminal request");
    assert_eq!(response.status(), reqwest::StatusCode::OK, "create terminal");
    let created: serde_json::Value = response.json().await.expect("create terminal JSON");
    created["terminalId"]
        .as_str()
        .expect("terminal id")
        .to_owned()
}

async fn post_turn_state(
    client: &reqwest::Client,
    base_url: &str,
    terminal_id: &str,
    token: &str,
) -> reqwest::StatusCode {
    client
        .post(format!(
            "{base_url}/api/dashboard/terminals/{terminal_id}/turn-state"
        ))
        .json(&serde_json::json!({ "token": token, "state": "working" }))
        .send()
        .await
        .expect("turn-state POST")
        .status()
}

fn read_callback_json(path: &std::path::Path) -> serde_json::Value {
    let raw = std::fs::read_to_string(path)
        .unwrap_or_else(|error| panic!("read callback.json at {}: {error}", path.display()));
    serde_json::from_str(&raw)
        .unwrap_or_else(|error| panic!("parse callback.json at {}: {error}", path.display()))
}

#[tokio::test]
async fn agent_terminal_survives_restart_with_correct_gc_ordering_and_rewritten_callback_url() {
    let client = http_client();
    let state_home = temp_fixture_path("state");
    std::fs::create_dir_all(&state_home).expect("create shared state home dir");
    let work_root = temp_fixture_path("root");
    std::fs::create_dir_all(&work_root).expect("create work root dir");
    let fake_claude_bin_dir = temp_fixture_path("fake-claude-bin");
    install_fake_claude_binary(&fake_claude_bin_dir);

    // --- Daemon #1: owner auth ENABLED, create a hook-bearing terminal so a
    // real callback token + `callback.json` get materialized. See
    // `install_fake_claude_binary`'s CONTRACT for why the resolved `claude`
    // profile must actually stay running across the restart.
    let daemon_a = spawn_real_daemon(&state_home, &fake_claude_bin_dir).await;
    let cookie_a = pair_and_get_cookie(&client, &daemon_a.pairing_url).await;
    let work_root_id = open_work_root(&client, &daemon_a.base_url, &cookie_a, &work_root).await;
    let terminal_id =
        create_terminal_with_profile(&client, &daemon_a.base_url, &cookie_a, &work_root_id).await;

    let profile_dir = state_home.join("agent-profiles").join(&terminal_id);
    let callback_path = profile_dir.join("callback.json");
    assert!(
        callback_path.exists(),
        "a claude-profile terminal must materialize a callback.json"
    );

    let callback_before = read_callback_json(&callback_path);
    assert_eq!(callback_before["baseUrl"], daemon_a.base_url);
    assert_eq!(callback_before["terminalId"], terminal_id);
    let token = callback_before["token"]
        .as_str()
        .expect("callback.json token field")
        .to_owned();

    // A real callback fire against daemon #1 itself, before any restart,
    // must already succeed - establishes the pre-restart baseline this test
    // then proves survives.
    assert_eq!(
        post_turn_state(&client, &daemon_a.base_url, &terminal_id, &token).await,
        reqwest::StatusCode::NO_CONTENT,
        "turn-state POST against daemon #1 must succeed before any restart"
    );

    // --- Hard-kill daemon #1. No graceful shutdown - only a genuinely
    // detached helper process (and its still-live terminal-helper) survives
    // this.
    let daemon_a_base_url = daemon_a.base_url.clone();
    daemon_a.kill_hard().await;

    // --- Daemon #2: same state home, fresh ephemeral port, owner auth
    // ENABLED again.
    let daemon_b = spawn_real_daemon(&state_home, &fake_claude_bin_dir).await;
    assert_ne!(
        daemon_a_base_url, daemon_b.base_url,
        "restart must bind a different ephemeral port for this test to be meaningful"
    );

    // CONTRACT (found by dogfooding this exact test - the "owner pairing
    // URL" stderr line is printed BEFORE `boot_reconcile` even starts, not
    // after - see `server.rs`'s startup sequence): a raw filesystem read of
    // `callback.json` has NO synchronization point forcing it to wait for
    // `boot_reconcile` to finish, unlike an HTTP request against the
    // protected router (which blocks until `axum::serve` actually starts
    // accepting - strictly after `boot_reconcile` and the GC-sweep-spawn
    // line complete). A real HTTP round trip - pairing itself is enough -
    // is therefore REQUIRED here before reading any daemon-private file off
    // disk; the first version of this test read the file immediately after
    // `spawn_real_daemon` returned and flaked exactly like a racy read would.
    let cookie_b = pair_and_get_cookie(&client, &daemon_b.pairing_url).await;

    // Confirms the adoption itself (not just the file-write side effect):
    // the terminal must be listed as running from daemon #2's own
    // browser-facing surface, exactly like the frontend's resume-by-id path
    // would observe it.
    let listed: serde_json::Value = client
        .get(format!(
            "{}/api/dashboard/work-roots/{work_root_id}/terminals",
            daemon_b.base_url
        ))
        .header(reqwest::header::COOKIE, &cookie_b)
        .send()
        .await
        .expect("list terminals on daemon #2 request")
        .json()
        .await
        .expect("list terminals on daemon #2 JSON");
    let adopted = listed
        .as_array()
        .expect("listed terminals array")
        .iter()
        .find(|entry| entry["terminalId"] == terminal_id);
    assert!(
        adopted.is_some(),
        "the terminal must be adopted and listed by daemon #2: {listed:?}"
    );

    // ORDERING REGRESSION ASSERTION (mandatory experiment 1): if the GC
    // sweep ran BEFORE `boot_reconcile` (this test's own regression guard),
    // it would see zero live terminal ids and delete this about-to-be-
    // adopted terminal's profile directory before adoption ever happened.
    assert!(
        callback_path.exists(),
        "GC sweep must never delete the profile directory of a terminal boot_reconcile is \
         about to adopt - see agent_profile_gc.rs's ordering CONTRACT"
    );

    // URL SURVIVAL ASSERTION (mandatory experiment 3): callback.json must
    // have been rewritten with daemon #2's fresh base URL, using the SAME
    // (never-rotated) token.
    let callback_after = read_callback_json(&callback_path);
    assert_eq!(
        callback_after["baseUrl"], daemon_b.base_url,
        "callback.json must be rewritten with the restarted daemon's fresh base URL"
    );
    assert_eq!(
        callback_after["token"], token,
        "the callback token must never rotate across a restart (Design Answer 1)"
    );
    assert_eq!(callback_after["terminalId"], terminal_id);

    // The ticket's headline acceptance line, proven literally: "after a
    // daemon restart on a DIFFERENT port with a live helper still running, a
    // callback from that helper must still arrive."
    assert_eq!(
        post_turn_state(&client, &daemon_b.base_url, &terminal_id, &token).await,
        reqwest::StatusCode::NO_CONTENT,
        "a callback using the SAME token must succeed against daemon #2's NEW base URL"
    );

    daemon_b.kill_hard().await;
    let _ = std::fs::remove_dir_all(&state_home);
    let _ = std::fs::remove_dir_all(&work_root);
    let _ = std::fs::remove_dir_all(&fake_claude_bin_dir);
}
