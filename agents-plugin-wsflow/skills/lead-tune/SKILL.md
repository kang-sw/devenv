---
name: lead-tune
description: Use when the user wants to tune or customize how the ws workflow runs by overriding workflow prompt text — for example the lead's delegation posture/eagerness. Fires on standing preferences such as "make the lead delegate less", and proposes the matching prompt-override tune.
---

# Workflow Tuning

Call `wsflow/playbook.print(name: "lead-tune")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
