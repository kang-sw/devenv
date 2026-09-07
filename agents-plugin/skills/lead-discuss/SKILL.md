---
name: lead-discuss
description: Use before code changes when the user wants to explore workflow design, migration direction, ticket scope, risks, or implementation approach.
---

# Discuss

Call in parallel:
- `ws/playbook.read(name: "lead-discuss", session_key: <your key, omit if fresh>)`
- `ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `ws/playbook.read`.
If this call fails to connect, run `/ws:mcp-server-repair`.
