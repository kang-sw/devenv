// Materializes a vendor hook-config file (`settings.json`) under
// `agent-profiles/<terminal_id>/` at spawn time (260725 Phase 3 step 2).
// Owns I/O only; the vendor-neutral event/state data lives in
// `agent_profile_registry::HookConfigShape` (mirrors the existing
// `agent_env_profile.rs` data / `terminal_registry_file.rs` I/O split - see
// plan Codebase Findings). Reuses `terminal_registry_file.rs`'s atomic
// temp-write + `0600`-on-Unix + rename shape rather than inventing a second
// atomic-write idiom.

use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use serde_json::json;

use crate::agent_profile_registry::HookConfigShape;

/// Builds the merged Claude hook-settings JSON (matching the spike's PROVEN
/// shape: `{"hooks": {"<Event>": [{"matcher": "*", "hooks": [{"type":
/// "command", "command": "<cmd>"}]}]}}`, one key per registered event) and
/// writes it to `<profile_dir>/settings.json`, `0600` on Unix. Creates
/// `profile_dir` with default/umask permissions (no directory lockdown - a
/// directory needs its execute bit to be traversable, unlike a file).
///
/// Each event's `command` invokes `notify_binary terminal-notify --callback
/// <callback_path> --state <state>`, both paths quoted (per-platform, see
/// `shell_quote`) so a space-containing state-dir path does not break the
/// vendor CLI's shell parsing of the hook command.
///
/// Returns the written `settings.json` path for the caller to append as a
/// `--settings` argv value.
pub fn materialize_hook_config(
    profile_dir: &Path,
    shape: &HookConfigShape,
    notify_binary: &Path,
    callback_path: &Path,
) -> io::Result<PathBuf> {
    fs::create_dir_all(profile_dir)?;

    let notify_binary_quoted = shell_quote(&notify_binary.display().to_string());
    let callback_path_quoted = shell_quote(&callback_path.display().to_string());

    let mut hooks = serde_json::Map::new();
    for (event, state) in shape.events {
        let command = format!(
            "{notify_binary_quoted} terminal-notify --callback {callback_path_quoted} --state {state}"
        );
        hooks.insert(
            (*event).to_owned(),
            json!([{
                "matcher": "*",
                "hooks": [{ "type": "command", "command": command }]
            }]),
        );
    }
    let settings = json!({ "hooks": hooks });

    let path = profile_dir.join("settings.json");
    // CONTRACT (review cycle 1, addendum finding I - checked, not vulnerable):
    // unlike `agent_callback::write_bound_base_url`'s single well-known
    // shared path, `profile_dir` here is already unique per terminal
    // (`agent-profiles/<terminal_id>/`, `terminal_id` a fresh random id per
    // spawn), so this fixed `settings.json.tmp` name never collides across
    // concurrent writers the way the fixed `bound-base-url.json.tmp` name
    // did - no pid/nonce suffix needed.
    let temp_path = path.with_extension("json.tmp");
    let raw = serde_json::to_string_pretty(&settings).map_err(io::Error::other)?;
    // NOTE (forward note, Phase 4 - record only, same as
    // `agent_callback::write_bound_base_url`'s identical note): `fs::write`
    // creates `temp_path` at umask-default mode and only THEN gets chmod'd to
    // 0600 below, so there is a brief world-readable window. Harmless for
    // this secret-free `settings.json`; load-bearing if a future writer of a
    // token-bearing file (Phase 4's `callback.json`) copies this sequence.
    fs::write(&temp_path, raw)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&temp_path, fs::Permissions::from_mode(0o600))?;
    }
    fs::rename(&temp_path, &path)?;
    Ok(path)
}

