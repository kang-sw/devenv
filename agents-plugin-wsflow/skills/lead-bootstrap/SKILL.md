---
name: lead-bootstrap
description: Bootstrap or upgrade a downstream project to AGENTS.md-based wsflow workflow context while preserving Claude compatibility.
---

# Bootstrap

<ws-preload playbook="lead-bootstrap">
Call `wsflow/playbook.read(name: "lead-bootstrap")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.
</ws-preload>
