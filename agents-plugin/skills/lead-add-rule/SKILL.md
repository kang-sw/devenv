---
name: lead-add-rule
description: Persist a user-requested workflow rule. Use only when the user explicitly asks to save, remember, persist, or add a durable rule for future sessions.
---

# Add Rule

Call `ws/playbook.read(name: "lead-add-rule")` and execute the returned procedure
inline against the user request.
If this call fails to connect, run `/ws:mcp-server-repair`.
