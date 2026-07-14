//! Pure, synchronous Claude CLI stream-json event projection.
//!
//! Phase 4 of `260620-feat-ws-dashboard-agent-client-activity-sources` mirrors
//! the Phase 2 Codex split: a runtime-free projection mapper (this module) and
//! an async transport/session owner (the daemon's `claude_cli`). This module
//! consumes the classified `claude -p --input-format stream-json
//! --output-format stream-json` NDJSON event stream and produces the
//! browser-facing `crate::activity::TranscriptBlock`s plus read-only usage
//! state. It has no I/O, no runtime, and no knowledge of process spawning, so
//! it is exhaustively fixture-testable.
//!
//! CONTRACT: browser-identity rule (same as `crate::activity`,
//! `crate::agent_client_provider`, and `crate::codex_projection`). Provider
//! `session_id`, `transcript_path`, `cwd`, `request_id`, `uuid`, model/tool
//! internal ids, and the `thinking` block's `signature` are transport-private.
//! The projector takes a `tool_use_id` only as a correlation input (tool call
//! <-> tool result stitching) and never copies it into any output
//! `TranscriptBlock`/`ClaudeUsage`; only the ordinal cursor position crosses.
//!
//! CONTRACT: degrade without breaking the feed. An unknown top-level `type` or
//! unknown content-block `type` becomes one bounded diagnostic status block
//! (never raw JSON/paths/ids/`signature`). `system` (init/status) and
//! `rate_limit_event` are recognized protocol-control and are silently
//! ignored, matching the Codex projector's "don't flood the transcript with
//! control-message diagnostics" rule. Malformed JSON lines degrade to one
//! bounded diagnostic block.
//!
//! CONTRACT (Finding A3): Claude does not echo the client's own sent user
//! prompt as an output event in plain stream-json mode (unlike Codex's
//! `userMessage` item echo), so this projector needs no prompt-echo
//! suppression path.
//!
//! CONTRACT (Finding A2/A3): `stream_event` token-level deltas
//! (`--include-partial-messages`) are an optional live-append path. This
//! projector does not fold them: the daemon transport does not request
//! `--include-partial-messages` by default, and every `assistant`/`user`
//! event already carries the complete content block for that message
//! (Finding A1/A3), so `stream_event` is treated as recognized
//! protocol-control and ignored here rather than partially/incorrectly
//! correlated by content-block index across messages.

use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::activity::TranscriptBlock;

/// `render_kind` values this projector emits. All are additive members of the
/// open `TranscriptBlock::render_kind` vocabulary documented in
/// `crate::activity` (the same vocabulary the Codex projector uses).
pub const CLAUDE_RENDER_KIND_MARKDOWN: &str = "markdown";
pub const CLAUDE_RENDER_KIND_THINKING: &str = "thinking";
pub const CLAUDE_RENDER_KIND_TOOL: &str = "tool";
pub const CLAUDE_RENDER_KIND_STATUS: &str = "status";

const MAX_BLOCK_TEXT: usize = 8_192;
const MAX_DIAGNOSTIC_TEXT: usize = 280;

const DIAG_MALFORMED_RECORD: &str = "native transcript record malformed";
const DIAG_UNSUPPORTED_EVENT: &str = "unsupported activity event projected as status";
const DIAG_UNSUPPORTED_CONTENT_BLOCK: &str = "unsupported content block projected as status";
const DIAG_PERMISSION_DENIED: &str = "tool call denied by permission policy";

/// Read-only token-usage snapshot derived from the turn-terminal `result`
/// event's `usage`/`modelUsage`. Drives `activity.session.usage` display;
/// never a transcript block. Carries no ids or paths.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ClaudeUsage {
    pub used_input_tokens: Option<u64>,
    pub used_output_tokens: Option<u64>,
    pub context_window: Option<u64>,
}

