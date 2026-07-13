---
title: Workflow manual bootstrap sentinel surface mismatch
related:
  260625-feat-ws-session-state-machine: workflow-manual restoration and session-state tools share the same session-key cache
related-mental-model:
  - mcp-runtime
dropped: 2026-07-13
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


## Resolution (2026-07-13)

Superseded by the M3 mercenary-reshape work: `WS_MCP_TOOL_PROFILE` env-based profile gating was retired (commit e900a46e) and folded into the server-side keyed capability gate (commit 8cd57344), and `260625-feat-ws-session-state-machine` hardened `workflow_manual` itself (lead-only gating, fail-loud on unresolvable key). `ai-docs/spec/mcp-tools.md {#260505-tool-profile-gating}` now documents that `tools/list` advertises the full lead surface regardless of caller environment and a lead-held session key is not restricted by this gate — the exact failure mode this ticket described no longer has a code path to reproduce.
