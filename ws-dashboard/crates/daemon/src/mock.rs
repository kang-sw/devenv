use ws_dashboard_core::DashboardResourcesView;

use crate::resources::DashboardResourcesProvider;

#[derive(Clone, Debug, Default)]
pub struct MockDashboardResourcesProvider;

impl DashboardResourcesProvider for MockDashboardResourcesProvider {
    fn dashboard_resources(&self) -> DashboardResourcesView {
        // CONTRACT: deterministic fixture-backed data must cover singleton
        // chains, multi-root workspaces, all workRoot kinds, offline or
        // inaccessible roots, main/sub instances, state fields, and actions.
        todo!("return deterministic dashboard resources fixture")
    }
}
