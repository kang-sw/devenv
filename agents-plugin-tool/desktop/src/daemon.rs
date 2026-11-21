use std::{
    cmp::Ordering,
    io::Write,
    path::{Path, PathBuf},
    process::{Command, Stdio},
    thread,
    time::{Duration, Instant},
};

use serde_json::Value;

use crate::{
    daemon_package::Candidate,
    endpoint::{self, Daemon},
    python,
};

const PYTHON_REQUIRED: &str = "Python 3 is required";
const STAGING_FAILED: &str = "Could not prepare the replacement daemon. Retry later.";
const VERSION_UNKNOWN: &str =
    "Cannot determine the daemon version. Update the plugin before retrying.";
const NEWER_RUNNING: &str =
    "The running daemon is newer than this app. Update Werkschaft before retrying.";
const REPLACEMENT_FAILED: &str =
    "No healthy replacement was found. Keeping the current connection.";

#[derive(Clone, Copy, PartialEq, Eq)]
pub enum Intent {
    Connect,
    Recover,
    Replace,
}

pub struct ConnectionError {
    pub reason: &'static str,
    pub python_required: bool,
}

impl From<&'static str> for ConnectionError {
    fn from(reason: &'static str) -> Self {
        Self {
            reason,
            python_required: false,
        }
    }
}

pub struct ConnectionResult {
    pub daemon: Daemon,
    pub refusal: Option<&'static str>,
}

impl From<Daemon> for ConnectionResult {
    fn from(daemon: Daemon) -> Self {
        Self {
            daemon,
            refusal: None,
        }
    }
}

fn refuse(daemon: Daemon, reason: &'static str) -> ConnectionResult {
    ConnectionResult {
        daemon,
        refusal: Some(reason),
    }
}

pub(crate) fn version_core(version: &str) -> Option<[u64; 3]> {
    let version = version.strip_prefix('v').unwrap_or(version);
    let core = match version.split_once('-') {
        Some((core, suffix))
            if !suffix.is_empty()
                && suffix
                    .bytes()
                    .all(|byte| byte.is_ascii_alphanumeric() || b".-".contains(&byte)) =>
        {
            core
        }
        Some(_) => return None,
        None => version,
    };
    let mut parts = core.split('.');
    let mut numeric = [0; 3];
    for number in &mut numeric {
        let part = parts.next()?;
        if part.is_empty() || !part.bytes().all(|byte| byte.is_ascii_digit()) {
            return None;
        }
        *number = part.parse().ok()?;
        if *number > (1 << 53) - 1 {
            return None;
        }
    }
    parts.next().is_none().then_some(numeric)
}

fn replacement(candidate: [u64; 3], running: &str, intent: Intent) -> Result<bool, &'static str> {
    match candidate.cmp(&version_core(running).ok_or(VERSION_UNKNOWN)?) {
        Ordering::Greater => Ok(true),
        Ordering::Equal => Ok(intent == Intent::Replace),
        Ordering::Less if intent == Intent::Replace => Err(NEWER_RUNNING),
        Ordering::Less => Ok(false),
    }
}

