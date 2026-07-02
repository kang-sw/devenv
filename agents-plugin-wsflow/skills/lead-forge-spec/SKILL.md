---
name: lead-forge-spec
description: Plan and, after required confirmation, reconstruct missing or drifted specs by surveying the project, confirming behavioral domains, and writing verified anchor-keyed specs under ai-docs/spec/.
---

# Forge Spec

Call `wsflow/playbook.print(name: "lead-forge-spec")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
