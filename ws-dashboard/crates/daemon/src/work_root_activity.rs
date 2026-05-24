use std::collections::{BTreeMap, VecDeque};
use std::convert::Infallible;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use axum::extract::{Path as AxumPath, Query, State};
use axum::http::StatusCode;
use axum::response::sse::{Event, Sse};
use axum::response::{IntoResponse, Response};
use axum::Json;
use futures_util::stream::{self, Stream};
use serde::{Deserialize, Serialize};
use ws_dashboard_core::{
    ActivityConsoleEvent, ActivityFeed, ActivityItem, ActivitySnapshotInvalidationReason,
    ActivitySourceDisplay, ActivityTranscript, ActivityTranscriptAvailability, ActivityUpdateMode,
    NamedAgentActivityView, NamedAgentCallActivityView, TranscriptBlock, WorkRootActivitySummary,
    WorkRootActivityView, WorkRootId,
};

use crate::router::AppState;
use crate::work_root_files::{resolve_online_available_work_root, WorkRootAccessError};

/// Upper bound applied to the wsagent-reported backend-error string so an
/// oversized `current/state.json` error cannot bloat the projection response.
/// Daemon-emitted diagnostics are fixed short constants and need no bounding.
const MAX_BOUNDED_TEXT: usize = 280;
const MAX_NATIVE_MESSAGE_TEXT: usize = 8_192;
const TOOL_OUTPUT_HEAD_LINES: usize = 10;
const TOOL_OUTPUT_TAIL_LINES: usize = 10;

const DIAG_METADATA_MISSING: &str = "agent metadata missing";
const DIAG_METADATA_UNREADABLE: &str = "agent metadata unreadable";
const DIAG_STATUS_UNAVAILABLE: &str = "agent status unavailable";
const DIAG_STATUS_UNRECOGNIZED: &str = "agent status unrecognized";
const DIAG_CURRENT_CALL_UNREADABLE: &str = "current call state unreadable";

