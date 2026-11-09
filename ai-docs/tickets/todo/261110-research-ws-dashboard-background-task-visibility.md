---
title: "Dashboard visibility of a session's background tasks (background shells, monitors): what Claude Code and Pi expose, and how to show 'N background tasks'"
related:
  261110-epic-ws-dashboard-agent-host: "daemon-hosted sessions would own their background tasks directly"
---

# Dashboard visibility of background tasks

## Background

User, 2026-11-10: while a session runs a background shell, the dashboard
shows nothing. They want a visible channel: "N 백그라운드 태스크" in the top
agents panel and next to the agent badge at the bottom.

The Claude dashboard feed is the ws claude-mod
(`agents-plugin/hooks/claude-mod/dashboard.ts`). Its header lists what it
sends through the session's intake tool: turn start and end, the lead's
batches, one `agent` event per subagent (from `agent.spawn` or
`$.agent.list()`), streamed reply text, user messages, pending permission
dialogs, and usage status. Background shells and monitors are not among
them.

## Topic 1: what Claude Code exposes

- Does the mod API (or a hook event) report a background Bash task's start,
  output, and exit, and Monitor or scheduled wakeups? Is there a list call
  like `$.agent.list()` for background tasks?
- If not, can the mod infer them from tool calls (`run_in_background: true`)
  and their completion notifications, and how reliable is that across
  compaction and resume?

## Topic 2: what Pi exposes

- Pi's equivalent of background processes, and whether its RPC or the ws Pi
  package can report them.

## Topic 3: what the dashboard shows

- The count in the agents panel and beside the bottom agent badge (the
  user's placement). Whether a row per task (command, elapsed time, state)
  is worth it, and whether a task can be stopped from the dashboard (a write
  that crosses the permission question in
  `261110-research-ws-dashboard-agent-host-foundations`, Topic 3).

## Outcome Ledger

### Verified Findings
- The claude-mod dashboard feed sends turns, lead batches, subagent `agent`
  events, reply text, user messages, permission waits, and usage; it sends
  nothing for background shells (`agents-plugin/hooks/claude-mod/dashboard.ts`
  header comment).

### Confirmed Decisions
- Show "N background tasks" in the top agents panel and next to the bottom
  agent badge (user, 2026-11-10).

### Proposals
- None yet.

### Open Questions
- Topics 1 to 3 above.

### Rejected Alternatives
- None yet.
