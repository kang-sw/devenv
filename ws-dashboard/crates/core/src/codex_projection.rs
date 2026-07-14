//! Pure, synchronous Codex app-server event projection.
//!
//! Phase 2 of `260620-feat-ws-dashboard-agent-client-activity-sources` splits
//! the Codex adapter into a runtime-free projection mapper (this module) and an
//! async transport/session owner (the daemon's `codex_app_server`). This module
//! consumes the classified Codex JSON-RPC notification stream and produces the
//! browser-facing `crate::activity::TranscriptBlock`s plus read-only usage
//! state. It has no I/O, no runtime, and no knowledge of process spawning, so
//! it is exhaustively fixture-testable.
//!
//! CONTRACT: browser-identity rule (same as `crate::activity` and
//! `crate::agent_client_provider`). Provider `thread.id`/`sessionId`, turn ids,
//! item ids, `thread.path`, `codexHome`, `installationId`, and raw event JSON
//! are transport-private. The projector takes provider ids only as correlation
//! inputs (delta<->item stitching) and never copies them into any output
//! `TranscriptBlock`/`CodexUsage`. Only projected/derived transcript content
//! crosses the boundary.
//!
//! CONTRACT: degrade without breaking the feed. An unknown Codex item `type`
//! becomes a single bounded diagnostic status block (never raw JSON/paths). An
//! unrecognized top-level notification `method` is silently ignored rather than
//! flooding the transcript with one diagnostic per protocol-control message
//! (rate limits, MCP startup status, remote-control status); the governing
//! rule from the ticket Constraints is "degrade without breaking the whole
//! Activity feed", and surfacing every control message as a diagnostic would
//! break the feed's usefulness. Malformed JSON lines degrade to one bounded
//! diagnostic block.

use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::activity::TranscriptBlock;

/// `render_kind` values this projector emits. All are additive members of the
/// open `TranscriptBlock::render_kind` vocabulary documented in
/// `crate::activity`; this module does not introduce a closed enum.
pub const CODEX_RENDER_KIND_MARKDOWN: &str = "markdown";
pub const CODEX_RENDER_KIND_THINKING: &str = "thinking";
pub const CODEX_RENDER_KIND_TOOL: &str = "tool";
pub const CODEX_RENDER_KIND_FILE_CHANGE: &str = "fileChange";
pub const CODEX_RENDER_KIND_STATUS: &str = "status";

const MAX_BLOCK_TEXT: usize = 8_192;
const MAX_DIAGNOSTIC_TEXT: usize = 280;

const DIAG_MALFORMED_RECORD: &str = "native transcript record malformed";
const DIAG_UNSUPPORTED_ITEM: &str = "unsupported activity item projected as status";

/// Read-only token-usage snapshot derived from `thread/tokenUsage/updated`.
/// Drives `activity.session.usage` display; never a transcript block. Carries
/// no ids or paths.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CodexUsage {
    pub used_total_tokens: Option<u64>,
    pub used_last_tokens: Option<u64>,
    pub context_window: Option<u64>,
}

/// Outcome of ingesting one classified server message, so the async transport
/// can decide which `ActivityConsoleEvent` (if any) to emit without re-deriving
/// projector state.
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum CodexIngestOutcome {
    /// A transcript block was created or updated for the item at this ordinal.
    BlockUpserted { ordinal: usize },
    /// Read-only usage state changed.
    UsageUpdated,
    /// A turn began; the transport tracks this for the `turn/steer`
    /// `expectedTurnId` guard. The turn id stays transport/projector-private.
    TurnStarted,
    /// A turn reached a terminal state.
    TurnCompleted,
    /// Thread status transitioned (idle/active).
    ThreadStatusChanged,
    /// A malformed or unsupported input degraded to a bounded diagnostic.
    Degraded,
    /// Recognized protocol-control message with no transcript effect.
    Ignored,
}

#[derive(Clone, Debug)]
struct BlockState {
    render_kind: String,
    title: Option<String>,
    text: String,
    degraded: bool,
    // `260713-bug-dashboard-agent-chat-transcript-role-turnid-echo` Phase 2:
    // derived purely from which `upsert_block`/`ensure_block` call site
    // handles the item (see `ingest_item`); never set from
    // `suppress_local_prompt` state. Left unset for `reasoning`/`fileChange`/
    // `plan`/`contextCompaction`/unsupported items per the ticket's role
    // vocabulary.
    role: Option<String>,
}

/// Stateful, runtime-free projector. Feed it classified server messages in
/// arrival order; read `transcript_blocks()`/`usage()` for browser-facing
/// output.
#[derive(Clone, Debug, Default)]
pub struct CodexProjector {
    order: Vec<String>,
    blocks: BTreeMap<String, BlockState>,
    suppressed_prompts: Vec<String>,
    suppressed_item_ids: Vec<String>,
    usage: Option<CodexUsage>,
    diagnostics: Vec<String>,
    seen_unsupported_types: Vec<String>,
    active_turn: bool,
    thread_status: Option<String>,
    turn_status: Option<String>,
    // Internal-only turn-id tracking for fork-from-here cut-point resolution.
    // Never copied into `TranscriptBlock` (see module CONTRACT); provider turn
    // ids stay correlation-only. `current_turn_id` is the turn the projector
    // is currently ingesting items under (set from `turn/started`);
    // `order_turn_ids` is a parallel array to `order`, recording which turn
    // each transcript-order item belongs to.
    current_turn_id: Option<String>,
    order_turn_ids: Vec<Option<String>>,
}

