---
name: lead-run
description: Execute work through a spawned worker. Drains `ready/` one ticket per invocation (re-invoked until nothing advanceable remains), or runs an ad-hoc `run <description>`. The lead selects, spawns, waits, handles stops, and merges the goal branch on approval; it never edits source.
---

# Run

Call in parallel:
- `wsflow/playbook.read(name: "lead-run", session_key: <your key, omit if fresh>)`
- `wsflow/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `wsflow/playbook.read`.
If this call fails to connect, run `/wsflow:mcp-server-repair`.