// CONTRACT (review cycle 1, finding D): the vendor CLI's own choice of shell
// to run `type: "command"` hooks through is NOT proven on Windows - the
// Phase 3 step-1 spike that proved the hook-JSON shape (`## Relevant Ticket
// Contract`) was macOS-only, and no Windows machine was available to spike
// this in review cycle 1 either. What IS certain, independent of that
// unknown: `cmd.exe` does not treat `'` as a quote character at all (it is
// passed through as a literal byte of whatever token contains it), so
// reusing `claude_cli.rs`'s POSIX `shell_single_quote` scheme unconditionally
// - which this module did before this fix - is provably wrong whenever the
// dispatch shell is `cmd.exe`. This is a DOCUMENTED, UNVERIFIED-ON-WINDOWS
// best-effort improvement, not an empirically proven fix (report this
// shipped with says so explicitly): `"`-based quoting is the argument-
// grouping convention both `cmd.exe` and PowerShell double-quoted strings
// honor, where a bare `'` is not a quote character in either, so it replaces
// a provably-wrong scheme with a plausibly-correct one. Closing this gap for
// real needs a Windows spike mirroring the macOS one Phase 3 step 1 already
// ran - tracked as a follow-up, not invented here.
#[cfg(not(windows))]
fn shell_quote(text: &str) -> String {
    // CONTRACT: mirrors `claude_cli.rs::shell_single_quote` exactly (that
    // function is private to its module, so this is a deliberate local
    // duplicate per the plan, not a divergent quoting scheme).
    format!("'{}'", text.replace('\'', "'\\''"))
}

