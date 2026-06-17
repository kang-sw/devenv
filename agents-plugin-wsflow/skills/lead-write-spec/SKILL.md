---
name: lead-write-spec
description: Use when the user explicitly requests a spec change, or when another wsflow playbook routes to spec authoring for caller-visible workflow behavior.
---

# Write Spec

Call `wsflow/playbook.print(name: "lead-write-spec")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