impl CodexProjector {
    pub fn new() -> Self {
        Self::default()
    }

    /// Build a projector pre-populated with already-projected transcript
    /// blocks (typically `project_fork_turns`'s output), so a forked
    /// session's projector starts non-empty and newly ingested live items
    /// append right after the seeded history. Seeded blocks carry no
    /// provider item id (there is none left to correlate against, since
    /// `project_fork_turns` already discarded it) and no turn id (turn-id
    /// resolution for a fork-of-a-fork is out of this phase's scope).
    pub fn seeded(blocks: Vec<TranscriptBlock>) -> Self {
        let mut projector = Self::default();
        for (index, block) in blocks.into_iter().enumerate() {
            let synthetic_id = format!("seed-{index}");
            projector.order.push(synthetic_id.clone());
            projector.order_turn_ids.push(None);
            projector.blocks.insert(
                synthetic_id,
                BlockState {
                    render_kind: block.render_kind,
                    title: block.title,
                    text: block.text.unwrap_or_default(),
                    degraded: block.degraded,
                    // `role` carries through a fork replay (a forked
                    // user/agent/tool block keeps the role it had when
                    // originally projected); `turn_id` does not (see
                    // `order_turn_ids.push(None)` above -- fork-of-a-fork
                    // turn-id resolution is out of this phase's scope).
                    role: block.role,
                },
            );
        }
        projector
    }

    /// Register a prompt the browser just sent locally so its echoed
    /// `userMessage` item is not double-rendered. One registration suppresses
    /// one matching echo.
    ///
    /// SETTLED (`260713-bug-dashboard-agent-chat-transcript-role-turnid-echo`
    /// Phase 2): do not remove or weaken this suppression. Phase 2's
    /// role/turn_id additions are purely additive and independent of it --
    /// fork/resume never calls `send_prompt`, so its seeded projector carries
    /// no suppression state, making suppression irrelevant to that ticket.
    /// Removing it to "complete" a future role/turn_id-adjacent ticket would
    /// reintroduce a previously-identified double-render risk that already
    /// blocked design review once.
    pub fn suppress_local_prompt(&mut self, text: impl Into<String>) {
        self.suppressed_prompts.push(text.into().trim().to_owned());
    }

    pub fn usage(&self) -> Option<CodexUsage> {
        self.usage
    }

    pub fn thread_status(&self) -> Option<&str> {
        self.thread_status.as_deref()
    }

    pub fn turn_status(&self) -> Option<&str> {
        self.turn_status.as_deref()
    }

    pub fn is_turn_active(&self) -> bool {
        self.active_turn
    }

    pub fn degraded(&self) -> bool {
        self.blocks.values().any(|block| block.degraded) || !self.diagnostics.is_empty()
    }

    /// Bounded, deduplicated diagnostics surfaced alongside the transcript.
    pub fn diagnostics(&self) -> &[String] {
        &self.diagnostics
    }

    /// Resolve a browser-facing ordinal `cursor` (as produced by
    /// `transcript_blocks`) to the provider turn id that item belongs to, for
    /// the fork-from-here `cutCursor` -> `lastTurnId` translation. Returns
    /// `None` if the cursor does not parse, is out of range, or the item was
    /// ingested before any `turn/started` was observed.
    pub fn turn_id_for_cursor(&self, cursor: &str) -> Option<String> {
        let index: usize = cursor.parse().ok()?;
        self.order_turn_ids.get(index)?.clone()
    }

    /// Browser-facing ordered transcript. Cursors are ordinal positions, not
    /// provider ids.
    pub fn transcript_blocks(&self) -> Vec<TranscriptBlock> {
        self.order
            .iter()
            .enumerate()
            .filter_map(|(index, item_id)| {
                let block = self.blocks.get(item_id)?;
                Some(TranscriptBlock {
                    cursor: index.to_string(),
                    timestamp: None,
                    render_kind: block.render_kind.clone(),
                    title: block.title.clone(),
                    text: (!block.text.is_empty()).then(|| block.text.clone()),
                    data: None,
                    degraded: block.degraded,
                    role: block.role.clone(),
                    turn_id: self.order_turn_ids.get(index)?.clone(),
                })
            })
            .collect()
    }

    /// Parse and dispatch one raw NDJSON line. Malformed JSON degrades to a
    /// bounded diagnostic. Responses to our own requests and server-initiated
    /// requests (approvals) carry no transcript content and are ignored here;
    /// the transport classifies and routes them (see `codex_app_server`).
    pub fn ingest_line(&mut self, line: &str) -> CodexIngestOutcome {
        if line.trim().is_empty() {
            return CodexIngestOutcome::Ignored;
        }
        match serde_json::from_str::<Value>(line) {
            Ok(value) => self.ingest_value(&value),
            Err(_) => {
                self.push_diagnostic(DIAG_MALFORMED_RECORD);
                CodexIngestOutcome::Degraded
            }
        }
    }

