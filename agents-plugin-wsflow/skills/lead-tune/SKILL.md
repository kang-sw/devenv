---
name: lead-tune
description: Use when the user wants to tune or customize how the wsflow workflow runs through catalog-backed prompt overrides or shared workflow delegation posture. Fires on standing preferences such as "make the lead delegate less", and proposes the matching tune.
---

# Workflow Tuning

Call `wsflow/playbook.read(name: "lead-tune")` and execute the returned procedure
inline against the current user request. If this call fails to connect, run `/wsflow:mcp-server-repair`.
