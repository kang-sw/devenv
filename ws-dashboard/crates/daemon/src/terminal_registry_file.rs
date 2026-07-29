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
    /// CONTRACT (boot-identity gate): qualifies `start_time` with the boot it
    /// was measured in, because on some platforms (Linux) `start_time` is
    /// ticks SINCE BOOT and therefore names a different instant on every
    /// boot - see `terminal_platform.rs`'s file-header "boot-identity gate"
    /// CONTRACT for the per-platform reasoning, and
    /// `terminal_reconcile::boot_identity_verified` for the decision itself.
    ///
    /// `#[serde(default)]` + `Option` is a HARD backward-compatibility
    /// requirement, not a convenience: entries written before this field
    /// existed are still on disk in live `<registry_dir>`s and must keep
    /// parsing rather than being discarded as malformed. The conservative
    /// reading of the resulting `None` is deliberate and asymmetric - on a
    /// boot-relative platform a `None` boot identity is UNVERIFIABLE, so it
    /// is drop-only and can never authorize a kill. The cost is that one
    /// legacy entry per pre-upgrade helper is dropped instead of reaped
    /// (a leaked helper process, self-exiting on its own idle/grace timers);
    /// the alternative - treating `None` as "trust the pid" - is exactly the
    /// arbitrary-process SIGKILL this field exists to prevent.
    #[serde(default)]
    pub boot_id: Option<String>,
    /// CONTRACT (260729 helper liveness probe): declares that the helper
    /// which wrote this entry understands
    /// `DaemonToHelperMessage::LivenessProbe`. Recorded HERE, in the registry
    /// file, rather than only in the IPC handshake, because the site that
    /// needs it most has no handshake to read a version out of: the periodic
    /// sweep probes helpers it never handshaked with, and a *busy* helper
    /// answers no handshake at all. All four daemon-side kill sites already
    /// read `<registry_dir>/<id>.json`, so this is the one fact every one of
    /// them can consult BEFORE putting a byte on the wire.
    ///
    /// `#[serde(default)]` (i.e. absent decodes as `false`) is a HARD
    /// backward-compatibility requirement, exactly like `boot_id` above and
    /// for a strictly worse failure mode: entries written by helpers that
    /// predate the probe are on disk in live registry dirs right now, and
    /// sending `LivenessProbe` to one of those helpers does not merely
    /// disconnect it - `read_message` turns the unknown variant into an
    /// `io::Error`, the helper's read site propagates it, and
    /// `run_terminal_helper`'s exit path then SIGKILLs the user's shell and
    /// erases its own registry entry. Absent therefore means "assume it
    /// cannot answer, and never ask".
    ///
    /// The daemon-side consequence is deliberate and stated in the ticket: a
    /// helper that declares no capability is LEFT ALONE on silence rather
    /// than reaped, because "connected but unanswered" is undecidable for it
    /// by construction. Only positive absence (no listener) may kill it.
    #[serde(default)]
    pub supports_liveness_probe: bool,
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
        if let Some(entry) = read_registry_entry_at(&path) {
            entries.push(entry);
        }
    }
    entries
}

/// Reads and parses exactly one `<registry_dir>/<terminal_id>.json` entry,
/// without scanning the rest of the directory - the daemon-side single-entry
/// lookup `scan_registry_dir`'s whole-directory scan does not fit (e.g.
/// sub-fix 1's post-handshake-failure read, sub-fix 3's per-entry sweep
/// re-check). `None` covers both "file does not exist" and "file exists but
/// is malformed" (logged the same way `scan_registry_dir` logs a malformed
/// entry) - callers must treat both the same, as "nothing usable here".
pub fn read_registry_entry(registry_dir: &Path, terminal_id: &str) -> Option<TerminalRegistryEntry> {
    read_registry_entry_at(&registry_entry_path(registry_dir, terminal_id))
}

