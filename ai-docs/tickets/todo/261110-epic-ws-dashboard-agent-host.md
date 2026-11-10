---
title: "ws dashboard as the first-tier agent host: daemon-spawned lead sessions, multi-session and workspace management, terminal, file preview"
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
created. Two tracks (user, 2026-11-10 whiteboard, (01), (02)):

- **Track A, agent management.** The daemon spawns and manages headless lead
  sessions itself (Claude Code and Pi), alongside CLI-created sessions;
  workspaces (folder units) with their sessions; continuing a conversation,
  including between the terminal and the daemon; sessions that stay alive
  across daemon restarts.
- **Track B, agent development environment (ADE).** A terminal in the
  dashboard, and a read-only file browser with HTML, image, and video
  preview.

The browser dashboard is the primary surface. A desktop shell (Tauri,
Electron, or similar) is only reviewed after the browser dashboard is
complete ((18) a and note, (19)).

## Non-Scope

- Editing files in the dashboard, at any depth ((04) a and note).
- Designing for Orca as a host ((24)).
- Building a desktop shell; it is a post-completion review item ((19)).

## Child Tickets

Order ((27) c): the holder first, then the two tracks in parallel; file
preview at any time. Sequence (user, 2026-11-10, (68) a; announced, (70)):

1. `261110-feat-ws-dashboard-session-rail`, with the research ticket's
   remaining verifications running alongside (they read, not write).
2. `261110-feat-ws-dashboard-agent-panel-telemetry`.
3. `261110-feat-ws-dashboard-session-holder` and
   `261110-feat-ws-hosted-session-mode-flag` in parallel; workspaces
   Phase 1 may run beside them ((69) a).
4. `261110-feat-ws-dashboard-headless-lead-sessions` and
   `261110-feat-ws-dashboard-terminal` in parallel. The research
   verifications finish before headless-lead-sessions is promoted to
   `ready/` ((70)).
5. `261110-feat-ws-dashboard-workspaces` Phase 2.

`261110-feat-ws-dashboard-file-preview` fills gaps at any point.

- `261110-feat-ws-dashboard-session-holder` - shared foundation: a Python
  holder that owns agent processes and PTYs across daemon restarts.
- `261110-feat-ws-hosted-session-mode-flag` - track A: one env flag that
  hands input delivery and the dashboard feed to the daemon in the Claude
  mod and the Pi extension.
- `261110-feat-ws-dashboard-headless-lead-sessions` - track A: spawn,
  input injection, approval, recovery, idle stop. After the holder and the
  flag.
- `261110-feat-ws-dashboard-workspaces` - track A: stored workspaces and
  the rail's + buttons. Phase 1 (add workspace, add worktree) has no
  prerequisite; Phase 2 (new session, CLI/daemon handoff) comes after the
  headless leads ((69) a).
- `261110-feat-ws-dashboard-terminal` - track B: browser terminal on holder
  PTYs. After the holder.
- `261110-feat-ws-dashboard-file-preview` - track B: read-only browser and
  previews. No prerequisite.
- `261110-feat-ws-dashboard-session-rail` - track A, multi-session UX for
  today's sessions: the session rail grouped by workspace and worktree
  ((62) a), and per-session draft restore. No prerequisite.
- `261110-feat-ws-dashboard-agent-panel-telemetry` - track A, multi-session
  UX: peer lead branch, subagent model, effort, context tokens, and output
  rate. No prerequisite.
- `261110-research-ws-dashboard-agent-host-foundations` - remaining
  verifications: `-p` feature coverage and ws hooks under `-p`, Pi parity.

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
  Revised (user, 2026-11-10, (18) note, (19)): the desktop shell is not a
  target; the browser dashboard is primary, and a shell is only reviewed
  once the browser side is complete.
- **Placement** (user, 2026-11-10, (547)-(549) ok): a new epic rather than
  reopening `261106-epic-ws-dashboard-serve-daemon`, whose settled Non-Scope
  excludes exactly this scope (browser terminal, browser permission approval)
  and whose board other children already rely on. The three earlier tickets
  are `related:`, not parents. Rejected: reopening 261106 in place.

- **Survival across daemon restarts** (user, (03) a, (06) a). Daemon-hosted
  agents and terminals keep running while the daemon is updated or
  restarted: a per-session holder owns each process and the daemon
  reattaches. Holders are Python standard-library scripts ((06) note, (09)).
  Rejected: conversation-only survival through resume ((06) b, c).
- **Daemon leads are headless** (user, (12) b): `claude -p` stream-json and
  Pi RPC. Rejected: an interactive TUI inside a PTY ((12) a).
- **One trusted-origin boundary** (technical, (05); announced, (13)).
  Tool approval and the terminal are both execution authority and share one
  boundary: loopback or a desktop-shell origin by default, never the LAN
  token alone. LAN viewers can watch and chat.
- **Daemon owns input for its sessions** (technical, (16), (20)). The daemon
  injects dashboard messages and mail; the Claude mod and the Pi extension
  stop their own delivery and feed under one hosted-session flag and keep
  the rest. Rejected: excluding the mod or extension entirely.
- **Workspace = root worktree or plain folder** (user, (08) b; (11)).
  Linked worktrees show under their root; idle ws pooled worktrees are not
  shown ((54)); one that holds a session shows while it does ((71) a).

## Completion Criteria

- Done: every child ticket is done or dropped and the user closes the epic.
- Dropped: the user drops the direction.
- Deferred: the desktop shell review ((19)), and whatever the children's
  results defer.
