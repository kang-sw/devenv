---
title: ws dashboard terminal and UX polishing
sage-review: required
related:
  260514-epic-ws-web-dashboard-mvp: retired predecessor board; this epic absorbs its UX/terminal/visual polish backlog
  260622-epic-ws-dashboard-session-key-realignment: sibling successor board that absorbs the agent-harness/session-key direction from the same split
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard terminal and UX polishing

## Scope

One of two successor boards created by splitting the retired
`260514-epic-ws-web-dashboard-mvp`. This epic owns the dashboard-centric
UX and terminal-interaction polish backlog: treating the dashboard as a
practical terminal/workbench environment (tmux-like PTY usability,
control-key fidelity, terminal UX redesign candidates), visual design
system coherence, accessibility-primitive adoption, document/editor
viewer polish, WorkRoot and Git-toolbar polish, and persisted UI state
coverage.

None of this work is gated on the agent-harness/session-key direction;
it can proceed independently of `260622`.

## Non-Scope

- Agent/session/harness integration, provider adapters, managed vendor
  CLI terminal bootstrap/submit policy, and anything gated on the
  ferrule/session-key model — owned by
  `260622-epic-ws-dashboard-session-key-realignment`.
- Cross-machine/linked-server forwarding and remote hardening — tracked
  under `260622` via `260525-feat-ws-dashboard-server-scoped-operation-forwarding`.
- New product surfaces or features beyond polishing what the retired MVP
  epic already delivered.

## Child Tickets

- `260711-idea-dashboard-readonly-file-pane-order-split-registry-bug` -
  idea; checks whether `readOnlyFilePaneOrderByGroup` has the same
  drag-move snap-back bug that was just fixed for
  `terminalPaneOrderByGroup` (commit `bc566a78`).
- `260720-bug-dashboard-terminal-split-nonhorizontal-snap-back` - todo;
  dogfooded report that 3-way, vertical, and other non-horizontal Dockview
  terminal splits still snap back at drop time even though the 2-way
  horizontal case (`260714`) is fixed; root cause not yet pinned down.
- `260714-bug-dashboard-terminal-pane-split-mirror-key-mismatch` - done;
  the `bc566a78` mirror fix itself compared `paneId`-space ids against a
  `logicalKey`-keyed map and always emptied the mirror, so the terminal
  split snap-back symptom persisted after that fix landed; corrected.
- `260517-bug-ws-dashboard-windows-terminal-control-keys` - todo; native
  Windows Ctrl-C/control-key interrupt gap in PTY-backed terminals.
- `260523-research-ws-dashboard-persistable-ui-state-map` - idea; backlog
  map for UI state worth persisting beyond what Phase 6/7 (terminal
  visual-buffer and workbench-layout restore) already covers.
- `260525-bug-ws-dashboard-agent-tab-close-confirmation-sticky` - idea;
  agent tab close-confirmation UI bug.
- `260525-feat-ws-dashboard-document-polishing-backlog` - todo; Markdown
  reader and source-editor quality pass.
- `260525-feat-ws-dashboard-workroot-polishing-backlog` - todo; WorkRoot
  lifecycle and Git-toolbar polish.
- `260524-research-ws-dashboard-visual-design-system-refresh` - idea;
  coherent visual-quality pass across the dashboard.
- `260524-research-ws-dashboard-react-aria-ui-primitives` - idea;
  accessibility-primitive adoption research.

## Cross-Child Decisions

- Treat the terminal surface as the nearer-term priority within this
  board: a dashboard that works well as a plain, tmux-like PTY
  environment is more immediately useful than further visual polish.
  Terminal-fidelity children (e.g. control-key behavior) should not sit
  behind visual/design-system work.
- If a polish item turns out to require agent-harness or session-key
  changes to implement correctly, move it to `260622` instead of forcing
  it into this board.

## Completion Criteria

- Done: terminal interaction fidelity, visual/design coherence, and the
  document/WorkRoot UI polish backlog reach a state the owner considers
  presentable. This board does not require net-new feature completeness.
- Dropped: the dashboard UI direction changes wholesale (e.g. a
  different frontend framework or design-system decision supersedes this
  backlog).
- Deferred: any child that turns out to depend on the agent-harness/
  session-key direction defers to `260622`.
