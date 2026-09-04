---
name: lead-scope-worktree
description: Scope this worktree's ticket board to one work line via git sparse-checkout, hiding out-of-topic ready/todo/idea tickets. Use when the user wants a worktree dedicated to one work line, topic, or ticket subset.
---

# Scope Worktree

Call `ws/playbook.read(name: "lead-scope-worktree")` and execute the returned procedure
inline against the user request.
If this call fails to connect, run `/ws:mcp-server-repair`.
