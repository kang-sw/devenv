# Survey: 25-260625-lead-tune-tuning-catalog.brief

## Reusable Components
- `agents-plugin-tool/internal/mcp/server.go#L2231-L2409` — `tools()`: canonical MCP input-schema registry with the existing writer schemas for `ws.lead.prefer_mercenary`, `config.agents_tier`, `config.prompt.set`, `config.prompt.unset`, and `config.prompt`.
- `agents-plugin-tool/internal/mcp/server.go#L2194-L2224` — `toolJSONResponse` / `toolTextResponse`: existing MCP response helpers for the required text default plus `format: "json"` escape hatch.
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L349-L386` — `scanOverridePoints`: tree scan used by `config.prompt` to discover declared prompt override markers; this is the shipped point-id authority.
- `agents-plugin-tool/internal/mcp/server.go#L1476-L1540` — prompt listing structs/builders: existing shape for prompt override entries with resolved harness buckets/scopes and compact text formatting.
- `agents-plugin-tool/internal/wsconfig/resolver.go#L72-L189` — `Resolver.Get/Set/Unset`: shared session > project > global > builtin resolver, including file-scope unset behavior.
- `agents-plugin-tool/internal/mcp/session_config_adapter.go#L1-L30` — `sessionConfigAdapter`: existing adapter from MCP session store to `wsconfig` resolver reads/writes and session-only key enumeration.
- `agents-plugin-tool/internal/wsconfig/config.go#L143-L201` — `SetAgentsTierForHarness`: current `config.agents_tier` write authority and harness target behavior.
- `agents-plugin-tool/internal/wsconfig/config.go#L258-L286` and `agents-plugin-tool/internal/wsconfig/config.go#L431-L443` — tier/model/effort normalization helpers backing accepted alias and effort values.
- `agents-plugin-tool/internal/mcp/server.go#L1032-L1080` — `ws.lead.prefer_mercenary` dispatch: maps public `value` `on|off|hide` and legacy `enabled` into stored resolver values.

## Existing Patterns
- MCP tool addition pattern: see `agents-plugin-tool/internal/mcp/server.go#L309-L348`, `agents-plugin-tool/internal/mcp/server.go#L2231-L2284`, and `ai-docs/mental-model/mcp-runtime.md#L67-L72` — schema, dispatch, role/profile filtering, runtime metadata, and `runtime.json` are reviewed together.
- Config listing pattern: see `agents-plugin-tool/internal/mcp/server.go#L588-L612` and `agents-plugin-tool/internal/mcp/server.go#L2400-L2409` — read-only `config.*` tools can accept optional `session_key`, use ambient config resolver state, and return text or JSON.
- Product-mode gate pattern: see `agents-plugin-tool/internal/mcp/server.go#L322-L327`, `agents-plugin-tool/internal/mcp/server.go#L2938-L3028`, and `agents-plugin-tool/internal/mcp/server.go#L3134-L3152` — no-agent filtering applies at explicit call, tools/list, and runtime capabilities.
- Runtime contract pattern: see `agents-plugin-tool/cmd/ws-mcp/main.go#L184-L195`, `agents-plugin-tool/cmd/ws-mcp/main_test.go#L82-L90`, and `agents-plugin-tool/cmd/ws-mcp/main_test.go#L140-L173` — `runtime.capabilities` is compared against full ws and exact wsflow manifests.
- Rsrc change pattern: see `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L17-L43`, `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L80`, and `ai-docs/ref/wsflow-mirroring.md#L104-L121` — canonical rsrc edits require manifest regeneration and byte-identical wsflow mirror regeneration.
- Lead playbook mirroring pattern: see `agents-plugin/skills/lead-tune/SKILL.md#L1-L9`, `agents-plugin-wsflow/skills/lead-tune/SKILL.md#L1-L10`, and `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L90-L117` — distributed skill files are thin shims over shared rsrc playbooks.

## Relevant Interfaces
- `agents-plugin-tool/internal/mcp/server.go#L2282-L2291` — `ws.lead.prefer_mercenary` schema: canonical `value` enum is present alongside compatibility-only `enabled`.
- `agents-plugin-tool/internal/mcp/server.go#L2356-L2368` — `config.agents_tier` schema: `tier` enum, optional `harness`, and value fields `backend`, `model`, `effort`.
- `agents-plugin-tool/internal/mcp/server.go#L2371-L2397` — `config.prompt.set` / `config.prompt.unset` schemas: prompt writer/reset fields, harness enum, and scope enum.
- `agents-plugin-tool/internal/mcp/server.go#L2886-L2926` — root-aware schema injection: only tools listed here get required `session_key`; `config.*` tools currently carry their own optional/required session-key shape.
- `agents-plugin/runtime.json#L8-L62` and `agents-plugin-wsflow/runtime.json#L11-L48` — plugin runtime tool manifests that launcher compatibility checks compare against live capabilities.
- `agents-plugin/rsrc/lead-tune/lead-tune.md#L24-L52` — current lead-tune invoke and handler prose duplicates prompt, mercenary, and tier details that the catalog is meant to replace.
- `agents-plugin/rsrc/lead-tune/lead-tune.md#L61-L88` — current routing judgments and Tuning Proposal template that remain lead-tune-owned after catalog discovery.

