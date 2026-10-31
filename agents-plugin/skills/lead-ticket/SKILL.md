---
name: lead-ticket
description: Create, edit, promote, drop, or close a workflow ticket. Owns the Open Decision Queue, cheap-tier fact population, and the design-review gate into `ready/`.
---

<ws-preload playbook="lead-ticket">
# Ticket

Call `ws/playbook.read(name: "lead-ticket", session_key: <your key, omit if fresh>)` for the procedure.
</ws-preload>

Execute the lead-ticket procedure inline against the user request.
On a fresh session only, also call `ws/workflow_manual(session_key: "obsidian-latch", root: <absolute worktree path>)` to bootstrap, in the same batch as the playbook read when you make one; skip it when you already hold a session key.