/// Outcome of ingesting one classified stream-json line, so the async
/// transport can decide which `ActivityConsoleEvent` (if any) to emit without
/// re-deriving projector state.
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum ClaudeIngestOutcome {
    /// A transcript block was created or updated at this ordinal.
    BlockUpserted { ordinal: usize },
    /// Read-only usage state changed.
    UsageUpdated,
    /// A turn began.
    TurnStarted,
    /// A turn reached a terminal state (the `result` event).
    TurnCompleted,
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
    // Tool-only lifecycle status ("running"/"completed"/"failed"/"blocked"),
    // rendered as a bounded suffix line; `None` for non-tool blocks.
    status: Option<&'static str>,
    degraded: bool,
    // `260713-bug-dashboard-agent-chat-transcript-role-turnid-echo` Phase 2:
    // `role` is `"agent"` for text content blocks, `"tool"` for tool_use/
    // tool_result, and unset for `thinking` -- never `"user"` (see module
    // CONTRACT: Claude's stream-json protocol never echoes the client's own
    // prompt). `turn_id` is a daemon-synthesized per-turn-boundary counter
    // (Claude's protocol carries no per-turn id), captured at block-creation
    // time from `ClaudeProjector::current_turn_id`.
    role: Option<String>,
    turn_id: Option<String>,
}

/// Stateful, runtime-free projector. Feed it classified stream-json lines in
/// arrival order; read `transcript_blocks()`/`usage()` for browser-facing
/// output.
#[derive(Clone, Debug, Default)]
pub struct ClaudeProjector {
    order: Vec<String>,
    blocks: BTreeMap<String, BlockState>,
    usage: Option<ClaudeUsage>,
    diagnostics: Vec<String>,
    seen_unsupported_event_types: Vec<String>,
    seen_unsupported_block_types: Vec<String>,
    active_turn: bool,
    turn_status: Option<String>,
    next_content_seq: u64,
    // CONTRACT: `result.modelUsage` can carry more than one model (e.g. a
    // background helper model alongside the primary responding model, seen
    // in a real capture); track the model name the *assistant* actually used
    // this turn so `ingest_result` reads the right entry instead of an
    // arbitrary (alphabetically-first) one. Daemon-private, never surfaced.
    active_model: Option<String>,
    // `260713` Phase 2: Claude's stream-json protocol carries no per-turn id
    // (unlike Codex's `turn/started` `turn.id`), so `turn_id` is synthesized
    // here as a monotonically increasing per-turn-boundary counter. Advanced
    // exactly once per turn, at `ingest_assistant`'s existing `turn_started`
    // transition (before `active_turn` flips true); persists across all of
    // that turn's block creations, only advancing again at the next turn's
    // start (`ingest_result`'s `active_turn = false` intentionally does not
    // touch it).
    current_turn_seq: u64,
    current_turn_id: Option<String>,
}

impl ClaudeProjector {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn usage(&self) -> Option<ClaudeUsage> {
        self.usage
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

    /// Browser-facing ordered transcript. Cursors are ordinal positions, never
    /// a `tool_use_id` or other provider correlation id.
    pub fn transcript_blocks(&self) -> Vec<TranscriptBlock> {
        self.order
            .iter()
            .enumerate()
            .filter_map(|(index, key)| {
                let block = self.blocks.get(key)?;
                let text = match &block.status {
                    Some(status) if !block.text.is_empty() => {
                        Some(format!("{}\nstatus: {status}", block.text))
                    }
                    Some(status) => Some(format!("status: {status}")),
                    None => (!block.text.is_empty()).then(|| block.text.clone()),
                };
                Some(TranscriptBlock {
                    cursor: index.to_string(),
                    timestamp: None,
                    render_kind: block.render_kind.clone(),
                    title: block.title.clone(),
                    text,
                    data: None,
                    degraded: block.degraded,
                    role: block.role.clone(),
                    turn_id: block.turn_id.clone(),
                })
            })
            .collect()
    }

    /// Parse and dispatch one raw NDJSON line. Malformed JSON degrades to a
    /// bounded diagnostic.
    pub fn ingest_line(&mut self, line: &str) -> ClaudeIngestOutcome {
        if line.trim().is_empty() {
            return ClaudeIngestOutcome::Ignored;
        }
        match serde_json::from_str::<Value>(line) {
            Ok(value) => self.ingest_value(&value),
            Err(_) => {
                self.push_diagnostic(DIAG_MALFORMED_RECORD);
                ClaudeIngestOutcome::Degraded
            }
        }
    }

