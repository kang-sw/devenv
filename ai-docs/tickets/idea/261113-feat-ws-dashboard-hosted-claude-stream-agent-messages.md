---
title: "Stream agent messages live in dashboard-hosted Claude sessions"
---

# Stream agent messages live in dashboard-hosted Claude sessions

## Background

Owner request (2026-11-13). In a dashboard-hosted Claude session, the agent's reply does not stream into the conversation view. The whole reply appears at once, as one block, only when the turn ends. The owner wants the agent's messages streamed as they are produced, the way a terminal Claude session shows them.

## Notes

- Non-authoritative. Check whether the hosted Claude runner launches Claude with a streaming output mode. For example, stream-json output with partial messages; check the real flag names against the installed Claude CLI. Also check whether the dashboard forwards partial-message events to the conversation view, or only the final assistant message.
- Pi-hosted sessions may already stream. Compare the two paths before designing.
- Compatibility: any new event or field must be T0/T1 (AGENTS.md "Avoid T2").
