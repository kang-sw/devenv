---
name: lead-discuss
description: Use before any change when the user wants to think through direction, scope, risk, or approach. Conversation only; capture goes through the ticket skill and execution through run.
---

# Discuss

Call in parallel:
- `ws/playbook.read(name: "lead-discuss", session_key: <your key, omit if fresh>)`
- `ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `ws/playbook.read`.
If this call fails to connect, run `/ws:mcp-server-repair`.
