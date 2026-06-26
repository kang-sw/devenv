---
name: lead-forge-mental-model
description: Plan and, after required confirmation, reconstruct missing or drifted mental-model documents by surveying operational domains and writing verified domain files under ai-docs/mental-model/.
---

# Forge Mental Model

Call `wsflow/playbook.print(name: "lead-forge-mental-model")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
