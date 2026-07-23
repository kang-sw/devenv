// On-disk terminal registry: one file per terminal at
// `<registry_dir>/<terminal_id>.json`, owned by the helper process
// (create-on-spawn, delete-on-exit - the daemon only ever prunes entries it
// has positively confirmed dead, never entries it merely hasn't heard from).
// Atomic writes reuse `persistent_state.rs::write_state_json`'s temp-rename
// shape, adding 0600 permissions on Unix (that existing helper does not).

use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalRegistryEntry {
    pub terminal_id: String,
    pub work_root_id: String,
    pub pid: u32,
    pub start_time: u64,
    pub socket_path: PathBuf,
    pub created_at_ms: u64,
    pub title: String,
    pub cwd_hint: Option<String>,
    pub columns: u16,
    pub rows: u16,
}

pub fn registry_entry_path(registry_dir: &Path, terminal_id: &str) -> PathBuf {
    registry_dir.join(format!("{terminal_id}.json"))
}

pub fn registry_socket_path(registry_dir: &Path, terminal_id: &str) -> PathBuf {
    registry_dir.join(format!("{terminal_id}.sock"))
}

/// Atomic temp-rename write, 0600 on Unix. Synchronous (`std::fs`) - callers
/// on the daemon side (boot reconcile, at startup, before serving traffic)
/// should offload via `spawn_blocking`; the helper process calls this
/// directly from its own dedicated startup path before any async I/O
/// matters.
pub fn write_registry_entry(registry_dir: &Path, entry: &TerminalRegistryEntry) -> io::Result<()> {
    fs::create_dir_all(registry_dir)?;
    let path = registry_entry_path(registry_dir, &entry.terminal_id);
    let temp_path = path.with_extension("json.tmp");
    let raw = serde_json::to_string_pretty(entry).map_err(io::Error::other)?;
    fs::write(&temp_path, raw)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&temp_path, fs::Permissions::from_mode(0o600))?;
    }
    fs::rename(&temp_path, &path)?;
    Ok(())
}

// CONTRACT (260723 Phase-1 review finding M-d): the helper's own
// delete-on-exit cleanup (`run_terminal_helper`'s end, after
// `serve_connections` returns) removes both its registry `.json` and its
// `.sock` file - but a hard-killed helper (verified-PID kill fallback tier)
// never runs that cleanup at all, so its `.sock` file would otherwise be
// orphaned under `<registry_dir>` forever. Every daemon-side caller of this
// function (boot-reconcile's row-4 kill-then-drop, and any future
// verified-kill path) is exactly the case where the helper's own cleanup
// did NOT run, so pruning the `.sock` here too is the right single choke
// point - disk hygiene only, the `.json` is what boot reconcile actually
// keys its identity check off of.
pub fn delete_registry_entry(registry_dir: &Path, terminal_id: &str) {
    let _ = fs::remove_file(registry_entry_path(registry_dir, terminal_id));
    let _ = fs::remove_file(registry_socket_path(registry_dir, terminal_id));
}

