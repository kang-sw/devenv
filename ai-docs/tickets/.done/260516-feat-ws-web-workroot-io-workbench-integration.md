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
plans:
  phase-1: 2026-05/16-260516-feat-ws-web-workroot-io-workbench-integration
  phase-2: 2026-05/16-260516-feat-ws-web-workroot-io-workbench-integration
  phase-3: 2026-05/16-260516-feat-ws-web-workroot-io-workbench-integration
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-16
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

### Result (d9d4bfbf) - 2026-05-16

Tightened terminal restore reconciliation so daemon live session listing removes
stale terminal panes for the selected workRoot while preserving panes created
after an older list request started. This keeps daemon terminal state
authoritative without letting stale async responses remove newly-created live
panes.

### Phase 2: Placement And Command Polish

Ensure file-open, create-terminal, focus-existing-surface, close-terminal, and
refresh commands use consistent command ids and placement behavior. Avoid
duplicating surfaces when the logical target is already open.

### Result (d9d4bfbf) - 2026-05-16

Verified the existing file-open, create-terminal, focus-existing, close-terminal,
and refresh command/placement behavior across the completed file, read-only pane,
and terminal surfaces. No extra command-system rewrite was needed; the follow-up
source change stayed focused on terminal reconciliation.

### Phase 3: End-To-End Dogfood Verification

Run the dashboard through the production-served frontend and verify the owner
workflow: open/select workRoot, browse files, open a read-only text pane, create
and use a terminal, refresh without losing the terminal, close the terminal,
and inspect narrow and desktop layouts. Capture screenshots or record exact
tooling blockers.

### Result (d9d4bfbf) - 2026-05-16

Recorded daemon-served production frontend dogfood in
`ai-docs/.plans/2026-05/16-260516-feat-ws-web-workroot-io-workbench-integration.dogfood.md`.
The flow opened `/Users/kang-sw/devenv` as a workRoot, listed files, read
`README.md`, created a terminal, sent `printf ws-dashboard-terminal\n`, observed
output, listed the live terminal, closed it, and confirmed the live list was
empty.

Interactive browser/screenshot tooling was unavailable, so desktop and narrow
visual breakpoint inspection remains weakly verified by build/tests and is
recorded as the exact dogfood blocker rather than silently claimed.
