use std::path::{Path, PathBuf};

use axum::extract::{Path as AxumPath, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::Serialize;
use ws_dashboard_core::{WorkRootActivitySummary, WorkRootActivityView, WorkRootId};

use crate::router::AppState;

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
struct WorkRootActivityError {
    error: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct WorkRootActivityProjectionConfig {
    // HINT: The wsstate Go manager accepts `WS_CACHE_HOME` as its cache-home
    // override. The dashboard keeps that override daemon-side so tests can
    // point at fixture cache trees without making browser API identity depend
    // on cache paths.
    pub cache_home: Option<PathBuf>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct WorkRootActivityProjector {
    cache_home: Option<PathBuf>,
}

impl WorkRootActivityProjector {
    pub fn new(config: WorkRootActivityProjectionConfig) -> Self {
        Self {
            cache_home: config.cache_home,
        }
    }

    pub async fn project(
        &self,
        work_root_id: WorkRootId,
        root_path: &Path,
    ) -> WorkRootActivityView {
        // CONTRACT: Phase 1 implementation reads wsstate/wsagent agent records for
        // this opened workRoot through daemon-owned projection logic. Browser callers
        // never receive cache paths, host paths, session ids, pids, or stream paths.
        // HINT: The concrete parser belongs here later: derive the wsstate worktree
        // layout from agents-plugin-tool/internal/wsstate and parse
        // agents/*/agent.json plus current/state.json from the selected workRoot.
        // Skeleton-only placeholder: keep the selected root and fixture cache-home
        // seam daemon-local, and return the no-agent projection shape.
        let _daemon_only_inputs = (root_path, self.cache_home.as_deref());

        WorkRootActivityView {
            work_root_id,
            status: "ok".to_owned(),
            summary: WorkRootActivitySummary::default(),
            agents: Vec::new(),
        }
    }
}

pub async fn work_root_activity(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let Some(root_path) = state.opened_work_roots.resolve(&work_root_id) else {
        return activity_error(StatusCode::NOT_FOUND, "unknown workRoot");
    };

    Json(
        state
            .work_root_activity
            .project(work_root_id, &root_path)
            .await,
    )
    .into_response()
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