const STATUS_UNAVAILABLE: &str = "unavailable";
const MAX_RECENT_ACTIVITY_LIMIT: usize = 30;
const DEFAULT_TRANSCRIPT_LIMIT: usize = 20;
const MAX_TRANSCRIPT_LIMIT: usize = 100;
const MAX_CODEX_SESSION_SCAN_ENTRIES: usize = 4096;
const ACTIVITY_KIND_NAMED_AGENT: &str = "namedAgent";
const ACTIVITY_ID_NAMED_AGENT_PREFIX: &str = "agent:";

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct WorkRootActivityError {
    error: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkRootActivityQuery {
    recent_limit: Option<usize>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ActivityTranscriptQuery {
    cursor: Option<String>,
    before: Option<String>,
    limit: Option<usize>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ActivityEventsQuery {
    after: Option<String>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct WorkRootActivityProjectionConfig {
    pub codex_home: Option<PathBuf>,
    // HINT: The wsstate Go manager accepts `WS_CACHE_HOME` as its cache-home
    // override. The dashboard keeps that override daemon-side so tests can
    // point at fixture cache trees without making browser API identity depend
    // on cache paths.
    pub cache_home: Option<PathBuf>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct WorkRootActivityProjector {
    cache_home: Option<PathBuf>,
    codex_home: Option<PathBuf>,
}

impl WorkRootActivityProjector {
    pub fn new(config: WorkRootActivityProjectionConfig) -> Self {
        Self {
            cache_home: config.cache_home,
            codex_home: config.codex_home,
        }
    }

    pub async fn project(
        &self,
        work_root_id: WorkRootId,
        root_path: &Path,
    ) -> WorkRootActivityView {
        self.project_with_recent_limit(work_root_id, root_path, None)
            .await
    }

    pub async fn project_with_recent_limit(
        &self,
        work_root_id: WorkRootId,
        root_path: &Path,
        recent_limit: Option<usize>,
    ) -> WorkRootActivityView {
        // CONTRACT: Phase 1 reads wsstate/wsagent agent records for this opened
        // workRoot through daemon-owned projection logic. Browser callers never
        // receive cache paths, host paths, session ids, pids, or stream paths.
        //
        // The wsstate Git layout discovery, synchronous cache scanning, and JSON
        // parsing all run on a blocking pool so they never stall an Axum async
        // worker thread.
        let cache_home = self.cache_home.clone();
        let codex_home = self.codex_home.clone();
        let root_path = root_path.to_path_buf();
        let recent_limit = normalize_recent_activity_limit(recent_limit);
        tokio::task::spawn_blocking(move || {
            project_blocking(
                work_root_id,
                &root_path,
                cache_home.as_deref(),
                codex_home.as_deref(),
                recent_limit,
            )
        })
        .await
        .expect("workRoot activity projection task panicked")
    }

    pub async fn named_agent_transcript(
        &self,
        work_root_id: WorkRootId,
        root_path: &Path,
        activity_id: String,
        agent_key: String,
        cursor: Option<String>,
        before: Option<String>,
        limit: Option<usize>,
    ) -> ActivityTranscript {
        let cache_home = self.cache_home.clone();
        let codex_home = self.codex_home.clone();
        let root_path = root_path.to_path_buf();
        tokio::task::spawn_blocking(move || {
            named_agent_transcript_blocking(
                work_root_id,
                &root_path,
                cache_home.as_deref(),
                codex_home.as_deref(),
                activity_id,
                &agent_key,
                cursor.as_deref(),
                before.as_deref(),
                normalize_transcript_limit(limit),
            )
        })
        .await
        .expect("workRoot activity transcript task panicked")
    }

    async fn watch_snapshot(
        &self,
        work_root_id: WorkRootId,
        root_path: &Path,
    ) -> ActivityWatchSnapshot {
        let cache_home = self.cache_home.clone();
        let codex_home = self.codex_home.clone();
        let root_path = root_path.to_path_buf();
        tokio::task::spawn_blocking(move || {
            watch_snapshot_blocking(
                work_root_id,
                &root_path,
                cache_home.as_deref(),
                codex_home.as_deref(),
            )
        })
        .await
        .expect("workRoot activity watch snapshot task panicked")
    }
}

pub async fn work_root_activity(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Query(query): Query<WorkRootActivityQuery>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let root_path = match resolve_online_available_work_root(&state, &work_root_id) {
        Ok(root_path) => root_path,
        Err(error) => return activity_access_error(error),
    };

    Json(
        state
            .work_root_activity
            .project_with_recent_limit(work_root_id, &root_path, query.recent_limit)
            .await,
    )
    .into_response()
}

pub async fn work_root_activity_transcript(
    State(state): State<AppState>,
    AxumPath((work_root_id, activity_id)): AxumPath<(String, String)>,
    Query(query): Query<ActivityTranscriptQuery>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let root_path = match resolve_online_available_work_root(&state, &work_root_id) {
        Ok(root_path) => root_path,
        Err(error) => return activity_access_error(error),
    };

    let Some(agent_key) = named_agent_key_from_activity_id(&activity_id) else {
        return activity_error(StatusCode::NOT_FOUND, "unknown activity");
    };

    Json(
        state
            .work_root_activity
            .named_agent_transcript(
                work_root_id,
                &root_path,
                activity_id,
                agent_key,
                query.cursor,
                query.before,
                query.limit,
            )
            .await,
    )
    .into_response()
}

pub async fn work_root_activity_events(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
    Query(query): Query<ActivityEventsQuery>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let root_path = match resolve_online_available_work_root(&state, &work_root_id) {
        Ok(root_path) => root_path,
        Err(error) => return activity_access_error(error),
    };

    let snapshot = state
        .work_root_activity
        .watch_snapshot(work_root_id.clone(), &root_path)
        .await;
    let stream = ActivityEventPollStream::new(
        state.work_root_activity.clone(),
        work_root_id,
        root_path,
        query.after,
        snapshot,
    )
    .into_stream();

    Sse::new(stream).into_response()
}

#[derive(Clone, Debug)]
struct ActivityWatchSnapshot {
    items: BTreeMap<String, ActivityItem>,
    item_versions: BTreeMap<String, String>,
    transcript_cursors: BTreeMap<String, Option<String>>,
}

struct ActivityEventPollStream {
    projector: WorkRootActivityProjector,
    work_root_id: WorkRootId,
    root_path: PathBuf,
    previous: ActivityWatchSnapshot,
    pending: VecDeque<ActivityConsoleEvent>,
    next_cursor: u64,
}

impl ActivityEventPollStream {
    fn new(
        projector: WorkRootActivityProjector,
        work_root_id: WorkRootId,
        root_path: PathBuf,
        after: Option<String>,
        snapshot: ActivityWatchSnapshot,
    ) -> Self {
        let mut next_cursor = after
            .as_deref()
            .and_then(|cursor| cursor.parse::<u64>().ok())
            .unwrap_or(0)
            .saturating_add(1);
        let mut pending = VecDeque::new();

        if after.is_some() {
            pending.push_back(ActivityConsoleEvent::SnapshotInvalidated {
                cursor: cursor_string(&mut next_cursor),
                reason: if after
                    .as_deref()
                    .and_then(|cursor| cursor.parse::<u64>().ok())
                    .is_some()
                {
                    ActivitySnapshotInvalidationReason::WatchReset
                } else {
                    ActivitySnapshotInvalidationReason::Overflow
                },
            });
        }

        pending.push_back(ActivityConsoleEvent::ModeChanged {
            cursor: cursor_string(&mut next_cursor),
            update_mode: ActivityUpdateMode::PollFallback,
        });
        pending.push_back(ActivityConsoleEvent::SnapshotInvalidated {
            cursor: cursor_string(&mut next_cursor),
            reason: ActivitySnapshotInvalidationReason::Fallback,
        });
        for item in snapshot.items.values() {
            pending.push_back(ActivityConsoleEvent::ItemUpserted {
                cursor: cursor_string(&mut next_cursor),
                item: item.clone(),
            });
        }
        pending.push_back(ActivityConsoleEvent::Heartbeat {
            cursor: cursor_string(&mut next_cursor),
        });

        Self {
            projector,
            work_root_id,
            root_path,
            previous: snapshot,
            pending,
            next_cursor,
        }
    }

    fn into_stream(self) -> impl Stream<Item = Result<Event, Infallible>> {
        stream::unfold(self, |mut state| async move {
            if state.pending.is_empty() {
                tokio::time::sleep(Duration::from_millis(200)).await;
                let next = state
                    .projector
                    .watch_snapshot(state.work_root_id.clone(), &state.root_path)
                    .await;
                state.enqueue_diff(next);
            }

            let event =
                state
                    .pending
                    .pop_front()
                    .unwrap_or_else(|| ActivityConsoleEvent::Heartbeat {
                        cursor: cursor_string(&mut state.next_cursor),
                    });
            let data = serde_json::to_string(&event).expect("serialize activity event");
            Some((Ok(Event::default().event("activity").data(data)), state))
        })
    }

    fn enqueue_diff(&mut self, next: ActivityWatchSnapshot) {
        for activity_id in self.previous.items.keys() {
            if !next.items.contains_key(activity_id) {
                self.pending.push_back(ActivityConsoleEvent::ItemRemoved {
                    cursor: cursor_string(&mut self.next_cursor),
                    activity_id: activity_id.clone(),
                });
            }
        }

        for (activity_id, item) in &next.items {
            let changed_item = self.previous.items.get(activity_id) != Some(item)
                || self.previous.item_versions.get(activity_id)
                    != next.item_versions.get(activity_id);
            if changed_item {
                self.pending.push_back(ActivityConsoleEvent::ItemUpserted {
                    cursor: cursor_string(&mut self.next_cursor),
                    item: item.clone(),
                });
            }

            if self.previous.transcript_cursors.get(activity_id)
                != next.transcript_cursors.get(activity_id)
                || self.previous.item_versions.get(activity_id)
                    != next.item_versions.get(activity_id)
            {
                self.pending
                    .push_back(ActivityConsoleEvent::TranscriptUpdated {
                        cursor: cursor_string(&mut self.next_cursor),
                        activity_id: activity_id.clone(),
                        transcript_cursor: next
                            .transcript_cursors
                            .get(activity_id)
                            .cloned()
                            .unwrap_or(None),
                    });
            }
        }

        if self.pending.is_empty() {
            self.pending.push_back(ActivityConsoleEvent::Heartbeat {
                cursor: cursor_string(&mut self.next_cursor),
            });
        }
        self.previous = next;
    }
}

fn cursor_string(next_cursor: &mut u64) -> String {
    let cursor = format!("{:016}", *next_cursor);
    *next_cursor = next_cursor.saturating_add(1);
    cursor
}

fn normalize_recent_activity_limit(limit: Option<usize>) -> Option<usize> {
    limit
        .filter(|limit| *limit > 0)
        .map(|limit| limit.min(MAX_RECENT_ACTIVITY_LIMIT))
}

fn activity_error(status: StatusCode, message: &str) -> Response {
    (
        status,
        Json(WorkRootActivityError {
            error: message.to_owned(),
        }),
    )
        .into_response()
}

fn activity_access_error(error: WorkRootAccessError) -> Response {
    activity_error(error.status(), error.message())
}

/// Resolve the wsstate named-agent directory for an opened workRoot under an
/// explicit cache home (the `WS_CACHE_HOME` cache root).
///
/// This mirrors `agents-plugin-tool/internal/wsstate` layout derivation: the
/// canonical Git worktree root and common root select a worktree key, and
/// agents live at `<cacheHome>/proj/<worktreeKey>/agents`. Returns `None` when
/// the `git` binary is unavailable or the workRoot is not a resolvable
/// non-bare Git worktree; all of those collapse to the same "no wsstate
/// layout, no agents" empty projection. Exposed so daemon route tests can seed
/// fixture cache trees at the same location the projector reads.
pub fn resolve_work_root_agents_dir(cache_home: &Path, root_path: &Path) -> Option<PathBuf> {
    let identity = git_identity(root_path)?;
    let project_key = short_hash(&canonical_path_bytes(&identity.common_root));
    let worktree_key = if identity.worktree_root == identity.common_root {
        project_key
    } else {
        let worktree_id = short_hash(&canonical_path_bytes(&identity.worktree_root));
        format!("{project_key}@{worktree_id}")
    };
    Some(cache_home.join("proj").join(worktree_key).join("agents"))
}

fn project_blocking(
    work_root_id: WorkRootId,
    root_path: &Path,
    cache_home: Option<&Path>,
    codex_home: Option<&Path>,
    recent_limit: Option<usize>,
) -> WorkRootActivityView {
    let projections = resolve_cache_root(cache_home)
        .and_then(|cache_root| resolve_work_root_agents_dir(&cache_root, root_path))
        .map(|agents_dir| scan_named_agents(&agents_dir, codex_home, recent_limit))
        .unwrap_or_default();

    let agents = projections
        .iter()
        .map(|projection| projection.row.clone())
        .collect::<Vec<_>>();
    let summary = summarize(&agents);
    let degraded = agents.iter().any(|agent| !agent.diagnostics.is_empty());
    let mut items = projections
        .iter()
        .map(named_agent_activity_item)
        .collect::<Vec<_>>();
    items.sort_by(activity_item_ordering);

    let selected_item_id = items.first().map(|item| item.id.clone());
    let feed_cursor = Some(feed_cursor(&items));

    ActivityFeed {
        work_root_id,
        status: if degraded { "degraded" } else { "ok" }.to_owned(),
        update_mode: "snapshot".to_owned(),
        feed_cursor,
        selected_item_id,
        summary,
        items,
        agents,
    }
}

fn watch_snapshot_blocking(
    work_root_id: WorkRootId,
    root_path: &Path,
    cache_home: Option<&Path>,
    codex_home: Option<&Path>,
) -> ActivityWatchSnapshot {
    let view = project_blocking(work_root_id, root_path, cache_home, codex_home, None);
    let item_versions = activity_item_versions(root_path, cache_home, codex_home);
    let mut items = BTreeMap::new();
    let mut transcript_cursors = BTreeMap::new();
    for item in view.items {
        transcript_cursors.insert(item.id.clone(), item.transcript.cursor.clone());
        items.insert(item.id.clone(), item);
    }

    ActivityWatchSnapshot {
        items,
        item_versions,
        transcript_cursors,
    }
}

fn activity_item_versions(
    root_path: &Path,
    cache_home: Option<&Path>,
    codex_home: Option<&Path>,
) -> BTreeMap<String, String> {
    let Some(agents_dir) = resolve_cache_root(cache_home)
        .and_then(|cache_root| resolve_work_root_agents_dir(&cache_root, root_path))
    else {
        return BTreeMap::new();
    };
    let Ok(entries) = std::fs::read_dir(agents_dir) else {
        return BTreeMap::new();
    };

    let mut versions = BTreeMap::new();
    for entry in entries.flatten() {
        let Ok(file_type) = entry.file_type() else {
            continue;
        };
        if !file_type.is_dir() {
            continue;
        }
        let agent_key = entry.file_name().to_string_lossy().into_owned();
        if agent_key.is_empty() {
            continue;
        }
        versions.insert(
            named_agent_activity_id(&agent_key),
            system_time_version(agent_record_modified_at(&entry.path(), codex_home)),
        );
    }
    versions
}

fn system_time_version(value: SystemTime) -> String {
    let duration = value.duration_since(UNIX_EPOCH).unwrap_or_default();
    format!("{}.{:09}", duration.as_secs(), duration.subsec_nanos())
}

fn feed_cursor(items: &[ActivityItem]) -> String {
    let latest = items.iter().map(recent_value).max().unwrap_or("");
    format!("snapshot:{}:{latest}", items.len())
}

fn summarize(agents: &[NamedAgentActivityView]) -> WorkRootActivitySummary {
    let mut summary = WorkRootActivitySummary {
        total: agents.len(),
        ..WorkRootActivitySummary::default()
    };
    for agent in agents {
        match agent.status.as_str() {
            "running" => summary.active += 1,
            "blocked" => summary.blocked += 1,
            "failed" => summary.failed += 1,
            STATUS_UNAVAILABLE => summary.unavailable += 1,
            _ => {}
        }
    }
    summary
}

/// Scan `<worktree>/agents` for `agents/*/agent.json` plus optional
/// `current/state.json`, mapping each directory into a bounded row.
fn scan_named_agents(
    agents_dir: &Path,
    codex_home: Option<&Path>,
    recent_limit: Option<usize>,
) -> Vec<NamedAgentProjection> {
    let Ok(entries) = std::fs::read_dir(agents_dir) else {
        // No agents directory yet (or unreadable): an empty, healthy
        // projection rather than a route failure.
        return Vec::new();
    };

    let mut agent_dirs = Vec::new();
    for entry in entries.flatten() {
        let Ok(file_type) = entry.file_type() else {
            continue;
        };
        if !file_type.is_dir() {
            continue;
        }
        let agent_key = entry.file_name().to_string_lossy().into_owned();
        if agent_key.is_empty() {
            continue;
        }
        let agent_dir = entry.path();
        agent_dirs.push(RecentAgentDir {
            modified_at: agent_record_modified_at(&agent_dir, codex_home),
            agent_key,
            agent_dir,
        });
    }

    if let Some(limit) = recent_limit {
        // CONTRACT: hot-path refreshes can ask for only the recently changed
        // rows. Use portable filesystem modification times from the agent dir
        // plus key child files instead of platform-specific watchers here.
        agent_dirs.sort_by(|left, right| {
            right
                .modified_at
                .cmp(&left.modified_at)
                .then_with(|| left.agent_key.cmp(&right.agent_key))
        });
        agent_dirs.truncate(limit);
    }

    let mut rows = agent_dirs
        .into_iter()
        .map(|entry| named_agent_projection(&entry.agent_dir, entry.agent_key, codex_home))
        .collect::<Vec<_>>();
    rows.sort_by(|left, right| left.row.agent_id.cmp(&right.row.agent_id));
    rows
}

struct RecentAgentDir {
    agent_key: String,
    agent_dir: PathBuf,
    modified_at: SystemTime,
}

#[derive(Clone, Debug)]
struct NamedAgentProjection {
    row: NamedAgentActivityView,
    output_available: bool,
    native_transcript_available: bool,
}

fn agent_record_modified_at(agent_dir: &Path, codex_home: Option<&Path>) -> SystemTime {
    let mut latest = modified_at(agent_dir);
    for relative in [
        "agent.json",
        "output.md",
        "current/state.json",
        "current/runtime.jsonl",
        "current/stdout",
        "current/stderr",
    ] {
        let candidate = modified_at(&agent_dir.join(relative));
        if candidate > latest {
            latest = candidate;
        }
    }
    if let Some(metadata) = read_agent_metadata(agent_dir).ok() {
        if let Some(path) = resolve_codex_session_file(codex_home, &metadata) {
            let candidate = modified_at(&path);
            if candidate > latest {
                latest = candidate;
            }
        }
    }
    latest
}

fn modified_at(path: &Path) -> SystemTime {
    std::fs::metadata(path)
        .and_then(|metadata| metadata.modified())
        .unwrap_or(UNIX_EPOCH)
}

fn named_agent_projection(
    agent_dir: &Path,
    agent_key: String,
    codex_home: Option<&Path>,
) -> NamedAgentProjection {
    let output_available = agent_dir.join("output.md").is_file();
    let metadata = read_agent_metadata(agent_dir);

    let metadata = match metadata {
        Ok(metadata) => metadata,
        Err(diagnostic) => {
            // CONTRACT: a malformed or missing record degrades only its own row
            // instead of failing the whole route.
            return NamedAgentProjection {
                row: NamedAgentActivityView {
                    agent_id: agent_key,
                    name: None,
                    backend: None,
                    harness: None,
                    tier: None,
                    model: None,
                    effort: None,
                    status: STATUS_UNAVAILABLE.to_owned(),
                    last_call_at: None,
                    session_present: false,
                    current_call: None,
                    detail_hints: Vec::new(),
                    diagnostics: vec![diagnostic.to_owned()],
                },
                output_available: false,
                native_transcript_available: false,
            };
        }
    };

    let native_transcript_available = resolve_codex_session_file(codex_home, &metadata).is_some();

    let mut diagnostics = Vec::new();
    let (status, status_diagnostic) = agent_status(&metadata.status);
    if let Some(diagnostic) = status_diagnostic {
        diagnostics.push(diagnostic.to_owned());
    }

    let (current_call, current_call_diagnostic) = read_current_call(agent_dir);
    if let Some(diagnostic) = current_call_diagnostic {
        diagnostics.push(diagnostic.to_owned());
    }

    let mut detail_hints = Vec::new();
    if !metadata.last_output_path.is_empty() {
        // CONTRACT: surface that output exists without leaking the cache path.
        detail_hints.push("recent output available".to_owned());
    }

    NamedAgentProjection {
        row: NamedAgentActivityView {
            agent_id: agent_key,
            name: non_empty(metadata.name),
            backend: non_empty(metadata.backend),
            harness: non_empty(metadata.harness),
            tier: non_empty(metadata.tier),
            model: non_empty(metadata.model),
            effort: non_empty(metadata.effort),
            status,
            last_call_at: non_empty(metadata.last_call_at),
            // CONTRACT: collapse the private session id into a presence flag.
            session_present: !metadata.session_id.is_empty(),
            current_call,
            detail_hints,
            diagnostics,
        },
        output_available,
        native_transcript_available,
    }
}

fn read_agent_metadata(agent_dir: &Path) -> Result<AgentMetadata, &'static str> {
    match std::fs::read(agent_dir.join("agent.json")) {
        Ok(raw) => {
            serde_json::from_slice::<AgentMetadata>(&raw).map_err(|_| DIAG_METADATA_UNREADABLE)
        }
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Err(DIAG_METADATA_MISSING),
        Err(_) => Err(DIAG_METADATA_UNREADABLE),
    }
}

fn named_agent_activity_item(projection: &NamedAgentProjection) -> ActivityItem {
    let agent = &projection.row;
    let source = named_agent_source(agent);
    let live = agent
        .current_call
        .as_ref()
        .map(|call| call.active)
        .unwrap_or(agent.status == "running");
    let attention = !agent.diagnostics.is_empty()
        || matches!(
            agent.status.as_str(),
            "blocked" | "failed" | STATUS_UNAVAILABLE
        )
        || agent
            .current_call
            .as_ref()
            .and_then(|call| call.error.as_ref())
            .is_some();
    let (started_at, updated_at, finished_at) = activity_item_timing(agent);
    let mut metadata = BTreeMap::new();
    metadata.insert(
        "agentId".to_owned(),
        serde_json::Value::String(agent.agent_id.clone()),
    );
    if agent.session_present {
        metadata.insert("sessionPresent".to_owned(), serde_json::Value::Bool(true));
    }
    if let Some(effort) = &agent.effort {
        metadata.insert(
            "effort".to_owned(),
            serde_json::Value::String(effort.clone()),
        );
    }

    ActivityItem {
        id: named_agent_activity_id(&agent.agent_id),
        kind: ACTIVITY_KIND_NAMED_AGENT.to_owned(),
        label: agent.name.clone().unwrap_or_else(|| agent.agent_id.clone()),
        status: agent.status.clone(),
        live,
        attention,
        started_at,
        updated_at,
        finished_at,
        source,
        transcript: ActivityTranscriptAvailability {
            status: if projection.output_available || projection.native_transcript_available {
                "available"
            } else if agent.status == STATUS_UNAVAILABLE {
                STATUS_UNAVAILABLE
            } else {
                "empty"
            }
            .to_owned(),
            available: projection.output_available || projection.native_transcript_available,
            cursor: (projection.output_available || projection.native_transcript_available)
                .then(|| "0".to_owned()),
        },
        diagnostics: agent.diagnostics.clone(),
        metadata,
    }
}

fn named_agent_source(agent: &NamedAgentActivityView) -> ActivitySourceDisplay {
    ActivitySourceDisplay {
        kind: ACTIVITY_KIND_NAMED_AGENT.to_owned(),
        label: agent.name.clone().unwrap_or_else(|| agent.agent_id.clone()),
        backend: agent.backend.clone(),
        harness: agent.harness.clone(),
        tier: agent.tier.clone(),
        model: agent.model.clone(),
    }
}

fn activity_item_timing(
    agent: &NamedAgentActivityView,
) -> (Option<String>, Option<String>, Option<String>) {
    if let Some(call) = &agent.current_call {
        return (
            call.started_at
                .clone()
                .or_else(|| agent.last_call_at.clone()),
            call.updated_at
                .clone()
                .or_else(|| agent.last_call_at.clone()),
            call.finished_at.clone(),
        );
    }
    (agent.last_call_at.clone(), agent.last_call_at.clone(), None)
}

fn activity_item_ordering(left: &ActivityItem, right: &ActivityItem) -> std::cmp::Ordering {
    activity_item_rank(left)
        .cmp(&activity_item_rank(right))
        .then_with(|| {
            recent_value(right)
                .cmp(&recent_value(left))
                .then_with(|| left.label.cmp(&right.label))
                .then_with(|| left.id.cmp(&right.id))
        })
}

fn activity_item_rank(item: &ActivityItem) -> u8 {
    if item.live {
        0
    } else if item.attention {
        1
    } else if matches!(
        item.status.as_str(),
        "blocked" | "failed" | STATUS_UNAVAILABLE
    ) {
        2
    } else {
        3
    }
}

fn recent_value(item: &ActivityItem) -> &str {
    item.updated_at
        .as_deref()
        .or(item.started_at.as_deref())
        .or(item.finished_at.as_deref())
        .unwrap_or("")
}

fn named_agent_activity_id(agent_key: &str) -> String {
    format!("{ACTIVITY_ID_NAMED_AGENT_PREFIX}{agent_key}")
}

fn named_agent_key_from_activity_id(activity_id: &str) -> Option<String> {
    activity_id
        .strip_prefix(ACTIVITY_ID_NAMED_AGENT_PREFIX)
        .filter(|agent_key| {
            !agent_key.is_empty() && !agent_key.contains('/') && !agent_key.contains('\\')
        })
        .map(str::to_owned)
}

fn normalize_transcript_limit(limit: Option<usize>) -> usize {
    limit
        .filter(|limit| *limit > 0)
        .unwrap_or(DEFAULT_TRANSCRIPT_LIMIT)
        .min(MAX_TRANSCRIPT_LIMIT)
}

fn transcript_cursor_offset(cursor: Option<&str>) -> usize {
    cursor
        .and_then(|cursor| cursor.parse::<usize>().ok())
        .unwrap_or(0)
}

fn paginate_transcript_blocks(
    all_blocks: Vec<TranscriptBlock>,
    cursor: Option<&str>,
    before: Option<&str>,
    limit: usize,
) -> (Vec<TranscriptBlock>, String, bool) {
    if let Some(before) = before {
        let end = transcript_cursor_offset(Some(before)).min(all_blocks.len());
        let start = end.saturating_sub(limit);
        return (
            all_blocks[start..end].to_vec(),
            start.to_string(),
            start > 0,
        );
    }
    if cursor.is_none() {
        let end = all_blocks.len();
        let start = end.saturating_sub(limit);
        return (
            all_blocks[start..end].to_vec(),
            start.to_string(),
            start > 0,
        );
    }
    let start = transcript_cursor_offset(cursor).min(all_blocks.len());
    let end = (start + limit).min(all_blocks.len());
    (
        all_blocks[start..end].to_vec(),
        end.to_string(),
        end < all_blocks.len(),
    )
}

fn named_agent_transcript_blocking(
    work_root_id: WorkRootId,
    root_path: &Path,
    cache_home: Option<&Path>,
    codex_home: Option<&Path>,
    activity_id: String,
    agent_key: &str,
    cursor: Option<&str>,
    before: Option<&str>,
    limit: usize,
) -> ActivityTranscript {
    let agents_dir = resolve_cache_root(cache_home)
        .and_then(|cache_root| resolve_work_root_agents_dir(&cache_root, root_path));
    let Some(agent_dir) = agents_dir.map(|agents_dir| agents_dir.join(agent_key)) else {
        return unavailable_transcript(
            work_root_id,
            activity_id,
            agent_key,
            "activity source unavailable",
        );
    };
    if !agent_dir.is_dir() {
        return unavailable_transcript(
            work_root_id,
            activity_id,
            agent_key,
            "activity source unavailable",
        );
    }

    let projection = named_agent_projection(&agent_dir, agent_key.to_owned(), codex_home);
    let source = named_agent_source(&projection.row);
    let live = projection
        .row
        .current_call
        .as_ref()
        .map(|call| call.active)
        .unwrap_or(false);

    let metadata = read_agent_metadata(&agent_dir).ok();
    let mut native_diagnostic: Option<&str> = None;
    if let Some(metadata) = metadata.as_ref() {
        if let Some(native_path) = resolve_codex_session_file(codex_home, metadata) {
            match std::fs::read_to_string(native_path) {
                Ok(raw) => {
                    let parsed = parse_codex_session_transcript(&raw);
                    let (blocks, next_cursor, has_more) =
                        paginate_transcript_blocks(parsed.blocks, cursor, before, limit);
                    let mut diagnostics = projection.row.diagnostics;
                    diagnostics.extend(parsed.diagnostics);
                    let degraded = !diagnostics.is_empty() || parsed.degraded;
                    return ActivityTranscript {
                        work_root_id,
                        activity_id,
                        status: if degraded { "degraded" } else { "available" }.to_owned(),
                        source_status: if degraded { "degraded" } else { "ok" }.to_owned(),
                        live,
                        source,
                        blocks,
                        next_cursor: Some(next_cursor),
                        has_more,
                        diagnostics,
                    };
                }
                Err(_) => {
                    // Fall through to the output.md source while reporting only
                    // a source-neutral diagnostic. Native paths stay daemon-side.
                    native_diagnostic = Some("native transcript source unreadable");
                }
            }
        }
    }

    let output_path = agent_dir.join("output.md");
    let raw = match std::fs::read_to_string(&output_path) {
        Ok(raw) => raw,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return ActivityTranscript {
                work_root_id,
                activity_id,
                status: if projection.row.status == STATUS_UNAVAILABLE {
                    STATUS_UNAVAILABLE.to_owned()
                } else {
                    "empty".to_owned()
                },
                source_status: if projection.row.status == STATUS_UNAVAILABLE {
                    "degraded".to_owned()
                } else {
                    "missing".to_owned()
                },
                live,
                source,
                blocks: Vec::new(),
                next_cursor: Some("0".to_owned()),
                has_more: false,
                diagnostics: projection.row.diagnostics,
            };
        }
        Err(_) => {
            let mut diagnostics = projection.row.diagnostics;
            if let Some(diagnostic) = native_diagnostic {
                diagnostics.push(diagnostic.to_owned());
            }
            diagnostics.push("transcript source unreadable".to_owned());
            return ActivityTranscript {
                work_root_id,
                activity_id,
                status: "degraded".to_owned(),
                source_status: "degraded".to_owned(),
                live,
                source,
                blocks: Vec::new(),
                next_cursor: Some("0".to_owned()),
                has_more: false,
                diagnostics,
            };
        }
    };

    let all_blocks = transcript_blocks_from_output(&raw);
    let (blocks, next_cursor, has_more) =
        paginate_transcript_blocks(all_blocks, cursor, before, limit);
    let mut diagnostics = projection.row.diagnostics;
    if let Some(diagnostic) = native_diagnostic {
        diagnostics.push(diagnostic.to_owned());
    }
    let degraded = !diagnostics.is_empty();
    ActivityTranscript {
        work_root_id,
        activity_id,
        status: if degraded { "degraded" } else { "available" }.to_owned(),
        source_status: if degraded { "degraded" } else { "ok" }.to_owned(),
        live,
        source,
        blocks,
        next_cursor: Some(next_cursor),
        has_more,
        diagnostics,
    }
}

#[derive(Debug, Default)]
struct CodexSessionParse {
    blocks: Vec<TranscriptBlock>,
    diagnostics: Vec<String>,
    degraded: bool,
}

#[derive(Debug, Deserialize)]
struct CodexSessionEnvelope {
    #[serde(default)]
    timestamp: Option<String>,
    #[serde(default, rename = "type")]
    event_type: String,
    #[serde(default)]
    payload: serde_json::Value,
}

enum CodexSessionRecord {
    Block(TranscriptBlock),
    Skip,
    Unsupported(TranscriptBlock),
}

fn parse_codex_session_transcript(raw: &str) -> CodexSessionParse {
    let mut parsed = CodexSessionParse::default();
    for (line_index, line) in raw.lines().enumerate() {
        if line.trim().is_empty() {
            continue;
        }
        let cursor = parsed.blocks.len().to_string();
        let envelope = match serde_json::from_str::<CodexSessionEnvelope>(line) {
            Ok(envelope) => envelope,
            Err(_) => {
                parsed.degraded = true;
                parsed
                    .diagnostics
                    .push("native transcript record malformed".to_owned());
                parsed.blocks.push(TranscriptBlock {
                    cursor,
                    timestamp: None,
                    render_kind: "status".to_owned(),
                    title: Some("Malformed transcript record".to_owned()),
                    text: Some(format!(
                        "Skipped malformed native transcript record {}",
                        line_index + 1
                    )),
                    data: None,
                    degraded: true,
                });
                continue;
            }
        };

        match codex_session_record(&envelope, cursor.clone()) {
            CodexSessionRecord::Block(block) => {
                if !is_duplicate_codex_dialogue_block(&parsed.blocks, &block) {
                    parsed.blocks.push(block);
                }
            }
            CodexSessionRecord::Skip => {}
            CodexSessionRecord::Unsupported(block) => {
                parsed.degraded = true;
                parsed.blocks.push(block);
            }
        }
    }
    parsed
}

fn codex_session_record(envelope: &CodexSessionEnvelope, cursor: String) -> CodexSessionRecord {
    let Some(payload_type) = envelope
        .payload
        .get("type")
        .and_then(|value| value.as_str())
    else {
        return match envelope.event_type.as_str() {
            "session_meta" | "turn_context" | "compacted" => CodexSessionRecord::Skip,
            _ => CodexSessionRecord::Unsupported(unsupported_codex_session_block(
                envelope, cursor, None,
            )),
        };
    };

    match (envelope.event_type.as_str(), payload_type) {
        ("event_msg", "token_count") | ("response_item", "reasoning") => CodexSessionRecord::Skip,
        ("event_msg", "task_started") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "status".to_owned(),
            title: Some("Task started".to_owned()),
            text: Some("Agent turn started".to_owned()),
            data: None,
            degraded: false,
        }),
        ("event_msg", "task_complete") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "status".to_owned(),
            title: Some("Task complete".to_owned()),
            text: Some("Agent turn completed".to_owned()),
            data: None,
            degraded: false,
        }),
        ("event_msg", "agent_message") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "assistant".to_owned(),
            title: Some("Assistant".to_owned()),
            text: envelope
                .payload
                .get("message")
                .and_then(|value| value.as_str())
                .map(bounded_native_text),
            data: None,
            degraded: false,
        }),
        ("event_msg", "user_message") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "user".to_owned(),
            title: Some("User".to_owned()),
            text: codex_user_message_text(&envelope.payload),
            data: None,
            degraded: false,
        }),
        ("response_item", "message") => {
            let role = envelope
                .payload
                .get("role")
                .and_then(|value| value.as_str())
                .unwrap_or("message");
            let (render_kind, title) = match role {
                "assistant" => ("assistant", "Assistant"),
                "user" => ("user", "User"),
                _ => ("text", "Message"),
            };
            CodexSessionRecord::Block(TranscriptBlock {
                cursor,
                timestamp: envelope.timestamp.clone(),
                render_kind: render_kind.to_owned(),
                title: Some(title.to_owned()),
                text: codex_message_content_text(envelope.payload.get("content")),
                data: None,
                degraded: false,
            })
        }
        ("response_item", "function_call") => {
            let name = envelope
                .payload
                .get("name")
                .and_then(|value| value.as_str())
                .unwrap_or("tool");
            let arguments_bytes = envelope
                .payload
                .get("arguments")
                .map(|value| value.to_string().len())
                .unwrap_or(0);
            CodexSessionRecord::Block(TranscriptBlock {
                cursor,
                timestamp: envelope.timestamp.clone(),
                render_kind: "toolCall".to_owned(),
                title: Some("Tool call".to_owned()),
                text: Some(bounded(&format!("Called {name}"))),
                data: Some(serde_json::json!({
                    "name": bounded(name),
                    "argumentsBytes": arguments_bytes,
                })),
                degraded: false,
            })
        }
        ("response_item", "custom_tool_call") => {
            let name = envelope
                .payload
                .get("name")
                .and_then(|value| value.as_str())
                .unwrap_or("tool");
            let input_bytes = envelope
                .payload
                .get("input")
                .map(|value| value.to_string().len())
                .unwrap_or(0);
            CodexSessionRecord::Block(TranscriptBlock {
                cursor,
                timestamp: envelope.timestamp.clone(),
                render_kind: "toolCall".to_owned(),
                title: Some("Tool call".to_owned()),
                text: Some(bounded(&format!("Called {name}"))),
                data: Some(serde_json::json!({
                    "name": bounded(name),
                    "argumentsBytes": input_bytes,
                    "inputBytes": input_bytes,
                })),
                degraded: false,
            })
        }
        ("response_item", "function_call_output") => {
            let output = codex_tool_output_snippet(envelope.payload.get("output"));
            CodexSessionRecord::Block(TranscriptBlock {
                cursor,
                timestamp: envelope.timestamp.clone(),
                render_kind: "toolResult".to_owned(),
                title: Some("Tool output".to_owned()),
                text: Some(output.text),
                data: Some(serde_json::json!({
                    "outputBytes": output.output_bytes,
                    "lineCount": output.line_count,
                    "omittedMiddleLines": output.omitted_middle_lines,
                })),
                degraded: false,
            })
        }
        ("response_item", "custom_tool_call_output") => {
            let output = codex_tool_output_snippet(envelope.payload.get("output"));
            CodexSessionRecord::Block(TranscriptBlock {
                cursor,
                timestamp: envelope.timestamp.clone(),
                render_kind: "toolResult".to_owned(),
                title: Some("Tool output".to_owned()),
                text: Some(output.text),
                data: Some(serde_json::json!({
                    "outputBytes": output.output_bytes,
                    "lineCount": output.line_count,
                    "omittedMiddleLines": output.omitted_middle_lines,
                })),
                degraded: false,
            })
        }
        ("event_msg", "mcp_tool_call_end") => CodexSessionRecord::Block(tool_result_event_block(
            envelope,
            cursor,
            "MCP tool result",
            "MCP tool call completed",
            None,
        )),
        ("event_msg", "exec_command_end") => {
            let exit_code = envelope
                .payload
                .get("exit_code")
                .and_then(|value| value.as_i64());
            CodexSessionRecord::Block(tool_result_event_block(
                envelope,
                cursor,
                "Command result",
                "Command completed",
                exit_code,
            ))
        }
        ("event_msg", "patch_apply_end") => {
            let change_count = envelope
                .payload
                .get("changes")
                .and_then(|value| value.as_array())
                .map(|items| items.len())
                .unwrap_or(0);
            let mut data = codex_event_result_data(envelope, None);
            data["changes"] = serde_json::json!(change_count);
            CodexSessionRecord::Block(TranscriptBlock {
                cursor,
                timestamp: envelope.timestamp.clone(),
                render_kind: "toolResult".to_owned(),
                title: Some("Patch apply".to_owned()),
                text: Some("Patch apply completed".to_owned()),
                data: Some(data),
                degraded: false,
            })
        }
        ("event_msg", "turn_aborted") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "status".to_owned(),
            title: Some("Turn aborted".to_owned()),
            text: Some("Agent turn aborted".to_owned()),
            data: Some(serde_json::json!({
                "reason": envelope
                    .payload
                    .get("reason")
                    .and_then(|value| value.as_str())
                    .map(safe_codex_type_label)
                    .unwrap_or_else(|| "unspecified".to_owned()),
            })),
            degraded: false,
        }),
        ("event_msg", "thread_rolled_back") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "status".to_owned(),
            title: Some("Thread rolled back".to_owned()),
            text: Some("Conversation history rolled back".to_owned()),
            data: Some(serde_json::json!({
                "numTurns": envelope
                    .payload
                    .get("num_turns")
                    .and_then(|value| value.as_u64()),
            })),
            degraded: false,
        }),
        ("event_msg", "context_compacted") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "status".to_owned(),
            title: Some("Context compacted".to_owned()),
            text: Some("Conversation context compacted".to_owned()),
            data: None,
            degraded: false,
        }),
        ("event_msg", "thread_goal_updated") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "status".to_owned(),
            title: Some("Goal updated".to_owned()),
            text: Some("Thread goal updated".to_owned()),
            data: None,
            degraded: false,
        }),
        ("event_msg", "collab_agent_spawn_end") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "status".to_owned(),
            title: Some("Agent handoff".to_owned()),
            text: Some("Sub-agent handoff completed".to_owned()),
            data: Some(serde_json::json!({
                "status": envelope
                    .payload
                    .get("status")
                    .and_then(|value| value.as_str())
                    .map(safe_codex_type_label)
                    .unwrap_or_else(|| "unknown".to_owned()),
            })),
            degraded: false,
        }),
        ("event_msg", "collab_waiting_end") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "status".to_owned(),
            title: Some("Agent wait".to_owned()),
            text: Some("Sub-agent wait completed".to_owned()),
            data: None,
            degraded: false,
        }),
        ("event_msg", "collab_close_end") => CodexSessionRecord::Block(TranscriptBlock {
            cursor,
            timestamp: envelope.timestamp.clone(),
            render_kind: "status".to_owned(),
            title: Some("Agent close".to_owned()),
            text: Some("Sub-agent close completed".to_owned()),
            data: None,
            degraded: false,
        }),
        _ => CodexSessionRecord::Unsupported(unsupported_codex_session_block(
            envelope,
            cursor,
            Some(payload_type),
        )),
    }
}

