use axum::extract::State;
use axum::Json;
use ws_dashboard_core::DashboardResourcesView;

use crate::discovery::{LocalDashboardResourcesProvider, LocalWorkRootCandidate};
use crate::router::AppState;
use crate::work_root_files::OpenedWorkRoots;

// CONTRACT: Provider seam shared by the live local provider and the mock
// fixture provider. Implementations return the same public core view-model
// contract.
pub trait DashboardResourcesProvider {
    fn dashboard_resources(&self) -> DashboardResourcesView;
}

// CONTRACT: GET /api/dashboard/resources is an owner-authenticated dashboard
// API route and must be nested under the protected router in router.rs.
//
// CONTRACT: the canonical route reports live opened-workRoot state, never the
// static mock fixture. Before any workRoot is opened it returns an honest
// empty live view (server present, `workspaces: []`).
pub async fn dashboard_resources(State(state): State<AppState>) -> Json<DashboardResourcesView> {
    Json(live_dashboard_resources(&state.opened_work_roots))
}

/// Build the live dashboard resource view from the daemon's opened workRoots.
///
/// Shared by the canonical resources route and the open-workRoot route so the
/// immediately-returned open response matches later canonical refreshes.
pub fn live_dashboard_resources(opened: &OpenedWorkRoots) -> DashboardResourcesView {
    LocalDashboardResourcesProvider::new(
        opened
            .candidate_paths()
            .into_iter()
            .map(LocalWorkRootCandidate::new)
            .collect(),
    )
    .dashboard_resources()
}
