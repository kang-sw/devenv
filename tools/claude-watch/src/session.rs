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
    /// Total tokens (input + output + cache_read) summed from assistant turns.
    /// `cache_creation_input_tokens` is excluded.
    /// `None` means the file has not been parsed yet.
    pub token_total: Option<u64>,
    /// Whether this session was started with `-p` (headless/SDK-CLI mode).
    /// `None` means the file has not been parsed yet.
    pub is_headless: Option<bool>,
    /// Whether a background parse has been enqueued for this session's current
    /// mtime.  Set to `true` only after a successful `try_send`.  Reset to
    /// `false` on two paths:
    /// - `poll_token_results`: when a result for the current mtime arrives
    ///   (covers both successful parses and parse failures).
    /// - `refresh_sessions`: when the file's mtime changes, resetting the
    ///   entire parsing state so the new version is re-queued.
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
fn claude_projects_dir() -> PathBuf {
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
///
/// The WS framework stores the session UUID in the `"uuid"` field of each
/// agent JSON.  Only this field is checked — a substring search on the raw
/// file text would risk false positives from UUIDs that appear in other
/// fields (e.g. `previous_session_id`, log messages).
fn find_agent_name(uuid: &str, git_root: &Path) -> Option<String> {
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
        // Parse the JSON and check only the "uuid" field to avoid false
        // positives from UUIDs referenced in other fields (C-4 fix).
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

fn scan_jsonl_in_dir(dir: &Path) -> Vec<PathBuf> {
    if !dir.is_dir() {
        return vec![];
    }
    fs::read_dir(dir)
        .into_iter()
        .flatten()
        .flatten()
        .map(|e| e.path())
        .filter(|p| p.extension().and_then(|e| e.to_str()) == Some("jsonl"))
        .collect()
}

/// Return the `~/.claude/projects/<escaped-cwd>` path for the current working
/// directory.  This mirrors the escape scheme used by the Claude CLI
/// (every `/` replaced with `-`).
#[cfg(not(windows))]
fn cwd_project_dir() -> PathBuf {
    let cwd = std::env::current_dir().unwrap_or_else(|_| PathBuf::from("/"));
    let escaped = cwd.to_string_lossy().replace('/', "-");
    claude_projects_dir().join(escaped)
}

/// Return all `~/.claude/projects/<escaped>` directories that correspond to
/// git worktrees of the current repo.
///
/// Steps:
/// 1. Run `git worktree list --porcelain`.  On failure fall back to
///    `vec![cwd_project_dir()]`.
/// 2. Parse lines beginning with `"worktree "` to obtain absolute worktree
///    paths.
/// 3. Escape each path (replace every `/` with `-`) and construct the
///    corresponding `~/.claude/projects/<escaped>` path.
/// 4. Keep only paths where the directory already exists.
/// 5. If the resulting list is empty, fall back to `vec![cwd_project_dir()]`.
#[cfg(not(windows))]
fn discover_project_dirs() -> Vec<PathBuf> {
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

/// Collect .jsonl files from the Claude projects directory.
///
/// Non-Windows: discover all git worktree project directories via
/// `discover_project_dirs()` and collect JSONL files from each.  Files are
/// deduplicated by UUID (the filename stem) in case the same session file
/// appears under more than one directory.
///
/// Windows: Claude CLI's escaping differs for Windows paths (`C:\...`), so
/// the project sub-directory cannot be reliably derived.  Instead, all
/// `.jsonl` files from all project sub-directories are returned.  This is
/// intentional and matches the spec: "scan all `~/.claude/projects/`
/// subdirectories; match UUIDs across all of them regardless of subdirectory
/// name."
#[cfg(not(windows))]
pub fn collect_jsonl_files() -> Vec<PathBuf> {
    let dirs = discover_project_dirs();
    let mut seen: HashMap<String, PathBuf> = HashMap::new();
    for dir in dirs {
        for path in scan_jsonl_in_dir(&dir) {
            if let Some(stem) = path.file_stem().and_then(|s| s.to_str()) {
                seen.entry(stem.to_string()).or_insert(path);
            }
        }
    }
    seen.into_values().collect()
}

#[cfg(windows)]
pub fn collect_jsonl_files(claude_projects: &Path) -> Vec<PathBuf> {
    // All sessions across all projects — intentional on Windows (see above).
    let mut results = Vec::new();
    if let Ok(entries) = fs::read_dir(claude_projects) {
        for entry in entries.flatten() {
            let path = entry.path();
            if path.is_dir() {
                results.extend(scan_jsonl_in_dir(&path));
            }
        }
    }
    results
}

/// Discover all session JSONL files for the current project.
pub fn discover_sessions() -> Vec<SessionEntry> {
    let git_root = find_git_root();

    #[cfg(not(windows))]
    let jsonl_files = collect_jsonl_files();
    #[cfg(windows)]
    let jsonl_files = collect_jsonl_files(&claude_projects_dir());

    let mut sessions: Vec<SessionEntry> = jsonl_files
        .into_iter()
        .filter_map(|path| {
            let stem = path.file_stem()?.to_string_lossy().into_owned();
            let uuid = stem.clone();

            let metadata = fs::metadata(&path).ok()?;
            let modified: DateTime<Local> = metadata.modified().ok()?.into();

            let label = git_root
                .as_ref()
                .and_then(|root| find_agent_name(&uuid, root))
                .unwrap_or_else(|| uuid.chars().take(8).collect());

            Some(SessionEntry {
                label,
                uuid,
                modified,
                path,
                active: false,
                token_total: None,
                is_headless: None,
                parse_queued: false,
            })
        })
        .collect();

    // Most recent first.
    sessions.sort_by(|a, b| b.modified.cmp(&a.modified));
    sessions
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
                    total_tokens += get("cache_read_input_tokens");
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