fn is_duplicate_codex_dialogue_block(blocks: &[TranscriptBlock], block: &TranscriptBlock) -> bool {
    if !matches!(block.render_kind.as_str(), "assistant" | "user") {
        return false;
    }
    let Some(previous) = blocks.last() else {
        return false;
    };
    previous.render_kind == block.render_kind
        && previous.title == block.title
        && previous.text == block.text
}

fn codex_user_message_text(payload: &serde_json::Value) -> Option<String> {
    if let Some(message) = payload.get("message").and_then(|value| value.as_str()) {
        return Some(bounded_native_text(message));
    }
    codex_message_content_text(payload.get("text_elements"))
}

fn codex_message_content_text(value: Option<&serde_json::Value>) -> Option<String> {
    let value = value?;
    let mut parts = Vec::new();
    collect_codex_message_text(value, &mut parts);
    if parts.is_empty() {
        None
    } else {
        Some(bounded_native_text(&parts.join("\n")))
    }
}

struct CodexToolOutputSnippet {
    text: String,
    output_bytes: usize,
    line_count: usize,
    omitted_middle_lines: usize,
}

fn codex_tool_output_snippet(value: Option<&serde_json::Value>) -> CodexToolOutputSnippet {
    let raw = match value {
        Some(serde_json::Value::String(text)) => text.clone(),
        Some(value) => value.to_string(),
        None => String::new(),
    };
    let output_bytes = raw.len();
    if raw.is_empty() {
        return CodexToolOutputSnippet {
            text: "Tool output empty".to_owned(),
            output_bytes,
            line_count: 0,
            omitted_middle_lines: 0,
        };
    }

    let lines: Vec<&str> = raw.lines().collect();
    let line_count = lines.len();
    let inline_line_budget = TOOL_OUTPUT_HEAD_LINES + TOOL_OUTPUT_TAIL_LINES;
    let (text, omitted_middle_lines) = if line_count > inline_line_budget {
        let omitted = line_count - inline_line_budget;
        let mut selected = Vec::with_capacity(inline_line_budget + 1);
        selected.extend_from_slice(&lines[..TOOL_OUTPUT_HEAD_LINES]);
        selected.push("... omitted middle lines ...");
        selected.extend_from_slice(&lines[line_count - TOOL_OUTPUT_TAIL_LINES..]);
        (selected.join("\n"), omitted)
    } else {
        (raw, 0)
    };

    CodexToolOutputSnippet {
        text: bounded_native_text(&text),
        output_bytes,
        line_count,
        omitted_middle_lines,
    }
}