    /// Dispatch one parsed stream-json event, classified by its top-level
    /// `type` discriminator (Claude's output is typed events, not JSON-RPC).
    pub fn ingest_value(&mut self, value: &Value) -> ClaudeIngestOutcome {
        let Some(event_type) = value.get("type").and_then(Value::as_str) else {
            self.push_diagnostic(DIAG_MALFORMED_RECORD);
            return ClaudeIngestOutcome::Degraded;
        };
        match event_type {
            // Recognized protocol-control: per-turn metadata refresh, rate
            // limit status, control-channel plumbing, and (deferred, see
            // module CONTRACT) token-level partial-message deltas. None
            // carry transcript content in this phase.
            "system" | "rate_limit_event" | "control_request" | "control_response"
            | "stream_event" => ClaudeIngestOutcome::Ignored,
            "assistant" => self.ingest_assistant(value),
            "user" => self.ingest_user(value),
            "result" => self.ingest_result(value),
            other => self.ingest_unsupported_event(other),
        }
    }

    fn ingest_assistant(&mut self, value: &Value) -> ClaudeIngestOutcome {
        let turn_started = !self.active_turn;
        if turn_started {
            self.current_turn_seq += 1;
            self.current_turn_id = Some(format!("claude-turn-{}", self.current_turn_seq));
        }
        self.active_turn = true;
        if let Some(model) = value
            .get("message")
            .and_then(|message| message.get("model"))
            .and_then(Value::as_str)
        {
            self.active_model = Some(model.to_owned());
        }
        let Some(content) = value
            .get("message")
            .and_then(|message| message.get("content"))
            .and_then(Value::as_array)
        else {
            return if turn_started {
                ClaudeIngestOutcome::TurnStarted
            } else {
                ClaudeIngestOutcome::Ignored
            };
        };

        let mut last_outcome = if turn_started {
            ClaudeIngestOutcome::TurnStarted
        } else {
            ClaudeIngestOutcome::Ignored
        };
        for block in content {
            let block_type = block.get("type").and_then(Value::as_str).unwrap_or("");
            let outcome = match block_type {
                "text" => {
                    let text = block.get("text").and_then(Value::as_str).unwrap_or("");
                    self.push_block(CLAUDE_RENDER_KIND_MARKDOWN, "Assistant", text, None, false, Some("agent"))
                }
                "thinking" => {
                    // CONTRACT: `signature` is transport-private and is never
                    // read or forwarded.
                    let text = block.get("thinking").and_then(Value::as_str).unwrap_or("");
                    self.push_block(CLAUDE_RENDER_KIND_THINKING, "Reasoning", text, None, false, None)
                }
                "tool_use" => {
                    let Some(tool_use_id) = block.get("id").and_then(Value::as_str) else {
                        continue;
                    };
                    let name = block.get("name").and_then(Value::as_str).unwrap_or("tool");
                    let summary = tool_use_summary(name, block.get("input"));
                    self.upsert_tool_block(tool_use_id, name, &summary)
                }
                other => self.ingest_unsupported_block(other),
            };
            last_outcome = outcome;
        }
        last_outcome
    }

    fn ingest_user(&mut self, value: &Value) -> ClaudeIngestOutcome {
        let Some(content) = value
            .get("message")
            .and_then(|message| message.get("content"))
            .and_then(Value::as_array)
        else {
            return ClaudeIngestOutcome::Ignored;
        };
        let mut last_outcome = ClaudeIngestOutcome::Ignored;
        for block in content {
            if block.get("type").and_then(Value::as_str) != Some("tool_result") {
                continue;
            }
            let Some(tool_use_id) = block.get("tool_use_id").and_then(Value::as_str) else {
                continue;
            };
            let is_error = block
                .get("is_error")
                .and_then(Value::as_bool)
                .unwrap_or(false);
            let content_text = tool_result_text(block.get("content"));
            last_outcome = self.complete_tool_block(tool_use_id, is_error, &content_text);
        }
        last_outcome
    }

