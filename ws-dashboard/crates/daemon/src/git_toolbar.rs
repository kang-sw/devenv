use std::collections::BTreeSet;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::{SystemTime, UNIX_EPOCH};

use axum::extract::{Path as AxumPath, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::{Deserialize, Serialize};
use ws_dashboard_core::{WorkRootActivation, WorkRootAvailability, WorkRootId, WorkRootKind};

use crate::resources::live_dashboard_resources;
use crate::router::AppState;

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkRootGitStatus {
    pub available: bool,
    pub reason: Option<String>,
    pub branch: Option<GitStatusBranch>,
    pub changes: GitChangeSummary,
    pub sync: GitSyncSummary,
    pub operations: Option<GitOperationAvailability>,
    pub refreshed_at_ms: u128,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitStatusBranch {
    pub name: Option<String>,
    pub detached_oid: Option<String>,
    pub upstream: Option<String>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitChangeSummary {
    pub added_lines: u64,
    pub removed_lines: u64,
    pub modified_files: u64,
    pub untracked_files: u64,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitSyncSummary {
    pub ahead: u64,
    pub behind: u64,
    pub upstream: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitOperationAvailability {
    pub can_fetch: bool,
    pub can_push: bool,
    pub can_pull_ff_only: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitBranchList {
    pub current: Option<String>,
    pub detached_oid: Option<String>,
    pub branches: Vec<GitBranchEntry>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GitBranchEntry {
    pub name: String,
    pub current: bool,
    pub checked_out: bool,
    pub upstream: Option<String>,
    pub ahead: Option<u64>,
    pub behind: Option<u64>,
    pub disabled_reason: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct SwitchBranchRequest {
    pub branch_name: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct CreateBranchRequest {
    pub branch_name: String,
    pub base_branch: Option<String>,
    pub switch_to: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct GitRouteError {
    error: String,
    status: Option<WorkRootGitStatus>,
}

#[derive(Clone, Debug)]
struct GitContext {
    root_path: PathBuf,
}

#[derive(Clone, Debug, Eq, PartialEq)]
enum GitContextError {
    Unknown,
    Offline,
    Unavailable,
    NonGit,
}

impl GitContextError {
    fn message(&self) -> &'static str {
        match self {
            GitContextError::Unknown => "unknown workRoot",
            GitContextError::Offline => "workRoot offline",
            GitContextError::Unavailable => "workRoot unavailable",
            GitContextError::NonGit => "workRoot is not a Git workRoot",
        }
    }

    fn status_code(&self) -> StatusCode {
        match self {
            GitContextError::Unknown => StatusCode::NOT_FOUND,
            GitContextError::Offline | GitContextError::Unavailable => StatusCode::CONFLICT,
            GitContextError::NonGit => StatusCode::BAD_REQUEST,
        }
    }
}

pub async fn git_status(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let state_for_task = state.clone();
    tokio::task::spawn_blocking(
        move || match resolve_git_context(&state_for_task, &work_root_id) {
            Ok(context) => Json(status_for_path(&context.root_path)).into_response(),
            Err(error) => bounded_error(error.status_code(), error.message(), None),
        },
    )
    .await
    .expect("git status task panicked")
}

pub async fn git_branches(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let state_for_task = state.clone();
    tokio::task::spawn_blocking(
        move || match resolve_git_context(&state_for_task, &work_root_id) {
            Ok(context) => Json(branches_for_path(&context.root_path)).into_response(),
            Err(error) => bounded_error(error.status_code(), error.message(), None),
        },
    )
    .await
    .expect("git branches task panicked")
}

pub async fn git_switch_branch(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Json(request): Json<SwitchBranchRequest>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let state_for_task = state.clone();
    tokio::task::spawn_blocking(move || {
        let context = match resolve_git_context(&state_for_task, &work_root_id) {
            Ok(context) => context,
            Err(error) => return bounded_error(error.status_code(), error.message(), None),
        };
        let branch = request.branch_name.trim();
        if !branch_exists(&context.root_path, branch) {
            return bounded_error(
                StatusCode::BAD_REQUEST,
                "branch is unavailable",
                Some(status_for_path(&context.root_path)),
            );
        }
        if branch_checked_out_elsewhere(&context.root_path, branch) {
            return bounded_error(
                StatusCode::BAD_REQUEST,
                "branch is already checked out",
                Some(status_for_path(&context.root_path)),
            );
        }
        match run_git(&context.root_path, &["switch", branch]) {
            Ok(()) => Json(status_for_path(&context.root_path)).into_response(),
            Err(_) => bounded_error(
                StatusCode::BAD_REQUEST,
                "branch switch failed",
                Some(status_for_path(&context.root_path)),
            ),
        }
    })
    .await
    .expect("git switch task panicked")
}

pub async fn git_create_branch(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Json(request): Json<CreateBranchRequest>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let state_for_task = state.clone();
    tokio::task::spawn_blocking(move || {
        let context = match resolve_git_context(&state_for_task, &work_root_id) {
            Ok(context) => context,
            Err(error) => return bounded_error(error.status_code(), error.message(), None),
        };
        if !request.switch_to {
            return bounded_error(
                StatusCode::BAD_REQUEST,
                "create without switch is unsupported",
                Some(status_for_path(&context.root_path)),
            );
        }
        let branch = request.branch_name.trim();
        if !valid_branch_name(&context.root_path, branch)
            || branch_exists(&context.root_path, branch)
            || branch_checked_out_elsewhere(&context.root_path, branch)
        {
            return bounded_error(
                StatusCode::BAD_REQUEST,
                "branch cannot be created",
                Some(status_for_path(&context.root_path)),
            );
        }
        let mut args = vec!["switch", "-c", branch];
        if let Some(base) = request
            .base_branch
            .as_deref()
            .map(str::trim)
            .filter(|base| !base.is_empty())
        {
            if !branch_exists(&context.root_path, base) {
                return bounded_error(
                    StatusCode::BAD_REQUEST,
                    "base branch is unavailable",
                    Some(status_for_path(&context.root_path)),
                );
            }
            args.push(base);
        }
        match run_git(&context.root_path, &args) {
            Ok(()) => Json(status_for_path(&context.root_path)).into_response(),
            Err(_) => bounded_error(
                StatusCode::BAD_REQUEST,
                "branch create failed",
                Some(status_for_path(&context.root_path)),
            ),
        }
    })
    .await
    .expect("git create branch task panicked")
}

pub async fn git_fetch(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    mutate_no_body(
        state,
        WorkRootId::from(work_root_id),
        &["fetch"],
        "fetch failed",
    )
    .await
}

pub async fn git_push(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    mutate_no_body(
        state,
        WorkRootId::from(work_root_id),
        &["push"],
        "push failed",
    )
    .await
}

pub async fn git_pull_ff_only(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    mutate_no_body(
        state,
        WorkRootId::from(work_root_id),
        &["pull", "--ff-only"],
        "pull --ff-only failed",
    )
    .await
}

async fn mutate_no_body(
    state: AppState,
    work_root_id: WorkRootId,
    args: &'static [&'static str],
    failure: &'static str,
) -> Response {
    tokio::task::spawn_blocking(move || {
        let context = match resolve_git_context(&state, &work_root_id) {
            Ok(context) => context,
            Err(error) => return bounded_error(error.status_code(), error.message(), None),
        };
        match run_git(&context.root_path, args) {
            Ok(()) => Json(status_for_path(&context.root_path)).into_response(),
            Err(_) => bounded_error(
                StatusCode::BAD_REQUEST,
                failure,
                Some(status_for_path(&context.root_path)),
            ),
        }
    })
    .await
    .expect("git mutation task panicked")
}

fn resolve_git_context(
    state: &AppState,
    work_root_id: &WorkRootId,
) -> Result<GitContext, GitContextError> {
    let resources = live_dashboard_resources(&state.opened_work_roots);
    let root = resources
        .workspaces
        .iter()
        .flat_map(|workspace| &workspace.work_roots)
        .find(|root| root.id == *work_root_id)
        .ok_or(GitContextError::Unknown)?;
    if root.activation != WorkRootActivation::Online {
        return Err(GitContextError::Offline);
    }
    if root.availability != WorkRootAvailability::Available {
        return Err(GitContextError::Unavailable);
    }
    if !matches!(
        root.kind,
        WorkRootKind::GitPrimaryRoot | WorkRootKind::GitLinkedWorktree
    ) {
        return Err(GitContextError::NonGit);
    }
    let root_path = state
        .opened_work_roots
        .resolve(work_root_id)
        .ok_or(GitContextError::Unknown)?;
    Ok(GitContext { root_path })
}

fn status_for_path(root: &Path) -> WorkRootGitStatus {
    let branch_name = git_text(root, &["branch", "--show-current"]).unwrap_or_default();
    let detached_oid = if branch_name.is_empty() {
        git_text(root, &["rev-parse", "--short", "HEAD"])
    } else {
        None
    };
    let upstream = git_text(
        root,
        &[
            "rev-parse",
            "--abbrev-ref",
            "--symbolic-full-name",
            "@{upstream}",
        ],
    );
    let sync = sync_for_path(root, upstream.clone());
    WorkRootGitStatus {
        available: true,
        reason: None,
        branch: Some(GitStatusBranch {
            name: (!branch_name.is_empty()).then_some(branch_name),
            detached_oid,
            upstream: upstream.clone(),
        }),
        changes: changes_for_path(root),
        sync: sync.clone(),
        operations: Some(GitOperationAvailability {
            can_fetch: true,
            can_push: sync.upstream.is_some() && sync.ahead > 0,
            can_pull_ff_only: sync.upstream.is_some() && sync.behind > 0,
        }),
        refreshed_at_ms: now_ms(),
    }
}

fn branches_for_path(root: &Path) -> GitBranchList {
    let current = git_text(root, &["branch", "--show-current"]).unwrap_or_default();
    let detached_oid = if current.is_empty() {
        git_text(root, &["rev-parse", "--short", "HEAD"])
    } else {
        None
    };
    let checked_out = checked_out_branches(root);
    let output = git_text(
        root,
        &[
            "for-each-ref",
            "--format=%(refname:short)%00%(upstream:short)",
            "refs/heads",
        ],
    )
    .unwrap_or_default();
    let mut branches = Vec::new();
    for line in output.lines() {
        let (name, upstream_raw) = line.split_once('\0').unwrap_or((line, ""));
        let name = name.trim();
        if name.is_empty() {
            continue;
        }
        let upstream = (!upstream_raw.trim().is_empty()).then(|| upstream_raw.trim().to_owned());
        let sync = sync_for_branch(root, name, upstream.clone());
        let is_checked_out = checked_out.contains(name);
        branches.push(GitBranchEntry {
            name: name.to_owned(),
            current: name == current,
            checked_out: is_checked_out,
            upstream,
            ahead: sync.map(|pair| pair.0),
            behind: sync.map(|pair| pair.1),
            disabled_reason: (is_checked_out && name != current)
                .then(|| "Already checked out".to_owned()),
        });
    }
    branches.sort_by(|left, right| left.name.cmp(&right.name));
    GitBranchList {
        current: (!current.is_empty()).then_some(current),
        detached_oid,
        branches,
    }
}

fn changes_for_path(root: &Path) -> GitChangeSummary {
    let mut summary = GitChangeSummary::default();
    if let Some(numstat) = git_text(root, &["diff", "--numstat", "HEAD", "--"]) {
        for line in numstat.lines() {
            let mut parts = line.split('\t');
            summary.added_lines += parts
                .next()
                .and_then(|v| v.parse::<u64>().ok())
                .unwrap_or(0);
            summary.removed_lines += parts
                .next()
                .and_then(|v| v.parse::<u64>().ok())
                .unwrap_or(0);
        }
    }
    if let Some(status) = git_text(
        root,
        &[
            "--no-optional-locks",
            "status",
            "--porcelain=v1",
            "--untracked-files=all",
        ],
    ) {
        let mut modified = BTreeSet::new();
        let mut untracked = BTreeSet::new();
        for line in status.lines() {
            if line.len() < 3 {
                continue;
            }
            let code = &line[..2];
            let path = line[3..].to_owned();
            if code == "??" {
                untracked.insert(path);
            } else {
                modified.insert(path);
            }
        }
        summary.modified_files = modified.len() as u64;
        summary.untracked_files = untracked.len() as u64;
    }
    summary
}

fn sync_for_path(root: &Path, upstream: Option<String>) -> GitSyncSummary {
    let (ahead, behind) = upstream
        .as_deref()
        .and_then(|upstream| rev_counts(root, "HEAD", upstream))
        .unwrap_or((0, 0));
    GitSyncSummary {
        ahead,
        behind,
        upstream,
    }
}

fn sync_for_branch(root: &Path, branch: &str, upstream: Option<String>) -> Option<(u64, u64)> {
    upstream
        .as_deref()
        .and_then(|upstream| rev_counts(root, branch, upstream))
}

fn rev_counts(root: &Path, left: &str, right: &str) -> Option<(u64, u64)> {
    let spec = format!("{right}...{left}");
    let output = git_text(root, &["rev-list", "--left-right", "--count", &spec])?;
    let mut parts = output.split_whitespace();
    let behind = parts.next()?.parse().ok()?;
    let ahead = parts.next()?.parse().ok()?;
    Some((ahead, behind))
}

fn branch_exists(root: &Path, branch: &str) -> bool {
    !branch.is_empty()
        && run_git(
            root,
            &[
                "show-ref",
                "--verify",
                "--quiet",
                &format!("refs/heads/{branch}"),
            ],
        )
        .is_ok()
}

fn branch_checked_out_elsewhere(root: &Path, branch: &str) -> bool {
    if git_text(root, &["branch", "--show-current"]).as_deref() == Some(branch) {
        return false;
    }
    checked_out_branches(root).contains(branch)
}

fn checked_out_branches(root: &Path) -> BTreeSet<String> {
    let raw = git_text(root, &["worktree", "list", "--porcelain"]).unwrap_or_default();
    raw.lines()
        .filter_map(|line| line.strip_prefix("branch refs/heads/"))
        .map(str::to_owned)
        .collect()
}

fn valid_branch_name(root: &Path, branch: &str) -> bool {
    !branch.is_empty() && run_git(root, &["check-ref-format", "--branch", branch]).is_ok()
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

fn run_git(root: &Path, args: &[&str]) -> Result<(), ()> {
    Command::new("git")
        .arg("-C")
        .arg(root)
        .args(args)
        .output()
        .map_err(|_| ())
        .and_then(|output| output.status.success().then_some(()).ok_or(()))
}

fn bounded_error(
    status: StatusCode,
    error: impl Into<String>,
    git_status: Option<WorkRootGitStatus>,
) -> Response {
    (
        status,
        Json(GitRouteError {
            error: error.into(),
            status: git_status,
        }),
    )
        .into_response()
}

fn now_ms() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_millis())
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn init_fixture_repo() -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "ws-dashboard-git-toolbar-test-{}-{}",
            std::process::id(),
            now_ms()
        ));
        fs::create_dir_all(&dir).expect("create fixture dir");
        run_git(&dir, &["init", "-q"]).expect("git init");
        run_git(&dir, &["config", "user.email", "test@example.com"]).expect("git config email");
        run_git(&dir, &["config", "user.name", "Test"]).expect("git config name");
        fs::write(dir.join("tracked.txt"), "one\n").expect("write tracked.txt");
        run_git(&dir, &["add", "tracked.txt"]).expect("git add");
        run_git(&dir, &["commit", "-q", "-m", "init"]).expect("git commit");
        dir
    }

    #[test]
    fn changes_for_path_reports_modified_and_untracked_without_index_lock() {
        let dir = init_fixture_repo();
        fs::write(dir.join("tracked.txt"), "one\ntwo\n").expect("modify tracked.txt");
        fs::write(dir.join("new.txt"), "new\n").expect("write new.txt");

        let summary = changes_for_path(&dir);
        assert_eq!(summary.modified_files, 1);
        assert_eq!(summary.untracked_files, 1);

        // `--no-optional-locks` must keep the poll's `git status` call from
        // ever creating `.git/index.lock`.
        assert!(!dir.join(".git").join("index.lock").exists());

        fs::remove_dir_all(&dir).ok();
    }
}
