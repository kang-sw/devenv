use std::fs;
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
                    std::env::var("HOMEDRIVE").and_then(|d| {
                        std::env::var("HOMEPATH").map(|p| format!("{}{}", d, p))
                    })
                })
                .unwrap_or_else(|_| "C:\\Users\\default".to_string()),
        )
    }
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

/// Collect .jsonl files from the Claude projects directory.
///
/// Non-Windows: derive the project sub-directory from CWD by replacing every
/// `/` with `-` (matching Claude CLI's own path-escaping scheme).
///
/// Windows: Claude CLI's escaping differs for Windows paths (`C:\...`), so
/// the project sub-directory cannot be reliably derived.  Instead, all
/// `.jsonl` files from all project sub-directories are returned.  This is
/// intentional and matches the spec: "scan all `~/.claude/projects/`
/// subdirectories; match UUIDs across all of them regardless of subdirectory
/// name."
#[cfg(not(windows))]
pub fn collect_jsonl_files(claude_projects: &Path) -> Vec<PathBuf> {
    let cwd = match std::env::current_dir() {
        Ok(d) => d,
        Err(_) => return vec![],
    };
    let escaped = cwd.to_string_lossy().replace('/', "-");
    scan_jsonl_in_dir(&claude_projects.join(&escaped))
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
    let claude_projects = home_dir().join(".claude").join("projects");
    let git_root = find_git_root();

    let jsonl_files = collect_jsonl_files(&claude_projects);

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
            })
        })
        .collect();

    // Most recent first.
    sessions.sort_by(|a, b| b.modified.cmp(&a.modified));
    sessions
}
