use serde::{Deserialize, Serialize};

use crate::ids::{InstanceId, ServerId, WorkRootId, WorkspaceId};

// CONTRACT: WorkRootStatus describes whether a remembered physical root can be
// used now without dropping it from recent dashboard context.
#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum WorkRootStatus {
    Online,
    Offline,
    Moved,
    Inaccessible,
}

// CONTRACT: Availability is derived from local discovery and is distinct from
// the user's activation choice. Do not reuse Online/Offline status vocabulary
// for activation.
#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum WorkRootAvailability {
    Available,
    Missing,
    Moved,
    Inaccessible,
    Unknown,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum WorkRootActivation {
    Online,
    Offline,
}

// CONTRACT: WorkRootKind is additive role metadata for the same core workRoot
// UI/API shape; primary roots and linked worktrees must stay distinguishable.
#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum WorkRootKind {
    PlainDirectory,
    GitPrimaryRoot,
    GitLinkedWorktree,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum ServerKind {
    Local,
    SshRemote,
    Wsl,
    Manual,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum ServerConnectionStatus {
    Connected,
    AuthRequired,
    Unreachable,
    Starting,
    StaleEndpoint,
    TunnelRequired,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum InstanceRole {
    Main,
    Sub,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum InstanceKind {
    Harness,
    Agent,
    Terminal,
    Editor,
    Viewer,
    Exec,
    Translation,
    Task,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum InteractionMode {
    Direct,
    Delegated,
    Passive,
}

// CONTRACT: Serialized resource paths use workRoot vocabulary, never worktree
// vocabulary. The UI may compact singleton rows, but the API keeps this full
// path shape.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ResourcePath {
    pub server_id: ServerId,
    pub workspace_id: WorkspaceId,
    pub work_root_id: WorkRootId,
    pub instance_id: Option<InstanceId>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::OpaqueId;

    #[test]
    fn resource_path_serializes_work_root_vocabulary() {
        // CONTRACT: JSON field names must be serverId, workspaceId, workRootId,
        // and instanceId. The serde rename_all attributes keep API structs on
        // the same naming convention.
        let path = ResourcePath {
            server_id: OpaqueId::from("server-local"),
            workspace_id: OpaqueId::from("workspace-devenv"),
            work_root_id: OpaqueId::from("root-main"),
            instance_id: Some(OpaqueId::from("instance-main")),
        };

        let value = serde_json::to_value(path).expect("serialize resource path");

        assert_eq!(
            value,
            serde_json::json!({
                "serverId": "server-local",
                "workspaceId": "workspace-devenv",
                "workRootId": "root-main",
                "instanceId": "instance-main"
            })
        );
        let object = value
            .as_object()
            .expect("resource path serializes to object");
        assert!(object.contains_key("workRootId"));
        assert!(!object.contains_key("worktreeId"));
        assert!(!object.contains_key("worktree_id"));
    }

    #[test]
    fn work_root_kind_serializes_dashboard_contract_values() {
        // CONTRACT: WorkRootKind serializes as plainDirectory, gitPrimaryRoot,
        // and gitLinkedWorktree.
        assert_eq!(
            serde_json::to_value(WorkRootKind::PlainDirectory)
                .expect("serialize plain directory kind"),
            serde_json::json!("plainDirectory")
        );
        assert_eq!(
            serde_json::to_value(WorkRootKind::GitPrimaryRoot)
                .expect("serialize git primary root kind"),
            serde_json::json!("gitPrimaryRoot")
        );
        assert_eq!(
            serde_json::to_value(WorkRootKind::GitLinkedWorktree)
                .expect("serialize git linked worktree kind"),
            serde_json::json!("gitLinkedWorktree")
        );
    }
}
