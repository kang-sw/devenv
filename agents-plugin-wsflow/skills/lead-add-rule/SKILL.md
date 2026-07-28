---
name: lead-add-rule
description: Persist a user-requested workflow rule. Use only when the user explicitly asks to save, remember, persist, or add a durable rule for future sessions.
---

# Add Rule

Call `wsflow/playbook.print(name: "lead-add-rule")` and execute the returned procedure
inline against the current user request. If this call fails to connect, run `/wsflow:mcp-server-repair`.
