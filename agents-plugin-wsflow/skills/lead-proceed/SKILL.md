---
name: lead-proceed
description: Required first step for implementation tasks. Use when starting from a ticket path or task description so existing specs, tickets, plans, and session context are routed before code is touched.
---

# Proceed

Call `wsflow/playbook.read(name: "lead-proceed")` and execute the returned procedure
inline against the current user request. If this call fails to connect, run `/wsflow:mcp-server-repair`.