pub fn connect(
    resource_root: Result<PathBuf, &'static str>,
    user_data: Result<PathBuf, &'static str>,
    intent: Intent,
) -> Result<ConnectionResult, ConnectionError> {
    let cache = endpoint::active_cache_root()?;
    let find = || endpoint::discover_at(&cache).ok();
    let running = find();
    if intent == Intent::Recover {
        if let Some(daemon) = running {
            return Ok(daemon.into());
        }
    }
    let selected = resource_root
        .as_ref()
        .map_err(|error| *error)
        .and_then(|root| {
            let development = std::env::var_os("WERKSCHAFT_DAEMON_ROOT")
                .filter(|path| !path.is_empty())
                .map(PathBuf::from);
            Candidate::select(&root.join("daemon"), development.as_deref())
        });
    let selected = match (selected, running.as_ref()) {
        (Ok(candidate), _) => candidate,
        (Err(_), Some(daemon)) => {
            return Ok(if intent == Intent::Replace {
                refuse(
                    daemon.clone(),
                    "Cannot select the replacement daemon. Keeping the current connection.",
                )
            } else {
                daemon.clone().into()
            });
        }
        (Err(reason), None) => {
            if !python::available() {
                return Err(ConnectionError {
                    reason: PYTHON_REQUIRED,
                    python_required: true,
                });
            }
            return Err(reason.into());
        }
    };
    let identity = if let Some(daemon) = &running {
        match replacement(selected.version, &daemon.version, intent) {
            Ok(false) => return Ok(daemon.clone().into()),
            Err(reason) => return Ok(refuse(daemon.clone(), reason)),
            Ok(true) => match daemon.predecessor() {
                Ok(identity) => identity,
                Err(_) => {
                    return Ok(refuse(
                        daemon.clone(),
                        "Cannot authenticate the replacement. Keeping the current connection.",
                    ));
                }
            },
        }
    } else {
        Value::Null
    };
    if !python::available() {
        if let Some(daemon) = running {
            return Ok(refuse(
                daemon,
                "Python 3 is required to replace the daemon. Keeping the current connection.",
            ));
        }
        return Err(ConnectionError {
            reason: PYTHON_REQUIRED,
            python_required: true,
        });
    }
    let staged =
        user_data.and_then(|data| selected.stage(&data, package_boundary(&resource_root?)));
    let staged = match staged {
        Ok(candidate) => candidate,
        Err(_) if running.is_some() => return Ok(refuse(running.unwrap(), STAGING_FAILED)),
        Err(reason) => return Err(reason.into()),
    };
    let current = find();
    if current != running {
        return match current {
            Some(daemon) => attach_winner(daemon, running.as_ref(), selected.version),
            None => {
                Err("Dashboard health became uncertain during preparation. Retry later.".into())
            }
        };
    }
    let started = launch(&staged, &cache, &identity, intent == Intent::Replace);
    let settle = Instant::now() + Duration::from_secs(15);
    loop {
        if let Some(daemon) = find() {
            if Some(&daemon) == running.as_ref() {
                return Ok(refuse(daemon, REPLACEMENT_FAILED));
            }
            return attach_winner(daemon, running.as_ref(), selected.version);
        }
        if Instant::now() >= settle {
            let reason = match (running.is_some(), started) {
                (true, _) => "No healthy daemon was found after replacement. Retry to reconnect.",
                (false, true) => "Dashboard did not become healthy. Retry to reconnect.",
                (false, false) => "Could not start the dashboard daemon. Retry to reconnect.",
            };
            return Err(reason.into());
        }
        thread::sleep(Duration::from_millis(200));
    }
}

fn package_boundary(resource_root: &Path) -> &Path {
    if cfg!(target_os = "macos") && resource_root.ends_with("Contents/Resources") {
        if let Some(app) = resource_root.ancestors().nth(2) {
            if app.extension().is_some_and(|extension| extension == "app") {
                return app;
            }
        }
    }
    resource_root
}

fn attach_winner(
    daemon: Daemon,
    running: Option<&Daemon>,
    candidate: [u64; 3],
) -> Result<ConnectionResult, ConnectionError> {
    let Some(predecessor) = running else {
        return Ok(daemon.into());
    };
    if let Some(core) = version_core(&daemon.version) {
        if core >= candidate {
            return Ok(daemon.into());
        }
        if version_core(&predecessor.version).is_some_and(|previous| core >= previous) {
            return Ok(refuse(
                daemon,
                "Another daemon is serving an older core than this app. Attaching to it.",
            ));
        }
    }
    Err("An older or unidentified daemon won the handoff. Retry to reconnect.".into())
}

