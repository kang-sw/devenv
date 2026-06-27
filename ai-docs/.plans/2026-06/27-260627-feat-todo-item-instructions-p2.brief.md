# Brief: 260627-feat-todo-item-instructions Phase 2

## Intent

Implement `Phase 2: Todo instruction rendering` for
`260627-feat-todo-item-instructions`: render the optional todo `instruction`
field consistently anywhere todo text is shown to the lead, so the todo list can
carry focused runbook prose without adding a second execution-steps surface.

This is a rendering-only slice. Phase 1 already added storage, setter
validation, and `ws.todo.read`. Phase 3 will later make `ws.enter.implement`
populate instructions from implementation verdicts; do not implement that here.

## Scope Boundary

Selected scope:

- Ticket: `ai-docs/tickets/ready/260627-feat-todo-item-instructions.md`
- Phase: `Phase 2: Todo instruction rendering`
- Branch: reuse current branch `implement/260627-enter-implement-verdict-engine`
  by user policy.

In scope:

- Update todo text rendering so instruction text appears as an indented second
  line under the todo title when present.
- Make summary/default todo rendering show a 60-character instruction preview.
- Make checkpoint/restoration todo rendering through `ws.workflow_manual` use
  the same summary preview path.
- Make `ws.todo.list(mode: "full")` show the full instruction text.
- Suppress the instruction line when `instruction` is absent, null, or empty.
- Add focused tests proving summary truncation, full untruncated rendering, and
  workflow-manual preview rendering.

Out of scope:

- Do not change todo storage shape, setter schemas, validation, or
  `ws.todo.read` behavior except as required by rendering tests.
- Do not make any `ws.enter.*` tool populate instruction prose.
- Do not change `ws.enter.implement` todo derivation, titles, verdict output, or
  lead-implement playbook text.
- Do not implement Phase 3 enter-tool instruction producers.
- Do not create a separate `execution_steps` or runbook list beside todos.

## Caller-Visible Contract

Todo list text keeps the existing first-line shape:

```text
- [ ] {key} Title
```

When a todo item has a non-empty `instruction`, render a second line indented
with six spaces:

```text
- [ ] {key} Title
      Instruction text here
```

Summary/default rendering:

- Applies to `ws.todo.list` when `mode` is omitted or anything other than
  `full`.
- Applies to checkpoint/restoration todo text because `ws.workflow_manual`
  already calls `renderTodos(record.Todos, false)` in `renderSessionState`.
- Shows the same active/context/collapsed rows as before.
- For each rendered todo row with a non-empty instruction, shows at most a
  60-character preview on the indented second line.

Full rendering:

- Applies to `ws.todo.list(mode: "full")`.
- Shows every todo in order as before.
- For each rendered todo with a non-empty instruction, shows the full
  instruction text untruncated on the indented second line.

Empty instruction behavior:

- `nil`, absent legacy fields, and `""` render no instruction line.
- Existing markers and status behavior are unchanged:
  `- [ ]`, `- [~]`, `- [x]`, and `- [>]`.
- Collapsed `...` lines never receive instruction text.

Preview length:

- Use 60 characters as the maximum preview length for summary/checkpoint
  rendering.
- The preview must be deterministic and must not exceed 60 characters. Prefer a
  small helper so pure rendering tests can assert exact output.

## Contract Instructions

Primary implementation file:

- `agents-plugin-tool/internal/mcp/session_state.go`

Likely functions to change or add:

- `renderTodos(list []todoItem, full bool) string`
- `renderTodoLine(item todoItem) string`
- Add a small pure helper for instruction rendering/truncation if it keeps the
  code clearer, for example `renderTodoLines(item todoItem, full bool) []string`
  or `todoInstructionPreview(instruction string) string`.

Current important coupling:

- `ws.todo.list` reads stored todos and calls `renderTodos(record.Todos, full)`.
- `ws.workflow_manual` renders restored session todos through
  `renderSessionState`, which calls `renderTodos(rec.Todos, false)`.
- Therefore workflow-manual preview behavior should be obtained by reusing the
  summary path, not by adding separate workflow-manual-specific formatting.

Do not add API surface for this phase:

- No new MCP tool.
- No new public mode.
- No new runtime manifest entry.
- No new todo item field.
- No new enter-tool instruction generation.

## Integration Test Instructions

Primary test file to extend:

- `agents-plugin-tool/internal/mcp/session_state_test.go`

Required tests:

- Pure rendering test for summary/default mode:
  - Include at least one rendered todo with an instruction longer than 60
    characters.
  - Assert the indented instruction line exists.
  - Assert the rendered instruction preview is no longer than 60 characters.
  - Assert collapsed `...` behavior remains intact.
- Pure rendering test for full mode:
  - Include a long instruction.
  - Assert `renderTodos(list, true)` includes the full instruction text
    untruncated.
- Empty-instruction test:
  - Assert nil and empty string instructions render no second line.
