# ws.setup root session API brief

## Target

Implement `260513-feat-ws-setup-root-session-api`, Phase 1.

Replace the public root-session setup surface with canonical `ws.setup(root?)`
behavior while preserving the existing resolver safety properties.

## Scope Boundary

Implement only root setup in this phase. Do not add model/default-model setup
fields yet, except designing `ws.setup` so future optional fields can be added
without replacing the tool.

## Required Behavior

- Add a public MCP tool named `ws.setup`.
- `ws.setup(root: <path>)` validates the path as a Git worktree, stores the
  canonical worktree root in the same volatile per-server state currently used
  by root-aware tools, and returns setup state.
- Omitted-root calls after `ws.setup(root: <path>)` use that root.
- Root resolution diagnostics should point callers to `ws.setup(root = current
  directory)` rather than `session.set_default_root`.
- `session.set_default_root` and `session.get_default_root` must stop being
  public/canonical tools. Prefer removing them from `tools()` and
  `runtime.json`. Hidden compatibility dispatch is acceptable only if tests and
  docs make clear it is not advertised.
- `agents.*` public schemas should stop exposing `root` as the normal caller
  field. Keep explicit root handling in `resolveToolRoot` if needed for
  compatibility or test setup.
- Preserve resolver safety:
  - do not guess among multiple host workspaces;
  - invalid authoritative startup roots fail closed;
  - single host workspace metadata remains a fallback;
  - explicit root override behavior may remain internally accepted.

## Expected Files

- `agents-plugin-tool/internal/mcp/server.go`
- `agents-plugin-tool/internal/mcp/server_test.go`
- `agents-plugin/runtime.json`
- `ai-docs/spec/mcp-tools.md`
- `ai-docs/spec/plugin-runtime.md` if runtime contract text needs tool-surface wording updates
- `ai-docs/mental-model/mcp-runtime.md`
- `ai-docs/mental-model/plugin-runtime.md`
- `ai-docs/ref/ws-mcp.md`
- `ai-docs/tickets/ready/260513-feat-ws-setup-root-session-api.md` only if implementation notes or Result are needed later; do not close the ticket

## Tests

Run from `agents-plugin-tool/`:

```text
go test ./...
```

At minimum, add or update tests for:

- `ws.setup(root)` stores session root and omitted-root calls use it.
- Session root does not persist across server instances.
- Explicit root arguments still override session root if compatibility support
  remains accepted.
- Multiple host workspaces produce actionable `ws.setup` guidance.
- Invalid server root does not fall back to `WS_MCP_PROJECT_ROOT` and reports
  `ws.setup` guidance.
- `tools/list` and runtime capability tests expose `ws.setup` and no longer
  require public `session.set_default_root` / `session.get_default_root`.
- `agents.*` schemas no longer advertise `root` as normal public input.

## References

- `ai-docs/tickets/ready/260513-feat-ws-setup-root-session-api.md`
- `ai-docs/spec/mcp-tools.md`:
  - `260505-mcp-session-default-root`
  - `260505-named-agent-mcp-tools`
  - `260512-metadata-trace-readable-output-defaults`
- `ai-docs/spec/plugin-runtime.md`:
  - `260505-runtime-contract-metadata`
  - `260506-runtime-capabilities-single-probe`
- `ai-docs/mental-model/mcp-runtime.md`
- `ai-docs/mental-model/named-agent-runtime.md`
- `ai-docs/mental-model/plugin-runtime.md`

## Implementation Notes

- The implementation is not alone in the codebase. Do not revert unrelated
  edits; adjust to existing branch state.
- Keep MCP tool additions synchronized across dispatch, schema, profile
  filtering, runtime capability metadata, and tests.
- Keep CLI `--root` as adapter behavior unless changing it is required by tests.
- Prefer small helpers if removing `root` from only `agents.*` schemas would
  duplicate schema maps.
