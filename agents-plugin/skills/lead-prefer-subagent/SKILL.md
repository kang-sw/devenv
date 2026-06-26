---
name: lead-prefer-subagent
description: Switch to maximum-delegation posture. Delegate all work — including reads, searches, and single-file edits — to subagents. Use when the lead should stay thin and route rather than execute.
---

# Prefer Subagent

Call `ws/playbook.print(name: "lead-prefer-subagent")` and execute the returned procedure
inline. If the playbook cannot be loaded, stop and report that blocker.
