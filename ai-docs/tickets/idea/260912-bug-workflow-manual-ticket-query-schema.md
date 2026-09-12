---
title: Align workflow-manual ticket query examples with the MCP schema
---

# Align workflow-manual ticket query examples with the MCP schema

## Background

The rendered `lead-workflow-manual` teaches calls such as
`tickets.query(status: "ready")`, while the live `tickets.query` schema exposes
`statuses: string[]` and no `status` field. This duplicated schema prose can
directly teach an invalid call and demonstrates the drift risk identified by
the prompt-layer audit.

## Phases

### Phase 1: Remove the stale ticket-query call contract

Align the workflow-manual guidance with the authoritative tool schema and add
coverage that prevents rendered examples from naming unsupported arguments.
