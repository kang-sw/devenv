---
title: wsstore CI SQLite busy under concurrent handles
related:
  260524-epic-mcp-actor-setup-state: SQLite state-store foundation exposed the write contention surface
---

# wsstore CI SQLite busy under concurrent handles

## Background

The `v0.29.0` tag release workflow failed on Ubuntu during
`internal/wsstore.TestConcurrentShortActorWrites` with `database is locked (5)
(SQLITE_BUSY)`. Local macOS and native Windows checks passed before publish, so
the failure appears sensitive to the GitHub runner's SQLite locking timing.

The immediate release fix should keep same-process writes against one state DB
short and serialized. Follow-up research can decide whether cross-process MCP
servers need a stronger lease, retry, or connection-management policy beyond
the current single-process runtime expectation.
