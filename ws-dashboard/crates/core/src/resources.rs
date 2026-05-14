use crate::ids::{InstanceId, ServerId, WorkspaceId, WorktreeId};

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum WorktreeState {
    Online,
    Offline,
    Moved,
    Inaccessible,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum InstanceRole {
    Main,
    Sub,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
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

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum InteractionMode {
    Direct,
    Delegated,
    Passive,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ResourcePath {
    pub server_id: ServerId,
    pub workspace_id: WorkspaceId,
    pub worktree_id: WorktreeId,
    pub instance_id: Option<InstanceId>,
}
