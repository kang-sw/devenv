---
title: Workflow manual bootstrap sentinel surface mismatch
related:
  260625-feat-ws-session-state-machine: workflow-manual restoration and session-state tools share the same session-key cache
related-mental-model:
  - mcp-runtime
---

# Workflow manual bootstrap sentinel surface mismatch

## Background

During 2026-06-26 dogfooding, `ws:lead-proceed` rendered guidance saying a fresh
start should call `ws.workflow_manual(session_key: "obsidian-latch")`.
Tool discovery in this host session did not expose a callable
`workflow_manual` tool, while exposed `ws.enter.*` tools required a real session
record. A direct `ws.enter.implement(session_key: "obsidian-latch", ...)` call
failed with:

```text
ws.enter.implement: session key not found: obsidian-latch
```

That failure is understandable for `ws.enter.*`, but the operator experience is
surprising: the playbook names a bootstrap sentinel that is not usable by the
visible state tools, and the manual entry tool was not discoverable through the
current tool-search surface.

## Follow-Up

Investigate whether this is only a tool-discovery exposure gap, stale playbook
bootstrap prose, or an intended sentinel-only path that needs clearer lead
guidance. The fix should preserve the current security invariant that unknown
session keys do not render the full workflow manual body or leak bootstrap
details to non-lead callers.

## Additional Evidence - 2026-06-27

During a `lead-proceed` dogfood relay, tool discovery exposed
`mcp__ws.ws_workflow_manual`, but calling it with a valid lead session key failed
with:

```text
tool not available in current ws MCP profile: ws.workflow_manual
```

The same session could call `ws.enter.proceed` and `ws.enter.implement`, so this
is not just a missing MCP connection. Investigate whether discovery is surfacing
profile-hidden tools, whether the active profile should expose
`ws.workflow_manual`, or whether the playbook should name a visible fallback
when profile filtering hides the manual.
