use std::collections::BTreeSet;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};
use tokio::fs;
use tracing::warn;

use crate::work_root_files::{OpenedWorkRoots, WorkRootProvenance};
use ws_dashboard_core::WorkRootActivation;

const OPENED_WORKROOTS_STATE_VERSION: u32 = 1;
const WORKROOT_REGISTRY_STATE_VERSION: u32 = 2;

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
        self.load_work_root_registry()
            .await
            .into_iter()
            .map(|entry| entry.path)
            .collect()
    }

    pub async fn load_work_root_registry(&self) -> Vec<PersistedRegistryWorkRoot> {
        let Some(path) = self.state_file.as_deref() else {
            return Vec::new();
        };

        match read_work_root_registry(path).await {
            Ok(entries) => entries,
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
        let pins = read_root_picker_pins(path).await.unwrap_or_default();
        write_work_root_registry(path, opened.candidate_roots(), pins).await
    }

    pub async fn load_root_picker_pins(&self) -> Vec<PathBuf> {
        let Some(path) = self.state_file.as_deref() else {
            return Vec::new();
        };

        match read_root_picker_pins(path).await {
            Ok(pins) => pins,
            Err(StateReadError::Missing) => Vec::new(),
            Err(error) => {
                warn!(%error, path = %path.display(), "ignoring dashboard root picker pins");
                Vec::new()
            }
        }
    }

    pub async fn persist_root_picker_pins(&self, pins: Vec<PathBuf>) -> Result<(), String> {
        let Some(path) = self.state_file.as_deref() else {
            return Ok(());
        };
        let registry = read_work_root_registry(path).await.unwrap_or_default();
        write_work_root_registry_from_entries(path, registry, pins).await
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

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PersistedRegistryWorkRoot {
    pub path: PathBuf,
    pub activation: WorkRootActivation,
    pub provenance: WorkRootProvenance,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct WorkRootRegistryState {
    version: u32,
    work_root_registry: Vec<PersistedWorkRootRegistryEntry>,
    #[serde(default)]
    root_picker_pins: Vec<PersistedRootPickerPin>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct PersistedRootPickerPin {
    path: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct PersistedWorkRootRegistryEntry {
    path: String,
    activation: WorkRootActivation,
    #[serde(default = "default_registry_provenance")]
    provenance: PersistedWorkRootProvenance,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
enum PersistedWorkRootProvenance {
    Opened,
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

async fn read_work_root_registry(
    path: &Path,
) -> Result<Vec<PersistedRegistryWorkRoot>, StateReadError> {
    read_dashboard_state_parts(path)
        .await
        .map(|parts| parts.work_root_registry)
}

async fn read_root_picker_pins(path: &Path) -> Result<Vec<PathBuf>, StateReadError> {
    read_dashboard_state_parts(path)
        .await
        .map(|parts| parts.root_picker_pins)
}

struct DashboardStateParts {
    work_root_registry: Vec<PersistedRegistryWorkRoot>,
    root_picker_pins: Vec<PathBuf>,
}

async fn read_dashboard_state_parts(path: &Path) -> Result<DashboardStateParts, StateReadError> {
    let raw = match fs::read_to_string(path).await {
        Ok(raw) => raw,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return Err(StateReadError::Missing);
        }
        Err(error) => return Err(StateReadError::Read(error)),
    };
    let value: serde_json::Value = serde_json::from_str(&raw).map_err(StateReadError::Parse)?;
    let version = value
        .get("version")
        .and_then(serde_json::Value::as_u64)
        .unwrap_or(0) as u32;
    if version == OPENED_WORKROOTS_STATE_VERSION {
        let state: OpenedWorkRootsState =
            serde_json::from_value(value).map_err(StateReadError::Parse)?;
        return Ok(DashboardStateParts {
            work_root_registry: deduplicate_paths(
                state
                    .opened_work_roots
                    .into_iter()
                    .filter_map(|entry| {
                        let trimmed = entry.path.trim();
                        (!trimmed.is_empty()).then(|| PathBuf::from(trimmed))
                    })
                    .collect(),
            )
            .into_iter()
            .map(|path| PersistedRegistryWorkRoot {
                path,
                activation: WorkRootActivation::Online,
                provenance: WorkRootProvenance::Opened,
            })
            .collect(),
            root_picker_pins: Vec::new(),
        });
    }
    if version != WORKROOT_REGISTRY_STATE_VERSION {
        return Err(StateReadError::UnsupportedVersion(version));
    }
    let state: WorkRootRegistryState =
        serde_json::from_value(value).map_err(StateReadError::Parse)?;
    Ok(DashboardStateParts {
        work_root_registry: deduplicate_registry_entries(
            state
                .work_root_registry
                .into_iter()
                .filter_map(|entry| {
                    let trimmed = entry.path.trim();
                    (!trimmed.is_empty()).then(|| PersistedRegistryWorkRoot {
                        path: PathBuf::from(trimmed),
                        activation: entry.activation,
                        provenance: entry.provenance.into(),
                    })
                })
                .collect(),
        ),
        root_picker_pins: deduplicate_paths(
            state
                .root_picker_pins
                .into_iter()
                .filter_map(|entry| {
                    let trimmed = entry.path.trim();
                    (!trimmed.is_empty()).then(|| PathBuf::from(trimmed))
                })
                .collect(),
        ),
    })
}

async fn write_work_root_registry(
    path: &Path,
    roots: Vec<crate::work_root_files::RegisteredWorkRoot>,
    root_picker_pins: Vec<PathBuf>,
) -> Result<(), String> {
    let entries = deduplicate_registry_entries(
        roots
            .into_iter()
            .filter(|root| root.provenance == WorkRootProvenance::Opened)
            .map(|root| PersistedRegistryWorkRoot {
                path: root.path,
                activation: root.activation,
                provenance: root.provenance,
            })
            .collect(),
    );
    write_work_root_registry_from_entries(path, entries, root_picker_pins).await
}

async fn write_work_root_registry_from_entries(
    path: &Path,
    roots: Vec<PersistedRegistryWorkRoot>,
    root_picker_pins: Vec<PathBuf>,
) -> Result<(), String> {
    let state = WorkRootRegistryState {
        version: WORKROOT_REGISTRY_STATE_VERSION,
        work_root_registry: deduplicate_registry_entries(roots)
            .into_iter()
            .map(|root| PersistedWorkRootRegistryEntry {
                path: root.path.to_string_lossy().into_owned(),
                activation: root.activation,
                provenance: root.provenance.into(),
            })
            .collect(),
        root_picker_pins: deduplicate_paths(root_picker_pins)
            .into_iter()
            .map(|path| PersistedRootPickerPin {
                path: path.to_string_lossy().into_owned(),
            })
            .collect(),
    };
    write_state_json(path, &state).await
}

fn default_registry_provenance() -> PersistedWorkRootProvenance {
    PersistedWorkRootProvenance::Opened
}

impl From<PersistedWorkRootProvenance> for WorkRootProvenance {
    fn from(provenance: PersistedWorkRootProvenance) -> Self {
        match provenance {
            PersistedWorkRootProvenance::Opened => Self::Opened,
        }
    }
}

impl From<WorkRootProvenance> for PersistedWorkRootProvenance {
    fn from(provenance: WorkRootProvenance) -> Self {
        match provenance {
            WorkRootProvenance::Opened => Self::Opened,
            WorkRootProvenance::Discovered => Self::Opened,
        }
    }
}

async fn write_state_json<T: Serialize>(path: &Path, state: &T) -> Result<(), String> {
    let raw = serde_json::to_string_pretty(state)
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

fn deduplicate_registry_entries(
    entries: Vec<PersistedRegistryWorkRoot>,
) -> Vec<PersistedRegistryWorkRoot> {
    let mut by_path = std::collections::BTreeMap::new();
    for entry in entries {
        by_path.insert(entry.path.clone(), entry);
    }
    by_path.into_values().collect()
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
        assert!(raw.contains("\"version\": 2"));
        assert!(raw.contains("\"workRootRegistry\""));
        assert!(raw.contains("\"rootPickerPins\""));
        assert!(raw.contains("\"activation\": \"online\""));
        assert!(raw.contains("\"provenance\": \"opened\""));
        remove_temp(&root);
    }

    #[tokio::test]
    async fn state_store_persists_root_picker_pins_without_dropping_registry() {
        let root = temp_path("pins");
        let state_file = root.join("opened-workroots.json");
        let store = DashboardStateStore::at_path(&state_file);
        let opened = OpenedWorkRoots::from_paths(vec![root.join("work")]);
        store
            .persist_opened_work_roots(&opened)
            .await
            .expect("persist opened workRoots before pins");

        store
            .persist_root_picker_pins(vec![root.join("pin-b"), root.join("pin-a")])
            .await
            .expect("persist root picker pins");

        let restored_registry = store.load_work_root_registry().await;
        assert_eq!(restored_registry.len(), 1);
        assert_eq!(restored_registry[0].path, root.join("work"));
        assert_eq!(
            store.load_root_picker_pins().await,
            vec![root.join("pin-a"), root.join("pin-b")]
        );

        let raw = fs::read_to_string(&state_file)
            .await
            .expect("read pinned state");
        assert!(raw.contains("\"rootPickerPins\""));
        assert!(raw.contains("pin-a"));
        remove_temp(&root);
    }

    #[tokio::test]
    async fn state_store_migrates_v1_opened_work_roots_as_online_registry_entries() {
        let root = temp_path("migrate");
        let state_file = root.join("opened-workroots.json");
        fs::create_dir_all(&root).await.expect("create state dir");
        fs::write(
            &state_file,
            serde_json::json!({
                "version": 1,
                "openedWorkRoots": [
                    { "path": root.join("legacy").to_string_lossy() }
                ]
            })
            .to_string(),
        )
        .await
        .expect("write v1 state");
        let store = DashboardStateStore::at_path(&state_file);

        let restored = store.load_work_root_registry().await;

        assert_eq!(restored.len(), 1);
        assert_eq!(restored[0].path, root.join("legacy"));
        assert_eq!(restored[0].activation, WorkRootActivation::Online);
        assert_eq!(restored[0].provenance, WorkRootProvenance::Opened);
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
