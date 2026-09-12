---
title: Align reference-discovery ticket query examples with the MCP schema
sage-review-completeness: completed
sage-review-design: completed
sage-review-design-reviewed: 6bcbbee708004ce9
sage-review-completeness-reviewed: 6bcbbee708004ce9
completed: 2026-09-12
---

# Align reference-discovery ticket query examples with the MCP schema

## Background

The rendered `lead-workflow-manual` does not teach a `tickets.query` argument
example; it directs callers to the tool (agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L77-L81).
The shipped `reference-discovery` playbooks teach calls such as
`tickets.query(status: "ready")`, while the live `tickets.query` schema exposes
`statuses: string[]` and no `status` field (agents-plugin/rsrc/reference-discovery/reference-discovery.md#L25-L30; agents-plugin-wsflow/rsrc/reference-discovery/reference-discovery.md#L25-L30; agents-plugin-tool/internal/mcp/server.go#L3532-L3542).
This duplicated schema prose can directly teach an invalid call.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/reference-discovery/reference-discovery.md, agents-plugin-wsflow/rsrc/reference-discovery/reference-discovery.md, agents-plugin-tool/internal/mcp/server.go |
| scope.surface | public-interface | shipped playbooks document the public tickets.query MCP call |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | tickets.query schema already exposes statuses |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/ticket_review_design_test.go asserts tickets.query(statuses: ["ready"]) |
| complexity.reuse_points | confirmed | shared tickets.query schema in agents-plugin-tool/internal/mcp/server.go |
| complexity.side_effect_risk | moderate | rendered guidance must remain synchronized across plugin variants |
| risk.correctness | moderate | invalid argument guidance produces invalid MCP calls |
| risk.fit | moderate | the ticket title and background previously attributed the invalid examples to lead-workflow-manual |
| risk.test | moderate | coverage must inspect rendered examples without overmatching valid status arguments on other tools |
| risk.security_or_contract | moderate | shipped MCP call documentation must match the tool input contract |

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Phases

### Phase 1: Remove stale ticket-query call contracts

Align shipped guidance with the authoritative tool schema and add coverage that
prevents rendered examples from naming unsupported arguments.

### Result (2b112390) - 2026-09-12

- Replaced the singular `status` examples in canonical `reference-discovery`
  guidance with the schema-supported `statuses: ["…"]` form, regenerated the
  rsrc manifest, and regenerated the byte-identical wsflow rsrc mirror.
- Added a render-level regression test for ws and wsflow that requires each
  ready/todo/idea array-valued query and rejects the unsupported singular form.
- Verified with `go test ./internal/mcp ./internal/wsrsrc -count=1` and
  `git diff --check`; partitioned correctness, fit, and test reviews plus the
  second review round found no remaining findings.


## Resolution (2026-09-12)

Phase 1 completed: reference-discovery now uses the schema-supported statuses array in both shipped renders, with regression coverage for ws and wsflow.
