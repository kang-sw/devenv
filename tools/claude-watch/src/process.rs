use std::collections::HashSet;
use sysinfo::System;

/// Return the set of session UUIDs for which a `claude -p` process is
/// currently running and readable.
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

        // Extract UUID from `--session-id <uuid>` or `--resume <uuid>`.
        if let Some(uuid) = find_uuid_in_args(&args) {
            active.insert(uuid);
        }
    }

    active
}

fn find_uuid_in_args(args: &[String]) -> Option<String> {
    let mut iter = args.iter().peekable();
    while let Some(arg) = iter.next() {
        if arg == "--session-id" || arg == "--resume" {
            if let Some(next) = iter.next() {
                return Some(next.clone());
            }
        }
    }
    None
}
