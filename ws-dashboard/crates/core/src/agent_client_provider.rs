//! Dashboard-owned ACP-shaped `AgentClientProvider` contract.
//!
//! Phase 1 of `260620-feat-ws-dashboard-agent-client-activity-sources` fixes
//! the vocabulary a future Codex app-server adapter (Phase 2), OpenCode ACP
//! adapter (Phase 3), and Claude CLI stream-json duplex adapter (Phase 4)
//! will implement. This module has no runtime behavior: no adapter spawns a
//! provider process, no route is registered, and nothing in this file is
//! called from daemon code yet. It exists so the contract shape can be
//! reviewed and hand-synchronized with
//! `ws-dashboard/frontend/src/workRootActivity.ts` /
//! `ws-dashboard/frontend/src/activitySessionApi.ts` before adapter code
//! exists.
//!
//! CONTRACT: every shape here follows the same browser-identity rule as
//! `crate::activity` — provider thread ids, turn ids, session ids, raw
//! provider event ids, process ids, and cache/transcript paths stay
//! daemon-private. `activity_id` is the only cross-provider identity a
//! caller of this contract may hold. See
//! `ai-docs/spec/ws-web-dashboard/index.md#260521-ws-dashboard-activity-console-read-model`
//! and `ai-docs/mental-model/ws-dashboard-agent-harness.md` (Passthrough/
//! Overlay/Hack/Unavailable tiering) for the rules this contract must keep
//! satisfying as concrete adapters are built.

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::activity::TranscriptBlock;
use crate::WorkRootId;

/// Per-(harness, capability) support flags a concrete provider reports so a
/// future frontend can hide/disable the per-harness-gated
/// `activity.session.compact/steer/goal.*/rewind/fork/skills` controls.
/// Tier classification (Passthrough/Overlay/Hack/Unavailable) and the
/// per-harness research trail live in
/// `ai-docs/mental-model/ws-dashboard-agent-harness.md`; this struct only
/// carries the resulting booleans a provider reports at `initialize` time,
/// never the tiering rationale itself.
#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientCapabilities {
    pub compact: bool,
    pub steer: bool,
    pub goal: bool,
    pub rewind: bool,
    pub fork: bool,
    pub skills: bool,
}