- MCP integration test for `ws.todo.list`:
  - Use existing `callToolWithKey` helpers to append todos with instructions.
  - Assert default list uses a preview and `mode:"full"` uses full text.
- Workflow manual test:
  - Populate a session todo with an instruction through `ws.todo.append` or
    existing session helpers.
  - Call `ws.workflow_manual(session_key: key)`.
  - Assert the `## Session State` todo summary includes the 60-character preview
    and not the full long instruction.

Run commands from `agents-plugin-tool/`:

```sh
go test ./internal/mcp -count=1 -run 'TestTodo|TestRenderTodos|TestWorkflowManual|TestServeStdioTodo'
go test ./... -count=1
```

Run from repository root:

```sh
git diff --check
```

## Implementation Strategy Decisions

- Reuse the existing `renderTodos` path as the single rendering authority. This
  keeps `ws.todo.list`, `ws.workflow_manual`, enter-tool responses that render
  todos, and commit/checkpoint reminders aligned when they call the same
  formatter.
- Keep the first line stable and add instruction text only as an indented second
  line. Do not alter key markers or summary active/context row selection.
- Treat instruction rendering as text formatting only. Phase 2 must not infer or
  generate instruction prose from route/verdict facts.
- Put truncation in pure code, close to todo rendering, so it is covered by unit
  tests without needing MCP server setup.

## Rejected Alternatives

- Separate workflow-manual formatting is rejected because it would fork the
  checkpoint rendering contract from `ws.todo.list` summary mode.
- Adding `execution_steps` beside todos is rejected by the ticket decision; todo
  items are the single source for reachable runbook steps.
- Generating instructions in `ws.enter.implement` during this phase is rejected
  because that is Phase 3 and depends on Phase 2 rendering being complete.
- Changing todo titles to carry prose is rejected; titles stay short scan labels
  and `instruction` carries prose.

## Approach

- Extend `renderTodoLine` or split it into a helper that can return one or two
  physical lines for a todo item.
- Thread the `full` flag into instruction formatting so summary uses preview
  text and full mode uses the full instruction.
- Preserve existing summary row-selection logic, appending rendered todo lines
  only for rows that are already selected.
- Add or update tests around the existing `TestRenderTodosSummaryAndFull`,
  `TestServeStdioTodoInstructionReadSurface`, and workflow-manual continue-mode
  tests.
- Keep implementation commits separate from this brief commit.

## Constraints

- AI-authored docs, tests, and commit messages must be in English.
- Do not edit source or tests in the prep/brief commit.
- Do not change already-written Phase 1 Result text in the ticket.
- Do not add a second `enter.*` call while testing this phase; enter calls
  replace the todo list.
- Read full test output before reporting pass.
- On structural mismatch, stop and escalate instead of guessing.

## Details

Phase 1 current state to rely on:

- `todoItem` has `Instruction *string json:"instruction,omitempty"`.
- `todoReadPayload` returns `Instruction *string json:"instruction"`.
- `ws.todo.append`, `ws.todo.insert_before`, and `ws.todo.insert_after` accept
  optional nullable `instruction`.
- `ws.todo.read(key)` returns the full payload including nullable instruction.
- Status and order mutations preserve existing instruction values.

Existing rendering behavior to preserve:

- Summary mode shows pending/wip items plus one adjacent context item on each
  side of contiguous active blocks.
- Non-shown runs collapse to a single `...` line.
- `defer` collapses like `done`.
- Full mode shows every item.

Expected example:

```text
- [ ] {prep} Prep - brief
      Read the brief and implement only Phase 2 todo instruction rendering...
```

## Verification Contract

Acceptance requires:

- Focused MCP/session-state tests pass:

```sh
go test ./internal/mcp -count=1 -run 'TestTodo|TestRenderTodos|TestWorkflowManual|TestServeStdioTodo'
```

- Full Go test suite for the tool passes:

```sh
go test ./... -count=1
```

- Whitespace check passes:

```sh
git diff --check
```

No spec closeout is required in this brief commit, but the implementation phase
should update `ai-docs/spec/mcp-tools.md` if the final behavior needs more exact
preview wording than the existing `## Spec Impact` and current spec text.

## References

- [Must] `ai-docs/tickets/ready/260627-feat-todo-item-instructions.md` - source
  ticket; implement only Phase 2.
- [Must] `ai-docs/spec/mcp-tools.md` - session-state todo rendering and
  workflow-manual restoration contracts.
- [Must] `ai-docs/spec/workflow-skills.md` - enter tools replace todo lists and
  todo state is workflow-visible.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP formatter ownership,
  workflow-manual handler coupling, and readable text defaults.
- [Must] `ai-docs/mental-model/workflow-skills.md` - todo/enter workflow
  invariants and lead-implement brief discipline.
- [Must] `agents-plugin/rsrc/impl-playbook.md` - verification and deviation
  discipline.
- [Maybe] `ai-docs/mental-model/documentation-system.md` - use only if doc
  closeout or spec wording becomes ambiguous during implementation.
