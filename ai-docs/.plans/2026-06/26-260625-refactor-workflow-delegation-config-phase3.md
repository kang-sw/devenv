# Survey: 26-260625-refactor-workflow-delegation-config-phase3

## Reusable Components

- `agents-plugin-tool/internal/mcp/server.go#L1606-L1669` — `buildTuningCatalog`: scans shipped override markers, projects prompt knobs, adds `"workflow.prefer_subagent"`, and returns before full-ws-only mercenary/tier knobs in no-agent mode.
- `agents-plugin-tool/internal/mcp/server.go#L1717-L1766` — `tuningFieldsFromSchema` / `toolInputSchemaDetails`: existing schema-backed projection for field names, enum values, and required flags.
- `agents-plugin-tool/internal/mcp/server.go#L504-L538` — workflow preference writers: `config.workflow_prefer_subagent` accepts `on|off`, `config.workflow_prefer_mercenary` accepts `on|off|hide`, both require lead authority and write global config.
- `agents-plugin-tool/internal/wsconfig/scope.go#L62-L99` — global-only registry: `"workflow.prefer_subagent"` and `"workflow.prefer_mercenary"` are registered as global-only with global default write scope.
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L760-L812` — pragmatic playbook concatenation hook: wraps already-rendered append content and appends `lead-prefer-subagent` to `lead-workflow-manual` only from global `"workflow.prefer_subagent"`.
- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L17-L44` — shipped manifest drift guard and regen command for canonical `agents-plugin/rsrc/manifest.json`.
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L88` — byte-identical wsflow rsrc mirror drift guard and env-gated regeneration entrypoint.

## Existing Patterns

- Catalog full-mode expectations: see `agents-plugin-tool/internal/mcp/prompt_override_test.go#L990-L1069` — asserts prompt knobs, workflow preference knobs, writer tools, and schema-derived enum values.
- Catalog no-agent expectations: see `agents-plugin-tool/internal/mcp/prompt_override_test.go#L1071-L1100` — keeps `"workflow.prefer_subagent"` and omits `"workflow.prefer_mercenary"`, legacy `delegation.prefer_mercenary`, and `agents.tier`.
- Removed prompt marker discovery: see `agents-plugin-tool/internal/mcp/prompt_override_test.go#L896-L922` — shipped `config.tuning` omits `prompt.DelegationSection` while preserving current shipped prompt knobs.
- Workflow manual append production path: see `agents-plugin-tool/internal/mcp/prefer_mercenary_phase2_test.go#L229-L289` — proves keyless manual print observes global `"workflow.prefer_subagent"` and the XML playbook wrapper.
- wsflow product-mode render checks: see `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L571-L628` — validates wsflow manual output stays namespace-clean and still appends prefer-subagent when enabled.
- No-agent tool visibility: see `agents-plugin-tool/internal/mcp/server_test.go#L1186-L1224` and `agents-plugin-tool/internal/mcp/mercenary_surface_test.go#L321-L349` — hides mercenary/tier tools while keeping `config.tuning` and `config.workflow_prefer_subagent`.
- Full ws mercenary visibility: see `agents-plugin-tool/internal/mcp/server_test.go#L1968-L2006` — default hides `ws.mercenary.*`, keeps `config.workflow_prefer_mercenary`, and exposes mercenary tools after enabling.
- wsflow package tests: see `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L91-L117` and `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L79-L117` — exact runtime capabilities and thin-shim/shared-playbook checks.

## Relevant Interfaces

