---
title: Trim actor-gate setup guidance in root-omitted agent tools
related:
  260524-mcp-actor-setup-bootstrap: actor setup and recovery contract
  260525-feat-ws-dashboard-sqlite-agent-activity-source: dogfood context that exposed the tool guidance shape
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
