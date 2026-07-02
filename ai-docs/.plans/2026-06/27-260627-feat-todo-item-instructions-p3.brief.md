# Brief: 260627-feat-todo-item-instructions Phase 3

## Intent

Implement `Phase 3: Enter-tool instruction producers` for
`260627-feat-todo-item-instructions`: make `ws.enter.implement` populate
focused todo `instruction` prose from the deterministic implementation verdict,
so the active todo list becomes the reachable runbook for the current
implementation path.

Phase 1 added storage/read surfaces. Phase 2 added rendering. This phase only
adds instruction production for `ws.enter.implement`.

## Scope Boundary

Selected scope:

- Ticket: `ai-docs/tickets/ready/260627-feat-todo-item-instructions.md`
- Phase: `Phase 3: Enter-tool instruction producers`
- Branch: reuse current branch `implement/260627-enter-implement-verdict-engine`
  by user policy.

In scope:

- Populate `instruction` on todos derived by `ws.enter.implement`.
- Derive instruction prose from the already-resolved implementation verdict:
  branch action, delegation mode, plan depth, review allocation, and doc mode.
- Include only instructions for tasks reachable under the current verdict.
- Keep todo titles short scan labels and put executable prose in `instruction`.
- Add tests for direct-edit, delegated, lead-only review, partitioned review,
  standard docs, skipped docs, and branch-stop behavior as applicable.

Out of scope:

- Do not change `ws.enter.proceed` instruction production.
- Do not add a separate `execution_steps` field or any runbook list beside
  todos.
- Do not alter the public `ws.enter.implement` input schema unless a test proves
  an existing field cannot express the needed verdict state.
- Do not shrink the `lead-implement` playbook body in this phase; that belongs
  to the related implement-verdict ticket after todo instructions are available.
- Do not change todo rendering, setter schemas, or `ws.todo.read` behavior
  except to adjust tests around the new instruction payloads.

## Caller-Visible Contract

After `ws.enter.implement` succeeds, the session todo list contains the normal
lead-implement task keys plus focused `instruction` strings for reachable work.
The `instruction` values are visible through:

- `ws.todo.read(key)` full JSON payload;
- `ws.todo.list(mode: "full")` full rendering;
- `ws.todo.list()` and `ws.workflow_manual(session_key: ...)` 60-rune previews.

Instructions should be imperative, concrete, and bounded to the current verdict.
They must not describe execution paths the verdict made unreachable.

Examples by verdict:

- Branch `stop`: todo instructions explain the branch safety blocker and avoid
  implementation/edit/review/doc instructions that cannot run.
- Branch `continue`, delegated, survey plan, partitioned review, standard docs:
  prep/edit/review/doc todos each describe only that selected path.
- Direct-edit with lead-only review: edit todo describes direct editing and
  verification; review todo records lead-only rationale rather than reviewer
  dispatch.
- Skipped docs: doc-related todos should say they are skipped and carry the skip
  reason where the todo remains present, or be omitted if the existing derived
  checklist omits unreachable doc tasks.

## Contract Instructions

Primary implementation area:

- `agents-plugin-tool/internal/mcp/session_state.go`
- `agents-plugin-tool/internal/mcp/implement_resolver.go` if verdict-derived
  instruction helpers already live with resolver output or need resolver labels.

Primary test area:

- `agents-plugin-tool/internal/mcp/session_state_test.go`
- Existing enter-implement tests in `agents-plugin-tool/internal/mcp/` if they
  already cover branch action, review allocation, or doc mode.

Implementation constraints:

- Reuse the existing `todoItem.Instruction *string` field; do not add a second
  storage field.
- Build instructions at the same point where `ws.enter.implement` replaces the
  todo list, so all enter-derived todos are written atomically with the agenda.
- Prefer small pure helpers for instruction text so tests can assert exact
  payloads without setting up full MCP transport for every case.
- Keep instruction text stable enough for tests, but do not expose it as a
  separate machine-parseable contract.
- Preserve existing todo keys, ordering, statuses, and titles unless a test
  proves an unreachable todo should be skipped under the existing checklist
  contract.
- Keep `ws.enter.implement` raw verdict and JSON verdict fields compatible with
  Phase 1 behavior.

## Integration Test Instructions

Required coverage:

- Direct-edit case:
  - Call the enter-implement derivation with facts that resolve to direct-edit.
  - Assert the edit todo has a direct-edit instruction and does not mention
    delegated implementer dispatch.
- Delegated case:
  - Assert prep/edit instructions reference brief/plan/delegated implementer
    work according to verdict plan depth.
- Lead-only review case:
  - Assert the review instruction records lead-owned review/rationale and does
    not instruct reviewer dispatch.
- Partitioned review case:
  - Assert the review instruction names only the selected partitions.
