use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Deserialize, Eq, Hash, Ord, PartialEq, PartialOrd, Serialize)]
pub struct OpaqueId(String);

impl OpaqueId {
    pub fn new(value: impl Into<String>) -> Self {
        Self(value.into())
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl From<&str> for OpaqueId {
    fn from(value: &str) -> Self {
        Self::new(value)
    }
}

impl From<String> for OpaqueId {
    fn from(value: String) -> Self {
        Self::new(value)
    }
}

pub type ServerId = OpaqueId;
pub type WorkspaceId = OpaqueId;
// CONTRACT: WorkRootId is the public dashboard id for the physical directory
// used as an open, spawn, and run target. Do not expose WorktreeId in dashboard
// APIs.
pub type WorkRootId = OpaqueId;
pub type InstanceId = OpaqueId;