fn collect_codex_message_text(value: &serde_json::Value, parts: &mut Vec<String>) {
    match value {
        serde_json::Value::String(text) => parts.push(text.clone()),
        serde_json::Value::Array(items) => {
            for item in items {
                collect_codex_message_text(item, parts);
            }
        }
        serde_json::Value::Object(map) => {
            for key in ["text", "content"] {
                if let Some(text) = map.get(key).and_then(|value| value.as_str()) {
                    parts.push(text.to_owned());
                    return;
                }
            }
        }
        _ => {}
    }
}

fn tool_result_event_block(
    envelope: &CodexSessionEnvelope,
    cursor: String,
    title: &str,
    text: &str,
    exit_code: Option<i64>,
) -> TranscriptBlock {
    TranscriptBlock {
        cursor,
        timestamp: envelope.timestamp.clone(),
        render_kind: "toolResult".to_owned(),
        title: Some(title.to_owned()),
        text: Some(text.to_owned()),
        data: Some(codex_event_result_data(envelope, exit_code)),
        degraded: false,
    }
}

fn codex_event_result_data(
    envelope: &CodexSessionEnvelope,
    exit_code: Option<i64>,
) -> serde_json::Value {
    let status = envelope
        .payload
        .get("status")
        .and_then(|value| value.as_str())
        .map(safe_codex_type_label);
    let success = envelope
        .payload
        .get("success")
        .and_then(|value| value.as_bool());
    let outcome = match (success, status.as_deref()) {
        (Some(true), _) => "success".to_owned(),
        (Some(false), _) => "failed".to_owned(),
        (_, Some(status)) => status.to_owned(),
        _ => "completed".to_owned(),
    };
    let output_bytes = [
        "stdout",
        "stderr",
        "formatted_output",
        "aggregated_output",
        "result",
    ]
    .iter()
    .filter_map(|key| envelope.payload.get(*key))
    .map(|value| value.to_string().len())
    .sum::<usize>();
    let duration_ms = envelope
        .payload
        .get("duration")
        .or_else(|| envelope.payload.get("duration_ms"))
        .and_then(|value| value.as_u64());
    serde_json::json!({
        "status": status.unwrap_or_else(|| "unknown".to_owned()),
        "outcome": outcome,
        "exitCode": exit_code,
        "durationMs": duration_ms,
        "outputBytes": output_bytes,
    })
}

