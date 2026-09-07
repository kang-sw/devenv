---
name: lead-update-spec
description: Post-implementation spec audit primitive for explicit commit ranges or workflow wrap-up; updates ai-docs/spec for caller-visible behavior changes.
---

# Update Spec

Call `wsflow/playbook.read(name: "lead-update-spec")` and execute the returned procedure
inline against the current user request. If this call fails to connect, run `/wsflow:mcp-server-repair`.
