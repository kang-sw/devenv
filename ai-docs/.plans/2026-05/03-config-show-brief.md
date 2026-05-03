# Brief: config.show

## Intent

Add a read-only ws configuration inspection surface so a lead can see the
current user-local agent tier mapping without mutating `config.json` or opening
cache files directly. This is also a small regression dogfood target for the
named-agent `write-code` workflow after nonblocking MCP orchestration and tier
configuration landed.

## Approach

- Add a read API in `agents-plugin-tool/internal/wsconfig` that returns the
  current config plus the resolved config path.
- Expose it as MCP tool `config.show`.
- Expose the CLI fallback as `ws-mcp config show`.
- Add runtime metadata entries for both the MCP tool and CLI command.
- Update the MCP/runtime reference docs and the stability epic with the dogfood
  result.

## Constraints

- `config.show` is read-only and must not create or modify `config.json`.
- If no config file exists, it should return the default empty config shape and
  the path where config would be read from.
- Preserve the existing `config.agents_tier` behavior and precedence rules.
- `delegate` and `leaf` MCP profiles should continue to reject all `config.*`
  tools, including the new read tool.
- Do not update specs or mental models on this branch.

## Out of scope

- Adding backend adapters beyond the existing Codex runner.
- Adding glob-based model route configuration.
- Changing prompt frontmatter tier aliases.
- Changing `agents.register` behavior beyond what is required to preserve the
  current tier config contract.

## Details

Expected output may be JSON text containing at least:

- `path`: absolute path to the config file location.
- `config`: current config object as loaded by ws.

The exact Go struct shape is implementation detail, but MCP and CLI should use
the same underlying read function so behavior stays aligned. Tests should cover
the no-file default case and the case after `config.agents_tier` has written a
mapping.

Verification targets:

- `cd agents-plugin-tool && go test ./...`
- `python3 -m json.tool agents-plugin/runtime.json`
- `claude plugin validate agents-plugin`
- `git diff --check`
- A CLI smoke using temporary `WS_CACHE_HOME`:
  configure `light -> gemini-3-1-pro`, then run `ws-mcp config show` and
  confirm the path plus mapping are present.

## References

- `[Must]` `agents-plugin-tool/internal/wsconfig/config.go` - config schema,
  cache path, load/write behavior, tier resolution, and backend inference.
- `[Must]` `agents-plugin-tool/cmd/ws-mcp/main.go` - CLI routing for
  `ws-mcp config ...`; currently only `config agents-tier` exists.
- `[Must]` `agents-plugin-tool/internal/mcp/server.go` - MCP dispatch,
  tools/list schemas, `config.agents_tier`, and profile filtering for
  `config.*`.
- `[Must]` `agents-plugin-tool/internal/wsconfig/config_test.go` - focused
  config persistence and inference coverage.
- `[Must]` `agents-plugin-tool/internal/mcp/server_test.go` - MCP tool,
  tools/list, and profile filtering coverage.
- `[Must]` `agents-plugin/runtime.json` - runtime compatibility metadata for
  tool and CLI command drift checks.
- `[Must]` `ai-docs/ref/ws-mcp.md` - canonical MCP tool contract.
- `[Must]` `ai-docs/ref/ws-agent-runtime.md` - tier model configuration and
  CLI/MCP surface inventory.
- `[Must]` `ai-docs/tickets/todo/260503-epic-ws-agent-workflow-stability.md` -
  active stability epic and dogfood record.
