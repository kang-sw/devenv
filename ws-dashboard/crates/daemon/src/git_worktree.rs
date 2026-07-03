use std::collections::BTreeSet;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use axum::extract::{Path as AxumPath, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::{Deserialize, Serialize};
use ws_dashboard_core::{
    DashboardResourcesView, WorkRootActivation, WorkRootAvailability, WorkRootKind, WorkspaceId,
};

use crate::discovery::local_work_root_id_for_path;
use crate::resources::live_dashboard_resources;
use crate::router::AppState;
use crate::work_root_files::{RegisteredWorkRoot, WorkRootProvenance};

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitWorktreeAddOptions {
    pub workspace_id: WorkspaceId,
    pub git: GitWorktreeAvailability,
    pub branches: Vec<GitWorktreeBranchOption>,
    pub defaults: GitWorktreeDefaults,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitWorktreeAvailability {
    pub available: bool,
    pub reason: Option<String>,
    pub root_label: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitWorktreeBranchOption {
    pub name: String,
    pub checked_out: bool,
    pub current: bool,
    pub disabled_reason: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitWorktreeDefaults {
    pub worktree_base_dir_label: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct GitWorktreeAddPreviewRequest {
    pub worktree_name: String,
    pub branch: GitWorktreeBranchRequest,
    pub path: GitWorktreePathRequest,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(tag = "mode", rename_all = "camelCase")]
pub enum GitWorktreeBranchRequest {
    Auto,
    Manual { name: String },
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(tag = "mode", rename_all = "camelCase")]
pub enum GitWorktreePathRequest {
    Auto,
    Custom {
        #[serde(rename = "targetPath")]
        target_path: String,
    },
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct AddGitWorktreeRequest {
    pub worktree_name: String,
    pub branch: GitWorktreeBranchRequest,
    pub path: GitWorktreePathRequest,
    pub activate: bool,
}

impl From<AddGitWorktreeRequest> for GitWorktreeAddPreviewRequest {
    fn from(request: AddGitWorktreeRequest) -> Self {
        Self {
            worktree_name: request.worktree_name,
            branch: request.branch,
            path: request.path,
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitWorktreeAddPreview {
    pub branch_name: String,
    pub filesystem_name: String,
    pub target_path_label: String,
    pub status: GitWorktreePreviewStatus,
    pub message: String,
    pub blockers: Vec<GitWorktreePreviewBlocker>,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum GitWorktreePreviewStatus {
    WillCreateBranch,
    WillCheckoutExisting,
    Blocked,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitWorktreePreviewBlocker {
    pub code: GitWorktreeBlockerCode,
    pub field: Option<GitWorktreeBlockerField>,
    pub message: String,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum GitWorktreeBlockerCode {
    InvalidWorktreeName,
    InvalidBranchName,
    BranchAlreadyCheckedOut,
    TargetExists,
    TargetParentMissing,
    NotGitWorkspace,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum GitWorktreeBlockerField {
    WorktreeName,
    Branch,
    Path,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AddGitWorktreeResponse {
    pub resources: DashboardResourcesView,
    pub created_work_root_id: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct WorktreeRouteError {
    error: String,
}

pub async fn git_worktree_add_options(
    State(state): State<AppState>,
    AxumPath(workspace_id): AxumPath<String>,
) -> Response {
    let workspace_id = WorkspaceId::from(workspace_id);
    match resolve_workspace_git(&state, &workspace_id) {
        Ok(context) => Json(options_for_context(workspace_id, &context)).into_response(),
        Err(error) => {
            let root_label = error.root_label.unwrap_or_else(|| "Workspace".to_owned());
            Json(GitWorktreeAddOptions {
                workspace_id,
                git: GitWorktreeAvailability {
                    available: false,
                    reason: Some(error.message),
                    root_label,
                },
                branches: Vec::new(),
                defaults: GitWorktreeDefaults {
                    worktree_base_dir_label: ".git/ws-worktree".to_owned(),
                },
            })
            .into_response()
        }
    }
}

pub async fn git_worktree_add_preview(
    State(state): State<AppState>,
    AxumPath(workspace_id): AxumPath<String>,
    Json(request): Json<GitWorktreeAddPreviewRequest>,
) -> Response {
    let workspace_id = WorkspaceId::from(workspace_id);
    Json(resolve_preview(&state, &workspace_id, request)).into_response()
}

pub async fn git_worktree_add_submit(
    State(state): State<AppState>,
    AxumPath(workspace_id): AxumPath<String>,
    Json(request): Json<AddGitWorktreeRequest>,
) -> Response {
    let workspace_id = WorkspaceId::from(workspace_id);
    let activate = request.activate;
    let preview_request: GitWorktreeAddPreviewRequest = request.into();
    let context = match resolve_workspace_git(&state, &workspace_id) {
        Ok(context) => context,
        Err(_) => {
            return bounded_error(
                StatusCode::BAD_REQUEST,
                "workspace is not available for Git worktree creation",
            )
        }
    };
    let resolved = resolve_preview_with_context(&context, preview_request);
    if resolved.preview.status == GitWorktreePreviewStatus::Blocked {
        return (StatusCode::BAD_REQUEST, Json(resolved.preview)).into_response();
    }

    let mut command = Command::new("git");
    command
        .arg("-C")
        .arg(&context.root_path)
        .arg("worktree")
        .arg("add");
    match resolved.preview.status {
        GitWorktreePreviewStatus::WillCreateBranch => {
            command
                .arg("-b")
                .arg(&resolved.preview.branch_name)
                .arg(&resolved.target_path);
        }
        GitWorktreePreviewStatus::WillCheckoutExisting => {
            command
                .arg(&resolved.target_path)
                .arg(&resolved.preview.branch_name);
        }
        GitWorktreePreviewStatus::Blocked => unreachable!(),
    }
    let output = match command.output() {
        Ok(output) => output,
        Err(_) => {
            return bounded_error(StatusCode::INTERNAL_SERVER_ERROR, "git worktree add failed")
        }
    };
    if !output.status.success() {
        return bounded_error(StatusCode::BAD_REQUEST, "git worktree add failed");
    }

    let created_id = local_work_root_id_for_path(&resolved.target_path);
    let _persist_guard = state.registry_persist_lock.lock().await;
    let previous_entry = state.opened_work_roots.register_registry_entry(
        created_id.clone(),
        RegisteredWorkRoot {
            path: resolved.target_path.clone(),
            activation: if activate {
                WorkRootActivation::Online
            } else {
                WorkRootActivation::Offline
            },
            provenance: WorkRootProvenance::Opened,
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
                    .register_registry_entry(created_id.clone(), previous);
            }
            None => {
                state.opened_work_roots.unregister(&created_id);
            }
        }
        tracing::warn!(%error, "failed to persist added Git worktree");
        return bounded_error(StatusCode::INTERNAL_SERVER_ERROR, "persist workRoot failed");
    }
    let resources = live_dashboard_resources(&state.opened_work_roots);
    let created_present = resources
        .workspaces
        .iter()
        .flat_map(|w| &w.work_roots)
        .any(|r| r.id == created_id);
    Json(AddGitWorktreeResponse {
        resources,
        created_work_root_id: created_present.then(|| created_id.as_str().to_owned()),
    })
    .into_response()
}

fn options_for_context(
    workspace_id: WorkspaceId,
    context: &GitWorkspaceContext,
) -> GitWorktreeAddOptions {
    GitWorktreeAddOptions {
        workspace_id,
        git: GitWorktreeAvailability {
            available: true,
            reason: None,
            root_label: context.root_label.clone(),
        },
        branches: context.branches.clone(),
        defaults: GitWorktreeDefaults {
            worktree_base_dir_label: ".git/ws-worktree".to_owned(),
        },
    }
}

fn resolve_preview(
    state: &AppState,
    workspace_id: &WorkspaceId,
    request: GitWorktreeAddPreviewRequest,
) -> GitWorktreeAddPreview {
    match resolve_workspace_git(state, workspace_id) {
        Ok(context) => resolve_preview_with_context(&context, request).preview,
        Err(error) => blocked_preview(
            "",
            "",
            "",
            GitWorktreeBlockerCode::NotGitWorkspace,
            None,
            error.message,
        ),
    }
}

struct ResolvedPreview {
    preview: GitWorktreeAddPreview,
    target_path: PathBuf,
}

fn resolve_preview_with_context(
    context: &GitWorkspaceContext,
    request: GitWorktreeAddPreviewRequest,
) -> ResolvedPreview {
    let filesystem_name = filesystem_compatible_name(&request.worktree_name);
    let branch_name = match &request.branch {
        GitWorktreeBranchRequest::Auto => branch_compatible_name(&request.worktree_name),
        GitWorktreeBranchRequest::Manual { name } => name.trim().to_owned(),
    };
    let target_path = match &request.path {
        GitWorktreePathRequest::Auto => context
            .common_dir
            .join("ws-worktree")
            .join(&filesystem_name),
        GitWorktreePathRequest::Custom { target_path } => PathBuf::from(target_path.trim()),
    };
    let target_path_label = target_path.display().to_string();
    let mut blockers = Vec::new();
    if filesystem_name.is_empty() {
        blockers.push(blocker(
            GitWorktreeBlockerCode::InvalidWorktreeName,
            Some(GitWorktreeBlockerField::WorktreeName),
            "worktree name is required",
        ));
    }
    if branch_name.is_empty() || !valid_branch_name(&context.root_path, &branch_name) {
        blockers.push(blocker(
            GitWorktreeBlockerCode::InvalidBranchName,
            Some(GitWorktreeBlockerField::Branch),
            "branch name is invalid",
        ));
    }
    if context.checked_out_branches.contains(&branch_name) {
        blockers.push(blocker(
            GitWorktreeBlockerCode::BranchAlreadyCheckedOut,
            Some(GitWorktreeBlockerField::Branch),
            "branch is already checked out in another worktree",
        ));
    }
    if target_path.exists() {
        blockers.push(blocker(
            GitWorktreeBlockerCode::TargetExists,
            Some(GitWorktreeBlockerField::Path),
            "target path already exists",
        ));
    } else if target_path.parent().map(|p| !p.is_dir()).unwrap_or(true) {
        blockers.push(blocker(
            GitWorktreeBlockerCode::TargetParentMissing,
            Some(GitWorktreeBlockerField::Path),
            "target parent directory is missing",
        ));
    }
    let branch_exists = context.branch_names.contains(&branch_name);
    let status = if blockers.is_empty() {
        if branch_exists {
            GitWorktreePreviewStatus::WillCheckoutExisting
        } else {
            GitWorktreePreviewStatus::WillCreateBranch
        }
    } else {
        GitWorktreePreviewStatus::Blocked
    };
    let message = match status {
        GitWorktreePreviewStatus::WillCreateBranch => "new branch will be created".to_owned(),
        GitWorktreePreviewStatus::WillCheckoutExisting => {
            "existing branch will be checked out".to_owned()
        }
        GitWorktreePreviewStatus::Blocked => blockers
            .first()
            .map(|b| b.message.clone())
            .unwrap_or_else(|| "blocked".to_owned()),
    };
    ResolvedPreview {
        preview: GitWorktreeAddPreview {
            branch_name,
            filesystem_name,
            target_path_label,
            status,
            message,
            blockers,
        },
        target_path,
    }
}

#[derive(Clone, Debug)]
struct GitWorkspaceContext {
    root_path: PathBuf,
    common_dir: PathBuf,
    root_label: String,
    branches: Vec<GitWorktreeBranchOption>,
    branch_names: BTreeSet<String>,
    checked_out_branches: BTreeSet<String>,
}

#[derive(Debug)]
struct GitWorkspaceError {
    message: String,
    root_label: Option<String>,
}

fn resolve_workspace_git(
    state: &AppState,
    workspace_id: &WorkspaceId,
) -> Result<GitWorkspaceContext, GitWorkspaceError> {
    let resources = live_dashboard_resources(&state.opened_work_roots);
    let workspace = resources
        .workspaces
        .iter()
        .find(|workspace| workspace.id == *workspace_id)
        .ok_or_else(|| GitWorkspaceError {
            message: "unknown workspace".to_owned(),
            root_label: None,
        })?;
    let root = workspace
        .work_roots
        .iter()
        .filter(|root| root.availability == WorkRootAvailability::Available)
        .find(|root| root.kind == WorkRootKind::GitPrimaryRoot)
        .or_else(|| {
            workspace
                .work_roots
                .iter()
                .filter(|root| root.availability == WorkRootAvailability::Available)
                .find(|root| root.kind == WorkRootKind::GitLinkedWorktree)
        })
        .ok_or_else(|| GitWorkspaceError {
            message: "workspace has no available Git workRoot".to_owned(),
            root_label: Some(workspace.label.clone()),
        })?;
    let root_path = state
        .opened_work_roots
        .resolve(&root.id)
        .ok_or_else(|| GitWorkspaceError {
            message: "workspace root is unavailable".to_owned(),
            root_label: Some(root.label.clone()),
        })?;
    let common_dir = git_path(
        &root_path,
        &["rev-parse", "--path-format=absolute", "--git-common-dir"],
    )
    .ok_or_else(|| GitWorkspaceError {
        message: "workspace is not a Git workRoot".to_owned(),
        root_label: Some(root.label.clone()),
    })?;
    let base = common_dir.join("ws-worktree");
    let _ = fs::create_dir_all(&base);
    let current = git_text(&root_path, &["rev-parse", "--abbrev-ref", "HEAD"]).unwrap_or_default();
    let branches = git_branches(&root_path, &current);
    let branch_names = branches.iter().map(|branch| branch.name.clone()).collect();
    let checked_out_branches = branches
        .iter()
        .filter(|branch| branch.checked_out)
        .map(|branch| branch.name.clone())
        .collect();
    Ok(GitWorkspaceContext {
        root_path,
        common_dir,
        root_label: root.label.clone(),
        branches,
        branch_names,
        checked_out_branches,
    })
}

fn git_branches(root: &Path, current: &str) -> Vec<GitWorktreeBranchOption> {
    let raw = git_text(
        root,
        &["branch", "--format", "%(refname:short)%00%(worktreepath)"],
    )
    .unwrap_or_default();
    let mut branches: Vec<_> = raw
        .lines()
        .filter_map(|line| {
            let (name, worktree_path) = line.split_once('\0').unwrap_or((line, ""));
            let name = name.trim();
            if name.is_empty() {
                return None;
            }
            let checked_out = !worktree_path.trim().is_empty();
            Some(GitWorktreeBranchOption {
                name: name.to_owned(),
                checked_out,
                current: name == current.trim(),
                disabled_reason: checked_out.then(|| "Already checked out".to_owned()),
            })
        })
        .collect();
    branches.sort_by(|a, b| a.name.cmp(&b.name));
    branches
}

fn git_path(root: &Path, args: &[&str]) -> Option<PathBuf> {
    git_text(root, args).map(|text| PathBuf::from(text.trim()))
}

fn git_text(root: &Path, args: &[&str]) -> Option<String> {
    let output = Command::new("git")
        .arg("-C")
        .arg(root)
        .args(args)
        .output()
        .ok()?;
    output
        .status
        .success()
        .then(|| String::from_utf8_lossy(&output.stdout).trim().to_owned())
}

fn valid_branch_name(root: &Path, name: &str) -> bool {
    Command::new("git")
        .arg("-C")
        .arg(root)
        .args(["check-ref-format", "--branch", name])
        .output()
        .map(|output| output.status.success())
        .unwrap_or(false)
}

fn branch_compatible_name(input: &str) -> String {
    sanitize_name(input, '-')
}
fn filesystem_compatible_name(input: &str) -> String {
    sanitize_name(input, '-')
}

fn sanitize_name(input: &str, replacement: char) -> String {
    let mut out = String::new();
    let mut last_repl = false;
    for ch in input.trim().chars() {
        if ch.is_ascii_alphanumeric() || matches!(ch, '.' | '_' | '-') {
            out.push(ch);
            last_repl = false;
        } else if !last_repl {
            out.push(replacement);
            last_repl = true;
        }
    }
    out.trim_matches(replacement).trim_matches('.').to_owned()
}

fn blocked_preview(
    branch_name: &str,
    filesystem_name: &str,
    target_path_label: &str,
    code: GitWorktreeBlockerCode,
    field: Option<GitWorktreeBlockerField>,
    message: String,
) -> GitWorktreeAddPreview {
    GitWorktreeAddPreview {
        branch_name: branch_name.to_owned(),
        filesystem_name: filesystem_name.to_owned(),
        target_path_label: target_path_label.to_owned(),
        status: GitWorktreePreviewStatus::Blocked,
        message: message.clone(),
        blockers: vec![blocker(code, field, message)],
    }
}

fn blocker(
    code: GitWorktreeBlockerCode,
    field: Option<GitWorktreeBlockerField>,
    message: impl Into<String>,
) -> GitWorktreePreviewBlocker {
    GitWorktreePreviewBlocker {
        code,
        field,
        message: message.into(),
    }
}

fn bounded_error(status: StatusCode, error: impl Into<String>) -> Response {
    (
        status,
        Json(WorktreeRouteError {
            error: error.into(),
        }),
    )
        .into_response()
}
