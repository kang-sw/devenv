# Survey: 260627-feat-todo-item-instructions Phase 3

## Outcome
- `[ok]` - survey is sufficient; no `[escalate-to-research]` needed. The implementation seam is already present at `ws.enter.implement` todo replacement.

## Source/Test Mapping
- `agents-plugin-tool/internal/mcp/session_state.go#L356-L399` -> `agents-plugin-tool/internal/mcp/session_state_test.go#L42-L82` - implement todo derivation owns keys, order, titles, and pending status; extend this seam to add instruction payloads while preserving shape.
- `agents-plugin-tool/internal/mcp/session_state.go#L723-L763` -> `agents-plugin-tool/internal/mcp/session_state_test.go#L1215-L1264` - new `target/facts/policy` `ws.enter.implement` path resolves the verdict, stores agenda, and atomically writes derived todos; add read/render assertions after this call.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L420-L472` -> `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L8-L45` - resolver emits the labels instruction text must consume: delegation, branch plan, plan depth, review allocation, need review, and doc mode.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L585-L649` -> `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L47-L159` - branch stop and doc skip behavior already have resolver coverage; todo-instruction tests should reuse those result shapes instead of recomputing policy.
- `agents-plugin-tool/internal/mcp/session_state.go#L333-L351` -> `agents-plugin-tool/internal/mcp/session_state_test.go#L269-L321` - renderer already handles nil/empty/full/preview instruction rendering; producer tests only need prove enter-derived items carry non-empty strings.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1370-L1477` and `agents-plugin-tool/internal/mcp/session_state_test.go#L1837-L1865` - read/list/workflow-manual instruction surfaces already exist; add one enter-implement integration assertion rather than duplicating all rendering cases.

## Existing Mechanisms To Reuse
- `agents-plugin-tool/internal/mcp/session_state.go#L33-L48` - reuse `todoItem.Instruction *string` and `todoReadPayload.Instruction`; do not add a second field or derive full instruction from rendered previews.
- `agents-plugin-tool/internal/mcp/session_state.go#L379-L399` - keep `deriveImplementTodosFromVerdict` as the producer boundary for enter-derived todos; it already receives the needed verdict labels.
- `agents-plugin-tool/internal/mcp/session_state.go#L439-L483` - reuse title helpers for short scan labels; instruction text should be separate prose and should not lengthen titles.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L636-L649` - use this as style/semantics reference for branch-action-specific prose, especially the branch-stop pre-edit blocker.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L119-L132` - `implementAgenda` already carries `DocReason`; if skipped-doc instructions need the reason, pass it through the todo verdict helper instead of parsing raw text.
- `agents-plugin-tool/internal/mcp/session_state.go#L587-L598` and `agents-plugin-tool/internal/mcp/session_state.go#L700-L720` - existing enter writes are atomic record mutations; keep instruction production before the single `enterMode` write.

## Implementation Steps
- `agents-plugin-tool/internal/mcp/session_state.go#L356-L362` - extend `implementTodoVerdict` with the minimum missing labels needed for instruction text: branch plan/action and doc skip reason are the likely additions.
- `agents-plugin-tool/internal/mcp/session_state.go#L379-L399` - add small pure helpers that attach stable instruction strings per existing todo key; branch-stop should produce blocker-focused instructions and avoid edit/review/doc continuation wording.
- `agents-plugin-tool/internal/mcp/session_state.go#L749-L755` - pass `result.Verdict.BranchPlan` and `result.Agenda.DocReason` into todo derivation on the new schema path.
- `agents-plugin-tool/internal/mcp/session_state.go#L766-L790` - decide deliberately whether the legacy `need_review`/`need_doc` path remains instruction-free or gets compatible fallback instructions; preserve current legacy tests either way.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L42-L82` - add focused pure derivation tests for direct-edit, delegated survey plan, lead-only review, partitioned review, standard docs, skipped docs, and branch-stop strings.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1215-L1290` - extend enter-implement integration tests to read one generated todo through `ws.todo.read` and render full mode through `ws.todo.list(mode: "full")`.

## Risks
- `agents-plugin-tool/internal/mcp/session_state.go#L424-L435` - legacy review allocation parsing collapses `partitioned: correctness` variants to plain `partitioned`; do not reuse this legacy path when testing partition names because the new resolver preserves the full partition list.
- `agents-plugin-tool/internal/mcp/session_state.go#L766-L790` - possible compatibility risk: legacy `ws.enter.implement` inputs still derive todos without resolver agenda, branch plan, or doc reason, so exact producer coverage should focus on the new `target/facts/policy` path.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L636-L649` - possible contract risk: instruction prose can drift from `NextInstruction`; keep both branch-stop and branch-create/continue/rename wording semantically aligned without making instruction text a machine contract.
- `agents-plugin-tool/internal/mcp/session_state.go#L388-L393` - skipped docs currently omit doc todos because `NeedDoc` is false; tests should assert absence unless implementation intentionally keeps a doc todo with a skip reason.
- `ai-docs/tickets/ready/260627-feat-todo-item-instructions.md#L187-L210` - ticket asks only `ws.enter.implement`; do not add `ws.enter.proceed` producers or shrink `lead-implement` playbook body in this phase.

## Constraints From References
- `ai-docs/spec/mcp-tools.md#L220-L242` - `ws.enter.implement` stores the implement agenda and replaces todos in one mode-switch call; branch `stop` must block source edits.
- `ai-docs/spec/mcp-tools.md#L261-L291` - todo instructions are durable full-prose payloads, summary/manual render previews are 60 runes, and `ws.todo.read` is the full payload surface.
- `ai-docs/mental-model/mcp-runtime.md#L85-L90` - changing session todos or enter tools requires checking derived todo builders and render/session-state behavior together.
- `ai-docs/mental-model/mcp-runtime.md#L109-L112` - do not rederive final implementation strategy outside `ws.enter.implement`.
- `ai-docs/mental-model/workflow-skills.md#L75-L81` - plan-populator survey should stop for strategy risks; none found for this slice.
- `ai-docs/mental-model/workflow-skills.md#L118-L120` - do not add a second `enter.*` call; extend state through the existing todo derivation.

## Verification Commands
- From `agents-plugin-tool/`: `go test ./internal/mcp -count=1 -run 'TestEnterModeReplacesTodos|TestTodo|TestRenderTodos|TestWorkflowManual|TestServeStdioTodo'`
- From `agents-plugin-tool/`: `go test ./... -count=1`
- From repo root: `git diff --check`
