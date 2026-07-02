---
title: add a session-state-only MCP tool to avoid re-dumping the full manual
sage-review: completed
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

For a session key that is invalid, expired, or otherwise not in a normal
continue state, the new tool must reuse `workflow_manual`'s existing
key-validation/error behavior rather than defining a separate error path —
this is a thin read of the same session state `workflow_manual` already
resolves, not a new state machine.

The new tool is lead-only, same as `workflow_manual` (`isLeadOnlyTool`),
not open to delegate/leaf scopes even though the underlying `todo.*`/
`agenda.*` data is itself scope-open. Rationale: this tool is a cheaper
view of the same lead-bootstrap/recovery surface `workflow_manual` serves
(compaction/revive context checks), not a general todo/agenda accessor —
keep it in the same tool family and gating as its sibling rather than
introducing a second, differently-scoped way to read session state.

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md`. Caller-visible change: new MCP tool
(name open to bikeshedding, e.g. `session_state`) returning only the Session
State (todos/agenda) for the caller's session key, without the full manual
body; `workflow_manual` itself is unchanged. Contract-first spec: no.

## Phases

### Phase 1: Add session-state-only MCP tool

- Add a new MCP tool (name TBD, e.g. `session_state`) that takes the
  caller's `session_key` and returns only the Session State portion
  (todos/agenda) for that session — no manual reference/primitives text.
- Reuse `workflow_manual`'s existing session-key resolution and
  error/validation behavior for invalid, expired, or unknown keys; do not
  invent a separate error path for this tool.
- `workflow_manual` itself stays unchanged — same always-full-dump
  behavior, same schema.
- Wire the new tool into the MCP tool registration/list alongside the
  existing `workflow_manual` tool.
- At implementation closeout, update `ai-docs/spec/mcp-tools.md` per the
  `## Spec Impact` note above (`Contract-first spec: no`).

Verification:
- For a given `session_key`, the new tool's output matches exactly the
  "Session State" section content that `workflow_manual` renders for that
  same session at the same point in time.
- The new tool's response is substantially shorter than a full
  `workflow_manual` render (no manual body/primitives reference text).
- An empty session (no todos, no agenda) returns an empty state payload,
  not an error.
- An invalid/expired/unknown `session_key` produces the same
  error/validation behavior as `workflow_manual` for that key, not a
  distinct error shape.
