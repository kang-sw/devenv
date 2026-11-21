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
const HEALTH_UNCERTAIN: &str = "Could not recheck the daemon after staging. Retry later.";

#[derive(Clone, Copy, PartialEq, Eq)]
pub enum Intent {
    Connect,
    Recover,
    Replace,
}

#[derive(Debug)]
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
    connect_with(
        &NativeLifecycle {
            cache: endpoint::active_cache_root()?,
            resource_root,
            user_data,
        },
        intent,
    )
}

trait Lifecycle {
    type Package;
    fn discover(&self) -> Option<Daemon>;
    fn select(&self) -> Result<(Self::Package, [u64; 3]), &'static str>;
    fn stage(&self, package: &Self::Package) -> Result<Self::Package, &'static str>;
    fn python_available(&self) -> bool;
    fn still_healthy(&self, daemon: &Daemon) -> bool;
    fn launch(&self, package: &Self::Package, identity: &Value, replace: bool) -> bool;
    fn settle(&self) -> Option<Daemon>;
}

struct NativeLifecycle {
    cache: PathBuf,
    resource_root: Result<PathBuf, &'static str>,
    user_data: Result<PathBuf, &'static str>,
}

impl Lifecycle for NativeLifecycle {
    type Package = Candidate;

    fn discover(&self) -> Option<Daemon> {
        endpoint::discover_at(&self.cache).ok()
    }

    fn select(&self) -> Result<(Candidate, [u64; 3]), &'static str> {
        let root = self.resource_root.as_ref().map_err(|error| *error)?;
        let development = std::env::var_os("WERKSCHAFT_DAEMON_ROOT")
            .filter(|path| !path.is_empty())
            .map(PathBuf::from);
        let candidate = Candidate::select(&root.join("daemon"), development.as_deref())?;
        let core = candidate.version;
        Ok((candidate, core))
    }

    fn stage(&self, candidate: &Candidate) -> Result<Candidate, &'static str> {
        let data = self.user_data.as_ref().map_err(|error| *error)?;
        let resources = self.resource_root.as_ref().map_err(|error| *error)?;
        candidate.stage(data, package_boundary(resources))
    }

    fn python_available(&self) -> bool {
        python::available()
    }

    fn still_healthy(&self, daemon: &Daemon) -> bool {
        daemon.is_healthy()
    }

    fn launch(&self, package: &Candidate, identity: &Value, replace: bool) -> bool {
        launch(package, &self.cache, identity, replace)
    }

    fn settle(&self) -> Option<Daemon> {
        let deadline = Instant::now() + Duration::from_secs(15);
        loop {
            let daemon = self.discover();
            if daemon.is_some() || Instant::now() >= deadline {
                return daemon;
            }
            thread::sleep(Duration::from_millis(200));
        }
    }
}

