---
title: "Dashboard agent host: a browser terminal on holder-owned PTYs behind the trusted-origin boundary"
parent: 261110-epic-ws-dashboard-agent-host
blocked-by: 261110-feat-ws-dashboard-session-holder
related:
  261110-research-ws-dashboard-agent-host-foundations: "Topic 1 (archived ws-dashboard terminal)"
  261106-epic-ws-dashboard-serve-daemon: "its Non-Scope (no browser terminal) is reopened here"
---

# Dashboard terminal

## Background

Track B of the epic: restore a terminal in the dashboard. The archived
ws-dashboard (tag `archive/ws-dashboard`, `bc2e54e89`) had one: Rust,
`portable_pty`, a websocket transport with output backfill, shell selection,
and tab restore as new shells after a daemon restart. Tickets in `.done/`:
`260516-feat-ws-web-terminal-session-substrate`,
`260516-bug-ws-web-terminal-websocket-transport`,
`260516-bug-ws-web-terminal-cross-platform-portability`,
`260523-feat-ws-dashboard-terminal-tab-restore`,
`260620-bug-dashboard-terminal-inherits-dumb-term`.

## Decisions

1. **PTYs live in the session holder, so shells survive a daemon restart
   (user, (06) a; technical, (09)).** Each terminal is a holder-owned PTY
   (`261110-feat-ws-dashboard-session-holder`); after a restart the daemon
   reattaches instead of creating new shells.
2. **The archived Rust PTY code is not carried over (announced, (23)).** Only
   its frontend and websocket protocol design are references. The holder
   decision made the research's "port to Go or run Rust as a sidecar"
   question moot.
3. **Terminal access uses the approval trust boundary (technical, (05)).** A
   browser terminal is arbitrary shell execution, more than the tool
   approval that `261106-epic-ws-dashboard-serve-daemon` 241 b refused to
   put behind the LAN token. By default it is available only from loopback
   or a desktop-shell origin, the same boundary as approval in
   `261110-feat-ws-dashboard-headless-lead-sessions`. Rejected: a separate
   terminal boundary (the weaker of two boundaries becomes the real one).

## Constraints

- Verify how the archived MVP gated terminal access before designing the
  gate; it was not checked on 2026-11-10.
- The terminal is independent of daemon lead sessions; it depends only on
  the holder.
- The working directory comes from the workspace model
  (`261110-feat-ws-dashboard-workspaces`) once it exists; before then, the
  terminal opens in a folder the user picks.
- To settle at promotion: the terminal emulator library; shell selection per
  platform; scrollback size; tab layout in the session page.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/windows-dogfood.md`.

## Phases

### Phase 1: terminal on holder PTYs

Scope and verification are settled at promotion, after the Constraints'
open items. Verification must include a daemon restart with an open shell
on Linux, macOS, and native Windows.