fn unsupported_codex_session_block(
    envelope: &CodexSessionEnvelope,
    cursor: String,
    payload_type: Option<&str>,
) -> TranscriptBlock {
    TranscriptBlock {
        cursor,
        timestamp: envelope.timestamp.clone(),
        render_kind: "status".to_owned(),
        title: Some("Unsupported transcript record".to_owned()),
        text: Some("Skipped unsupported native transcript record".to_owned()),
        data: Some(serde_json::json!({
            "eventType": safe_codex_type_label(&envelope.event_type),
            "payloadType": payload_type
                .map(safe_codex_type_label)
                .unwrap_or_else(|| "none".to_owned()),
            "payloadFieldCount": envelope
                .payload
                .as_object()
                .map(|value| value.len())
                .unwrap_or(0),
            "omissionReason": "unsupported codex native shape",
        })),
        degraded: true,
    }
}

fn safe_codex_type_label(value: &str) -> String {
    if !value.is_empty()
        && value.len() <= 64
        && value
            .chars()
            .all(|ch| ch.is_ascii_alphanumeric() || ch == '_' || ch == '-')
    {
        value.to_owned()
    } else {
        "private".to_owned()
    }
}

fn bounded_native_text(value: &str) -> String {
    let bounded = bounded_to_chars(value, MAX_NATIVE_MESSAGE_TEXT);
    let mut redacted = String::with_capacity(bounded.len());
    let mut token = String::new();
    for ch in bounded.chars() {
        if ch.is_whitespace() {
            push_native_text_token(&mut redacted, &mut token);
            redacted.push(ch);
        } else {
            token.push(ch);
        }
    }
    push_native_text_token(&mut redacted, &mut token);
    redacted
}