fn connect_with(
    lifecycle: &impl Lifecycle,
    intent: Intent,
) -> Result<ConnectionResult, ConnectionError> {
    let running = lifecycle.discover();
    if intent == Intent::Recover {
        if let Some(daemon) = running {
            return Ok(daemon.into());
        }
    }
    if running.is_none() && !lifecycle.python_available() {
        return Err(ConnectionError {
            reason: PYTHON_REQUIRED,
            python_required: true,
        });
    }
    let (selected, core) = match (lifecycle.select(), running.as_ref()) {
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
        (Err(reason), None) => return Err(reason.into()),
    };
    let identity = if let Some(daemon) = &running {
        match replacement(core, &daemon.version, intent) {
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
    if let Some(daemon) = &running {
        if !lifecycle.python_available() {
            return Ok(refuse(
                daemon.clone(),
                "Python 3 is required to replace the daemon. Keeping the current connection.",
            ));
        }
    }
    let staged = match lifecycle.stage(&selected) {
        Ok(candidate) => candidate,
        Err(_) if running.is_some() => return Ok(refuse(running.unwrap(), STAGING_FAILED)),
        Err(reason) => return Err(reason.into()),
    };
    let current = lifecycle.discover();
    if current != running {
        return match current {
            Some(daemon) => attach_winner(daemon, running.as_ref(), core),
            None => running
                .filter(|daemon| lifecycle.still_healthy(daemon))
                .map(|daemon| refuse(daemon, HEALTH_UNCERTAIN))
                .ok_or(HEALTH_UNCERTAIN.into()),
        };
    }
    let started = lifecycle.launch(&staged, &identity, intent == Intent::Replace);
    match lifecycle.settle() {
        Some(daemon) if Some(&daemon) == running.as_ref() => Ok(refuse(daemon, REPLACEMENT_FAILED)),
        Some(daemon) => attach_winner(daemon, running.as_ref(), core),
        None if running.is_some() => {
            Err("No healthy daemon was found after replacement. Retry to reconnect.".into())
        }
        None if started => Err("Dashboard did not become healthy. Retry to reconnect.".into()),
        None => Err("Could not start the dashboard daemon. Retry to reconnect.".into()),
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
        eprintln!("Werkschaft: WS_DAEMON_BREAKAWAY_DENIED=5. Retrying within the containing job.");
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
    use std::{cell::RefCell, collections::VecDeque};

    struct Fixture {
        observations: RefCell<VecDeque<Option<Daemon>>>,
        settled: Option<Daemon>,
        python: bool,
        healthy: bool,
        stage_fails: bool,
        start_succeeds: bool,
        actions: RefCell<Vec<&'static str>>,
        start: RefCell<Option<(Value, bool)>>,
    }

    impl Fixture {
        fn with(
            before: Option<Daemon>,
            after_stage: Option<Daemon>,
            settled: Option<Daemon>,
        ) -> Self {
            Self {
                observations: RefCell::new([before, after_stage].into()),
                settled,
                python: true,
                healthy: true,
                stage_fails: false,
                start_succeeds: true,
                actions: RefCell::default(),
                start: RefCell::default(),
            }
        }
    }

    impl Lifecycle for Fixture {
        type Package = ();

        fn discover(&self) -> Option<Daemon> {
            self.observations
                .borrow_mut()
                .pop_front()
                .expect("unexpected discovery")
        }
        fn select(&self) -> Result<((), [u64; 3]), &'static str> {
            self.actions.borrow_mut().push("select");
            Ok(((), [0, 50, 8]))
        }
        fn stage(&self, _: &()) -> Result<(), &'static str> {
            self.actions.borrow_mut().push("stage");
            if self.stage_fails {
                Err("staging failed")
            } else {
                Ok(())
            }
        }
        fn python_available(&self) -> bool {
            self.actions.borrow_mut().push("python");
            self.python
        }
        fn still_healthy(&self, _: &Daemon) -> bool {
            self.actions.borrow_mut().push("health");
            self.healthy
        }
        fn launch(&self, _: &(), identity: &Value, replace: bool) -> bool {
            self.actions.borrow_mut().push("launch");
            *self.start.borrow_mut() = Some((identity.clone(), replace));
            self.start_succeeds
        }
        fn settle(&self) -> Option<Daemon> {
            self.actions.borrow_mut().push("settle");
            self.settled.clone()
        }
    }

    #[test]
    fn missing_start_and_raced_attachment_require_no_drain() {
        let winner = Daemon::fixture(31, "0.50.8");
        let lifecycle = Fixture::with(None, None, Some(winner.clone()));
        let answer = connect_with(&lifecycle, Intent::Connect).unwrap();
        assert!(answer.daemon == winner && answer.refusal.is_none());
        assert_eq!(*lifecycle.start.borrow(), Some((Value::Null, false)));
        assert_eq!(
            *lifecycle.actions.borrow(),
            ["python", "select", "stage", "launch", "settle"]
        );

        let raced = Fixture::with(None, Some(winner.clone()), None);
        let answer = connect_with(&raced, Intent::Connect).unwrap();
        assert!(answer.daemon == winner && answer.refusal.is_none());
        assert!(raced.start.borrow().is_none());
    }

    #[test]
    fn automatic_minor_and_explicit_same_core_replacement_use_the_observed_identity() {
        let successor = Daemon::fixture(31, "0.50.8");
        for (running, intent, replace) in [
            ("0.49.7", Intent::Connect, false),
            ("0.50.8-dev", Intent::Replace, true),
        ] {
            let prior = Daemon::fixture(12, running);
            let lifecycle =
                Fixture::with(Some(prior.clone()), Some(prior), Some(successor.clone()));
            let answer = connect_with(&lifecycle, intent).unwrap();
            assert!(answer.daemon == successor && answer.refusal.is_none());
            let data = lifecycle.start.borrow();
            let (identity, explicit) = data.as_ref().unwrap();
            assert_eq!(identity["pid"], 12);
            assert_eq!(identity["version"], running);
            assert!(identity.get("token").is_none());
            assert_eq!(*explicit, replace);
        }
    }

    #[test]
    fn newer_daemon_and_recovery_attach_without_staging() {
        for (version, intent, refusal, actions) in [
            ("0.51.0", Intent::Connect, None, &["select"][..]),
            (
                "0.51.0",
                Intent::Replace,
                Some(NEWER_RUNNING),
                &["select"][..],
            ),
            ("0.49.7", Intent::Recover, None, &[][..]),
        ] {
            let prior = Daemon::fixture(12, version);
            let lifecycle = Fixture::with(Some(prior.clone()), None, None);
            let answer = connect_with(&lifecycle, intent).unwrap();
            assert!(answer.daemon == prior);
            assert_eq!(answer.refusal, refusal);
            assert_eq!(lifecycle.actions.borrow().as_slice(), actions);
        }
    }

    #[test]
    fn staging_refusal_uncertain_endpoint_and_failed_start_keep_the_incumbent() {
        let prior = Daemon::fixture(12, "0.50.8");
        let mut failed_stage = Fixture::with(Some(prior.clone()), None, None);
        failed_stage.stage_fails = true;
        let uncertain = Fixture::with(Some(prior.clone()), None, None);
        let mut failed_start = Fixture::with(
            Some(prior.clone()),
            Some(prior.clone()),
            Some(prior.clone()),
        );
        failed_start.start_succeeds = false;
        for (fixture, reason, launched) in [
            (failed_stage, STAGING_FAILED, false),
            (uncertain, HEALTH_UNCERTAIN, false),
            (failed_start, REPLACEMENT_FAILED, true),
        ] {
            let answer = connect_with(&fixture, Intent::Replace).unwrap();
            assert!(answer.daemon == prior);
            assert_eq!(answer.refusal, Some(reason));
            assert_eq!(fixture.start.borrow().is_some(), launched);
        }
    }

    #[test]
    fn missing_python_prevents_staging_and_start() {
        let mut missing = Fixture::with(None, None, None);
        missing.python = false;
        let error = connect_with(&missing, Intent::Connect).err().unwrap();
        assert!(error.python_required);
        assert_eq!(*missing.actions.borrow(), ["python"]);

        let prior = Daemon::fixture(12, "0.49.7");
        let mut replacing = Fixture::with(Some(prior.clone()), None, None);
        replacing.python = false;
        let answer = connect_with(&replacing, Intent::Connect).unwrap();
        assert!(answer.daemon == prior && answer.refusal.unwrap().contains("Python 3"));
        assert_eq!(*replacing.actions.borrow(), ["select", "python"]);
    }

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
