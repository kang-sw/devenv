use std::collections::BTreeSet;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};
use tokio::fs;
use tracing::warn;

use crate::work_root_files::OpenedWorkRoots;

const OPENED_WORKROOTS_STATE_VERSION: u32 = 1;

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct DashboardStateStore {
    state_file: Option<PathBuf>,
}

impl DashboardStateStore {
    pub fn disabled() -> Self {
        Self { state_file: None }
    }

    pub fn at_path(path: impl Into<PathBuf>) -> Self {
        Self {
            state_file: Some(path.into()),
        }
    }

    pub fn default_local() -> Self {
        match default_state_file() {
            Some(path) => Self::at_path(path),
            None => Self::disabled(),
        }
    }

    pub async fn load_opened_work_roots(&self) -> Vec<PathBuf> {
        let Some(path) = self.state_file.as_deref() else {
            return Vec::new();
        };

        match read_opened_work_roots(path).await {
            Ok(paths) => paths,
            Err(StateReadError::Missing) => Vec::new(),
            Err(error) => {
                warn!(%error, path = %path.display(), "ignoring dashboard state file");
                Vec::new()
            }
        }
    }

    pub async fn persist_opened_work_roots(&self, opened: &OpenedWorkRoots) -> Result<(), String> {
        let Some(path) = self.state_file.as_deref() else {
            return Ok(());
        };
        write_opened_work_roots(path, opened.candidate_paths()).await
    }
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct OpenedWorkRootsState {
    version: u32,
    opened_work_roots: Vec<PersistedWorkRoot>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct PersistedWorkRoot {
    path: String,
}

#[derive(Debug)]
enum StateReadError {
    Missing,
    Read(std::io::Error),
    Parse(serde_json::Error),
    UnsupportedVersion(u32),
}

impl std::fmt::Display for StateReadError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Missing => write!(formatter, "state file missing"),
            Self::Read(error) => write!(formatter, "state read failed: {error}"),
            Self::Parse(error) => write!(formatter, "state parse failed: {error}"),
            Self::UnsupportedVersion(version) => {
                write!(formatter, "unsupported state version {version}")
            }
        }
    }
}

async fn read_opened_work_roots(path: &Path) -> Result<Vec<PathBuf>, StateReadError> {
    let raw = match fs::read_to_string(path).await {
        Ok(raw) => raw,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return Err(StateReadError::Missing);
        }
        Err(error) => return Err(StateReadError::Read(error)),
    };
    let state: OpenedWorkRootsState = serde_json::from_str(&raw).map_err(StateReadError::Parse)?;
    if state.version != OPENED_WORKROOTS_STATE_VERSION {
        return Err(StateReadError::UnsupportedVersion(state.version));
    }

    Ok(deduplicate_paths(
        state
            .opened_work_roots
            .into_iter()
            .filter_map(|entry| {
                let trimmed = entry.path.trim();
                (!trimmed.is_empty()).then(|| PathBuf::from(trimmed))
            })
            .collect(),
    ))
}

async fn write_opened_work_roots(path: &Path, paths: Vec<PathBuf>) -> Result<(), String> {
    let state = OpenedWorkRootsState {
        version: OPENED_WORKROOTS_STATE_VERSION,
        opened_work_roots: deduplicate_paths(paths)
            .into_iter()
            .map(|path| PersistedWorkRoot {
                path: path.to_string_lossy().into_owned(),
            })
            .collect(),
    };
    let raw = serde_json::to_string_pretty(&state)
        .map_err(|error| format!("serialize state failed: {error}"))?;

    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)
            .await
            .map_err(|error| format!("create state directory failed: {error}"))?;
    }

    let temp_path = path.with_extension("json.tmp");
    fs::write(&temp_path, raw)
        .await
        .map_err(|error| format!("write state file failed: {error}"))?;
    fs::rename(&temp_path, path)
        .await
        .map_err(|error| format!("replace state file failed: {error}"))?;
    Ok(())
}

fn deduplicate_paths(paths: Vec<PathBuf>) -> Vec<PathBuf> {
    let mut seen = BTreeSet::new();
    for path in paths {
        seen.insert(path);
    }
    seen.into_iter().collect()
}

fn default_state_file() -> Option<PathBuf> {
    if let Some(path) = std::env::var_os("WS_DASHBOARD_STATE_FILE") {
        return Some(PathBuf::from(path));
    }
    if let Some(path) = std::env::var_os("WS_DASHBOARD_STATE_HOME") {
        return Some(PathBuf::from(path).join("opened-workroots.json"));
    }
    if let Some(path) = std::env::var_os("XDG_STATE_HOME") {
        return Some(PathBuf::from(path).join("ws-dashboard/opened-workroots.json"));
    }
    std::env::var_os("HOME")
        .map(PathBuf::from)
        .map(|home| home.join(".local/state/ws-dashboard/opened-workroots.json"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU64, Ordering};
    use std::time::{SystemTime, UNIX_EPOCH};

    static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

    #[tokio::test]
    async fn state_store_persists_deduplicated_opened_work_roots() {
        let root = temp_path("persist");
        let state_file = root.join("state/opened-workroots.json");
        let store = DashboardStateStore::at_path(&state_file);
        let opened =
            OpenedWorkRoots::from_paths(vec![root.join("b"), root.join("a"), root.join("b")]);

        store
            .persist_opened_work_roots(&opened)
            .await
            .expect("persist opened workRoots");
        let restored = store.load_opened_work_roots().await;

        assert_eq!(restored, vec![root.join("a"), root.join("b")]);
        let raw = fs::read_to_string(&state_file)
            .await
            .expect("read persisted state");
        assert!(raw.contains("\"version\": 1"));
        remove_temp(&root);
    }

    #[tokio::test]
    async fn state_store_degrades_missing_and_malformed_state_to_empty() {
        let root = temp_path("malformed");
        let state_file = root.join("opened-workroots.json");
        let store = DashboardStateStore::at_path(&state_file);

        assert!(store.load_opened_work_roots().await.is_empty());

        fs::create_dir_all(&root)
            .await
            .expect("create malformed state dir");
        fs::write(&state_file, "not json")
            .await
            .expect("write malformed state");

        assert!(store.load_opened_work_roots().await.is_empty());
        remove_temp(&root);
    }

    fn temp_path(label: &str) -> PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        let nanos = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system time after epoch")
            .as_nanos();
        std::env::temp_dir().join(format!("ws-dashboard-state-{label}-{nanos}-{unique}"))
    }

    fn remove_temp(path: &Path) {
        let _ = std::fs::remove_dir_all(path);
    }
}
