use serde::{Deserialize, Serialize};

use crate::WorkRootId;

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkRootActivityView {
    // CONTRACT: Browser callers identify the opened workRoot by opaque
    // workRootId only. Host paths and ws cache paths are never API identity.
    pub work_root_id: WorkRootId,
    pub status: String,
    pub summary: WorkRootActivitySummary,
    pub agents: Vec<NamedAgentActivityView>,
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
    fn work_root_activity_view_serializes_camel_case_without_host_internals() {
        let view = WorkRootActivityView {
            work_root_id: OpaqueId::from("root-local-abc"),
            status: "degraded".to_owned(),
            summary: WorkRootActivitySummary {
                total: 1,
                active: 1,
                blocked: 0,
                failed: 0,
                unavailable: 0,
            },
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

        let value = serde_json::to_value(view).expect("serialize workRoot activity");
        assert_eq!(value["workRootId"], "root-local-abc");
        assert_eq!(value["summary"]["unavailable"], 0);
        assert_eq!(value["agents"][0]["agentId"], "agent-reviewer");
        assert_eq!(value["agents"][0]["lastCallAt"], "2026-05-17T09:00:00Z");
        assert_eq!(value["agents"][0]["sessionPresent"], true);
        assert_eq!(value["agents"][0]["currentCall"]["executionId"], "000123");
        assert_eq!(value["agents"][0]["currentCall"]["cleanupNeeded"], false);

        let body = serde_json::to_string(&value).expect("activity JSON string");
        for forbidden in [
            "work_root_id",
            "agent_id",
            "current_call",
            "session_id",
            "pid",
            "stdout_path",
            "stderr_path",
            "agent.json",
            "current/state.json",
        ] {
            assert!(
                !body.contains(forbidden),
                "activity JSON leaked {forbidden}"
            );
        }
    }
}