- Standard docs case:
  - Assert doc todos describe spec/mental-model/ticket closeout as reachable.
- Skipped docs case:
  - Assert doc instructions are skipped with the policy reason or unreachable
    doc todos are absent according to the existing checklist shape.
- Branch-stop case:
  - Assert branch/edit/review/doc instructions do not imply source edits should
    continue before the blocker is resolved.
- Rendering integration:
  - Assert at least one `ws.enter.implement` todo instruction is readable through
    `ws.todo.read` and rendered in `ws.todo.list(mode: "full")`.

Run commands from `agents-plugin-tool/`:

```sh
go test ./internal/mcp -count=1 -run 'TestEnterModeReplacesTodos|TestTodo|TestRenderTodos|TestWorkflowManual|TestServeStdioTodo'
go test ./... -count=1
```

Run from repository root:

```sh
git diff --check
```

## Implementation Strategy Decisions

- Treat `ws.enter.implement` as the producer boundary. The playbook should not
  append instructions later with `ws.todo.*`; a second `enter.*` call would
  replace the list again.
- Derive instructions from verdict labels, not from freeform raw verdict text.
- Keep all instruction production private to MCP helper code. The public MCP
  surface stays the single `ws.enter.implement` mode-switch call.
- Use exact, short prose per todo. The rendering layer already handles preview
  truncation, so producer text can be full instruction prose.
- Leave `lead-implement` body reduction to
  `260627-feat-enter-implement-deterministic-verdict-engine` after this phase
  lands.

## Rejected Alternatives

- Adding `execution_steps` beside todos is rejected by ticket decision; todo
  items are the single source for reachable runbook steps.
- Producing instructions in the playbook with setter calls is rejected because
  `enter.*` is the mode-switch and todo-replacement boundary.
- Generating `ws.enter.proceed` instructions is rejected for this phase because
  the ticket explicitly defers proceed unless a separate ticket requests it.
- Making titles long enough to carry executable prose is rejected; titles stay
  scan labels.

## Approach

- Locate the current `ws.enter.implement` todo derivation and resolver result
  structs.
- Add a helper that maps the resolved implement verdict to instruction strings
  for the existing todo keys.
- Keep branch-stop behavior explicit: edit/review/doc instructions must not
  tell the lead to proceed past a stop verdict.
- Add focused unit tests around the pure derivation helper first, then add one
  MCP/session integration assertion for read/render visibility.
- Commit implementation separately from this brief.

## Constraints

- AI-authored docs, tests, and commit messages must be in English.
- Do not edit source or tests in the prep/brief commit.
- Do not change already-written Phase 1 or Phase 2 Result text in the ticket.
- Do not add a second `enter.*` call in a single skill invocation.
- Read full test output before reporting pass.
- Stop and escalate if existing checklist shape makes a required reachable vs
  unreachable distinction ambiguous.

## Details

Relevant existing behavior:

- `ws.enter.implement` stores the implement agenda and replaces the todo list.
- The MCP resolver owns branch action, delegation, plan depth, review
  allocation, need-review, and doc mode.
- `todoItem` already has `Instruction *string`.
- `ws.todo.list` summary/workflow manual renders previews, and full mode renders
  full instruction text.
- `ws.todo.read(key)` returns full payloads including instruction.

Expected instruction style:

```text
Dispatch correctness, fit, and test reviewers with the implemented commit range.
Relay only non-clean Critical/Important findings back through the implementer.
```

Not:

```text
If partitioned, dispatch reviewers; if lead-only, skip reviewers; if direct...
```

The latter repeats unreachable branches and defeats the phase goal.

## Verification Contract

Acceptance requires:

- Focused MCP/session tests pass:

```sh
go test ./internal/mcp -count=1 -run 'TestEnterModeReplacesTodos|TestTodo|TestRenderTodos|TestWorkflowManual|TestServeStdioTodo'
```

- Full Go test suite for the tool passes:

```sh
go test ./... -count=1
```

- Whitespace check passes:

```sh
git diff --check
```

## References

- [Must] `ai-docs/tickets/ready/260627-feat-todo-item-instructions.md` - target
  ticket and Phase 3 acceptance boundary.
- [Must] `ai-docs/tickets/ready/260627-feat-enter-implement-deterministic-verdict-engine.md` -
  first consumer and follow-up playbook-reduction direction.
- [Must] `ai-docs/spec/mcp-tools.md` - session-state, enter-tool, and todo
  instruction contracts.
- [Must] `ai-docs/spec/workflow-skills.md` - lead-implement verdict ownership
  and review/doc workflow contracts.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP runtime change recipes and
  enter/todo ownership boundaries.
- [Must] `ai-docs/mental-model/workflow-skills.md` - workflow skill ownership,
  one-enter-call rule, and lead-implement boundaries.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` -
  migration anchor for playbook-factory and native-subagent direction.
