---
name: lead-ship
description: Use when the user explicitly asks to ship, release, publish, tag, or deploy a configured project; follows the playbook gates and stops before irreversible external actions unless authorization is explicit.
---

# Ship

Call `wsflow/playbook.print(name: "lead-ship")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
