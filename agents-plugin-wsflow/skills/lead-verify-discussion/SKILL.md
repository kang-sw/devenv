---
name: lead-verify-discussion
description: Run a compact discussion verification checkpoint with premise, evidence, and over-alignment checks.
---

# Verify Discussion

Call `wsflow/playbook.print(name: "lead-verify-discussion")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
