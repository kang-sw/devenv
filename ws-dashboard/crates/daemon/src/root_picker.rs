use std::collections::{BTreeSet, HashMap};
use std::fs;
use std::path::{Component, Path, PathBuf};
use std::time::UNIX_EPOCH;

use axum::extract::{Query, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::{Deserialize, Serialize};
use ws_dashboard_core::{
    DashboardResourcesView, WorkRootActivation, WorkRootAvailability, WorkRootId, WorkspaceId,
};

use crate::discovery::{LocalDashboardResourcesProvider, LocalWorkRootCandidate};
use crate::resources::{local_dashboard_resources_view, DashboardResourcesProvider};
use crate::router::AppState;
use crate::work_root_activity::normalize_display_path;
use crate::work_root_files::RegisteredWorkRoot;

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RootPickerView {
    pub current_path: String,
    pub parent_path: Option<String>,
    pub entries: Vec<RootPickerEntry>,
    pub places: Vec<RootPickerPlace>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RootPickerEntry {
    pub name: String,
    pub path: String,
    pub entry_type: RootPickerEntryType,
    pub selectable: bool,
    pub kind_label: Option<String>,
    pub modified_time: Option<String>,
    pub size: Option<u64>,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum RootPickerEntryType {
    Directory,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RootPickerPlace {
    pub id: String,
    pub label: String,
    pub path: String,
    pub kind: RootPickerPlaceKind,
    pub source: RootPickerPlaceSource,
    pub available: bool,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum RootPickerPlaceKind {
    Home,
    Root,
    Mount,
    Drive,
    Pin,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum RootPickerPlaceSource {
    BuiltIn,
    Pin,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RootPickerPlacesView {
    pub places: Vec<RootPickerPlace>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct CreateEmptyDirectoryRequest {
    pub parent_path: String,
    pub name: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct OpenWorkRootRequest {
    pub path: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct RootPickerPinRequest {
    pub path: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct SetWorkRootActivationRequest {
    pub activation: WorkRootActivation,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct RootPickerError {
    error: String,
}

pub async fn list_root_picker(
    State(state): State<AppState>,
    Query(query): Query<HashMap<String, String>>,
) -> Response {
    let path = query
        .get("path")
        .map(PathBuf::from)
        .unwrap_or_else(default_picker_path);
    let pins = state.dashboard_state.load_root_picker_pins().await;

    // Listing runs synchronous filesystem work (canonicalize/metadata/read_dir
    // across the target dir, home, drive letters, mounts, and pins), so keep
    // it off the async worker threads.
    let view = tokio::task::spawn_blocking(move || root_picker_view(&path, pins))
        .await
        .expect("root picker listing task panicked");

    match view {
        Ok(view) => Json(view).into_response(),
        Err(error) => picker_error(StatusCode::BAD_REQUEST, error),
    }
}

pub async fn create_empty_directory(Json(request): Json<CreateEmptyDirectoryRequest>) -> Response {
    let Some(name) = safe_child_name(&request.name) else {
        return picker_error(
            StatusCode::BAD_REQUEST,
            "directory name must be one path segment",
        );
    };

    let parent = PathBuf::from(request.parent_path);
    let path = parent.join(name);

    match fs::create_dir(&path) {
        Ok(()) => Json(entry_for_directory(&path)).into_response(),
        Err(error) => picker_error(StatusCode::BAD_REQUEST, format!("create failed: {error}")),
    }
}

pub async fn pin_root_picker_directory(
    State(state): State<AppState>,
    Json(request): Json<RootPickerPinRequest>,
) -> Response {
    let Some(path) = clean_pin_path(&request.path) else {
        return picker_error(StatusCode::BAD_REQUEST, "pin path is required");
    };

    let _persist_guard = state.registry_persist_lock.lock().await;
    let mut pins = state.dashboard_state.load_root_picker_pins().await;
    pins.push(path);
    pins = deduplicate_pin_paths(pins);
    if let Err(error) = state
        .dashboard_state
        .persist_root_picker_pins(pins.clone())
        .await
    {
        tracing::warn!(%error, "failed to persist dashboard root picker pins");
        return picker_error(
            StatusCode::INTERNAL_SERVER_ERROR,
            "persist root picker pins failed",
        );
    }
    Json(RootPickerPlacesView {
        places: known_picker_places(pins),
    })
    .into_response()
}

pub async fn unpin_root_picker_directory(
    State(state): State<AppState>,
    Json(request): Json<RootPickerPinRequest>,
) -> Response {
    let Some(path) = clean_pin_path(&request.path) else {
        return picker_error(StatusCode::BAD_REQUEST, "pin path is required");
    };

    let target = path.display().to_string();
    let _persist_guard = state.registry_persist_lock.lock().await;
    let pins: Vec<PathBuf> = state
        .dashboard_state
        .load_root_picker_pins()
        .await
        .into_iter()
        .filter(|pin| pin.display().to_string() != target)
        .collect();
    if let Err(error) = state
        .dashboard_state
        .persist_root_picker_pins(pins.clone())
        .await
    {
        tracing::warn!(%error, "failed to persist dashboard root picker pins");
        return picker_error(
            StatusCode::INTERNAL_SERVER_ERROR,
            "persist root picker pins failed",
        );
    }
    Json(RootPickerPlacesView {
        places: known_picker_places(pins),
    })
    .into_response()
}

pub async fn open_work_root(
    State(state): State<AppState>,
    Json(request): Json<OpenWorkRootRequest>,
) -> Response {
    let requested_path = PathBuf::from(request.path);
    let requested_path = PathBuf::from(normalize_display_path(&requested_path));
    let provider = LocalDashboardResourcesProvider::new(vec![LocalWorkRootCandidate::new(
        requested_path.clone(),
    )]);
    // Discovery runs synchronous filesystem and `git` subprocess work, so
    // keep it off the async worker threads.
    let view = tokio::task::spawn_blocking(move || provider.dashboard_resources())
        .await
        .expect("workRoot discovery task panicked");
    let Some(work_root) = view
        .workspaces
        .first()
        .and_then(|workspace| workspace.work_roots.first())
    else {
        return picker_error(StatusCode::BAD_REQUEST, "workRoot was not discovered");
    };

    if work_root.availability != WorkRootAvailability::Available {
        return picker_error(
            StatusCode::BAD_REQUEST,
            work_root
                .state
                .error
                .clone()
                .unwrap_or_else(|| work_root.state.status.clone()),
        );
    }

    let opened_work_root_id = work_root.id.clone();
    let _persist_guard = state.registry_persist_lock.lock().await;
    let previous_entry = state.opened_work_roots.register_registry_entry(
        opened_work_root_id.clone(),
        RegisteredWorkRoot {
            path: requested_path,
            activation: WorkRootActivation::Online,
            provenance: crate::work_root_files::WorkRootProvenance::Opened,
        },
    );
    if let Err(error) = state
        .dashboard_state
        .persist_opened_work_roots(&state.opened_work_roots)
        .await
    {
        match previous_entry {
            Some(previous) => {
                state
                    .opened_work_roots
                    .register_registry_entry(opened_work_root_id, previous);
            }
            None => {
                state.opened_work_roots.unregister(&opened_work_root_id);
            }
        }
        tracing::warn!(%error, "failed to persist opened dashboard workRoots");
        return picker_error(StatusCode::INTERNAL_SERVER_ERROR, "persist workRoot failed");
    }

    // CONTRACT: return the aggregated live view of every opened workRoot so the
    // immediate open response is consistent with later GET /api/dashboard/resources
    // refreshes. The single-candidate `view` above is only the Online gate.
    let aggregated = local_dashboard_resources_view(&state).await;
    (
        [(
            "x-ws-dashboard-opened-work-root-id",
            opened_work_root_id.as_str().to_owned(),
        )],
        Json::<DashboardResourcesView>(aggregated),
    )
        .into_response()
}

pub async fn set_work_root_activation(
    State(state): State<AppState>,
    axum::extract::Path(work_root_id): axum::extract::Path<String>,
    Json(request): Json<SetWorkRootActivationRequest>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let _persist_guard = state.registry_persist_lock.lock().await;
    let Some(previous_activation) = state
        .opened_work_roots
        .set_activation(&work_root_id, request.activation)
    else {
        return picker_error(StatusCode::NOT_FOUND, "unknown workRoot");
    };
    if let Err(error) = state
        .dashboard_state
        .persist_opened_work_roots(&state.opened_work_roots)
        .await
    {
        state
            .opened_work_roots
            .set_activation(&work_root_id, previous_activation);
        tracing::warn!(%error, "failed to persist dashboard workRoot registry");
        return picker_error(
            StatusCode::INTERNAL_SERVER_ERROR,
            "persist activation failed",
        );
    }
    Json::<DashboardResourcesView>(local_dashboard_resources_view(&state).await)
        .into_response()
}

pub async fn remove_workspace(
    State(state): State<AppState>,
    axum::extract::Path(workspace_id): axum::extract::Path<String>,
) -> Response {
    let workspace_id = WorkspaceId::from(workspace_id);
    let _persist_guard = state.registry_persist_lock.lock().await;
    let current = local_dashboard_resources_view(&state).await;
    let Some(workspace) = current
        .workspaces
        .iter()
        .find(|workspace| workspace.id == workspace_id)
    else {
        return picker_error(StatusCode::NOT_FOUND, "unknown workspace");
    };
    let work_root_ids: BTreeSet<WorkRootId> = workspace
        .work_roots
        .iter()
        .map(|root| root.id.clone())
        .collect();
    let removed_entries: Vec<_> = work_root_ids
        .iter()
        .filter_map(|work_root_id| {
            state
                .opened_work_roots
                .unregister(work_root_id)
                .map(|root| (work_root_id.clone(), root))
        })
        .collect();
    if let Err(error) = state
        .dashboard_state
        .persist_opened_work_roots(&state.opened_work_roots)
        .await
    {
        for (work_root_id, root) in removed_entries {
            state
                .opened_work_roots
                .register_registry_entry(work_root_id, root);
        }
        tracing::warn!(%error, "failed to persist dashboard workspace removal");
        return picker_error(
            StatusCode::INTERNAL_SERVER_ERROR,
            "persist workspace removal failed",
        );
    }
    state.terminals.remove_for_work_roots(&work_root_ids);
    state.codex_sessions.remove_for_work_roots(&work_root_ids);
    state.claude_sessions.remove_for_work_roots(&work_root_ids);
    Json::<DashboardResourcesView>(local_dashboard_resources_view(&state).await)
        .into_response()
}

fn root_picker_view(path: &Path, root_picker_pins: Vec<PathBuf>) -> Result<RootPickerView, String> {
    let path = path
        .canonicalize()
        .map_err(|error| format!("path unavailable: {error}"))?;
    let metadata = fs::metadata(&path).map_err(|error| format!("metadata failed: {error}"))?;
    if !metadata.is_dir() {
        return Err("path is not a directory".to_owned());
    }

    let mut entries = Vec::new();
    for entry in fs::read_dir(&path).map_err(|error| format!("read failed: {error}"))? {
        let entry = entry.map_err(|error| format!("entry failed: {error}"))?;
        let entry_path = entry.path();
        if entry
            .file_type()
            .map_err(|error| format!("entry type failed: {error}"))?
            .is_dir()
        {
            entries.push(entry_for_directory(&entry_path));
        }
    }
    entries.sort_by(|left, right| left.name.cmp(&right.name));

    Ok(RootPickerView {
        current_path: normalize_display_path(&path),
        parent_path: path.parent().map(normalize_display_path),
        places: known_picker_places(root_picker_pins),
        entries,
    })
}

fn entry_for_directory(path: &Path) -> RootPickerEntry {
    let metadata = fs::metadata(path).ok();
    RootPickerEntry {
        name: path
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("")
            .to_owned(),
        path: normalize_display_path(path),
        entry_type: RootPickerEntryType::Directory,
        selectable: true,
        kind_label: Some("Folder".to_owned()),
        modified_time: metadata
            .as_ref()
            .and_then(|metadata| metadata.modified().ok())
            .and_then(|modified| modified.duration_since(UNIX_EPOCH).ok())
            .map(|duration| duration.as_secs().to_string()),
        size: None,
    }
}

fn known_picker_places(root_picker_pins: Vec<PathBuf>) -> Vec<RootPickerPlace> {
    let mut places = Vec::new();
    let mut seen = BTreeSet::new();

    if let Some(home) = home_directory() {
        push_place(
            &mut places,
            &mut seen,
            "home",
            "Home",
            home,
            RootPickerPlaceKind::Home,
        );
    }

    for root in filesystem_roots() {
        let label = if cfg!(windows) {
            root.display().to_string()
        } else {
            "File system".to_owned()
        };
        let id = format!("root-{}", place_id_fragment(&root));
        push_place(
            &mut places,
            &mut seen,
            &id,
            &label,
            root,
            RootPickerPlaceKind::Root,
        );
    }

    for mount in common_mount_roots() {
        let label = mount
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("Mounts")
            .to_owned();
        let id = format!("mount-{}", place_id_fragment(&mount));
        push_place(
            &mut places,
            &mut seen,
            &id,
            &label,
            mount,
            RootPickerPlaceKind::Mount,
        );
    }

    for pin in deduplicate_pin_paths(root_picker_pins) {
        push_pin_place(&mut places, &mut seen, pin);
    }

    places
}

fn push_place(
    places: &mut Vec<RootPickerPlace>,
    seen: &mut BTreeSet<String>,
    id: &str,
    label: &str,
    path: PathBuf,
    kind: RootPickerPlaceKind,
) {
    let Ok(canonical) = path.canonicalize() else {
        return;
    };
    if !canonical.is_dir() {
        return;
    }
    let display_path = normalize_display_path(&canonical);
    if !seen.insert(display_path.clone()) {
        return;
    }
    places.push(RootPickerPlace {
        id: id.to_owned(),
        label: label.to_owned(),
        path: display_path,
        kind,
        source: RootPickerPlaceSource::BuiltIn,
        available: true,
    });
}

fn push_pin_place(places: &mut Vec<RootPickerPlace>, seen: &mut BTreeSet<String>, path: PathBuf) {
    let (display_path, available) = match path.canonicalize() {
        Ok(canonical) if canonical.is_dir() => (normalize_display_path(&canonical), true),
        _ => (normalize_display_path(&path), false),
    };
    let seen_key = format!("pin:{display_path}");
    if !seen.insert(seen_key) {
        return;
    }
    places.push(RootPickerPlace {
        id: format!("pin-{}", place_id_fragment(Path::new(&display_path))),
        label: root_picker_path_label(Path::new(&display_path)),
        path: display_path,
        kind: RootPickerPlaceKind::Pin,
        source: RootPickerPlaceSource::Pin,
        available,
    });
}

fn home_directory() -> Option<PathBuf> {
    std::env::var_os("HOME")
        .map(PathBuf::from)
        .or_else(|| std::env::var_os("USERPROFILE").map(PathBuf::from))
        .or_else(|| {
            let drive = std::env::var_os("HOMEDRIVE")?;
            let path = std::env::var_os("HOMEPATH")?;
            Some(PathBuf::from(format!(
                "{}{}",
                drive.to_string_lossy(),
                path.to_string_lossy()
            )))
        })
}

fn filesystem_roots() -> Vec<PathBuf> {
    if cfg!(windows) {
        ('A'..='Z')
            .map(|letter| PathBuf::from(format!("{letter}:\\")))
            .filter(|path| path.is_dir())
            .collect()
    } else {
        vec![PathBuf::from("/")]
    }
}

fn common_mount_roots() -> Vec<PathBuf> {
    if cfg!(windows) {
        Vec::new()
    } else {
        ["/mnt", "/media", "/Volumes"]
            .into_iter()
            .map(PathBuf::from)
            .filter(|path| path.is_dir())
            .collect()
    }
}

fn place_id_fragment(path: &Path) -> String {
    path.display()
        .to_string()
        .chars()
        .map(|ch| {
            if ch.is_ascii_alphanumeric() {
                ch.to_ascii_lowercase()
            } else {
                '-'
            }
        })
        .collect::<String>()
        .trim_matches('-')
        .to_owned()
}

fn root_picker_path_label(path: &Path) -> String {
    path.file_name()
        .and_then(|name| name.to_str())
        .filter(|name| !name.is_empty())
        .map(str::to_owned)
        .unwrap_or_else(|| path.display().to_string())
}

fn clean_pin_path(path: &str) -> Option<PathBuf> {
    let trimmed = path.trim();
    (!trimmed.is_empty()).then(|| PathBuf::from(trimmed))
}

fn deduplicate_pin_paths(paths: Vec<PathBuf>) -> Vec<PathBuf> {
    let mut pins: Vec<PathBuf> = paths
        .into_iter()
        .filter(|path| !path.as_os_str().is_empty())
        .collect();
    pins.sort();
    pins.dedup();
    pins
}

fn safe_child_name(name: &str) -> Option<&str> {
    if name.is_empty() {
        return None;
    }

    let mut components = Path::new(name).components();
    match (components.next(), components.next()) {
        (Some(Component::Normal(_)), None) => Some(name),
        _ => None,
    }
}

fn picker_error(status: StatusCode, error: impl Into<String>) -> Response {
    (
        status,
        Json(RootPickerError {
            error: error.into(),
        }),
    )
        .into_response()
}

fn default_picker_path() -> PathBuf {
    std::env::current_dir().unwrap_or_else(|_| PathBuf::from("."))
}
