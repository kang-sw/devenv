---
name: lead-tune
description: Use when the user wants to tune or customize how the wsflow workflow runs through catalog-backed prompt overrides, shared workflow delegation posture, or review depth, or finds it too heavy, slow, or expensive. Fires on standing preferences such as "make the lead delegate less", "the workflow is too heavy", "turn review on/off", or "more/less review", and explains the levers or proposes the matching tune.
---

# Workflow Tuning

Call `wsflow/playbook.read(name: "lead-tune")` and execute the returned procedure
inline against the current user request. If this call fails to connect, run `/wsflow:mcp-server-repair`.
