---
name: lead-ticket
description: Create, edit, promote, drop, or close a workflow ticket. Owns the Open Decision Queue and delegates promotion, epic review, and revision to a ticket handler.
---

<ws-preload playbook="lead-ticket">
# Ticket

Call `wsflow/playbook.read(name: "lead-ticket")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
</ws-preload>