## Constraints
- `ai-docs/spec/mcp-tools.md#L208-L239` — `config.tuning` is read-only, projection-only, derives schemas from writers where possible, uses marker-driven prompt point ids, supports text and JSON, and omits full-ws-only knobs in no-agent mode.
- `ai-docs/tickets/ready/260625-feat-lead-tune-schema-backed-knob-catalog.md#L69-L112` — Phase 1 requires full ws prompt/delegation/tier entries, wsflow prompt-only retention, lead-tune catalog invocation, and tests for text, JSON, wsflow filtering, schema-derived prefer values, and rsrc prose drift.
- `ai-docs/mental-model/mcp-runtime.md#L36-L47` — MCP tool results are text content, config scope resolution is layered, and no-agent product gates must be applied consistently across call, tools/list, and capabilities.
- `ai-docs/mental-model/plugin-runtime.md#L52-L57` — non-mercenary/non-exec MCP tools belong in both full ws and wsflow runtime contracts; packaging changes that affect wsflow require wsflow package tests.
- `ai-docs/mental-model/prompt-bundle.md#L25-L35` and `ai-docs/mental-model/prompt-bundle.md#L52-L55` — rsrc is the single prompt source of truth, rsrc edits need manifest regeneration, and runtime.json is not refreshed for text-only rsrc edits.
- `ai-docs/ref/wsflow-mirroring.md#L14-L27` — changing a shared lead playbook requires checking the wsflow shim, wsflow product-mode rendering, and package tests.

## Risk Signals
- `agents-plugin-tool/internal/mcp/server.go#L2282-L2291` — Possible schema projection risk: a naive projection of `ws.lead.prefer_mercenary` would expose legacy `enabled`, but the ticket requires only canonical `value` options.
- `agents-plugin-tool/internal/mcp/server.go#L3119-L3132` and `agents-plugin-tool/internal/wsconfig/resolver.go#L255-L265` — Possible current-value risk: `hide` is a string mode for tool visibility while `GetBool` collapses every non-`true` value to false for render guidance, so catalog reporting should not conflate `hide` with `off`.
- `agents-plugin-tool/internal/mcp/server.go#L546-L586` and `agents-plugin-tool/internal/wsconfig/resolver.go#L154-L189` — Possible reset semantics risk: `config.prompt.unset` exists but rejects session-scope unset through the resolver; catalog reset fields should reflect the real writer/reset schema rather than implying all scopes can be cleared.
- `agents-plugin-tool/internal/mcp/server.go#L471-L486` and `ai-docs/mental-model/mcp-runtime.md#L42-L42` — Possible current-scope risk: `config.agents_tier` is not routed through the layered resolver, so the catalog may have less scope detail for `agents.tier` than prompt/prefer knobs.
- `agents-plugin-tool/internal/mcp/server.go#L2886-L2926` — Possible contract risk: adding `config.tuning` to root-aware schema injection would make `session_key` required, while the brief says it is optional.
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L90-L115` and `agents-plugin-wsflow/runtime.json#L11-L48` — Possible packaging risk: exposing `config.tuning` in no-agent mode without adding it to wsflow `runtime.json` will fail exact runtime-contract tests and launcher compatibility.
- `ai-docs/tickets/idea/260624-feat-prefer-mercenary-hide-option.md#L42-L49` and `agents-plugin-tool/internal/mcp/server_test.go#L1967-L1990` — Possible doc drift signal: the old idea ticket says default remains `off`, while current tests/code treat unset as hidden but keep `ws.lead.prefer_mercenary` visible.

## Opinion
- `ai-docs/tickets/ready/260625-feat-lead-tune-schema-backed-knob-catalog.md#L45-L65` — Survey is sufficient; the brief already settles projection-vs-setter, registry-vs-manual schema, and raw-tools/list rejection, so no research escalation is needed.
- `agents-plugin-tool/internal/mcp/server.go#L2231-L2409` — Code quality signal: the relevant schema and dispatch authority is centralized enough for a small catalog projection to avoid duplicating enum/property values.