    /// Dispatch one parsed JSON-RPC message. Only notifications
    /// (`method` present, `id` absent) carry transcript content.
    pub fn ingest_value(&mut self, value: &Value) -> CodexIngestOutcome {
        let has_id = value.get("id").is_some();
        let method = value.get("method").and_then(Value::as_str);
        match (method, has_id) {
            (Some(method), false) => {
                let params = value.get("params").unwrap_or(&Value::Null);
                self.ingest_notification(method, params)
            }
            // Responses to our requests and server-initiated requests are
            // transport concerns, not transcript content.
            _ => CodexIngestOutcome::Ignored,
        }
    }

    fn ingest_notification(&mut self, method: &str, params: &Value) -> CodexIngestOutcome {
        match method {
            "item/started" | "item/completed" => self.ingest_item(params),
            "item/agentMessage/delta" => {
                self.ingest_text_delta(params, CODEX_RENDER_KIND_MARKDOWN, "Assistant", Some("agent"))
            }
            "item/reasoning/textDelta" | "item/reasoning/summaryTextDelta" => {
                self.ingest_text_delta(params, CODEX_RENDER_KIND_THINKING, "Reasoning", None)
            }
            "item/commandExecution/outputDelta" => {
                self.ingest_text_delta(params, CODEX_RENDER_KIND_TOOL, "Command", Some("tool"))
            }
            "thread/tokenUsage/updated" => self.ingest_usage(params),
            "thread/status/changed" => {
                self.thread_status = params
                    .get("status")
                    .and_then(|status| status.get("type"))
                    .and_then(Value::as_str)
                    .map(str::to_owned);
                CodexIngestOutcome::ThreadStatusChanged
            }
            "turn/started" => {
                self.active_turn = true;
                self.turn_status = Some("inProgress".to_owned());
                self.current_turn_id = params
                    .get("turn")
                    .and_then(|turn| turn.get("id"))
                    .and_then(Value::as_str)
                    .map(str::to_owned);
                CodexIngestOutcome::TurnStarted
            }
            "turn/completed" | "turn/failed" | "turn/aborted" | "turn/interrupted" => {
                self.active_turn = false;
                self.turn_status = params
                    .get("turn")
                    .and_then(|turn| turn.get("status"))
                    .and_then(Value::as_str)
                    .map(str::to_owned)
                    .or_else(|| Some("completed".to_owned()));
                CodexIngestOutcome::TurnCompleted
            }
            // Recognized protocol-control / not-yet-projected notifications.
            // Silently ignored (see module CONTRACT) rather than flooding the
            // transcript with diagnostics.
            _ => CodexIngestOutcome::Ignored,
        }
    }

    fn ingest_item(&mut self, params: &Value) -> CodexIngestOutcome {
        let Some(item) = params.get("item") else {
            return CodexIngestOutcome::Ignored;
        };
        let Some(item_id) = item.get("id").and_then(Value::as_str) else {
            return CodexIngestOutcome::Ignored;
        };
        let item_type = item.get("type").and_then(Value::as_str).unwrap_or("");

        match item_type {
            "userMessage" | "hookPrompt" => {
                let text = extract_content_text(item);
                // The same userMessage item arrives as item/started then
                // item/completed; suppress both once matched, keyed by item id.
                // SETTLED: this suppression check stays untouched by Phase 2's
                // role/turn_id work (see `suppress_local_prompt`'s doc comment
                // -- fork/resume never calls `send_prompt`, so suppression is
                // orthogonal to this ticket's additive metadata).
                if self.suppressed_item_ids.iter().any(|id| id == item_id)
                    || self.take_suppressed_prompt(&text)
                {
                    if !self.suppressed_item_ids.iter().any(|id| id == item_id) {
                        self.suppressed_item_ids.push(item_id.to_owned());
                    }
                    return CodexIngestOutcome::Ignored;
                }
                self.upsert_block(item_id, CODEX_RENDER_KIND_MARKDOWN, "User", &text, false, Some("user"))
            }
            "agentMessage" => {
                let text = item.get("text").and_then(Value::as_str).unwrap_or_default();
                self.upsert_block(item_id, CODEX_RENDER_KIND_MARKDOWN, "Assistant", text, false, Some("agent"))
            }
            "reasoning" => {
                let text = reasoning_text(item);
                self.upsert_block(item_id, CODEX_RENDER_KIND_THINKING, "Reasoning", &text, false, None)
            }
            "commandExecution" => {
                let summary = command_summary(item);
                self.upsert_block(item_id, CODEX_RENDER_KIND_TOOL, "Command", &summary, false, Some("tool"))
            }
            "mcpToolCall" | "dynamicToolCall" | "collabAgentToolCall" => {
                let summary = tool_summary(item);
                self.upsert_block(item_id, CODEX_RENDER_KIND_TOOL, "Tool", &summary, false, Some("tool"))
            }
            "fileChange" => {
                // CONTRACT: never leak absolute host paths. Only a bounded,
                // path-free change summary crosses the boundary in this phase.
                let summary = file_change_summary(item);
                self.upsert_block(
                    item_id,
                    CODEX_RENDER_KIND_FILE_CHANGE,
                    "File change",
                    &summary,
                    false,
                    None,
                )
            }
            "plan" | "contextCompaction" => {
                let text = item
                    .get("text")
                    .and_then(Value::as_str)
                    .unwrap_or("")
                    .to_owned();
                let title = if item_type == "plan" {
                    "Plan"
                } else {
                    "Context compaction"
                };
                self.upsert_block(item_id, CODEX_RENDER_KIND_STATUS, title, &text, false, None)
            }
            other => {
                // Unknown item type: one bounded diagnostic status block per
                // distinct type, carrying only the type name (never raw JSON).
                if !self.seen_unsupported_types.iter().any(|seen| seen == other) {
                    self.seen_unsupported_types.push(other.to_owned());
                    self.push_diagnostic(DIAG_UNSUPPORTED_ITEM);
                }
                let text = bound_text(
                    &format!("Unsupported Codex activity item type: {other}"),
                    MAX_DIAGNOSTIC_TEXT,
                );
                self.upsert_block(
                    item_id,
                    CODEX_RENDER_KIND_STATUS,
                    "Unsupported activity",
                    &text,
                    true,
                    None,
                );
                CodexIngestOutcome::Degraded
            }
        }
    }

