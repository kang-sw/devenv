---
name: lead-discuss
description: Use before code changes when the user wants to explore workflow design, migration direction, ticket scope, risks, or implementation approach.
---

# Discuss

Call `wsflow/playbook.print(name: "lead-discuss")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
