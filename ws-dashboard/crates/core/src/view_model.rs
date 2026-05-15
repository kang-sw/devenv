use serde::{Deserialize, Serialize};

use crate::{
    InstanceId, InstanceKind, InstanceRole, InteractionMode, ResourcePath, ServerId, WorkRootId,
    WorkRootKind, WorkRootStatus, WorkspaceId,
};

// CONTRACT: The first visible dashboard API returns the full hierarchy instead
// of pre-collapsing compactable singleton rows.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DashboardResourcesView {
    pub server: ServerView,
    pub workspaces: Vec<WorkspaceView>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ServerView {
    pub id: ServerId,
    pub label: String,
    pub state: ViewState,
    pub actions: Vec<ActionHint>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceView {
    pub id: WorkspaceId,
    pub label: String,
    pub state: ViewState,
    pub compactable: bool,
    pub work_roots: Vec<WorkRootView>,
    pub actions: Vec<ActionHint>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkRootView {
    pub id: WorkRootId,
    pub resource_path: ResourcePath,
    pub label: String,
    pub kind: WorkRootKind,
    pub status: WorkRootStatus,
    pub state: ViewState,
    pub compactable: bool,
    pub main_instances: Vec<InstanceView>,
    pub actions: Vec<ActionHint>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstanceView {
    pub id: InstanceId,
    pub resource_path: ResourcePath,
    pub role: InstanceRole,
    pub kind: InstanceKind,
    pub interaction_mode: InteractionMode,
    pub label: String,
    pub state: ViewState,
    pub sub_instances: Vec<InstanceView>,
    pub actions: Vec<ActionHint>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ViewState {
    pub status: String,
    pub loading: bool,
    pub stale: bool,
    pub error: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ActionHint {
    pub id: String,
    pub label: String,
    pub enabled: bool,
}

#[cfg(test)]
mod tests {
    #[test]
    fn dashboard_resources_view_preserves_full_hierarchy_shape() {
        // CONTRACT: serialized JSON must expose server, workspaces, workRoots,
        // mainInstances, and subInstances using camelCase dashboard API names.
        todo!("assert DashboardResourcesView JSON shape for full hierarchy");
    }

    #[test]
    fn view_rows_include_state_and_action_hints() {
        // CONTRACT: rows expose status/loading/stale/error state and action
        // hints so frontend empty/error/loading/action UI can use one shape.
        todo!("assert row state and actions serialize consistently");
    }
}
