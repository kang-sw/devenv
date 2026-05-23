use std::collections::HashMap;
use std::fs;
use std::path::{Component, Path, PathBuf};

use axum::extract::{Query, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::{Deserialize, Serialize};
use ws_dashboard_core::{
    DashboardResourcesView, WorkRootActivation, WorkRootAvailability, WorkRootId,
};

use crate::discovery::{LocalDashboardResourcesProvider, LocalWorkRootCandidate};
use crate::resources::{live_dashboard_resources, DashboardResourcesProvider};
use crate::router::AppState;
use crate::work_root_files::RegisteredWorkRoot;

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RootPickerView {
    pub current_path: String,
    pub parent_path: Option<String>,
    pub entries: Vec<RootPickerEntry>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RootPickerEntry {
    pub name: String,
    pub path: String,
    pub entry_type: RootPickerEntryType,
    pub selectable: bool,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum RootPickerEntryType {
    Directory,
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
pub struct SetWorkRootActivationRequest {
    pub activation: WorkRootActivation,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct RootPickerError {
    error: String,
}

pub async fn list_root_picker(Query(query): Query<HashMap<String, String>>) -> Response {
    let path = query
        .get("path")
        .map(PathBuf::from)
        .unwrap_or_else(default_picker_path);

    match root_picker_view(&path) {
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

pub async fn open_work_root(
    State(state): State<AppState>,
    Json(request): Json<OpenWorkRootRequest>,
) -> Response {
    let requested_path = PathBuf::from(request.path);
    let provider = LocalDashboardResourcesProvider::new(vec![LocalWorkRootCandidate::new(
        requested_path.clone(),
    )]);
    let view = provider.dashboard_resources();
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
    let aggregated = live_dashboard_resources(&state.opened_work_roots);
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
    Json::<DashboardResourcesView>(live_dashboard_resources(&state.opened_work_roots))
        .into_response()
}

fn root_picker_view(path: &Path) -> Result<RootPickerView, String> {
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
        current_path: path.display().to_string(),
        parent_path: path.parent().map(|parent| parent.display().to_string()),
        entries,
    })
}

fn entry_for_directory(path: &Path) -> RootPickerEntry {
    RootPickerEntry {
        name: path
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("")
            .to_owned(),
        path: path.display().to_string(),
        entry_type: RootPickerEntryType::Directory,
        selectable: true,
    }
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
