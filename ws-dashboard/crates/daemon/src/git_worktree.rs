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
    DashboardResourcesView, WorkRootActivation, WorkRootAvailability, WorkRootId, WorkRootKind,
    WorkspaceId,
};

use crate::discovery::local_work_root_id_for_path;
use crate::git_toolbar::changes_for_path;
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

// ---------------------------------------------------------------------------
// Worktree removal (260525 Phase 1 prerequisite + Phase 3 B-1/B-2 UX).
//
// SAFETY: `git worktree remove` and any branch delete ALWAYS run with
// `-C <primary-root-path>`, never `-C <target-worktree>` — you cannot remove a
// worktree from a git context rooted inside itself. Branch deletion is plain
// `git branch -d` gated on a non-mutating `merge-base --is-ancestor` merged
// check; it NEVER becomes `git branch -D`.
// ---------------------------------------------------------------------------

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase", default)]
pub struct RemoveGitWorktreeRequest {
    pub delete_branch: bool,
    pub force: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitWorktreeRemovePreview {
    pub work_root_id: String,
    pub target_path_label: String,
    pub branch_name: Option<String>,
    pub has_uncommitted_changes: bool,
    pub modified_files: u64,
    pub untracked_files: u64,
    pub branch_unmerged: bool,
    pub available: bool,
    pub reason: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoveGitWorktreeResponse {
    pub resources: DashboardResourcesView,
    pub removed_work_root_id: Option<String>,
    pub branch_deleted: bool,
    pub branch_delete_skipped_unmerged: bool,
}

struct WorktreeRemoveContext {
    primary_root_path: PathBuf,
    target_path: PathBuf,
    branch_name: Option<String>,
}

fn resolve_worktree_remove(
    state: &AppState,
    work_root_id: &WorkRootId,
) -> Result<WorktreeRemoveContext, GitWorkspaceError> {
    let resources = live_dashboard_resources(&state.opened_work_roots);
    let workspace = resources
        .workspaces
        .iter()
        .find(|workspace| {
            workspace
                .work_roots
                .iter()
                .any(|root| root.id == *work_root_id)
        })
        .ok_or_else(|| GitWorkspaceError {
            message: "unknown workRoot".to_owned(),
            root_label: None,
        })?;
    let target = workspace
        .work_roots
        .iter()
        .find(|root| root.id == *work_root_id)
        .expect("target root present in owning workspace");
    if target.kind != WorkRootKind::GitLinkedWorktree {
        return Err(GitWorkspaceError {
            message: "workRoot is not a linked worktree".to_owned(),
            root_label: Some(target.label.clone()),
        });
    }
    let target_path =
        state
            .opened_work_roots
            .resolve(work_root_id)
            .ok_or_else(|| GitWorkspaceError {
                message: "worktree path is unavailable".to_owned(),
                root_label: Some(target.label.clone()),
            })?;
    // Resolve the workspace's PRIMARY root: `git worktree remove` must run from
    // there, never from inside the worktree being removed.
    let primary = workspace
        .work_roots
        .iter()
        .filter(|root| root.availability == WorkRootAvailability::Available)
        .find(|root| root.kind == WorkRootKind::GitPrimaryRoot)
        .ok_or_else(|| GitWorkspaceError {
            message: "workspace primary Git root is unavailable".to_owned(),
            root_label: Some(workspace.label.clone()),
        })?;
    let primary_root_path =
        state
            .opened_work_roots
            .resolve(&primary.id)
            .ok_or_else(|| GitWorkspaceError {
                message: "primary root path is unavailable".to_owned(),
                root_label: Some(primary.label.clone()),
            })?;
    let branch_name = git_text(&target_path, &["branch", "--show-current"])
        .filter(|branch| !branch.is_empty());
    Ok(WorktreeRemoveContext {
        primary_root_path,
        target_path,
        branch_name,
    })
}

/// Non-mutating equivalent of "would `git branch -d <branch>` refuse". Returns
/// `true` when the branch has commits not reachable from its chosen safety ref
/// (its configured upstream if any, else the primary root's current HEAD),
/// i.e. deleting it would lose commits. A git invocation failure is treated as
/// unmerged (conservative: never let a spawn error greenlight a delete).
fn branch_unmerged(primary_root: &Path, branch: &str) -> bool {
    let upstream = git_text(
        primary_root,
        &[
            "rev-parse",
            "--abbrev-ref",
            "--symbolic-full-name",
            &format!("{branch}@{{upstream}}"),
        ],
    )
    .filter(|upstream| !upstream.is_empty());
    let reference = upstream.unwrap_or_else(|| "HEAD".to_owned());
    let is_ancestor = Command::new("git")
        .arg("-C")
        .arg(primary_root)
        .args(["merge-base", "--is-ancestor", branch, &reference])
        .output()
        .map(|output| output.status.success())
        .unwrap_or(false);
    !is_ancestor
}

fn run_git_ok(root: &Path, args: &[&str]) -> bool {
    Command::new("git")
        .arg("-C")
        .arg(root)
        .args(args)
        .output()
        .map(|output| output.status.success())
        .unwrap_or(false)
}

pub async fn git_worktree_remove_preview(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let context = match resolve_worktree_remove(&state, &work_root_id) {
        Ok(context) => context,
        Err(error) => {
            return Json(GitWorktreeRemovePreview {
                work_root_id: work_root_id.as_str().to_owned(),
                target_path_label: String::new(),
                branch_name: None,
                has_uncommitted_changes: false,
                modified_files: 0,
                untracked_files: 0,
                branch_unmerged: false,
                available: false,
                reason: Some(error.message),
            })
            .into_response()
        }
    };
    let changes = changes_for_path(&context.target_path);
    let has_uncommitted = changes.modified_files > 0 || changes.untracked_files > 0;
    let branch_unmerged = context
        .branch_name
        .as_deref()
        .map(|branch| branch_unmerged(&context.primary_root_path, branch))
        .unwrap_or(false);
    Json(GitWorktreeRemovePreview {
        work_root_id: work_root_id.as_str().to_owned(),
        target_path_label: context.target_path.display().to_string(),
        branch_name: context.branch_name,
        has_uncommitted_changes: has_uncommitted,
        modified_files: changes.modified_files,
        untracked_files: changes.untracked_files,
        branch_unmerged,
        available: true,
        reason: None,
    })
    .into_response()
}

pub async fn git_worktree_remove_submit(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Json(request): Json<RemoveGitWorktreeRequest>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let context = match resolve_worktree_remove(&state, &work_root_id) {
        Ok(context) => context,
        Err(_) => {
            return bounded_error(
                StatusCode::BAD_REQUEST,
                "worktree is not available for removal",
            )
        }
    };

    // B-1 force gate: never destroy uncommitted/untracked work without an
    // explicit force flag set after the owner has seen the data-loss warning.
    let changes = changes_for_path(&context.target_path);
    let has_uncommitted = changes.modified_files > 0 || changes.untracked_files > 0;
    if has_uncommitted && !request.force {
        return bounded_error(
            StatusCode::CONFLICT,
            "worktree has uncommitted or untracked changes; force required",
        );
    }

    let mut command = Command::new("git");
    command
        .arg("-C")
        .arg(&context.primary_root_path)
        .arg("worktree")
        .arg("remove");
    if request.force {
        command.arg("--force");
    }
    command.arg(&context.target_path);
    let output = match command.output() {
        Ok(output) => output,
        Err(_) => {
            return bounded_error(StatusCode::INTERNAL_SERVER_ERROR, "git worktree remove failed")
        }
    };
    if !output.status.success() {
        return bounded_error(StatusCode::BAD_REQUEST, "git worktree remove failed");
    }

    // B-2 branch delete: plain `-d` only, and only when the non-mutating
    // merged check confirms it is safe. An unmerged branch is left intact and
    // reported back — this path must NEVER escalate to `git branch -D`.
    let mut branch_deleted = false;
    let mut branch_delete_skipped_unmerged = false;
    if request.delete_branch {
        if let Some(branch) = context.branch_name.as_deref() {
            if branch_unmerged(&context.primary_root_path, branch) {
                branch_delete_skipped_unmerged = true;
            } else {
                branch_deleted =
                    run_git_ok(&context.primary_root_path, &["branch", "-d", branch]);
            }
        }
    }

    let _persist_guard = state.registry_persist_lock.lock().await;
    let previous_entry = state.opened_work_roots.unregister(&work_root_id);
    if let Err(error) = state
        .dashboard_state
        .persist_opened_work_roots(&state.opened_work_roots)
        .await
    {
        if let Some(previous) = previous_entry {
            state
                .opened_work_roots
                .register_registry_entry(work_root_id.clone(), previous);
        }
        tracing::warn!(%error, "failed to persist removed Git worktree");
        return bounded_error(
            StatusCode::INTERNAL_SERVER_ERROR,
            "persist worktree removal failed",
        );
    }

    let ids: BTreeSet<WorkRootId> = std::iter::once(work_root_id.clone()).collect();
    state.terminals.remove_for_work_roots(&ids);
    state.codex_sessions.remove_for_work_roots(&ids);
    state.claude_sessions.remove_for_work_roots(&ids);

    let resources = live_dashboard_resources(&state.opened_work_roots);
    let removed = !resources
        .workspaces
        .iter()
        .flat_map(|workspace| &workspace.work_roots)
        .any(|root| root.id == work_root_id);
    Json(RemoveGitWorktreeResponse {
        resources,
        removed_work_root_id: removed.then(|| work_root_id.as_str().to_owned()),
        branch_deleted,
        branch_delete_skipped_unmerged,
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

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn git_available() -> bool {
        Command::new("git")
            .arg("--version")
            .output()
            .map(|output| output.status.success())
            .unwrap_or(false)
    }

    fn run(path: &Path, args: &[&str]) {
        let output = Command::new("git")
            .arg("-C")
            .arg(path)
            .args(args)
            .output()
            .expect("run git");
        assert!(
            output.status.success(),
            "git {args:?} failed: {}",
            String::from_utf8_lossy(&output.stderr)
        );
    }

    fn init_fixture_repo(name: &str) -> PathBuf {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system time after epoch")
            .as_nanos();
        let dir = std::env::temp_dir().join(format!(
            "ws-dashboard-git-worktree-remove-{name}-{}-{unique}",
            std::process::id()
        ));
        fs::create_dir_all(&dir).expect("create fixture dir");
        run(&dir, &["init", "-q"]);
        run(&dir, &["config", "user.email", "test@example.com"]);
        run(&dir, &["config", "user.name", "Test"]);
        fs::write(dir.join("tracked.txt"), "one\n").expect("write tracked.txt");
        run(&dir, &["add", "tracked.txt"]);
        run(&dir, &["commit", "-q", "-m", "init"]);
        dir
    }

    #[test]
    fn branch_unmerged_distinguishes_merged_and_dangling_branches() {
        if !git_available() {
            return;
        }
        let dir = init_fixture_repo("branch-check");
        let base_branch =
            git_text(&dir, &["branch", "--show-current"]).expect("default branch name");

        // A branch with no unique commits (freshly created off HEAD) is safe to
        // delete — `git branch -d` would accept it.
        run(&dir, &["branch", "merged-topic"]);
        assert!(
            !branch_unmerged(&dir, "merged-topic"),
            "a branch with no commits beyond HEAD must read as merged/safe"
        );

        // A branch carrying a commit not reachable from HEAD is unmerged —
        // `git branch -d` would refuse and only `-D` would force it.
        run(&dir, &["switch", "-c", "dangling-topic"]);
        fs::write(dir.join("tracked.txt"), "one\ntwo\n").expect("modify tracked.txt");
        run(&dir, &["commit", "-aqm", "dangling commit"]);
        run(&dir, &["switch", "-q", &base_branch]);
        assert!(
            branch_unmerged(&dir, "dangling-topic"),
            "a branch with a commit not on HEAD must read as unmerged/dangling"
        );

        fs::remove_dir_all(&dir).ok();
    }
}
