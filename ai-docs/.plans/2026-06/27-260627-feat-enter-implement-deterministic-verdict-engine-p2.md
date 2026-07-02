# Survey: 260627-feat-enter-implement-deterministic-verdict-engine Phase 2

## Reusable Components

- `agents-plugin-tool/internal/mcp/session_state.go#L382-L403` — `deriveImplementTodosFromVerdict`: central producer for implement todo keys, titles, order, and instruction payloads; use this seam before adding any parallel runbook list.
- `agents-plugin-tool/internal/mcp/session_state.go#L492-L600` — implement instruction helpers: existing branch, prep, edit, review, doc, final-action, and merge prose helpers already consume verdict labels and branch/doc reasons.
- `agents-plugin-tool/internal/mcp/session_state.go#L161-L176` and `agents-plugin-tool/internal/mcp/session_state.go#L1124-L1155` — `ws.todo.read` / `ws.todo.list`: full instruction read and rendering surfaces already exist for acceptance checks.
- `agents-plugin-tool/internal/mcp/session_state.go#L881-L924` — new-schema `ws.enter.implement`: resolves verdict, derives todo instructions, stores agenda, replaces todos, and returns raw or JSON verdict from the same call.
- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L90-L105` and `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L83-L113` — env-gated regeneration tests for `agents-plugin/rsrc/manifest.json` and the byte-identical `agents-plugin-wsflow/rsrc/` mirror.
- `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md#L93-L105` — Fresh-Reader Audit procedure required after editing `agents-plugin/rsrc/lead-*/lead-*.md` playbook text.

## Existing Patterns

- Render regression tests: see `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1423-L1476` — current `lead-implement` golden/wsflow tests assert required full-ws mercenary anchors and wsflow omission; adjacent negative assertions can cover removed unreachable prose.
- Enter-derived instruction tests: see `agents-plugin-tool/internal/mcp/session_state_test.go#L103-L148` and `agents-plugin-tool/internal/mcp/session_state_test.go#L150-L230` — pure helper tests already cover direct-edit, delegated survey, lead-only review, partitioned review, standard/skipped docs, and branch-stop wording.
- Full payload integration: see `agents-plugin-tool/internal/mcp/session_state_test.go#L1365-L1425` — new-schema `ws.enter.implement` already asserts JSON/raw output, agenda storage, todo replacement, `ws.todo.read`, and `ws.todo.list(mode: "full")`.
- Todo rendering substrate: see `agents-plugin-tool/internal/mcp/session_state_test.go#L420-L455` and `agents-plugin-tool/internal/mcp/session_state_test.go#L1532-L1639` — summary mode previews instructions while full/read surfaces expose complete prose.
- Product-mode hiding: see `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1453-L1476` and `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L81-L119` — wsflow checks guard forbidden full-ws references in skill shims and rendered lead-implement output.
- Prior prerequisite survey: see `ai-docs/.plans/2026-06/27-260627-feat-todo-item-instructions-p3.md#L14-L35` — earlier map identified the same producer boundary and legacy-path risk; current code has since implemented those helpers.

## Relevant Interfaces

- `agents-plugin-tool/internal/mcp/implement_resolver.go#L12-L17` and `agents-plugin-tool/internal/mcp/implement_resolver.go#L78-L89` — `implementInput` / `implementResult`: JSON `target + facts + policy + format` input and structured `verdict`, `next_instruction`, `agenda`, `todo_replaced`, and `raw` output.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L100-L117` and `agents-plugin-tool/internal/mcp/implement_resolver.go#L438-L469` — `implementVerdict` and resolver assembly: source of delegation, branch plan, plan depth, review allocation, need-review, and doc mode consumed by todo derivation.
- `agents-plugin-tool/internal/mcp/server.go#L2672-L2756` — public `ws.enter.implement` schema: required `session_key` + `target`, optional grouped facts/policy, and `format: text|json`.
- `agents-plugin-tool/internal/mcp/server.go#L2923-L2946` — public todo surfaces: `ws.todo.list(mode: "full")` and `ws.todo.read(key)` are the contract surfaces for full focused instructions.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L35-L111` — current Route/Follow Verdict/Prep body: preserves fact gathering and verdict handoff, but still contains branch-action and plan-depth execution prose targeted by this phase.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L113-L160` — current Edit/Review/Doc body: contains direct/delegated, review-allocation, and doc-skip conditional prose that the brief says should move behind reachable todo instructions.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L210-L239` and `agents-plugin/rsrc/lead-implement/lead-implement.md#L307-L394` — delegate dispatch, implementer, reviewer, relay, and re-review templates that must remain available for todo instructions to reference.

