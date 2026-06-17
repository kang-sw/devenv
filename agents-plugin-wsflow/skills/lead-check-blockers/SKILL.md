---
name: lead-check-blockers
description: Check whether the active discussion is blocked; return either the blocker and needed input, or the next workflow route.
---

# Check Blockers

Call `wsflow/playbook.print(name: "lead-check-blockers")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
