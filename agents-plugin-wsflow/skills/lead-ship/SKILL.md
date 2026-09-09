---
name: lead-ship
description: Ship, release, publish, tag, or deploy a configured project from its `ai-docs/ship` config. The release gate and the publish confirmation stay with the lead and the user; the mechanical steps go to a delegate.
---

# Ship

Call `wsflow/playbook.read(name: "lead-ship")` and execute the returned procedure
inline against the current user request. If this call fails to connect, run `/wsflow:mcp-server-repair`.
