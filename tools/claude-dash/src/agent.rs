//! Named-agent discovery for ws-framework registered agents.
//!
//! `discover_named_agents(git_root)` reads `*.json` files from
//! `.git/ws@<repo>/agents/`, extracts UUIDs, locates the corresponding
//! JSONL session files, and returns a list sorted by mtime descending
//! (newest first), truncated to 8 entries (slots Ctrl+2..Ctrl+9).

use std::fs;
use std::path::{Path, PathBuf};
use std::time::SystemTime;

use chrono::{DateTime, Local};

use crate::session::claude_projects_dir;
#[cfg(not(windows))]
use crate::session::discover_project_dirs;

/// A ws-framework registered named agent.
pub struct NamedAgent {
    /// File stem of `<name>.json` in the agents directory.
    pub name: String,
    /// Session UUID from the agent JSON file.
    pub uuid: String,
    /// Path to the JSONL session file under `~/.claude/projects/`.
    pub session_path: PathBuf,
    pub mtime: DateTime<Local>,
    /// Total tokens (input + output).  `None` until background parse completes.
    pub token_total: Option<u64>,
    /// Whether a background token-parse is in flight for the current mtime.
    pub parse_queued: bool,
}

/// Discover named agents registered under `.git/ws@<repo>/agents/`.
///
/// Returns agents sorted newest-first, capped at 8.
/// Returns an empty `Vec` when the directory does not exist or is unreadable.
pub fn discover_named_agents(git_root: &Path) -> Vec<NamedAgent> {
    let repo_name = match git_root.file_name() {
        Some(n) => n.to_string_lossy().into_owned(),
        None => return vec![],
    };
    let agents_dir = git_root
        .join(".git")
        .join(format!("ws@{}", repo_name))
        .join("agents");

    if !agents_dir.is_dir() {
        return vec![];
    }

    let entries = match fs::read_dir(&agents_dir) {
        Ok(e) => e,
        Err(_) => return vec![],
    };

    let mut agents: Vec<NamedAgent> = entries
        .flatten()
        .filter_map(|entry| {
            let path = entry.path();
            if path.extension().and_then(|e| e.to_str()) != Some("json") {
                return None;
            }
            let name = path.file_stem()?.to_string_lossy().into_owned();
            let content = fs::read_to_string(&path).ok()?;
            let json: serde_json::Value = serde_json::from_str(&content).ok()?;
            let uuid = json.get("uuid")?.as_str()?.to_string();

            // Locate the JSONL session file.
            let session_path = locate_session_file(&uuid, git_root)?;

            // mtime of the session file.
            let metadata = fs::metadata(&session_path).ok()?;
            let mtime: DateTime<Local> = metadata.modified().ok()?.into();

            Some(NamedAgent {
                name,
                uuid,
                session_path,
                mtime,
                token_total: None,
                parse_queued: false,
            })
        })
        .collect();

    // Sort newest first, truncate to 8 (slots Ctrl+2..9).
    agents.sort_by(|a, b| b.mtime.cmp(&a.mtime));
    agents.truncate(8);
    agents
}

/// Find the JSONL session file for `uuid`.
///
/// Checks the specific worktree's project directory first; falls back to
/// scanning all worktree project directories.
fn locate_session_file(uuid: &str, git_root: &Path) -> Option<PathBuf> {
    let filename = format!("{}.jsonl", uuid);

    // Primary: check the project dir corresponding to git_root.
    #[cfg(not(windows))]
    {
        let escaped = git_root.to_string_lossy().replace('/', "-");
        let primary = claude_projects_dir().join(escaped).join(&filename);
        if primary.is_file() {
            return Some(primary);
        }

        // Fallback: scan all worktree project dirs.
        for dir in discover_project_dirs() {
            let candidate = dir.join(&filename);
            if candidate.is_file() {
                return Some(candidate);
            }
        }
    }

    // Windows: scan all project subdirectories.
    #[cfg(windows)]
    {
        let base = claude_projects_dir();
        if let Ok(entries) = std::fs::read_dir(&base) {
            for entry in entries.flatten() {
                let candidate = entry.path().join(&filename);
                if candidate.is_file() {
                    return Some(candidate);
                }
            }
        }
    }

    None
}

/// Get the mtime of a file as `SystemTime`.
pub(crate) fn file_mtime(path: &Path) -> Option<SystemTime> {
    fs::metadata(path).ok()?.modified().ok()
}
