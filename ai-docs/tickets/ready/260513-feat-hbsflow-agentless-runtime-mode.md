---
title: hbsflow agentless runtime mode
parent: 260513-epic-hbsflow-agentless-plugin
related:
  260513-feat-hbsflow-agentless-plugin-scaffold: provides the shared runtime mode required before packaging hbsflow
  260429-research-host-neutral-ws-plugin: host-neutral plugin architecture anchor
spec:
  - 260513-hbsflow-agentless-runtime-mode
  - 260513-hbsflow-runtime-contract-mode
related-mental-model:
  - plugin-runtime
  - mcp-runtime
  - named-agent-runtime
---

# hbsflow agentless runtime mode

## Background

The hbsflow plugin distribution should reuse the shared `ws-mcp` runtime while
presenting an agentless surface. The riskiest part is not copying the plugin
package; it is ensuring the shared runtime can expose a reduced hbsflow contract
without changing the default full `ws` behavior.

This ticket isolates the runtime-only work before `agents-plugin-hbsflow/`
packaging. Its primary success condition is proving that the existing full ws
tool surface, launcher capability contract, and CLI command surface remain
unchanged when the new environment variables are unset.

## Decisions

- Add `WS_MCP_NO_AGENT=1` as a product-mode gate for agent-backed surfaces. It
  is separate from `WS_MCP_TOOL_PROFILE`, which remains only a containment
  filter.
- Hide all agent-backed MCP tools in no-agent mode:
  `agents.*`, `subquery`, `config.agents_tier`, `api.ask`, `api.ask_async`,
  `api.status`, `api.result`, and `api.cancel`.
- Keep `api.list` visible in no-agent mode as read-only cache/domain discovery
  because it does not start a named-agent worker.
- Hide matching agent-backed CLI commands in no-agent mode:
  `agents`, `subquery`, and `config agents-tier`.
- Add `WS_MCP_NAMESPACE=hbsflow` for user-facing namespace text. Do not rename
  generic MCP tool names such as `project_tree`, `git.status`, `tickets.list`,
  or `specs.find`; the host MCP server key supplies the `hbsflow/<tool>`
  namespace.
- Add `WS_MCP_SETUP_TOOL=setup` so hbsflow can advertise `setup` instead of
  `ws.setup`. The default full ws behavior still advertises `ws.setup`.
- In hbsflow mode, `setup` should be the advertised tool name. `ws.setup` may
  remain as hidden compatibility dispatch if that reduces shared-runtime churn.

## Constraints

- Environment variables unset must preserve the existing full ws behavior.
- No-agent mode must not silently dispatch hidden agent-backed tools to
  `wsagent`; explicit calls should return clear disabled errors.
- Runtime compatibility data must match the mode being validated. Full ws
  `runtime.capabilities` stays full; hbsflow/no-agent capabilities omit hidden
  tools and commands.
- Namespace substitution is limited to user-facing guidance, errors, follow-up
  text, and runtime metadata where relevant. It must not rename the `ws-mcp`
  binary, Go packages, persisted ws state paths, or spec anchors.
- In hbsflow/no-agent mode, ordinary user-facing runtime text should not require
  ws awareness. Internal diagnostics may still name `ws-mcp`, Go packages, or
  hidden compatibility paths when that is the precise implementation surface.
- Do not create `agents-plugin-hbsflow/` in this ticket except as a test fixture
  if one is needed for runtime validation.

## Phases

### Phase 1: Add no-agent and namespace runtime mode

Implement the environment-driven runtime mode and tests while preserving the
default full ws surface.

Suggested approach:

- Add helper functions for no-agent mode, runtime namespace, and setup tool
  name resolution.
- Filter MCP `tools/list` and `tools/call` for agent-backed surfaces when
  `WS_MCP_NO_AGENT=1`.
- Filter `runtime.capabilities` tool and command lists according to no-agent
  mode.
- Gate CLI `agents`, `subquery`, and `config agents-tier` with clear no-agent
  disabled errors.
- Advertise `setup` instead of `ws.setup` when `WS_MCP_SETUP_TOOL=setup`, while
  preserving default `ws.setup` behavior.
- Apply `WS_MCP_NAMESPACE` only where user-visible runtime guidance needs the
  server namespace.

Acceptance criteria:

- With no new environment variables, existing full ws `tools/list`,
  `runtime.capabilities`, CLI command listing, and launcher capability tests
  remain unchanged.
- With `WS_MCP_NO_AGENT=1`, MCP `tools/list` omits `agents.*`, `subquery`,
  `config.agents_tier`, `api.ask`, `api.ask_async`, `api.status`,
  `api.result`, and `api.cancel`.
- With `WS_MCP_NO_AGENT=1`, `api.list` remains visible and callable.
- With `WS_MCP_NO_AGENT=1`, explicit calls to hidden agent-backed tools return a
  clear disabled error and do not dispatch to `wsagent`.
- With `WS_MCP_NO_AGENT=1`, `runtime.capabilities` omits hidden agent-backed MCP
  tools and CLI commands.
- With `WS_MCP_NO_AGENT=1`, CLI `agents`, `subquery`, and
  `config agents-tier` return clear disabled errors.
- With `WS_MCP_SETUP_TOOL=setup`, `tools/list` advertises `setup`; default full
  ws still advertises `ws.setup`.
- With `WS_MCP_NAMESPACE=hbsflow`, user-facing guidance and error text that name
  the MCP namespace use `hbsflow` notation.
- With hbsflow/no-agent environment active, ordinary user-facing runtime text
  does not present hbsflow as a ws variant or instruct users to call ws-named
  tools.
