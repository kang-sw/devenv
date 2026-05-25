# Brief: 260523-bug-agents-root-schema-visible

## Intent

Hide `root` from the public and generated `agents.*` MCP schema surface so named-agent workflows use `ws.setup(root)` as the normal repository binding mechanism while preserving hidden explicit-root dispatch compatibility.

## Scope Boundary

Implement Phase 1 of `260523-bug-agents-root-schema-visible`. Change only the named-agent MCP schema and directly related runtime reference/test surfaces needed to make `agents.*` root invisible end-to-end. Keep non-agent root-aware tools unchanged.

## Caller-Visible Contract

Public and generated schemas for `agents.register`, `agents.call`, `agents.wait`, `agents.result`, `agents.status`, `agents.tail`, `agents.cancel`, and `agents.erase` must not advertise `root`. Publicly advertised compatibility aliases or debug agent tools must also avoid presenting `root` unless the tool is intentionally non-public. Callers establish the repository once through `ws.setup(root)` and then omit `root` on normal named-agent calls.

Explicit `root` in an `agents.*` tool call may continue to work as hidden compatibility input. Do not remove dispatch parsing unless tests and docs prove the compatibility path is intentionally dropped.

## Contract Instructions

- Edit `agents-plugin-tool/internal/mcp/server.go` so the raw advertised schema for public `agents.*` tools no longer includes `root`.
- Preserve root resolution in `callTool` for `agents.*` dispatch so hidden explicit-root arguments still work.
- Keep `root` on non-agent root-aware tools.
- Keep or simplify any defensive schema filtering only if it remains useful after the raw schema cleanup.
- Update `ai-docs/ref/ws-mcp.md` if it still documents or examples `root` as public `agents.*` input.
- Check `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`; update only if the runtime contract actually changes.
- Do not make workflow skills pass per-call root arguments. `ws.setup` remains the session setup surface.

## Integration Test Instructions

Extend `agents-plugin-tool/internal/mcp/server_test.go` to prove the raw public `tools/list` schema for every public `agents.*` tool omits `root`, not only one filtered path. Preserve tests proving hidden explicit-root compatibility still works.

Run:

```sh
cd agents-plugin-tool && go test ./internal/mcp ./cmd/ws-mcp ./internal/wsagent
cd agents-plugin-tool && go test ./...
python3 -m unittest discover agents-plugin-wsflow/tests
```

If runtime contract metadata is touched, also run the relevant runtime capability checks for full and no-agent modes.

## Implementation Strategy Decisions

- Treat this as schema visibility cleanup, not a root-resolution redesign.
- Public schemas must align with the `ws.setup` session-root contract.
- Hidden explicit-root dispatch compatibility is preserved for stale callers, tests, and broken host root discovery.
- wsflow/no-agent mode should stay agentless and must not regain an agent schema surface.

## Rejected Alternatives

- Removing `root` from every root-aware MCP tool is too broad for this slice.
- Removing explicit-root dispatch compatibility is riskier than necessary and would break older callers without improving the public schema.
- Relying only on late `toolForList` filtering leaves raw schema metadata stale and can leak through generated host tool definitions.

## Approach

- Inspect the `tools()` definitions and agent-specific schema helpers in `agents-plugin-tool/internal/mcp/server.go`.
- Remove `root` from public `agents.*` input schemas at definition time.
- Keep dispatch calls to `resolveToolRoot` unchanged where hidden compatibility is intended.
- Adjust or add tests around `tools/list` and explicit-root calls.
- Update runtime reference text only where it drifts from the intended public surface.

## Constraints

- Do not change named-agent cache layout, agent lifecycle, or backend runner behavior.
- Do not add a new MCP tool or CLI command.
- Do not alter `WS_MCP_NO_AGENT`, namespace, setup-tool alias, or profile semantics.
- Keep docs in English.

## Out of scope

- Dashboard behavior.
- Root visibility for tickets/specs/git/path/API tools.
- Plugin version bump or release packaging.
- Broader skill rewrite for root handling.

## Details

The planned spec anchor is `260523-agents-root-schema-invisibility` in `ai-docs/spec/mcp-tools.md`.

Success means a host constructing tool definitions from the public MCP schema sees no `root` parameter for normal named-agent tools, while explicit JSON-RPC calls that include `root` can still route through the compatibility resolver if preserved.

## Verification Contract

- Full ws `tools/list` schema has no `root` property on public `agents.*` tools.
- Root-omitted `agents.*` calls work after `ws.setup(root)`.
- Explicit-root `agents.*` compatibility tests still pass.
- `WS_MCP_NO_AGENT=1` advertises no public `agents.*` schema surface.
- The listed Go and wsflow tests pass with full output reviewed.

## References

- [Must] `ai-docs/tickets/ready/260523-bug-agents-root-schema-visible.md` - selected phase and acceptance criteria.
- [Must] `ai-docs/spec/mcp-tools.md` - `260523-agents-root-schema-invisibility`, session-root, named-agent, and wsflow no-agent contracts.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP schema, dispatch, profile, and wsflow coupling.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - named-agent wrapper and cache-root invariants.
- [Must] `ai-docs/mental-model/plugin-runtime.md` - runtime contract and wsflow packaging coupling.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow runtime-contract review expectations.
- [Maybe] `ai-docs/ref/ws-mcp.md` - runtime reference schema examples.
