---
name: lead-tune
description: Use when the user wants to tune or customize how the ws workflow runs — prompt overrides, lead delegation posture/eagerness, review depth, or model tiers — or finds it too heavy, slow, or expensive. Fires on standing preferences such as "delegate less", "stop spawning so many agents", "use a cheaper model", "the workflow is too heavy", "turn review on/off", or "more/less review", and explains the levers or proposes the matching tune.
---

# Workflow Tuning

Call `ws/playbook.read(name: "lead-tune")` and execute the returned procedure
inline against the user request.
If this call fails to connect, run `/ws:mcp-server-repair`.
