---
name: lead-ticket
description: Create, edit, promote, drop, or close a workflow ticket. Owns the Open Decision Queue, cheap-tier fact population, and the design-review gate into `ready/`.
---

# Ticket

Call `ws/playbook.read(name: "lead-ticket", session_key: <your key, omit if fresh>)` and execute the returned procedure inline against the user request.
On a fresh session only, also call `ws/workflow_manual(session_key: "obsidian-latch", root: <absolute worktree path>)` in the same batch to bootstrap; skip it when you already hold a session key.
If this call fails to connect, run `/ws:mcp-server-repair`.
