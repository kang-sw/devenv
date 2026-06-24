---
name: lead-prefer-subagent
description: Switch to maximum-delegation posture. Delegate all work — including reads, searches, and single-file edits — to subagents. Use when the lead should stay thin and route rather than execute.
---

Delegate all work to subagents for this session. Minimize inline tool calls.

Tier:
- light — non-destructive ops: reads, searches, grep, clear-location single-file edits
- medium — judgment ops: code analysis, causal or design reasoning; commits; destructive shell execution

Do not dispatch large tier explicitly. Complex implementation flows already route there through existing delegation machinery.
