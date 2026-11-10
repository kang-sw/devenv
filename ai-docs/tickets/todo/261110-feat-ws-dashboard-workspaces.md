---
title: "Dashboard agent host: workspaces (root worktree or plain folder), per-workspace sessions, and conversation handoff between CLI and daemon"
parent: 261110-epic-ws-dashboard-agent-host
blocked-by: 261110-feat-ws-dashboard-headless-lead-sessions
related:
  261110-research-ws-dashboard-agent-host-foundations: "Topic 4 (multi-session and workspace management)"
---

# Dashboard workspaces

## Background

The user wants to manage agents from the dashboard by folder: add a
workspace, see and start its sessions, and continue a conversation
(2026-11-10 whiteboard, (01)). Session spawn, the terminal's working
directory, and the file preview root are all chosen by workspace.

## Decisions

1. **A workspace is a repository's root worktree or a plain non-git folder
   (user, (08) b and note; announced, (11)).** Only those can be added. A
   linked worktree, including a ws pooled worktree from
   `ws/worktree.acquire`, shows under its root workspace, and a session binds
   to a worktree within its workspace, so a lead and its workers sit in one
   workspace.
   Rejected: every folder its own workspace, grouped in the UI only ((08) a);
   a free multi-root set of folders ((08) c).
2. **Conversation handoff in both directions (user, (15) a).** The daemon can
   take over a Claude or Pi session that a CLI process started, once that
   process has exited, and resume it headless; a daemon session can be
   handed to the terminal by stopping its process and giving the user a
   `claude --resume <id>` command. One session ID is never held by two
   processes at once: before a handoff the previous process must have
   exited.
   Rejected: CLI to daemon only ((15) b); daemon sessions only ((15) c).

## Constraints

- The user also asked (2026-11-10) for multi-session management UX that
  does not depend on the terminal, to be discussed next. Its scope overlaps
  this ticket's session list; reconcile the two before promotion.
- To settle at promotion: where workspaces are stored; how an existing
  CLI session is matched to a workspace; the session list per workspace
  (lead and worker visibility); stop, archive, and remove; how the agents
  panel and the session page scale to many sessions.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`.

## Phases

### Phase 1: workspace model and session list

Scope and verification are settled at promotion, after the Constraints'
open items.
