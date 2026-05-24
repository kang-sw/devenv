use axum::extract::State;
use axum::Json;
use ws_dashboard_core::{DashboardResourcesView, WorkRootId};

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
    // Live discovery runs synchronous filesystem and `git` subprocess work, so
    // keep it off the async worker threads.
    let opened = state.opened_work_roots.clone();
    let (view, pruned_work_root_ids) =
        tokio::task::spawn_blocking(move || live_dashboard_resources_with_sync(&opened))
            .await
            .expect("dashboard resources discovery task panicked");
    if !pruned_work_root_ids.is_empty() {
        state
            .terminals
            .remove_for_work_roots(&pruned_work_root_ids.into_iter().collect());
    }
    Json(view)
}

/// Build the live dashboard resource view from the daemon's opened workRoots.
///
/// Shared by the canonical resources route and the open-workRoot route so the
/// immediately-returned open response matches later canonical refreshes.
pub fn live_dashboard_resources(opened: &OpenedWorkRoots) -> DashboardResourcesView {
    live_dashboard_resources_with_sync(opened).0
}

pub fn live_dashboard_resources_with_sync(
    opened: &OpenedWorkRoots,
) -> (DashboardResourcesView, Vec<WorkRootId>) {
    let sync = LocalDashboardResourcesProvider::with_registry_activations(
        opened
            .owner_candidate_roots()
            .into_iter()
            .map(|root| LocalWorkRootCandidate::with_activation(root.path, root.activation))
            .collect(),
        opened.activation_by_work_root_id(),
    )
    .dashboard_resources_with_registry_sync();
    for work_root_id in &sync.pruned_work_root_ids {
        opened.unregister(work_root_id);
    }
    opened.sync_discovered_roots(sync.discovered_registry_roots);
    (sync.view, sync.pruned_work_root_ids)
}