- `agents-plugin/rsrc/lead-tune/lead-tune.md#L25-L40` — invoke and prompt-override handlers already require loading `config.tuning` first and using catalog-provided writer/field metadata.
- `agents-plugin/rsrc/lead-tune/lead-tune.md#L42-L53` — current examples and delegation handler still name `prompt.DelegationSection`, `delegation.prefer_mercenary`, and session-scoped reporting.
- `agents-plugin/rsrc/lead-tune/lead-tune.md#L70-L85` — `judge: tune-target` routes delegation posture to `DelegationSection` and mercenary mode to `delegation.prefer_mercenary`.
- `agents-plugin/skills/lead-tune/SKILL.md#L1-L9` and `agents-plugin-wsflow/skills/lead-tune/SKILL.md#L1-L10` — full ws and wsflow entry shims; wsflow has a product-specific description but shared behavior comes from the rsrc playbook.
- `agents-plugin/runtime.json#L8-L20` and `agents-plugin-wsflow/runtime.json#L11-L21` — full ws contract lists both workflow writers; wsflow lists `config.workflow_prefer_subagent` and omits `config.workflow_prefer_mercenary`.
- `agents-plugin-tool/internal/mcp/server.go#L3276-L3297` and `agents-plugin-tool/internal/mcp/server.go#L3490-L3507` — `LeadToolNames` and `noAgentHiddenTool` enforce runtime-capability and no-agent visibility for workflow tuning tools.
- `agents-plugin-tool/internal/mcp/server.go#L1799-L1848` — text formatter for `config.tuning`, including writer, selector/value field labels, and JSON-marshaled current values.

## Constraints

- `ai-docs/spec/mcp-tools.md#L218-L255` — `config.tuning` is read-only, schema-backed, marker-driven, product-mode-aware, and must not copy writer enums/properties by hand.
- `ai-docs/spec/mcp-tools.md#L208-L216` and `ai-docs/spec/mcp-tools.md#L300-L310` — workflow preference writers require a lead session key for authority but always resolve/write global-only config; old `prefer_mercenary` and `prompt.DelegationSection.*` values remain orphaned.
- `ai-docs/mental-model/prompt-bundle.md#L35-L38` — prompt override discovery is marker-driven, `DelegationSection` is removed, and `config.tuning` derives workflow knobs from writer schemas.
- `ai-docs/mental-model/mcp-runtime.md#L41-L49` — workflow preference visibility and no-agent/runtime-capabilities filtering must stay aligned across explicit calls, tools/list, and `LeadToolNames`.
- `ai-docs/mental-model/workflow-skills.md#L35-L37` — `lead-tune` must treat `config.tuning` as source of truth and keep routing prose compact without duplicating writer enums.
- `ai-docs/ref/wsflow-mirroring.md#L14-L27` and `ai-docs/ref/wsflow-mirroring.md#L104-L121` — shared playbook edits require wsflow product-mode checks, canonical manifest regen, and byte-identical wsflow rsrc mirror regen.
- `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md#L93-L105` — changed skill/playbook text requires a fresh-reader audit whose findings are classified before edits.
- `agents-plugin/rsrc/impl-playbook.md#L3-L12` — implementation must report any spec/doc assumption found wrong by source checks and claim pass only after reading verification output.

## Risk Signals

- `agents-plugin/rsrc/lead-tune/lead-tune.md#L42-L53` — Possible contract risk: current lead-tune text recommends removed `prompt.DelegationSection`, legacy `delegation.prefer_mercenary`, and session-scoped reporting despite Phase 1-2 global-only writer behavior.
- `agents-plugin/rsrc/lead-tune/lead-tune.md#L72-L79` — Possible routing risk: `judge: tune-target` still maps "delegate more/less" to a prompt override rather than `"workflow.prefer_subagent"` and uses the old mercenary knob id.
- `agents-plugin-wsflow/skills/lead-tune/SKILL.md#L1-L10` — Possible product-surface risk: wsflow shim description still frames tuning as prompt-text override only, while wsflow should expose shared config catalog behavior for `"workflow.prefer_subagent"`.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L236-L248` — Possible stale-doc risk: full-only delegation dispatch text still mentions removed `ws.lead.prefer_mercenary`; grep shows the byte-identical wsflow rsrc copy carries the same source text even though product-mode may strip it at render time.
- `agents-plugin-tool/internal/mcp/prompt_override_test.go#L990-L1100` — Possible test risk: existing tests lock catalog contents but not rendered `lead-tune` guidance, so stale lead-tune recommendations can survive while catalog tests stay green.

## Opinion

- Survey found enough existing machinery for implementation without research escalation; the main work is stale rsrc/shim wording plus render/catalog visibility assertions, not a new catalog design.
