use ws_dashboard_core::DashboardResourcesView;

use crate::resources::DashboardResourcesProvider;

// CONTRACT: the canonical /api/dashboard/resources route is live (see
// resources.rs). This mock provider is retained only for deterministic tests
// and explicit fixture/development paths; it must not back the daemon route.
#[derive(Clone, Debug, Default)]
pub struct MockDashboardResourcesProvider;

impl DashboardResourcesProvider for MockDashboardResourcesProvider {
    fn dashboard_resources(&self) -> DashboardResourcesView {
        // CONTRACT: deterministic fixture-backed data must cover singleton
        // chains, multi-root workspaces, all workRoot kinds, offline or
        // inaccessible roots, main/sub instances, state fields, and actions.
        serde_json::from_str(include_str!("../tests/fixtures/dashboard_resources.json"))
            .expect("dashboard resources fixture is valid")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn mock_provider_still_deserializes_golden_fixture() {
        // The live route no longer exercises this fixture, so keep explicit
        // coverage that the golden artifact stays a valid view-model contract.
        let view = MockDashboardResourcesProvider.dashboard_resources();
        assert_eq!(view.server.id.as_str(), "server-local");
        assert_eq!(view.workspaces.len(), 2);
    }
}
