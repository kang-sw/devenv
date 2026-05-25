# Survey: 23-agents-root-schema-invisibility

## Reusable Components
- `agents-plugin-tool/internal/mcp/server.go#L1456-L1494` — `resolveToolRoot`: central root resolver already accepts explicit `root`, session root from `ws.setup`, Codex workspace metadata, server root, and `WS_MCP_PROJECT_ROOT`; this is the compatibility path public `agents.*` dispatch currently uses.
- `agents-plugin-tool/internal/mcp/server.go#L2252-L2290` — `publicToolDefinition`: existing defensive public-schema clone/namespace filter that strips `root` from `agents.*` after `tools()`; useful as a pattern or fallback while raw schemas are cleaned.
- `agents-plugin-tool/internal/mcp/server.go#L2449-L2458` — `agentDebugSchema`: shared schema helper for all `agents.debug.*` tools; it currently includes `root`, so one helper change affects every debug agent schema.
- `agents-plugin-tool/internal/mcp/server_test.go#L142-L155` — tools/list schema assertion loop: already iterates public `agents.*` tools from JSON-RPC `tools/list` and fails if `root` is present.
- `agents-plugin-tool/internal/mcp/server_test.go#L548-L600` — `TestServeStdioSetupRoot`: proves `ws.setup(root)` drives root-omitted calls and explicit `root` overrides the session root for non-agent tools.
- `agents-plugin-tool/internal/mcp/server_test.go#L488-L544` — no-agent/wsflow tools/list test: proves `WS_MCP_NO_AGENT=1` hides agent tools, keeps setup alias behavior, and returns a clear disabled error for explicit hidden agent calls.

## Existing Patterns
- Public schema generation: see `agents-plugin-tool/internal/mcp/server.go#L1634-L1666` and `agents-plugin-tool/internal/mcp/server.go#L2064-L2219` — `tools()` is the raw advertised MCP schema source; `ws.setup` keeps `root`, while current raw agent entries also include `root`.
- Dispatch/schema separation: see `agents-plugin-tool/internal/mcp/server.go#L735-L903` — every `agents.*` dispatch resolves root before calling `wsagent.Manager`, so schema cleanup can be separate from hidden argument parsing.
- Product-mode hiding: see `agents-plugin-tool/internal/mcp/server.go#L268-L292` and `agents-plugin-tool/internal/mcp/server.go#L2397-L2407` — environment-selected wsflow/no-agent mode hides `agents.*`, subquery, config tier, and agent-backed API tools without renaming generic tool identifiers.
- Runtime capabilities derive names, not schemas: see `agents-plugin-tool/cmd/ws-mcp/main.go#L190-L215` and `agents-plugin-tool/internal/mcp/server.go#L2223-L2237` — `runtime.capabilities` uses `LeadToolNames()`, so schema-only changes may not require runtime.json unless tool names change.
- Runtime contract verification: see `agents-plugin-tool/cmd/ws-mcp/main_test.go#L50-L89` and `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L85-L109` — full ws and wsflow tests compare runtime contract tool/command name sets against capability output.

