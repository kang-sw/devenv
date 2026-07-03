---
title: "enter.implement explicit_direct_edit_request field not exposed in JSON Schema description"
completed: 2026-06-30
---

# enter.implement explicit_direct_edit_request field not exposed in JSON Schema description

## Background

Dogfood surprise (2026-06-30, policy-sweep Track 1 dogfood run).

`ws/enter.implement` accepts and correctly processes `explicit_direct_edit_request`
in the `facts.scope` input (verified: `yes` overrides multi-file span to
`direct-edit` verdict). However, the field is not listed in the MCP tool's JSON
Schema description for the `facts.scope` parameter.

## Impact

- Callers relying on schema introspection (e.g. agent tool-discovery) will not
  discover `explicit_direct_edit_request` as a valid field.
- The field exists in the playbook (`lead-implement` facts table) but not in the
  MCP schema, creating a documentation split between the playbook surface and the
  tool surface.
- Future callers may omit the field by mistake, defeating the explicit-override
  design.

## Direction

Add `explicit_direct_edit_request` to the `facts.scope` parameter description in
`agents-plugin-tool/internal/mcp/server.go` (or wherever the tool schema is
defined for `enter.implement`). The description should state accepted values
(`yes`, `no`, `unknown`) and that `yes` overrides all other predicates to force
`direct-edit`.


## Resolution (2026-06-30)

Added explicit_direct_edit_request to the facts.scope property map in server.go (ws.enter.implement tool schema). Field now visible to schema introspection callers with accepted values yes/no/unknown and override semantics documented.
