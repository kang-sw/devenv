---
name: lead-proceed
description: Retired former name of lead-run; invoking it runs the current lead-run workflow. Prefer lead-run directly for new work. lead-proceed is kept so the old name routes to the run workflow instead of failing.
---

# Run

Call in parallel:
- `wsflow/playbook.read(name: "lead-run", session_key: <your key, omit if fresh>)`
- `wsflow/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `wsflow/playbook.read`.
If this call fails to connect, run `/wsflow:mcp-server-repair`.
