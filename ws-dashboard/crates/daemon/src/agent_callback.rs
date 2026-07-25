// Shared callback-target shape and I/O for the PTY-agent attention
// notification path (260725 Phase 3 steps 2-3). Both the daemon's write side
// (this phase: the global `bound-base-url.json`, written at every bind) and
// the `terminal-notify` read side (this phase's CLI subcommand) share this
// ONE JSON shape now instead of inventing two - Phase 4 extends it with a
// real `terminalId`/`token` per-terminal `callback.json` rather than
// replacing it.
//
// CONTRACT: `resolve_callback_target` reads the file FRESH on every call, no
// caching of any kind. This is load-bearing: the ticket's central
// verification line is "`terminal-notify` resolves a base URL written after
// the config file" - the daemon rewrites the bound-base-url on every bind
// (ephemeral port), so a cached/memoized read would silently observe a
// stale, possibly-dead base URL. Do not add a `OnceCell`/`static`/memoized
// wrapper around this function.

use std::fmt;
use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CallbackTarget {
    pub base_url: String,
    #[serde(default)]
    pub terminal_id: Option<String>,
    #[serde(default)]
    pub token: Option<String>,
}

/// Distinct failure classes for `resolve_callback_target`, so a caller (in
/// particular `terminal-notify`'s CLI entry point) can name WHICH failure
/// occurred rather than reporting one generic "resolution failed" message.
///
/// FIX (review cycle 1, finding H): the design (plan `## Design questions`
/// answer 1) promises three distinguishable failure classes - "file not
/// found" / "invalid JSON" / "missing baseUrl" - but a syntactically valid
/// JSON object lacking `baseUrl` used to collapse into the SAME message as a
/// genuine syntax error, because both were represented as
/// `serde_json::Error` from one `serde_json::from_str::<CallbackTarget>`
/// call. `MissingBaseUrl` is now a separate variant so the message says
/// precisely what happened.
#[derive(Debug)]
pub enum ResolveError {
    Io(io::Error),
    Parse(serde_json::Error),
    MissingBaseUrl,
}

impl fmt::Display for ResolveError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ResolveError::Io(error) if error.kind() == io::ErrorKind::NotFound => {
                write!(f, "callback file not found")
            }
            ResolveError::Io(error) => write!(f, "callback file unreadable: {error}"),
            ResolveError::Parse(error) => write!(f, "callback file is not valid JSON: {error}"),
            ResolveError::MissingBaseUrl => {
                write!(f, "callback file is valid JSON but is missing the required `baseUrl` field")
            }
        }
    }
}

impl std::error::Error for ResolveError {}

/// Sibling of `terminals/` and `agent-profiles/` under the daemon state dir -
/// deliberately NOT inside `agent-profiles/`, since that directory is the
/// Phase 4 GC sweep's scan root over per-terminal-id subdirectories; a stray
/// non-terminal-id file at its top level would need special-casing there.
pub fn bound_base_url_path(state_dir: &Path) -> PathBuf {
    state_dir.join("bound-base-url.json")
}

/// The `--callback` target for a specific terminal's materialized hooks
/// (Phase 4 writes the real file; this phase only reserves the path).
pub fn callback_path(profile_dir: &Path) -> PathBuf {
    profile_dir.join("callback.json")
}

