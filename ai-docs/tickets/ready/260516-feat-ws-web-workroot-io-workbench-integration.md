---
title: ws web dashboard workRoot IO workbench integration
parent: 260516-epic-ws-web-dashboard-workroot-io-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workroot-io-substrate: containing milestone
  260516-feat-ws-web-workroot-file-navigation: file navigation child
  260516-feat-ws-web-readonly-text-pane: read-only text pane child
  260516-feat-ws-web-terminal-session-substrate: terminal session child
  260516-feat-ws-web-workbench-editor-chrome-polish: current workbench chrome baseline
spec:
  - 260516-ws-web-dashboard-workroot-io-restore-model
  - 260516-ws-web-dashboard-workroot-io-command-placement-polish
  - 260516-ws-web-dashboard-workroot-io-dogfood-verification
related-mental-model:
  - ws-web-dashboard
---

# ws web dashboard workRoot IO workbench integration

## Background

Filesystem and terminal slices are useful only if the workbench presents them
as one coherent workRoot workflow. This ticket owns the cross-child integration
and dogfood gate after file navigation, read-only text panes, and terminal
sessions exist.

## Decisions

- Keep daemon-owned resource existence separate from browser-owned arrangement.
- Restore live terminal panes from daemon terminal session state after browser
  refresh.
- Restore file pane arrangement only where the file is still previewable; show
  honest unavailable state if the file disappeared, became unreadable, or no
  longer passes preview guards.
- Preserve the default workbench intent: terminal work should not be displaced
  by file opens when a support split is available.
- Verify the actual daemon-served frontend, not only isolated model tests.

## Phases

### Phase 1: Cross-Surface Restore Model

Define and implement the restore model that combines daemon live terminal
sessions, read-only file pane state, and browser workbench arrangement. Daemon
state remains authoritative for live terminal existence; browser arrangement
remains presentation state.

### Phase 2: Placement And Command Polish

Ensure file-open, create-terminal, focus-existing-surface, close-terminal, and
refresh commands use consistent command ids and placement behavior. Avoid
duplicating surfaces when the logical target is already open.

### Phase 3: End-To-End Dogfood Verification

Run the dashboard through the production-served frontend and verify the owner
workflow: open/select workRoot, browse files, open a read-only text pane, create
and use a terminal, refresh without losing the terminal, close the terminal,
and inspect narrow and desktop layouts. Capture screenshots or record exact
tooling blockers.
