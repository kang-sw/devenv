---
title: Todo check adjacent instruction rendering
sage-review: skipped
spec:
  - 260625-session-state-tools
---

# Todo check adjacent instruction rendering

## Background

`ws.todo.check` currently returns only a compact confirmation after marking an
item complete. Once todo items can carry long `instruction` text, relying on the
LLM to remember to call `ws.todo.list` or `ws.todo.read` after each checkpoint
loses the immediate guidance that makes the todo runbook useful.

The desired behavior is a focused checkpoint rendering mode built into
`ws.todo.check` itself. After a todo item is checked, the tool should return the
confirmation plus a todo checkpoint rendering. The checkpoint rendering should
show the whole todo list compactly, but include full `instruction` text only for
actionable todo items immediately adjacent to the checked item, meaning the
previous and next list positions after the status update.

The purpose is to guide the next actionable instruction without requiring an
extra autonomous tool call and without turning every checkpoint into a full
workflow manual.

## Doctrine

State-changing workflow tools should return the next required execution guidance
when omitting it would force the LLM to remember a follow-up tool call. This is
the same guardrail family as commit-time todo reminders: do not make workflow
continuity depend on tool-use initiative.

## Spec Impact

Contract-first spec: no. The existing todo rendering contract in
`260625-session-state-tools` already defines `ws.todo.check`, todo
instructions, summary rendering, full rendering, and commit reminder rendering.
Implementation should update that contract after behavior changes:

- `ws.todo.check` no longer returns only a compact confirmation; successful
  status changes also return checkpoint todo rendering.
- Do not add a `format: json` scheme for this checkpoint response. The tool is
  intentionally raw/text-only so the model sees the next instruction without
  preferring or unpacking structured output.
- Summary/full rendering for `ws.todo.list`, workflow-manual restoration, and
  commit reminders remain separate contracts unless the implementation finds a
  reusable renderer beneath them.
- The checkpoint rendering contract is caller-visible and belongs in
  `ai-docs/spec/mcp-tools.md`.

## Phases

### Phase 1: Return focused checkpoint rendering from todo.check

Change successful `ws.todo.check` responses so the tool returns both the compact
status confirmation and a focused checkpoint todo rendering:

- Base adjacency on the stable todo order after the status update.
- Consider only the checked item's immediate previous and next list positions.
- Render the full `instruction` only for adjacent actionable items. Actionable
  means `pending` or `wip`; `done` and `defer` adjacent items stay compact.
- Render every non-adjacent item compactly with its key, title, and status
  marker.
- Keep the full ordered todo list visible in the checkpoint output; do not use
  summary ellipses for this checkpoint renderer.
- Omit instruction lines for adjacent actionable items with absent, null, or
  empty instructions.
- Preserve existing item mutation behavior: `check` changes only the target
  status and does not rewrite untouched todo payloads.
- Keep `ws.todo.check` raw/text-only for this checkpoint output; do not add a
  `format: json` parameter or parallel structured response path.
- Update the MCP schema description and the todo rendering spec/mental model to
  describe the new checkpoint output.
- Cover edge cases for first item, last item, adjacent completed items, adjacent
  deferred items, missing instruction values, and checking an item to each valid
  status.

Verification boundary: focused MCP/session-state tests cover checkpoint
rendering, existing summary/full rendering tests still pass, `go test
./internal/mcp -count=1` passes, `spec_index.verify` passes, and `git diff
--check` passes.
