// CONTRACT (ticket 260724-chore-dashboard-windows-terminal-reaper-native-
// acceptance): runtime-verifies the `#[cfg(windows)]` dead-shell reaper
// (`spawn_process_exit_reaper` in `terminal_helper_process.rs`) actually
// wakes on a real Windows host, not merely compiles for one.
//
// Scenario: spawn the real per-terminal helper subprocess through the same
// entry point the daemon uses (`ws-dashboard terminal-helper ...`), drive
// its IPC handshake so it spawns a real ConPTY-backed shell, then kill that
// shell's OS process **out-of-band** - directly via `taskkill`, never
// through the crate's own `kill_shell_if_running`/`child.kill()` path - so
// the PTY master never observes EOF and the ring is never pre-stamped
// `Terminated`. Only the reaper's own `wait_for_process_exit` wake can flip
// the ring to `Exited` afterward; asserting that transition arrives over
// IPC is what proves the reaper, not some other exit-detection path, did
// the work.
//
// Dedicated `tests/*.rs` integration test, deliberately NOT a co-located
// `#[cfg(test)] mod` inside `terminal_helper_process.rs`: `spawn_shell`
// assigns the CALLING process into a `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`
// job (`terminal_platform::windows::create_kill_on_close_job`), and
// dropping/closing that job handle tears down every process still assigned
// to it. Calling `spawn_shell` in-process from `cargo test` itself would
// assign the whole test binary process into that job, and later closing the
// job would kill the entire test binary (including any other tests running
// concurrently in the same process) - not just the shell. Spawning the real
// helper as a separate OS subprocess (this file) confines that blast radius
// exactly the way production does.
//
// Windows-only for the whole file: nothing here is meaningful on Unix (no
// reaper thread exists there - see `spawn_process_exit_reaper`'s own
// `#[cfg(windows)]` gate), and `#![cfg(windows)]` as the file's first line
// excludes the entire compiled test binary on Linux/macOS rather than
// requiring per-item `#[cfg(windows)]` annotations throughout.
//
// NON-VACUITY PROOF (manual, execution-time only - not automated here,
// never committed as a source change): on a real Windows host, temporarily
// comment out the single `shared.transition(TerminalHelperStatus::Exited)`
// call inside `spawn_process_exit_reaper`
// (`crates/daemon/src/terminal_helper_process.rs`, the line right after
// `wait_for_process_exit(&handle)` returns), rebuild, rerun this test with
// `cargo test -p ws-dashboard-daemon terminal_windows_reaper_acceptance` and
// confirm it hangs to the bounded timeout below and fails; then revert the
// edit, rebuild, rerun, and confirm it passes again. This is what proves the
// assertion below is actually pinned on the reaper's wake, not on some
// other, coincidentally-satisfied path.
#![cfg(windows)]

use std::path::PathBuf;
use std::process::Stdio;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

use tokio::process::Command;
use tokio::time::{timeout, Instant};

use ws_dashboard_daemon::terminal_helper_ipc::{write_ndjson, NdjsonReader};
use ws_dashboard_daemon::terminal_helper_protocol::{
    DaemonToHelperMessage, HelperToDaemonMessage, TerminalHelperStatus,
};
use ws_dashboard_daemon::terminal_ipc_transport;

// Generous on purpose - real OS process spawn plus a real interactive-shell
// startup, consistent with the connect-timeout idioms already used by
// `terminal.rs::connect_and_handshake` and the sibling Unix integration
// tests in `terminal_lifetime.rs`.
const CONNECT_TIMEOUT: Duration = Duration::from_secs(10);
// Bounded wait for the reaper's wake after the out-of-band kill. Generous
// relative to `wait_for_process_exit`'s kernel-level wait (near-instant on a
// real handle signal), so this only ever times out if the reaper genuinely
// never wakes - never from ordinary scheduling jitter.
const EXIT_WAIT_TIMEOUT: Duration = Duration::from_secs(15);
// Polling budget for discovering the shell's real child PID via
// `Get-CimInstance` after sending `HandshakeAck` - the shell spawns
// asynchronously inside the helper, so a handful of short retries covers
// ordinary scheduling delay without materially slowing the test down.
const CHILD_DISCOVERY_ATTEMPTS: u32 = 40;
const CHILD_DISCOVERY_INTERVAL: Duration = Duration::from_millis(50);