    fn ingest_result(&mut self, value: &Value) -> ClaudeIngestOutcome {
        self.active_turn = false;
        self.turn_status = value
            .get("subtype")
            .and_then(Value::as_str)
            .map(str::to_owned);

        let usage = value.get("usage");
        let used_input = usage
            .and_then(|usage| usage.get("input_tokens"))
            .and_then(Value::as_u64);
        let used_output = usage
            .and_then(|usage| usage.get("output_tokens"))
            .and_then(Value::as_u64);
        // CONTRACT: prefer the entry for the model that actually produced
        // this turn's assistant content; `modelUsage` can carry additional
        // background-helper models whose map key may sort first (Rust's
        // `serde_json::Map` is a `BTreeMap` without the `preserve_order`
        // feature), which would otherwise silently pick the wrong model's
        // context window (a real capture surfaced this: a background haiku
        // helper model recorded alongside the primary sonnet model).
        let context_window = value.get("modelUsage").and_then(Value::as_object).and_then(|models| {
            self.active_model
                .as_deref()
                .and_then(|model| models.get(model))
                .or_else(|| models.values().next())
                .and_then(|model_usage| model_usage.get("contextWindow"))
                .and_then(Value::as_u64)
        });
        self.usage = Some(ClaudeUsage {
            used_input_tokens: used_input,
            used_output_tokens: used_output,
            context_window,
        });

        if let Some(denials) = value.get("permission_denials").and_then(Value::as_array) {
            for denial in denials {
                if let Some(tool_use_id) = denial.get("tool_use_id").and_then(Value::as_str) {
                    if let Some(block) = self.blocks.get_mut(&tool_key(tool_use_id)) {
                        block.status = Some("blocked");
                    }
                }
                self.push_diagnostic(DIAG_PERMISSION_DENIED);
            }
        }

        ClaudeIngestOutcome::TurnCompleted
    }

    fn ingest_unsupported_event(&mut self, event_type: &str) -> ClaudeIngestOutcome {
        if !self
            .seen_unsupported_event_types
            .iter()
            .any(|seen| seen == event_type)
        {
            self.seen_unsupported_event_types
                .push(event_type.to_owned());
            self.push_diagnostic(DIAG_UNSUPPORTED_EVENT);
        }
        let text = bound_text(
            &format!("Unsupported Claude event type: {event_type}"),
            MAX_DIAGNOSTIC_TEXT,
        );
        self.push_block(CLAUDE_RENDER_KIND_STATUS, "Unsupported activity", &text, None, true, None);
        ClaudeIngestOutcome::Degraded
    }

    fn ingest_unsupported_block(&mut self, block_type: &str) -> ClaudeIngestOutcome {
        if !self
            .seen_unsupported_block_types
            .iter()
            .any(|seen| seen == block_type)
        {
            self.seen_unsupported_block_types
                .push(block_type.to_owned());
            self.push_diagnostic(DIAG_UNSUPPORTED_CONTENT_BLOCK);
        }
        let text = bound_text(
            &format!("Unsupported Claude content block type: {block_type}"),
            MAX_DIAGNOSTIC_TEXT,
        );
        self.push_block(CLAUDE_RENDER_KIND_STATUS, "Unsupported activity", &text, None, true, None);
        ClaudeIngestOutcome::Degraded
    }

    /// Create a new block for a one-shot content block (text/thinking/
    /// diagnostic). These are never updated after creation, unlike tool
    /// blocks, so each call allocates a fresh unique key.
    fn push_block(
        &mut self,
        render_kind: &str,
        title: &str,
        text: &str,
        status: Option<&'static str>,
        degraded: bool,
        role: Option<&str>,
    ) -> ClaudeIngestOutcome {
        let key = format!("blk:{}", self.next_content_seq);
        self.next_content_seq += 1;
        let ordinal = self.order.len();
        self.order.push(key.clone());
        self.blocks.insert(
            key,
            BlockState {
                render_kind: render_kind.to_owned(),
                title: Some(title.to_owned()),
                text: bound_text(text, MAX_BLOCK_TEXT),
                status,
                degraded,
                role: role.map(str::to_owned),
                turn_id: self.current_turn_id.clone(),
            },
        );
        ClaudeIngestOutcome::BlockUpserted { ordinal }
    }