    fn ingest_text_delta(
        &mut self,
        params: &Value,
        render_kind: &str,
        title: &str,
        role: Option<&str>,
    ) -> CodexIngestOutcome {
        let Some(item_id) = params.get("itemId").and_then(Value::as_str) else {
            return CodexIngestOutcome::Ignored;
        };
        let delta = params.get("delta").and_then(Value::as_str).unwrap_or("");
        let ordinal = self.ensure_block(item_id, render_kind, title, role);
        if let Some(block) = self.blocks.get_mut(item_id) {
            block.text.push_str(delta);
            block.text = bound_text(&block.text, MAX_BLOCK_TEXT);
        }
        CodexIngestOutcome::BlockUpserted { ordinal }
    }

    fn ingest_usage(&mut self, params: &Value) -> CodexIngestOutcome {
        let usage = params.get("tokenUsage");
        let used_total = usage
            .and_then(|usage| usage.get("total"))
            .and_then(|total| total.get("totalTokens"))
            .and_then(Value::as_u64);
        let used_last = usage
            .and_then(|usage| usage.get("last"))
            .and_then(|last| last.get("totalTokens"))
            .and_then(Value::as_u64);
        let context_window = usage
            .and_then(|usage| usage.get("modelContextWindow"))
            .and_then(Value::as_u64);
        self.usage = Some(CodexUsage {
            used_total_tokens: used_total,
            used_last_tokens: used_last,
            context_window,
        });
        CodexIngestOutcome::UsageUpdated
    }

    fn upsert_block(
        &mut self,
        item_id: &str,
        render_kind: &str,
        title: &str,
        text: &str,
        degraded: bool,
        role: Option<&str>,
    ) -> CodexIngestOutcome {
        let ordinal = self.ensure_block(item_id, render_kind, title, role);
        if let Some(block) = self.blocks.get_mut(item_id) {
            block.render_kind = render_kind.to_owned();
            block.title = Some(title.to_owned());
            // item/completed is authoritative: replace accumulated deltas with
            // the final snapshot text when the snapshot is non-empty.
            if !text.is_empty() {
                block.text = bound_text(text, MAX_BLOCK_TEXT);
            }
            block.degraded = block.degraded || degraded;
            block.role = role.map(str::to_owned);
        }
        CodexIngestOutcome::BlockUpserted { ordinal }
    }

    fn ensure_block(
        &mut self,
        item_id: &str,
        render_kind: &str,
        title: &str,
        role: Option<&str>,
    ) -> usize {
        if let Some(index) = self.order.iter().position(|id| id == item_id) {
            return index;
        }
        self.order.push(item_id.to_owned());
        self.order_turn_ids.push(self.current_turn_id.clone());
        self.blocks.insert(
            item_id.to_owned(),
            BlockState {
                render_kind: render_kind.to_owned(),
                title: Some(title.to_owned()),
                text: String::new(),
                degraded: false,
                role: role.map(str::to_owned),
            },
        );
        self.order.len() - 1
    }

    fn take_suppressed_prompt(&mut self, text: &str) -> bool {
        let trimmed = text.trim();
        if let Some(index) = self
            .suppressed_prompts
            .iter()
            .position(|prompt| prompt == trimmed)
        {
            self.suppressed_prompts.remove(index);
            true
        } else {
            false
        }
    }