static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

fn temp_fixture_path(name: &str) -> PathBuf {
    let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    std::env::temp_dir().join(format!(
        "ws-dashboard-terminal-windows-reaper-{name}-{}-{unique}",
        std::process::id()
    ))
}

/// Leak-safe cleanup for the temp registry/cwd dirs, mirroring
/// `terminal_lifetime.rs`'s `HelperReaper` Drop-guard shape. Declared BEFORE
/// the helper `Child` in the test body so it drops LAST (Rust drops locals
/// in reverse declaration order) - i.e. only after the helper subprocess
/// itself has already been torn down by `Child`'s own `kill_on_drop`, on
/// every exit path including a panicking assertion.
struct TempDirGuard {
    registry_dir: PathBuf,
    cwd: PathBuf,
}

impl Drop for TempDirGuard {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.registry_dir);
        let _ = std::fs::remove_dir_all(&self.cwd);
    }
}

/// Retry-connect-with-deadline, mirroring `terminal.rs::connect_and_
/// handshake`'s idiom: the helper's IPC listener bind is not synchronous
/// with the OS-level `Command::spawn()` returning, so the first few connect
/// attempts are expected to fail while the helper is still starting up.
async fn connect_with_retry(
    socket_path: &std::path::Path,
    deadline: Instant,
) -> ws_dashboard_daemon::terminal_ipc_transport::BoxedIpcStream {
    loop {
        match terminal_ipc_transport::connect(socket_path).await {
            Ok(stream) => return stream,
            Err(error) => {
                if Instant::now() >= deadline {
                    panic!("failed to connect to helper IPC transport before deadline: {error}");
                }
                tokio::time::sleep(Duration::from_millis(20)).await;
            }
        }
    }
}

/// Shell executables `select_terminal_shell` (`terminal.rs`) can pick on
/// Windows, in the order it tries them: `pwsh.exe` if on PATH, else
/// `powershell.exe` if on PATH, else the `COMSPEC`/fallback `cmd.exe`.
const KNOWN_SHELL_EXECUTABLES: [&str; 3] = ["pwsh.exe", "powershell.exe", "cmd.exe"];

