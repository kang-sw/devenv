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
use ws_dashboard_core::WorkRootId;

use crate::router::AppState;

const MAX_READ_ONLY_TEXT_BYTES: u64 = 512 * 1024;

#[derive(Clone, Debug, Default)]
pub struct OpenedWorkRoots {
    roots: Arc<RwLock<HashMap<WorkRootId, PathBuf>>>,
}

impl OpenedWorkRoots {
    pub fn register(&self, work_root_id: WorkRootId, root_path: PathBuf) {
        self.roots
            .write()
            .expect("opened workRoots lock poisoned")
            .insert(work_root_id, root_path);
    }

    pub fn resolve(&self, work_root_id: &WorkRootId) -> Option<PathBuf> {
        self.roots
            .read()
            .expect("opened workRoots lock poisoned")
            .get(work_root_id)
            .cloned()
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
    let Some(root_path) = state.opened_work_roots.resolve(&work_root_id) else {
        return file_error(StatusCode::NOT_FOUND, "unknown workRoot");
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
    let Some(root_path) = state.opened_work_roots.resolve(&work_root_id) else {
        return file_error(StatusCode::NOT_FOUND, "unknown workRoot");
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
