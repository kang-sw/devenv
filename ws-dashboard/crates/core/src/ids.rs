#[derive(Clone, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
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
pub type WorktreeId = OpaqueId;
pub type InstanceId = OpaqueId;