    /// Create (or, if somehow re-seen, keep) a tool block keyed by its
    /// `tool_use_id`, so a later `tool_result` in a `user` event can look it
    /// up for correlation without leaking the id into any output. Role is
    /// always `"tool"` -- `tool_use`/`tool_result` never map to `"user"`/
    /// `"agent"`.
    fn upsert_tool_block(&mut self, tool_use_id: &str, name: &str, summary: &str) -> ClaudeIngestOutcome {
        let key = tool_key(tool_use_id);
        if let Some(index) = self.order.iter().position(|existing| existing == &key) {
            return ClaudeIngestOutcome::BlockUpserted { ordinal: index };
        }
        let ordinal = self.order.len();
        self.order.push(key.clone());
        self.blocks.insert(
            key,
            BlockState {
                render_kind: CLAUDE_RENDER_KIND_TOOL.to_owned(),
                title: Some(name.to_owned()),
                text: bound_text(summary, MAX_DIAGNOSTIC_TEXT),
                status: Some("running"),
                degraded: false,
                role: Some("tool".to_owned()),
                turn_id: self.current_turn_id.clone(),
            },
        );
        ClaudeIngestOutcome::BlockUpserted { ordinal }
    }

    fn complete_tool_block(
        &mut self,
        tool_use_id: &str,
        is_error: bool,
        content_text: &str,
    ) -> ClaudeIngestOutcome {
        let key = tool_key(tool_use_id);
        let Some(index) = self.order.iter().position(|existing| existing == &key) else {
            return ClaudeIngestOutcome::Ignored;
        };
        if let Some(block) = self.blocks.get_mut(&key) {
            block.status = Some(if is_error { "failed" } else { "completed" });
            if !content_text.is_empty() {
                if !block.text.is_empty() {
                    block.text.push('\n');
                }
                block.text.push_str(content_text);
                block.text = bound_text(&block.text, MAX_BLOCK_TEXT);
            }
        }
        ClaudeIngestOutcome::BlockUpserted { ordinal: index }
    }

