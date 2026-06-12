---
title: Root-aware MCP tool schemas omit required session_key
related:
  260609-refactor-ws-spawn-runtime-deletion-session-auth: introduced mandatory session-key root resolution
  260611-refactor-ws-tier-taxonomy-delegate-tier-routing: current dogfood branch exposing the issue after plugin cache refresh
related-mental-model:
  - mcp-runtime
  - plugin-runtime
---

# Root-aware MCP tool schemas omit required session_key

## Background

Codex plugin reinstall fixed the ws MCP launcher/cache mismatch, and the server
now starts from the refreshed plugin cache. `runtime.info` and `ws.lead.login`
both work through Codex MCP. However, root-aware tools remain effectively
uncallable from normal model-visible tool use.

Observed through the live Codex MCP bridge:

- `runtime.info({"format":"json"})` succeeds and returns
  `{"source_commit":"dev","version":"0.30.0-dev"}`.
- `ws.lead.login({"root":"/home/swkang/devenv","format":"json"})` succeeds and
  returns a `session_key`.
- `project_tree({})` fails with
  `mandatory_session_key: root-aware ws tools require session_key; call ws.lead.login(root) first and pass the returned session_key`.

The failure is not that the server cannot use the key. A raw JSON-RPC probe that
logged in and then called:

```json
{"name":"project_tree","arguments":{"session_key":"<returned-key>"}}
```

returned the expected `ai-docs/` project tree. The failure is that
`tools/list` advertises schemas for many root-aware tools without a
`session_key` property, so Codex cannot naturally retry with the key it just
received.

Representative source evidence:

- `internal/mcp/server.go` `resolveToolRoot` requires `session_key` and returns
  `mandatory_session_key` when absent.
- `tools()` still advertises `project_tree` with an empty `properties` map and
  `git.status` with only `format`.
- `server_test.go` injects `session_key` into test calls after login, but the
  tools/list schema assertion only rejects `root`; it does not require
  `session_key` on root-aware tools.

## Phases

### Phase 1: Advertise session_key on root-aware MCP schemas

Update the public MCP schemas for every root-aware tool so model-visible
callers can pass the session key returned by `ws.lead.login`.

Scope includes root-aware docs, Git, ticket, spec, mental-model, references,
path, exec, playbook render, mercenary lifecycle, and any other handler routed
through `resolveToolRoot`. `ws.lead.login` remains the only schema that accepts
`root`; `runtime.info`, `runtime.debug_events`, `config.*`, `infra.read`, and
`convention.read` should not gain unnecessary root/session requirements unless
their handlers actually require them.

Verification boundary:

- `tools/list` schemas for root-aware tools include `session_key` and still do
  not include `root`.
- A Codex MCP Level 3 smoke can call `runtime.info`, then `ws.lead.login`, then
  `project_tree` successfully without shell commands.
- Existing mandatory-key tests still verify that keyless root-aware calls fail
  with `mandatory_session_key`.
- `go test ./...` under `agents-plugin-tool/` remains green.
