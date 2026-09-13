---
name: lead-audit-doc
description: Draft, revise, or audit frequently reread project documents such as AGENTS.md, rules, manuals, procedures, and references. Also use after materially editing one of these documents to remove over-negation and defensive prose, and to offer an independent fresh-read audit.
---

# Audit Doc

Call `ws/playbook.read(name: "lead-audit-doc")` and execute the returned procedure
inline against the user request.
If this call fails to connect, run `/ws:mcp-server-repair`.
