use serde::{Deserialize, Serialize};

use crate::ids::{InstanceId, ServerId, WorkRootId, WorkspaceId};

// CONTRACT: WorkRootStatus describes whether a remembered physical root can be
// used now without dropping it from recent dashboard context.
#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub enum WorkRootStatus {
    Online,
    Offline,
    Moved,
    Inaccessible,
}

// CONTRACT: WorkRootKind is additive role metadata for the same core workRoot
// UI/API shape; primary roots and linked worktrees must stay distinguishable.
#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub enum WorkRootKind {
    PlainDirectory,
    GitPrimaryRoot,
    GitLinkedWorktree,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub enum InstanceRole {
    Main,
    Sub,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
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
pub enum InteractionMode {
    Direct,
    Delegated,
    Passive,
}

// CONTRACT: Serialized resource paths use workRoot vocabulary, never worktree
// vocabulary. The UI may compact singleton rows, but the API keeps this full
// path shape.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
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
        // and instanceId. HINT: use serde rename_all rather than hand-built
        // strings so API structs share one naming convention.
        let _path = ResourcePath {
            server_id: OpaqueId::from("server-local"),
            workspace_id: OpaqueId::from("workspace-devenv"),
            work_root_id: OpaqueId::from("root-main"),
            instance_id: Some(OpaqueId::from("instance-main")),
        };

        todo!("assert ResourcePath JSON contains workRootId and not worktreeId");
    }

    #[test]
    fn work_root_kind_serializes_dashboard_contract_values() {
        // CONTRACT: WorkRootKind serializes as plainDirectory, gitPrimaryRoot,
        // and gitLinkedWorktree.
        todo!("assert WorkRootKind JSON values match the dashboard API contract");
    }
}
