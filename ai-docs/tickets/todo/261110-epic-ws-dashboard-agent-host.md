---
title: "ws dashboard as the first-tier agent host: daemon-spawned lead sessions, multi-session and workspace management, terminal, desktop shell"
sage-review-design: required
related:
  260730-research-ws-dashboard-drop-for-orca: partially reversed - the daemon takes back session hosting and the terminal that this decision ceded to Orca
  261106-epic-ws-dashboard-serve-daemon: the daemon this epic builds on; its Non-Scope (no browser terminal; no browser permission approval, 241 b) is reopened here
  261107-epic-ws-dashboard-messenger-round2: the session page as the primary conversation surface; reused for daemon-hosted sessions
---

# ws dashboard as the first-tier agent host

## Scope

Make the ws dashboard daemon a program the user starts work from, instead of
a surface that only attaches to sessions a `claude` (or `pi`) CLI process
created:

- The daemon spawns and manages lead sessions itself (Claude Code and Pi,
  headless), alongside CLI-created sessions.
- Multi-session and workspace management from the dashboard: creating,
  resuming, and stopping sessions; binding sessions to workspaces and
  worktrees.
- A terminal in the dashboard.
- Eventually a desktop shell around the dashboard (Tauri, Electron, or
  similar), in the manner of an agent development environment.

## Non-Scope

None settled yet; `261110-research-ws-dashboard-agent-host-foundations`
proposes boundaries for the user to confirm.

## Child Tickets

- `261110-research-ws-dashboard-agent-host-foundations` - foundations
  research: the archived ws-dashboard terminal substrate, headless lead
  sessions, the permission-approval boundary, multi-session and workspace
  management, desktop shell options. Implementation children are planned
  from its Confirmed Decisions.

## Cross-Child Decisions

- **Direction** (user, 2026-11-10, whiteboard (538) b): pursue the full
  agent-host scope above as one epic, terminal and desktop shell included.
  This partially reverses `260730-research-ws-dashboard-drop-for-orca`,
  whose reason was that Orca already did the generic half better, not a
  technical failure; since then the daemon regained session control
  (messenger, steer, interrupt) and `wsagent` runs headless Claude and Pi
  conversations for the tracker support agent. Rejected: a narrower research
  ticket limited to headless lead sessions and the permission boundary
  ((538) a); keeping CLI-created sessions as the only kind ((538) c).
- **Placement** (user, 2026-11-10, (547)-(549) ok): a new epic rather than
  reopening `261106-epic-ws-dashboard-serve-daemon`, whose settled Non-Scope
  excludes exactly this scope (browser terminal, browser permission approval)
  and whose board other children already rely on. The three earlier tickets
  are `related:`, not parents. Rejected: reopening 261106 in place.

## Completion Criteria

- Done: every child ticket is done or dropped and the user closes the epic.
- Dropped: the user drops the direction.
- Deferred: whatever the research's confirmed boundaries leave out.
