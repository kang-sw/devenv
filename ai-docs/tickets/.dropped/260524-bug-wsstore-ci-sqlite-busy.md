---
title: wsstore CI SQLite busy under concurrent handles
related:
  260524-epic-mcp-actor-setup-state: SQLite state-store foundation exposed the write contention surface
---

# wsstore CI SQLite busy under concurrent handles

## Pending Removal (2026-06-09)

Resolved-by-deletion candidate under `260605-epic-ws-playbook-factory-pivot`.
The wsstore actor state-store this write contention lives in is removed at
milestone M3: the session-auth model is an in-memory `{session-key → root}` map
with no SQLite actor records, so `TestConcurrentShortActorWrites` and its busy
surface disappear. Do not invest in a standalone fix; drop this ticket to
`.dropped/` in the same commit that removes wsstore actor state. Retained in
place until then so git blame and ticket scans surface the coupling.

## Background

The `v0.29.0` tag release workflow failed on Ubuntu during
`internal/wsstore.TestConcurrentShortActorWrites` with `database is locked (5)
(SQLITE_BUSY)`. Local macOS and native Windows checks passed before publish, so
the failure appears sensitive to the GitHub runner's SQLite locking timing.

The immediate release fix should keep same-process writes against one state DB
short and serialized. Follow-up research can decide whether cross-process MCP
servers need a stronger lease, retry, or connection-management policy beyond
the current single-process runtime expectation.
