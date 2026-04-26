// Adapted from tools/claude-watch/src/session.rs.
// Changes vs. original:
// - Removed `discover_sessions` and `collect_jsonl_files` (not used here;
//   agent.rs discovers sessions via the agent registry, not by walking
//   ~/.claude/projects/).
// - Removed `SessionEntry` (its producer `discover_sessions` was dropped;
//   no caller in this crate).
// - Removed `find_agent_name` (dead code; agent.rs implements its own
//   agents-directory walk via `discover_named_agents`).
// - Changed `discover_project_dirs` from private `fn` to `pub(crate)` so
//   `agent.rs` can call it.
// - `parse_session_metadata` was already `pub(crate)` — no change.

use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};

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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};

    static TEMP_CTR: AtomicUsize = AtomicUsize::new(0);

    fn write_temp_jsonl(data: &str) -> std::path::PathBuf {
        let n = TEMP_CTR.fetch_add(1, Ordering::Relaxed);
        let path = std::env::temp_dir().join(format!("cldash_session_test_{}.jsonl", n));
        std::fs::write(&path, data).unwrap();
        path
    }

    #[test]
    fn parse_session_metadata_file_not_found() {
        let result = parse_session_metadata(std::path::Path::new("/nonexistent/__x__.jsonl"));
        assert!(result.is_none());
    }

    #[test]
    fn parse_session_metadata_empty_file() {
        let p = write_temp_jsonl("");
        let result = parse_session_metadata(&p);
        let _ = std::fs::remove_file(&p);
        assert_eq!(result, Some((0, false)));
    }

    #[test]
    fn parse_session_metadata_all_error_lines() {
        let p = write_temp_jsonl("not json\nalso not json\n");
        let result = parse_session_metadata(&p);
        let _ = std::fs::remove_file(&p);
        assert_eq!(result, Some((0, false)));
    }

    #[test]
    fn parse_session_metadata_token_accumulation() {
        // Two assistant entries; tokens should be summed.
        let data = concat!(
            r#"{"type":"assistant","message":{"usage":{"input_tokens":10,"output_tokens":5}}}"#,
            "\n",
            r#"{"type":"assistant","message":{"usage":{"input_tokens":20,"output_tokens":15}}}"#,
            "\n",
        );
        let p = write_temp_jsonl(data);
        let result = parse_session_metadata(&p);
        let _ = std::fs::remove_file(&p);
        assert_eq!(result, Some((50, false)));
    }

    #[test]
    fn parse_session_metadata_headless_via_last_prompt() {
        let data = r#"{"type":"last-prompt"}"#;
        let p = write_temp_jsonl(data);
        let result = parse_session_metadata(&p);
        let _ = std::fs::remove_file(&p);
        assert_eq!(result, Some((0, true)));
    }

    #[test]
    fn parse_session_metadata_headless_via_system_entrypoint() {
        let data = r#"{"type":"system","entrypoint":"sdk-cli/0.1"}"#;
        let p = write_temp_jsonl(data);
        let result = parse_session_metadata(&p);
        let _ = std::fs::remove_file(&p);
        assert_eq!(result, Some((0, true)));
    }

    #[test]
    fn parse_session_metadata_not_headless_without_marker() {
        let data = r#"{"type":"system","entrypoint":"interactive"}"#;
        let p = write_temp_jsonl(data);
        let result = parse_session_metadata(&p);
        let _ = std::fs::remove_file(&p);
        assert_eq!(result, Some((0, false)));
    }
}