fn read_registry_entry_at(path: &Path) -> Option<TerminalRegistryEntry> {
    match fs::read_to_string(path)
        .map_err(io::Error::from)
        .and_then(|raw| serde_json::from_str::<TerminalRegistryEntry>(&raw).map_err(io::Error::other))
    {
        Ok(entry) => Some(entry),
        Err(error) => {
            if error.kind() != io::ErrorKind::NotFound {
                tracing::warn!(
                    %error,
                    path = %path.display(),
                    "skipping malformed terminal registry entry"
                );
            }
            None
        }
    }
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
            boot_id: Some("boot-uuid-sample".to_owned()),
            supports_liveness_probe: true,
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

    // CONTRACT (boot-identity gate, backward compatibility): entries written
    // before `bootId` existed are still on disk in live registry dirs. They
    // MUST keep parsing - `#[serde(default)]` on an `Option` - rather than
    // being rejected as malformed and silently discarded by
    // `scan_registry_dir`. This is a byte-for-byte legacy payload (the exact
    // field set `write_registry_entry` produced before this change), so it
    // fails if anyone drops the `#[serde(default)]` or makes the field
    // non-optional.
    #[test]
    fn a_legacy_entry_written_without_boot_id_still_parses_with_none() {
        let dir = temp_dir("legacy-no-boot-id");
        fs::create_dir_all(&dir).expect("create dir");
        let legacy = r#"{
  "terminalId": "term_legacy",
  "workRootId": "root-1",
  "pid": 4242,
  "startTime": 100,
  "socketPath": "/tmp/term.sock",
  "createdAtMs": 1,
  "title": "Terminal",
  "cwdHint": null,
  "columns": 80,
  "rows": 24
}"#;
        fs::write(registry_entry_path(&dir, "term_legacy"), legacy).expect("write legacy entry");

        let entry = read_registry_entry(&dir, "term_legacy")
            .expect("a pre-boot-id registry entry must still parse, not be skipped as malformed");
        assert_eq!(entry.terminal_id, "term_legacy");
        assert_eq!(entry.pid, 4242);
        assert_eq!(
            entry.boot_id, None,
            "a legacy entry must read back as an ABSENT boot identity, which the daemon-side gate \
             treats as unverifiable (drop-only, never killable) on a boot-relative platform"
        );
        assert_eq!(
            scan_registry_dir(&dir).len(),
            1,
            "the directory scan must keep legacy entries too, not drop them"
        );

        let _ = fs::remove_dir_all(&dir);
    }

    // CONTRACT (260729 helper liveness probe, compatibility): the field must
    // decode as ABSENT-means-false from a hand-written legacy payload, not
    // merely round-trip through a struct the writer already populated. This
    // is the exact byte shape a pre-probe helper left on disk, and the whole
    // upgrade-safety argument rests on it: `false` here is what stops the
    // daemon ever putting `LivenessProbe` on that helper's wire (which would
    // SIGKILL its shell) and what makes "connected but silent" resolve to
    // leave-alone rather than kill.
    #[test]
    fn a_legacy_entry_written_without_the_probe_capability_decodes_as_unsupported() {
        let dir = temp_dir("legacy-no-probe-capability");
        fs::create_dir_all(&dir).expect("create dir");
        let legacy = r#"{
  "terminalId": "term_legacy_probe",
  "workRootId": "root-1",
  "pid": 4242,
  "startTime": 100,
  "bootId": "boot-uuid-sample",
  "socketPath": "/tmp/term.sock",
  "createdAtMs": 1,
  "title": "Terminal",
  "cwdHint": null,
  "columns": 80,
  "rows": 24
}"#;
        fs::write(registry_entry_path(&dir, "term_legacy_probe"), legacy)
            .expect("write legacy entry");

        let entry = read_registry_entry(&dir, "term_legacy_probe")
            .expect("a pre-probe registry entry must still parse, not be skipped as malformed");
        assert!(
            !entry.supports_liveness_probe,
            "an absent `supportsLivenessProbe` must decode as false - the daemon must never \
             send a probe to a helper that predates the variant"
        );

        let _ = fs::remove_dir_all(&dir);
    }

    // CONTRACT (260729 helper liveness probe): the declaration must actually
    // reach disk under the camelCase name the rest of this file family uses.
    // A `#[serde(skip)]` or a rename would make every freshly written entry
    // read back as "predates the probe", silently disabling the whole
    // three-way predicate while every round-trip test stayed green.
    #[test]
    fn a_written_entry_persists_its_probe_capability_as_camel_case_json() {
        let dir = temp_dir("probe-capability-on-disk");
        write_registry_entry(&dir, &sample_entry("term_probe")).expect("write entry");

        let raw = fs::read_to_string(registry_entry_path(&dir, "term_probe")).expect("read raw json");
        let value: serde_json::Value = serde_json::from_str(&raw).expect("parse raw json");
        assert_eq!(
            value.get("supportsLivenessProbe").and_then(serde_json::Value::as_bool),
            Some(true),
            "the probe capability must be persisted as `supportsLivenessProbe`"
        );

        let _ = fs::remove_dir_all(&dir);
    }

    // CONTRACT (boot-identity gate): the field must actually reach disk under
    // the camelCase name the rest of the file family uses. A `#[serde(skip)]`
    // or a rename would leave every freshly written entry unverifiable, which
    // no round-trip-through-serde test would catch.
    #[test]
    fn a_written_entry_persists_its_boot_id_as_camel_case_json() {
        let dir = temp_dir("boot-id-on-disk");
        write_registry_entry(&dir, &sample_entry("term_boot")).expect("write entry");

        let raw = fs::read_to_string(registry_entry_path(&dir, "term_boot")).expect("read raw json");
        let value: serde_json::Value = serde_json::from_str(&raw).expect("parse raw json");
        assert_eq!(
            value.get("bootId").and_then(serde_json::Value::as_str),
            Some("boot-uuid-sample"),
            "the recorded boot identity must be persisted as `bootId`"
        );

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
    fn read_registry_entry_round_trips_a_single_entry_without_scanning_the_directory() {
        let dir = temp_dir("read-single");
        let entry = sample_entry("term_a");
        write_registry_entry(&dir, &entry).expect("write entry");

        assert_eq!(
            read_registry_entry(&dir, "term_a"),
            Some(entry),
            "must read back exactly the entry just written"
        );
        assert_eq!(
            read_registry_entry(&dir, "term_missing"),
            None,
            "a terminal id with no registry file must read as None, not error"
        );

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