/// Atomic temp-rename write, `0600` on Unix - same shape as
/// `terminal_registry_file::write_registry_entry`. Called on every daemon
/// bind (the base URL is ephemeral-port-dependent), unconditional overwrite.
///
/// CONTRACT (forward note, Phase 4 - do NOT read this file to derive
/// `callback.json`'s `baseUrl`): this file is presently WRITE-ONLY - nothing
/// in the tree reads `bound_base_url_path`. It is a single well-known path
/// shared by every daemon under one state dir, which is EXACTLY the shape
/// the ticket's `## Decisions` "Ephemeral port" bullet rejects for callback
/// targets ("a shared file would let a second concurrent daemon silently
/// steal every agent's callback target"). That rejection is about
/// `callback.json` specifically and this file carries no token, so the
/// letter is respected today - but if Phase 4 derives a per-terminal
/// `callback.json`'s `baseUrl` by READING this file instead of from the
/// `bound_addr` the daemon already holds in memory at bind time, the
/// rejected multi-daemon steal is reintroduced. Do not reach for this file
/// from Phase 4 for that purpose.
pub fn write_bound_base_url(state_dir: &Path, base_url: &str) -> io::Result<()> {
    fs::create_dir_all(state_dir)?;
    let path = bound_base_url_path(state_dir);
    // FIX (review cycle 1, addendum finding I): the temp path used to be the
    // FIXED name `bound-base-url.json.tmp`, shared by every writer of this
    // state dir. Two daemons sharing a `WS_DASHBOARD_STATE_HOME` (e.g. the
    // acceptance harness's own daemon alongside a developer's) interleave
    // truncate-then-write on that same temp file before renaming, so the
    // rename can publish a TORN file - an exposure the precedent this
    // pattern was borrowed from (`write_registry_entry`) does not have,
    // since its temp name is derived from the per-terminal id and so never
    // collides. A pid+nonce suffix restores that property here, where no
    // per-writer id exists (this file has exactly one well-known path,
    // written by whichever daemon process binds).
    let temp_path = unique_temp_path(&path);
    let raw = serde_json::to_string_pretty(&CallbackTarget {
        base_url: base_url.to_owned(),
        terminal_id: None,
        token: None,
    })
    .map_err(io::Error::other)?;
    // NOTE (forward note, Phase 4 - record only, not this phase's job to
    // fix): `fs::write` creates `temp_path` at umask-default mode (typically
    // 0644) and only THEN gets chmod'd to 0600 below, so there is a brief
    // window where the temp file is world-readable. Harmless here (neither
    // this file nor `settings.json` carries a secret), but load-bearing if
    // Phase 4's token-bearing `callback.json` copies this exact sequence -
    // that writer should create at the target mode directly (e.g.
    // `OpenOptions::new().mode(0o600)` on Unix) or write into an already-0700
    // directory, not chmod after the fact.
    fs::write(&temp_path, raw)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&temp_path, fs::Permissions::from_mode(0o600))?;
    }
    fs::rename(&temp_path, &path)?;
    Ok(())
}

// FIX (review cycle 1, addendum finding I): produces a temp path unique per
// writer (pid + a monotonic in-process counter + a timestamp), so concurrent
// writers never truncate-then-write the same temp file before renaming. See
// the CONTRACT above `write_bound_base_url` for why a per-terminal-id temp
// name (the precedent's own scheme) is not available here.
fn unique_temp_path(path: &Path) -> PathBuf {
    static COUNTER: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);
    let counter = COUNTER.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
    let nanos = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|duration| duration.as_nanos())
        .unwrap_or(0);
    path.with_extension(format!("json.tmp.{}.{counter}.{nanos}", std::process::id()))
}

