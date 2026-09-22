---
name: lead-run
description: Use when the user moves to execution on a ready or directly-named ticket, or when a change's behavioral impact, cross-module scope, or review needs warrant the full worker workflow. The worker explores, implements, verifies, runs independent review, records the result, and commits.
---

# Run

Call in parallel:
- `ws/playbook.read(name: "lead-run", session_key: <your key, omit if fresh>)`
- `ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `ws/playbook.read`.
If this call fails to connect, run `/ws:mcp-server-repair`.