/// Discovers the shell's real OS child PID with no new Cargo dependency, by
/// polling `Get-CimInstance Win32_Process -Filter "ParentProcessId=<helper
/// pid>"` via `powershell.exe`. No message in the helper<->daemon IPC
/// protocol ever carries the shell's own PID (only the helper's), so this
/// indirection is required - and it mirrors how a genuine external actor
/// (Task Manager, an OOM killer, a human running `taskkill`) would identify
/// the process, which is exactly the out-of-band kill this test performs
/// next.
///
/// CONTRACT (discovered empirically running this test on real Windows -
/// portable-pty's `CreatePseudoConsole` spawns the shell as a direct child
/// with no intermediary, per the plan's Codebase Findings, but that is not
/// the ONLY direct child of the helper): modern Windows additionally spawns
/// a ConPTY host process (`OpenConsole.exe`, the Windows Terminal successor
/// to `conhost.exe`) as a SIBLING direct child of the helper to back the
/// pseudo console. `ParentProcessId=<helper pid>` therefore legitimately
/// returns two children in the common case, not one. Filtering to
/// `KNOWN_SHELL_EXECUTABLES` - exactly the executables `select_terminal_
/// shell` itself can choose - is what the plan's own Codebase Findings
/// anticipated as the fallback for "more than one child process ever shows
/// up"; this is that fallback, now confirmed necessary rather than merely
/// theoretical.
async fn discover_shell_pid(helper_pid: u32) -> u32 {
    let mut last_failure: Option<String> = None;
    let mut last_seen: Vec<(u32, String)> = Vec::new();
    for _ in 0..CHILD_DISCOVERY_ATTEMPTS {
        let output = Command::new("powershell.exe")
            .args([
                "-NoProfile",
                "-Command",
                &format!(
                    "(Get-CimInstance Win32_Process -Filter \"ParentProcessId={helper_pid}\") \
                     | Select-Object ProcessId,Name | ConvertTo-Csv -NoTypeInformation"
                ),
            ])
            .stdin(Stdio::null())
            .output()
            .await
            .expect("spawn powershell.exe to discover the shell's child pid");

        if !output.status.success() {
            last_failure = Some(format!(
                "powershell exited with {}: stdout={} stderr={}",
                output.status,
                String::from_utf8_lossy(&output.stdout),
                String::from_utf8_lossy(&output.stderr)
            ));
            tokio::time::sleep(CHILD_DISCOVERY_INTERVAL).await;
            continue;
        }

        // `ConvertTo-Csv -NoTypeInformation` on Windows PowerShell emits a
        // `"ProcessId","Name"` header line followed by one double-quoted,
        // comma-separated line per process - skip the header, then split
        // each remaining line on the first comma and strip quotes.
        let stdout = String::from_utf8_lossy(&output.stdout);
        let children: Vec<(u32, String)> = stdout
            .lines()
            .map(str::trim)
            .filter(|line| !line.is_empty())
            .skip(1)
            .filter_map(|line| {
                let mut fields = line.splitn(2, ',');
                let pid = fields.next()?.trim_matches('"').parse::<u32>().ok()?;
                let name = fields.next()?.trim_matches('"').to_owned();
                Some((pid, name))
            })
            .collect();

        let shell_children: Vec<&(u32, String)> = children
            .iter()
            .filter(|(_, name)| {
                KNOWN_SHELL_EXECUTABLES
                    .iter()
                    .any(|candidate| name.eq_ignore_ascii_case(candidate))
            })
            .collect();
        last_seen = children;

        match shell_children.as_slice() {
            [] => {
                tokio::time::sleep(CHILD_DISCOVERY_INTERVAL).await;
                continue;
            }
            [(pid, _)] => return *pid,
            multiple => panic!(
                "expected exactly one shell-named child (one of {KNOWN_SHELL_EXECUTABLES:?}) of \
                 helper pid {helper_pid}, found {}: {:?}; all observed direct children were {:?}",
                multiple.len(),
                multiple,
                last_seen
            ),
        }
    }
    panic!(
        "no shell child (one of {KNOWN_SHELL_EXECUTABLES:?}) of helper pid {helper_pid} appeared \
         within {:?} ({CHILD_DISCOVERY_ATTEMPTS} attempts, {CHILD_DISCOVERY_INTERVAL:?} apart); \
         last powershell failure: {last_failure:?}; last observed direct children: {last_seen:?}",
        CHILD_DISCOVERY_INTERVAL * CHILD_DISCOVERY_ATTEMPTS
    );
}