    fn push_diagnostic(&mut self, diagnostic: &str) {
        if !self.diagnostics.iter().any(|seen| seen == diagnostic) {
            self.diagnostics.push(diagnostic.to_owned());
        }
    }
}

fn extract_content_text(item: &Value) -> String {
    let mut text = String::new();
    if let Some(content) = item.get("content").and_then(Value::as_array) {
        for part in content {
            if let Some(part_text) = part.get("text").and_then(Value::as_str) {
                if !text.is_empty() {
                    text.push('\n');
                }
                text.push_str(part_text);
            }
        }
    } else if let Some(direct) = item.get("text").and_then(Value::as_str) {
        text.push_str(direct);
    }
    bound_text(&text, MAX_BLOCK_TEXT)
}

fn reasoning_text(item: &Value) -> String {
    if let Some(text) = item.get("text").and_then(Value::as_str) {
        return bound_text(text, MAX_BLOCK_TEXT);
    }
    if let Some(summary) = item.get("summary").and_then(Value::as_str) {
        return bound_text(summary, MAX_BLOCK_TEXT);
    }
    extract_content_text(item)
}

fn command_summary(item: &Value) -> String {
    let command = match item.get("command") {
        Some(Value::String(command)) => command.clone(),
        Some(Value::Array(parts)) => parts
            .iter()
            .filter_map(Value::as_str)
            .collect::<Vec<_>>()
            .join(" "),
        _ => String::new(),
    };
    if command.is_empty() {
        "command execution".to_owned()
    } else {
        bound_text(&format!("$ {command}"), MAX_DIAGNOSTIC_TEXT)
    }
}

fn tool_summary(item: &Value) -> String {
    let name = item
        .get("name")
        .or_else(|| item.get("tool"))
        .or_else(|| item.get("server"))
        .and_then(Value::as_str)
        .unwrap_or("tool call");
    bound_text(name, MAX_DIAGNOSTIC_TEXT)
}

fn file_change_summary(item: &Value) -> String {
    let count = item
        .get("changes")
        .and_then(Value::as_array)
        .map(|changes| changes.len())
        .or_else(|| {
            item.get("files")
                .and_then(Value::as_array)
                .map(|files| files.len())
        });
    match count {
        Some(count) => format!("{count} file change(s)"),
        None => "file change".to_owned(),
    }
}

fn bound_text(text: &str, max: usize) -> String {
    if text.len() <= max {
        return text.to_owned();
    }
    let mut end = max;
    while end > 0 && !text.is_char_boundary(end) {
        end -= 1;
    }
    let mut bounded = text[..end].to_owned();
    bounded.push('…');
    bounded
}

/// Map one already-assembled Codex `ThreadItem` (from `thread/fork`'s
/// response `thread.turns[].items[]`, not the live `item/started`/
/// `item/completed` delta stream) into a `(render_kind, title, text,
/// degraded, role)` tuple, mirroring `ingest_item`'s per-type mapping
/// (including its `role` vocabulary). Unlike `ingest_item` this is a pure,
/// stateless function: fork-seeded items are a one-shot snapshot, not live
/// echoes, so there is no suppression/dedup state to consult.
fn classify_thread_item(item: &Value) -> (&'static str, &'static str, String, bool, Option<&'static str>) {
    let item_type = item.get("type").and_then(Value::as_str).unwrap_or("");
    match item_type {
        "userMessage" | "hookPrompt" => (
            CODEX_RENDER_KIND_MARKDOWN,
            "User",
            extract_content_text(item),
            false,
            Some("user"),
        ),
        "agentMessage" => {
            let text = item.get("text").and_then(Value::as_str).unwrap_or_default();
            (
                CODEX_RENDER_KIND_MARKDOWN,
                "Assistant",
                bound_text(text, MAX_BLOCK_TEXT),
                false,
                Some("agent"),
            )
        }
        "reasoning" => (
            CODEX_RENDER_KIND_THINKING,
            "Reasoning",
            reasoning_text(item),
            false,
            None,
        ),
        "commandExecution" => (
            CODEX_RENDER_KIND_TOOL,
            "Command",
            command_summary(item),
            false,
            Some("tool"),
        ),
        "mcpToolCall" | "dynamicToolCall" | "collabAgentToolCall" => {
            (CODEX_RENDER_KIND_TOOL, "Tool", tool_summary(item), false, Some("tool"))
        }
        "fileChange" => (
            CODEX_RENDER_KIND_FILE_CHANGE,
            "File change",
            file_change_summary(item),
            false,
            None,
        ),
        "plan" => (
            CODEX_RENDER_KIND_STATUS,
            "Plan",
            item.get("text").and_then(Value::as_str).unwrap_or("").to_owned(),
            false,
            None,
        ),
        "contextCompaction" => (
            CODEX_RENDER_KIND_STATUS,
            "Context compaction",
            item.get("text").and_then(Value::as_str).unwrap_or("").to_owned(),
            false,
            None,
        ),
        other => {
            let text = bound_text(
                &format!("Unsupported Codex activity item type: {other}"),
                MAX_DIAGNOSTIC_TEXT,
            );
            (CODEX_RENDER_KIND_STATUS, "Unsupported activity", text, true, None)
        }
    }
}

