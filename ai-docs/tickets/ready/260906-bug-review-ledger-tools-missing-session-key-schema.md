---
title: Review marker requires a session key absent from its advertised schema
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 94f5970df44846a9
sage-review-completeness-reviewed: 94f5970df44846a9
---

# Review marker requires a session key absent from its advertised schema

## Background

During the authorized ws release on 2026-09-06, the connected MCP tool
`review.marker(format: "json")` returned:

```text
mandatory_session_key: root-aware ws tools require a session_key; if you are the lead, obtain one per ws:workflow-manual and pass it
```

The advertised schema exposed only optional `bootstrap` and `format`, so the
schema-conforming call could not satisfy the runtime's session requirement.
Retrying the same read with the existing lead `session_key` as an additional
top-level field succeeded and returned the review frontier. This host permits
that extra argument; downstream hosts may not.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/mcp/server_test.go, agents-plugin-tool/internal/mcp/review_watermark_checkpoint_test.go |
| scope.surface | public-interface | MCP input schemas for review.marker and review.stamp in agents-plugin-tool/internal/mcp/server.go#L3561-L3583 |
| scope.new_public_symbol | no | no new tool name is proposed |
| scope.new_type_contract | yes | session_key input-schema field for two public MCP tools |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/server_test.go and agents-plugin-tool/internal/mcp/review_watermark_checkpoint_test.go |
| complexity.reuse_points | confirmed | existing session_key schema convention and resolveToolRoot in agents-plugin-tool/internal/mcp/server.go#L2729-L2742 |
| complexity.side_effect_risk | moderate | review.stamp mutates the ledger and must preserve its semantics |
| risk.correctness | high | schema-conforming review.marker calls currently fail before reading the frontier |
| risk.fit | moderate | schema must align with the established root-aware session contract |
| risk.test | moderate | both tools need schema and keyed-call coverage without changing ledger behavior |
| risk.security_or_contract | high | this changes the public MCP input contract consumed by hosts |

## Phases

### Phase 1: Reconcile review-tool session arguments

Investigate the missing schema field and align the advertised contract with
runtime session requirements. Check the adjacent `review.stamp` surface,
whose currently advertised fields also omit `session_key`; its failure was
not reproduced at capture time. Reuse the shared root-aware session-key schema
injection and cover both tools with schema assertions plus keyed marker and
stamp calls. Preserve existing ledger semantics.
