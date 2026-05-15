use axum::Json;
use ws_dashboard_core::DashboardResourcesView;

use crate::mock::MockDashboardResourcesProvider;

// CONTRACT: Provider seam shared by mock data now and live providers later.
// Implementations return the same public core view-model contract.
pub trait DashboardResourcesProvider {
    fn dashboard_resources(&self) -> DashboardResourcesView;
}

// CONTRACT: GET /api/dashboard/resources is an owner-authenticated dashboard
// API route and must be nested under the protected router in router.rs.
pub async fn dashboard_resources() -> Json<DashboardResourcesView> {
    Json(MockDashboardResourcesProvider::default().dashboard_resources())
}