/// Pure projection of a `thread/fork` response's `thread` object into the same
/// browser-facing `TranscriptBlock` shape/render-kinds `CodexProjector`
/// produces from the live event stream, so a forked session's seeded
/// transcript looks identical in the browser to one built up live. Mirrors
/// `project_skills_list`'s "project a raw JSON-RPC response" pattern but
/// produces `TranscriptBlock`s, so it stays colocated here rather than in
/// `codex_app_server.rs`.
pub fn project_fork_turns(thread: &Value) -> Vec<TranscriptBlock> {
    let turns = thread
        .get("turns")
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[]);
    let mut blocks = Vec::new();
    for turn in turns {
        // `turn_id` here is the provider's own turn id, used only as a
        // browser-side bubble-merge-equality key (see `TranscriptBlock::
        // turn_id`'s CONTRACT in `crate::activity`) -- distinct from this
        // module's general "provider ids stay correlation-only, never copied
        // into `TranscriptBlock`" rule, which governs `order_turn_ids`'s
        // internal fork-cut-point use, not this ticket-approved wire field.
        let turn_id = turn.get("id").and_then(Value::as_str).map(str::to_owned);
        let items = turn
            .get("items")
            .and_then(Value::as_array)
            .map(Vec::as_slice)
            .unwrap_or(&[]);
        for item in items {
            let (render_kind, title, text, degraded, role) = classify_thread_item(item);
            let cursor = blocks.len().to_string();
            blocks.push(TranscriptBlock {
                cursor,
                timestamp: None,
                render_kind: render_kind.to_owned(),
                title: Some(title.to_owned()),
                text: (!text.is_empty()).then_some(text),
                data: None,
                degraded,
                role: role.map(str::to_owned),
                turn_id: turn_id.clone(),
            });
        }
    }
    blocks
}

#[cfg(test)]
mod tests {
    use super::*;

    const TURN_FIXTURE: &str = include_str!("../tests/fixtures/codex-app-server-turn.ndjson");

    fn project_fixture(suppress: Option<&str>) -> CodexProjector {
        let mut projector = CodexProjector::new();
        if let Some(prompt) = suppress {
            projector.suppress_local_prompt(prompt);
        }
        for line in TURN_FIXTURE.lines() {
            projector.ingest_line(line);
        }
        projector
    }

    #[test]
    fn projects_real_turn_fixture_to_ordered_blocks() {
        let projector = project_fixture(None);
        let blocks = projector.transcript_blocks();
        // Real turn: one echoed userMessage + one agentMessage.
        assert_eq!(blocks.len(), 2, "unexpected blocks: {blocks:#?}");
        assert_eq!(blocks[0].render_kind, CODEX_RENDER_KIND_MARKDOWN);
        assert_eq!(blocks[0].title.as_deref(), Some("User"));
        assert_eq!(blocks[1].title.as_deref(), Some("Assistant"));
        // agentMessage deltas ("HEL"+"LO") fold, and item/completed overrides
        // with the authoritative "HELLO" snapshot.
        assert_eq!(blocks[1].text.as_deref(), Some("HELLO"));
        assert_eq!(blocks[0].cursor, "0");
        assert_eq!(blocks[1].cursor, "1");
        assert!(!projector.degraded());
        // `260713` Phase 2: role assigned per item type.
        assert_eq!(blocks[0].role.as_deref(), Some("user"));
        assert_eq!(blocks[1].role.as_deref(), Some("agent"));
    }

    #[test]
    fn suppresses_browser_prompt_echo() {
        let projector =
            project_fixture(Some("reply with exactly the single word HELLO and nothing else"));
        let blocks = projector.transcript_blocks();
        assert_eq!(blocks.len(), 1, "echo not suppressed: {blocks:#?}");
        assert_eq!(blocks[0].title.as_deref(), Some("Assistant"));
        assert_eq!(blocks[0].text.as_deref(), Some("HELLO"));
    }

    #[test]
    fn reads_token_usage_from_fixture() {
        let projector = project_fixture(None);
        let usage = projector.usage().expect("usage present");
        assert_eq!(usage.used_total_tokens, Some(16733));
        assert_eq!(usage.context_window, Some(353400));
    }

    #[test]
    fn folds_agent_message_deltas_before_completion() {
        // Deltas only (no item/completed) still fold into one assistant block.
        let mut projector = CodexProjector::new();
        projector.ingest_line(
            r#"{"method":"item/started","params":{"item":{"type":"agentMessage","id":"m1","text":""}}}"#,
        );
        projector.ingest_line(
            r#"{"method":"item/agentMessage/delta","params":{"itemId":"m1","delta":"foo "}}"#,
        );
        projector.ingest_line(
            r#"{"method":"item/agentMessage/delta","params":{"itemId":"m1","delta":"bar"}}"#,
        );
        let blocks = projector.transcript_blocks();
        assert_eq!(blocks.len(), 1);
        assert_eq!(blocks[0].text.as_deref(), Some("foo bar"));
    }

