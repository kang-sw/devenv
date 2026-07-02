---
title: add a session-state-only MCP tool to avoid re-dumping the full manual
---

# add a session-state-only MCP tool to avoid re-dumping the full manual

## Context

Found during a v0.31.1 dogfooding pass. `workflow_manual` re-dumps the entire
~150-line manual on every call. In one session it was called 3 times when only
the session key plus the Session State section (todos/agenda) was actually
needed. This is especially costly right after compaction or during a
`lead-revive`, precisely when context budget is tightest — the caller wants a
cheap "what's my key and current state" check, not a full manual reload.

## Suggestion

Do not overload `workflow_manual` with a `state_only` flag — the name
`workflow_manual` should mean the manual, full stop, not a mode-dependent
mix of manual-or-state. Instead, add a new, separate MCP tool (naming open to
bikeshedding, e.g. `session_state` or `workflow_state`) whose sole
responsibility is returning the Session State section (todos/agenda) for the
caller's session key, with no manual reference text at all.
`workflow_manual` keeps its current always-full-dump behavior unchanged.
