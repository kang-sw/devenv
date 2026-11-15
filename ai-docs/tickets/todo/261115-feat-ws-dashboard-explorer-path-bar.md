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

- **Inside the current root only.** The path input addresses folders inside
  the current root; a path outside it shows the existing refusal and never
  switches roots. Source: the mockup labels the input "path inside the root",
  and the epic forbids widening roots; root changes stay with the root
  switcher and the change-root popover.
- **History lives in memory per inner tab** and is not persisted. Rejected:
  persisting it in per-card browser state - the persisted location already
  restores where the tab was, and a stored history adds a store field and
  stale entries for no visible gain.
- **Ctrl/Cmd+L is scoped to the focused explorer instance.** Rejected: a
  page-wide shortcut - the browser's own Ctrl/Cmd+L must keep working
  elsewhere, and several explorer instances may be on the page.
- Settled 2026-11-15 by MAIN acknowledgement of the announced and technical
  calls (no owner policy question).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Read-only and client-side: no route change, no new symlink permission; the
  existing `/fs/list` guards decide what may be listed.
- Implementation waits for the card ticket's Phase 1 and for MAIN's start
  signal after `261114-refactor-ws-dashboard-whiteboard-asset-split`.

## Phases

### Phase 1: Path bar, history and up

Depends on the card ticket's Phase 1 (navigation row and inner tabs).

- An editable path input shows the active inner tab's directory (the list
  answer's `Abs`); Enter navigates through `/fs/list` with that path, Esc
  restores the shown path. A refused path (outside the root, a symlink, a
  missing folder) shows the existing refusal text and keeps the current
  listing.
- Back, forward and up buttons; back/forward follow an in-memory history per
  inner tab, up goes to the parent and is disabled at the root.
- Ctrl/Cmd+L focuses the input while focus is inside that instance.

Verification: browser checks for navigation inside the root, refusal outside
it and through a symlink without losing the listing, independent history in
two inner tabs, up disabled at the root, and Ctrl/Cmd+L scoping with two
instances (the browser shortcut untouched outside them); `TestFiles`
neighbours and `./internal/wsi18n` for new strings; side-by-side against the
navigation row of frames 01 and 06.
