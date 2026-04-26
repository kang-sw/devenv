//! Worktree discovery and polling.
//!
//! `discover_worktrees()` runs `git worktree list --porcelain` and constructs
//! a `Worktree` record for each entry.  The first entry is always the main
//! checkout; the rest are sorted alphabetically by name.

use std::path::PathBuf;

/// A git worktree discovered from `git worktree list --porcelain`.
pub struct Worktree {
    pub path: PathBuf,
    /// Last path component — used as the tab label.
    pub name: String,
    /// True when `<repo_root>/.claude/worktrees/<name>` is a directory.
    /// Used to auto-spawn a PTY on startup (active ws-framework worktrees).
    pub is_active_ws: bool,
    /// True when the worktree was removed externally but the tab's PTY is
    /// still live.  Tracked by `App::reconcile_worktrees`.
    pub is_removed: bool,
}

/// Discover all worktrees for the current git repository.
///
/// Returns an empty `Vec` when not inside a git repository or when
/// `git worktree list` fails.  The first entry is always the main checkout;
/// subsequent entries are sorted alphabetically by name.
pub fn discover_worktrees() -> Vec<Worktree> {
    let output = std::process::Command::new("git")
        .args(["worktree", "list", "--porcelain"])
        .output();

    let output = match output {
        Ok(o) if o.status.success() => o,
        _ => return vec![],
    };

    let stdout = String::from_utf8_lossy(&output.stdout);

    let paths: Vec<PathBuf> = stdout
        .lines()
        .filter_map(|line| line.strip_prefix("worktree ").map(PathBuf::from))
        .collect();

    if paths.is_empty() {
        return vec![];
    }

    // First path is the main checkout; its parent directory contains .claude/.
    let repo_root = paths[0].clone();
    let worktrees_marker = repo_root.join(".claude").join("worktrees");

    let mut result: Vec<Worktree> = paths
        .into_iter()
        .map(|path| {
            let name = path
                .file_name()
                .map(|n| n.to_string_lossy().into_owned())
                .unwrap_or_default();
            let is_active_ws = worktrees_marker.join(&name).is_dir();
            Worktree {
                path,
                name,
                is_active_ws,
                is_removed: false,
            }
        })
        .collect();

    // Preserve main checkout at index 0; sort the rest alphabetically.
    if result.len() > 1 {
        result[1..].sort_by(|a, b| a.name.cmp(&b.name));
    }

    result
}
