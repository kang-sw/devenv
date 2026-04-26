// Adapted from tools/claude-watch/src/session.rs.
// Changes vs. original:
// - Removed `discover_sessions` and `collect_jsonl_files` (not used here;
//   agent.rs discovers sessions via the agent registry, not by walking
//   ~/.claude/projects/).
// - Changed `find_agent_name` and `discover_project_dirs` from private `fn`
//   to `pub(crate)` so `agent.rs` can call them.
// - `parse_session_metadata` was already `pub(crate)` — no change.

#[cfg(not(windows))]
use std::collections::HashMap;
use std::fs;
use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};

use chrono::{DateTime, Local};

/// A single discovered Claude session JSONL file.
#[derive(Debug, Clone)]
pub struct SessionEntry {
    pub label: String,
    pub uuid: String,
    pub modified: DateTime<Local>,
    pub path: PathBuf,
    pub active: bool,
    /// Total tokens (input + output) summed from assistant turns.
    /// `None` means the file has not been parsed yet.
    pub token_total: Option<u64>,
    /// Whether this session was started with `-p` (headless/SDK-CLI mode).
    /// `None` means the file has not been parsed yet.
    pub is_headless: Option<bool>,
    /// Whether a background parse has been enqueued for this session's current
    /// mtime.
    pub parse_queued: bool,
}

fn home_dir() -> PathBuf {
    #[cfg(not(windows))]
    {
        PathBuf::from(std::env::var("HOME").unwrap_or_else(|_| "/root".to_string()))
    }
    #[cfg(windows)]
    {
        PathBuf::from(
            std::env::var("USERPROFILE")
                .or_else(|_| {
                    std::env::var("HOMEDRIVE")
                        .and_then(|d| std::env::var("HOMEPATH").map(|p| format!("{}{}", d, p)))
                })
                .unwrap_or_else(|_| "C:\\Users\\default".to_string()),
        )
    }
}

/// Return the `~/.claude/projects` base directory used by the Claude CLI.
pub(crate) fn claude_projects_dir() -> PathBuf {
    home_dir().join(".claude").join("projects")
}

/// Walk up from cwd to find the nearest .git directory root.
pub fn find_git_root() -> Option<PathBuf> {
    let mut dir = std::env::current_dir().ok()?;
    loop {
        if dir.join(".git").is_dir() {
            return Some(dir);
        }
        if !dir.pop() {
            return None;
        }
    }
}

/// Search agent JSON files under `.git/ws@<repo>/agents/` for a UUID match.
pub(crate) fn find_agent_name(uuid: &str, git_root: &Path) -> Option<String> {
    let repo_name = git_root.file_name()?.to_string_lossy().into_owned();
    let agents_dir = git_root
        .join(".git")
        .join(format!("ws@{}", repo_name))
        .join("agents");

    if !agents_dir.is_dir() {
        return None;
    }

    for entry in fs::read_dir(&agents_dir).ok()?.flatten() {
        let path = entry.path();
        if path.extension().and_then(|e| e.to_str()) != Some("json") {
            continue;
        }
        let content = fs::read_to_string(&path).unwrap_or_default();
        if let Ok(json) = serde_json::from_str::<serde_json::Value>(&content) {
            let stored = json.get("uuid").and_then(|v| v.as_str());
            if stored == Some(uuid) {
                if let Some(stem) = path.file_stem() {
                    return Some(stem.to_string_lossy().into_owned());
                }
            }
        }
    }
    None
}

/// Return the `~/.claude/projects/<escaped-cwd>` path for the current working
/// directory.
#[cfg(not(windows))]
fn cwd_project_dir() -> PathBuf {
    let cwd = std::env::current_dir().unwrap_or_else(|_| PathBuf::from("/"));
    let escaped = cwd.to_string_lossy().replace('/', "-");
    claude_projects_dir().join(escaped)
}

/// Return all `~/.claude/projects/<escaped>` directories that correspond to
/// git worktrees of the current repo.
#[cfg(not(windows))]
pub(crate) fn discover_project_dirs() -> Vec<PathBuf> {
    let output = std::process::Command::new("git")
        .args(["worktree", "list", "--porcelain"])
        .output();

    let output = match output {
        Ok(o) if o.status.success() => o,
        _ => return vec![cwd_project_dir()],
    };

    let stdout = String::from_utf8_lossy(&output.stdout);
    let base = claude_projects_dir();

    let dirs: Vec<PathBuf> = stdout
        .lines()
        .filter_map(|line| {
            let path_str = line.strip_prefix("worktree ")?;
            let escaped = path_str.replace('/', "-");
            Some(base.join(escaped))
        })
        .filter(|p| p.is_dir())
        .collect();

    if dirs.is_empty() {
        vec![cwd_project_dir()]
    } else {
        dirs
    }
}

/// Parse a session JSONL file and return `(total_tokens, is_headless)`.
///
/// Tokens are summed from every `assistant` entry's `message.usage` object.
/// `is_headless` is set when the file contains a `last-prompt` entry or a
/// `system` entry whose `entrypoint` field contains `"sdk-cli"`.
///
/// Always returns `Some`; `None` only when the file cannot be opened.
pub(crate) fn parse_session_metadata(path: &Path) -> Option<(u64, bool)> {
    let file = std::fs::File::open(path).ok()?;
    let reader = BufReader::new(file);

    let mut total_tokens: u64 = 0;
    let mut is_headless = false;

    for line in reader.lines() {
        let line = match line {
            Ok(l) => l,
            Err(_) => continue,
        };
        let v: serde_json::Value = match serde_json::from_str(&line) {
            Ok(v) => v,
            Err(_) => continue,
        };
        let obj = match v.as_object() {
            Some(o) => o,
            None => continue,
        };

        let entry_type = obj.get("type").and_then(|t| t.as_str()).unwrap_or("");

        match entry_type {
            "assistant" => {
                if let Some(usage) = v.get("message").and_then(|m| m.get("usage")) {
                    let get =
                        |key: &str| -> u64 { usage.get(key).and_then(|v| v.as_u64()).unwrap_or(0) };
                    total_tokens += get("input_tokens");
                    total_tokens += get("output_tokens");
                }
            }
            "last-prompt" => {
                is_headless = true;
            }
            "system" => {
                let entrypoint = obj.get("entrypoint").and_then(|e| e.as_str()).unwrap_or("");
                if entrypoint.contains("sdk-cli") {
                    is_headless = true;
                }
            }
            _ => {}
        }
    }

    Some((total_tokens, is_headless))
}
