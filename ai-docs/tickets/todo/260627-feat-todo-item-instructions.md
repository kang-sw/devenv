---
title: Todo item instruction field and focused runbook rendering
sage-review: recommended
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260625-feat-ws-session-state-machine: owns persisted session todo state and enter-tool todo replacement
  260627-feat-enter-implement-deterministic-verdict-engine: first consumer; lead-implement should emit focused reachable instructions through todo items
related-mental-model:
  - mcp-runtime
  - workflow-skills
---

# Todo item instruction field and focused runbook rendering

## Background

Typed `ws.enter.*` calls now replace the session todo list when switching
workflow modes. That makes the todo list the natural single source for the
current run's executable steps.

`lead-implement` exposed the next limitation: after `ws.enter.implement`
resolves routing facts into a verdict, the playbook still carries prose for
execution paths that the current verdict cannot reach. A separate
`execution_steps` return field would duplicate the todo list. The better shape
is to extend todo items so each reachable task can carry the precise instruction
for this run.

## Decisions

- Add a nullable optional `instruction` field to todo items.
- Keep `title` as a short scan label.
- Use `instruction` for focused execution prose generated after routing facts
  are resolved.
- Treat the todo list as the single source for reachable workflow runbook steps.
- Do not add a separate `execution_steps` list beside todos.
- Preserve compatibility with existing session records that lack
  `instruction`.

## Contract Sketch

Todo item shape:

```json
{
  "key": "review",
  "title": "Review - partitioned",
  "status": "pending",
  "instruction": "Dispatch correctness, fit, and test reviewers. Relay only non-clean partitions back through the implementer."
}
```

Rendering shape:

```text
- [ ] {key} Review - partitioned
      Dispatch correctness, fit, and test reviewers. Relay only non-clean...
```

Rendering policy:

- `ws.todo.list` summary/default mode renders at most a 60-character instruction
  preview when `instruction` is present.
- Checkpoint reminders, including todo text rendered through
  `ws.workflow_manual(session_key: ...)`, use the same 60-character preview.
- `ws.todo.list(mode: "full")` renders the full instruction text for every todo.
- `ws.todo.read(key: "<key>")` returns the full todo payload, including the full
  instruction.
- `instruction` absent, null, or empty renders no second line.

Setter/getter policy:

- Todo setter surfaces that create or mutate todo items accept optional nullable
  `instruction`.
- Todo getter surfaces return `instruction` when present.
- Existing callers that only send `key`, `title`, and `status` keep working.
- Validation should reject non-string `instruction` values except null.

## Spec Impact

Target spec areas:

- `ai-docs/spec/mcp-tools.md`: todo item schema, setter/getter behavior,
  `ws.todo.read`, and rendering modes.
- `ai-docs/spec/workflow-skills.md`: enter-tool todo replacement as the single
  source for focused reachable runbook steps.

Ready-addressing note:

- Phase 1 changes the shared MCP/session-state contract but does not need a
  contract-first `🚧` spec entry before implementation; implementation can update
  the existing session-state and workflow-skill spec sections with the exact
  landed behavior.
- The setter/read surfaces and backward-compatibility verification are pinned in
  the phase requirements below.

## Phases

### Phase 1: Todo instruction field and read surface

Add `instruction` to session todo items and expose full-instruction reads.

Required behavior:

- Extend persisted todo item records with optional nullable `instruction`.
- Update todo creation setters that write item payloads to accept optional
  nullable `instruction`: `ws.todo.append`, `ws.todo.insert_before`, and
  `ws.todo.insert_after`.
- Keep non-payload status/order setters (`ws.todo.check`, `ws.todo.erase`,
  `ws.todo.clear`, and `ws.todo.reorder`) behaviorally unchanged except that
  they must preserve any existing `instruction` value on untouched items.
- Update todo getter/list paths to preserve and return `instruction` in
  structured payloads.
- Add `ws.todo.read(key)` for reading a single todo's full payload, including
  `key`, `title`, `status`, and nullable `instruction`.
- Keep absent/null `instruction` backward-compatible for old session files.
- Reject non-string non-null `instruction` inputs with a clear validation error.

Verification:

- Unit-test old todo records without `instruction`.
- Unit-test append, insert-before, and insert-after flows with and without
  `instruction`.
- Unit-test status/order mutations preserve existing `instruction` values.
- Unit-test `ws.todo.read(key)` success and missing-key errors.

### Phase 2: Todo instruction rendering

Render instruction text consistently across todo surfaces.

Required behavior:

- Render summary/default todo lists with a 60-character instruction preview.
- Render workflow-manual session todo reminders with the same preview.
- Render `ws.todo.list(mode: "full")` with full instruction text.
- Use the indented second-line shape for instruction text.
- Do not render an instruction line when the field is absent, null, or empty.

Verification:

- Unit-test summary/default rendering truncation.
- Unit-test full rendering with untruncated instruction text.
- Unit-test `ws.workflow_manual(session_key: ...)` todo rendering uses previews.

### Phase 3: Enter-tool instruction producers

Make enter tools populate focused instructions where deterministic routing facts
already determine the reachable path.

Required behavior:

- Update `ws.enter.implement` todo derivation to populate instructions from the
  implementation verdict.
- Include only instructions for tasks reachable under the current verdict.
- Keep `title` short and put prose in `instruction`.
- Ensure lead-implement can follow todo instructions without carrying unreachable
  direct/delegated/review/doc path prose in its always-rendered body.

Deferred scope:

- Do not change `ws.enter.proceed` unless a separate ticket requests focused
  proceed todo instructions.

Verification:

- Unit-test `ws.enter.implement` todo instructions for direct-edit, delegated,
  lead-only review, partitioned review, standard docs, and skipped docs cases.
- Verify lead-implement rendered text no longer needs duplicated unreachable
  execution branches after it depends on todo instructions.
