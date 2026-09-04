---
name: lead-backfill-docs
description: Use when commits have landed without a documentation pass and spec or mental-model coverage needs to be reconciled retroactively, including ad-hoc work that never went through an implementation flow.
---

# Backfill Docs

Call in parallel:
- `wsflow/playbook.read(name: "lead-backfill-docs", session_key: <your key, omit if fresh>)`
- `wsflow/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `wsflow/playbook.read`.
If this call fails to connect, run `/wsflow:mcp-server-repair`.