## Relevant Interfaces
- `agents-plugin-tool/internal/wsagent/agent.go#L78-L161` — `RegisterOptions`, `CallOptions`, `WaitOptions`, `ResultOptions`, `TailOptions`, and `DiagnosticStreamOptions`: manager interfaces still require a resolved `Root`; MCP wrappers should continue to provide it internally.
- `agents-plugin-tool/internal/wsagent/agent.go#L398-L455` — `Manager.Register`: defaults empty root to `.` and resolves prompts/model alias before agent layout creation.
- `agents-plugin-tool/internal/wsagent/agent.go#L711-L740` — `Manager.Call`: uses `Root`, `Name`, and `Prompt` to locate a registered agent and snapshot the prompt before async start.
- `agents-plugin-tool/internal/wsagent/agent.go#L1057-L1078` and `agents-plugin-tool/internal/wsagent/agent.go#L1143-L1175` — `Manager.Result` / `Manager.Wait`: use root-scoped layouts and support timeout/context cancellation paths.
- `agents-plugin-tool/internal/wsagent/agent.go#L1267-L1288`, `agents-plugin-tool/internal/wsagent/agent.go#L1383-L1406`, `agents-plugin-tool/internal/wsagent/agent.go#L1572-L1588`, and `agents-plugin-tool/internal/wsagent/agent.go#L2027-L2038` — `Status`, `Tail`, `Cancel`, and `Erase`: all consume the resolved root from MCP dispatch.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L928-L1208` — CLI `agents` subcommands: separate adapter surface still exposes `--root`; brief scope is public/generated MCP schema, not CLI flag cleanup.
- `agents-plugin/runtime.json` — full ws runtime contract lists `agents.*` tool names and commands but no per-tool schemas; likely unchanged for schema-only cleanup unless names/mode behavior changes.
- `agents-plugin-wsflow/runtime.json` — wsflow contract omits agent-backed tools under exact capability matching; likely unchanged unless no-agent capability names change.

## Constraints
- `ai-docs/spec/mcp-tools.md#L258-L275` records planned behavior: public/generated `agents.*` schemas omit `root`, but hidden explicit-root compatibility may remain.
- `ai-docs/mental-model/mcp-runtime.md#L35-L42` defines root resolution and no-agent product-mode behavior; `ws.setup(root)` is the public session-root surface and hidden session dispatch is compatibility only.
- `ai-docs/mental-model/plugin-runtime.md#L42-L44` says tool additions/removals require `runtime.json`; this task is schema-only unless it changes advertised tool names.
- `ai-docs/ref/ws-mcp.md#L1110-L1134` already states public `agents.*` schemas intentionally omit `root` and normal calls should establish worktree with setup first.
- `ai-docs/ref/wsflow-mirroring.md#L61-L66` requires wsflow package tests after runtime contract or agent-surface-adjacent changes.

## Risk Signals
- `agents-plugin-tool/internal/mcp/server.go#L2064-L2219` — Possible contract risk: raw `tools()` schemas for `agents.register`, `agents.call`, `agents.wait`, `agents.result`, `agents.status`, `agents.interrupt`, `agents.tail`, `agents.cancel`, `agents.print`, and `agents.erase` still include `root`; generated hosts that bypass `publicToolDefinition` can see the stale parameter.
- `agents-plugin-tool/internal/mcp/server.go#L2449-L2458` — Possible contract risk: `agentDebugSchema` injects `root` into all public `agents.debug.*` schemas; the brief says publicly advertised compatibility aliases or debug agent tools should also avoid presenting `root` unless intentionally non-public.
- `agents-plugin-tool/internal/mcp/server_test.go#L142-L155` — Possible test risk: existing schema assertion covers filtered `tools/list` only; it does not prove the raw `tools()` metadata is root-free, which is the known leak path in the brief.
- `agents-plugin-tool/internal/mcp/server_test.go#L417-L440` and `agents-plugin-tool/internal/mcp/server_test.go#L684-L724` — Possible test/contract risk: several current alias/harness tests exercise explicit `root` on `agents.register` and `agents.status`; useful for compatibility, but new root-omitted-after-`ws.setup` coverage may need separate assertions so tests do not keep modeling explicit root as normal.
- `agents-plugin-tool/internal/mcp/server.go#L2197-L2206` — Possible compatibility-alias risk: `agents.print` is deprecated but publicly advertised and still includes `root`; brief examples do not list it in the primary eight tools, but the contract mentions publicly advertised compatibility aliases.

## Opinion
- The survey found no need for strategy escalation: the code has a clear schema source (`tools()`), a separate dispatch resolver (`resolveToolRoot`), and tests that can be extended in-package to inspect raw schemas.
- Main uncertainty is scope treatment for `agents.interrupt`, `agents.print`, and `agents.debug.*`: they are public `agents.*` surfaces even though not all are named in the primary eight-tool list, so lead/planner may want them inspected before implementation locks the set.
