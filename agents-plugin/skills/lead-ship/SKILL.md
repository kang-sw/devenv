---
name: lead-ship
description: Use when the user asks to ship, release, publish, tag, or deploy a configured project; follows the ai-docs/ship configuration.
---

# Ship

Call `ws/playbook.read(name: "lead-ship")` and execute the returned procedure
inline against the user request.
If this call fails to connect, run `/ws:mcp-server-repair`.
