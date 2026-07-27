---
name: lead-write-ticket
description: Use when the user asks to create, edit, promote, drop, close, or durably capture a repository workflow ticket.
---

# Write Ticket

Call `wsflow/playbook.print(name: "lead-write-ticket")` and execute the returned procedure
inline against the current user request. If this call fails to connect, run `/wsflow:mcp-server-repair`.
