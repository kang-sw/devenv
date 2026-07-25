// CONTRACT (260725 Phase 4 review cycle 1, finding D): proves REAL delivery
// end-to-end - the compiled `ws-dashboard terminal-notify` subprocess (real
// CLI argv parsing, real `agent_callback::resolve_callback_target` file
// read, real `reqwest` HTTP client construction) driven against the REAL,
// now-implemented `/api/dashboard/terminals/{terminal_id}/turn-state` route
// on a REAL running daemon process, with owner auth ENABLED throughout.
//
// This is the half of the inherited constraint the rest of this diff did NOT
// close: every other new test either drives the route with a hand-built
// JSON POST (`routes.rs`'s `turn_state_route_*` tests,
// `terminal_notify_callback_restart.rs`'s `post_turn_state`) or drives the
// real CLI against a THROWAWAY mock listener that is never Phase 4's real
// route (`terminal_notify.rs`'s own existing test, whose header comment
// names this exact gap). A regression that changed the request shape on one
// side while each side's own isolated tests kept passing would not be
// caught by any of those.
//
// The CLI is silent-by-design regardless of outcome
// (`terminal_notify.rs`'s own module CONTRACT - deliberate, and this ticket
// must not remove it), so this test cannot use the CLI's exit code or
// stdio as a delivery signal, and Phase 4's own scope boundary is explicit
// that the route does not persist or broadcast the turn-state value
// anywhere observable (Phase 5's job) - so there is no application-level
// state this test could poll either. Instead it interposes a small
// transparent TCP relay between the real CLI and the real daemon: the CLI's
// `callback.json` fixture names the relay's own ephemeral port rather than
// the daemon's, so every byte the CLI sends is still genuinely handled by
// the daemon's real route (a real TCP hop, real router dispatch, real token
// check), and the relay tees the response bytes flowing back so THIS TEST -
// never the CLI - can observe the real "HTTP/1.1 204 No Content" status
// line the route actually returned.

use std::net::SocketAddr;
use std::path::PathBuf;
use std::process::Stdio;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
use tokio::net::{TcpListener, TcpStream};
use tokio::process::{Child, Command};
use tokio::time::timeout;

const STARTUP_TIMEOUT: Duration = Duration::from_secs(10);
const DELIVERY_TIMEOUT: Duration = Duration::from_secs(5);

static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

// CONTRACT (macOS Unix-domain-socket path-length ceiling - same root cause
// documented in `routes.rs::terminal_registry_temp_dir` and
// `terminal_lifetime.rs::temp_fixture_path`): the terminal helper's `.sock`
// path is derived from `WS_DASHBOARD_STATE_HOME`, so this fixture uses the
// same short-base-on-macOS scheme those files already established.
fn temp_fixture_path(name: &str) -> PathBuf {
    let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    #[cfg(target_os = "macos")]
    let base = PathBuf::from("/tmp");
    #[cfg(not(target_os = "macos"))]
    let base = std::env::temp_dir();
    base.join(format!(
        "ws-dashboard-terminal-notify-e2e-{name}-{}-{unique}",
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
        self.child.kill().await.expect("kill daemon process");
        let _ = self.child.wait().await;
    }
}

async fn spawn_real_daemon(state_home: &std::path::Path) -> DaemonProcess {
    let mut child = Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))
        .arg("serve")
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
// way.
fn http_client() -> reqwest::Client {
    reqwest::Client::builder()
        .redirect(reqwest::redirect::Policy::none())
        .build()
        .expect("build reqwest client")
}