fn push_native_text_token(output: &mut String, token: &mut String) {
    if token.is_empty() {
        return;
    }
    if native_text_token_looks_private(token) {
        output.push_str("[redacted]");
    } else {
        output.push_str(token);
    }
    token.clear();
}

fn native_text_token_looks_private(token: &str) -> bool {
    token.contains('/')
        || token.contains('\\')
        || token.contains(".jsonl")
        || token.contains("session_id")
        || token
            .chars()
            .next()
            .zip(token.chars().nth(1))
            .zip(token.chars().nth(2))
            .map(|((first, second), third)| {
                first.is_ascii_alphabetic() && second == ':' && (third == '\\' || third == '/')
            })
            .unwrap_or(false)
}

fn unavailable_transcript(
    work_root_id: WorkRootId,
    activity_id: String,
    agent_key: &str,
    diagnostic: &str,
) -> ActivityTranscript {
    let fallback = NamedAgentActivityView {
        agent_id: agent_key.to_owned(),
        name: None,
        backend: None,
        harness: None,
        tier: None,
        model: None,
        effort: None,
        status: STATUS_UNAVAILABLE.to_owned(),
        last_call_at: None,
        session_present: false,
        current_call: None,
        detail_hints: Vec::new(),
        diagnostics: Vec::new(),
    };
    ActivityTranscript {
        work_root_id,
        activity_id,
        status: STATUS_UNAVAILABLE.to_owned(),
        source_status: "missing".to_owned(),
        live: false,
        source: named_agent_source(&fallback),
        blocks: Vec::new(),
        next_cursor: Some("0".to_owned()),
        has_more: false,
        diagnostics: vec![diagnostic.to_owned()],
    }
}

fn transcript_blocks_from_output(raw: &str) -> Vec<TranscriptBlock> {
    if raw.is_empty() {
        return Vec::new();
    }
    raw.lines()
        .enumerate()
        .map(|(index, line)| TranscriptBlock {
            cursor: index.to_string(),
            timestamp: None,
            render_kind: "markdown".to_owned(),
            title: if index == 0 {
                Some("Agent output".to_owned())
            } else {
                None
            },
            text: Some(bounded(line)),
            data: None,
            degraded: false,
        })
        .collect()
}

fn read_current_call(
    agent_dir: &Path,
) -> (Option<NamedAgentCallActivityView>, Option<&'static str>) {
    let raw = match std::fs::read(agent_dir.join("current").join("state.json")) {
        Ok(raw) => raw,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return (None, None),
        Err(_) => return (None, Some(DIAG_CURRENT_CALL_UNREADABLE)),
    };

    let Ok(state) = serde_json::from_slice::<CurrentCallState>(&raw) else {
        return (None, Some(DIAG_CURRENT_CALL_UNREADABLE));
    };

    let active = matches!(state.status.as_str(), "queued" | "running");
    let terminal = matches!(state.status.as_str(), "completed" | "failed" | "cancelled");

    (
        Some(NamedAgentCallActivityView {
            status: state.status,
            active,
            terminal,
            execution_id: non_empty(state.execution_id),
            started_at: non_empty(state.started_at),
            updated_at: non_empty(state.updated_at),
            finished_at: non_empty(state.finished_at),
            cleanup_needed: state.cleanup_needed,
            error: non_empty(state.error).map(|error| bounded(&error)),
        }),
        None,
    )
}

/// Map a wsagent agent status into a dashboard status plus an optional bounded
/// diagnostic for empty or unrecognized values.
fn agent_status(raw: &str) -> (String, Option<&'static str>) {
    match raw {
        "idle" | "running" | "blocked" | "failed" | "erased" => (raw.to_owned(), None),
        "" => (STATUS_UNAVAILABLE.to_owned(), Some(DIAG_STATUS_UNAVAILABLE)),
        _ => (
            STATUS_UNAVAILABLE.to_owned(),
            Some(DIAG_STATUS_UNRECOGNIZED),
        ),
    }
}

fn non_empty(value: String) -> Option<String> {
    (!value.is_empty()).then_some(value)
}

fn bounded(value: &str) -> String {
    bounded_to_chars(value, MAX_BOUNDED_TEXT)
}

fn bounded_to_chars(value: &str, max_chars: usize) -> String {
    if value.chars().count() <= max_chars {
        return value.to_owned();
    }
    value.chars().take(max_chars).collect()
}

/// Resolve the wsstate cache root, mirroring `wsstate.CacheRoot`: an explicit
/// daemon override, then `WS_CACHE_HOME`, then `~/.cache/ws@kang-sw-devenv`.
fn resolve_cache_root(cache_home: Option<&Path>) -> Option<PathBuf> {
    if let Some(cache_home) = cache_home {
        return Some(cache_home.to_path_buf());
    }
    if let Some(env) = std::env::var_os("WS_CACHE_HOME") {
        if !env.is_empty() {
            return Some(PathBuf::from(env));
        }
    }
    Some(home_dir()?.join(".cache").join("ws@kang-sw-devenv"))
}

fn home_dir() -> Option<PathBuf> {
    #[cfg(windows)]
    let key = "USERPROFILE";
    #[cfg(not(windows))]
    let key = "HOME";
    std::env::var_os(key)
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
}

fn resolve_codex_home(configured: Option<&Path>) -> Option<PathBuf> {
    if let Some(configured) = configured {
        return Some(configured.to_path_buf());
    }
    if let Some(env) = std::env::var_os("CODEX_HOME") {
        if !env.is_empty() {
            return Some(PathBuf::from(env));
        }
    }
    Some(home_dir()?.join(".codex"))
}

fn resolve_codex_session_file(
    configured_codex_home: Option<&Path>,
    metadata: &AgentMetadata,
) -> Option<PathBuf> {
    let backend = metadata.backend.to_ascii_lowercase();
    let harness = metadata.harness.to_ascii_lowercase();
    if backend != "codex" && harness != "codex" {
        return None;
    }
    let session_id = metadata.session_id.trim();
    if session_id.is_empty()
        || session_id.contains('/')
        || session_id.contains('\\')
        || session_id.contains('\0')
    {
        return None;
    }
    let sessions_dir = resolve_codex_home(configured_codex_home)?.join("sessions");
    find_codex_session_file(&sessions_dir, session_id)
}

fn find_codex_session_file(sessions_dir: &Path, session_id: &str) -> Option<PathBuf> {
    let mut pending = VecDeque::from([sessions_dir.to_path_buf()]);
    let suffix = format!("-{session_id}.jsonl");
    let mut visited_entries = 0usize;
    while let Some(dir) = pending.pop_front() {
        let Ok(entries) = std::fs::read_dir(dir) else {
            continue;
        };
        for entry in entries.flatten() {
            visited_entries = visited_entries.saturating_add(1);
            if visited_entries > MAX_CODEX_SESSION_SCAN_ENTRIES {
                return None;
            }
            let Ok(file_type) = entry.file_type() else {
                continue;
            };
            let path = entry.path();
            if file_type.is_dir() {
                pending.push_back(path);
                continue;
            }
            if !file_type.is_file() {
                continue;
            }
            let Some(name) = entry.file_name().to_str().map(str::to_owned) else {
                continue;
            };
            if name.starts_with("rollout-") && name.ends_with(&suffix) {
                return Some(path);
            }
        }
    }
    None
}

struct GitIdentity {
    worktree_root: PathBuf,
    common_root: PathBuf,
}

/// Discover the canonical Git worktree root and common root for `root_path`,
/// matching `wsstate.gitIdentity`. Returns `None` when the `git` binary is
/// unavailable, the path is not in a Git repository, or the repository is bare.
fn git_identity(root_path: &Path) -> Option<GitIdentity> {
    let toplevel = git_output(root_path, &["rev-parse", "--show-toplevel"])?;
    let common_git_dir = git_output(
        root_path,
        &["rev-parse", "--path-format=absolute", "--git-common-dir"],
    )?;

    let worktree_root = std::fs::canonicalize(&toplevel).ok()?;
    let common_git_dir = std::fs::canonicalize(&common_git_dir).ok()?;
    // wsstate only supports non-bare repositories: the common dir must be a
    // `.git` directory whose parent is the common root.
    if common_git_dir.file_name().and_then(|name| name.to_str()) != Some(".git") {
        return None;
    }
    let common_root = std::fs::canonicalize(common_git_dir.parent()?).ok()?;

    Some(GitIdentity {
        worktree_root,
        common_root,
    })
}

fn git_output(repo: &Path, args: &[&str]) -> Option<String> {
    let output = Command::new("git")
        .arg("-C")
        .arg(repo)
        .args(args)
        .output()
        .ok()?;
    if !output.status.success() {
        return None;
    }
    let value = String::from_utf8(output.stdout).ok()?;
    let trimmed = value.trim();
    (!trimmed.is_empty()).then(|| trimmed.to_owned())
}

/// Byte representation of a canonical path used as the SHA-256 hashing input
/// for wsstate project/worktree keys.
///
/// `wsstate.shortHash` hashes the raw bytes of a `filepath`-cleaned path. To
/// stay key-compatible this:
/// - strips the Windows `\\?\` / `\\?\UNC\` verbatim prefix that
///   `std::fs::canonicalize` adds (Go's `filepath` produces no such prefix);
/// - on Unix hashes the raw `OsStr` bytes rather than a lossy UTF-8 string, so
///   a non-UTF-8 path still derives the same key as the Go tool.
fn canonical_path_bytes(path: &Path) -> Vec<u8> {
    #[cfg(windows)]
    {
        let raw = path.to_string_lossy();
        let normalized = if let Some(rest) = raw.strip_prefix(r"\\?\UNC\") {
            format!(r"\\{rest}")
        } else if let Some(rest) = raw.strip_prefix(r"\\?\") {
            rest.to_owned()
        } else {
            raw.into_owned()
        };
        normalized.into_bytes()
    }
    #[cfg(not(windows))]
    {
        use std::os::unix::ffi::OsStrExt;
        path.as_os_str().as_bytes().to_vec()
    }
}

/// First eight lowercase hex characters of the SHA-256 digest of `value`,
/// matching `wsstate.shortHash` used for project and worktree keys.
fn short_hash(value: &[u8]) -> String {
    let digest = sha256(value);
    let mut hex = String::with_capacity(8);
    for byte in &digest[..4] {
        hex.push_str(&format!("{byte:02x}"));
    }
    hex
}

