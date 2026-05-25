pub mod activity;
pub mod events;
pub mod ids;
pub mod resources;
pub mod view_model;

pub use activity::{
    ActivityConsoleEvent, ActivityFeed, ActivityItem, ActivitySnapshotInvalidationReason,
    ActivitySourceDisplay, ActivityTranscript, ActivityTranscriptAvailability, ActivityUpdateMode,
    NamedAgentActivityView, NamedAgentCallActivityView, TranscriptBlock, WorkRootActivitySummary,
    WorkRootActivityView,
};
pub use events::{
    InstanceEvent, InstanceEventCategory, InstanceEventFixtures, InstanceEventPayload,
    InstanceEventTranscript,
};
pub use ids::{InstanceId, OpaqueId, ServerId, WorkRootId, WorkspaceId};
pub use resources::{
    InstanceKind, InstanceRole, InteractionMode, ResourcePath, ServerConnectionStatus, ServerKind,
    WorkRootActivation, WorkRootAvailability, WorkRootKind, WorkRootStatus,
};
pub use view_model::{
    ActionHint, DashboardResourcesView, DashboardServersView, InstanceView,
    ServerConnectionView, ServerView, ViewState, WorkRootView, WorkspaceView,
};
