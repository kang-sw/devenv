//! Worktree discovery and polling (Phase 2+).
//!
//! Phase 1 stub — real implementation added in Phase 2.

use std::path::PathBuf;

/// A git worktree discovered from `git worktree list --porcelain`.
#[allow(dead_code)]
pub struct Worktree {
    pub path: PathBuf,
    /// Last path component — used as the tab label.
    pub name: String,
    /// True when `.claude/worktrees/<name>` is a directory (active ws worktree).
    pub is_active_ws: bool,
    /// True when the worktree was removed externally but its PTY is still live.
    pub is_removed: bool,
}

/// Discover all worktrees for the current git repository.
///
/// Phase 1 stub: returns an empty list.  Real implementation is added in
/// Phase 2.
pub fn discover_worktrees() -> Vec<Worktree> {
    // TODO(phase-2): implement via `git worktree list --porcelain`
    vec![]
}
