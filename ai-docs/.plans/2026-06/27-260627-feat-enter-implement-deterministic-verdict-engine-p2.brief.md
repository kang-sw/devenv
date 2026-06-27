# Brief: 260627-feat-enter-implement-deterministic-verdict-engine Phase 2

## Intent

Focus `lead-implement` on executing the current `ws.enter.implement` verdict by moving reachable execution guidance into todo item instructions and removing always-rendered prose for paths that cannot be reached under the current verdict.

## Scope Boundary

Implement only Phase 2: `Focus lead-implement on reachable todo instructions` from `260627-feat-enter-implement-deterministic-verdict-engine`.

In scope:
- Refine `ws.enter.implement` todo instruction prose where needed so branch, prep, edit, review, doc, final-action, and merge todos are enough to guide the reachable path.
- Reduce `agents-plugin/rsrc/lead-implement/lead-implement.md` so the always-rendered body keeps fact gathering, verdict handoff, ambiguous execution judgments, delegate prompt templates, and safety gates, but stops carrying detailed if/then prose for unreachable post-verdict branches.
- Preserve wsflow behavior by regenerating the canonical manifest and byte-identical wsflow rsrc mirror after shared rsrc edits.

Out of scope:
- Do not change `ws.enter.proceed`.
- Do not add a second `execution_steps` result or any parallel runbook list.
- Do not add another public MCP helper outside `ws.enter.implement`.
- Do not change branch cleanup, merge policy, or ticket phase shape.

## Caller-Visible Contract

After `ws.enter.implement(format: "json")`, the authoritative executable runbook is the session todo list with full `instruction` payloads. `lead-implement` still calls `ws.enter.implement`, follows the raw `Next:` instruction, and executes todos in order, but the playbook body no longer exposes detailed delegated/direct, review, or doc branch prose that the verdict has already made unreachable.

Expected visible outcomes:
- Direct-edit render does not force readers through delegated implementer instructions in the always-rendered `lead-implement` body.
- Lead-only review render does not force readers through reviewer relay loops in the always-rendered body.
- Skipped-doc verdicts do not require always-rendered doc-pipeline instructions to know what to do; the todo instructions carry the skip guidance.
- `ws.todo.read(key)` and `ws.todo.list(mode: "full")` expose the full focused instruction for reachable todos.

## Contract Instructions

- Keep `agents-plugin/rsrc/lead-implement/lead-implement.md` as the shared source. Mirror changes through `agents-plugin-wsflow/rsrc/` only by regeneration, not manual divergence.
- Use `{{.McpNamespace}}/` and `{{.SkillNamespace}}:` in playbook text. Do not hard-code `ws/` for shared playbook calls.
- Keep Route schema and fact guidance in the playbook; the LLM still owns ambiguous fact gathering.
- Keep delegate dispatch and prompt templates in the playbook; todo instructions may point to these named templates instead of duplicating their full bodies.
- Keep review convergence judgments lead-owned: test failures, reviewer findings, blocker classification, fixed/wont-fix/deferred disposition, and merge approval remain execution-time judgments.
- Do not call `ws.enter.implement` a second time after plan or branch execution.

## Integration Test Instructions

Extend existing Go tests under `agents-plugin-tool/internal/mcp/`.

Required checks:
- `TestPlaybookPrintGoldenLeadImplement` or adjacent tests should prove the always-rendered `lead-implement` body no longer contains detailed unreachable-path prose targeted by this phase, while still containing required route/verdict/delegate template anchors.
- Add or extend `ws.enter.implement` todo tests so direct-edit, lead-only review, skipped-doc, and branch-stop verdicts have focused full instructions available through `ws.todo.read` or `ws.todo.list(mode: "full")`.
- Preserve wsflow product-mode hiding for full-ws-only mercenary text.

Run:
- `go test ./internal/mcp -count=1 -run 'TestEnterModeReplacesTodos|TestTodo|TestRenderTodos|TestWorkflowManual|TestServeStdioTodo|TestPlaybookPrintGoldenLeadImplement|TestPlaybookPrintWsflowLeadImplementOmitsMercenaryCommands'`
- `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
- `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
- `go test ./internal/wsrsrc -count=1`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `go test ./... -count=1`
- `git diff --check`
- `ws/spec_index_verify`

## Implementation Strategy Decisions

