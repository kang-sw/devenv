---
name: lead-goal-fan-out-step
description: Advance a goal-pursuit run by one step with batch-parallel worktree fan-out when two or more ready tickets are mutually independent and recursive subagent dispatch is available; falls back to lead-goal-step's serial single-ticket step otherwise.
---

# Goal Fan-Out Step

Call in parallel:
- `ws/playbook.print(name: "lead-goal-fan-out-step", session_key: <your key, omit if fresh>)`
- `ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `ws/playbook.print`.
