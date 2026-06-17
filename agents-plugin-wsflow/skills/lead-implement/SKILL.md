---
name: lead-implement
description: Use only after lead-proceed or an equivalent approved routing step has determined that implementation should begin in wsflow.
---

# Implement

Call `wsflow/playbook.print(name: "lead-implement")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