- The todo list is the runbook. Do not introduce `execution_steps`.
- Use concise always-rendered stage text that says which todo or template to consult, then let todo instructions provide path-specific details.
- Treat the MCP verdict labels and todo instructions as derived state. The playbook should not restate conditional execution matrices for direct/delegated edit, review allocations, doc skip, or branch stop.
- Preserve minimal handler order in `lead-implement`; this phase is a focus reduction, not a flow redesign.
- Use existing `deriveImplementTodosFromVerdict` and instruction helper functions before adding new data structures.

## Rejected Alternatives

- Separate `execution_steps` output was rejected because todo instruction payloads already carry ordered executable prose with stable keys.
- Removing delegate templates from `lead-implement` was rejected because todo instructions need stable named templates to reference, and `lead-implement` still owns actual dispatch.
- Re-running `ws.enter.implement` after plan population was rejected because enter tools replace the todo list and must remain one-shot per invocation.

## Approach

- Inspect current `lead-implement` rendered body and list the conditional post-verdict prose to remove or compress.
- Inspect current `deriveImplementTodosFromVerdict` instruction helpers and fill any missing runbook detail there.
- Update tests first around todo instruction focus and playbook body regression checks where practical.
- Edit the shared rsrc playbook and MCP instruction helpers in the smallest coherent pass.
- Regenerate manifests and wsflow rsrc mirror after rsrc edits.

## Constraints

- `lead-implement` remains readable without relying on source code.
- Todo instructions must stay full-prose enough for `ws.todo.read(key)` to guide execution.
- Summary/workflow-manual todo rendering may remain preview-length; full mode and read expose complete instruction text.
- wsflow distributed text must not contain forbidden full-ws-only references after product-mode rendering.
- Fresh-reader audit is required after skill/playbook text edits.

## Details

Likely edit points:
- `agents-plugin/rsrc/lead-implement/lead-implement.md`
- `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md` via regeneration
- `agents-plugin/rsrc/manifest.json`
- `agents-plugin-wsflow/rsrc/manifest.json`
- `agents-plugin-tool/internal/mcp/session_state.go`
- `agents-plugin-tool/internal/mcp/session_state_test.go`
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go`

Candidate body reduction:
- Replace detailed branch-action enumerations with one instruction to follow `Next:` and the todo list, while preserving `Branch Action: stop` as a pre-edit blocker.
- Replace detailed Edit mode if/then prose with “execute the `{edit}` todo; use direct edit or delegate template according to its instruction.”
- Replace detailed Review allocation branches with “execute the `{review}` todo when present; use review templates and lead-owned clean judgment.”
- Replace detailed Doc skip/standard branches with “execute doc todos when present; absence means verdict-derived skip.”
- Keep Final Action Gate and Merge user-approval boundaries explicit.

## Verification Contract

Acceptance requires:
- Focused todo instruction tests pass for direct-edit, lead-only review, skipped-doc, and branch-stop paths.
- Lead-implement body tests prove removed prose does not regress.
- Manifest and wsflow mirror tests pass after regeneration.
- Fresh-reader audit on the edited `lead-implement` body reports no fix-class blockers or fix-class findings have been addressed.

## References

- [Must] `ai-docs/tickets/ready/260627-feat-enter-implement-deterministic-verdict-engine.md` - selected Phase 2 scope and acceptance criteria.
- [Must] `ai-docs/spec/workflow-skills.md` - lead-implement ownership, verdict timing, todo/runbook boundary.
- [Must] `ai-docs/spec/mcp-tools.md` - enter.implement todo replacement, instruction payload, and rendering contracts.
- [Must] `ai-docs/mental-model/workflow-skills.md` - playbook and lead-implement behavioral model.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - session todo instruction and enter-tool ownership model.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - migration anchor for playbook/native-subagent boundary work.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow shared rsrc mirror requirements.
- [Must] `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md` - skill/playbook authoring and fresh-reader audit rules.
- [Maybe] `ai-docs/mental-model/prompt-bundle.md` - rsrc playbook source and manifest context.
- [Maybe] `ai-docs/mental-model/git-workflow-tools.md` - commit/final action wording context.
- [Maybe] `ai-docs/tickets/ready/260627-feat-enter-proceed-deterministic-verdict-engine.md` - predecessor deterministic verdict pattern.
- [Maybe] `ai-docs/tickets/todo/260523-bug-implement-merge-target-discovery.md` - branch safety context.
- [Maybe] `ai-docs/tickets/idea/260525-bug-lead-implement-delegation-pre-edit-guard.md` - pre-edit delegation safety context.
