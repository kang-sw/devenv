---
title: "Hosted sessions: the context meter shows the model window and refreshes only when a turn settles"
related:
  261110-feat-ws-dashboard-headless-lead-sessions: hosted Claude lead sessions
  261110-feat-ws-dashboard-agent-panel-telemetry: dashboard usage figures
completed: 2026-11-12
---

# Hosted sessions: the context meter shows the model window and refreshes only when a turn settles

## Background

Found while dogfooding a daemon-hosted Claude lead session (2026-11-12,
reply-id `autisticcavalierharpistdairy`, Claude Code 2.1.322).

1. **Wrong window.** The page showed a 1M maximum context. The effective
   window is 300k: the user's `~/.claude/settings.json` sets
   `CLAUDE_CODE_AUTO_COMPACT_WINDOW=300000`, and the harness compacts against
   that. `agents-plugin-tool/internal/wsagent/claude.go` sets `m.window` from
   the largest `modelUsage.contextWindow` of the `result` line, which is the
   model's window, and `status()` reports it. The terminal path is correct:
   `agents-plugin/hooks/claude-mod/dashboard.ts` uses the harness's
   `context.breakdown.rawMaxTokens`.
2. **Late refresh.** After a `/compact` the page kept showing about 220k until
   the next turn settled. `assistant()` already updates `m.context` from every
   lead assistant message's `usage`, but the `status()` event that carries it is
   emitted only on `result`, the idle state change, and `rate_limit_event`.

## Decisions

- The meter refreshes at every lead assistant message that carries `usage`
  (each model call, so each tool-call boundary), by emitting `status()` there.
  Subagent messages (`parent_tool_use_id` set) and alert messages are left out,
  as they already are for the token count. User board (18).
  Rejected: a timer; it adds writes with no new data.
- The window is the effective compaction window the harness itself uses, the
  same figure the terminal path shows (`rawMaxTokens`). Preferred source: the
  stream-json `get_context_usage` control request (present in the 2.1.322
  binary), asked at a point that costs no extra turn. User board (16), (18).
  Rejected: parsing `CLAUDE_CODE_AUTO_COMPACT_WINDOW` from settings files and
  the environment in the daemon; it re-implements the harness's settings
  precedence and drifts from it.
- Compatibility tier: T0. Snapshot and record shapes are unchanged; only the
  value and the refresh frequency change.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md` (paths under
  `agents-plugin-tool/internal/`).
- If `get_context_usage` is unavailable or unusable in the hosted `claude -p`
  stream-json mode, stop and report the evidence; do not fall back to settings
  parsing without a new decision.
- Heavy Go builds and tests run through `~/.local/bin/ws-rgo` on this machine.


## Resolution (2026-11-12)

Fixed in d6598b446 (merge 39d78eed3): the window comes from the get_context_usage control answer's rawMaxTokens, asked once per model; the meter refreshes on each changed lead usage. Verified live after the daemon replacement: the snapshot reads window 300000 (was 1000000). User board (25).