/// Minimal SHA-256 (FIPS 180-4) over `data`.
///
/// Implemented locally to keep wsstate-compatible path-key derivation without
/// adding a hashing crate; the value is a cache-layout key, not a security
/// primitive.
fn sha256(data: &[u8]) -> [u8; 32] {
    const K: [u32; 64] = [
        0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4,
        0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe,
        0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f,
        0x4a7484aa, 0x5cb0a9dc, 0x76f988da, 0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7,
        0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc,
        0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b,
        0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116,
        0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
        0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7,
        0xc67178f2,
    ];

    let mut h: [u32; 8] = [
        0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab,
        0x5be0cd19,
    ];

    let bit_len = (data.len() as u64).wrapping_mul(8);
    let mut message = data.to_vec();
    message.push(0x80);
    while message.len() % 64 != 56 {
        message.push(0);
    }
    message.extend_from_slice(&bit_len.to_be_bytes());

    for block in message.chunks_exact(64) {
        let mut w = [0u32; 64];
        for (index, word) in w.iter_mut().take(16).enumerate() {
            let offset = index * 4;
            *word = u32::from_be_bytes([
                block[offset],
                block[offset + 1],
                block[offset + 2],
                block[offset + 3],
            ]);
        }
        for index in 16..64 {
            let s0 = w[index - 15].rotate_right(7)
                ^ w[index - 15].rotate_right(18)
                ^ (w[index - 15] >> 3);
            let s1 = w[index - 2].rotate_right(17)
                ^ w[index - 2].rotate_right(19)
                ^ (w[index - 2] >> 10);
            w[index] = w[index - 16]
                .wrapping_add(s0)
                .wrapping_add(w[index - 7])
                .wrapping_add(s1);
        }

        let [mut a, mut b, mut c, mut d, mut e, mut f, mut g, mut hh] = h;
        for index in 0..64 {
            let s1 = e.rotate_right(6) ^ e.rotate_right(11) ^ e.rotate_right(25);
            let ch = (e & f) ^ ((!e) & g);
            let t1 = hh
                .wrapping_add(s1)
                .wrapping_add(ch)
                .wrapping_add(K[index])
                .wrapping_add(w[index]);
            let s0 = a.rotate_right(2) ^ a.rotate_right(13) ^ a.rotate_right(22);
            let maj = (a & b) ^ (a & c) ^ (b & c);
            let t2 = s0.wrapping_add(maj);

            hh = g;
            g = f;
            f = e;
            e = d.wrapping_add(t1);
            d = c;
            c = b;
            b = a;
            a = t1.wrapping_add(t2);
        }

        h[0] = h[0].wrapping_add(a);
        h[1] = h[1].wrapping_add(b);
        h[2] = h[2].wrapping_add(c);
        h[3] = h[3].wrapping_add(d);
        h[4] = h[4].wrapping_add(e);
        h[5] = h[5].wrapping_add(f);
        h[6] = h[6].wrapping_add(g);
        h[7] = h[7].wrapping_add(hh);
    }

    let mut digest = [0u8; 32];
    for (index, word) in h.iter().enumerate() {
        digest[index * 4..index * 4 + 4].copy_from_slice(&word.to_be_bytes());
    }
    digest
}

/// Subset of `wsagent` `agent.json` the projection needs. Private fields such
/// as `pid` and stream paths are intentionally not deserialized. The raw
/// session id is deserialized only as daemon-private resolver input and is
/// collapsed to `sessionPresent` before any browser response is built.
#[derive(Debug, Default, Deserialize)]
struct AgentMetadata {
    #[serde(default)]
    name: String,
    #[serde(default)]
    backend: String,
    #[serde(default)]
    harness: String,
    #[serde(default)]
    tier: String,
    #[serde(default)]
    model: String,
    #[serde(default)]
    effort: String,
    #[serde(default)]
    session_id: String,
    #[serde(default)]
    status: String,
    #[serde(default)]
    last_call_at: String,
    #[serde(default)]
    last_output_path: String,
}

/// Subset of `wsagent` `current/state.json` the projection needs. PID, stream
/// paths, and the session id are intentionally omitted.
#[derive(Debug, Default, Deserialize)]
struct CurrentCallState {
    #[serde(default)]
    status: String,
    #[serde(default)]
    execution_id: String,
    #[serde(default)]
    started_at: String,
    #[serde(default)]
    updated_at: String,
    #[serde(default)]
    finished_at: String,
    #[serde(default)]
    error: String,
    #[serde(default)]
    cleanup_needed: bool,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn hex(bytes: &[u8]) -> String {
        bytes.iter().map(|byte| format!("{byte:02x}")).collect()
    }

    #[test]
    fn sha256_matches_known_answer_vectors() {
        // FIPS 180-4 / NIST known-answer vectors. The 56-byte and one-million
        // byte inputs both span multiple 64-byte blocks, exercising the
        // `chunks_exact(64)` loop and the full 32-byte digest, not just the
        // single-block path or the leading bytes used by `short_hash`.
        assert_eq!(
            hex(&sha256(b"abc")),
            "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
        );
        assert_eq!(
            hex(&sha256(b"")),
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
        );
        // 56 bytes -> two blocks after padding.
        assert_eq!(
            hex(&sha256(
                b"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"
            )),
            "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"
        );
        // One million 'a' bytes -> many blocks.
        let million_a = vec![b'a'; 1_000_000];
        assert_eq!(
            hex(&sha256(&million_a)),
            "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0"
        );
    }

    #[test]
    fn short_hash_matches_known_sha256_prefix() {
        // SHA-256("abc") = ba7816bf8f01cfea...; wsstate keys take the first
        // eight hex characters.
        assert_eq!(short_hash(b"abc"), "ba7816bf");
        // SHA-256("") = e3b0c44298fc1c14...
        assert_eq!(short_hash(b""), "e3b0c442");
    }

    #[cfg(not(windows))]
    #[test]
    fn canonical_path_bytes_uses_raw_unix_bytes() {
        // On Unix the hashing input is the exact path bytes, no verbatim
        // prefix and no lossy UTF-8 substitution.
        assert_eq!(
            canonical_path_bytes(Path::new("/tmp/ws-root")),
            b"/tmp/ws-root".to_vec()
        );
    }

    #[cfg(windows)]
    #[test]
    fn canonical_path_bytes_strips_windows_verbatim_prefix() {
        // `std::fs::canonicalize` emits `\\?\` verbatim paths; wsstate's
        // `filepath`-based path has no such prefix, so it must be stripped
        // before hashing or Windows keys diverge from the Go tool.
        assert_eq!(
            canonical_path_bytes(Path::new(r"\\?\C:\repo")),
            br"C:\repo".to_vec()
        );
        assert_eq!(
            canonical_path_bytes(Path::new(r"\\?\UNC\server\share")),
            br"\\server\share".to_vec()
        );
    }

    #[test]
    fn agent_status_maps_known_and_degraded_values() {
        assert_eq!(agent_status("idle"), ("idle".to_owned(), None));
        assert_eq!(agent_status("running"), ("running".to_owned(), None));
        assert_eq!(agent_status("blocked"), ("blocked".to_owned(), None));
        assert_eq!(agent_status("failed"), ("failed".to_owned(), None));
        assert_eq!(agent_status("erased"), ("erased".to_owned(), None));
        assert_eq!(
            agent_status(""),
            (STATUS_UNAVAILABLE.to_owned(), Some(DIAG_STATUS_UNAVAILABLE))
        );
        assert_eq!(
            agent_status("mystery"),
            (
                STATUS_UNAVAILABLE.to_owned(),
                Some(DIAG_STATUS_UNRECOGNIZED)
            )
        );
    }

    #[test]
    fn bounded_truncates_to_the_text_limit() {
        let long = "x".repeat(MAX_BOUNDED_TEXT + 50);
        assert_eq!(bounded(&long).chars().count(), MAX_BOUNDED_TEXT);
        assert_eq!(bounded("short"), "short");
    }

    #[test]
    fn codex_session_jsonl_fixture_records_parse_to_bounded_blocks() {
        let raw = r#"{"timestamp":"2026-05-22T00:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-private"}}
{"timestamp":"2026-05-22T00:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"assistant text"}}
{"timestamp":"2026-05-22T00:00:02Z","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":{"cmd":"cat /private/path"}}}
{"timestamp":"2026-05-22T00:00:03Z","type":"response_item","payload":{"type":"function_call_output","output":"secret output"}}
{"timestamp":"2026-05-22T00:00:04Z","type":"event_msg","payload":{"type":"task_complete","last_agent_message":"done"}}
"#;

        let parsed = parse_codex_session_transcript(raw);

        assert!(!parsed.degraded);
        assert!(parsed.diagnostics.is_empty());
        assert_eq!(
            parsed
                .blocks
                .iter()
                .map(|block| block.render_kind.as_str())
                .collect::<Vec<_>>(),
            vec!["status", "assistant", "toolCall", "toolResult", "status"]
        );
        assert_eq!(parsed.blocks[0].title.as_deref(), Some("Task started"));
        assert_eq!(parsed.blocks[1].text.as_deref(), Some("assistant text"));
        assert_eq!(parsed.blocks[2].title.as_deref(), Some("Tool call"));
        assert_eq!(parsed.blocks[2].data.as_ref().unwrap()["name"], "shell");
        assert_eq!(parsed.blocks[3].title.as_deref(), Some("Tool output"));
        assert_eq!(parsed.blocks[3].data.as_ref().unwrap()["outputBytes"], 13);
        assert_eq!(parsed.blocks[3].data.as_ref().unwrap()["lineCount"], 1);
        assert_eq!(
            parsed.blocks[3].data.as_ref().unwrap()["omittedMiddleLines"],
            0
        );
        assert_eq!(parsed.blocks[3].text.as_deref(), Some("secret output"));
        assert_eq!(parsed.blocks[4].title.as_deref(), Some("Task complete"));
        let encoded = serde_json::to_string(&parsed.blocks).expect("serialize parser blocks");
        assert!(!encoded.contains("/private/path"));
        assert!(!encoded.contains("turn-private"));
    }

