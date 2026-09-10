---
name: lead-delegate
description: Start or continue a session-local native subagent when the user wants a bounded task handed to an executor and may steer that same agent across turns. Use for investigation, diagnosis, drafting a requested deliverable, operational chores, and eligible low-impact changes. The lead chooses the prompt, model, tools, and permissions. Route collaborative direction-setting to lead-discuss and material implementation to lead-run.
---

# Delegate

Call in parallel:
- `wsflow/playbook.read(name: "lead-delegate", session_key: <your key, omit if fresh>)`
- `wsflow/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `wsflow/playbook.read`.
If this call fails to connect, run `/wsflow:mcp-server-repair`.
