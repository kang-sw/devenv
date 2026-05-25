---
title: plugin-managed default root discovery fails on Windows
related-mental-model:
  - plugin-runtime
  - mcp-runtime
---

# plugin-managed default root discovery fails on Windows

## Background

A Windows Codex plugin-managed MCP smoke test in
`C:\Users\ki608\repos\test-repo` showed that `agents.register` fails without an
explicit `root` argument:

```text
git rev-parse --show-toplevel: fatal: not a git repository
```

Passing `root: "C:\\Users\\ki608\\repos\\test-repo"` succeeds. This indicates
the MCP server is defaulting to its own process context or plugin cache context
when the Python launcher cannot discover the caller repository root.

Current launcher discovery uses parent `PWD`/`OLDPWD`, process environment, and
current directory candidates. Those are unreliable on native Windows Codex
plugin-managed startup because PowerShell/Codex may not expose a POSIX-style
`PWD` to the MCP launcher, and the MCP process cwd is intentionally the plugin
cache so relative `.mcp.json` paths resolve.

## Questions

- Does Codex expose MCP `roots` or another caller-workspace signal that ws-mcp can
  request during initialization?
- Should ws-mcp return a clearer root-required diagnostic when no project root is
  detected and the default root is the plugin cache or user home?
- Should plugin-managed skills always pass `root` explicitly until a host-level
  caller-root contract exists?

## Suggested Direction

Do not guess arbitrary Windows home directories. Prefer a host-provided root
contract if available. If none exists, document explicit `root` as mandatory for
plugin-managed MCP calls whose launcher cannot set `WS_MCP_PROJECT_ROOT`.

## Dropped

This stale backlog item is superseded by completed session-root work and a
narrower follow-up:

- `260505-feat-mcp-session-default-root` implemented the intended recovery path:
  callers can set the current MCP server process root once with `ws.setup(root)`
  and omit `root` from later root-aware tool calls.
- `260523-bug-agents-root-schema-visible` now tracks the remaining problem:
  `agents.*` root arguments still leak into public/generated host tool schemas
  and encourage callers to pass root repeatedly instead of using the session
  setup surface.