async fn pair_and_get_cookie(client: &reqwest::Client, pairing_url: &str) -> String {
    let response = client.get(pairing_url).send().await.expect("pair request");
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
// command (Phase 2's own finding, already reused by
// `terminal_notify_callback_restart.rs` and `routes.rs`'s own
// `turn_state_route_*` tests for the same reason), so this costs nothing and
// avoids a real vendor-binary dependency. Unlike the restart test, this test
// never kills and re-spawns the daemon, so it does not need a stand-in
// `claude` binary that stays alive - the token/`callback.json` materialize
// synchronously inside `TerminalSession::spawn`, before this response is
// ever returned.
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
            "title": "terminal-notify e2e test terminal",
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

fn read_callback_json(path: &std::path::Path) -> serde_json::Value {
    let raw = std::fs::read_to_string(path)
        .unwrap_or_else(|error| panic!("read callback.json at {}: {error}", path.display()));
    serde_json::from_str(&raw)
        .unwrap_or_else(|error| panic!("parse callback.json at {}: {error}", path.display()))
}

/// A transparent per-connection TCP relay: accepts on `listener`, opens a
/// fresh connection to `upstream_addr` for each accepted client connection,
/// and copies bytes in both directions - except the upstream-to-client leg
/// is also teed into `captured` as it streams through, so a caller holding
/// the other side of `captured` can observe exactly what the real upstream
/// (the real daemon) sent back, without altering a single byte of what the
/// client actually receives.
async fn run_snooping_relay(listener: TcpListener, upstream_addr: SocketAddr, captured: Arc<Mutex<Vec<u8>>>) {
    loop {
        let Ok((client_stream, _)) = listener.accept().await else {
            continue;
        };
        let captured = captured.clone();
        tokio::spawn(async move {
            let Ok(upstream_stream) = TcpStream::connect(upstream_addr).await else {
                return;
            };
            let (mut client_read, mut client_write) = client_stream.into_split();
            let (mut upstream_read, mut upstream_write) = upstream_stream.into_split();

            let request_leg = tokio::spawn(async move {
                let _ = tokio::io::copy(&mut client_read, &mut upstream_write).await;
            });
            let response_leg = tokio::spawn(async move {
                let mut buf = [0u8; 4096];
                loop {
                    match upstream_read.read(&mut buf).await {
                        Ok(0) | Err(_) => break,
                        Ok(n) => {
                            captured
                                .lock()
                                .expect("captured buffer lock")
                                .extend_from_slice(&buf[..n]);
                            if client_write.write_all(&buf[..n]).await.is_err() {
                                break;
                            }
                        }
                    }
                }
            });
            let _ = tokio::join!(request_leg, response_leg);
        });
    }
}

#[tokio::test]
async fn terminal_notify_cli_delivers_a_real_turn_state_post_through_the_real_route() {
    let client = http_client();
    let state_home = temp_fixture_path("state");
    std::fs::create_dir_all(&state_home).expect("create state home dir");
    let work_root = temp_fixture_path("root");
    std::fs::create_dir_all(&work_root).expect("create work root dir");

    let daemon = spawn_real_daemon(&state_home).await;
    let cookie = pair_and_get_cookie(&client, &daemon.pairing_url).await;
    let work_root_id = open_work_root(&client, &daemon.base_url, &cookie, &work_root).await;
    let terminal_id = create_terminal_with_profile(&client, &daemon.base_url, &cookie, &work_root_id).await;

    let profile_dir = state_home.join("agent-profiles").join(&terminal_id);
    let callback_path = profile_dir.join("callback.json");
    assert!(
        callback_path.exists(),
        "a claude-profile terminal must materialize a callback.json"
    );
    let real_callback = read_callback_json(&callback_path);
    assert_eq!(real_callback["baseUrl"], daemon.base_url);
    assert_eq!(real_callback["terminalId"], terminal_id);
    let token = real_callback["token"]
        .as_str()
        .expect("callback.json token field")
        .to_owned();

    // The relay stands in for "the daemon's address" as far as the CLI's
    // fixture is concerned - every byte still reaches the real daemon (the
    // relay dials `daemon_addr` for each accepted connection), but the
    // relay's own copy of the response bytes is what this test asserts
    // against, since the CLI itself discards its own response by design.
    let daemon_addr: SocketAddr = daemon
        .base_url
        .strip_prefix("http://")
        .expect("base url has http scheme")
        .parse()
        .expect("parse daemon base url as a socket addr");
    let relay_listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind snooping relay");
    let relay_addr = relay_listener.local_addr().expect("relay local addr");
    let captured: Arc<Mutex<Vec<u8>>> = Arc::new(Mutex::new(Vec::new()));
    let relay_captured = captured.clone();
    tokio::spawn(async move {
        run_snooping_relay(relay_listener, daemon_addr, relay_captured).await;
    });

    // A fixture the real CLI resolves - same shape `write_callback_target`
    // produces (`baseUrl`/`terminalId`/`token`), except `baseUrl` names the
    // relay instead of the daemon directly, purely so this test can observe
    // the real route's real response without the CLI's own by-design
    // silence getting in the way.
    let fixture_dir = temp_fixture_path("cli-fixture");
    std::fs::create_dir_all(&fixture_dir).expect("create cli fixture dir");
    let fixture_callback_path = fixture_dir.join("callback.json");
    std::fs::write(
        &fixture_callback_path,
        serde_json::json!({
            "baseUrl": format!("http://{relay_addr}"),
            "terminalId": terminal_id,
            "token": token,
        })
        .to_string(),
    )
    .expect("write cli fixture callback.json");

    let status = Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))
        .arg("terminal-notify")
        .arg("--callback")
        .arg(&fixture_callback_path)
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
        "terminal-notify always exits 0 by design - this is NOT the delivery proof, see below"
    );

    // THE delivery proof: poll the relay's captured bytes for the real
    // route's real response status line. Nothing else in this diff can
    // distinguish "the real CLI delivered a POST the real route accepted"
    // from "silently dropped somewhere in between" - that is exactly the
    // gap review finding D named.
    let poll_result = timeout(DELIVERY_TIMEOUT, async {
        loop {
            let seen = captured.lock().expect("captured buffer lock").clone();
            if String::from_utf8_lossy(&seen).contains("204") {
                return;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
    })
    .await;
    let snapshot = String::from_utf8_lossy(&captured.lock().expect("captured buffer lock")).into_owned();
    assert!(
        poll_result.is_ok(),
        "the real terminal-notify CLI must deliver a turn-state POST that the real route \
         accepts (204 No Content) within {DELIVERY_TIMEOUT:?} - relay captured so far: {snapshot:?}"
    );
    assert!(
        snapshot.starts_with("HTTP/1.1 204"),
        "expected the real route's response status line to open with \"HTTP/1.1 204\", got: {snapshot:?}"
    );

    daemon.kill_hard().await;
    let _ = std::fs::remove_dir_all(&state_home);
    let _ = std::fs::remove_dir_all(&work_root);
    let _ = std::fs::remove_dir_all(&fixture_dir);
}
