---
name: lead-ticket
description: Create, edit, promote, drop, or close a workflow ticket. Owns the Open Decision Queue, cheap-tier fact population, and the design-review gate into `ready/`.
---

# Ticket

<ws-preload playbook="lead-ticket">
Call `wsflow/playbook.read(name: "lead-ticket")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
</ws-preload>
