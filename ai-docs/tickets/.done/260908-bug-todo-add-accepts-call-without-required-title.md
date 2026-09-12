---
title: todo.add accepts a call that omits the required title and silently stores an empty item
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 8de8d4fc5d2a7f5a
sage-review-completeness-reviewed: 8de8d4fc5d2a7f5a
completed: 2026-09-12
---

# todo.add accepts a call that omits the required title and silently stores an empty item

## Background

Dogfood surprise (2026-09-08, Claude Code host, ws-mcp on the Pi track).
The advertised `todo.add` schema lists `title` as required and has no `text`
field. A lead called `todo.add(session_key, key, text: "...")` — wrong field
name, no `title` — six times; every call returned `todo added: <key>` and the
items were created with an empty title (`todo.read` → `{"title": "",
"status": "pending", "instruction": null}`; `todo.list` rendered
`- [ ] {odq-01} ` with nothing after the key). No validation error was
raised for the missing required field or the unknown one, so the mistake
surfaced only later, when the list was re-read and the queue items turned out
to be unreadable.

## Decisions

- Make `todo.add` reject a missing or empty `title` with a fail-loud error that
  identifies the invalid argument.
- Keep unknown-field handling unchanged in this fix.
- Keep sibling `todo.*` mutation tools outside this ticket unless the same
  missing-title path is directly shared by `todo.add`.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/session_state.go#L1001-L1050 and agents-plugin-tool/internal/mcp/session_state_test.go#L2842-L2958 |
| scope.surface | public-interface | todo.add is an advertised MCP tool; agents-plugin-tool/internal/mcp/server.go#L3147-L3159 |
| scope.new_public_symbol | no | existing todo.add handler and schema |
| scope.new_type_contract | no | validates an existing title string argument |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/session_state_test.go#L2842-L2958 already covers todo.add error branches |
| complexity.reuse_points | confirmed | rawStringArg already rejects empty required strings; agents-plugin-tool/internal/mcp/session_state.go#L745-L751 |
| complexity.side_effect_risk | moderate | validation must precede todo list mutation on both append and insert paths |
| risk.correctness | moderate | must reject omitted and empty titles while preserving valid title behavior |
| risk.fit | moderate | runtime behavior must match the advertised required schema |
| risk.test | moderate | omitted, empty, and valid title contract cases need coverage |
| risk.security_or_contract | moderate | changes the error contract of the public todo.add tool |

## Phases

### Phase 1: Reject todo.add calls that violate the advertised schema

Validate the advertised required `title` at the `todo.add` handler boundary and
add contract coverage for omitted, empty, and valid titles without changing
unknown-field behavior.

### Result (3ef60be2) - 2026-09-12

- `todo.add` now validates `title` with the existing raw required-string helper
  before append or insert mutation, returning `todo.add: title is required` for
  omitted or empty values.
- Contract coverage verifies omitted and empty titles fail without creating an
  item, while a valid title succeeds and is stored unchanged.
- Verification: `go test ./internal/mcp -run '^TestServeStdioTodoAdd(ErrorBranches|ReproducesOldPlacements)$' -count=1` and `go test ./...` passed from `agents-plugin-tool`; correctness, fit, and test reviews plus the round-two convergence check were clean.
