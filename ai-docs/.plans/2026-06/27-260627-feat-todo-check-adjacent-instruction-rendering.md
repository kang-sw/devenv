# Survey: 260627-feat-todo-check-adjacent-instruction-rendering Phase 1

## Outcome

- `[ok]` - survey is sufficient; no `[escalate-to-research]` needed. The
  implementation seam is the existing `ws.todo.check` handler plus a new
  private checkpoint renderer over the post-update todo list.

## Source/Test Mapping

- `agents-plugin-tool/internal/mcp/session_state.go#L333-L344` ->
  `agents-plugin-tool/internal/mcp/session_state_test.go#L469-L521` -
  existing per-item rendering and nil/empty instruction omission should be
  reused, but summary/full semantics must remain unchanged.
- `agents-plugin-tool/internal/mcp/session_state.go#L757-L766` ->
  `agents-plugin-tool/internal/mcp/session_state_test.go#L364-L400` -
  `mutateTodos` is the current status/order mutation path; adapt the check path
  so the handler can format the post-update list without rewriting untouched
  payloads.
- `agents-plugin-tool/internal/mcp/session_state.go#L1055-L1083` ->
  `agents-plugin-tool/internal/mcp/session_state_test.go#L1403-L1439` -
  `handleTodoCheck` currently returns only `todo <status>: <key>`; extend this
  handler response and update MCP integration expectations.
- `agents-plugin-tool/internal/mcp/server.go#L2886-L2898` ->
  `agents-plugin-tool/internal/mcp/session_state_test.go` or
  `agents-plugin-tool/internal/mcp/server_test.go` - update the
  `ws.todo.check` schema description to mention checkpoint text output, and add
  coverage that no `format` property exists.
- `ai-docs/spec/mcp-tools.md#L270-L298` ->
  `ai-docs/mental-model/mcp-runtime.md#L48-L50` - update the durable todo
  contract and modification mental model for the new checkpoint renderer.

## Existing Mechanisms To Reuse

- `todoItem.Instruction *string` is already the durable full-prose payload; do
  not add storage or derive full instructions from rendered preview text.
- `todoCheck` already updates one target status and preserves other item fields;
  keep that pure mutation behavior.
- `renderTodoLine` provides compact marker/key/title lines for all statuses.
- `renderTodoLines(item, true)` can render a full instruction line for selected
  adjacent actionable items, while `renderTodoLine(item)` keeps compact items
  compact.
- `parseTodoStatus` already accepts `pending`, `wip`, `done`, `defer`; use the
  same valid status set for checkpoint tests.
- `callToolWithKey` in MCP integration tests already exercises `ws.todo.check`
  over stdio and is the right handler-level harness.

## Implementation Steps

1. Add a small pure renderer, for example
   `renderTodosCheckpoint(list []todoItem, checkedKey string) string`, that
   returns all todo lines in stored order.
2. In that renderer, find the checked item's post-update index. Only indexes
   `index-1` and `index+1` are eligible for full instruction rendering.
3. Add `todoIsActionable(status)` or equivalent private logic for `pending` and
   `wip`; do not treat `done` or `defer` as actionable.
4. For adjacent actionable items with non-empty instructions, render
   `renderTodoLines(item, true)`. For every other item, render only
   `renderTodoLine(item)`.
5. Change `handleTodoCheck` so it captures the post-update list produced by
   `todoCheck` and returns:

   ```text
   todo <status>: <normalized-key>
   <checkpoint rendering>
   ```

6. Update `ws.todo.check` schema description only; do not add any new input
   property.
7. Update `ai-docs/spec/mcp-tools.md` and `ai-docs/mental-model/mcp-runtime.md`
   with the checkpoint contract.
8. Add exact-string or focused substring tests for middle/first/last adjacency,
   adjacent `done`, adjacent `defer`, nil/empty instructions, each valid target
   status, mutation preservation, and schema no-`format`.

## Risks

- Returning the checkpoint by calling `ws.todo.list`-style summary rendering
  would violate the no-ellipsis whole-list requirement.
- Returning full-mode rendering would violate the requirement to compact
  non-adjacent items.
- Computing adjacency before the status update would produce wrong instruction
  expansion when the checked item changes to `pending` or `wip`; the contract is
  explicitly post-update.
- Adding a JSON mode would contradict the ticket's raw/text-only decision and
  may encourage callers to ignore the prose guidance.
- If `mutateTodos` cannot return the updated list cleanly, prefer a narrowly
  scoped helper for check rather than a broader store refactor.

## Constraints From References

- `ai-docs/tickets/ready/260627-feat-todo-check-adjacent-instruction-rendering.md`
  - Phase 1 requires confirmation plus focused checkpoint rendering from
  `ws.todo.check`.
- `ai-docs/spec/mcp-tools.md#L270-L298` - todo items carry nullable
  `instruction`; status mutations preserve untouched payloads; current
  summary/full rendering contracts must remain coherent.
- `ai-docs/mental-model/mcp-runtime.md#L48-L50` - instruction payloads are
  durable full prose and should not be duplicated into another store.
- `agents-plugin/rsrc/impl-playbook.md` - read full verification output before
  claiming pass, diagnose failures before patching, and record non-obvious
  contracts in commit `## AI Context` when implementation creates them.

## Verification Commands

From `agents-plugin-tool/`:

```sh
go test ./internal/mcp -count=1
go test ./... -count=1
```

From repository root:

```sh
ws spec_index.verify
git diff --check
```

If `ws spec_index.verify` is unavailable in the shell environment, use the MCP
or repository-local equivalent and report the exact substitute.