#[cfg(windows)]
fn shell_quote(text: &str) -> String {
    format!("\"{}\"", text.replace('"', "\"\""))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU64, Ordering};

    static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

    fn temp_dir(label: &str) -> PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        std::env::temp_dir().join(format!(
            "ws-dashboard-agent-hook-config-{label}-{}-{unique}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time after epoch")
                .as_nanos()
        ))
    }

    fn sample_shape() -> HookConfigShape {
        HookConfigShape {
            events: &[("UserPromptSubmit", "working"), ("Stop", "ready")],
        }
    }

    #[test]
    fn materialize_writes_a_settings_file_with_both_events_and_the_notify_command_shape() {
        let profile_dir = temp_dir("both-events");
        let notify_binary = PathBuf::from("/usr/local/bin/ws-dashboard");
        let callback_path = profile_dir.join("callback.json");

        let path = materialize_hook_config(&profile_dir, &sample_shape(), &notify_binary, &callback_path)
            .expect("materialize hook config");
        assert_eq!(path, profile_dir.join("settings.json"));

        let raw = fs::read_to_string(&path).expect("read settings.json");
        let parsed: serde_json::Value = serde_json::from_str(&raw).expect("parse settings.json");
        let hooks = parsed.get("hooks").expect("hooks key present").as_object().expect("hooks object");
        assert!(hooks.contains_key("UserPromptSubmit"));
        assert!(hooks.contains_key("Stop"));

        for (event, state) in sample_shape().events {
            let command = hooks[*event][0]["hooks"][0]["command"]
                .as_str()
                .unwrap_or_else(|| panic!("{event} command string"));
            assert!(command.contains("terminal-notify"));
            assert!(command.contains(&format!("--state {state}")));
            assert!(command.contains(&callback_path.display().to_string()));
            assert!(command.contains(&notify_binary.display().to_string()));
        }

        let _ = fs::remove_dir_all(&profile_dir);
    }

    #[test]
    #[cfg(unix)]
    fn materialize_writes_settings_file_at_mode_0600() {
        use std::os::unix::fs::PermissionsExt;

        let profile_dir = temp_dir("mode-0600");
        let notify_binary = PathBuf::from("/usr/local/bin/ws-dashboard");
        let callback_path = profile_dir.join("callback.json");

        let path = materialize_hook_config(&profile_dir, &sample_shape(), &notify_binary, &callback_path)
            .expect("materialize hook config");

        let mode = fs::metadata(&path).expect("settings.json metadata").permissions().mode() & 0o777;
        assert_eq!(mode, 0o600);

        let _ = fs::remove_dir_all(&profile_dir);
    }

    #[test]
    #[cfg(not(windows))]
    fn materialize_quotes_paths_containing_spaces() {
        let profile_dir = temp_dir("space in path");
        let notify_binary = PathBuf::from("/usr/local/bin/ws dashboard");
        let callback_path = profile_dir.join("callback.json");

        let path = materialize_hook_config(&profile_dir, &sample_shape(), &notify_binary, &callback_path)
            .expect("materialize hook config with space-containing paths");
        let raw = fs::read_to_string(&path).expect("read settings.json");
        assert!(raw.contains("'/usr/local/bin/ws dashboard'"));

        let _ = fs::remove_dir_all(&profile_dir);
    }

    // FIX (review cycle 1, finding G): the ONLY prior test of the quoting
    // helper used a space-containing path, which needs no escaping once
    // wrapped in quotes at all - it never exercises the literal-quote-escape
    // branch, which is the actual reason `shell_quote`'s POSIX side exists
    // (`claude_cli.rs`'s precedent handles the same risk). A path containing
    // a literal `'` (e.g. a macOS user directory named "O'Brien") must have
    // that quote escaped as `'\''`, not left to terminate the quoted string
    // early.
    #[test]
    #[cfg(not(windows))]
    fn materialize_escapes_a_literal_single_quote_in_a_path() {
        let profile_dir = temp_dir("literal-quote");
        let notify_binary = PathBuf::from("/Users/O'Brien/bin/ws-dashboard");
        let callback_path = profile_dir.join("callback.json");

        let path = materialize_hook_config(&profile_dir, &sample_shape(), &notify_binary, &callback_path)
            .expect("materialize hook config with a literal-single-quote path");
        let raw = fs::read_to_string(&path).expect("read settings.json");
        // Parse (not raw-text-match): the produced command text contains a
        // literal `\`, which JSON string-escapes to `\\` on disk - decoding
        // via serde_json is the only correct way to recover the literal
        // command text `shell_quote` produced.
        let parsed: serde_json::Value = serde_json::from_str(&raw).expect("parse settings.json");
        let command = parsed["hooks"]["Stop"][0]["hooks"][0]["command"]
            .as_str()
            .expect("Stop command string");
        assert!(
            command.contains(r"'/Users/O'\''Brien/bin/ws-dashboard'"),
            "expected the embedded quote escaped as '\\'', got: {command}"
        );

        let _ = fs::remove_dir_all(&profile_dir);
    }

    // Windows counterpart of the two tests above: `shell_quote`'s Windows
    // branch wraps in `"..."` and doubles embedded `"` characters instead.
    // Compiled only on Windows (mirrors the `#[cfg(unix)]` mode-assertion
    // test's own platform-gating pattern in this file) - not executed by
    // this fix's own verification run on a non-Windows machine, but keeps
    // the Windows branch under test wherever Windows CI does run this crate.
    #[test]
    #[cfg(windows)]
    fn materialize_quotes_windows_paths_with_double_quotes_and_escapes_embedded_quotes() {
        let profile_dir = temp_dir("windows-quote");
        let notify_binary = PathBuf::from(r#"C:\Users\O"Brien\bin\ws-dashboard.exe"#);
        let callback_path = profile_dir.join("callback.json");

        let path = materialize_hook_config(&profile_dir, &sample_shape(), &notify_binary, &callback_path)
            .expect("materialize hook config with a Windows path");
        let raw = fs::read_to_string(&path).expect("read settings.json");
        // Parse (not raw-text-match): the "command" value is a JSON string,
        // so backslashes and quotes in the path are JSON-escaped on disk -
        // decoding via serde_json is the only correct way to recover the
        // literal command text `shell_quote` produced.
        let parsed: serde_json::Value = serde_json::from_str(&raw).expect("parse settings.json");
        let command = parsed["hooks"]["Stop"][0]["hooks"][0]["command"]
            .as_str()
            .expect("Stop command string");
        assert!(command.contains(r#""C:\Users\O""Brien\bin\ws-dashboard.exe""#));

        let _ = fs::remove_dir_all(&profile_dir);
    }
}
