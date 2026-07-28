---
name: lead-implement
description: Use only after lead-proceed or an equivalent approved routing step has determined that implementation should begin in wsflow.
---

# Implement

Call `wsflow/playbook.print(name: "lead-implement")` and execute the returned procedure
inline against the current user request. If this call fails to connect, run `/wsflow:mcp-server-repair`.
