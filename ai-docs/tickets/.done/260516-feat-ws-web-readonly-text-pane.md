---
title: ws web dashboard read-only text pane
parent: 260516-epic-ws-web-dashboard-workroot-io-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workroot-io-substrate: containing milestone
  260516-feat-ws-web-workroot-file-navigation: file navigation source surface
  260516-epic-ws-web-dashboard-workbench-substrate: workbench placement substrate
spec:
  - 260516-ws-web-dashboard-readonly-file-api
  - 260516-ws-web-dashboard-readonly-text-pane
  - 260516-ws-web-dashboard-file-open-placement-policy
plans:
  phase-1: 2026-05/16-260516-feat-ws-web-readonly-text-pane
  phase-2: 2026-05/16-260516-feat-ws-web-readonly-text-pane
  phase-3: 2026-05/16-260516-feat-ws-web-readonly-text-pane
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-16
---

# ws web dashboard read-only text pane

## Background

The dashboard workbench can host editor-like panes, but file opens are not live.
The first editor slice should be deliberately read-only so the dashboard becomes
inspectable without taking on save, dirty-state, conflict, formatting, or
language-server complexity.

## Decisions

- Implement a read-only text pane, not a write-back editor.
- Open files from the workRoot file explorer into the workbench, preferring the
  second or later split group so terminal or future agent work is not displaced.
- Use honest unavailable states for binary, oversized, unreadable, missing, or
  unsupported files.
- Keep file identity daemon-owned. The frontend may carry an opaque file handle
  or relative path token from the daemon, but raw host paths must not become
  browser route identity.
- Do not add save, rename, delete, move, copy, conflict handling, or dirty-state
  UI.

## Phases

### Phase 1: Authenticated Read-Only File API

Add a protected API for reading previewable text files below a selected
workRoot. The route should reject traversal, enforce size and binary guards,
report text metadata needed by the frontend, and return clear unavailable
states without weakening the owner-auth boundary.

### Result (e784c291) - 2026-05-16

Implemented the owner-authenticated read-only file API below opened workRoots,
including traversal, unknown root, missing path, directory, unreadable, binary,
and oversize guards. The route keeps file identity workRoot-relative and uses
bounded unavailable responses without exposing absolute host paths.

### Phase 2: Workbench Text Pane Surface

Add a workbench surface for read-only text content. Opening an already-open file
should focus the existing pane unless the user explicitly asks for another view.
The pane should render as a real editor/viewer body rather than a card or
debug placeholder, while clearly showing read-only status.

### Result (e784c291) - 2026-05-16

Implemented read-only workbench panes for previewable file content. Duplicate
opens focus existing panes, panes are keyed by workRoot-relative identity, and
pane rendering stays read-only without save, dirty-state, terminal, or restore
behavior.

### Phase 3: File-Open Placement Policy

Wire file explorer open actions into workbench placement. File opens should
prefer the second or later split group and preserve active terminal work where
practical. Placement remains browser arrangement state; file content and
preview authorization remain daemon-owned.

### Result (e784c291) - 2026-05-16

Implemented the read-only file preview flow across all three phases. The daemon
now exposes an owner-authenticated read-only file API below opened workRoots,
with traversal, unknown root, missing path, directory, unreadable, binary, and
oversize guards. The frontend reads previewable files from the file explorer and
opens read-only workbench panes keyed by workRoot-relative identity.

Duplicate opens focus existing panes, panes render only under their owning
selected workRoot, and file opens route through the workbench placement policy
so new panes prefer the support split. No write-back editing, save state,
terminal sessions, or restore UX were added.

Verification covered Rust formatting, daemon route tests, the full daemon crate
test suite, workspace check, frontend route/workRoot/workbench tests,
production build, and delegated correctness/fit/test review with one follow-up
fix cycle.
