---
title: investigate tool-sprawl and session_key threading overhead across the wsflow surface
sage-review: required
---

# investigate tool-sprawl and session_key threading overhead across the wsflow surface

## Context

Found during a v0.31.1 dogfooding pass that exercised the full wsflow tool
surface (54 tools). All 54 tools are deferred, so each tool's first use in a
session pays a schema-load round trip; covering the full surface required
several separate load batches. Separately, nearly every tool requires a
`session_key` parameter, so a lead ends up re-injecting the same key on
essentially every call.

This may be partly inherent to how MCP tool deferral and the harness work
(not fully within wsflow's control), so this is framed as an open research
question rather than a committed feature.

## Suggestion

Investigate whether a curated bundle of commonly used read/query tools could
be pre-loaded together (reducing load-batch count), and whether a
session-scoped default key (so most calls don't need to pass `session_key`
explicitly) is feasible given the MCP/harness constraints. Treat this as
research: determine what's fixable on the wsflow side versus what's inherent
to the MCP + harness deferral model before committing to any specific
implementation.
