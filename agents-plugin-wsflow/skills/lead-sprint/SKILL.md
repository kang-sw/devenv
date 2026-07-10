---
name: lead-sprint
description: Use when the user wants an ongoing wsflow sprint session for discussion, exploration, small interactive edits, and handoff to the appropriate wsflow workflow.
---

# Sprint

Call in parallel:
- `wsflow/playbook.print(name: "lead-sprint", session_key: <your key, omit if fresh>)`
- `wsflow/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `wsflow/playbook.print`.
