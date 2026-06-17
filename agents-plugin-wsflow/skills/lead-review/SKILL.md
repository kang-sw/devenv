---
name: lead-review
description: Use when the user wants to review a pull request or merge request branch; loads or creates review config, runs structured review phases, and routes to caller-approved fix, comment, or merge actions.
---

# Review

Call `wsflow/playbook.print(name: "lead-review")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
