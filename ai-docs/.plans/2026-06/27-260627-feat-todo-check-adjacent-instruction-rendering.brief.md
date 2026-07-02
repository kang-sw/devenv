# Brief: 260627-feat-todo-check-adjacent-instruction-rendering Phase 1

## Intent

Implement `Phase 1: Return focused checkpoint rendering from todo.check` for
`260627-feat-todo-check-adjacent-instruction-rendering`.

The behavior change is deliberately narrow: after a successful `ws.todo.check`,
the raw text response should confirm the status update and include a focused
checkpoint rendering of the whole ordered todo list. This removes the need for
the model to remember a follow-up `ws.todo.list` call after each checkpoint.

## Scope Boundary

Selected scope:

- Ticket:
  `ai-docs/tickets/ready/260627-feat-todo-check-adjacent-instruction-rendering.md`
- Phase: `Phase 1: Return focused checkpoint rendering from todo.check`
- Branch: `implement/260627-todo-check-adjacent-instruction-rendering`

In scope:

- Change successful `ws.todo.check` text output to include the compact
  confirmation plus a checkpoint todo rendering.
- Keep the checkpoint rendering raw/text-only.
- Render the whole ordered todo list, without summary ellipses.
- Include full `instruction` text only for immediate previous/next actionable
  items after the target status update.
- Treat actionable as `pending` or `wip` only.
- Render non-adjacent, `done`, `defer`, and instruction-less items compactly.
- Update the MCP schema description, spec, and mental model for the
  caller-visible behavior.
- Add focused tests for the checkpoint renderer and handler behavior.

Out of scope:

- Do not add `format: json` to `ws.todo.check`.
- Do not add a parallel structured response path for todo checkpoint output.
- Do not change `ws.todo.list`, workflow-manual restoration, or commit reminder
  rendering except through reusable private helpers that preserve their current
  contracts.
- Do not alter todo status mutation semantics: `check` changes only the target
  status and must preserve untouched todo payloads.
- Do not change `ws.enter.*` todo production in this phase.

## Caller-Visible Contract

On a successful check, `ws.todo.check` returns raw text shaped as:

```text
todo <status>: <key>
<checkpoint todo rendering>
```

The checkpoint rendering rules are:

- Base adjacency on the stable list order after the status update.
- Consider only the checked item's immediate previous and next list positions.
- Show full `instruction` text only when an adjacent item is actionable
  (`pending` or `wip`) and has a non-empty instruction.
- Keep adjacent `done` and `defer` items compact.
- Keep every non-adjacent item compact, regardless of status or instruction.
- Keep instruction-less adjacent actionable items compact.
- Keep every todo visible in order; no `...` summary collapse lines.

`ws.todo.check` remains a raw/text-only tool for this response. The schema must
not grow `format: json`, and tests should protect that absence.

## Contract Instructions

Primary implementation area:

- `agents-plugin-tool/internal/mcp/session_state.go`
- `agents-plugin-tool/internal/mcp/server.go`

Primary documentation area:

- `ai-docs/spec/mcp-tools.md`
- `ai-docs/mental-model/mcp-runtime.md`

Primary test area:

- `agents-plugin-tool/internal/mcp/session_state_test.go`

Implementation constraints:

- Prefer a private renderer helper for checkpoint output rather than changing
  `renderTodos(list, full bool)` semantics.
- Reuse `renderTodoLine`, `renderTodoLines`, `todoMarker`, and existing
  instruction omission behavior where possible.
- Avoid deriving full instruction text back from summary rendering.
- Read the updated list after mutation from the same store mutation path, or
  return it from the mutation callback, so the response reflects the
  post-update order and statuses.
- Keep all output deterministic for exact-string tests.

## Integration Test Instructions

Required focused coverage:

- Middle item checked: immediate previous and next actionable items render full
  instructions; non-adjacent items remain compact.
- First item checked: only the immediate next item is considered adjacent.
- Last item checked: only the immediate previous item is considered adjacent.
- Adjacent `done` item remains compact even when it has an instruction.
- Adjacent `defer` item remains compact even when it has an instruction.
- Adjacent actionable item with nil or empty instruction remains compact.
- Checking to each valid status (`pending`, `wip`, `done`, `defer`) succeeds and
  renders the checkpoint from the post-update state.
- Existing status/order mutation tests still prove `check` preserves untouched
  `instruction` payloads.
- `tools/list` schema for `ws.todo.check` does not advertise `format`.

Run commands from `agents-plugin-tool/`:

```sh
go test ./internal/mcp -count=1
go test ./... -count=1
```

Run from repository root:

```sh
ws spec_index.verify
git diff --check
```

If the local MCP CLI is unavailable, use the repository's available spec-index
verification path and report the substitute command explicitly.

## Implementation Strategy Decisions

- Treat `ws.todo.check` as the checkpoint guidance boundary. The caller should
  not need a second autonomous `ws.todo.list` call to see the next adjacent
  instruction.
- Keep checkpoint rendering distinct from summary rendering. Summary mode still
  collapses distant items and previews instructions; checkpoint rendering shows
  every item and only expands immediate actionable neighbors.
- Keep the response readable prose/text because this tool is guiding the next
  model action directly.
- Protect the no-JSON decision at schema/test level to avoid model preference
  drift toward structured output.

## Rejected Alternatives

- Adding `format: json` is rejected by ticket decision.
- Returning only the next actionable item is rejected because the ticket asks
  for the whole ordered list as context.
- Reusing full-mode rendering directly is rejected because it would expand every
  instruction, not just adjacent actionable items.
- Reusing summary-mode rendering directly is rejected because it can hide
  non-adjacent ordered context behind `...`.

## Verification Contract

Acceptance requires:

- Focused MCP/session-state tests pass.
- Existing todo summary/full rendering tests still pass.
- `go test ./internal/mcp -count=1` passes.
- `go test ./... -count=1` passes from `agents-plugin-tool/`.
- Spec and mental-model docs describe the new `ws.todo.check` checkpoint output.
- Spec index verification passes.
- `git diff --check` passes.
