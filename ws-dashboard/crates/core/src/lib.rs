pub mod activity;
pub mod agent_client_provider;
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
pub use agent_client_provider::{
    AgentClientCapabilities, AgentClientFileChangeSummary, AgentClientInitializeRequest,
    AgentClientInitializeResult, AgentClientInterruptRequest, AgentClientMessageEvent,
    AgentClientMessageRole, AgentClientPermissionEvent, AgentClientPermissionState,
    AgentClientPromptSendRequest, AgentClientPromptSendResult, AgentClientProvider,
    AgentClientProviderError, AgentClientProviderMetadata, AgentClientSessionCreateRequest,
    AgentClientSessionCreateResult, AgentClientSessionListRequest, AgentClientSessionListResult,
    AgentClientSessionResumeRequest, AgentClientSessionSummary,
    AgentClientTranscriptBackfillRequest, AgentClientTranscriptBackfillResult,
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
