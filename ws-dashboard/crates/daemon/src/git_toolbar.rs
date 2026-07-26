use std::collections::BTreeSet;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use axum::extract::{Path as AxumPath, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::{Deserialize, Serialize};
use ws_dashboard_core::{WorkRootId, WorkRootKind};

use crate::discovery::watch_key;
use crate::git_exec::{capture, git_timeout_from_env, GitFailureExpectation, GitSpawnStats};
use crate::git_state_cache::{git_cache_ttl_from_env, EpochSource, GitStateCache, RefState};
use crate::router::AppState;
use crate::work_root_files::{resolve_online_available_work_root, WorkRootAccessError};

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

// D5 (confirmed): `WorkRootAccessError`'s three variants are byte-identical
// (message and status code) to `GitContextError`'s corresponding three, so
// this mapping is a trivial `From`, not a new conversion layer.
impl From<WorkRootAccessError> for GitContextError {
    fn from(error: WorkRootAccessError) -> Self {
        match error {
            WorkRootAccessError::Unknown => GitContextError::Unknown,
            WorkRootAccessError::Offline => GitContextError::Offline,
            WorkRootAccessError::Unavailable => GitContextError::Unavailable,
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
            Ok(context) => Json(status_for_path(
                &state_for_task.git_state_cache,
                state_for_task.epoch_source.as_ref(),
                &context.root_path,
                &state_for_task.git_spawn_stats,
            ))
            .into_response(),
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
            Ok(context) => Json(branches_for_path(
                &state_for_task.git_state_cache,
                state_for_task.epoch_source.as_ref(),
                &context.root_path,
                &state_for_task.git_spawn_stats,
            ))
            .into_response(),
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
        let stats: &GitSpawnStats = &state_for_task.git_spawn_stats;
        let cache = &state_for_task.git_state_cache;
        let epoch_source = state_for_task.epoch_source.as_ref();
        let context = match resolve_git_context(&state_for_task, &work_root_id) {
            Ok(context) => context,
            Err(error) => return bounded_error(error.status_code(), error.message(), None),
        };
        let branch = request.branch_name.trim();
        if !branch_exists(&context.root_path, stats, branch) {
            return bounded_error(
                StatusCode::BAD_REQUEST,
                "branch is unavailable",
                Some(status_for_path(cache, epoch_source, &context.root_path, stats)),
            );
        }
        if branch_checked_out_elsewhere(&context.root_path, stats, branch) {
            return bounded_error(
                StatusCode::BAD_REQUEST,
                "branch is already checked out",
                Some(status_for_path(cache, epoch_source, &context.root_path, stats)),
            );
        }
        match run_git(
            stats,
            &context.root_path,
            &["switch", branch],
            GitFailureExpectation::Unexpected,
        ) {
            Ok(()) => {
                // A user-initiated switch is never TTL-delayed: bump the
                // refs epoch before the now-cache-aware status read below, so
                // that read misses the (now-stale) cached refs slot.
                epoch_source.bump_refs(&watch_key(&context.root_path));
                Json(status_for_path(cache, epoch_source, &context.root_path, stats)).into_response()
            }
            Err(_) => bounded_error(
                StatusCode::BAD_REQUEST,
                "branch switch failed",
                Some(status_for_path(cache, epoch_source, &context.root_path, stats)),
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
        let stats: &GitSpawnStats = &state_for_task.git_spawn_stats;
        let cache = &state_for_task.git_state_cache;
        let epoch_source = state_for_task.epoch_source.as_ref();
        let context = match resolve_git_context(&state_for_task, &work_root_id) {
            Ok(context) => context,
            Err(error) => return bounded_error(error.status_code(), error.message(), None),
        };
        if !request.switch_to {
            return bounded_error(
                StatusCode::BAD_REQUEST,
                "create without switch is unsupported",
                Some(status_for_path(cache, epoch_source, &context.root_path, stats)),
            );
        }
        let branch = request.branch_name.trim();
        if !valid_branch_name(&context.root_path, stats, branch)
            || branch_exists(&context.root_path, stats, branch)
            || branch_checked_out_elsewhere(&context.root_path, stats, branch)
        {
            return bounded_error(
                StatusCode::BAD_REQUEST,
                "branch cannot be created",
                Some(status_for_path(cache, epoch_source, &context.root_path, stats)),
            );
        }
        let mut args = vec!["switch", "-c", branch];
        if let Some(base) = request
            .base_branch
            .as_deref()
            .map(str::trim)
            .filter(|base| !base.is_empty())
        {
            if !branch_exists(&context.root_path, stats, base) {
                return bounded_error(
                    StatusCode::BAD_REQUEST,
                    "base branch is unavailable",
                    Some(status_for_path(cache, epoch_source, &context.root_path, stats)),
                );
            }
            args.push(base);
        }
        match run_git(stats, &context.root_path, &args, GitFailureExpectation::Unexpected) {
            Ok(()) => {
                // See git_switch_branch: never TTL-delay a user-initiated
                // mutation.
                epoch_source.bump_refs(&watch_key(&context.root_path));
                Json(status_for_path(cache, epoch_source, &context.root_path, stats)).into_response()
            }
            Err(_) => bounded_error(
                StatusCode::BAD_REQUEST,
                "branch create failed",
                Some(status_for_path(cache, epoch_source, &context.root_path, stats)),
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
        EpochBump::RefsOnly,
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
        EpochBump::RefsOnly,
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
        EpochBump::RefsAndWorktree,
    )
    .await
}

/// Which cache axes a `mutate_no_body` mutation invalidates. `fetch`/`push`
/// only move refs (branch tips, sync counts); `pull --ff-only` also changes
/// the working tree, so it must additionally invalidate the worktree slot
/// (`changes_for_path`'s modified/untracked counts).
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum EpochBump {
    RefsOnly,
    RefsAndWorktree,
}

async fn mutate_no_body(
    state: AppState,
    work_root_id: WorkRootId,
    args: &'static [&'static str],
    failure: &'static str,
    epoch_bump: EpochBump,
) -> Response {
    tokio::task::spawn_blocking(move || {
        let stats: &GitSpawnStats = &state.git_spawn_stats;
        let cache = &state.git_state_cache;
        let epoch_source = state.epoch_source.as_ref();
        let context = match resolve_git_context(&state, &work_root_id) {
            Ok(context) => context,
            Err(error) => return bounded_error(error.status_code(), error.message(), None),
        };
        match run_git(stats, &context.root_path, args, GitFailureExpectation::Unexpected) {
            Ok(()) => {
                let key = watch_key(&context.root_path);
                epoch_source.bump_refs(&key);
                if epoch_bump == EpochBump::RefsAndWorktree {
                    epoch_source.bump_worktree(&key);
                }
                Json(status_for_path(cache, epoch_source, &context.root_path, stats)).into_response()
            }
            Err(_) => bounded_error(
                StatusCode::BAD_REQUEST,
                failure,
                Some(status_for_path(cache, epoch_source, &context.root_path, stats)),
            ),
        }
    })
    .await
    .expect("git mutation task panicked")
}

// D1/D4 (Phase 2 Lead Dispositions): resolves ONE root's git context directly
// (no more full-registry discovery scan, `2N+W` spawns -> at most 1). The
// 404/409/409 gate reuses `resolve_online_available_work_root` as-is (D5) -
// availability (moved/missing/inaccessible) is already fully settled by that
// gate's live `is_dir`/`read_dir` check, so the only question left is
// git-vs-plain, answered by `GitProbeCache::git_root_kind` off the same
// memoized probe the Activity path's `git_identity` shares (D1's actual win).
fn resolve_git_context(
    state: &AppState,
    work_root_id: &WorkRootId,
) -> Result<GitContext, GitContextError> {
    let root_path = resolve_online_available_work_root(state, work_root_id)?;
    match state
        .git_probe_cache
        .git_root_kind(&root_path, &state.git_spawn_stats)
    {
        Some(WorkRootKind::GitPrimaryRoot) | Some(WorkRootKind::GitLinkedWorktree) => {
            Ok(GitContext { root_path })
        }
        // Explicit match against the two known git kinds (D4), not "any
        // Some", so a future third git kind cannot pass silently.
        Some(WorkRootKind::PlainDirectory) | None => Err(GitContextError::NonGit),
    }
}

fn status_for_path(
    cache: &GitStateCache,
    epoch_source: &dyn EpochSource,
    root: &Path,
    stats: &GitSpawnStats,
) -> WorkRootGitStatus {
    let key = watch_key(root);
    // D7: sample both epochs once, before either fill closure runs, so a
    // concurrent mutation landing mid-`git`-spawn is caught as a miss on the
    // NEXT read rather than being silently blessed into this slot.
    let (worktree_epoch, refs_epoch) = epoch_source.epochs(&key);
    let ttl = git_cache_ttl_from_env();
    let changes = cache.worktree(&key, worktree_epoch, ttl, || changes_for_path(root, stats));
    let refs = cache.refs(&key, refs_epoch, ttl, || compute_ref_state(root, stats));
    WorkRootGitStatus {
        available: true,
        reason: None,
        branch: Some(GitStatusBranch {
            name: refs.branch_name.clone(),
            detached_oid: refs.detached_oid.clone(),
            upstream: refs.upstream.clone(),
        }),
        changes,
        sync: refs.sync.clone(),
        operations: Some(GitOperationAvailability {
            can_fetch: true,
            can_push: refs.sync.upstream.is_some() && refs.sync.ahead > 0,
            can_pull_ff_only: refs.sync.upstream.is_some() && refs.sync.behind > 0,
        }),
        refreshed_at_ms: now_ms(),
    }
}

fn branches_for_path(
    cache: &GitStateCache,
    epoch_source: &dyn EpochSource,
    root: &Path,
    stats: &GitSpawnStats,
) -> GitBranchList {
    let key = watch_key(root);
    // Only the refs epoch matters here, but it is still sampled through the
    // same `epochs()` call as `status_for_path` (D7) - `branches_for_path`
    // simply never calls `cache.worktree`.
    let (_worktree_epoch, refs_epoch) = epoch_source.epochs(&key);
    let ttl = git_cache_ttl_from_env();
    let refs = cache.refs(&key, refs_epoch, ttl, || compute_ref_state(root, stats));
    GitBranchList {
        current: refs.branch_name.clone(),
        detached_oid: refs.detached_oid.clone(),
        branches: refs.branch_list.clone(),
    }
}

/// The refs-slot fill closure for `GitStateCache::refs`: the union of what
/// `status_for_path` and `branches_for_path` used to each compute
/// independently (Phase 3 D1). Folds the former inline branch-name/
/// detached-oid/upstream/sync computation together with the former inline
/// branch-list/checked-out computation into one function, so the two routes'
/// concurrent per-tick `Promise.all` fetch collapses onto one refs
/// computation instead of two (D2, backed by `GitStateCache::refs`'s
/// per-key single-flight lock).
fn compute_ref_state(root: &Path, stats: &GitSpawnStats) -> RefState {
    let branch_name = git_text(
        stats,
        root,
        &["branch", "--show-current"],
        GitFailureExpectation::Unexpected,
    )
    .unwrap_or_default();
    let detached_oid = if branch_name.is_empty() {
        // Fails routinely on an unborn HEAD (no commits yet).
        git_text(
            stats,
            root,
            &["rev-parse", "--short", "HEAD"],
            GitFailureExpectation::ExpectedNonZero,
        )
    } else {
        None
    };
    let upstream = git_text(
        stats,
        root,
        &[
            "rev-parse",
            "--abbrev-ref",
            "--symbolic-full-name",
            "@{upstream}",
        ],
        // Fails routinely for every branch with no upstream configured.
        GitFailureExpectation::ExpectedNonZero,
    );
    // Computed as raw `Option<(ahead, behind)>` (not through `sync_for_path`,
    // which defaults a failed/absent lookup to `(0, 0)`) so the exact
    // "no data" vs "zero" distinction can be reused below for the current
    // branch's `branch_list` entry without changing its null-vs-0 shape.
    let current_branch_counts = upstream
        .as_deref()
        .and_then(|upstream| rev_counts(root, stats, "HEAD", upstream));
    let sync = GitSyncSummary {
        ahead: current_branch_counts.map(|pair| pair.0).unwrap_or(0),
        behind: current_branch_counts.map(|pair| pair.1).unwrap_or(0),
        upstream: upstream.clone(),
    };
    let checked_out = checked_out_branches(root, stats);
    let output = git_text(
        stats,
        root,
        &[
            "for-each-ref",
            "--format=%(refname:short)%00%(upstream:short)",
            "refs/heads",
        ],
        GitFailureExpectation::Unexpected,
    )
    .unwrap_or_default();
    let mut branch_list = Vec::new();
    for line in output.lines() {
        let (name, upstream_raw) = line.split_once('\0').unwrap_or((line, ""));
        let name = name.trim();
        if name.is_empty() {
            continue;
        }
        let branch_upstream =
            (!upstream_raw.trim().is_empty()).then(|| upstream_raw.trim().to_owned());
        let is_current = name == branch_name;
        // D1's whole point: `current_branch_counts` above already answered
        // "ahead/behind vs upstream" for the checked-out branch via one
        // `rev-list` call - reuse it here (guarded by the upstream string
        // actually matching, so a `for-each-ref`/`@{upstream}` disagreement
        // falls back to a fresh, independently-correct spawn) instead of
        // re-running the exact same comparison a second time through
        // `sync_for_branch`. This is the specific duplicate (current-branch
        // `rev-list --left-right --count`, once from the old
        // `status_for_path` and once from the old `branches_for_path`) the
        // union refs slot exists to collapse.
        let branch_sync = if is_current && branch_upstream == upstream {
            current_branch_counts
        } else {
            sync_for_branch(root, stats, name, branch_upstream.clone())
        };
        let is_checked_out = checked_out.contains(name);
        branch_list.push(GitBranchEntry {
            name: name.to_owned(),
            current: is_current,
            checked_out: is_checked_out,
            upstream: branch_upstream,
            ahead: branch_sync.map(|pair| pair.0),
            behind: branch_sync.map(|pair| pair.1),
            disabled_reason: (is_checked_out && !is_current)
                .then(|| "Already checked out".to_owned()),
        });
    }
    branch_list.sort_by(|left, right| left.name.cmp(&right.name));
    RefState {
        branch_name: (!branch_name.is_empty()).then_some(branch_name),
        detached_oid,
        upstream,
        sync,
        branch_list,
        checked_out,
    }
}

pub(crate) fn changes_for_path(root: &Path, stats: &GitSpawnStats) -> GitChangeSummary {
    let mut summary = GitChangeSummary::default();
    // Use the plumbing `diff-index` instead of the porcelain `diff`. The
    // porcelain form opportunistically refreshes and rewrites the on-disk
    // index for stat-dirty-but-content-clean files, which takes
    // `.git/index.lock` and collides with agents' own git operations in the
    // same worktree on every 5s poll tick. `--no-optional-locks` is verified
    // ineffective on `diff`; only the plumbing form is lock-free (it is added
    // here as harmless defense-in-depth, matching the sibling status call).
    // `-M` restores the rename detection that porcelain `diff` enables by
    // default (and plumbing does not), keeping the `<added>\t<removed>` totals
    // byte-identical to the prior command across modified, rename, and
    // mode-change cases. Mode-only changes surface as `0\t0\tpath`, summed
    // harmlessly below.
    // ExpectedNonZero: `diff-index ... HEAD` exits 128 (`fatal: bad revision
    // 'HEAD'`) on an unborn HEAD, which is a routine state on this 5s poll
    // path - `GitDiscovery::probe` classifies a freshly-`git init`ed root as a
    // primary root and `branch --show-current` succeeds there, so
    // `status_for_path` reaches this call. Warning would emit one line per poll
    // per such root, which is the noise the warn policy exists to prevent. Same
    // precondition as the sibling `rev-parse --short HEAD` above, which is
    // already ExpectedNonZero. Failure leaves the line totals at zero.
    if let Some(numstat) = git_text(
        stats,
        root,
        &[
            "--no-optional-locks",
            "diff-index",
            "-M",
            "--numstat",
            "HEAD",
            "--",
        ],
        GitFailureExpectation::ExpectedNonZero,
    ) {
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
        stats,
        root,
        &[
            "--no-optional-locks",
            "status",
            "--porcelain=v1",
            "--untracked-files=all",
        ],
        GitFailureExpectation::Unexpected,
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

fn sync_for_branch(
    root: &Path,
    stats: &GitSpawnStats,
    branch: &str,
    upstream: Option<String>,
) -> Option<(u64, u64)> {
    upstream
        .as_deref()
        .and_then(|upstream| rev_counts(root, stats, branch, upstream))
}

fn rev_counts(root: &Path, stats: &GitSpawnStats, left: &str, right: &str) -> Option<(u64, u64)> {
    let spec = format!("{right}...{left}");
    let output = git_text(
        stats,
        root,
        &["rev-list", "--left-right", "--count", &spec],
        GitFailureExpectation::Unexpected,
    )?;
    let mut parts = output.split_whitespace();
    let behind = parts.next()?.parse().ok()?;
    let ahead = parts.next()?.parse().ok()?;
    Some((ahead, behind))
}

fn branch_exists(root: &Path, stats: &GitSpawnStats, branch: &str) -> bool {
    // Routinely non-zero for a branch name that does not exist - that's the
    // check's entire purpose.
    !branch.is_empty()
        && run_git(
            stats,
            root,
            &[
                "show-ref",
                "--verify",
                "--quiet",
                &format!("refs/heads/{branch}"),
            ],
            GitFailureExpectation::ExpectedNonZero,
        )
        .is_ok()
}

fn branch_checked_out_elsewhere(root: &Path, stats: &GitSpawnStats, branch: &str) -> bool {
    if git_text(
        stats,
        root,
        &["branch", "--show-current"],
        GitFailureExpectation::Unexpected,
    )
    .as_deref()
        == Some(branch)
    {
        return false;
    }
    checked_out_branches(root, stats).contains(branch)
}

fn checked_out_branches(root: &Path, stats: &GitSpawnStats) -> BTreeSet<String> {
    let raw = git_text(
        stats,
        root,
        &["worktree", "list", "--porcelain"],
        GitFailureExpectation::Unexpected,
    )
    .unwrap_or_default();
    raw.lines()
        .filter_map(|line| line.strip_prefix("branch refs/heads/"))
        .map(str::to_owned)
        .collect()
}

fn valid_branch_name(root: &Path, stats: &GitSpawnStats, branch: &str) -> bool {
    // Validates user-entered branch names; a bad name is an expected
    // non-zero exit, not a daemon-side failure.
    !branch.is_empty()
        && run_git(
            stats,
            root,
            &["check-ref-format", "--branch", branch],
            GitFailureExpectation::ExpectedNonZero,
        )
        .is_ok()
}

/// Every caller parses this stdout, so a truncated collection must read as
/// "no answer" rather than as a short one: `stdout_text` returns `None` when
/// `GitOutcome::output_truncated` is set.
fn git_text(
    stats: &GitSpawnStats,
    root: &Path,
    args: &[&str],
    expect: GitFailureExpectation,
) -> Option<String> {
    capture(stats, root, args, expect, git_timeout_from_env())
        .ok()
        .and_then(|outcome| outcome.stdout_text().map(|text| text.trim().to_owned()))
}

/// Status-only: the output is discarded, so `GitOutcome::output_truncated` is
/// deliberately ignored. A `push`/`fetch`/`switch` whose exit status was observed
/// must report that status even when a lingering `ssh` master kept the pipes
/// open — there is no parsed output here to be short.
fn run_git(
    stats: &GitSpawnStats,
    root: &Path,
    args: &[&str],
    expect: GitFailureExpectation,
) -> Result<(), ()> {
    capture(stats, root, args, expect, git_timeout_from_env())
        .map(|_| ())
        .map_err(|_| ())
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
    use std::sync::atomic::{AtomicU64, Ordering};
    use std::time::Duration;

    // Disambiguates fixture directories across tests running in the same
    // process at the same millisecond (parallel test execution).
    static FIXTURE_SEQ: AtomicU64 = AtomicU64::new(0);

    fn test_run_git(stats: &GitSpawnStats, dir: &Path, args: &[&str]) {
        run_git(stats, dir, args, GitFailureExpectation::Unexpected).expect("run_git");
    }

    fn init_fixture_repo(stats: &GitSpawnStats) -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "ws-dashboard-git-toolbar-test-{}-{}-{}",
            std::process::id(),
            now_ms(),
            FIXTURE_SEQ.fetch_add(1, Ordering::Relaxed)
        ));
        fs::create_dir_all(&dir).expect("create fixture dir");
        test_run_git(stats, &dir, &["init", "-q"]);
        test_run_git(stats, &dir, &["config", "user.email", "test@example.com"]);
        test_run_git(stats, &dir, &["config", "user.name", "Test"]);
        fs::write(dir.join("tracked.txt"), "one\n").expect("write tracked.txt");
        test_run_git(stats, &dir, &["add", "tracked.txt"]);
        test_run_git(stats, &dir, &["commit", "-q", "-m", "init"]);
        dir
    }

    #[test]
    fn changes_for_path_reports_modified_and_untracked_without_index_lock() {
        let stats = GitSpawnStats::default();
        let dir = init_fixture_repo(&stats);
        fs::write(dir.join("tracked.txt"), "one\ntwo\n").expect("modify tracked.txt");
        fs::write(dir.join("new.txt"), "new\n").expect("write new.txt");

        let summary = changes_for_path(&dir, &stats);
        assert_eq!(summary.modified_files, 1);
        assert_eq!(summary.untracked_files, 1);

        // `--no-optional-locks` must keep the poll's `git status` call from
        // ever creating `.git/index.lock`.
        assert!(!dir.join(".git").join("index.lock").exists());

        fs::remove_dir_all(&dir).ok();
    }

    /// Make `tracked.txt` stat-dirty but content-clean: identical content with
    /// an mtime pushed into the future so it no longer matches the index's
    /// cached stat data. This is the exact state in which porcelain
    /// `git diff --numstat` opportunistically refreshes and rewrites
    /// `.git/index` (taking `.git/index.lock`); the plumbing `git diff-index`
    /// form never does. Uses `File::set_modified` rather than a real sleep so
    /// the trigger is deterministic without wall-clock delay.
    fn make_stat_dirty_content_clean(dir: &Path) {
        fs::write(dir.join("tracked.txt"), "one\n").expect("rewrite identical content");
        let file = fs::OpenOptions::new()
            .write(true)
            .open(dir.join("tracked.txt"))
            .expect("open tracked.txt");
        file.set_modified(SystemTime::now() + Duration::from_secs(5))
            .expect("bump mtime into the future");
    }

    /// Non-vacuous lock pin: assert `changes_for_path` leaves `.git/index`
    /// byte-identical. A rewrite of the index is precisely what forces git to
    /// take `.git/index.lock` mid-call — an in-flight lock the prior test
    /// (which only checked lock *absence after* the call) could never catch.
    /// Reads the raw index bytes immediately before and after the call. This
    /// FAILS against the old `git diff --numstat HEAD --` command, which
    /// rewrites the index in this stat-dirty-content-clean state, and PASSES
    /// against the plumbing `git diff-index` form that this fix installs.
    #[test]
    fn changes_for_path_does_not_rewrite_index_or_take_lock() {
        let stats = GitSpawnStats::default();
        let dir = init_fixture_repo(&stats);
        make_stat_dirty_content_clean(&dir);

        let index_path = dir.join(".git").join("index");
        let before = fs::read(&index_path).expect("read index before");

        let _summary = changes_for_path(&dir, &stats);

        let after = fs::read(&index_path).expect("read index after");
        assert_eq!(
            before, after,
            "changes_for_path must not rewrite .git/index (a rewrite takes .git/index.lock)"
        );
        assert!(!dir.join(".git").join("index.lock").exists());

        fs::remove_dir_all(&dir).ok();
    }

    /// Output-parity pin across the three cases the prior test never exercised:
    /// a modified tracked file, a staged pure rename, and a mode-only change.
    /// The plumbing swap must keep the accumulated added/removed line totals
    /// identical to the old porcelain command: `+2/-0` for the modification,
    /// `0/0` for the rename (rename detection restored via `-M`), and `0/0`
    /// for the mode change (emitted as `0\t0\tpath`, summed harmlessly).
    #[test]
    fn changes_for_path_line_totals_cover_modified_rename_and_mode_change() {
        let stats = GitSpawnStats::default();
        let dir = init_fixture_repo(&stats);

        // Extra tracked files, committed clean, for the rename and mode cases.
        fs::write(dir.join("rename-me.txt"), "r1\nr2\n").expect("write rename-me.txt");
        fs::write(dir.join("mode.txt"), "x\n").expect("write mode.txt");
        test_run_git(&stats, &dir, &["add", "rename-me.txt", "mode.txt"]);
        test_run_git(&stats, &dir, &["commit", "-q", "-m", "extra"]);

        // Modified tracked file: +2 lines, -0.
        fs::write(dir.join("tracked.txt"), "one\ntwo\nthree\n").expect("modify tracked.txt");
        // Staged pure rename (identical content => identical blob OID). This is
        // an *exact* rename, matched by git's exact-rename pass independent of
        // `-M`'s similarity threshold, so the 0/0 result is deterministic and
        // not marginal-similarity sensitive (see the follow-up flake probe).
        test_run_git(&stats, &dir, &["mv", "rename-me.txt", "renamed.txt"]);
        // Mode-only change (content unchanged).
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mode_path = dir.join("mode.txt");
            let mut perms = fs::metadata(&mode_path)
                .expect("stat mode.txt")
                .permissions();
            perms.set_mode(0o755);
            fs::set_permissions(&mode_path, perms).expect("chmod mode.txt");
        }

        let summary = changes_for_path(&dir, &stats);
        assert_eq!(
            summary.added_lines, 2,
            "added lines (modified +2, rename/mode 0)"
        );
        assert_eq!(
            summary.removed_lines, 0,
            "removed lines (rename detected, mode-only)"
        );

        fs::remove_dir_all(&dir).ok();
    }

    /// Real-world-scenario guard for the ticket's Phase 1 requirement: while an
    /// agent holds `.git/index.lock` mid-`git add`/`commit` in the same
    /// worktree, the dashboard poll must (a) still return the correct summary
    /// and (b) leave that externally-held lock byte-for-byte untouched — never
    /// waiting on it, deleting it, or clobbering it. This exercises the whole
    /// `changes_for_path` (both the `diff-index` and the sibling
    /// `git status --porcelain=v1 --no-optional-locks` plumbing calls), which
    /// the index byte-compare test could not: that test pins that the *index*
    /// is not rewritten, whereas this one pins that a *pre-existing external
    /// lock* is not disturbed and does not corrupt the poll result.
    #[test]
    fn changes_for_path_leaves_externally_held_index_lock_untouched() {
        let stats = GitSpawnStats::default();
        let dir = init_fixture_repo(&stats);
        fs::write(dir.join("tracked.txt"), "one\ntwo\n").expect("modify tracked.txt");
        fs::write(dir.join("new.txt"), "new\n").expect("write new.txt");

        // Baseline summary with no lock present.
        let expected = changes_for_path(&dir, &stats);
        assert_eq!(expected.modified_files, 1, "baseline modified files");
        assert_eq!(expected.untracked_files, 1, "baseline untracked files");
        assert_eq!(expected.added_lines, 1, "baseline added lines");

        // Simulate an agent holding the index lock mid-operation.
        let lock_path = dir.join(".git").join("index.lock");
        let lock_bytes = b"STRAY-AGENT-LOCK\n".to_vec();
        fs::write(&lock_path, &lock_bytes).expect("create stray index.lock");

        let with_lock = changes_for_path(&dir, &stats);

        // (a) The poll result is unaffected by the externally-held lock.
        assert_eq!(
            with_lock, expected,
            "summary must be identical whether or not an external index.lock is held"
        );
        // (b) The externally-held lock is left exactly as the agent wrote it.
        assert!(
            lock_path.exists(),
            "external index.lock must not be deleted"
        );
        assert_eq!(
            fs::read(&lock_path).expect("read index.lock"),
            lock_bytes,
            "external index.lock must be left byte-for-byte untouched"
        );

        fs::remove_file(&lock_path).ok();
        fs::remove_dir_all(&dir).ok();
    }
}