## Constraints

- `ai-docs/tickets/ready/260627-feat-enter-implement-deterministic-verdict-engine.md#L389-L423` — Phase 2 scope is focused todo instructions plus lead-implement body reduction; verification must prove direct-edit, lead-only review, skipped-doc, and full todo instruction behavior.
- `ai-docs/spec/mcp-tools.md#L218-L246` — enter tools replace the todo list atomically; `ws.enter.implement` owns focused todo instructions and branch-stop blocker wording.
- `ai-docs/spec/workflow-skills.md#L453-L484` — `lead-implement` gathers facts, calls `ws.enter.implement`, follows MCP's verdict/`Next:`, and does not recompute deterministic labels.
- `ai-docs/spec/workflow-skills.md#L486-L539` — review convergence, plan-populator escalation, and execution-time clean/blocker judgments remain lead-owned.
- `ai-docs/mental-model/mcp-runtime.md#L49-L50` and `ai-docs/mental-model/mcp-runtime.md#L88-L89` — todo `instruction` is durable full-prose payload; changing enter/todo behavior requires checking builders, renderers, and session-state behavior together.
- `ai-docs/mental-model/workflow-skills.md#L69-L81` — one `enter.*` call per skill instance; survey escalation routes to research before implementation and replaces the same plan artifact.
- `ai-docs/ref/wsflow-mirroring.md#L104-L134` — `agents-plugin-wsflow/rsrc/` must stay a generated byte-identical copy of canonical `agents-plugin/rsrc/`; do not hand-edit the mirror.
- `ai-docs/mental-model/prompt-bundle.md#L53-L56` and `ai-docs/mental-model/prompt-bundle.md#L64-L75` — rsrc text edits require manifest and wsflow mirror regeneration, not `runtime.json` edits.
- `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md#L393-L416` — `lead-implement` remains an internal playbook invoked through `lead-proceed`, not a user-exposed entry skill.

## Risk Signals

- `agents-plugin/rsrc/lead-implement/lead-implement.md#L19-L24` — Possible contract risk: invariant still says "Create the task list during Prep" even though `ws.enter.implement` replaces the todo list during Route; lead/planner should inspect wording during body reduction.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L90-L111` — Possible shortcut risk: Follow Verdict/Prep still has explicit branch-action and plan-depth prose; editing only Go todo helpers would leave always-rendered unreachable-path guidance in place.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L113-L160` — Possible contract risk: current Edit/Review/Doc sections still render direct/delegated, reviewer relay, and doc-skip branches for every verdict; this is the visible behavior the phase intends to remove or compress.
- `agents-plugin-tool/internal/mcp/session_state.go#L526-L563` — Possible reuse risk: edit/review todo instructions are concise and mention dispatch/relay but not the named template anchors; if the playbook body is compressed heavily, instructions may need explicit references to existing template names instead of duplicating bodies.
- `agents-plugin-tool/internal/mcp/session_state.go#L927-L951` and `agents-plugin-tool/internal/mcp/session_state_test.go#L1303-L1363` — Possible test risk: legacy `ws.enter.implement` arguments still populate todos without the new resolver's branch plan/doc reason; Phase 2 coverage should exercise the required `target + facts + policy` path for verdict-specific full instructions.
- `ai-docs/ref/wsflow-mirroring.md#L113-L120` — Possible artifact risk: regeneration tests are env-gated and require `-count=1`; omitting it can report cached success while leaving manifest or wsflow mirror stale.

## Opinion

- `agents-plugin-tool/internal/mcp/session_state.go#L382-L403` and `agents-plugin/rsrc/lead-implement/lead-implement.md#L90-L160` — Survey is sufficient; no research escalation is needed. The implementation seam exists, and the remaining uncertainty is prose calibration plus fresh-reader audit, not mechanism selection.