    #[test]
    fn emits_thinking_block_for_reasoning_item() {
        let mut projector = CodexProjector::new();
        projector.ingest_line(
            r#"{"method":"item/started","params":{"item":{"type":"reasoning","id":"r1","text":""}}}"#,
        );
        projector.ingest_line(
            r#"{"method":"item/reasoning/textDelta","params":{"itemId":"r1","delta":"weighing options"}}"#,
        );
        let blocks = projector.transcript_blocks();
        assert_eq!(blocks.len(), 1);
        assert_eq!(blocks[0].render_kind, CODEX_RENDER_KIND_THINKING);
        assert_eq!(blocks[0].title.as_deref(), Some("Reasoning"));
        assert_eq!(blocks[0].text.as_deref(), Some("weighing options"));
        // `260713` Phase 2: thinking/reasoning blocks leave role unset.
        assert_eq!(blocks[0].role, None);
    }

    #[test]
    fn degrades_unknown_item_type_to_bounded_diagnostic_without_leaking_json() {
        let mut projector = CodexProjector::new();
        let outcome = projector.ingest_line(
            r#"{"method":"item/completed","params":{"item":{"type":"holographicMemory","id":"x1","secretPath":"/home/user/.codex/sessions/leak.jsonl","payload":{"raw":"do-not-leak"}}}}"#,
        );
        assert_eq!(outcome, CodexIngestOutcome::Degraded);
        let blocks = projector.transcript_blocks();
        assert_eq!(blocks.len(), 1);
        assert!(blocks[0].degraded);
        assert_eq!(blocks[0].render_kind, CODEX_RENDER_KIND_STATUS);
        let text = blocks[0].text.as_deref().unwrap_or_default();
        assert!(text.contains("holographicMemory"));
        assert!(!text.contains("leak"));
        assert!(!text.contains("do-not-leak"));
        assert!(projector.degraded());

        // Full serialized projection must not carry the raw path/payload.
        let serialized = serde_json::to_string(&blocks).expect("serialize blocks");
        assert!(!serialized.contains("secretPath"));
        assert!(!serialized.contains("/home/user/.codex"));
        assert!(!serialized.contains("do-not-leak"));
    }

    #[test]
    fn malformed_line_degrades_without_panicking() {
        let mut projector = CodexProjector::new();
        let outcome = projector.ingest_line("{not json");
        assert_eq!(outcome, CodexIngestOutcome::Degraded);
        assert!(projector.diagnostics().contains(&DIAG_MALFORMED_RECORD.to_owned()));
    }

    #[test]
    fn ignores_protocol_control_notifications() {
        let mut projector = CodexProjector::new();
        for line in [
            r#"{"method":"account/rateLimits/updated","params":{"rateLimits":{}}}"#,
            r#"{"method":"mcpServer/startupStatus/updated","params":{"name":"wsflow","status":"ready"}}"#,
            r#"{"id":7,"result":{"thread":{"id":"secret","path":"/home/x/.codex/y"}}}"#,
        ] {
            assert_eq!(projector.ingest_line(line), CodexIngestOutcome::Ignored);
        }
        assert!(projector.transcript_blocks().is_empty());
        assert!(!projector.degraded());
    }

