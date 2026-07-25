// Daemon-owned per-terminal callback token store (260725 Phase 4):
// `terminal-tokens/<terminal_id>.json`, one file per terminal, holding the
// opaque credential a spawned agent's hook fires back with (see
// `terminal.rs::post_terminal_turn_state`). Sibling of `terminals/` and
// `agent-profiles/` under the daemon state dir - same layout precedent as
// `agent_callback.rs::bound_base_url_path`.
//
// CONTRACT (ticket "the token never touches helper argv or
// `TerminalRegistryEntry`"): this file is the ONLY place the token is
// persisted. It must never be logged, never appear in a URL, and never be
// added as a field on `TerminalRegistryEntry` (`terminal_registry_file.rs`) -
// see the hard constraints on the parent ticket's Phase 4 plan.
//
// CONTRACT (deliberate divergence from `agent_callback::write_bound_base_url`
// / `agent_hook_config::materialize_hook_config`): those two writers create
// their temp file at umask-default mode and chmod to `0600` only AFTER
// `fs::write` - a brief world-readable window that is harmless for their own
// secret-free content. This file carries a real credential for the first
// time in this codebase, so the temp file is created AT `0600` directly
// (`OpenOptions` with `.mode(0o600)` on Unix), never chmod'd after the fact.

use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct TokenFile {
    token: String,
}

pub fn token_store_dir(state_dir: &Path) -> PathBuf {
    state_dir.join("terminal-tokens")
}

pub fn token_store_path(state_dir: &Path, terminal_id: &str) -> PathBuf {
    token_store_dir(state_dir).join(format!("{terminal_id}.json"))
}

/// Atomic temp-rename write, `0600` from creation (no chmod-after window -
/// see the module CONTRACT above). `terminal_id` is a fresh random id per
/// spawn (see `terminal.rs::opaque_terminal_id`), so the fixed temp name
/// below never collides across concurrent writers - same reasoning
/// `agent_hook_config::materialize_hook_config` already documents for its own
/// per-terminal-id-namespaced `settings.json.tmp`.
pub fn write_token(state_dir: &Path, terminal_id: &str, token: &str) -> io::Result<()> {
    let dir = token_store_dir(state_dir);
    fs::create_dir_all(&dir)?;
    let path = token_store_path(state_dir, terminal_id);
    let temp_path = path.with_extension("json.tmp");
    let raw = serde_json::to_string_pretty(&TokenFile {
        token: token.to_owned(),
    })
    .map_err(io::Error::other)?;
    create_new_file_at_mode_0600(&temp_path, raw.as_bytes())?;
    fs::rename(&temp_path, &path)?;
    Ok(())
}

/// Tolerant read: a missing or malformed file returns `None` (logged, never
/// panics) - mirrors `terminal_registry_file::scan_registry_dir`'s tolerance
/// for a single corrupt entry.
pub fn read_token(state_dir: &Path, terminal_id: &str) -> Option<String> {
    let path = token_store_path(state_dir, terminal_id);
    let raw = match fs::read_to_string(&path) {
        Ok(raw) => raw,
        Err(error) if error.kind() == io::ErrorKind::NotFound => return None,
        Err(error) => {
            tracing::warn!(
                %error,
                path = %path.display(),
                "terminal token file unreadable"
            );
            return None;
        }
    };
    match serde_json::from_str::<TokenFile>(&raw) {
        Ok(parsed) => Some(parsed.token),
        Err(error) => {
            tracing::warn!(
                %error,
                path = %path.display(),
                "skipping malformed terminal token file"
            );
            None
        }
    }
}

/// Best-effort delete, mirrors `terminal_registry_file::delete_registry_entry`.
pub fn delete_token(state_dir: &Path, terminal_id: &str) {
    let _ = fs::remove_file(token_store_path(state_dir, terminal_id));
}

#[cfg(unix)]
fn create_new_file_at_mode_0600(path: &Path, data: &[u8]) -> io::Result<()> {
    use std::io::Write as _;
    use std::os::unix::fs::OpenOptionsExt;
    // Best-effort clear of a stale leftover from an earlier crashed write at
    // this exact temp path - `terminal_id` is fresh per spawn, so a
    // collision here only follows a prior crash mid-write. `create_new`
    // below (not `create`/`truncate`) is what guarantees this write always
    // creates its OWN file at the requested mode rather than silently
    // reopening one some other process left behind at a looser mode.
    let _ = fs::remove_file(path);
    let mut file = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .open(path)?;
    file.write_all(data)
}

#[cfg(not(unix))]
fn create_new_file_at_mode_0600(path: &Path, data: &[u8]) -> io::Result<()> {
    fs::write(path, data)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU64, Ordering};

    static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

    fn temp_dir(label: &str) -> PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        std::env::temp_dir().join(format!(
            "ws-dashboard-agent-token-store-{label}-{}-{unique}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time after epoch")
                .as_nanos()
        ))
    }

    #[test]
    fn write_then_read_round_trips_a_token() {
        let dir = temp_dir("roundtrip");
        write_token(&dir, "term_a", "secret-token-value").expect("write token");

        assert_eq!(
            read_token(&dir, "term_a").as_deref(),
            Some("secret-token-value")
        );

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    #[cfg(unix)]
    fn write_token_writes_at_mode_0600() {
        use std::os::unix::fs::PermissionsExt;

        let dir = temp_dir("mode-0600");
        write_token(&dir, "term_a", "secret").expect("write token");

        let mode = fs::metadata(token_store_path(&dir, "term_a"))
            .expect("token file metadata")
            .permissions()
            .mode()
            & 0o777;
        assert_eq!(mode, 0o600);

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn read_token_returns_none_for_a_missing_file_without_panicking() {
        let dir = temp_dir("missing");
        assert_eq!(read_token(&dir, "term_missing"), None);
    }

    #[test]
    fn read_token_returns_none_for_a_malformed_file_without_panicking() {
        let dir = temp_dir("malformed");
        fs::create_dir_all(token_store_dir(&dir)).expect("create token store dir");
        fs::write(token_store_path(&dir, "term_bad"), "not json").expect("write malformed token file");

        assert_eq!(read_token(&dir, "term_bad"), None);

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn delete_token_removes_the_file() {
        let dir = temp_dir("delete");
        write_token(&dir, "term_a", "secret").expect("write token");
        assert!(read_token(&dir, "term_a").is_some());

        delete_token(&dir, "term_a");

        assert_eq!(read_token(&dir, "term_a"), None);
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn delete_token_on_a_missing_file_is_a_harmless_no_op() {
        let dir = temp_dir("delete-missing");
        delete_token(&dir, "term_never_existed");
        let _ = fs::remove_dir_all(&dir);
    }
}