fn launch(candidate: &Candidate, cache: &Path, identity: &Value, replace: bool) -> bool {
    let mut command = Command::new(&candidate.binary);
    command.current_dir(&candidate.root);
    for key in [
        "WS_HOSTED_SESSION",
        "WS_HOSTED_SESSION_KEY",
        "WS_HOSTED_SESSION_ROW",
        "WS_MCP_HARNESS_PID",
        "WS_MCP_PROJECT_ROOT",
        "WS_MCP_DASHBOARD_DEV_SOURCE",
        "WS_RSRC_DEV_CHECKOUT",
    ] {
        command.env_remove(key);
    }
    let mut manifests = std::ffi::OsString::new();
    for path in &candidate.config_manifests {
        if !manifests.is_empty() {
            manifests.push(";");
        }
        manifests.push(path);
    }
    command
        .env("WS_CACHE_HOME", cache)
        .env("WS_RSRC_ROOT", candidate.root.join("rsrc"))
        .env("WS_SKILLS_ROOT", candidate.root.join("skills"))
        .env("WS_MCP_CONFIG_MANIFESTS", manifests)
        .env("WS_MCP_RUNTIME_BINARY", &candidate.binary)
        .env("WS_MCP_RUNTIME_DIR", &candidate.root)
        .env("CLAUDE_PLUGIN_ROOT", &candidate.root)
        .env("WS_MCP_DAEMON_JOB_LIFETIME", "0")
        .args(["daemon-start", "--desktop-start", "--start-wait", "13s"])
        .stdin(Stdio::piped())
        .stdout(Stdio::null());
    if replace && !identity.is_null() {
        command.arg("--replace");
    }
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        const CREATE_NO_WINDOW: u32 = 0x0800_0000;
        command
            .creation_flags(CREATE_NO_WINDOW)
            .stderr(Stdio::piped());
        command.arg("--require-independent-lifetime");
    }
    #[cfg(not(windows))]
    command.stderr(Stdio::null());
    let input = identity.to_string();
    let (success, stderr) = run_starter(&mut command, input.as_bytes());
    if cfg!(windows) && !success && breakaway_denied(&stderr) {
        eprintln!(
            "Werkschaft: Independent daemon launch denied; retrying within the containing job"
        );
        command.env("WS_MCP_DAEMON_JOB_LIFETIME", "1");
        return run_starter(&mut command, input.as_bytes()).0;
    }
    success
}

fn breakaway_denied(stderr: &[u8]) -> bool {
    stderr
        .split(|byte| *byte == b'\n')
        .any(|line| line.strip_suffix(b"\r").unwrap_or(line) == b"WS_DAEMON_BREAKAWAY_DENIED=5")
}

fn run_starter(command: &mut Command, input: &[u8]) -> (bool, Vec<u8>) {
    let Ok(mut child) = command.spawn() else {
        return (false, Vec::new());
    };
    let output = child.stderr.take().map(|stderr| {
        use std::io::Read;
        let (send, receive) = std::sync::mpsc::channel();
        thread::spawn(move || {
            let mut bytes = Vec::new();
            let _ = stderr.take(16 * 1024).read_to_end(&mut bytes);
            let _ = send.send(bytes);
        });
        receive
    });
    let wrote_identity = child
        .stdin
        .take()
        .is_some_and(|mut stdin| stdin.write_all(input).is_ok());
    // The native starter's own budget is 2*DefaultStartWait + 13 s.
    let deadline = Instant::now() + Duration::from_secs(42);
    while wrote_identity && Instant::now() < deadline {
        match child.try_wait() {
            Ok(Some(status)) => {
                let bytes = output
                    .and_then(|rx| rx.recv_timeout(Duration::from_millis(200)).ok())
                    .unwrap_or_default();
                return (status.success(), bytes);
            }
            Ok(None) => thread::sleep(Duration::from_millis(50)),
            Err(_) => break,
        }
    }
    let _ = child.kill();
    let _ = child.wait();
    (false, Vec::new())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn numeric_core_controls_automatic_and_explicit_replacement() {
        let candidate = [0, 50, 8];
        for (running, intent, want) in [
            ("0.49.99", Intent::Connect, Ok(true)),
            ("0.50.7", Intent::Connect, Ok(true)),
            ("v0.50.8-canary.1", Intent::Connect, Ok(false)),
            ("0.50.8-dev", Intent::Replace, Ok(true)),
            ("0.51.0", Intent::Connect, Ok(false)),
            ("1.0.0", Intent::Replace, Err(NEWER_RUNNING)),
            ("garbage", Intent::Connect, Err(VERSION_UNKNOWN)),
        ] {
            assert_eq!(replacement(candidate, running, intent), want, "{running}");
        }
        for invalid in [
            "0.50",
            "0.50.8-",
            "0.50.+8",
            "0.50.8/extra",
            "0.50.9007199254740992",
        ] {
            assert!(version_core(invalid).is_none(), "{invalid}");
        }
    }

    #[test]
    fn only_the_exact_os_signal_permits_a_job_lifetime_retry() {
        assert!(breakaway_denied(b"log\r\nWS_DAEMON_BREAKAWAY_DENIED=5\r\n"));
        assert!(!breakaway_denied(b"WS_DAEMON_BREAKAWAY_DENIED=50\n"));
        assert!(!breakaway_denied(
            b"spawn error: WS_DAEMON_BREAKAWAY_DENIED=5\n"
        ));
        assert!(!breakaway_denied(b"Access denied\n"));
    }
}