    #[test]
    fn tracks_turn_lifecycle_for_steer_guard() {
        let mut projector = CodexProjector::new();
        assert!(!projector.is_turn_active());
        projector.ingest_line(r#"{"method":"turn/started","params":{"turn":{"id":"t1"}}}"#);
        assert!(projector.is_turn_active());
        projector.ingest_line(
            r#"{"method":"turn/completed","params":{"turn":{"id":"t1","status":"completed"}}}"#,
        );
        assert!(!projector.is_turn_active());
        assert_eq!(projector.turn_status(), Some("completed"));
    }

    #[test]
    fn turn_id_for_cursor_maps_order_index_across_multiple_turns() {
        let mut projector = CodexProjector::new();
        // Item ingested before any turn/started has no turn id.
        projector.ingest_line(
            r#"{"method":"item/completed","params":{"item":{"type":"userMessage","id":"u0","text":"hi"}}}"#,
        );
        projector.ingest_line(r#"{"method":"turn/started","params":{"turn":{"id":"t1"}}}"#);
        projector.ingest_line(
            r#"{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"a1","text":"first"}}}"#,
        );
        projector.ingest_line(
            r#"{"method":"turn/completed","params":{"turn":{"id":"t1","status":"completed"}}}"#,
        );
        projector.ingest_line(r#"{"method":"turn/started","params":{"turn":{"id":"t2"}}}"#);
        projector.ingest_line(
            r#"{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"a2","text":"second"}}}"#,
        );

        let blocks = projector.transcript_blocks();
        assert_eq!(blocks.len(), 3);
        assert_eq!(projector.turn_id_for_cursor(&blocks[0].cursor), None);
        assert_eq!(
            projector.turn_id_for_cursor(&blocks[1].cursor),
            Some("t1".to_owned())
        );
        assert_eq!(
            projector.turn_id_for_cursor(&blocks[2].cursor),
            Some("t2".to_owned())
        );
        assert_eq!(projector.turn_id_for_cursor("not-a-number"), None);
        assert_eq!(projector.turn_id_for_cursor("999"), None);

        // `260713` Phase 2: the wire `turn_id` field (via `transcript_blocks`)
        // mirrors `turn_id_for_cursor`'s internal correlation exactly, so
        // multi-block same-turn items can merge into one bubble browser-side.
        assert_eq!(blocks[0].turn_id, None);
        assert_eq!(blocks[1].turn_id.as_deref(), Some("t1"));
        assert_eq!(blocks[2].turn_id.as_deref(), Some("t2"));
        assert_eq!(blocks[0].role.as_deref(), Some("user"));
        assert_eq!(blocks[1].role.as_deref(), Some("agent"));
        assert_eq!(blocks[2].role.as_deref(), Some("agent"));
    }

    #[test]
    fn project_fork_turns_maps_thread_fork_response_to_transcript_blocks() {
        let thread = serde_json::json!({
            "id": "thread-secret",
            "turns": [
                {
                    "id": "t1",
                    "items": [
                        { "type": "userMessage", "id": "u1", "text": "hello" },
                        { "type": "agentMessage", "id": "a1", "text": "HELLO" },
                    ],
                },
                {
                    "id": "t2",
                    "items": [
                        { "type": "unknownFutureItem", "id": "x1" },
                    ],
                },
            ],
        });
        let blocks = project_fork_turns(&thread);
        assert_eq!(blocks.len(), 3);
        assert_eq!(blocks[0].cursor, "0");
        assert_eq!(blocks[0].render_kind, CODEX_RENDER_KIND_MARKDOWN);
        assert_eq!(blocks[0].title.as_deref(), Some("User"));
        assert_eq!(blocks[0].text.as_deref(), Some("hello"));
        assert_eq!(blocks[1].cursor, "1");
        assert_eq!(blocks[1].title.as_deref(), Some("Assistant"));
        assert_eq!(blocks[1].text.as_deref(), Some("HELLO"));
        assert_eq!(blocks[2].cursor, "2");
        assert_eq!(blocks[2].render_kind, CODEX_RENDER_KIND_STATUS);
        assert!(blocks[2].degraded);
        let text = blocks[2].text.as_deref().unwrap_or_default();
        assert!(text.contains("unknownFutureItem"));

        // `260713` Phase 2: role assigned per item type; `turn_id` groups the
        // two items from turn "t1" and separates the "t2" item, for
        // browser-side bubble-merge equality. `turn_id` is explicitly a
        // ticket-approved exception to the "provider ids never cross the
        // boundary" rule below (see `project_fork_turns`'s inline comment) --
        // it is the provider's own turn id, used only as an opaque merge key.
        assert_eq!(blocks[0].role.as_deref(), Some("user"));
        assert_eq!(blocks[1].role.as_deref(), Some("agent"));
        assert_eq!(blocks[2].role, None);
        assert_eq!(blocks[0].turn_id.as_deref(), Some("t1"));
        assert_eq!(blocks[1].turn_id.as_deref(), Some("t1"));
        assert_eq!(blocks[2].turn_id.as_deref(), Some("t2"));

        // No provider *item*/*thread* ids leak into the projected blocks
        // (only `turn_id` carries a provider-originated id, and only for its
        // ticket-approved bubble-merge purpose).
        let serialized = serde_json::to_string(&blocks).expect("serialize blocks");
        assert!(!serialized.contains("thread-secret"));
        assert!(!serialized.contains("\"u1\""));
    }

    #[test]
    fn seeded_projector_replays_forked_blocks_then_continues_live() {
        let thread = serde_json::json!({
            "id": "thread-secret",
            "turns": [{
                "id": "t1",
                "items": [{ "type": "agentMessage", "id": "a1", "text": "seeded" }],
            }],
        });
        let mut projector = CodexProjector::seeded(project_fork_turns(&thread));
        assert_eq!(projector.transcript_blocks().len(), 1);
        assert_eq!(
            projector.transcript_blocks()[0].text.as_deref(),
            Some("seeded")
        );
        // `260713` Phase 2: `role` carries through `seeded()` (a forked
        // agent block keeps the role it had when originally projected).
        assert_eq!(
            projector.transcript_blocks()[0].role.as_deref(),
            Some("agent")
        );
        // `turn_id` does NOT carry through `seeded()` -- fork-of-a-fork
        // turn-id resolution is out of this phase's scope (see `seeded()`'s
        // doc comment).
        assert_eq!(projector.transcript_blocks()[0].turn_id, None);

        projector.ingest_line(
            r#"{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"live1","text":"continued"}}}"#,
        );
        let blocks = projector.transcript_blocks();
        assert_eq!(blocks.len(), 2);
        assert_eq!(blocks[0].cursor, "0");
        assert_eq!(blocks[1].cursor, "1");
        assert_eq!(blocks[1].text.as_deref(), Some("continued"));
        assert_eq!(blocks[1].role.as_deref(), Some("agent"));
    }
}
