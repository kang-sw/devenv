---
name: lead-write-ticket
description: Use when the user asks to create, edit, promote, drop, close, or durably capture a repository workflow ticket.
---

# Write Ticket

Call `ws/playbook.read(name: "lead-write-ticket", session_key: <your key, omit if fresh>)` and execute the returned procedure inline against the user request.
On a fresh session only, also call `ws/workflow_manual(session_key: "obsidian-latch", root: <absolute worktree path>)` in the same batch to bootstrap; skip it when you already hold a session key.
If this call fails to connect, run `/ws:mcp-server-repair`.
