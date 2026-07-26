// Daemon-owned per-terminal spawn-profile provenance sidecar
// (260726-bug-dashboard-agent-profile-provenance-lost-on-restart):
// `agent-profiles/<terminal_id>/profile.json`, written at spawn for ANY
// resolved profile (hooked or hookless) and read back in `reconcile_entry`'s
// adopt arm so a session re-adopted after a daemon restart still reports the
// profile it was spawned with. Third file in the same daemon-owned
// per-terminal lane as `agent_token_store.rs`'s `terminal-tokens/<id>.json`
// and `agent_callback.rs`'s `agent-profiles/<id>/callback.json`, reclaimed by
// the same GC sweep (`agent_profile_gc.rs` removes the whole orphaned
// directory) - this module adds a file, not a lifecycle.
//
// CONTRACT (ticket "It must also never carry a secret"): this file is
// deliberately NOT merged into `callback.json`. The callback token stays in
// `terminal-tokens/` and `callback.json`, and nothing secret may ever be
// added to `ProfileFile` - merging the two would force either minting a
// callback token for a hookless profile (broadening the credential surface
// for no reason) or letting `callback.json` exist without a token, which
// would break the invariant `terminal.rs::recover_callback_token` depends on
// (the PRESENCE of `callback.json` is what distinguishes "spawned with hooks"
// from a plain shell).
//
// CONTRACT (ticket "Forward-compatibility guardrail"): this file is
// deliberately unversioned, so every field must stay `Option<T>` +
// `#[serde(default)]` and a malformed file must read back as absent. That is
// what keeps it from ever becoming a second instance of the helper-owned
// registry's skip-the-whole-entry hazard
// (`260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`,
// `terminal_registry_file.rs:98-110`): a reader that cannot parse this file
// degrades to today's pre-fix behavior (`profileId: null`) rather than
// losing the terminal.
//
// The `0600` write mode carries no secret here; it is simply the mode the
// shared `agent_token_store::create_new_file_at_mode_0600` helper produces,
// and matching the sibling files in the same directory is cheaper than
// justifying a laxer one.

use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ProfileFile {
    #[serde(default)]
    profile_id: Option<String>,
}

pub fn profile_path(profile_dir: &Path) -> PathBuf {
    profile_dir.join("profile.json")
}

/// Atomic temp-rename write into an existing-or-created
/// `agent-profiles/<terminal_id>/` - same shape as
/// `agent_callback::write_callback_target`, including the fixed temp name
/// (`terminal_id` is a fresh random id per spawn, so the temp path never
/// collides across concurrent writers).
pub fn write_profile(profile_dir: &Path, profile_id: &str) -> io::Result<()> {
    fs::create_dir_all(profile_dir)?;
    let path = profile_path(profile_dir);
    let temp_path = path.with_extension("json.tmp");
    let raw = serde_json::to_string_pretty(&ProfileFile {
        profile_id: Some(profile_id.to_owned()),
    })
    .map_err(io::Error::other)?;
    crate::agent_token_store::create_new_file_at_mode_0600(&temp_path, raw.as_bytes())?;
    fs::rename(&temp_path, &path)?;
    Ok(())
}

/// Tolerant read, mirroring `agent_token_store::read_token`: a missing file
/// returns `None` silently (the ordinary case for a plain-shell terminal, or
/// for any terminal spawned before this sidecar existed - there is no
/// backfill), a malformed one returns `None` plus a warning, and neither ever
/// panics. Every degrade lands on today's pre-fix observable behavior
/// (`profileId: null`), never worse.
pub fn read_profile_id(profile_dir: &Path) -> Option<String> {
    let path = profile_path(profile_dir);
    let raw = match fs::read_to_string(&path) {
        Ok(raw) => raw,
        Err(error) if error.kind() == io::ErrorKind::NotFound => return None,
        Err(error) => {
            tracing::warn!(
                %error,
                path = %path.display(),
                "terminal profile sidecar unreadable"
            );
            return None;
        }
    };
    match serde_json::from_str::<ProfileFile>(&raw) {
        Ok(parsed) => parsed.profile_id,
        Err(error) => {
            tracing::warn!(
                %error,
                path = %path.display(),
                "skipping malformed terminal profile sidecar"
            );
            None
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU64, Ordering};

    static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

    fn temp_dir(label: &str) -> PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        std::env::temp_dir().join(format!(
            "ws-dashboard-agent-profile-store-{label}-{}-{unique}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time after epoch")
                .as_nanos()
        ))
    }

    #[test]
    fn write_then_read_round_trips_a_profile_id() {
        let dir = temp_dir("roundtrip");
        write_profile(&dir, "claude").expect("write profile");

        assert_eq!(read_profile_id(&dir).as_deref(), Some("claude"));

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn write_profile_serializes_the_camel_case_profile_id_key() {
        let dir = temp_dir("shape");
        write_profile(&dir, "dummy-echo").expect("write profile");

        let raw = fs::read_to_string(profile_path(&dir)).expect("read profile sidecar");
        let parsed: serde_json::Value = serde_json::from_str(&raw).expect("profile sidecar JSON");
        assert_eq!(parsed["profileId"], "dummy-echo");
        // The token lane stays separate on purpose - see the module CONTRACT.
        assert!(
            parsed.get("token").is_none(),
            "the profile sidecar must never carry a credential: {raw}"
        );

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    #[cfg(unix)]
    fn write_profile_writes_at_mode_0600() {
        use std::os::unix::fs::PermissionsExt;

        let dir = temp_dir("mode-0600");
        write_profile(&dir, "claude").expect("write profile");

        let mode = fs::metadata(profile_path(&dir))
            .expect("profile sidecar metadata")
            .permissions()
            .mode()
            & 0o777;
        assert_eq!(mode, 0o600);

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn read_profile_id_returns_none_for_a_missing_file_without_panicking() {
        let dir = temp_dir("missing");
        assert_eq!(read_profile_id(&dir), None);
    }

    #[test]
    fn read_profile_id_returns_none_for_a_malformed_file_without_panicking() {
        let dir = temp_dir("malformed");
        fs::create_dir_all(&dir).expect("create profile dir");
        fs::write(profile_path(&dir), "not json").expect("write malformed profile sidecar");

        assert_eq!(read_profile_id(&dir), None);

        let _ = fs::remove_dir_all(&dir);
    }

    // Forward-compatibility guardrail: a file written by a FUTURE daemon that
    // added fields must still yield today's `profileId` rather than failing
    // the whole read, and a file that omits `profileId` entirely must read as
    // absent instead of erroring.
    #[test]
    fn read_profile_id_tolerates_unknown_fields_and_a_missing_profile_id() {
        let dir = temp_dir("forward-compat");
        fs::create_dir_all(&dir).expect("create profile dir");

        fs::write(
            profile_path(&dir),
            r#"{"profileId":"claude","somethingAddedLater":42}"#,
        )
        .expect("write forward-compatible profile sidecar");
        assert_eq!(read_profile_id(&dir).as_deref(), Some("claude"));

        fs::write(profile_path(&dir), "{}").expect("write empty profile sidecar");
        assert_eq!(read_profile_id(&dir), None);

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn write_profile_overwrites_an_existing_sidecar_atomically() {
        let dir = temp_dir("overwrite");
        write_profile(&dir, "claude").expect("write profile");
        write_profile(&dir, "dummy-echo").expect("rewrite profile");

        assert_eq!(read_profile_id(&dir).as_deref(), Some("dummy-echo"));

        let _ = fs::remove_dir_all(&dir);
    }
}
