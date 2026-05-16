---
title: ws web dashboard workRoot IO substrate
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workbench-substrate: completed workbench host substrate
  260516-feat-ws-web-local-workspace-discovery: completed workRoot discovery and picker prerequisite
  260516-feat-ws-web-instance-event-stream: completed event envelope and stream scaffold prerequisite
related-mental-model:
  - ws-web-dashboard
---

# ws web dashboard workRoot IO substrate

## Scope

Make an opened workRoot useful without introducing a harness-specific agent UI.
This milestone should add the first live filesystem and terminal surfaces on
top of the existing authenticated daemon, resource model, route basis, and
workbench substrate.

The milestone should establish:

- A workRoot-local file navigator in the lower portion of the left navigation
  area.
- Read-only text file panes that open from file navigation into the workbench,
  preferably the second or later split group so active terminal or future agent
  work is not displaced.
- A daemon-owned terminal session substrate with create, list, stream, input,
  resize, and close behavior.
- Refresh-safe terminal persistence: browser refresh or route re-entry must
  not destroy daemon-owned terminal sessions.
- Explicit terminal close semantics: closing a terminal panel terminates that
  terminal session rather than creating hidden detached terminal state.
- Workbench integration that combines daemon-owned session existence with
  browser-owned arrangement and placement.

## Non-Scope

- Write-back editing, save, dirty state, conflict handling, formatting, or
  language-server integration.
- Generic file-manager operations such as delete, rename, move, copy, recursive
  folder deletion, chmod, or symlink management.
- Hardcoded Codex, Claude, or other agent spawn presets.
- Harness-specific protocol integration, transcript parsing, subagent list
  extraction, ws MCP context-file parsing, or named-agent control.
- Detached terminal restore UX. Terminal sessions remain visible after refresh
  because the daemon owns them, but explicit close terminates them.
- Full terminal multiplexing, shared panes, terminal search, scrollback
  persistence beyond the first usable substrate, or custom keybinding UI.

## Child Tickets

- `260516-feat-ws-web-workroot-file-navigation` - done; authenticated
  workRoot file listing and a left-nav file explorer draft.
- `260516-feat-ws-web-readonly-text-pane` - done; authenticated read-only text
  file open path and workbench text pane placement.
- `260516-feat-ws-web-terminal-session-substrate` - todo; daemon-owned PTY
  terminal session lifecycle, I/O forwarding, refresh persistence, and close
  termination.
- `260516-feat-ws-web-workroot-io-workbench-integration` - todo; combine file
  panes and terminal sessions with workbench placement, restore, and dogfood
  verification.

## Cross-Child Decisions

- The milestone centers `workRoot`, not `mainInstance`. Main instances remain
  durable workbench surfaces for later agent-oriented work, but this milestone
  should prioritize concrete filesystem and terminal use.
- File navigation belongs in the left navigation area as a selected-workRoot
  auxiliary surface. It should not turn the browser into a general file
  manager.
- File opens prefer the second or later split group. If only one group exists,
  the workbench may create or choose a support group rather than replacing an
  active terminal where practical.
- Text panes are read-only. Unsafe, binary, unreadable, too-large, or
  unsupported files should surface an honest preview-unavailable state.
- Terminal sessions are daemon-owned live resources. Browser layout state may
  decide where a terminal is shown, but session existence and process lifecycle
  are daemon authority.
- Terminal close terminates the session. Future confirmation prompts should
  protect that operation, especially when foreground process detection exists,
  but the first substrate may reserve the command hook before adding rich
  process inspection.
- Agent-oriented usage remains possible by running tools inside a terminal.
  The dashboard should not hardcode Codex or Claude spawn options in this
  milestone.
- Terminal logical dimensions should not churn continuously during visual split
  drag. Resize forwarding should be explicit, throttled, committed, or otherwise
  bounded enough to avoid destructive TUI redraw behavior.

## Completion Criteria

- Done: child tickets let an owner open a workRoot, browse files, open
  read-only text panes, create live terminal sessions, refresh without losing
  terminal sessions, explicitly close terminal sessions, and verify the flow
  through the daemon-served frontend.
- Dropped: a different near-term usability direction replaces the workRoot
  filesystem and terminal substrate, or live PTY support proves unsuitable for
  the intended dashboard MVP.
- Deferred: write-back editing, hardcoded agent presets, named-agent controls,
  terminal multiplexing depth, detached terminal restore UX, and full IDE file
  management belong to later milestones.
