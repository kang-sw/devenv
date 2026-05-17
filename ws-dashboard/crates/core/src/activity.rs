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
