---
title: Trim actor-gate setup guidance in root-omitted agent tools
related:
  260524-mcp-actor-setup-bootstrap: actor setup and recovery contract
  260525-feat-ws-dashboard-sqlite-agent-activity-source: dogfood context that exposed the tool guidance shape
spec:
  - 260524-mcp-actor-setup-bootstrap
completed: 2026-05-25
---

# Trim actor-gate setup guidance in root-omitted agent tools

## Background

During ws-dashboard SQLite-backed agent detection dogfood, calling
`agents.register` before `ws.setup` returned a detailed setup explanation:

```text
setup required before root-omitted agents.register: call ws.setup(method:
"lead-workflow-bootstrap", root: "<absolute-working-directory>") from
lead-workflow-manual, or recover with ws.setup(id: "<actor-id>")
```

The behavior is technically actionable but too verbose for a tool error. The
full bootstrap ceremony belongs in `lead-workflow-manual`, which lead workflow
skills load before orchestration. The direct tool guidance should be short and
should emphasize the recovery shape, for example `ws.setup(id: "<actor-id>")`,
without embedding workflow-manual-level briefing text in every tool failure.

## Notes

Current evidence points to `Server.actorGate` in
`agents-plugin-tool/internal/mcp/server.go`; tests currently assert the verbose
absolute-root guidance, so the fix should update both the string and the
contract test expectations.

## Phases

### Phase 1: Trim root-omitted actor setup errors

Change root-omitted `agents.register`, `agents.call`, and `subquery` setup-gate
errors so the tool layer reports only the missing setup condition plus compact
recovery guidance such as `ws.setup(id: "<actor-id>")`.

Preserve the authoritative bootstrap ceremony in `lead-workflow-manual` and the
existing `ws.setup` tool schema/docs. Do not embed
`lead-workflow-bootstrap`, absolute-root examples, or "from
lead-workflow-manual" briefing text in the actor-gate error.

Verification should cover the MCP contract test that exercises root-omitted
`agents.register` before setup and should prove the removed verbose fragments
are absent.

### Result (41c7127) - 2026-05-25

`Server.actorGate` now reports root-omitted actor setup failures with compact
recovery guidance only: `ws.setup(id: "<actor-id>")`. The MCP contract test now
asserts that `agents.register` before setup still fails, includes the compact
recovery shape, and omits `lead-workflow-manual`,
`lead-workflow-bootstrap`, and absolute-root bootstrap briefing text.

Verification:

- `cd agents-plugin-tool && go test ./internal/mcp -run TestServeStdioSetupRootAndExplicitOverride`
- `cd agents-plugin-tool && go test ./internal/mcp`