    fn push_diagnostic(&mut self, diagnostic: &str) {
        if !self.diagnostics.iter().any(|seen| seen == diagnostic) {
            self.diagnostics.push(diagnostic.to_owned());
        }
    }
}

fn tool_key(tool_use_id: &str) -> String {
    format!("tool:{tool_use_id}")
}

/// CONTRACT: never surface the raw `input` payload; a bounded, path-free
/// summary only. `Bash`'s `command` is a display string, not a filesystem
/// path, so it is safe to bound and show.
fn tool_use_summary(name: &str, input: Option<&Value>) -> String {
    if let Some(command) = input.and_then(|input| input.get("command")).and_then(Value::as_str) {
        return bound_text(&format!("{name}: $ {command}"), MAX_DIAGNOSTIC_TEXT);
    }
    bound_text(name, MAX_DIAGNOSTIC_TEXT)
}

/// Extract bounded display text from a `tool_result`'s `content`, which may be
/// a plain string or an array of content blocks.
fn tool_result_text(content: Option<&Value>) -> String {
    match content {
        Some(Value::String(text)) => bound_text(text, MAX_BLOCK_TEXT),
        Some(Value::Array(parts)) => {
            let mut text = String::new();
            for part in parts {
                if let Some(part_text) = part.get("text").and_then(Value::as_str) {
                    if !text.is_empty() {
                        text.push('\n');
                    }
                    text.push_str(part_text);
                }
            }
            bound_text(&text, MAX_BLOCK_TEXT)
        }
        _ => String::new(),
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

#[cfg(test)]
mod tests {
    use super::*;

    const TURN_FIXTURE: &str = include_str!("../tests/fixtures/claude-cli-turn.ndjson");

    fn project_fixture() -> ClaudeProjector {
        let mut projector = ClaudeProjector::new();
        for line in TURN_FIXTURE.lines() {
            projector.ingest_line(line);
        }
        projector
    }

    #[test]
    fn projects_text_turn_to_ordered_assistant_block() {
        let projector = project_fixture();
        let blocks = projector.transcript_blocks();
        // Turn 1: one assistant text block.
        assert_eq!(blocks[0].render_kind, CLAUDE_RENDER_KIND_MARKDOWN);
        assert_eq!(blocks[0].title.as_deref(), Some("Assistant"));
        assert_eq!(blocks[0].text.as_deref(), Some("HELLO"));
        assert_eq!(blocks[0].cursor, "0");
        // `260713` Phase 2: text content blocks get role "agent" (Claude's
        // protocol never echoes the client's own prompt, so "user" never
        // appears), and a synthesized per-turn-boundary turn_id.
        assert_eq!(blocks[0].role.as_deref(), Some("agent"));
        assert_eq!(blocks[0].turn_id.as_deref(), Some("claude-turn-1"));
    }

    #[test]
    fn projects_tool_call_turn_with_thinking_and_correlated_tool_result() {
        let projector = project_fixture();
        let blocks = projector.transcript_blocks();
        // Real capture: a `thinking` block (with a real, non-empty
        // `signature` the projector must never surface) and a `Bash pwd`
        // tool_use correlated with its `tool_result`.
        let thinking = blocks
            .iter()
            .find(|block| block.render_kind == CLAUDE_RENDER_KIND_THINKING)
            .expect("thinking block present");
        assert_eq!(thinking.title.as_deref(), Some("Reasoning"));
        // `260713` Phase 2: thinking blocks leave role unset, like Codex's.
        assert_eq!(thinking.role, None);
        // Never leaks the thinking block's signature field (real captured
        // signature, not a synthetic marker).
        let serialized = serde_json::to_string(&blocks).expect("serialize blocks");
        assert!(!serialized.contains("EqUCCokBCA8YAipA9QieI27oSl2O6ONgC1WVLU6GBJfOeaPn85Y"));

        let tool_block = blocks
            .iter()
            .find(|block| {
                block.render_kind == CLAUDE_RENDER_KIND_TOOL
                    && block.title.as_deref() == Some("Bash")
                    && block.text.as_deref().unwrap_or_default().contains("pwd")
            })
            .expect("tool block present");
        assert!(tool_block.text.as_deref().unwrap().contains("status: completed"));
        assert!(tool_block
            .text
            .as_deref()
            .unwrap()
            .contains("/tmp/claude-cli-adapter-capture/workdir"));
        // `260713` Phase 2: tool_use/tool_result blocks get role "tool".
        assert_eq!(tool_block.role.as_deref(), Some("tool"));
        // Correlation key (tool_use_id) never crosses into the output.
        assert!(!serialized.contains("toolu_01UbL6NLnGbxxSosoEzMaG4a"));
    }

    #[test]
    fn turn_id_groups_multiple_blocks_from_the_same_turn_and_separates_others() {
        let projector = project_fixture();
        let blocks = projector.transcript_blocks();
        // The real capture's second turn (line 5-9 of the fixture) produces
        // three blocks: a leading assistant text, a Bash `pwd` tool call, and
        // a trailing assistant text -- all three must share the same
        // synthesized turn_id so they merge into one bubble browser-side,
        // while the first turn's lone text block gets a distinct turn_id.
        let turn1_ids: Vec<_> = blocks
            .iter()
            .filter(|block| block.text.as_deref() == Some("HELLO"))
            .map(|block| block.turn_id.clone())
            .collect();
        assert_eq!(turn1_ids, vec![Some("claude-turn-1".to_owned())]);

        let pwd_tool_turn_id = blocks
            .iter()
            .find(|block| {
                block.render_kind == CLAUDE_RENDER_KIND_TOOL
                    && block.text.as_deref().unwrap_or_default().contains("pwd")
            })
            .and_then(|block| block.turn_id.clone());
        assert_eq!(pwd_tool_turn_id, Some("claude-turn-2".to_owned()));

        // Every distinct turn_id observed is exactly {turn-1, turn-2, turn-3}
        // (the fixture has three `result`-delimited turns), confirming the
        // counter advances once per turn boundary, not once per block.
        let mut distinct_turn_ids: Vec<_> = blocks
            .iter()
            .filter_map(|block| block.turn_id.clone())
            .collect();
        distinct_turn_ids.sort();
        distinct_turn_ids.dedup();
        assert_eq!(
            distinct_turn_ids,
            vec![
                "claude-turn-1".to_owned(),
                "claude-turn-2".to_owned(),
                "claude-turn-3".to_owned(),
            ]
        );
    }

    #[test]
    fn hook_denied_tool_call_renders_blocked_status_without_leaking_reason_path() {
        let projector = project_fixture();
        let blocks = projector.transcript_blocks();
        let denied = blocks
            .iter()
            .find(|block| {
                block
                    .text
                    .as_deref()
                    .unwrap_or_default()
                    .contains("DENY_MARKER_TEST_BLOCK")
            })
            .expect("denied tool block present");
        assert!(denied.text.as_deref().unwrap().contains("status: blocked"));
        assert!(projector.diagnostics().contains(&DIAG_PERMISSION_DENIED.to_owned()));
        let serialized = serde_json::to_string(&blocks).expect("serialize blocks");
        assert!(!serialized.contains("toolu_01YE6wv1EEi9p38BgVimveyo"));
    }

    #[test]
    fn reads_usage_and_context_window_from_result_event() {
        let projector = project_fixture();
        let usage = projector.usage().expect("usage present");
        // Real capture's final (third) `result` event: top-level `usage`
        // gives the token counts, `modelUsage` carries two models this turn
        // (the primary `claude-sonnet-5` responder plus a background
        // `claude-haiku-4-5-...` helper) - the projector must pick the
        // *active* (assistant-responding) model's context window, not an
        // arbitrary map-ordered one.
        assert_eq!(usage.used_input_tokens, Some(4));
        assert_eq!(usage.used_output_tokens, Some(184));
        assert_eq!(usage.context_window, Some(1_000_000));
    }

    #[test]
    fn tracks_turn_lifecycle_across_result_boundaries() {
        let mut projector = ClaudeProjector::new();
        assert!(!projector.is_turn_active());
        projector.ingest_line(
            r#"{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}"#,
        );
        assert!(projector.is_turn_active());
        projector.ingest_line(r#"{"type":"result","subtype":"success"}"#);
        assert!(!projector.is_turn_active());
        assert_eq!(projector.turn_status(), Some("success"));
    }

    #[test]
    fn degrades_unknown_event_type_to_bounded_diagnostic_without_leaking_json() {
        let mut projector = ClaudeProjector::new();
        let outcome = projector.ingest_line(
            r#"{"type":"mysteryEvent","session_id":"secret-session","transcript_path":"/home/user/.claude/projects/x/secret.jsonl","payload":{"raw":"do-not-leak"}}"#,
        );
        assert_eq!(outcome, ClaudeIngestOutcome::Degraded);
        let blocks = projector.transcript_blocks();
        assert_eq!(blocks.len(), 1);
        assert!(blocks[0].degraded);
        assert_eq!(blocks[0].render_kind, CLAUDE_RENDER_KIND_STATUS);
        let text = blocks[0].text.as_deref().unwrap_or_default();
        assert!(text.contains("mysteryEvent"));
        assert!(!text.contains("secret-session"));
        assert!(!text.contains("do-not-leak"));
        let serialized = serde_json::to_string(&blocks).expect("serialize blocks");
        assert!(!serialized.contains("secret-session"));
        assert!(!serialized.contains("/home/user/.claude"));
        assert!(!serialized.contains("do-not-leak"));
    }

    #[test]
    fn degrades_unknown_content_block_type_to_bounded_diagnostic() {
        let mut projector = ClaudeProjector::new();
        let outcome = projector.ingest_line(
            r#"{"type":"assistant","message":{"role":"assistant","content":[{"type":"holographic_memory","payload":"do-not-leak"}]}}"#,
        );
        assert_eq!(outcome, ClaudeIngestOutcome::Degraded);
        let blocks = projector.transcript_blocks();
        assert_eq!(blocks.len(), 1);
        assert!(blocks[0].degraded);
        let text = blocks[0].text.as_deref().unwrap_or_default();
        assert!(text.contains("holographic_memory"));
        assert!(!text.contains("do-not-leak"));
    }

    #[test]
    fn malformed_line_degrades_without_panicking() {
        let mut projector = ClaudeProjector::new();
        let outcome = projector.ingest_line("{not json");
        assert_eq!(outcome, ClaudeIngestOutcome::Degraded);
        assert!(projector.diagnostics().contains(&DIAG_MALFORMED_RECORD.to_owned()));
    }

    #[test]
    fn ignores_protocol_control_events() {
        let mut projector = ClaudeProjector::new();
        for line in [
            r#"{"type":"system","subtype":"init","session_id":"s","cwd":"/private/x"}"#,
            r#"{"type":"rate_limit_event","rate_limit_info":{"status":"ok"}}"#,
            r#"{"type":"control_response","response":{"subtype":"success","request_id":"r1"}}"#,
            r#"{"type":"stream_event","event":{"type":"message_start"}}"#,
        ] {
            assert_eq!(projector.ingest_line(line), ClaudeIngestOutcome::Ignored);
        }
        assert!(projector.transcript_blocks().is_empty());
        assert!(!projector.degraded());
    }
}
