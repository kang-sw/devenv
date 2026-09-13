---
name: lead-use-mailbox
description: Guide the lead and user through cross-session mail between independent harness sessions — registering a mailbox address, discovering peers, sending and receiving, arming a wait so an idle session wakes on mail, and remote-controlling another session's idle agent loop by mailing it an instruction such as running a ticket. Use when the user wants to message another session, set up or find a mailbox address, wake an idle executor, or have one session direct another.
---

# Use Mailbox

Call `wsflow/playbook.read(name: "lead-use-mailbox")` and execute the returned procedure
inline against the current user request. If this call fails to connect, run `/wsflow:mcp-server-repair`.
