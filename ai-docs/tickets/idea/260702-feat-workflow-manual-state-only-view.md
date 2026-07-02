---
title: workflow_manual needs a state-only view to avoid re-dumping the full manual
---

# workflow_manual needs a state-only view to avoid re-dumping the full manual

## Context

Found during a v0.31.1 dogfooding pass. `workflow_manual` re-dumps the entire
~150-line manual on every call. In one session it was called 3 times when only
the session key plus the Session State section (todos/agenda) was actually
needed. This is especially costly right after compaction or during a
`lead-revive`, precisely when context budget is tightest — the caller wants a
cheap "what's my key and current state" check, not a full manual reload.

## Suggestion

Add a lightweight session-state-only view, e.g. a `state_only` flag (or a
separate thin tool) that returns just the session key and Session State
(todos/agenda) without re-rendering the full manual body. Keep the full manual
as the default/no-flag behavior for first-load and explicit re-reads.