#[tokio::test]
async fn dead_shell_out_of_band_kill_wakes_windows_reaper_to_exited() {
    let registry_dir = temp_fixture_path("registry");
    let cwd = temp_fixture_path("cwd");
    std::fs::create_dir_all(&registry_dir).expect("create temp registry dir");
    std::fs::create_dir_all(&cwd).expect("create temp cwd dir");

    // Declared before `child` so it drops LAST - see `TempDirGuard`'s doc
    // comment. Runs on every exit path, including a panicking assertion.
    let _cleanup = TempDirGuard {
        registry_dir: registry_dir.clone(),
        cwd: cwd.clone(),
    };

    let terminal_id = format!(
        "windows-reaper-acceptance-{}-{}",
        std::process::id(),
        TEMP_COUNTER.fetch_add(1, Ordering::Relaxed)
    );
    let socket_path = registry_dir.join(format!("{terminal_id}.sock"));

    // Mirrors `terminal.rs::TerminalSession::spawn`'s arg shape exactly,
    // minus the `terminal_platform::spawn_detached` wrapper (orthogonal to
    // the reaper under test - see the plan's Implementation Plan step 2).
    // Spawned as a directly-tracked tokio `Child` (not detached), so its
    // `kill_on_drop(true)` gives this test a leak-safe, identity-exact
    // cleanup handle for the helper subprocess itself - stronger than a
    // rediscovered bare PID, since it is the actual OS process handle this
    // test spawned.
    let mut child = Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))
        .arg("terminal-helper")
        .arg("--registry-dir")
        .arg(&registry_dir)
        .arg("--terminal-id")
        .arg(&terminal_id)
        .arg("--work-root-id")
        .arg("windows-reaper-acceptance-work-root")
        .arg("--cwd")
        .arg(&cwd)
        .arg("--title")
        .arg("windows-reaper-acceptance")
        .arg("--columns")
        .arg("80")
        .arg("--rows")
        .arg("24")
        .arg("--socket-path")
        .arg(&socket_path)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .kill_on_drop(true)
        .spawn()
        .expect("spawn real ws-dashboard terminal-helper subprocess");

    // Connect and read the Handshake (carries the helper's own real PID -
    // never the shell's; see `terminal_helper_protocol.rs`). Deliberately
    // does NOT wait for the helper's next, proactive `Status{Running}`
    // message: that message reflects the ring's optimistic default sent
    // before the shell has actually spawned, so it is not a "shell is up"
    // signal and is safely left unread here (it will simply be skipped over
    // by the drain loop below).
    let connect_deadline = Instant::now() + CONNECT_TIMEOUT;
    let stream = connect_with_retry(&socket_path, connect_deadline).await;
    let (read_half, mut write_half) = terminal_ipc_transport::split(stream);
    let mut reader = NdjsonReader::new(read_half);

    let handshake = timeout(
        connect_deadline.saturating_duration_since(Instant::now()),
        reader.read_message::<HelperToDaemonMessage>(),
    )
    .await
    .expect("read helper handshake before the connect deadline")
    .expect("read helper handshake message without a transport error")
    .expect("helper IPC connection closed before sending its handshake");
    let HelperToDaemonMessage::Handshake {
        pid: helper_pid, ..
    } = handshake
    else {
        panic!("expected Handshake as the helper's first IPC message, got {handshake:?}");
    };

    // Triggers `spawn_shell` on the helper side (see
    // `handle_connection`'s `HandshakeAck` arm).
    write_ndjson(&mut write_half, &DaemonToHelperMessage::HandshakeAck)
        .await
        .expect("send HandshakeAck to trigger the helper's real shell spawn");

    // Discover the shell's real OS PID (no IPC message ever carries it) and
    // kill it OUT-OF-BAND - directly via `taskkill`, entirely outside the
    // crate's own `kill_shell_if_running`/`child.kill()` path. This never
    // pre-stamps the ring `Terminated`, so it stays `Running` until only
    // the reaper's wake (`spawn_process_exit_reaper` ->
    // `wait_for_process_exit` -> `transition(Exited)`) can flip it - the
    // exact non-vacuity property this test is pinned on.
    let shell_pid = discover_shell_pid(helper_pid).await;
    let kill_output = Command::new("taskkill")
        .args(["/F", "/PID", &shell_pid.to_string()])
        .stdin(Stdio::null())
        .output()
        .await
        .expect("run taskkill to terminate the shell out-of-band");
    assert!(
        kill_output.status.success(),
        "taskkill /F /PID {shell_pid} failed: status={:?} stdout={} stderr={}",
        kill_output.status,
        String::from_utf8_lossy(&kill_output.stdout),
        String::from_utf8_lossy(&kill_output.stderr)
    );

    // Drain NDJSON messages under a bounded deadline until the reaper's wake
    // reports `Exit{status: Exited, ..}` - per `handle_connection`'s
    // `notify.notified()` arm, a `Running -> Exited` transition is ALWAYS
    // reported via the `Exit` variant, never a bare `Status`. Any other
    // message (the initial `Status{Running}`, shell-startup `Output`
    // chunks) is skipped; any `Exit` with a status other than `Exited`
    // fails immediately (pinning the exact path being guarded, not merely
    // "non-running") rather than waiting out the rest of the deadline.
    let wait_for_exit = async {
        loop {
            match reader.read_message::<HelperToDaemonMessage>().await {
                Ok(Some(HelperToDaemonMessage::Exit { status, .. })) => return status,
                Ok(Some(_other)) => continue,
                Ok(None) => panic!(
                    "helper IPC connection closed before reporting an exit status \
                     (reaper wake was never observed)"
                ),
                Err(error) => panic!("helper IPC read error while waiting for exit: {error}"),
            }
        }
    };
    let status = timeout(EXIT_WAIT_TIMEOUT, wait_for_exit)
        .await
        .unwrap_or_else(|_| {
            panic!(
                "windows dead-shell reaper did not wake within {EXIT_WAIT_TIMEOUT:?}: the \
                 out-of-band-killed shell's exit was never observed over IPC"
            )
        });
    assert_eq!(
        status,
        TerminalHelperStatus::Exited,
        "expected the reaper to report Exited after an out-of-band shell kill, got {status:?}"
    );

    let _ = child.kill().await;
}
