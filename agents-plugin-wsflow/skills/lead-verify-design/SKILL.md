---
name: lead-verify-design
description: Run a critical design review from the provided brief, checking premises first and using an isolated reviewer context when available.
---

# Verify Design

Call `wsflow/playbook.print(name: "lead-verify-design")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
