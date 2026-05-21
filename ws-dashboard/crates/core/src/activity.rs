use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::WorkRootId;

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ActivityFeed {
    // CONTRACT: Browser callers identify the opened workRoot by opaque
    // workRootId only. Host paths and ws cache paths are never API identity.
    pub work_root_id: WorkRootId,
    pub status: String,
    pub summary: WorkRootActivitySummary,
    pub items: Vec<ActivityItem>,
    // Compatibility projection for the existing read-only named-agent pane.
    // New Activity Console consumers should use `items`.
    pub agents: Vec<NamedAgentActivityView>,
}

pub type WorkRootActivityView = ActivityFeed;

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkRootActivitySummary {
    // CONTRACT: Phase 1 summarizes named-agent state only. Running command
    // counts stay out of this shape until the async exec job model exists.
    pub total: usize,
    pub active: usize,
    pub blocked: usize,
    pub failed: usize,
    pub unavailable: usize,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ActivityItem {
    pub id: String,
    pub kind: String,
    pub label: String,
    pub status: String,
    pub live: bool,
    pub attention: bool,
    pub started_at: Option<String>,
    pub updated_at: Option<String>,
    pub finished_at: Option<String>,
    pub source: ActivitySourceDisplay,
    pub transcript: ActivityTranscriptAvailability,
    pub diagnostics: Vec<String>,
    pub metadata: BTreeMap<String, Value>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ActivitySourceDisplay {
    pub kind: String,
    pub label: String,
    pub backend: Option<String>,
    pub harness: Option<String>,
    pub tier: Option<String>,
    pub model: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ActivityTranscriptAvailability {
    pub status: String,
    pub available: bool,
    pub cursor: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ActivityTranscript {
    pub work_root_id: WorkRootId,
    pub activity_id: String,
    pub status: String,
    pub live: bool,
    pub source: ActivitySourceDisplay,
    pub blocks: Vec<TranscriptBlock>,
    pub next_cursor: Option<String>,
    pub has_more: bool,
    pub diagnostics: Vec<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TranscriptBlock {
    pub cursor: String,
    pub timestamp: Option<String>,
    pub render_kind: String,
    pub title: Option<String>,
    pub text: Option<String>,
    pub data: Option<Value>,
    pub degraded: bool,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct NamedAgentActivityView {
    // CONTRACT: `agent_id` is an opaque row key derived from the agent directory
    // key when needed. It is not a host path, cache path, or session id.
    pub agent_id: String,
    pub name: Option<String>,
    pub backend: Option<String>,
    pub harness: Option<String>,
    pub tier: Option<String>,
    pub model: Option<String>,
    pub effort: Option<String>,
    pub status: String,
    pub last_call_at: Option<String>,
    pub session_present: bool,
    pub current_call: Option<NamedAgentCallActivityView>,
    pub detail_hints: Vec<String>,
    pub diagnostics: Vec<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct NamedAgentCallActivityView {
    // CONTRACT: Active/terminal are dashboard-ready booleans derived from
    // wsagent current-call state. They do not expose process ids or stream paths.
    pub status: String,
    pub active: bool,
    pub terminal: bool,
    pub execution_id: Option<String>,
    pub started_at: Option<String>,
    pub updated_at: Option<String>,
    pub finished_at: Option<String>,
    pub cleanup_needed: bool,
    pub error: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::OpaqueId;

    #[test]
    fn activity_feed_and_transcript_serialize_camel_case_without_host_internals() {
        let item = ActivityItem {
            id: "agent:reviewer".to_owned(),
            kind: "namedAgent".to_owned(),
            label: "reviewer".to_owned(),
            status: "running".to_owned(),
            live: true,
            attention: false,
            started_at: Some("2026-05-17T09:00:00Z".to_owned()),
            updated_at: Some("2026-05-17T09:01:00Z".to_owned()),
            finished_at: None,
            source: ActivitySourceDisplay {
                kind: "namedAgent".to_owned(),
                label: "reviewer".to_owned(),
                backend: Some("codex".to_owned()),
                harness: Some("codex".to_owned()),
                tier: Some("core".to_owned()),
                model: Some("gpt-5.3-codex".to_owned()),
            },
            transcript: ActivityTranscriptAvailability {
                status: "available".to_owned(),
                available: true,
                cursor: Some("0".to_owned()),
            },
            diagnostics: vec!["bounded diagnostic".to_owned()],
            metadata: BTreeMap::from([(
                "agentId".to_owned(),
                Value::String("agent-reviewer".to_owned()),
            )]),
        };
        let view = ActivityFeed {
            work_root_id: OpaqueId::from("root-local-abc"),
            status: "degraded".to_owned(),
            summary: WorkRootActivitySummary {
                total: 1,
                active: 1,
                blocked: 0,
                failed: 0,
                unavailable: 0,
            },
            items: vec![item],
            agents: vec![NamedAgentActivityView {
                agent_id: "agent-reviewer".to_owned(),
                name: Some("reviewer".to_owned()),
                backend: Some("codex".to_owned()),
                harness: Some("codex".to_owned()),
                tier: Some("core".to_owned()),
                model: Some("gpt-5.3-codex".to_owned()),
                effort: Some("medium".to_owned()),
                status: "running".to_owned(),
                last_call_at: Some("2026-05-17T09:00:00Z".to_owned()),
                session_present: true,
                current_call: Some(NamedAgentCallActivityView {
                    status: "running".to_owned(),
                    active: true,
                    terminal: false,
                    execution_id: Some("000123".to_owned()),
                    started_at: Some("2026-05-17T09:00:00Z".to_owned()),
                    updated_at: Some("2026-05-17T09:01:00Z".to_owned()),
                    finished_at: None,
                    cleanup_needed: false,
                    error: None,
                }),
                detail_hints: vec!["recent output available".to_owned()],
                diagnostics: vec!["bounded diagnostic".to_owned()],
            }],
        };

        let value = serde_json::to_value(view).expect("serialize activity feed");
        assert_eq!(value["workRootId"], "root-local-abc");
        assert_eq!(value["items"][0]["id"], "agent:reviewer");
        assert_eq!(value["items"][0]["kind"], "namedAgent");
        assert_eq!(value["items"][0]["startedAt"], "2026-05-17T09:00:00Z");
        assert_eq!(value["items"][0]["transcript"]["status"], "available");
        assert_eq!(value["agents"][0]["agentId"], "agent-reviewer");
        assert_eq!(value["agents"][0]["currentCall"]["executionId"], "000123");

        let transcript = ActivityTranscript {
            work_root_id: OpaqueId::from("root-local-abc"),
            activity_id: "agent:reviewer".to_owned(),
            status: "available".to_owned(),
            live: false,
            source: ActivitySourceDisplay {
                kind: "namedAgent".to_owned(),
                label: "reviewer".to_owned(),
                backend: Some("codex".to_owned()),
                harness: None,
                tier: None,
                model: None,
            },
            blocks: vec![TranscriptBlock {
                cursor: "1".to_owned(),
                timestamp: Some("2026-05-17T09:02:00Z".to_owned()),
                render_kind: "markdown".to_owned(),
                title: Some("Result".to_owned()),
                text: Some("bounded transcript text".to_owned()),
                data: None,
                degraded: false,
            }],
            next_cursor: Some("2".to_owned()),
            has_more: true,
            diagnostics: Vec::new(),
        };
        let transcript_value = serde_json::to_value(transcript).expect("serialize transcript");
        assert_eq!(transcript_value["activityId"], "agent:reviewer");
        assert_eq!(transcript_value["blocks"][0]["renderKind"], "markdown");
        assert_eq!(transcript_value["nextCursor"], "2");
        assert_eq!(transcript_value["hasMore"], true);

        let body = serde_json::to_string(&(value, transcript_value)).expect("activity JSON string");
        for forbidden in [
            "work_root_id",
            "activity_id",
            "render_kind",
            "next_cursor",
            "agent_id",
            "current_call",
            "session_id",
            "pid",
            "stdout_path",
            "stderr_path",
            "agent.json",
            "current/state.json",
            "/Users/",
            "/cache/",
        ] {
            assert!(
                !body.contains(forbidden),
                "activity JSON leaked {forbidden}"
            );
        }
    }
}