/// Reads and parses `path` fresh on every call - see the module CONTRACT
/// above. Distinguishes "file not found or unreadable", "malformed JSON",
/// and "valid JSON missing `baseUrl`" (finding H) so a caller can report
/// exactly which of the three happened rather than collapsing the last two
/// into one message.
pub fn resolve_callback_target(path: &Path) -> Result<CallbackTarget, ResolveError> {
    let raw = fs::read_to_string(path).map_err(ResolveError::Io)?;
    let value: serde_json::Value = serde_json::from_str(&raw).map_err(ResolveError::Parse)?;
    if value.get("baseUrl").is_none() {
        return Err(ResolveError::MissingBaseUrl);
    }
    serde_json::from_value(value).map_err(ResolveError::Parse)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU64, Ordering};

    static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

    fn temp_dir(label: &str) -> PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        std::env::temp_dir().join(format!(
            "ws-dashboard-agent-callback-{label}-{}-{unique}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time after epoch")
                .as_nanos()
        ))
    }

    #[test]
    fn resolve_callback_target_round_trips_a_well_formed_fixture() {
        let dir = temp_dir("roundtrip");
        fs::create_dir_all(&dir).expect("create fixture dir");
        let path = dir.join("callback.json");
        fs::write(
            &path,
            r#"{"baseUrl":"http://127.0.0.1:1234","terminalId":"t1","token":"secret"}"#,
        )
        .expect("write fixture");

        let target = resolve_callback_target(&path).expect("resolve well-formed fixture");
        assert_eq!(target.base_url, "http://127.0.0.1:1234");
        assert_eq!(target.terminal_id.as_deref(), Some("t1"));
        assert_eq!(target.token.as_deref(), Some("secret"));

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn resolve_callback_target_on_a_phase_3_era_file_missing_terminal_id_and_token_still_parses() {
        let dir = temp_dir("phase3-era");
        fs::create_dir_all(&dir).expect("create fixture dir");
        let path = dir.join("bound-base-url.json");
        fs::write(&path, r#"{"baseUrl":"http://127.0.0.1:9999"}"#).expect("write fixture");

        let target = resolve_callback_target(&path).expect("resolve phase-3-era fixture");
        assert_eq!(target.base_url, "http://127.0.0.1:9999");
        assert_eq!(target.terminal_id, None);
        assert_eq!(target.token, None);

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn resolve_callback_target_on_a_missing_file_returns_an_io_error_not_a_panic() {
        let dir = temp_dir("missing");
        let path = dir.join("does-not-exist.json");

        match resolve_callback_target(&path) {
            Err(ResolveError::Io(_)) => {}
            other => panic!("expected ResolveError::Io, got {other:?}"),
        }
    }

    #[test]
    fn resolve_callback_target_on_malformed_json_returns_a_parse_error_not_a_silent_default() {
        let dir = temp_dir("malformed");
        fs::create_dir_all(&dir).expect("create fixture dir");
        let path = dir.join("callback.json");
        fs::write(&path, "not json").expect("write malformed fixture");

        match resolve_callback_target(&path) {
            Err(ResolveError::Parse(_)) => {}
            other => panic!("expected ResolveError::Parse, got {other:?}"),
        }

        let _ = fs::remove_dir_all(&dir);
    }

    // FIX (review cycle 1, finding H): a syntactically valid JSON object
    // missing `baseUrl` used to collapse into the same `ResolveError::Parse`
    // ("not valid JSON") message as a genuine syntax error, contradicting the
    // plan's design answer 1, which promises three DISTINGUISHABLE failure
    // classes. This proves the third class is now its own variant with its
    // own message.
    #[test]
    fn resolve_callback_target_on_valid_json_missing_base_url_returns_a_distinct_error() {
        let dir = temp_dir("missing-base-url");
        fs::create_dir_all(&dir).expect("create fixture dir");
        let path = dir.join("callback.json");
        fs::write(&path, r#"{"terminalId":"t1","token":"secret"}"#).expect("write fixture");

        match resolve_callback_target(&path) {
            Err(ResolveError::MissingBaseUrl) => {}
            other => panic!("expected ResolveError::MissingBaseUrl, got {other:?}"),
        }
        assert_eq!(
            ResolveError::MissingBaseUrl.to_string(),
            "callback file is valid JSON but is missing the required `baseUrl` field"
        );
        // The message must not misdescribe a well-formed file as malformed -
        // that was the defect: reusing the "not valid JSON" text here.
        assert!(!ResolveError::MissingBaseUrl.to_string().contains("not valid JSON"));

        let _ = fs::remove_dir_all(&dir);
    }

    // CONTRACT: THE LOAD-BEARING ORDERING TEST. Proves `resolve_callback_target`
    // reads fresh on every call rather than memoizing the first-observed
    // value - the exact regression the ticket's "resolves a base URL written
    // after the config file" verification line targets. See the module
    // CONTRACT comment above.
    #[test]
    fn resolve_callback_target_observes_a_rewrite_after_the_first_read_not_the_first_observed_value() {
        let dir = temp_dir("ordering");
        fs::create_dir_all(&dir).expect("create fixture dir");
        let path = dir.join("bound-base-url.json");

        fs::write(&path, r#"{"baseUrl":"http://127.0.0.1:1111"}"#).expect("write first bind");
        let first = resolve_callback_target(&path).expect("resolve first bind");
        assert_eq!(first.base_url, "http://127.0.0.1:1111");

        // Simulate a daemon rebind on a different ephemeral port, happening
        // strictly after the first write/read.
        fs::write(&path, r#"{"baseUrl":"http://127.0.0.1:2222"}"#).expect("write second bind");
        let second = resolve_callback_target(&path).expect("resolve second bind");
        assert_eq!(
            second.base_url, "http://127.0.0.1:2222",
            "resolve_callback_target must observe the rewrite, not the first-observed value"
        );

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn write_bound_base_url_writes_a_readable_base_url_only_fixture() {
        let dir = temp_dir("write-bound");
        write_bound_base_url(&dir, "http://127.0.0.1:5555").expect("write bound base url");

        let target = resolve_callback_target(&bound_base_url_path(&dir)).expect("resolve written file");
        assert_eq!(target.base_url, "http://127.0.0.1:5555");
        assert_eq!(target.terminal_id, None);
        assert_eq!(target.token, None);

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    #[cfg(unix)]
    fn write_bound_base_url_writes_at_mode_0600() {
        use std::os::unix::fs::PermissionsExt;

        let dir = temp_dir("write-bound-mode");
        write_bound_base_url(&dir, "http://127.0.0.1:6666").expect("write bound base url");

        let mode = fs::metadata(bound_base_url_path(&dir))
            .expect("bound-base-url metadata")
            .permissions()
            .mode()
            & 0o777;
        assert_eq!(mode, 0o600);

        let _ = fs::remove_dir_all(&dir);
    }
}
