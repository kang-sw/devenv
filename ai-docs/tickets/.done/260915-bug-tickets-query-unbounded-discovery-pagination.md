---
title: "Bound tickets.query discovery output with deterministic offset pagination"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c5c373382a9f814c
sage-review-completeness-reviewed: c5c373382a9f814c
completed: 2026-09-16
---

# Bound tickets.query discovery output with deterministic offset pagination

## Background

The owner observed a `tickets.query` discovery call over the completed-ticket archive return nearly 500K of text in one tool result. Completed tickets form an unbounded inventory, so an omitted result bound allows a routine query to consume a large fraction of the caller's context in one response.

The current discovery result is deterministic: it scans tickets in ticket-status-rank and ticket-stem order, then filters matching tickets, preserving that order in the result (`agents-plugin-tool/internal/wsdoc/tickets.go`). The correction adds bounded offset pagination over that existing result order. It addresses response/context size; discovery may still scan the selected ticket bodies to evaluate filters.

## Decisions

- Add integer `offset` and `limit` inputs to `tickets.query` discovery calls.
- `offset` defaults to `0` and rejects negative or non-integral values.
- `limit` defaults to `50`, accepts values from `1` through `200`, and rejects out-of-range or non-integral values.
- Apply pagination after all existing status, text, stem, and mention filters and after the existing deterministic status-rank/stem ordering.
- Preserve the existing JSON array response shape. A caller requests the next page with `offset + limit`; no cursor or response envelope is added.
- Leave the exact `ticket_stem` point-resolve form unchanged and unpaginated.
- Apply the default bound to every discovery listing, not only `.done/`, so every potentially growing result set is context-safe.

## Constraints

- Preserve current discovery matching, snippets, sparse-checkout visibility behavior, status ordering, and stem ordering.
- Preserve existing point-resolve object/error semantics byte-for-byte.
- Do not present pagination as a scanner-performance optimization; the required behavior is a bounded returned projection.
- Update the published tool description and input schema so callers can discover the defaults, range, ordering basis, and next-offset rule.
- Add boundary and behavior tests for omitted inputs, a second page, a short terminal page, zero/negative/oversized/non-integral values, filtering-before-pagination, deterministic ordering, unchanged JSON array shape, and unchanged point resolve.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/wsdoc/tickets.go, and existing Go tests |
| scope.surface | public-interface | tickets.query MCP input schema and tool description in agents-plugin-tool/internal/mcp/server.go |
| scope.new_public_symbol | no | no new exported Go symbol is required; offset and limit are tool inputs |
| scope.new_type_contract | yes | tickets.query gains integer offset and limit input-contract fields |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/tickets_test.go and agents-plugin-tool/internal/mcp/tickets_scope_test.go |
| complexity.reuse_points | confirmed | TicketsFind already filters a sorted discovery slice in agents-plugin-tool/internal/wsdoc/tickets.go |
| complexity.side_effect_risk | moderate | pagination must not affect the point-resolve branch or sparse-scope annotation behavior |
| risk.correctness | moderate | slicing must occur after filtering while preserving status-rank/stem order |
| risk.fit | moderate | the public schema, description, and MCP boundary must agree on defaults and validation |
| risk.test | moderate | boundary validation, page behavior, ordering, shape, and point-resolve regressions need coverage |
| risk.security_or_contract | moderate | the read-only MCP tool gains a caller-visible pagination and validation contract |

## Phases

### Phase 1: Add bounded offset pagination to discovery queries

Implement and validate the two inputs at the MCP boundary, slice the fully filtered and sorted discovery result, and keep the point-resolve branch untouched. Pin both text and JSON behavior, including the default 50-item cap and explicit next-page calls. Run the focused ticket-query tests and the full Go test suite.

### Result (2053ec32) - 2026-09-15

Added validated discovery-only `offset`/`limit` inputs, with defaults of 0/50 and a limit range of 1 through 200. `TicketsFind` now slices only after existing filters and deterministic status-rank/stem ordering; its zero-value options remain unbounded for non-MCP internal callers, while `tickets.query` supplies the bounded default. The exact `ticket_stem` point-resolve branch remains ahead of pagination parsing and retains its object/error behavior.

Published schema and description text document the defaults, range, ordering basis, and `offset + limit` next-page rule. Tests cover filtered ordering, default unfiltered and filtered caps, text and bare-array JSON projections, full and terminal pages, accepted endpoints, invalid integer/range inputs, and existing point-resolve behavior.

Verification: `go test ./internal/wsdoc -run 'TestTickets' -count=1`; `go test ./internal/mcp -run 'TestTickets|TestServeStdioTicketsQuery' -count=1`; `go test ./...`; `scripts/smoke-ws-mcp.sh ..` all passed. Partitioned review found correctness and fit clean; test review's Important request for an unfiltered >50-ticket case was fixed in `bbef57f3`, and round 2 confirmed it clean.
