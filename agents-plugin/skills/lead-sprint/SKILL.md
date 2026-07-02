---
name: lead-sprint
description: Use when the user wants an ongoing sprint session for discussion, exploration, small interactive edits, and normal workflow handoff.
---

# Sprint

Call in parallel:
- `ws/playbook.print(name: "lead-sprint", session_key: <your key>)`
- `ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `ws/playbook.print`.
