---
title: "Explorer path bar with back, forward and up"
parent: 261114-epic-ws-dashboard-file-explorer
related:
  261114-feat-ws-dashboard-reusable-file-explorer-card: "prerequisite: the path bar lives in the card core's navigation row and keeps history per inner tab (Phase 1)"
  261115-feat-ws-dashboard-explorer-root-switcher: "sibling: same navigation row; the root switcher picks the root the path bar addresses"
---

# Explorer path bar with back, forward and up

## Background

The approved explorer epic includes a path bar. The approved mockup
([mockup.html](../assets/261114-epic-ws-dashboard-file-explorer/mockup.html),
navigation row in frames 01 and 06) shows back, forward and up buttons and an
editable input labeled "path inside the root", focused by clicking it or with
Ctrl/Cmd+L.

Today (evidence gathered 2026-11-15): files.js has no navigation history and
no editable path input; its only text input is the change-root popover.
`/fs/list` already accepts `dir` relative to the root or absolute inside it
(`containedDir` with `relativeInside`), answers with the absolute directory in
`Abs`, and refuses a path outside the root, a symlink, a missing path or a
non-directory with an indistinguishable 404. `dir=""` addresses the root.

## Decisions

Pending the Open Decision Queue below.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Read-only and client-side: no route change, no new symlink permission; the
  existing `/fs/list` guards decide what may be listed.
- Implementation waits for the card ticket's Phase 1 and for MAIN's start
  signal after `261114-refactor-ws-dashboard-whiteboard-asset-split`.

## Open Decision Queue

Last ID: (13) (working labels; owner-visible questions go to MAIN unnumbered)

(11) [open] [announced] The path input addresses folders inside the current root only; a path outside it shows the existing refusal and never switches roots - the mockup labels the input "path inside the root", and the epic forbids widening roots; root changes stay with the root switcher and the change-root popover.
(12) [open] [technical] Back/forward history is kept in memory per inner tab and is not persisted, over persisting it in per-card browser state - the persisted location already restores where the tab was, and a stored history adds a store field and stale entries for no visible gain.
(13) [open] [technical] Ctrl/Cmd+L focuses the path input only while focus is inside that explorer instance, over a page-wide shortcut - the browser's own Ctrl/Cmd+L (address bar) must keep working everywhere else, and several explorer instances may be on the page.

## Phases

### Phase 1: Path bar, history and up

Drafted after the queue settles: an editable path input showing the active
inner tab's directory, Enter navigates through `/fs/list`, Esc restores the
shown path; back/forward/up buttons with per-tab history; a refused path
shows the existing refusal text without losing the current listing.
Verification will include navigation inside the root, refusal outside it and
through a symlink, history across inner tabs, shortcut scoping with two
instances, and a side-by-side against the navigation row of frames 01 and 06.