    #[test]
    fn codex_session_jsonl_malformed_and_unsupported_records_degrade_individually() {
        let raw = r#"not json with /private/path
{"timestamp":"2026-05-22T00:00:02Z","type":"/private/native/type/thread-secret","payload":{"type":"/private/native/payload","raw":"/private/path"}}
"#;

        let parsed = parse_codex_session_transcript(raw);

        assert!(parsed.degraded);
        assert!(!parsed.diagnostics.is_empty());
        assert_eq!(parsed.blocks.len(), 2);
        assert!(parsed.blocks.iter().all(|block| block.degraded));
        assert_eq!(
            parsed.blocks[0].title.as_deref(),
            Some("Malformed transcript record")
        );
        assert_eq!(
            parsed.blocks[1].title.as_deref(),
            Some("Unsupported transcript record")
        );
        let encoded = serde_json::to_string(&parsed.blocks).expect("serialize degraded blocks");
        assert!(!encoded.contains("not json"));
        assert!(!encoded.contains("/private/path"));
        assert!(!encoded.contains("thread-secret"));
        assert_eq!(
            parsed.blocks[1].data.as_ref().unwrap()["eventType"],
            "private"
        );
        assert_eq!(
            parsed.blocks[1].data.as_ref().unwrap()["payloadType"],
            "private"
        );
        assert_eq!(
            parsed.blocks[1].data.as_ref().unwrap()["payloadFieldCount"],
            2
        );
        assert_eq!(
            parsed.blocks[1].data.as_ref().unwrap()["omissionReason"],
            "unsupported codex native shape"
        );
    }

    #[test]
    fn codex_session_jsonl_common_native_records_parse_to_safe_blocks() {
        let raw = r#"{"timestamp":"2026-05-22T00:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"Please inspect /private/repo and continue"}}
{"timestamp":"2026-05-22T00:00:01Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Here is the next step."}]}}
{"timestamp":"2026-05-22T00:00:02Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"patch touching /private/repo/file"}}
{"timestamp":"2026-05-22T00:00:03Z","type":"response_item","payload":{"type":"custom_tool_call_output","output":"private /host/path output"}}
{"timestamp":"2026-05-22T00:00:04Z","type":"event_msg","payload":{"type":"mcp_tool_call_end","status":"success","duration":12,"invocation":{"tool":"tickets_status","arguments":{"path":"/private/repo"}},"result":{"text":"/private/result"}}}
{"timestamp":"2026-05-22T00:00:05Z","type":"event_msg","payload":{"type":"exec_command_end","status":"success","exit_code":0,"duration":34,"command":"cat /private/repo/file","cwd":"/private/repo","stdout":"secret stdout","stderr":""}}
{"timestamp":"2026-05-22T00:00:06Z","type":"event_msg","payload":{"type":"patch_apply_end","status":"success","success":true,"changes":[{"path":"/private/repo/file"}],"stdout":"applied /private/repo/file","stderr":""}}
{"timestamp":"2026-05-22T00:00:07Z","type":"event_msg","payload":{"type":"turn_aborted","reason":"user_interrupt"}}
{"timestamp":"2026-05-22T00:00:08Z","type":"event_msg","payload":{"type":"thread_rolled_back","num_turns":2}}
{"timestamp":"2026-05-22T00:00:09Z","type":"event_msg","payload":{"type":"token_count","info":{"private":"/private/repo"}}}
{"timestamp":"2026-05-22T00:00:10Z","type":"session_meta","id":"thread-secret","cwd":"/private/repo"}
"#;

        let parsed = parse_codex_session_transcript(raw);

        assert!(!parsed.degraded);
        assert!(parsed.diagnostics.is_empty());
        assert_eq!(parsed.blocks.len(), 9);
        assert_eq!(parsed.blocks[0].render_kind, "user");
        assert_eq!(
            parsed.blocks[0].text.as_deref(),
            Some("Please inspect [redacted] and continue")
        );
        assert_eq!(parsed.blocks[1].render_kind, "assistant");
        assert_eq!(
            parsed.blocks[1].text.as_deref(),
            Some("Here is the next step.")
        );
        assert_eq!(parsed.blocks[2].render_kind, "toolCall");
        assert_eq!(
            parsed.blocks[2].data.as_ref().unwrap()["name"],
            "apply_patch"
        );
        assert_eq!(parsed.blocks[3].render_kind, "toolResult");
        assert_eq!(parsed.blocks[4].title.as_deref(), Some("MCP tool result"));
        assert_eq!(
            parsed.blocks[4].data.as_ref().unwrap()["outcome"],
            "success"
        );
        assert_eq!(parsed.blocks[5].title.as_deref(), Some("Command result"));
        assert_eq!(parsed.blocks[5].data.as_ref().unwrap()["exitCode"], 0);
        assert_eq!(parsed.blocks[6].title.as_deref(), Some("Patch apply"));
        assert_eq!(parsed.blocks[6].data.as_ref().unwrap()["changes"], 1);
        assert_eq!(parsed.blocks[7].title.as_deref(), Some("Turn aborted"));
        assert_eq!(
            parsed.blocks[7].data.as_ref().unwrap()["reason"],
            "user_interrupt"
        );
        assert_eq!(
            parsed.blocks[8].title.as_deref(),
            Some("Thread rolled back")
        );
        assert_eq!(parsed.blocks[8].data.as_ref().unwrap()["numTurns"], 2);
        let encoded = serde_json::to_string(&parsed.blocks).expect("serialize parser blocks");
        for forbidden in [
            "/private",
            "/host/path",
            "secret stdout",
            "thread-secret",
            "cat ",
        ] {
            assert!(
                !encoded.contains(forbidden),
                "native transcript block must not leak {forbidden}"
            );
        }
    }

    #[test]
    fn codex_session_tool_output_uses_bounded_head_tail_snippet() {
        let long_output = (0..25)
            .map(|index| format!("line-{index:02}"))
            .collect::<Vec<_>>()
            .join("\n");
        let raw = format!(
            r#"{{"timestamp":"2026-05-22T00:00:03Z","type":"response_item","payload":{{"type":"function_call_output","output":{}}}}}"#,
            serde_json::to_string(&long_output).expect("encode long output")
        );

        let parsed = parse_codex_session_transcript(&raw);

        assert_eq!(parsed.blocks.len(), 1);
        let block = &parsed.blocks[0];
        assert_eq!(block.render_kind, "toolResult");
        let text = block.text.as_deref().expect("tool output text");
        assert!(text.contains("line-00"));
        assert!(text.contains("line-09"));
        assert!(text.contains("... omitted middle lines ..."));
        assert!(text.contains("line-15"));
        assert!(text.contains("line-24"));
        assert!(!text.contains("line-10\n"));
        assert!(!text.contains("line-14\n"));
        assert_eq!(block.data.as_ref().unwrap()["lineCount"], 25);
        assert_eq!(block.data.as_ref().unwrap()["omittedMiddleLines"], 5);
    }

    #[test]
    fn codex_session_jsonl_deduplicates_adjacent_dialogue_records() {
        let raw = r#"{"timestamp":"2026-05-22T00:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"Brief path: /private/repo\nContinue"}}
{"timestamp":"2026-05-22T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Brief path: /private/repo\nContinue"}]}}
{"timestamp":"2026-05-22T00:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"Assistant line\n\n- inspect /private/cache\n  next"}}
{"timestamp":"2026-05-22T00:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Assistant line\n\n- inspect /private/cache\n  next"}]}}
"#;

        let parsed = parse_codex_session_transcript(raw);

        assert!(!parsed.degraded);
        assert_eq!(parsed.blocks.len(), 2);
        assert_eq!(parsed.blocks[0].render_kind, "user");
        assert_eq!(
            parsed.blocks[0].text.as_deref(),
            Some("Brief path: [redacted]\nContinue")
        );
        assert_eq!(parsed.blocks[1].render_kind, "assistant");
        assert_eq!(
            parsed.blocks[1].text.as_deref(),
            Some("Assistant line\n\n- inspect [redacted]\n  next")
        );
    }

    #[test]
    fn codex_session_jsonl_oversized_native_text_is_bounded() {
        let long_message = "m".repeat(MAX_NATIVE_MESSAGE_TEXT + 50);
        let long_tool_name = "tool".repeat(MAX_BOUNDED_TEXT);
        let raw = format!(
            "{}\n{}\n",
            serde_json::json!({
                "timestamp": "2026-05-22T00:00:00Z",
                "type": "event_msg",
                "payload": {
                    "type": "agent_message",
                    "message": long_message,
                }
            }),
            serde_json::json!({
                "timestamp": "2026-05-22T00:00:01Z",
                "type": "response_item",
                "payload": {
                    "type": "function_call",
                    "name": long_tool_name,
                    "arguments": {
                        "path": "/private/path/that/must/not/be/copied",
                    }
                }
            })
        );

        let parsed = parse_codex_session_transcript(&raw);

        assert_eq!(parsed.blocks.len(), 2);
        assert_eq!(
            parsed.blocks[0]
                .text
                .as_ref()
                .expect("assistant text")
                .chars()
                .count(),
            MAX_NATIVE_MESSAGE_TEXT
        );
        assert_eq!(
            parsed.blocks[1]
                .text
                .as_ref()
                .expect("tool call text")
                .chars()
                .count(),
            MAX_BOUNDED_TEXT
        );
        let encoded = serde_json::to_string(&parsed.blocks).expect("serialize bounded blocks");
        assert!(!encoded.contains("/private/path"));
    }

    #[test]
    fn resolve_cache_root_prefers_explicit_override_then_env() {
        // Explicit daemon override wins and is returned verbatim.
        let override_root = PathBuf::from("/fixture/cache-home");
        assert_eq!(
            resolve_cache_root(Some(override_root.as_path())),
            Some(override_root.clone())
        );

        // With no override, `WS_CACHE_HOME` is honored. No other test reads
        // this variable, so the set/clear stays self-contained.
        let previous = std::env::var_os("WS_CACHE_HOME");
        std::env::set_var("WS_CACHE_HOME", "/fixture/env-cache");
        assert_eq!(
            resolve_cache_root(None),
            Some(PathBuf::from("/fixture/env-cache"))
        );
        match previous {
            Some(value) => std::env::set_var("WS_CACHE_HOME", value),
            None => std::env::remove_var("WS_CACHE_HOME"),
        }
    }
}
