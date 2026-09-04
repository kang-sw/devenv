---
name: lead-forge-spec
description: Reconstruct specs from scratch by surveying the project, confirming behavioral domains, and guiding a user-confirmed authoring loop that produces complete anchor-keyed specs under ai-docs/spec/.
---

# Forge Spec

Call `ws/playbook.read(name: "lead-forge-spec")` and execute the returned procedure
inline against the user request.
If this call fails to connect, run `/ws:mcp-server-repair`.
