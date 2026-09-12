---
title: Align workflow-manual ticket query examples with the MCP schema
---

# Align workflow-manual ticket query examples with the MCP schema

## Background

The rendered `lead-workflow-manual` teaches calls such as
`tickets.query(status: "ready")`, while the live `tickets.query` schema exposes
`statuses: string[]` and no `status` field. This duplicated schema prose can
directly teach an invalid call and demonstrates the drift risk identified by
the prompt-layer audit. The same singular argument also remains in the shipped
`reference-discovery` playbook, so the drift is not isolated to the manual.

## Phases

### Phase 1: Remove stale ticket-query call contracts

Align shipped guidance with the authoritative tool schema and add coverage that
prevents rendered examples from naming unsupported arguments.
