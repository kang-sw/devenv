---
title: add a session-state-only MCP tool to avoid re-dumping the full manual
sage-review: blocked
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

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md`. Caller-visible change: new MCP tool
(name open to bikeshedding, e.g. `session_state`) returning only the Session
State (todos/agenda) for the caller's session key, without the full manual
body; `workflow_manual` itself is unchanged. Contract-first spec: no.

## Blocked (2026-07-02)

### Design Reviewer — concern

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Session-state tool behavior for non-continue key states unspecified | important | autonomous |
| 2 | Tool name left open | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | missing frontmatter title field | important |
| 2 | no spec/related link in frontmatter | important |
| 3 | no phase sections | critical |
| 4 | no verification expectations | critical |
| 5 | tool name left unresolved | minor |

Completeness reviewer verdict is `block`: the ticket has no `### Phase N:`
breakdown (no completion boundary or scope split for naming the tool,
defining its output shape, wiring it into the MCP tool list, and updating
the spec) and no explicit test/probe/acceptance check for the new tool's
behavior. Per the aggregation rule, a `block` completeness verdict forces
the final verdict to `block` regardless of the `concern`-level design
review. Needs a phase breakdown and verification expectations before
promotion.
