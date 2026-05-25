---
title: ws MCP launcher runtime repair race
---

# ws MCP launcher runtime repair race

## Background

During MCP startup dogfooding, concurrent first-start or repair attempts against
the same plugin cache runtime directory can fail intermittently. The Python
launcher writes fixed temporary paths such as `ws-mcp.download`,
`SHA256SUMS.download`, and `.compatibility.json.tmp` under the shared runtime
directory while installing or repairing the `ws-mcp` binary.

When multiple Codex sessions or MCP server instances start at the same time, one
launcher process can replace or delete another process's temporary file before
checksum verification or install completion. A local parallel first-start
reproduction produced one successful startup and multiple `FileNotFoundError`
failures from `sha256_file(tmp)`.

## Reproduction

Run several launcher processes with the same empty `WS_MCP_RUNTIME_DIR`:

```bash
WS_MCP_RUNTIME_DIR=<empty-dir> python3 agents-plugin/bin/ws-mcp-launcher.py runtime info
```

Expected behavior: all startup attempts should either install a compatible
runtime or reuse a compatible runtime installed by another process.

Observed behavior: only one startup may win the install race; other startup
attempts can fail before the MCP server reaches stdio initialization.

## Notes

The implemented direction is process-unique temporary paths, a cache-local
runtime binary name derived from `plugin_version` plus the `runtime.json` hash,
and best-effort replacement. If a final replace fails because another process
already installed or is using the same target, the launcher rechecks that target
and proceeds when it is compatible.