/// Directory scan tolerant of partial failure: a whole-directory-unreadable
/// condition (e.g. missing directory on first-ever boot) degrades to an
/// empty registry with a loud warning; a single malformed entry is skipped
/// (logged) without discarding the rest of the directory.
pub fn scan_registry_dir(registry_dir: &Path) -> Vec<TerminalRegistryEntry> {
    let read_dir = match fs::read_dir(registry_dir) {
        Ok(read_dir) => read_dir,
        Err(error) if error.kind() == io::ErrorKind::NotFound => return Vec::new(),
        Err(error) => {
            tracing::warn!(
                %error,
                path = %registry_dir.display(),
                "terminal registry directory unreadable; starting fresh"
            );
            return Vec::new();
        }
    };

    let mut entries = Vec::new();
    for item in read_dir {
        let Ok(item) = item else { continue };
        let path = item.path();
        if path.extension().and_then(|extension| extension.to_str()) != Some("json") {
            continue;
        }
        match fs::read_to_string(&path)
            .map_err(io::Error::from)
            .and_then(|raw| serde_json::from_str::<TerminalRegistryEntry>(&raw).map_err(io::Error::other))
        {
            Ok(entry) => entries.push(entry),
            Err(error) => {
                tracing::warn!(
                    %error,
                    path = %path.display(),
                    "skipping malformed terminal registry entry"
                );
            }
        }
    }
    entries
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp_dir(label: &str) -> PathBuf {
        std::env::temp_dir().join(format!(
            "ws-dashboard-terminal-registry-file-{label}-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time after epoch")
                .as_nanos()
        ))
    }

    fn sample_entry(terminal_id: &str) -> TerminalRegistryEntry {
        TerminalRegistryEntry {
            terminal_id: terminal_id.to_owned(),
            work_root_id: "root-1".to_owned(),
            pid: 4242,
            start_time: 100,
            socket_path: PathBuf::from("/tmp/term.sock"),
            created_at_ms: 1,
            title: "Terminal".to_owned(),
            cwd_hint: None,
            columns: 80,
            rows: 24,
        }
    }

    #[test]
    fn write_then_scan_round_trips_an_entry() {
        let dir = temp_dir("roundtrip");
        let entry = sample_entry("term_a");
        write_registry_entry(&dir, &entry).expect("write entry");

        let scanned = scan_registry_dir(&dir);
        assert_eq!(scanned, vec![entry]);

        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let path = registry_entry_path(&dir, "term_a");
            let mode = fs::metadata(&path).expect("entry metadata").permissions().mode() & 0o777;
            assert_eq!(mode, 0o600);
        }

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn scan_of_missing_directory_returns_empty_without_error() {
        let dir = temp_dir("missing");
        assert!(scan_registry_dir(&dir).is_empty());
    }

    #[test]
    fn scan_skips_single_malformed_entry_but_keeps_the_rest() {
        let dir = temp_dir("malformed");
        write_registry_entry(&dir, &sample_entry("term_good")).expect("write good entry");
        fs::write(registry_entry_path(&dir, "term_bad"), "not json").expect("write bad entry");

        let scanned = scan_registry_dir(&dir);
        assert_eq!(scanned.len(), 1);
        assert_eq!(scanned[0].terminal_id, "term_good");

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn delete_registry_entry_removes_the_file() {
        let dir = temp_dir("delete");
        write_registry_entry(&dir, &sample_entry("term_a")).expect("write entry");
        assert_eq!(scan_registry_dir(&dir).len(), 1);

        delete_registry_entry(&dir, "term_a");
        assert!(scan_registry_dir(&dir).is_empty());

        let _ = fs::remove_dir_all(&dir);
    }

    // CONTRACT (260723 Phase-1 review finding M-d): a hard-killed helper
    // (verified-PID kill fallback) never reaches its own delete-on-exit
    // cleanup, so the daemon-side `delete_registry_entry` call site must
    // prune the stray `.sock` file itself, not just the `.json` - otherwise
    // `<registry_dir>` accumulates orphaned socket files indefinitely.
    #[test]
    fn delete_registry_entry_also_prunes_a_stray_socket_file() {
        let dir = temp_dir("delete-socket");
        write_registry_entry(&dir, &sample_entry("term_a")).expect("write entry");
        let socket_path = registry_socket_path(&dir, "term_a");
        fs::write(&socket_path, b"not a real socket, just a stand-in file")
            .expect("write stand-in socket file");
        assert!(socket_path.exists(), "stand-in socket file must exist before delete");

        delete_registry_entry(&dir, "term_a");

        assert!(scan_registry_dir(&dir).is_empty(), "json entry must be gone");
        assert!(!socket_path.exists(), "stale .sock file must be pruned alongside the .json entry");

        let _ = fs::remove_dir_all(&dir);
    }
}
