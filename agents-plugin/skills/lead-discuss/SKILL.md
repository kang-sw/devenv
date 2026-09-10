---
name: lead-discuss
description: Use when the user wants to reason with the lead about direction, scope, risk, or trade-offs before capture or execution. The lead owns the conversation and decision-making; subagents may gather evidence only. Conversation only.
---

# Discuss

Call in parallel:
- `ws/playbook.read(name: "lead-discuss", session_key: <your key, omit if fresh>)`
- `ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `ws/playbook.read`.
If this call fails to connect, run `/ws:mcp-server-repair`.
