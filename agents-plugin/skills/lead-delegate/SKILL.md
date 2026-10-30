---
name: lead-delegate
description: Start or continue a session-local native subagent when the user wants a bounded task handed to an executor and may steer that same agent across turns. Use for investigation, diagnosis, drafting a requested deliverable, operational chores, and settled code changes whose files can be named up front. The lead chooses the prompt, model, tools, and permissions. Route collaborative direction-setting to lead-discuss; capture work with an open decision, new structure, or unnamed scope through lead-ticket, then run it through lead-run.
---

# Delegate

<ws-preload playbook="lead-delegate">
Call in parallel:
- `ws/playbook.read(name: "lead-delegate", session_key: <your key, omit if fresh>)`
- `ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `ws/playbook.read`.
</ws-preload>
