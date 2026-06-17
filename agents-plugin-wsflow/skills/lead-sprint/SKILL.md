---
name: lead-sprint
description: Use when the user wants an ongoing wsflow sprint session for discussion, exploration, small interactive edits, and handoff to the appropriate wsflow workflow.
---

# Sprint

Call `wsflow/playbook.print(name: "lead-sprint")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