/// Provider identity/version metadata for display only. Never a substitute
/// for the opaque `workRootId`/`activityId` identity model, and never a
/// vessel for a provider-native session/thread id.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientProviderMetadata {
    // CONTRACT: dashboard-owned discriminator (e.g. "codex", "opencode",
    // "claude"), not a raw provider binary/package name.
    pub provider: String,
    pub version: Option<String>,
    pub capabilities: AgentClientCapabilities,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientInitializeRequest {
    pub work_root_id: WorkRootId,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientInitializeResult {
    pub metadata: AgentClientProviderMetadata,
}

/// Compact session-list row. Full row-to-`ActivityItem` projection is the
/// caller's job (see `crate::activity::ActivityItem`); this shape only
/// carries what a provider itself can report before projection.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientSessionSummary {
    pub activity_id: String,
    pub label: String,
    pub status: String,
    pub updated_at: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientSessionListRequest {
    pub work_root_id: WorkRootId,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientSessionListResult {
    pub sessions: Vec<AgentClientSessionSummary>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientSessionCreateRequest {
    pub work_root_id: WorkRootId,
    pub initial_prompt: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientSessionCreateResult {
    pub activity_id: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientSessionResumeRequest {
    pub activity_id: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientPromptSendRequest {
    pub activity_id: String,
    pub text: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientPromptSendResult {
    pub accepted: bool,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum AgentClientMessageRole {
    User,
    Assistant,
}

/// A single provider-reported message/thinking fragment, prior to
/// projection into a `crate::activity::TranscriptBlock`. `render_kind`
/// mirrors the same open vocabulary as `TranscriptBlock::render_kind`
/// (including the new `thinking` value) so projection is a near-identity
/// mapping for the common subset.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientMessageEvent {
    pub activity_id: String,
    pub role: AgentClientMessageRole,
    pub render_kind: String,
    pub text: Option<String>,
    pub data: Option<Value>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientToolActivityEvent {
    pub activity_id: String,
    pub name: String,
    // CONTRACT: bounded display status only, e.g. "started" | "completed" |
    // "failed"; never a raw provider tool-call payload.
    pub status: String,
    pub summary: Option<String>,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum AgentClientPermissionState {
    Granted,
    Denied,
    Pending,
    Blocked,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientPermissionEvent {
    pub activity_id: String,
    pub state: AgentClientPermissionState,
    pub reason: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientInterruptRequest {
    pub activity_id: String,
}

/// Bounded file-change summary row. `path_hint` is a repo-relative display
/// string, never an absolute host path (same rule as diagnostics elsewhere
/// in `crate::activity`).
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientFileChangeSummary {
    pub path_hint: String,
    pub change_kind: String,
    pub lines_added: Option<u32>,
    pub lines_removed: Option<u32>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientTranscriptBackfillRequest {
    pub activity_id: String,
    pub cursor: Option<String>,
    pub before: Option<String>,
    pub limit: Option<u32>,
}

/// Reuses `crate::activity::TranscriptBlock` directly: a provider's
/// transcript backfill result is already the browser-facing block shape,
/// not a separate provider-native record type.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientTranscriptBackfillResult {
    pub blocks: Vec<TranscriptBlock>,
    pub next_cursor: Option<String>,
    pub has_more: bool,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AgentClientProviderError {
    pub code: String,
    pub message: String,
}

/// Contract a Phase 2-4 adapter (Codex app-server, OpenCode ACP, Claude CLI
/// stream-json duplex) implements. Phase 1 pinned the DTO shapes with
/// synchronous placeholder methods and explicitly deferred the async-ness
/// decision to Phase 2.
///
/// PHASE-2 DECISION (async): methods are `async fn`. The Codex transport is an
/// inherently async duplex over a tokio child process (piped stdio, per-request
/// `oneshot` correlation, notification fan-out), and all callers are async axum
/// handlers. Async methods let an adapter `.await` its transport directly
/// rather than `block_on`/`block_in_place` inside an async worker (which the
/// plan rejects and which risks starving the multi-thread runtime). The DTO
/// shapes are unchanged from Phase 1. The trait is consumed as a concrete type
/// (the daemon's `CodexProviderRegistry` owns a concrete adapter), so
/// async-fn-in-trait's lack of `dyn` compatibility does not apply here.
#[allow(async_fn_in_trait)]
pub trait AgentClientProvider {
    async fn initialize(
        &self,
        request: AgentClientInitializeRequest,
    ) -> Result<AgentClientInitializeResult, AgentClientProviderError>;

    async fn list_sessions(
        &self,
        request: AgentClientSessionListRequest,
    ) -> Result<AgentClientSessionListResult, AgentClientProviderError>;

    async fn create_session(
        &self,
        request: AgentClientSessionCreateRequest,
    ) -> Result<AgentClientSessionCreateResult, AgentClientProviderError>;

    async fn resume_session(
        &self,
        request: AgentClientSessionResumeRequest,
    ) -> Result<(), AgentClientProviderError>;

    async fn send_prompt(
        &self,
        request: AgentClientPromptSendRequest,
    ) -> Result<AgentClientPromptSendResult, AgentClientProviderError>;

    async fn interrupt(
        &self,
        request: AgentClientInterruptRequest,
    ) -> Result<(), AgentClientProviderError>;

    async fn backfill_transcript(
        &self,
        request: AgentClientTranscriptBackfillRequest,
    ) -> Result<AgentClientTranscriptBackfillResult, AgentClientProviderError>;
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::OpaqueId;

    #[test]
    fn agent_client_capabilities_round_trip_camel_case() {
        let capabilities = AgentClientCapabilities {
            compact: true,
            steer: true,
            goal: true,
            rewind: true,
            fork: true,
            skills: false,
        };
        let value = serde_json::to_value(capabilities).expect("serialize capabilities");
        assert_eq!(value["compact"], true);
        assert_eq!(value["skills"], false);
        let round_tripped: AgentClientCapabilities =
            serde_json::from_value(value).expect("deserialize capabilities");
        assert_eq!(round_tripped, capabilities);
    }

    #[test]
    fn agent_client_session_create_request_round_trips_without_provider_identity_leak() {
        let request = AgentClientSessionCreateRequest {
            work_root_id: OpaqueId::from("root-local-abc"),
            initial_prompt: Some("review this diff".to_owned()),
        };
        let value = serde_json::to_value(&request).expect("serialize session create request");
        assert_eq!(value["workRootId"], "root-local-abc");
        assert_eq!(value["initialPrompt"], "review this diff");
        let round_tripped: AgentClientSessionCreateRequest =
            serde_json::from_value(value).expect("deserialize session create request");
        assert_eq!(round_tripped, request);
    }

    #[test]
    fn agent_client_transcript_backfill_result_reuses_transcript_block_contract() {
        let result = AgentClientTranscriptBackfillResult {
            blocks: vec![TranscriptBlock {
                cursor: "1".to_owned(),
                timestamp: Some("2026-07-11T00:00:00Z".to_owned()),
                render_kind: "thinking".to_owned(),
                title: Some("Reasoning".to_owned()),
                text: Some("considering the diff".to_owned()),
                data: None,
                degraded: false,
            }],
            next_cursor: Some("2".to_owned()),
            has_more: false,
        };
        let value = serde_json::to_value(&result).expect("serialize backfill result");
        assert_eq!(value["blocks"][0]["renderKind"], "thinking");
        assert_eq!(value["nextCursor"], "2");
        assert_eq!(value["hasMore"], false);
    }

    #[test]
    fn agent_client_provider_metadata_serializes_capabilities_nested() {
        let metadata = AgentClientProviderMetadata {
            provider: "codex".to_owned(),
            version: Some("0.144.1".to_owned()),
            capabilities: AgentClientCapabilities {
                compact: true,
                steer: true,
                goal: true,
                rewind: true,
                fork: true,
                skills: true,
            },
        };
        let value = serde_json::to_value(&metadata).expect("serialize provider metadata");
        assert_eq!(value["provider"], "codex");
        assert_eq!(value["capabilities"]["fork"], true);
    }
}
