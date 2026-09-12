---
name: lead-run
description: Drain the ready queue one ticket at a time, or execute implementation whose scope, behavioral impact, or review needs warrant the full worker workflow. The worker explores, implements, verifies, runs independent review, records the result, and commits.
---

# Run

Call in parallel:
- `ws/playbook.read(name: "lead-run", session_key: <your key, omit if fresh>)`
- `ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `ws/playbook.read`.
If this call fails to connect, run `/ws:mcp-server-repair`.
