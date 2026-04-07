---
name: execute-plan
description: "Deprecated — use /implement instead. Plans are loaded via write-plan's plan-mode output."
argument-hint: <plan-path>
---

# Execute Plan — Deprecated

This skill has been merged into `/implement`. The implement skill now
handles both plan-driven and ad-hoc implementation.

Use `/implement` instead. If a plan file needs to be loaded, use
write-plan's plan-mode output which directs: `Load /implement skill`,
`Read @<plan-path>`.
