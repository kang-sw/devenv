//! Named-agent discovery and JSONL session tracking (Phase 3+).
//!
//! Phase 1 stub — real implementation added in Phase 3.

use std::path::PathBuf;

use chrono::{DateTime, Local};

/// A ws-framework registered named agent.
#[allow(dead_code)]
pub struct NamedAgent {
    /// File stem of `<name>.json` in the agents directory.
    pub name: String,
    /// Session UUID from the agent JSON file.
    pub uuid: String,
    /// Path to the JSONL session file under `~/.claude/projects/`.
    pub session_path: PathBuf,
    pub mtime: DateTime<Local>,
    pub token_total: Option<u64>,
}

/// Discover named agents registered under `.git/ws@<repo>/agents/`.
///
/// Phase 1 stub: returns an empty list.  Real implementation in Phase 3.
pub fn discover_named_agents(_git_root: &std::path::Path) -> Vec<NamedAgent> {
    // TODO(phase-3): implement discovery
    vec![]
}
