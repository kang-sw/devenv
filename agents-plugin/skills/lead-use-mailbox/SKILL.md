---
name: lead-use-mailbox
description: Guide the lead and user through cross-session mail between independent harness sessions — registering a mailbox address, discovering peers, sending and receiving, arming a wait so an idle session wakes on mail, and working with another session as a peer lead of equal authority, such as asking it to run a ticket. Use when the user wants to message another session, set up or find a mailbox address, wake an idle session, or coordinate work with another lead session.
---

<ws-preload playbook="lead-use-mailbox">
# Use Mailbox

Call in parallel:
- `ws/playbook.read(name: "lead-use-mailbox", session_key: <your key, omit if fresh>)`
- `ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

After both return, execute the procedure returned by `ws/playbook.read`.
</ws-preload>
