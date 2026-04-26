use std::collections::HashSet;
use sysinfo::System;

/// Return the set of session UUIDs for which a `claude -p` process is
/// currently running and its args are readable.
///
/// On macOS, `sysctl(KERN_PROCARGS2)` silently returns empty args for
/// cross-user processes.  An empty args list is treated as "not readable" —
/// the session is left inactive, no error is surfaced.
pub fn find_active_uuids() -> HashSet<String> {
    let mut sys = System::new_all();
    sys.refresh_all();

    let mut active: HashSet<String> = HashSet::new();

    for (_pid, process) in sys.processes() {
        // Filter: executable name must contain "claude".
        let name = process.name().to_string_lossy();
        if !name.to_ascii_lowercase().contains("claude") {
            continue;
        }

        let args: Vec<String> = process
            .cmd()
            .iter()
            .map(|s| s.to_string_lossy().into_owned())
            .collect();

        // Empty args = unreadable (macOS cross-user) — skip silently.
        if args.is_empty() {
            continue;
        }

        // Filter: must include the `-p` / `--print` flag.
        if !args.iter().any(|a| a == "-p" || a == "--print") {
            continue;
        }

        // Extract UUID from `--session-id <uuid>`, `--session-id=<uuid>`,
        // `--resume <uuid>`, or `--resume=<uuid>`.
        if let Some(uuid) = find_uuid_in_args(&args) {
            active.insert(uuid);
        }
    }

    active
}

/// Scan `args` for a session UUID attached to `--session-id` or `--resume`.
///
/// Handles both the space-separated form (`--session-id <uuid>`) and the
/// combined form (`--session-id=<uuid>`).
fn find_uuid_in_args(args: &[String]) -> Option<String> {
    let mut iter = args.iter().peekable();
    while let Some(arg) = iter.next() {
        // Space-separated form: --session-id <uuid>
        if arg == "--session-id" || arg == "--resume" {
            if let Some(next) = iter.next() {
                return Some(next.clone());
            }
        }

        // Combined form: --session-id=<uuid> or --resume=<uuid>
        if let Some(uuid) = arg
            .strip_prefix("--session-id=")
            .or_else(|| arg.strip_prefix("--resume="))
        {
            if !uuid.is_empty() {
                return Some(uuid.to_string());
            }
        }
    }
    None
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::find_uuid_in_args;

    fn args(v: &[&str]) -> Vec<String> {
        v.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn space_separated_session_id() {
        assert_eq!(
            find_uuid_in_args(&args(&["--session-id", "abc-123"])),
            Some("abc-123".to_string())
        );
    }

    #[test]
    fn space_separated_resume() {
        assert_eq!(
            find_uuid_in_args(&args(&["--resume", "abc-123"])),
            Some("abc-123".to_string())
        );
    }

    #[test]
    fn combined_equals_form_session_id() {
        assert_eq!(
            find_uuid_in_args(&args(&["--session-id=abc-123"])),
            Some("abc-123".to_string())
        );
    }

    #[test]
    fn combined_equals_form_resume() {
        assert_eq!(
            find_uuid_in_args(&args(&["--resume=abc-123"])),
            Some("abc-123".to_string())
        );
    }

    #[test]
    fn flag_at_end_returns_none() {
        assert_eq!(find_uuid_in_args(&args(&["--session-id"])), None);
    }

    #[test]
    fn empty_args_returns_none() {
        assert_eq!(find_uuid_in_args(&args(&[])), None);
    }

    #[test]
    fn no_uuid_flag_returns_none() {
        assert_eq!(find_uuid_in_args(&args(&["--print", "-p"])), None);
    }

    #[test]
    fn uuid_in_later_position() {
        let a = args(&["-p", "--other", "val", "--session-id", "xyz-789"]);
        assert_eq!(find_uuid_in_args(&a), Some("xyz-789".to_string()));
    }
}
