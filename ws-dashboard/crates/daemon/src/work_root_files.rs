use std::collections::HashMap;
use std::ffi::OsStr;
use std::path::{Component, Path, PathBuf};
use std::sync::{Arc, RwLock};

use axum::extract::{Path as AxumPath, Query, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::{Deserialize, Serialize};
use tokio::fs;
use ws_dashboard_core::{WorkRootActivation, WorkRootId};

use crate::discovery::local_work_root_id_for_path;
use crate::router::AppState;

const MAX_READ_ONLY_TEXT_BYTES: u64 = 512 * 1024;

#[derive(Clone, Debug, Default)]
pub struct OpenedWorkRoots {
    roots: Arc<RwLock<HashMap<WorkRootId, RegisteredWorkRoot>>>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RegisteredWorkRoot {
    pub path: PathBuf,
    pub activation: WorkRootActivation,
    pub provenance: WorkRootProvenance,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum WorkRootProvenance {
    Opened,
}

impl OpenedWorkRoots {
    pub fn from_paths(paths: Vec<PathBuf>) -> Self {
        let opened = Self::default();
        for path in paths {
            opened.register_path(path);
        }
        opened
    }

    pub fn register_path(&self, root_path: PathBuf) -> WorkRootId {
        let work_root_id = local_work_root_id_for_path(&root_path);
        self.register(work_root_id.clone(), root_path);
        work_root_id
    }

    pub fn register(&self, work_root_id: WorkRootId, root_path: PathBuf) {
        self.register_with_activation(work_root_id, root_path, WorkRootActivation::Online);
    }

    pub fn register_with_activation(
        &self,
        work_root_id: WorkRootId,
        root_path: PathBuf,
        activation: WorkRootActivation,
    ) {
        self.register_registry_entry(
            work_root_id,
            RegisteredWorkRoot {
                path: root_path,
                activation,
                provenance: WorkRootProvenance::Opened,
            },
        );
    }

    pub fn register_registry_entry(&self, work_root_id: WorkRootId, root: RegisteredWorkRoot) {
        self.roots
            .write()
            .expect("opened workRoots lock poisoned")
            .insert(work_root_id, root);
    }

    pub fn resolve(&self, work_root_id: &WorkRootId) -> Option<PathBuf> {
        self.get(work_root_id).map(|root| root.path)
    }

    pub fn get(&self, work_root_id: &WorkRootId) -> Option<RegisteredWorkRoot> {
        self.roots
            .read()
            .expect("opened workRoots lock poisoned")
            .get(work_root_id)
            .cloned()
    }

    pub fn set_activation(
        &self,
        work_root_id: &WorkRootId,
        activation: WorkRootActivation,
    ) -> Option<WorkRootActivation> {
        let mut roots = self.roots.write().expect("opened workRoots lock poisoned");
        let Some(root) = roots.get_mut(work_root_id) else {
            return None;
        };
        let previous = root.activation;
        root.activation = activation;
        Some(previous)
    }

    /// Registered workRoot paths in a deterministic order.
    ///
    /// The backing store is an unordered `HashMap`, so callers that build
    /// aggregated resource views (the live `/api/dashboard/resources` route)
    /// must sort to keep route responses and route tests stable.
    pub fn candidate_paths(&self) -> Vec<PathBuf> {
        let mut paths: Vec<PathBuf> = self
            .roots
            .read()
            .expect("opened workRoots lock poisoned")
            .values()
            .map(|root| root.path.clone())
            .collect();
        paths.sort();
        paths
    }

    pub fn candidate_roots(&self) -> Vec<RegisteredWorkRoot> {
        let mut roots: Vec<RegisteredWorkRoot> = self
            .roots
            .read()
            .expect("opened workRoots lock poisoned")
            .values()
            .cloned()
            .collect();
        roots.sort_by(|left, right| left.path.cmp(&right.path));
        roots
    }
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
pub struct WorkRootFileListQuery {
    #[serde(default)]
    path: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkRootFileListView {
    pub work_root_id: WorkRootId,
    pub path: String,
    pub status: String,
    pub entries: Vec<WorkRootFileEntryView>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkRootFileEntryView {
    pub name: String,
    pub path: String,
    pub kind: WorkRootFileEntryKind,
    pub status: String,
    pub readable: bool,
    pub preview_eligible: bool,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum WorkRootFileEntryKind {
    Directory,
    File,
    Other,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct WorkRootFileError {
    error: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkRootTextFileView {
    pub work_root_id: WorkRootId,
    pub path: String,
    pub name: String,
    pub status: String,
    pub read_only: bool,
    pub content: String,
    pub size_bytes: u64,
    pub language_hint: Option<String>,
    pub extension: Option<String>,
}

pub async fn list_work_root_files(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Query(query): Query<WorkRootFileListQuery>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let root_path = match resolve_online_available_work_root(&state, &work_root_id) {
        Ok(root_path) => root_path,
        Err(error) => return error.into_file_response(),
    };

    let Some(relative_path) = safe_work_root_relative_path(&query.path) else {
        return file_error(StatusCode::BAD_REQUEST, "invalid workRoot path");
    };

    match list_directory(&root_path, &relative_path).await {
        Ok(entries) => Json(WorkRootFileListView {
            work_root_id,
            path: relative_path_to_string(&relative_path),
            status: "ok".to_owned(),
            entries,
        })
        .into_response(),
        Err(ListError::NotFound) => file_error(StatusCode::NOT_FOUND, "workRoot path not found"),
        Err(ListError::NotDirectory) => {
            file_error(StatusCode::BAD_REQUEST, "workRoot path is not a directory")
        }
        Err(ListError::Forbidden) => file_error(StatusCode::FORBIDDEN, "workRoot path unavailable"),
        Err(ListError::Unavailable) => {
            file_error(StatusCode::BAD_REQUEST, "workRoot path unavailable")
        }
    }
}

pub async fn read_work_root_file(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Query(query): Query<WorkRootFileListQuery>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let root_path = match resolve_online_available_work_root(&state, &work_root_id) {
        Ok(root_path) => root_path,
        Err(error) => return error.into_file_response(),
    };

    let Some(relative_path) = safe_work_root_relative_path(&query.path) else {
        return file_error(StatusCode::BAD_REQUEST, "invalid workRoot path");
    };
    if relative_path.as_os_str().is_empty() {
        return file_error(StatusCode::BAD_REQUEST, "file path is required");
    }

    match read_text_file(&root_path, &relative_path).await {
        Ok(view) => Json(WorkRootTextFileView {
            work_root_id,
            path: relative_path_to_string(&relative_path),
            ..view
        })
        .into_response(),
        Err(ReadError::NotFound) => file_error(StatusCode::NOT_FOUND, "file not found"),
        Err(ReadError::Directory) => file_error(StatusCode::BAD_REQUEST, "path is a directory"),
        Err(ReadError::Forbidden) => file_error(StatusCode::FORBIDDEN, "file unavailable"),
        Err(ReadError::Oversized) => file_error(StatusCode::BAD_REQUEST, "file is too large"),
        Err(ReadError::Unsupported) => file_error(StatusCode::BAD_REQUEST, "unsupported text file"),
        Err(ReadError::Unavailable) => file_error(StatusCode::BAD_REQUEST, "file unavailable"),
    }
}

async fn list_directory(
    root_path: &Path,
    relative_path: &Path,
) -> Result<Vec<WorkRootFileEntryView>, ListError> {
    let root = root_path.canonicalize().map_err(map_io_error)?;
    let target = root.join(relative_path);
    let target = target.canonicalize().map_err(map_io_error)?;
    if !target.starts_with(&root) {
        return Err(ListError::Forbidden);
    }

    let metadata = fs::metadata(&target).await.map_err(map_io_error)?;
    if !metadata.is_dir() {
        return Err(ListError::NotDirectory);
    }

    let mut read_dir = fs::read_dir(&target).await.map_err(map_io_error)?;
    let mut entries = Vec::new();
    while let Some(entry) = read_dir.next_entry().await.map_err(map_io_error)? {
        let name = entry.file_name().to_string_lossy().into_owned();
        let entry_relative_path = relative_path.join(&name);
        let entry_path = entry.path();
        let file_type = match entry.file_type().await {
            Ok(file_type) => file_type,
            Err(_) => {
                entries.push(WorkRootFileEntryView {
                    name,
                    path: relative_path_to_string(&entry_relative_path),
                    kind: WorkRootFileEntryKind::Other,
                    status: "unavailable".to_owned(),
                    readable: false,
                    preview_eligible: false,
                });
                continue;
            }
        };
        let kind = if file_type.is_dir() {
            WorkRootFileEntryKind::Directory
        } else if file_type.is_file() {
            WorkRootFileEntryKind::File
        } else {
            WorkRootFileEntryKind::Other
        };
        let readable = entry_path.metadata().is_ok();
        let preview_eligible = kind == WorkRootFileEntryKind::File && readable;

        entries.push(WorkRootFileEntryView {
            name,
            path: relative_path_to_string(&entry_relative_path),
            kind,
            status: if kind == WorkRootFileEntryKind::Other {
                "unsupported"
            } else if readable {
                "ok"
            } else {
                "unavailable"
            }
            .to_owned(),
            readable,
            preview_eligible,
        });
    }

    entries.sort_by(|left, right| {
        entry_sort_rank(left.kind)
            .cmp(&entry_sort_rank(right.kind))
            .then_with(|| left.name.cmp(&right.name))
    });
    Ok(entries)
}

async fn read_text_file(
    root_path: &Path,
    relative_path: &Path,
) -> Result<WorkRootTextFileView, ReadError> {
    let root = root_path.canonicalize().map_err(map_read_io_error)?;
    let target = root.join(relative_path);
    let target = target.canonicalize().map_err(map_read_io_error)?;
    if !target.starts_with(&root) {
        return Err(ReadError::Forbidden);
    }

    let metadata = fs::metadata(&target).await.map_err(map_read_io_error)?;
    if metadata.is_dir() {
        return Err(ReadError::Directory);
    }
    if !metadata.is_file() {
        return Err(ReadError::Unsupported);
    }
    if metadata.len() > MAX_READ_ONLY_TEXT_BYTES {
        return Err(ReadError::Oversized);
    }

    let bytes = fs::read(&target).await.map_err(map_read_io_error)?;
    if bytes.len() as u64 > MAX_READ_ONLY_TEXT_BYTES {
        return Err(ReadError::Oversized);
    }
    if bytes.contains(&0) {
        return Err(ReadError::Unsupported);
    }
    let content = String::from_utf8(bytes).map_err(|_| ReadError::Unsupported)?;
    let extension = target
        .extension()
        .and_then(OsStr::to_str)
        .filter(|extension| !extension.is_empty())
        .map(str::to_owned);
    let language_hint = extension.as_deref().and_then(language_hint_for_extension);

    Ok(WorkRootTextFileView {
        work_root_id: WorkRootId::from("pending"),
        path: String::new(),
        name: target
            .file_name()
            .and_then(OsStr::to_str)
            .unwrap_or("")
            .to_owned(),
        status: "ok".to_owned(),
        read_only: true,
        content,
        size_bytes: metadata.len(),
        language_hint: language_hint.map(str::to_owned),
        extension,
    })
}

fn language_hint_for_extension(extension: &str) -> Option<&'static str> {
    match extension {
        "rs" => Some("rust"),
        "ts" => Some("typescript"),
        "tsx" => Some("tsx"),
        "js" => Some("javascript"),
        "json" => Some("json"),
        "md" => Some("markdown"),
        "css" => Some("css"),
        "html" => Some("html"),
        "toml" => Some("toml"),
        "yaml" | "yml" => Some("yaml"),
        "sh" => Some("shell"),
        "py" => Some("python"),
        _ => None,
    }
}

fn safe_work_root_relative_path(path: &str) -> Option<PathBuf> {
    if path.contains('\\') || looks_like_windows_drive_path(path) {
        return None;
    }

    if path.is_empty() {
        return Some(PathBuf::new());
    }

    let mut safe = PathBuf::new();
    for component in Path::new(path).components() {
        match component {
            Component::Normal(part) => safe.push(part),
            _ => return None,
        }
    }
    Some(safe)
}

fn looks_like_windows_drive_path(path: &str) -> bool {
    let bytes = path.as_bytes();
    bytes.len() >= 2 && bytes[1] == b':' && bytes[0].is_ascii_alphabetic()
}

fn relative_path_to_string(path: &Path) -> String {
    path.components()
        .filter_map(|component| match component {
            Component::Normal(part) => Some(part.to_string_lossy()),
            _ => None,
        })
        .collect::<Vec<_>>()
        .join("/")
}

fn entry_sort_rank(kind: WorkRootFileEntryKind) -> u8 {
    match kind {
        WorkRootFileEntryKind::Directory => 0,
        WorkRootFileEntryKind::File => 1,
        WorkRootFileEntryKind::Other => 2,
    }
}

fn map_read_io_error(error: std::io::Error) -> ReadError {
    match error.kind() {
        std::io::ErrorKind::NotFound => ReadError::NotFound,
        std::io::ErrorKind::PermissionDenied => ReadError::Forbidden,
        _ => ReadError::Unavailable,
    }
}

fn map_io_error(error: std::io::Error) -> ListError {
    match error.kind() {
        std::io::ErrorKind::NotFound => ListError::NotFound,
        std::io::ErrorKind::PermissionDenied => ListError::Forbidden,
        _ => ListError::Unavailable,
    }
}

fn file_error(status: StatusCode, error: impl Into<String>) -> Response {
    (
        status,
        Json(WorkRootFileError {
            error: error.into(),
        }),
    )
        .into_response()
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum WorkRootAccessError {
    Unknown,
    Offline,
    Unavailable,
}

impl WorkRootAccessError {
    pub fn status(self) -> StatusCode {
        match self {
            Self::Unknown => StatusCode::NOT_FOUND,
            Self::Offline | Self::Unavailable => StatusCode::CONFLICT,
        }
    }

    pub fn message(self) -> &'static str {
        match self {
            Self::Unknown => "unknown workRoot",
            Self::Offline => "workRoot offline",
            Self::Unavailable => "workRoot unavailable",
        }
    }

    pub fn into_file_response(self) -> Response {
        file_error(self.status(), self.message())
    }
}

pub fn resolve_online_available_work_root(
    state: &AppState,
    work_root_id: &WorkRootId,
) -> Result<PathBuf, WorkRootAccessError> {
    let Some(root) = state.opened_work_roots.get(work_root_id) else {
        return Err(WorkRootAccessError::Unknown);
    };
    if root.activation == WorkRootActivation::Offline {
        return Err(WorkRootAccessError::Offline);
    }
    if !root.path.is_dir() || std::fs::read_dir(&root.path).is_err() {
        return Err(WorkRootAccessError::Unavailable);
    }
    Ok(root.path)
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum ReadError {
    Directory,
    Forbidden,
    NotFound,
    Oversized,
    Unsupported,
    Unavailable,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum ListError {
    Forbidden,
    NotDirectory,
    NotFound,
    Unavailable,
}
