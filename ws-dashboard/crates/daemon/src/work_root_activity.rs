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

pub async fn work_root_activity(
    State(state): State<AppState>,
    AxumPath(work_root_id): AxumPath<String>,
) -> Response {
    let work_root_id = WorkRootId::from(work_root_id);
    let Some(_root_path) = state.opened_work_roots.resolve(&work_root_id) else {
        return activity_error(StatusCode::NOT_FOUND, "unknown workRoot");
    };

    // CONTRACT: Phase 1 implementation reads wsstate/wsagent agent records for
    // this opened workRoot through daemon-owned projection logic. Browser callers
    // never receive cache paths, host paths, session ids, pids, or stream paths.
    // HINT: Derive the cache layout from agents-plugin-tool/internal/wsstate
    // behavior and parse agents/*/agent.json plus current/state.json.
    // HOLE: Choose the project-local cache-root override/test fixture seam.
    Json(WorkRootActivityView {
        work_root_id,
        status: "ok".to_owned(),
        summary: WorkRootActivitySummary::default(),
        agents: Vec::new(),
    })
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
