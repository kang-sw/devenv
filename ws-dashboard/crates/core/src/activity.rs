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
    pub update_mode: String,
    pub feed_cursor: Option<String>,
    pub selected_item_id: Option<String>,
    pub summary: WorkRootActivitySummary,
    pub items: Vec<ActivityItem>,
    // Compatibility projection for the existing read-only named-agent pane.
    // New Activity Console consumers should use `items`.
    pub agents: Vec<NamedAgentActivityView>,
}

pub type WorkRootActivityView = ActivityFeed;

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(
    tag = "type",
    rename_all = "camelCase",
    rename_all_fields = "camelCase"
)]
pub enum ActivityConsoleEvent {
    ItemUpserted {
        cursor: String,
        item: ActivityItem,
    },
    ItemRemoved {
        cursor: String,
        activity_id: String,
    },
    TranscriptUpdated {
        cursor: String,
        activity_id: String,
        transcript_cursor: Option<String>,
    },
    SnapshotInvalidated {
        cursor: String,
        reason: ActivitySnapshotInvalidationReason,
    },
    ModeChanged {
        cursor: String,
        update_mode: ActivityUpdateMode,
    },
    Heartbeat {
        cursor: String,
    },
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum ActivitySnapshotInvalidationReason {
    Overflow,
    WatchReset,
    Fallback,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum ActivityUpdateMode {
    Watch,
    PollFallback,
    Snapshot,
}

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
    // CONTRACT: open string vocabulary, not a closed enum — adding a value is
    // additive, doc-comment-only. Known values as of
    // `260620-feat-ws-dashboard-agent-client-activity-sources` Phase 1:
    // `namedAgent` (legacy ws-mercenary/named-agent compatibility source),
    // `exec`, `agent.codex` (Codex app-server interactive source),
    // `agent.opencode` (OpenCode ACP interactive source), and
    // `agent.claude` (Claude CLI headless stream-json duplex interactive
    // source). Parsers/tests must tolerate unrecognized future values.
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
    // CONTRACT: same open string vocabulary and known-value set as
    // `ActivityItem::kind` above; kept a separate field rather than a shared
    // type alias to match the existing struct shape.
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
    pub source_status: String,
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
    // CONTRACT: open string vocabulary, not a closed enum. Known values as
    // of `260620-feat-ws-dashboard-agent-client-activity-sources` Phase 1:
    // `markdown`, `text`, `json`, and `thinking` — extractable
    // reasoning/thinking content (Claude `assistant` stream "thinking"
    // segments, Codex reasoning item stream), kept distinct from ordinary
    // `assistant`-role text blocks. Parsers/tests must tolerate
    // unrecognized future values.
    pub render_kind: String,
    pub title: Option<String>,
    pub text: Option<String>,
    pub data: Option<Value>,
    pub degraded: bool,
    // CONTRACT (`260713-bug-dashboard-agent-chat-transcript-role-turnid-echo`
    // Phase 2): additive, backward-compatible fields. `role` is an open
    // string vocabulary (`"user"`, `"agent"`, `"tool"`, or unset for
    // thinking/reasoning content); Claude's projector never emits `"user"`
    // (its stream-json protocol never echoes the client's own prompt).
    // `turn_id` is browser-side bubble-merge-equality-only (see
    // `agentChatBubbles.tsx`'s `canMerge`) -- it carries no session/cache/
    // process identity and must not be treated as such.
    pub role: Option<String>,
    pub turn_id: Option<String>,
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
            update_mode: "snapshot".to_owned(),
            feed_cursor: Some("snapshot:1".to_owned()),
            selected_item_id: Some("agent:reviewer".to_owned()),
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
        assert_eq!(value["updateMode"], "snapshot");
        assert_eq!(value["feedCursor"], "snapshot:1");
        assert_eq!(value["selectedItemId"], "agent:reviewer");
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
            source_status: "ok".to_owned(),
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
                role: None,
                turn_id: None,
            }],
            next_cursor: Some("2".to_owned()),
            has_more: true,
            diagnostics: Vec::new(),
        };
        let transcript_value = serde_json::to_value(transcript).expect("serialize transcript");
        assert_eq!(transcript_value["activityId"], "agent:reviewer");
        assert_eq!(transcript_value["sourceStatus"], "ok");
        assert_eq!(transcript_value["blocks"][0]["renderKind"], "markdown");
        assert_eq!(transcript_value["nextCursor"], "2");
        assert_eq!(transcript_value["hasMore"], true);

        let body = serde_json::to_string(&(value, transcript_value)).expect("activity JSON string");
        for forbidden in [
            "work_root_id",
            "activity_id",
            "render_kind",
            "next_cursor",
            "source_status",
            "selected_item_id",
            "feed_cursor",
            "update_mode",
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

    #[test]
    fn activity_console_events_serialize_source_neutral_contract() {
        let item = ActivityItem {
            id: "agent:reviewer".to_owned(),
            kind: "namedAgent".to_owned(),
            label: "reviewer".to_owned(),
            status: "running".to_owned(),
            live: true,
            attention: false,
            started_at: None,
            updated_at: Some("2026-05-21T00:00:00Z".to_owned()),
            finished_at: None,
            source: ActivitySourceDisplay {
                kind: "namedAgent".to_owned(),
                label: "reviewer".to_owned(),
                backend: Some("codex".to_owned()),
                harness: None,
                tier: None,
                model: None,
            },
            transcript: ActivityTranscriptAvailability {
                status: "available".to_owned(),
                available: true,
                cursor: Some("0".to_owned()),
            },
            diagnostics: Vec::new(),
            metadata: BTreeMap::new(),
        };

        let value = serde_json::to_value(ActivityConsoleEvent::ItemUpserted {
            cursor: "0000000001".to_owned(),
            item,
        })
        .expect("serialize item event");
        assert_eq!(value["type"], "itemUpserted");
        assert_eq!(value["cursor"], "0000000001");
        assert_eq!(value["item"]["id"], "agent:reviewer");
        assert!(value.get("work_root_path").is_none());

        let mode = serde_json::to_value(ActivityConsoleEvent::ModeChanged {
            cursor: "0000000002".to_owned(),
            update_mode: ActivityUpdateMode::PollFallback,
        })
        .expect("serialize mode event");
        assert_eq!(mode["type"], "modeChanged");
        assert_eq!(mode["updateMode"], "pollFallback");

        let invalidation = serde_json::to_value(ActivityConsoleEvent::SnapshotInvalidated {
            cursor: "0000000003".to_owned(),
            reason: ActivitySnapshotInvalidationReason::WatchReset,
        })
        .expect("serialize invalidation event");
        assert_eq!(invalidation["type"], "snapshotInvalidated");
        assert_eq!(invalidation["reason"], "watchReset");
    }

    #[test]
    fn new_source_and_render_kinds_are_additive_not_a_schema_break() {
        let item = ActivityItem {
            id: "codex:thread-1".to_owned(),
            kind: "agent.codex".to_owned(),
            label: "codex session".to_owned(),
            status: "running".to_owned(),
            live: true,
            attention: false,
            started_at: None,
            updated_at: Some("2026-07-11T00:00:00Z".to_owned()),
            finished_at: None,
            source: ActivitySourceDisplay {
                kind: "agent.codex".to_owned(),
                label: "codex".to_owned(),
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
            diagnostics: Vec::new(),
            metadata: BTreeMap::new(),
        };
        let value = serde_json::to_value(&item).expect("serialize agent.codex item");
        assert_eq!(value["kind"], "agent.codex");
        assert_eq!(value["source"]["kind"], "agent.codex");
        let round_tripped: ActivityItem =
            serde_json::from_value(value).expect("deserialize agent.codex item");
        assert_eq!(round_tripped, item);

        let block = TranscriptBlock {
            cursor: "1".to_owned(),
            timestamp: Some("2026-07-11T00:00:01Z".to_owned()),
            render_kind: "thinking".to_owned(),
            title: Some("Reasoning".to_owned()),
            text: Some("weighing two approaches".to_owned()),
            data: None,
            degraded: false,
            role: None,
            turn_id: None,
        };
        let block_value = serde_json::to_value(&block).expect("serialize thinking block");
        assert_eq!(block_value["renderKind"], "thinking");
        let round_tripped_block: TranscriptBlock =
            serde_json::from_value(block_value).expect("deserialize thinking block");
        assert_eq!(round_tripped_block, block);
    }
}
