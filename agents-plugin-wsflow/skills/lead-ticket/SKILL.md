---
name: lead-ticket
description: Create, edit, promote, drop, or close a workflow ticket. Owns the Open Decision Queue, cheap-tier fact population, and the design-review gate into `ready/`.
---

# Ticket

Call `wsflow/playbook.read(name: "lead-ticket")` and execute the returned procedure
inline against the current user request. If this call fails to connect, run `/wsflow:mcp-server-repair`.
